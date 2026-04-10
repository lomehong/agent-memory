package model

import "time"

// Agent represents a registered AI agent.
// Corresponds to DESIGN-003-A.
type Agent struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	UserID     string    `json:"user_id" db:"user_id"`
	Team       string    `json:"team" db:"team"`
	APIKeyHash string    `json:"-" db:"api_key_hash"` // never expose
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// AgentRegisterReq is the request to register a new agent.
type AgentRegisterReq struct {
	Name string `json:"name"`
	Team string `json:"team,omitempty"`
}
