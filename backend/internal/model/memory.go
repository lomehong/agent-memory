package model

import "time"

// Visibility constants. Corresponds to REQ-021.
const (
	VisibilityPrivate = "private" // 仅创建者Agent可读写
	VisibilityTeam    = "team"    // 同team所有Agent可读，仅创建者可写
	VisibilityUser    = "user"    // 同user所有Agent可读写
)

// Category constants. Corresponds to REQ-001.
const (
	CategoryIdentity  = "identity"
	CategoryPrinciple = "principle"
	CategoryKnowledge = "knowledge"
	CategoryWorking   = "working"
)

// Status constants. Corresponds to REQ-009.
const (
	StatusActive   = "active"
	StatusDegraded = "degraded"
	StatusArchived = "archived"
	StatusDeleted  = "deleted"
)

// TTL level constants. Corresponds to REQ-008.
const (
	TTLPermanent = "permanent"
	TTLYear      = "year"
	TTLMonth     = "month"
	TTLWeek      = "week"
	TTLSession   = "session"
)

// ValidCategories is the set of valid category values.
var ValidCategories = map[string]bool{
	CategoryIdentity: true, CategoryPrinciple: true,
	CategoryKnowledge: true, CategoryWorking: true,
}

// ValidVisibilities is the set of valid visibility values.
var ValidVisibilities = map[string]bool{
	VisibilityPrivate: true, VisibilityTeam: true, VisibilityUser: true,
}

// ValidStatuses is the set of valid status values.
var ValidStatuses = map[string]bool{
	StatusActive: true, StatusDegraded: true, StatusArchived: true, StatusDeleted: true,
}

// ValidTTLs is the set of valid TTL values.
var ValidTTLs = map[string]bool{
	TTLPermanent: true, TTLYear: true, TTLMonth: true, TTLWeek: true, TTLSession: true,
}

// Memory represents a stored memory record.
// Corresponds to DESIGN-002.
type Memory struct {
	ID           string     `json:"id" db:"id"`
	UserID       string     `json:"user_id" db:"user_id"`
	AgentID      string     `json:"agent_id" db:"agent_id"`
	Team         string     `json:"team" db:"team"`
	Visibility   string     `json:"visibility" db:"visibility"`
	Content      string     `json:"content" db:"content"`
	Category     string     `json:"category" db:"category"`
	Priority     int        `json:"priority" db:"priority"` // 1-5, 1=highest
	Source       string     `json:"source" db:"source"`
	Confidence   float64    `json:"confidence" db:"confidence"` // 0.0-1.0
	TTL          string     `json:"ttl" db:"ttl"`             // permanent/year/month/week/session
	Tags         []string   `json:"tags" db:"tags"`
	Version      int        `json:"version" db:"version"`
	Status       string     `json:"status" db:"status"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	LastAccessed time.Time  `json:"last_accessed" db:"last_accessed"`
	AccessCount  int        `json:"access_count" db:"access_count"`
	MergedFrom   []string   `json:"merged_from" db:"merged_from"`
}

// MemoryCreateReq is the request to create a new memory.
type MemoryCreateReq struct {
	Content    string   `json:"content"`
	Category   string   `json:"category,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
	Priority   *int     `json:"priority,omitempty"`
	Source     string   `json:"source,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	TTL        string   `json:"ttl,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// MemoryUpdateReq is the request to update an existing memory.
type MemoryUpdateReq struct {
	Content    *string  `json:"content,omitempty"`
	Category   *string  `json:"category,omitempty"`
	Priority   *int     `json:"priority,omitempty"`
	Visibility *string  `json:"visibility,omitempty"`
	TTL        *string  `json:"ttl,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Status     *string  `json:"status,omitempty"`
}

// MemoryWithScore wraps a Memory with its search score.
type MemoryWithScore struct {
	Memory
	Score float64 `json:"score"`
}

// MemoryFilter defines filter parameters for listing memories.
type MemoryFilter struct {
	UserID     string `json:"user_id,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
	Team       string `json:"team,omitempty"`
	Visibility string `json:"visibility,omitempty"`
	Category   string `json:"category,omitempty"`
	Status     string `json:"status,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// SearchOpts defines options for searching memories.
type SearchOpts struct {
	TopK       int     `json:"top_k,omitempty"`
	MinScore   float64 `json:"min_score,omitempty"`
	Category   string  `json:"category,omitempty"`
	PageSize   int     `json:"page_size,omitempty"`
	PageToken  string  `json:"page_token,omitempty"`
}

// WriteSuggestion contains the system's suggestions for a memory being written.
// Corresponds to REQ-019.
type WriteSuggestion struct {
	RecommendedCategory   string  `json:"recommended_category,omitempty"`
	RecommendedVisibility string  `json:"recommended_visibility,omitempty"`
	RecommendedPriority   int     `json:"recommended_priority,omitempty"`
	RecommendedTTL        string  `json:"recommended_ttl,omitempty"`
	DedupHit              bool    `json:"dedup_hit"`
	DedupMemoryID         string  `json:"dedup_memory_id,omitempty"`
	DedupScore            float64 `json:"dedup_score,omitempty"`
}

// CompressReport summarizes the result of a compression run.
type CompressReport struct {
	TotalScanned int              `json:"total_scanned"`
	Merged       int              `json:"merged"`
	Archived     int              `json:"archived"`
	Errors       int              `json:"errors"`
	Details      []CompressDetail `json:"details,omitempty"`
}

// CompressDetail describes a single merge/compress operation.
type CompressDetail struct {
	Action    string   `json:"action"`
	SourceIDs []string `json:"source_ids,omitempty"`
	TargetID  string   `json:"target_id,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// HealthReport contains memory health statistics.
// Corresponds to DESIGN-012.
type HealthReport struct {
	TotalCount              int              `json:"total_count"`
	ByCategory              map[string]int    `json:"by_category"`
	ByStatus                map[string]int    `json:"by_status"`
	TopAccessed             []Memory          `json:"top_accessed"`
	ZeroAccess              []Memory          `json:"zero_access"`
	StaleMemories           []Memory          `json:"stale_memories"`
	Heat                    *HeatSection      `json:"heat,omitempty"`
	ChangesSinceLastReport  map[string]int    `json:"changes_since_last_report,omitempty"`
}
