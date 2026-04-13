package model

import "time"

// Dream evolution and review models.

// DreamRequest initiates a dream review run.
type DreamRequest struct {
	UserID       string `json:"user_id"`
	AgentID      string `json:"agent_id"`        // "all" 扫描所有 Agent，或指定单个 Agent
	LookbackDays int    `json:"lookback_days"`    // 回溯天数，默认 7
	DryRun       bool   `json:"dry_run"`          // true = 只分析不写入
}

// DreamReport is the result of a dream run.
type DreamReport struct {
	RunID           string    `json:"run_id"`
	UserID          string    `json:"user_id"`
	AgentID         string    `json:"agent_id"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	TotalMemories   int       `json:"total_memories"`
	AgentsScanned   int       `json:"agents_scanned"`
	PatternsFound   int       `json:"patterns_found"`
	InsightsCreated int       `json:"insights_created"`
	InsightsUpdated int       `json:"insights_updated"`
	Insights        []Insight `json:"insights"`
	DurationMs      int64     `json:"duration_ms"`
	LLMUsed         bool      `json:"llm_used"`
	LLMModel        string    `json:"llm_model,omitempty"`
	FallbackUsed    bool      `json:"fallback_used"`
	DryRun          bool      `json:"dry_run"`
	Errors          []string  `json:"errors,omitempty"`
}

// CandidatePattern represents a pattern detected during dream analysis.
type CandidatePattern struct {
	Type        string   `json:"type"`        // duplicate, trend, isolated, conflict
	MemoryIDs   []string `json:"memory_ids"`
	Score       float64  `json:"score"`
	Description string   `json:"description"`
}

// Insight represents a synthesized insight from dream analysis.
type Insight struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`        // pattern, trend, anomaly
	Content    string    `json:"content"`
	SourceIDs  []string  `json:"source_ids"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// ReviewRequest initiates a memory review run.
type ReviewRequest struct {
	AgentID string `json:"agent_id"`
	Since   string `json:"since"`     // ISO 8601 时间，必填
	UseLLM  bool   `json:"use_llm"`   // 是否使用 GLM 增强
}

// ReviewReport is the result of a review run.
type ReviewReport struct {
	RunID         string         `json:"run_id"`
	UserID        string         `json:"user_id"`
	AgentID       string         `json:"agent_id"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   time.Time      `json:"completed_at"`
	TotalMemories int            `json:"total_memories"`
	ReviewedCount int            `json:"reviewed_count"`
	Findings      ReviewFindings `json:"findings"`
	Errors        []string       `json:"errors,omitempty"`
	FallbackUsed  bool           `json:"fallback_used"`
}

// ReviewFindings contains all findings from a review run (five-question framework).
type ReviewFindings struct {
	Experience         []FindingItem `json:"experience"`                // Q1: 踩坑经验
	Skills             []FindingItem `json:"skills"`                    // Q2: 可复用流程
	Principles         []FindingItem `json:"principles"`                // Q3: 抽象准则
	Insights           []FindingItem `json:"insights"`                  // Q4: 跨场景泛化
	Queries            []FindingItem `json:"queries"`                   // Q5: 值得留存的问答
	LowConfidence      []FindingItem `json:"low_confidence,omitempty"`  // 遗留：低置信度
	Stale              []FindingItem `json:"stale,omitempty"`           // 遗留：陈旧记忆
	RecommendedActions []string      `json:"recommended_actions"`       // 建议行动
}

// FindingItem represents a single finding item.
type FindingItem struct {
	MemoryID   string  `json:"memory_id"`
	Content    string  `json:"content"`
	Reason     string  `json:"reason"`
	Score      float64 `json:"score,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// HeatDistribution contains heat score distribution statistics.
type HeatDistribution struct {
	Hot70to100  int `json:"hot_70_100"`   // 高热度
	Warm30to70  int `json:"warm_30_70"`   // 中热度
	Cold0to30   int `json:"cold_0_30"`    // 低热度
}

// HeatReportItem represents a memory with its heat score for report.
type HeatReportItem struct {
	ID             string  `json:"id"`
	ContentPreview string  `json:"content_preview"`
	HeatScore      float64 `json:"heat_score"`
	AccessCount    int     `json:"access_count"`
}

// HeatSection contains heat-related data for the health report.
// Per design §4.4.
type HeatSection struct {
	TopHot        []HeatReportItem `json:"top_hot"`
	TopCold       []HeatReportItem `json:"top_cold"`
	Distribution  HeatDistribution `json:"distribution"`
}
