package storage

import (
	"context"
	"time"

	"github.com/lomehong/agent-memory/internal/model"
)

// DAL defines all data access methods.
type DAL interface {
	// Memory CRUD
	CreateMemory(ctx context.Context, m *model.Memory) error
	GetMemory(ctx context.Context, id string) (*model.Memory, error)
	UpdateMemory(ctx context.Context, m *model.Memory) error
	DeleteMemory(ctx context.Context, id string) error
	ListMemories(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, error)
	ListVisibleMemories(ctx context.Context, userID, agentID, team string, extraFilters model.MemoryFilter) ([]*model.Memory, error)
	GetMemoriesByIDs(ctx context.Context, ids []string) ([]*model.Memory, error)
	IncrementAccessCount(ctx context.Context, id string) error
	UpdateLastAccessed(ctx context.Context, id string) error

	// Agent CRUD
	CreateAgent(ctx context.Context, a *model.Agent) error
	GetAgent(ctx context.Context, id string) (*model.Agent, error)
	ListAgents(ctx context.Context, userID string) ([]*model.Agent, error)
	UpdateAgent(ctx context.Context, a *model.Agent) error
	DeleteAgent(ctx context.Context, id string) error
	GetAgentByAPIKeyHash(ctx context.Context, hash string) (*model.Agent, error)
	GetAgentByUserID(ctx context.Context, userID string) (*model.Agent, error)

	// Logging
	CreateLog(ctx context.Context, log *model.MemoryLog) error

	// Batch operations
	BatchCreateMemories(ctx context.Context, memories []*model.Memory) error

	// Stats
	GetMemoryCountByAgent(ctx context.Context, agentID string) (int, error)
	GetMemoriesByStatus(ctx context.Context, status string, limit int) ([]*model.Memory, error)
	GetTopAccessedMemories(ctx context.Context, userID string, limit int) ([]*model.Memory, error)
	GetZeroAccessMemories(ctx context.Context, userID string, limit int) ([]*model.Memory, error)
	GetStaleMemories(ctx context.Context, userID string, hoursThreshold int, limit int) ([]*model.Memory, error)
	UpdateMemoryStatus(ctx context.Context, id, status string) error

	// Close
	Close() error
}

// timeNow is a variable for testing.
var timeNow = time.Now
