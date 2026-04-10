package model

import "time"

// MemoryLog represents an audit log entry for memory operations.
type MemoryLog struct {
	ID        string    `json:"id" db:"id"`
	MemoryID  string    `json:"memory_id" db:"memory_id"`
	AgentID   string    `json:"agent_id" db:"agent_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Action    string    `json:"action" db:"action"` // create | update | delete | search | merge | archive | access
	Details   string    `json:"details,omitempty" db:"details"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
