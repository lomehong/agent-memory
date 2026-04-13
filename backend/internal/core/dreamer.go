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
// Corresponds to evolution-design.md §2.
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
	if cfg.Evolution.LLM.Enabled && cfg.Evolution.LLM.APIKey != "" {
		glmClient = llm.NewGLMClient(
			cfg.Evolution.LLM.APIKey,
			cfg.Evolution.LLM.BaseURL,
			cfg.Evolution.LLM.Model,
			cfg.Evolution.LLM.TimeoutSeconds,
			cfg.Evolution.LLM.MaxTokens,
			logger,
		)
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
// agentID="all" scans all registered agents.
// lookbackDays controls the time window (default from config, typically 7).
// dryRun=true skips writing insights to the database.
func (d *Dreamer) Run(ctx context.Context, userID, agentID string, lookbackDays int, dryRun bool) (*model.DreamReport, error) {
	startedAt := time.Now().UTC()

	// Apply defaults
	if lookbackDays <= 0 {
		lookbackDays = d.config.Evolution.Dream.GetDefaultLookbackDays()
	}

	report := &model.DreamReport{
		RunID:      uuid.New().String(),
		UserID:     userID,
		AgentID:    agentID,
		StartedAt:  startedAt,
		DryRun:     dryRun,
	}

	// Resolve agent list
	agentIDs, err := d.resolveAgentIDs(ctx, userID, agentID)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("resolve agents: %v", err))
		report.CompletedAt = time.Now().UTC()
		return report, err
	}
	report.AgentsScanned = len(agentIDs)

	// Time window
	since := time.Now().AddDate(0, 0, -lookbackDays)

	// Process each agent
	for _, aid := range agentIDs {
		agentReport := d.runForAgent(ctx, userID, aid, since, dryRun)
		mergeAgentReport(report, agentReport)
	}

	durationMs := time.Since(startedAt).Milliseconds()
	report.DurationMs = durationMs
	report.CompletedAt = time.Now().UTC()

	// Persist dream run log
	d.logDreamRun(ctx, report)

	return report, nil
}

// resolveAgentIDs returns the list of agent IDs to scan.
// agentID="all" returns all registered agents; otherwise returns [agentID].
func (d *Dreamer) resolveAgentIDs(ctx context.Context, userID, agentID string) ([]string, error) {
	if strings.ToLower(agentID) == "all" {
		agents, err := d.dal.ListAgents(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("list agents: %w", err)
		}
		var ids []string
		for _, a := range agents {
			ids = append(ids, a.ID)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("no registered agents")
		}
		return ids, nil
	}
	return []string{agentID}, nil
}

// runForAgent executes dream analysis for a single agent.
func (d *Dreamer) runForAgent(ctx context.Context, userID, agentID string, since time.Time, dryRun bool) *model.DreamReport {
	report := &model.DreamReport{
		RunID:   uuid.New().String(),
		UserID:  userID,
		AgentID: agentID,
	}

	// Load memories within time window
	filter := model.MemoryFilter{
		UserID:  userID,
		AgentID: agentID,
		Status:  model.StatusActive,
		Limit:   10000,
	}
	memories, err := d.dal.ListMemories(ctx, filter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("load memories: %v", err))
		return report
	}

	// Filter by time window
	var windowed []*model.Memory
	for _, m := range memories {
		if m.CreatedAt.After(since) {
			windowed = append(windowed, m)
		}
	}
	report.TotalMemories = len(windowed)

	minMemories := d.config.Evolution.Dream.MinMemories
	if minMemories == 0 {
		minMemories = 3
	}
	if len(windowed) < minMemories {
		report.Errors = append(report.Errors, fmt.Sprintf("insufficient memories (%d < %d)", len(windowed), minMemories))
		return report
	}

	// Step 1: Detect patterns
	patterns := d.detectPatterns(windowed)
	report.PatternsFound = len(patterns)

	// Step 2: Synthesize insights using LLM or fallback
	insights, fallbackUsed := d.synthesizeInsights(ctx, patterns, windowed)
	report.FallbackUsed = fallbackUsed

	// Step 3: Deduplicate against existing dream insights
	dedupedInsights, created, updated := d.deduplicateInsights(ctx, insights, userID, agentID)
	report.InsightsCreated = created
	report.InsightsUpdated = updated
	report.Insights = dedupedInsights

	// Step 4: Persist insights (skip if dry_run)
	if !dryRun {
		if err := d.persistInsights(ctx, userID, agentID, dedupedInsights); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("persist insights: %v", err))
		}
	}

	report.LLMUsed = !fallbackUsed
	if d.llm != nil {
		report.LLMModel = d.config.Evolution.LLM.Model
	}

	return report
}

