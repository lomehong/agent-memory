package core

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/llm"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/rs/zerolog"
)

// Dreamer performs dream evolution: pattern detection and insight synthesis.
type Dreamer struct {
	dal       storage.DAL
	embedding embedding.EmbeddingProvider
	vector    storage.VectorStore
	config    *config.Config
	llm       *llm.GLMClient
	logger    *zerolog.Logger
}

// NewDreamer creates a new dreamer instance.
func NewDreamer(dal storage.DAL, embedding embedding.EmbeddingProvider, vector storage.VectorStore, cfg *config.Config, logger *zerolog.Logger) *Dreamer {
	var glmClient *llm.GLMClient
	if cfg.Evolution.LLM.APIKey != "" {
		glmClient = llm.NewGLMClient(cfg.Evolution.LLM.APIKey, cfg.Evolution.LLM.BaseURL, cfg.Evolution.LLM.Model, logger)
	}

	return &Dreamer{
		dal:       dal,
		embedding: embedding,
		vector:    vector,
		config:    cfg,
		llm:       glmClient,
		logger:    logger,
	}
}

// Run executes a dream cycle for the given agent.
func (d *Dreamer) Run(ctx context.Context, userID, agentID string) (*model.DreamReport, error) {
	startedAt := time.Now().UTC()
	report := &model.DreamReport{
		RunID:       uuid.New().String(),
		UserID:      userID,
		AgentID:     agentID,
		StartedAt:   startedAt,
		CompletedAt: time.Now().UTC(),
	}

	// Load all active memories for the agent
	filter := model.MemoryFilter{
		UserID:  userID,
		AgentID: agentID,
		Status:  model.StatusActive,
		Limit:   10000,
	}
	memories, err := d.dal.ListMemories(ctx, filter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("load memories: %v", err))
		return report, fmt.Errorf("load memories: %w", err)
	}

	report.TotalMemories = len(memories)
	if len(memories) < 3 {
		report.Errors = append(report.Errors, "insufficient memories for analysis")
		return report, nil
	}

	// Detect patterns
	patterns := d.detectPatterns(memories)
	report.PatternsFound = len(patterns)

	// Synthesize insights using LLM or fallback
	insights, fallbackUsed := d.synthesizeInsights(ctx, patterns, memories)
	report.FallbackUsed = fallbackUsed
	report.Insights = insights

	// Write insights as new memories
	if err := d.persistInsights(ctx, userID, agentID, insights); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("persist insights: %v", err))
	}

	// Log the dream run
	d.logDreamRun(ctx, report)

	return report, nil
}

// detectPatterns analyzes memories for patterns: duplicates, trends, isolated, conflicts.
func (d *Dreamer) detectPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

	// 1. Check for duplicates using vector similarity
	duplicates := d.findDuplicatePatterns(memories)
	patterns = append(patterns, duplicates...)

	// 2. Check for trends (memories with similar content over time)
	trends := d.findTrendPatterns(memories)
	patterns = append(patterns, trends...)

	// 3. Check for isolated memories (low access, old)
	isolated := d.findIsolatedPatterns(memories)
	patterns = append(patterns, isolated...)

	// 4. Check for conflicts (contradictory information)
	conflicts := d.findConflictPatterns(memories)
	patterns = append(patterns, conflicts...)

	return patterns
}

// findDuplicatePatterns finds semantically similar memories.
func (d *Dreamer) findDuplicatePatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern
	threshold := 0.85 // High threshold for potential duplicates

	for i, mem1 := range memories {
		if mem1.Category != model.CategoryWorking {
			continue
		}

		vec1, err := d.embedding.Embed(context.Background(), mem1.Content)
		if err != nil {
			continue
		}

		var similarIDs []string
		for j, mem2 := range memories {
			if i >= j || mem2.Category != mem1.Category {
				continue
			}

			vec2, err := d.embedding.Embed(context.Background(), mem2.Content)
			if err != nil {
				continue
			}

			sim := cosineSimilarity(vec1, vec2)
			if sim >= threshold {
				similarIDs = append(similarIDs, mem2.ID)
			}
		}

		if len(similarIDs) > 0 {
			patterns = append(patterns, model.CandidatePattern{
				Type:        "duplicate",
				MemoryIDs:   append([]string{mem1.ID}, similarIDs...),
				Score:       threshold,
				Description: fmt.Sprintf("Found %d similar working memories", len(similarIDs)+1),
			})
		}
	}

	return patterns
}

