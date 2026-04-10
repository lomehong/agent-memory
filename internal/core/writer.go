package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/rs/zerolog"
)

// Writer handles memory creation and deduplication.
// Corresponds to DESIGN-005.
type Writer struct {
	dal       storage.DAL
	embedding embedding.EmbeddingProvider
	vector    storage.VectorStore
	config    *config.Config
	logger    *zerolog.Logger
}

func NewWriter(dal storage.DAL, embedding embedding.EmbeddingProvider, vector storage.VectorStore, cfg *config.Config, logger *zerolog.Logger) *Writer {
	return &Writer{dal: dal, embedding: embedding, vector: vector, config: cfg, logger: logger}
}

// WriteResult contains the created/merged memory and write suggestions.
// Corresponds to REQ-019.
type WriteResult struct {
	Memory     *model.Memory      `json:"memory"`
	Suggestion *model.WriteSuggestion `json:"suggestion"`
}

// Write creates a new memory or merges with an existing duplicate.
// Full flow: validate → infer category → infer visibility → infer priority/TTL →
// generate embedding → semantic dedup (DESIGN-006 thresholds) → merge or create →
// persist to SQLite + vector store → log → return with suggestion.
func (w *Writer) Write(ctx context.Context, userID, agentID, team string, req model.MemoryCreateReq) (*WriteResult, error) {
	now := time.Now().UTC()

	// Step 1: Validate.
	if err := w.validate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// Step 2: Infer category.
	category := req.Category
	if category == "" {
		category = InferCategory(req.Content)
	}
	if !model.ValidCategories[category] {
		return nil, fmt.Errorf("invalid category: %s", category)
	}

	// Step 3: Infer visibility.
	visibility := req.Visibility
	if visibility == "" {
		visibility = InferVisibility(req.Content, category)
	}
	if !model.ValidVisibilities[visibility] {
		return nil, fmt.Errorf("invalid visibility: %s", visibility)
	}

	// Step 4: Infer priority.
	priority := InferPriority(category)
	if req.Priority != nil {
		priority = *req.Priority
		if priority < 1 { priority = 1 }
		if priority > 5 { priority = 5 }
	}

	// Step 5: Infer TTL.
	ttl := InferTTL(category)
	if req.TTL != "" {
		if model.ValidTTLs[req.TTL] {
			ttl = req.TTL
		}
	}

	// Step 6: Infer confidence.
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}

	// Step 7: Generate embedding.
	vec, err := w.embedding.Embed(ctx, req.Content)
	if err != nil {
		w.logger.Error().Err(err).Msg("embedding failed")
		return nil, fmt.Errorf("embedding: %w", err)
	}

	// Step 8: Semantic deduplication (DESIGN-006: threshold per category).
	duplicate, dupScore := w.findDuplicate(ctx, vec, userID, agentID, category)
	suggestion := &model.WriteSuggestion{
		RecommendedCategory:   category,
		RecommendedVisibility: visibility,
		RecommendedPriority:   priority,
		RecommendedTTL:        ttl,
	}

	if duplicate != nil {
		suggestion.DedupHit = true
		suggestion.DedupMemoryID = duplicate.ID
		suggestion.DedupScore = dupScore

		merged, err := w.mergeMemory(ctx, duplicate, req, vec, now)
		if err != nil {
			w.logger.Warn().Err(err).Msg("merge failed, creating new memory instead")
			// Fall through to create new memory
		} else {
			return &WriteResult{Memory: merged, Suggestion: suggestion}, nil
		}
	}

	// Step 9: Check governance limits.
	count, _ := w.dal.GetMemoryCountByAgent(ctx, agentID)
	if count >= w.config.Governance.GetMaxMemoriesPerAgent() {
		return nil, fmt.Errorf("agent %s at max memory limit (%d)", agentID, w.config.Governance.GetMaxMemoriesPerAgent())
	}

	// Step 10: Create memory.
	memory := &model.Memory{
		ID: uuid.New().String(), UserID: userID, AgentID: agentID, Team: team,
		Visibility: visibility, Content: req.Content, Category: category,
		Priority: priority, Source: req.Source, Confidence: confidence,
		TTL: ttl, Tags: req.Tags, Version: 1, Status: model.StatusActive,
		CreatedAt: now, UpdatedAt: now, LastAccessed: now,
		AccessCount: 0, MergedFrom: []string{},
	}

	if err := w.dal.CreateMemory(ctx, memory); err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}

	// Step 11: Persist vector.
	if err := w.vector.Upsert(ctx, memory.ID, vec, vectorMetadata(memory)); err != nil {
		w.logger.Error().Err(err).Str("id", memory.ID).Msg("vector upsert failed")
	}

	w.logAction(ctx, memory.ID, agentID, userID, "create", "memory created")
	w.logger.Info().Str("id", memory.ID).Str("cat", category).Str("vis", visibility).Int("pri", priority).Msg("memory created")

	return &WriteResult{Memory: memory, Suggestion: suggestion}, nil
}

