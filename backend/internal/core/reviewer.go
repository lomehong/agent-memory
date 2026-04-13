package core

import (
	"context"
	"fmt"
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

// Reviewer performs structured memory review using the five-question framework.
// Corresponds to DESIGN: Task Review Analysis.
type Reviewer struct {
	dal       storage.DAL
	vector    storage.VectorStore
	embedding embedding.EmbeddingProvider
	config    *config.Config
	llm       *llm.GLMClient
	logger    *zerolog.Logger
}

// NewReviewer creates a new Reviewer instance.
func NewReviewer(dal storage.DAL, embedding embedding.EmbeddingProvider, vector storage.VectorStore, cfg *config.Config, logger *zerolog.Logger) *Reviewer {
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
	return &Reviewer{
		dal:       dal,
		vector:    vector,
		embedding: embedding,
		config:    cfg,
		llm:       glmClient,
		logger:    logger,
	}
}

// Review performs a comprehensive five-question review of an agent's memories.
func (rv *Reviewer) Review(ctx context.Context, userID, agentID string, since time.Time) (*model.ReviewReport, error) {
	startedAt := time.Now().UTC()
	report := &model.ReviewReport{
		RunID:      fmt.Sprintf("review-%s", uuid.New().String()[:8]),
		UserID:     userID,
		AgentID:    agentID,
		StartedAt:  startedAt,
		CompletedAt: time.Now().UTC(),
	}

	// Load memories within time window
	filter := model.MemoryFilter{
		UserID:  userID,
		AgentID: agentID,
		Status:  model.StatusActive,
		Limit:   10000,
	}
	memories, err := rv.dal.ListMemories(ctx, filter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("load memories: %v", err))
		return report, fmt.Errorf("load memories: %w", err)
	}

	// Filter by time window
	var windowed []*model.Memory
	for _, m := range memories {
		if m.CreatedAt.After(since) {
			windowed = append(windowed, m)
		}
	}
	report.TotalMemories = len(windowed)

	// Category summary
	categoryCount := make(map[string]int)
	for _, m := range windowed {
		categoryCount[m.Category]++
	}

	// Five-question analysis
	errorKeywords := rv.config.Evolution.Review.GetErrorKeywords()

	// Q1: 踩坑了吗？— 错误/失败/问题信号
	rv.findExperience(ctx, windowed, errorKeywords, report)

	// Q2: 可复用流程？— 重复操作模式
	rv.findSkills(ctx, windowed, report)

	// Q3: 抽象准则？— 原则规则缺口
	rv.findPrinciples(ctx, windowed, report)

	// Q4: 跨场景泛化？— 跨 Agent 共享知识
	rv.findInsights(ctx, windowed, report)

	// Q5: 值得留存的问答？— 问答对
	rv.findQueries(ctx, windowed, report)

	// Generate action items
	report.Findings.RecommendedActions = rv.generateActionItems(report)

	// Optionally enhance with LLM
	if rv.llm != nil {
		rv.enhanceWithLLM(ctx, report)
	}

	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// findExperience identifies error/failure/problem patterns (Q1).
func (rv *Reviewer) findExperience(ctx context.Context, memories []*model.Memory, keywords []string, report *model.ReviewReport) {
	for _, mem := range memories {
		if mem.Category != model.CategoryWorking {
			continue
		}
		contentLower := strings.ToLower(mem.Content)
		for _, kw := range keywords {
			if strings.Contains(contentLower, strings.ToLower(kw)) {
				report.Findings.Experience = append(report.Findings.Experience, model.FindingItem{
					MemoryID:  mem.ID,
					Content:   truncateContent(mem.Content, 200),
					Reason:    fmt.Sprintf("Contains error keyword: %q", kw),
					Score:     0.8,
					Confidence: 0.85,
				})
				report.ReviewedCount++
				break // One memory, one finding
			}
		}
	}
}

// findSkills identifies repeated operational patterns (Q2).
func (rv *Reviewer) findSkills(ctx context.Context, memories []*model.Memory, report *model.ReviewReport) {
	// Group working memories by semantic similarity
	workingMems := filterByCategory(memories, model.CategoryWorking)
	if len(workingMems) < 2 {
		return
	}

	// Cluster by embedding similarity
	visited := make(map[int]bool)
	for i, mem1 := range workingMems {
		if visited[i] {
			continue
		}
		vec1, err := rv.embedding.Embed(ctx, mem1.Content)
		if err != nil {
			continue
		}

		var clusterIDs []string
		clusterIDs = append(clusterIDs, mem1.ID)
		visited[i] = true

		for j, mem2 := range workingMems {
			if visited[j] || j == i {
				continue
			}
			vec2, err := rv.embedding.Embed(ctx, mem2.Content)
			if err != nil {
				continue
			}
			if cosineSimilarity(vec1, vec2) >= 0.75 {
				clusterIDs = append(clusterIDs, mem2.ID)
				visited[j] = true
			}
		}

		if len(clusterIDs) >= 2 {
			report.Findings.Skills = append(report.Findings.Skills, model.FindingItem{
				MemoryID:   clusterIDs[0],
				Content:    fmt.Sprintf("Found %d similar working memories (potential reusable process)", len(clusterIDs)),
				Reason:     "Semantic clustering detected repeated operations",
				Score:      0.75,
				Confidence: 0.75,
			})
			report.ReviewedCount++
		}
	}
}

// findPrinciples identifies gaps in principle coverage (Q3).
func (rv *Reviewer) findPrinciples(ctx context.Context, memories []*model.Memory, report *model.ReviewReport) {
	// Check if working memories contain repeated patterns that should be rules
	workingMems := filterByCategory(memories, model.CategoryWorking)
	principleMems := filterByCategory(memories, model.CategoryPrinciple)

	// Simple heuristic: if a category of working memories is large relative to principles
	workingCount := len(workingMems)
	principleCount := len(principleMems)

	if workingCount > 10 && principleCount < 3 {
		report.Findings.Principles = append(report.Findings.Principles, model.FindingItem{
			MemoryID:   "",
			Content:    fmt.Sprintf("High working memory count (%d) but few principles (%d) — consider extracting rules", workingCount, principleCount),
			Reason:     "Working-to-principle ratio suggests missing abstractions",
			Score:      0.6,
			Confidence: 0.6,
		})
	}
}

// findInsights identifies knowledge that could be shared across agents (Q4).
func (rv *Reviewer) findInsights(ctx context.Context, memories []*model.Memory, report *model.ReviewReport) {
	for _, mem := range memories {
		if mem.Visibility != model.VisibilityPrivate {
			continue
		}
		if mem.Category != model.CategoryKnowledge {
			continue
		}
		// Check if this private knowledge has high access count (used frequently)
		if mem.AccessCount >= 3 {
			report.Findings.Insights = append(report.Findings.Insights, model.FindingItem{
				MemoryID:   mem.ID,
				Content:    truncateContent(mem.Content, 200),
				Reason:     fmt.Sprintf("Frequently accessed private knowledge (access_count=%d) — consider promoting to team visibility", mem.AccessCount),
				Score:      float64(mem.AccessCount) * 2,
				Confidence: 0.7,
			})
		}
	}
}

// findQueries identifies valuable Q&A pairs (Q5).
func (rv *Reviewer) findQueries(ctx context.Context, memories []*model.Memory, report *model.ReviewReport) {
	for i, mem := range memories {
		content := strings.TrimSpace(mem.Content)
		// Detect question patterns
		if strings.Contains(content, "?") || strings.Contains(content, "？") {
			// Look for a follow-up answer in subsequent memories
			for _, later := range memories {
				if later.CreatedAt.After(mem.CreatedAt) &&
					later.CreatedAt.Sub(mem.CreatedAt) < 24*time.Hour &&
					later.ID != mem.ID {
					answerContent := strings.ToLower(later.Content)
					questionContent := strings.ToLower(content)
					// Simple heuristic: answer should be longer and share some terms
					if len(later.Content) > len(content) && jaccardSimilarity(questionContent, answerContent) > 0.1 {
						report.Findings.Queries = append(report.Findings.Queries, model.FindingItem{
							MemoryID:   mem.ID,
							Content:    truncateContent(content, 200),
							Reason:     "Question with follow-up answer detected",
							Score:      0.6,
							Confidence: 0.65,
						})
						_ = i // suppress unused warning
						break
					}
				}
			}
		}
	}
}

// generateActionItems creates actionable recommendations based on findings.
func (rv *Reviewer) generateActionItems(report *model.ReviewReport) []string {
	var actions []string

	if len(report.Findings.Experience) > 0 {
		actions = append(actions, fmt.Sprintf("建议将 %d 条踩坑经验写入 knowledge 类记忆", len(report.Findings.Experience)))
	}
	if len(report.Findings.Skills) > 0 {
		actions = append(actions, fmt.Sprintf("建议将 %d 条可复用流程文档化", len(report.Findings.Skills)))
	}
	if len(report.Findings.Principles) > 0 {
		actions = append(actions, "建议从 working 记忆中抽象出更多 principle 规则")
	}
	if len(report.Findings.Insights) > 0 {
		actions = append(actions, fmt.Sprintf("建议将 %d 条高频私有知识提升为 team 可见", len(report.Findings.Insights)))
	}
	if len(report.Findings.Queries) > 0 {
		actions = append(actions, fmt.Sprintf("建议保留 %d 条问答对作为长期知识", len(report.Findings.Queries)))
	}

	return actions
}

// enhanceWithLLM optionally uses GLM to improve action items.
func (rv *Reviewer) enhanceWithLLM(ctx context.Context, report *model.ReviewReport) {
	findingsStr := fmt.Sprintf(
		"踩坑经验: %d条\n可复用流程: %d条\n准则缺口: %d条\n跨场景泛化: %d条\n问答对: %d条",
		len(report.Findings.Experience),
		len(report.Findings.Skills),
		len(report.Findings.Principles),
		len(report.Findings.Insights),
		len(report.Findings.Queries),
	)

	enhanced, err := rv.llm.EnhanceReviewActionItems(ctx, findingsStr)
	if err != nil {
		rv.logger.Warn().Err(err).Msg("LLM enhance failed, using rule-based actions")
		return
	}

	// Replace action items with LLM-enhanced version
	if enhanced != "" {
		report.Findings.RecommendedActions = strings.Split(enhanced, "\n")
		// Clean empty lines
		var cleaned []string
		for _, a := range report.Findings.RecommendedActions {
			a = strings.TrimSpace(a)
			if a != "" {
				cleaned = append(cleaned, a)
			}
		}
		report.Findings.RecommendedActions = cleaned
	}
}