// detectPatterns analyzes memories for patterns: duplicates, trends, isolated, conflicts.
func (d *Dreamer) detectPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

	// 3a. Duplicate pattern detection (vector similarity clustering)
	patterns = append(patterns, d.findDuplicatePatterns(memories)...)

	// 3b. Trend pattern detection (category growth)
	patterns = append(patterns, d.findTrendPatterns(memories)...)

	// 3c. Isolated memory detection (low access, old)
	patterns = append(patterns, d.findIsolatedPatterns(memories)...)

	// 3d. Conflict pattern detection (contradictory information)
	patterns = append(patterns, d.findConflictPatterns(memories)...)

	return patterns
}

// findDuplicatePatterns finds semantically similar memories via vector clustering.
// Cluster size >= 3 → candidate duplicate pattern (per design §2.2).
func (d *Dreamer) findDuplicatePatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern
	threshold := d.config.Evolution.Dream.GetPatternThreshold()

	for i, mem1 := range memories {
		if mem1.Category != model.CategoryWorking {
			continue
		}

		vec1, err := d.embedding.Embed(context.Background(), mem1.Content)
		if err != nil {
			continue
		}

		var similarIDs []string
		similarIDs = append(similarIDs, mem1.ID)

		for j, mem2 := range memories {
			if j == i || mem2.Category != mem1.Category {
				continue
			}

			vec2, err := d.embedding.Embed(context.Background(), mem2.Content)
			if err != nil {
				continue
			}

			if cosineSimilarity(vec1, vec2) >= threshold {
				similarIDs = append(similarIDs, mem2.ID)
			}
		}

		// Only report clusters with >= 3 similar memories (per design)
		if len(similarIDs) >= 3 {
			patterns = append(patterns, model.CandidatePattern{
				Type:        "duplicate",
				MemoryIDs:   similarIDs,
				Score:       threshold,
				Description: fmt.Sprintf("Found %d similar working memories (potential duplicate pattern)", len(similarIDs)),
			})
		}
	}

	return patterns
}

// findTrendPatterns finds temporal trends by category growth.
// A category with >= 5 memories in the window suggests a trend.
func (d *Dreamer) findTrendPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

	categoryCounts := make(map[string]int)
	for _, mem := range memories {
		categoryCounts[mem.Category]++
	}

	for cat, count := range categoryCounts {
		if count >= 5 {
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
					Description: fmt.Sprintf("Growing trend in %s category (%d memories in window)", cat, count),
				})
			}
		}
	}

	return patterns
}

// findIsolatedPatterns finds rarely accessed old memories.
// Priority <= 2, access_count == 0, older than 30 days.
func (d *Dreamer) findIsolatedPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern
	const isolationThresholdDays = 30

	for _, mem := range memories {
		daysSinceAccess := time.Since(mem.LastAccessed).Hours() / 24
		if mem.AccessCount == 0 && daysSinceAccess > isolationThresholdDays {
			patterns = append(patterns, model.CandidatePattern{
				Type:        "isolated",
				MemoryIDs:   []string{mem.ID},
				Score:       daysSinceAccess,
				Description: fmt.Sprintf("Never accessed in %.0f days, priority=%d", daysSinceAccess, mem.Priority),
			})
		}
	}

	return patterns
}

// findConflictPatterns finds potentially conflicting information in identity memories.
// Memories with contradiction keywords and high Jaccard similarity.
func (d *Dreamer) findConflictPatterns(memories []*model.Memory) []model.CandidatePattern {
	var patterns []model.CandidatePattern

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
	if len(patterns) == 0 {
		return []model.Insight{}, false
	}

	if d.llm != nil {
		insights, err := d.synthesizeWithLLM(ctx, patterns, memories)
		if err == nil && len(insights) > 0 {
			return insights, false
		}
		d.logger.Warn().Err(err).Msg("LLM synthesis failed, using rule-based fallback")
	}

	return d.synthesizeWithRules(patterns), true
}