// BatchWrite creates multiple memories.
func (w *Writer) BatchWrite(ctx context.Context, userID, agentID, team string, reqs []model.MemoryCreateReq) ([]*model.Memory, []error) {
	var results []*model.Memory
	var errs []error

	for i, req := range reqs {
		result, err := w.Write(ctx, userID, agentID, team, req)
		if err != nil {
			errs = append(errs, fmt.Errorf("batch[%d]: %w", i, err))
			continue
		}
		results = append(results, result.Memory)
	}
	return results, errs
}

// findDuplicate checks for semantically similar existing memories using DESIGN-006 thresholds.
func (w *Writer) findDuplicate(ctx context.Context, vec []float32, userID, agentID, category string) (*model.Memory, float64) {
	filter := map[string]string{
		"user_id":  userID,
		"category": category,
	}

	results, err := w.vector.Search(ctx, vec, 10, filter)
	if err != nil {
		w.logger.Warn().Err(err).Msg("vector search for dedup failed")
		return nil, 0
	}

	// Use per-category threshold from DESIGN-006.
	threshold := w.config.Dedup.Threshold(category)

	for _, r := range results {
		if r.Score >= threshold {
			mem, err := w.dal.GetMemory(ctx, r.ID)
			if err != nil || mem == nil || mem.Status != model.StatusActive {
				continue
			}
			// For private memories, only match same agent.
			if mem.Visibility == model.VisibilityPrivate && mem.AgentID != agentID {
				continue
			}
			return mem, r.Score
		}
	}
	return nil, 0
}

// mergeMemory merges new content into an existing memory.
func (w *Writer) mergeMemory(ctx context.Context, existing *model.Memory, req model.MemoryCreateReq, vec []float32, now time.Time) (*model.Memory, error) {
	// Merge content if different.
	newContent := existing.Content
	if req.Content != existing.Content && contentDiffers(existing.Content, req.Content) {
		newContent = existing.Content + "\n[Updated] " + req.Content
	}

	// Merge tags.
	mergedTags := mergeTags(existing.Tags, req.Tags)

	existing.Content = newContent
	existing.Tags = mergedTags
	existing.Version++
	existing.UpdatedAt = now
	existing.MergedFrom = append(existing.MergedFrom, "merge:"+now.Format("20060102T150405"))

	if req.Priority != nil && *req.Priority < existing.Priority {
		existing.Priority = *req.Priority
	}

	if err := w.dal.UpdateMemory(ctx, existing); err != nil {
		return nil, fmt.Errorf("update merged memory: %w", err)
	}

	if err := w.vector.Upsert(ctx, existing.ID, vec, vectorMetadata(existing)); err != nil {
		w.logger.Error().Err(err).Str("id", existing.ID).Msg("vector update after merge failed")
	}

	w.logAction(ctx, existing.ID, existing.AgentID, existing.UserID, "merge", "merged with new content")
	w.logger.Info().Str("id", existing.ID).Int("version", existing.Version).Msg("memory merged")

	return existing, nil
}

func (w *Writer) validate(req model.MemoryCreateReq) error {
	if strings.TrimSpace(req.Content) == "" {
		return fmt.Errorf("content is required")
	}
	if len(req.Content) > w.config.Governance.GetMaxContentLength() {
		return fmt.Errorf("content exceeds max length %d", w.config.Governance.GetMaxContentLength())
	}
	if len(req.Tags) > w.config.Governance.GetMaxTagsPerMemory() {
		return fmt.Errorf("exceeds max tags %d", w.config.Governance.GetMaxTagsPerMemory())
	}
	return nil
}

func contentDiffers(a, b string) bool {
	if a == b { return false }
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if len(longer) == 0 { return false }
	return float64(len(shorter))/float64(len(longer)) < 0.8
}

func mergeTags(existing, newTags []string) []string {
	tagSet := make(map[string]bool)
	for _, t := range existing { tagSet[t] = true }
	for _, t := range newTags { tagSet[t] = true }
	result := make([]string, 0, len(tagSet))
	for t := range tagSet { result = append(result, t) }
	return result
}

func (w *Writer) logAction(ctx context.Context, memoryID, agentID, userID, action, details string) {
	log := &model.MemoryLog{
		ID: uuid.New().String(), MemoryID: memoryID, AgentID: agentID,
		UserID: userID, Action: action, Details: details, CreatedAt: time.Now().UTC(),
	}
	if err := w.dal.CreateLog(ctx, log); err != nil {
		w.logger.Error().Err(err).Str("id", memoryID).Msg("log creation failed")
	}
}