// findTrendPatterns finds temporal trends in memories.
func (d *Dreamer) findTrendPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

	// Group by category and look for increasing counts over time
	categoryCounts := make(map[string]int)
	for _, mem := range memories {
		categoryCounts[mem.Category]++
	}

	for cat, count := range categoryCounts {
		if count >= 5 {
			// Find sample memories
			var sampleIDs []string
			for _, mem := range memories {
				if mem.Category == cat && len(sampleIDs) < 3 {
					sampleIDs = append(sampleIDs, mem.ID)
				}
			}
			if len(sampleIDs) > 0 {
				patterns = append(patterns, model.CandidatePattern{
					Type:        "trend",
					MemoryIDs:   sampleIDs,
					Score:       float64(count) / 10.0,
					Description: fmt.Sprintf("Growing trend in %s category (%d memories)", cat, count),
				})
			}
		}
	}

	return patterns
}

// findIsolatedPatterns finds rarely accessed memories.
func (d *Dreamer) findIsolatedPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern
	const isolationThreshold = 30 // days

	for _, mem := range memories {
		daysSinceAccess := time.Since(mem.LastAccessed).Hours() / 24
		if mem.AccessCount == 0 && daysSinceAccess > isolationThreshold {
			patterns = append(patterns, model.CandidatePattern{
				Type:        "isolated",
				MemoryIDs:   []string{mem.ID},
				Score:       daysSinceAccess,
				Description: fmt.Sprintf("Never accessed in %.0f days", daysSinceAccess),
			})
		}
	}

	return patterns
}

// findConflictPatterns finds potentially conflicting information.
func (d *Dreamer) findConflictPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

	// Look for memories in identity category with similar but contradictory content
	identityMemories := filterByCategory(memories, model.CategoryIdentity)
	for i, mem1 := range identityMemories {
		for _, mem2 := range identityMemories[i+1:] {
			if hasPotentialConflict(mem1.Content, mem2.Content) {
				patterns = append(patterns, model.CandidatePattern{
					Type:        "conflict",
					MemoryIDs:   []string{mem1.ID, mem2.ID},
					Score:       0.8,
					Description: "Potential conflicting information in identity memories",
				})
			}
		}
	}

	return patterns
}

// synthesizeInsights creates insights from patterns using LLM or rule-based fallback.
func (d *Dreamer) synthesizeInsights(ctx context.Context, patterns []model.CandidatePattern, memories []*model.Memory) ([]model.Insight, bool) {
	if d.llm != nil {
		// Try LLM-based synthesis
		insights, err := d.synthesizeWithLLM(ctx, patterns)
		if err == nil && len(insights) > 0 {
			return insights, false
		}
		d.logger.Warn().Err(err).Msg("LLM synthesis failed, using rule-based fallback")
	}

	// Fallback to rule-based synthesis
	return d.synthesizeWithRules(patterns), true
}

// synthesizeWithLLM uses GLM to synthesize insights.
func (d *Dreamer) synthesizeWithLLM(ctx context.Context, patterns []model.CandidatePattern) ([]model.Insight, error) {
	if len(patterns) == 0 {
		return []model.Insight{}, nil
	}

	var patternDescs []string
	for _, p := range patterns {
		patternDescs = append(patternDescs, fmt.Sprintf("%s: %s", p.Type, p.Description))
	}

	summary, err := d.llm.SummarizeDreamPatterns(ctx, patternDescs)
	if err != nil {
		return nil, err
	}

	// Create a single insight from the summary
	var allSourceIDs []string
	for _, p := range patterns {
		allSourceIDs = append(allSourceIDs, p.MemoryIDs...)
	}

	insight := model.Insight{
		ID:         uuid.New().String(),
		Type:       "pattern",
		Content:    summary,
		SourceIDs:  allSourceIDs,
		Confidence: 0.8,
		CreatedAt:  time.Now().UTC(),
	}

	return []model.Insight{insight}, nil
}