// synthesizeWithLLM uses GLM to analyze candidate patterns and generate insights.
// Per design §2.4: sends patterns + memory summaries, asks GLM to validate and describe.
func (d *Dreamer) synthesizeWithLLM(ctx context.Context, patterns []model.CandidatePattern, memories []*model.Memory) ([]model.Insight, error) {
	var patternDescs []string
	for _, p := range patterns {
		patternDescs = append(patternDescs, fmt.Sprintf("[%s] score=%.2f: %s (memories: %v)", p.Type, p.Score, p.Description, p.MemoryIDs))
	}

	// Build memory summaries for context
	var memSummaries []string
	maxSource := d.config.Evolution.Dream.GetMaxSourceMemories()
	for i, m := range memories {
		if i >= maxSource {
			break
		}
		preview := m.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		memSummaries = append(memSummaries, fmt.Sprintf("[%s/%s] %s", m.Category, m.ID[:8], preview))
	}

	summary, err := d.llm.SummarizeDreamPatterns(ctx, patternDescs, memSummaries)
	if err != nil {
		return nil, err
	}

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

// synthesizeWithRules creates insights using simple rules (fallback).
// Per design §2.4: format "[规则生成] <pattern_type>: 发现 <N> 条相似记忆"
func (d *Dreamer) synthesizeWithRules(patterns []model.CandidatePattern) []model.Insight {
	var insights []model.Insight

	for _, pattern := range patterns {
		insight := model.Insight{
			ID:         uuid.New().String(),
			Type:       pattern.Type,
			Content:    fmt.Sprintf("[规则生成] %s: %s", pattern.Type, pattern.Description),
			SourceIDs:  pattern.MemoryIDs,
			Confidence: 0.7,
			CreatedAt:  time.Now().UTC(),
		}
		insights = append(insights, insight)
	}

	return insights
}

// deduplicateInsights checks new insights against existing dream insights.
// Per design §2.2 step 5: similarity > threshold → update existing, append source_memories.
func (d *Dreamer) deduplicateInsights(ctx context.Context, newInsights []model.Insight, userID, agentID string) ([]model.Insight, int, int) {
	threshold := d.config.Evolution.Dream.GetInsightDedupThreshold()

	// Load existing memories and filter by source="dream" in code
	filter := model.MemoryFilter{
		UserID:  userID,
		AgentID: agentID,
		Status:  model.StatusActive,
		Limit:   1000,
	}
	allExisting, err := d.dal.ListMemories(ctx, filter)
	if err != nil {
		d.logger.Warn().Err(err).Msg("load existing memories for dedup, skipping")
		return newInsights, len(newInsights), 0
	}

	// Filter to dream-sourced memories
	var existing []*model.Memory
	for _, m := range allExisting {
		if m.Source == "dream" {
			existing = append(existing, m)
		}
	}

	var deduped []model.Insight
	created := 0
	updated := 0

	for _, insight := range newInsights {
		duplicated := false
		for _, ex := range existing {
			sim := jaccardSimilarity(strings.ToLower(insight.Content), strings.ToLower(ex.Content))
			if sim >= threshold {
				// Merge source IDs
				merged := make(map[string]bool)
				for _, id := range insight.SourceIDs {
					merged[id] = true
				}
				if ex.MergedFrom != nil {
					for _, id := range ex.MergedFrom {
						merged[id] = true
					}
				}
				var allIDs []string
				for id := range merged {
					allIDs = append(allIDs, id)
				}

				// Update existing memory with merged sources
				ex.Content = insight.Content
				ex.MergedFrom = allIDs
				ex.Confidence = math.Max(ex.Confidence, insight.Confidence)
				ex.UpdatedAt = time.Now().UTC()
				if err := d.dal.UpdateMemory(ctx, ex); err != nil {
					d.logger.Error().Err(err).Str("id", ex.ID).Msg("update dedup insight failed")
				} else {
					updated++
				}
				duplicated = true
				break
			}
		}
		if !duplicated {
			deduped = append(deduped, insight)
			created++
		}
	}

	return deduped, created, updated
}

// persistInsights saves insights as new memories.
// Per design §2.2 step 6: category=knowledge, priority=2, tags=["dream", <pattern_type>, <date>]
func (d *Dreamer) persistInsights(ctx context.Context, userID, agentID string, insights []model.Insight) error {
	now := time.Now().UTC()
	dateTag := now.Format("2006-01-02")

	for _, insight := range insights {
		mem := &model.Memory{
			ID:           uuid.New().String(),
			UserID:       userID,
			AgentID:      agentID,
			Team:         "",
			Visibility:   model.VisibilityUser,
			Content:      fmt.Sprintf("[Dream Insight] %s: %s", insight.Type, insight.Content),
			Category:     model.CategoryKnowledge,
			Priority:     2,
			Source:       "dream",
			Confidence:   insight.Confidence,
			TTL:          model.TTLMonth,
			Tags:         []string{"dream", insight.Type, dateTag},
			Version:      1,
			Status:       model.StatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
			LastAccessed: now,
			AccessCount:  0,
			MergedFrom:   insight.SourceIDs,
		}

		if err := d.dal.CreateMemory(ctx, mem); err != nil {
			return fmt.Errorf("create insight memory: %w", err)
		}
	}

	return nil
}

// logDreamRun persists the dream run record.
func (d *Dreamer) logDreamRun(ctx context.Context, report *model.DreamReport) {
	// Write to dream_runs table
	d.logger.Info().
		Str("run_id", report.RunID).
		Str("agent_id", report.AgentID).
		Int("total_memories", report.TotalMemories).
		Int("patterns_found", report.PatternsFound).
		Int("insights_created", report.InsightsCreated).
		Int("insights_updated", report.InsightsUpdated).
		Bool("fallback_used", report.FallbackUsed).
		Bool("dry_run", report.DryRun).
		Msg("dream run completed")
}

// StartScheduler runs Dream at configured intervals.
// Per design §9: defaults to daily at 03:00, scanning all agents.
func (d *Dreamer) StartScheduler(ctx context.Context, logger zerolog.Logger) {
	interval := d.config.Evolution.Dream.RunIntervalHours
	if interval <= 0 {
		interval = 24
	}
	logger.Info().Int("interval_hours", interval).Msg("Dream scheduler started")

	ticker := time.NewTicker(time.Duration(interval) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Dream scheduler stopped")
			return
		case <-ticker.C:
			// Only run at 03:00 (per design §9.1)
			now := time.Now()
			if now.Hour() != 3 {
				continue
			}
			logger.Info().Msg("Dream scheduled run starting")
			report, err := d.Run(ctx, "", "all", d.config.Evolution.Dream.GetDefaultLookbackDays(), false)
			if err != nil {
				logger.Error().Err(err).Msg("Dream scheduled run failed")
				continue
			}
			logger.Info().
				Str("run_id", report.RunID).
				Int("agents", report.AgentsScanned).
				Int("memories", report.TotalMemories).
				Int("patterns", report.PatternsFound).
				Int("insights_created", report.InsightsCreated).
				Int("insights_updated", report.InsightsUpdated).
				Bool("llm_used", report.LLMUsed).
				Msg("Dream scheduled run completed")
		}
	}
}

