package model

import "time"

// Dream evolution and review models.

// DreamRequest initiates a dream review run.
type DreamRequest struct {
	UserID        string `json:"user_id"`
	AgentID       string `json:"agent_id"`
	LookbackDays  int    `json:"lookback_days,omitempty"`  // 回溯天数，默认 7
	DryRun        bool   `json:"dry_run,omitempty"`        // 只分析不写入
}

// DreamReport is the result of a dream run.
type DreamReport struct {
	RunID         string    `json:"run_id"`
	UserID        string    `json:"user_id"`
	AgentID       string    `json:"agent_id"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	TotalMemories int       `json:"total_memories"`
	PatternsFound int       `json:"patterns_found"`
	Insights      []Insight `json:"insights"`
	Errors        []string  `json:"errors,omitempty"`
	FallbackUsed  bool      `json:"fallback_used"`
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
	ID          string    `json:"id"`
	Type        string    `json:"type"`        // pattern, trend, anomaly
	Content     string    `json:"content"`
	SourceIDs   []string  `json:"source_ids"`
	Confidence  float64   `json:"confidence"`
	CreatedAt   time.Time `json:"created_at"`
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
	TotalMemories int           `json:"total_memories"`
	ReviewedCount int           `json:"reviewed_count"`
	Findings      ReviewFindings `json:"findings"`
	Errors        []string      `json:"errors,omitempty"`
	FallbackUsed  bool          `json:"fallback_used"`
}

// ReviewFindings contains all findings from a review run (five-question framework).
type ReviewFindings struct {
	Experience         []FindingItem `json:"experience"`           // Q1: 踩坑经验
	Skills             []FindingItem `json:"skills"`               // Q2: 可复用流程
	Principles         []FindingItem `json:"principles"`           // Q3: 抽象准则
	Insights           []FindingItem `json:"insights"`             // Q4: 跨场景泛化
	Queries            []FindingItem `json:"queries"`              // Q5: 值得留存的问答
	LowConfidence      []FindingItem `json:"low_confidence,omitempty"` // 遗留：低置信度
	Stale              []FindingItem `json:"stale,omitempty"`           // 遗留：陈旧记忆
	RecommendedActions []string      `json:"recommended_actions"`      // 建议行动
}

// FindingItem represents a single finding item.
type FindingItem struct {
	MemoryID   string  `json:"memory_id"`
	Content    string  `json:"content"`
	Reason     string  `json:"reason"`
	Score      float64 `json:"score,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

