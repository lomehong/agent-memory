package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/lomehong/agent-memory/pkg/scoring"
	"github.com/rs/zerolog"
)

// Retriever handles memory search and retrieval.
// Corresponds to DESIGN-008.
type Retriever struct {
	dal       storage.DAL
	embedding embedding.EmbeddingProvider
	vector    storage.VectorStore
	config    *config.Config
	logger    *zerolog.Logger
}

func NewRetriever(dal storage.DAL, embedding embedding.EmbeddingProvider, vector storage.VectorStore, cfg *config.Config, logger *zerolog.Logger) *Retriever {
	return &Retriever{dal: dal, embedding: embedding, vector: vector, config: cfg, logger: logger}
}

// DAL exposes the underlying data access layer.
func (r *Retriever) DAL() storage.DAL {
	return r.dal
}

// Search performs semantic search with visibility filtering and multi-dimensional scoring.
// Corresponds to DESIGN-008, DESIGN-009.
func (r *Retriever) Search(ctx context.Context, userID, agentID, team, query string, opts model.SearchOpts) ([]model.MemoryWithScore, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = r.config.Search.GetDefaultPageSize()
	}
	if topK > r.config.Search.GetMaxPageSize() {
		topK = r.config.Search.GetMaxPageSize()
	}

	// Step 1: Generate query embedding.
	vec, err := r.embedding.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	// Step 2: Vector search with user filter.
	vectorFilter := map[string]string{"user_id": userID}
	if opts.Category != "" {
		vectorFilter["category"] = opts.Category
	}

	searchK := topK * 5 // Over-fetch for re-ranking.
	vectorResults, err := r.vector.Search(ctx, vec, searchK, vectorFilter)
	if err != nil {
		r.logger.Error().Err(err).Msg("vector search failed")
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if len(vectorResults) == 0 {
		return []model.MemoryWithScore{}, nil
	}

	// Step 3: Retrieve full records.
	ids := make([]string, len(vectorResults))
	for i, vr := range vectorResults {
		ids[i] = vr.ID
	}
	memories, err := r.dal.GetMemoriesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("retrieve memories: %w", err)
	}
	memMap := make(map[string]*model.Memory, len(memories))
	for _, m := range memories {
		memMap[m.ID] = m
	}

	// Step 4: Apply visibility filter (DESIGN-008 step 0) and multi-dimensional scoring (DESIGN-009).
	now := time.Now().UTC()
	var scored []model.MemoryWithScore
	for _, vr := range vectorResults {
		mem, ok := memMap[vr.ID]
		if !ok || mem == nil {
			continue
		}
		// Skip non-active/degraded.
		if mem.Status != model.StatusActive && mem.Status != model.StatusDegraded {
			continue
		}
		// Visibility filter: private only for same agent, team for same team, user for all.
		if !isVisible(mem, agentID, team) {
			continue
		}
		// Category filter.
		if opts.Category != "" && mem.Category != opts.Category {
			continue
		}

		// Calculate score with 5 dimensions (DESIGN-009).
		params := scoring.ScoreParams{
			Similarity:     vr.Score,
			Priority:       mem.Priority,
			AccessCount:    mem.AccessCount,
			Category:       mem.Category,
			RecencyHours:   now.Sub(mem.UpdatedAt).Hours(),
			CategoryWeight: r.config.Search.CategoryWeight(mem.Category),
			TTL:            mem.TTL,
			UrgencyBoost:   r.config.Search.Urgency.GetBoost(),
			UrgencyDays:    r.config.Search.Urgency.GetThresholdDays(),
		}
		finalScore := scoring.Score(params)

		if opts.MinScore > 0 && finalScore < opts.MinScore {
			continue
		}

		scored = append(scored, model.MemoryWithScore{Memory: *mem, Score: finalScore})
	}

	// Step 5: Sort by score descending.
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	// Step 6: Pagination.
	if len(scored) > topK {
		scored = scored[:topK]
	}

	// Step 7: Update access counts async.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, s := range scored {
			_ = r.dal.IncrementAccessCount(bgCtx, s.ID)
		}
	}()

	return scored, nil
}

// GetMemory retrieves a single memory with permission check and access tracking.
func (r *Retriever) GetMemory(ctx context.Context, userID, agentID, id string) (*model.Memory, error) {
	mem, err := r.dal.GetMemory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	if mem == nil {
		return nil, fmt.Errorf("memory %s not found", id)
	}
	// Admin (empty userID) can access any memory
	if userID != "" {
		if mem.UserID != userID {
			return nil, fmt.Errorf("access denied: memory %s belongs to different user", id)
		}
		if !isVisible(mem, agentID, "") {
			return nil, fmt.Errorf("access denied: memory %s not visible", id)
		}
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.dal.IncrementAccessCount(bgCtx, id)
	}()

	return mem, nil
}

// ListMemories lists visible memories with filters.
func (r *Retriever) ListMemories(ctx context.Context, userID, agentID, team string, filter model.MemoryFilter) ([]*model.Memory, error) {
	return r.dal.ListVisibleMemories(ctx, userID, agentID, team, filter)
}

// GetHealthReport generates a comprehensive health report.
// Corresponds to DESIGN-012.
func (r *Retriever) GetHealthReport(ctx context.Context, userID string) (*model.HealthReport, error) {
	report := &model.HealthReport{
		ByCategory: make(map[string]int),
		ByStatus:   make(map[string]int),
	}

	// Get all visible active memories.
	allFilter := model.MemoryFilter{UserID: userID, Limit: 10000}
	allMemories, err := r.dal.ListVisibleMemories(ctx, userID, "", "", allFilter)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	report.TotalCount = len(allMemories)

	for _, m := range allMemories {
		report.ByCategory[m.Category]++
		report.ByStatus[m.Status]++
	}

	// Top accessed.
	topAccessed, _ := r.dal.GetTopAccessedMemories(ctx, userID, 10)
	for _, m := range topAccessed {
		report.TopAccessed = append(report.TopAccessed, *m)
	}

	// Zero access.
	zeroAccess, _ := r.dal.GetZeroAccessMemories(ctx, userID, 10)
	for _, m := range zeroAccess {
		report.ZeroAccess = append(report.ZeroAccess, *m)
	}

	// Stale (not accessed in 30 days, non-permanent).
	stale, _ := r.dal.GetStaleMemories(ctx, userID, 30*24, 10)
	for _, m := range stale {
		report.StaleMemories = append(report.StaleMemories, *m)
	}

	return report, nil
}

// isVisible checks if a memory is visible to the given agent.
// Corresponds to REQ-021 visibility rules.
func isVisible(mem *model.Memory, agentID, team string) bool {
	switch mem.Visibility {
	case model.VisibilityUser:
		return true
	case model.VisibilityTeam:
		return mem.Team == team
	case model.VisibilityPrivate:
		return mem.AgentID == agentID
	default:
		return false
	}
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}