// mergeAgentReport aggregates a single-agent report into the combined report.
func mergeAgentReport(target, source *model.DreamReport) {
	target.TotalMemories += source.TotalMemories
	target.PatternsFound += source.PatternsFound
	target.InsightsCreated += source.InsightsCreated
	target.InsightsUpdated += source.InsightsUpdated
	target.Insights = append(target.Insights, source.Insights...)
	if source.FallbackUsed {
		target.FallbackUsed = true
	}
	if source.LLMUsed {
		target.LLMUsed = true
	}
	if source.LLMModel != "" {
		target.LLMModel = source.LLMModel
	}
	target.Errors = append(target.Errors, source.Errors...)
}

// --- Helper functions ---

// filterByCategory filters memories by category.
func filterByCategory(memories []*model.Memory, category string) []*model.Memory {
	var result []*model.Memory
	for _, mem := range memories {
		if mem.Category == category {
			result = append(result, mem)
		}
	}
	return result
}

// hasPotentialConflict checks if two memories may contain contradictory information.
func hasPotentialConflict(content1, content2 string) bool {
	contradictions := []string{"not", "never", "opposite", "different", "不", "不是", "没有"}
	hasContradiction := false
	for _, kw := range contradictions {
		if strings.Contains(strings.ToLower(content1), kw) && strings.Contains(strings.ToLower(content2), kw) {
			hasContradiction = true
			break
		}
	}
	return hasContradiction && jaccardSimilarity(content1, content2) > 0.3
}

// jaccardSimilarity computes Jaccard similarity between two strings.
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

// splitWords splits a string into words (alphanumeric only).
func splitWords(s string) []string {
	words := make([]string, 0)
	current := ""
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || (ch >= 0x4e00 && ch <= 0x9fff) {
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

// cosineSimilarity computes cosine similarity between two vectors.
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