// synthesizeWithRules creates insights using simple rules.
func (d *Dreamer) synthesizeWithRules(patterns []model.CandidatePattern) []model.Insight {
	var insights []model.Insight

	for _, pattern := range patterns {
		insight := model.Insight{
			ID:         uuid.New().String(),
			Type:       pattern.Type,
			Content:    pattern.Description,
			SourceIDs:  pattern.MemoryIDs,
			Confidence: 0.7,
			CreatedAt:  time.Now().UTC(),
		}
		insights = append(insights, insight)
	}

	return insights
}

// persistInsights saves insights as new memories.
func (d *Dreamer) persistInsights(ctx context.Context, userID, agentID string, insights []model.Insight) error {
	now := time.Now().UTC()

	for _, insight := range insights {
		mem := &model.Memory{
			ID:         uuid.New().String(),
			UserID:     userID,
			AgentID:    agentID,
			Team:       "",
			Visibility: model.VisibilityUser,
			Content:    fmt.Sprintf("[Dream Insight] %s: %s", insight.Type, insight.Content),
			Category:   model.CategoryKnowledge,
			Priority:   3,
			Source:     "dream",
			Confidence: insight.Confidence,
			TTL:        model.TTLMonth,
			Tags:       []string{"dream", "insight", insight.Type},
			Version:    1,
			Status:     model.StatusActive,
			CreatedAt:  now,
			UpdatedAt:  now,
			LastAccessed: now,
			AccessCount: 0,
			MergedFrom: []string{},
		}

		if err := d.dal.CreateMemory(ctx, mem); err != nil {
			return fmt.Errorf("create insight memory: %w", err)
		}
	}

	return nil
}

func (d *Dreamer) logDreamRun(ctx context.Context, report *model.DreamReport) {
	log := &model.MemoryLog{
		ID:        uuid.New().String(),
		MemoryID:  "dream-" + report.RunID,
		AgentID:   report.AgentID,
		UserID:    report.UserID,
		Action:    "dream",
		Details:   fmt.Sprintf("Dream run: %d memories, %d patterns, %d insights", report.TotalMemories, report.PatternsFound, len(report.Insights)),
		CreatedAt: time.Now().UTC(),
	}
	if err := d.dal.CreateLog(ctx, log); err != nil {
		d.logger.Error().Err(err).Msg("log dream run failed")
	}
}

// Helper functions

func filterByCategory(memories []*model.Memory, category string) []*model.Memory {
	var result []*model.Memory
	for _, mem := range memories {
		if mem.Category == category {
			result = append(result, mem)
		}
	}
	return result
}

func hasPotentialConflict(content1, content2 string) bool {
	// Simple heuristic: check for contradiction keywords
	contradictions := []string{"not", "never", "opposite", "different"}
	hasContradiction := false
	for _, kw := range contradictions {
		if contains(content1, kw) && contains(content2, kw) {
			hasContradiction = true
			break
		}
	}
	// Check if contents share some words but have contradictions
	return hasContradiction && jaccardSimilarity(content1, content2) > 0.3
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func jaccardSimilarity(s1, s2 string) float64 {
	words1 := make(map[string]bool)
	words2 := make(map[string]bool)
	for _, w := range splitWords(s1) {
		words1[w] = true
	}
	for _, w := range splitWords(s2) {
		words2[w] = true
	}
	intersection := 0
	for w := range words1 {
		if words2[w] {
			intersection++
		}
	}
	union := len(words1) + len(words2) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func splitWords(s string) []string {
	// Simple word split
	words := make([]string, 0)
	current := ""
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			current += string(ch)
		} else if current != "" {
			words = append(words, current)
			current = ""
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
