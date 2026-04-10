package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// VectorStore defines the interface for vector storage operations.
// Corresponds to DESIGN-003, DESIGN-017.
type VectorStore interface {
	Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error
	Search(ctx context.Context, vector []float32, topK int, filter map[string]string) ([]VectorResult, error)
	Delete(ctx context.Context, id string) error
	Close() error
}

// VectorResult represents a single vector search result.
type VectorResult struct {
	ID       string
	Score    float64
	Metadata map[string]string
}

// --- Qdrant implementation (DESIGN-003, DESIGN-016) ---

// QdrantStore implements VectorStore using Qdrant REST API.
type QdrantStore struct {
	baseURL     string
	collection  string
	httpClient  *http.Client
	dims        int
	logger      *zerolog.Logger
	mu          sync.RWMutex
	initialized bool
}

// NewQdrantStore creates a Qdrant-backed vector store.
// The collection parameter specifies the Qdrant collection name.
func NewQdrantStore(host string, port int, collection string, dims int, logger *zerolog.Logger) *QdrantStore {
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	if collection == "" {
		collection = "memories"
	}
	return &QdrantStore{
		baseURL:    baseURL,
		collection: collection,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		dims:       dims,
		logger:     logger,
	}
}

// ensureCollection creates the collection if it doesn't exist.
func (q *QdrantStore) ensureCollection(ctx context.Context) error {
	q.mu.RLock()
	if q.initialized {
		q.mu.RUnlock()
		return nil
	}
	q.mu.RUnlock()

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.initialized {
		return nil
	}

	// Check if collection exists.
	resp, err := q.httpClient.Get(q.baseURL + "/collections/" + q.collection)
	if err == nil && resp.StatusCode == 200 {
		q.initialized = true
		resp.Body.Close()
		return nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Create collection.
	body := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     q.dims,
			"distance": "Cosine",
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, q.baseURL+"/collections/"+q.collection, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create qdrant collection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create qdrant collection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create qdrant collection: status=%d body=%s", resp.StatusCode, string(b))
	}

	q.initialized = true
	q.logger.Info().Str("collection", q.collection).Msg("qdrant collection ready")
	return nil
}

func (q *QdrantStore) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	if err := q.ensureCollection(ctx); err != nil {
		return err
	}
	points := []map[string]interface{}{
		{"id": id, "vector": vector, "payload": metadata},
	}
	body := map[string]interface{}{"points": points}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, q.baseURL+"/collections/"+q.collection+"/points", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("qdrant upsert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	upResp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant upsert: %w", err)
	}
	defer upResp.Body.Close()
	if upResp.StatusCode != 200 && upResp.StatusCode != 201 {
		b, _ := io.ReadAll(upResp.Body)
		return fmt.Errorf("qdrant upsert: status=%d body=%s", upResp.StatusCode, string(b))
	}
	return nil
}

func (q *QdrantStore) Search(ctx context.Context, vector []float32, topK int, filter map[string]string) ([]VectorResult, error) {
	if err := q.ensureCollection(ctx); err != nil {
		return nil, err
	}

	var qFilter map[string]interface{}
	if len(filter) > 0 {
		must := []map[string]interface{}{}
		for k, v := range filter {
			must = append(must, map[string]interface{}{
				"key": k, "match": map[string]interface{}{"value": v},
			})
		}
		qFilter = map[string]interface{}{"must": must}
	}

	body := map[string]interface{}{
		"vector": vector, "top": topK, "with_payload": true,
	}
	if qFilter != nil {
		body["filter"] = qFilter
	}
	data, _ := json.Marshal(body)
	resp, err := q.httpClient.Post(q.baseURL+"/collections/"+q.collection+"/points/search", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search: status=%d body=%s", resp.StatusCode, string(b))
	}

	var result struct {
		Result []struct {
			ID      string                 `json:"id"`
			Score   float64                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode qdrant: %w", err)
	}

	out := make([]VectorResult, len(result.Result))
	for i, r := range result.Result {
		meta := make(map[string]string)
		for k, v := range r.Payload {
			meta[k] = fmt.Sprintf("%v", v)
		}
		out[i] = VectorResult{ID: r.ID, Score: r.Score, Metadata: meta}
	}
	return out, nil
}

func (q *QdrantStore) Delete(ctx context.Context, id string) error {
	if err := q.ensureCollection(ctx); err != nil {
		return err
	}
	body := map[string]interface{}{"points": []string{id}}
	data, _ := json.Marshal(body)
	resp, err := q.httpClient.Post(q.baseURL+"/collections/"+q.collection+"/points/delete", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("qdrant delete: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (q *QdrantStore) Close() error { return nil }

// --- In-memory implementation (for dev/test) ---

// MemoryVectorStore is an in-memory vector store for development and testing.
type MemoryVectorStore struct {
	mu      sync.RWMutex
	vectors map[string][]float32
	metas   map[string]map[string]string
	dims    int
	logger  *zerolog.Logger
}

func NewMemoryVectorStore(dims int, logger *zerolog.Logger) *MemoryVectorStore {
	return &MemoryVectorStore{
		vectors: make(map[string][]float32),
		metas:   make(map[string]map[string]string),
		dims:    dims,
		logger:  logger,
	}
}

func (s *MemoryVectorStore) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(vector) != s.dims {
		return fmt.Errorf("vector dim mismatch: expected %d, got %d", s.dims, len(vector))
	}
	vec := make([]float32, len(vector))
	copy(vec, vector)
	s.vectors[id] = vec
	meta := make(map[string]string, len(metadata))
	for k, v := range metadata {
		meta[k] = v
	}
	s.metas[id] = meta
	return nil
}

func (s *MemoryVectorStore) Search(ctx context.Context, vector []float32, topK int, filter map[string]string) ([]VectorResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(vector) != s.dims {
		return nil, fmt.Errorf("query dim mismatch: expected %d, got %d", s.dims, len(vector))
	}
	queryNorm := vecNorm(vector)
	if queryNorm == 0 {
		return nil, fmt.Errorf("zero norm query vector")
	}

	type scored struct {
		id    string
		score float64
		meta  map[string]string
	}
	var results []scored
	for id, vec := range s.vectors {
		if filter != nil && !matchesFilter(s.metas[id], filter) {
			continue
		}
		vn := vecNorm(vec)
		if vn == 0 {
			continue
		}
		results = append(results, scored{id: id, score: cosineSim(vector, vec, queryNorm, vn), meta: s.metas[id]})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	out := make([]VectorResult, len(results))
	for i, r := range results {
		out[i] = VectorResult{ID: r.id, Score: r.score, Metadata: r.meta}
	}
	return out, nil
}

func (s *MemoryVectorStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.vectors, id)
	delete(s.metas, id)
	return nil
}

func (s *MemoryVectorStore) Close() error { return nil }

func matchesFilter(meta, filter map[string]string) bool {
	for k, v := range filter {
		if meta[k] != v {
			return false
		}
	}
	return true
}

func cosineSim(a, b []float32, normA, normB float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (normA * normB)
}

func vecNorm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}

var _ VectorStore = (*MemoryVectorStore)(nil)
var _ VectorStore = (*QdrantStore)(nil)
