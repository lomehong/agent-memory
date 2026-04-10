package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lomehong/agent-memory/internal/model"
	"github.com/rs/zerolog"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements DAL using SQLite.
type SQLiteStore struct {
	db     *sql.DB
	logger *zerolog.Logger
}

// Init creates a new SQLiteStore and initializes the database schema.
func Init(dbPath string, logger *zerolog.Logger) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", dbPath, err)
	}
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}
	store := &SQLiteStore{db: db, logger: logger}
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}
	return store, nil
}

func (s *SQLiteStore) createTables() error {
	ctx := context.Background()
	queries := []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			team TEXT NOT NULL DEFAULT '',
			visibility TEXT NOT NULL DEFAULT 'private',
			content TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'working',
			priority INTEGER NOT NULL DEFAULT 3,
			source TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 1.0,
			ttl TEXT NOT NULL DEFAULT 'month',
			tags TEXT NOT NULL DEFAULT '[]',
			version INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_accessed DATETIME NOT NULL,
			access_count INTEGER NOT NULL DEFAULT 0,
			merged_from TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_user_id ON memories(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_agent_id ON memories(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_team ON memories(team)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_status ON memories(status)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_visibility ON memories(visibility)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_user_vis ON memories(user_id, visibility)`,
		`CREATE TABLE IF NOT EXISTS memory_logs (
			id TEXT PRIMARY KEY,
			memory_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_logs_memory_id ON memory_logs(memory_id)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			user_id TEXT NOT NULL,
			team TEXT NOT NULL DEFAULT '',
			api_key_hash TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_user_id ON agents(user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_api_key_hash ON agents(api_key_hash)`,
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("DDL: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteStore) CreateMemory(ctx context.Context, m *model.Memory) error {
	tagsJSON, _ := json.Marshal(m.Tags)
	mergedJSON, _ := json.Marshal(m.MergedFrom)
	query := `INSERT INTO memories (id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		m.ID, m.UserID, m.AgentID, m.Team, m.Visibility, m.Content, m.Category,
		m.Priority, m.Source, m.Confidence, m.TTL, string(tagsJSON), m.Version, m.Status,
		m.CreatedAt.UTC(), m.UpdatedAt.UTC(), m.LastAccessed.UTC(), m.AccessCount, string(mergedJSON))
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetMemory(ctx context.Context, id string) (*model.Memory, error) {
	query := `SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE id = ? AND status != ?`
	row := s.db.QueryRowContext(ctx, query, id, model.StatusDeleted)
	return s.scanMemory(row)
}

func (s *SQLiteStore) UpdateMemory(ctx context.Context, m *model.Memory) error {
	tagsJSON, _ := json.Marshal(m.Tags)
	mergedJSON, _ := json.Marshal(m.MergedFrom)
	query := `UPDATE memories SET content=?, category=?, priority=?, visibility=?,
		ttl=?, tags=?, version=?, status=?, updated_at=?, last_accessed=?, access_count=?,
		merged_from=? WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query,
		m.Content, m.Category, m.Priority, m.Visibility,
		m.TTL, string(tagsJSON), m.Version, m.Status, m.UpdatedAt.UTC(),
		m.LastAccessed.UTC(), m.AccessCount, string(mergedJSON), m.ID)
	if err != nil {
		return fmt.Errorf("update memory %s: %w", m.ID, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory %s not found", m.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteMemory(ctx context.Context, id string) error {
	query := `UPDATE memories SET status=?, updated_at=? WHERE id = ? AND status != ?`
	result, err := s.db.ExecContext(ctx, query, model.StatusDeleted, timeNow().UTC(), id, model.StatusDeleted)
	if err != nil {
		return fmt.Errorf("delete memory %s: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("memory %s not found", id)
	}
	return nil
}

// ListMemories lists memories with optional filters.
func (s *SQLiteStore) ListMemories(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, error) {
	conditions, args := buildFilterConditions(filter)
	query := fmt.Sprintf(`SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE %s ORDER BY updated_at DESC`, strings.Join(conditions, " AND "))
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}
	return s.queryMemories(ctx, query, args)
}

// ListVisibleMemories lists memories visible to a specific agent.
// Implements the visibility rules from DESIGN-008 and REQ-021:
//   - private: only the creating agent
//   - team: all agents in the same team
//   - user: all agents of the same user
func (s *SQLiteStore) ListVisibleMemories(ctx context.Context, userID, agentID, team string, extraFilters model.MemoryFilter) ([]*model.Memory, error) {
	conditions, args := buildFilterConditions(extraFilters)
	// Override/add user_id filter
	conditions = append(conditions, "user_id = ?")
	args = append(args, userID)
	// Visibility rule: agent sees private(self) + team(same team) + user(all)
	visCondition := "(agent_id = ? OR (visibility = 'team' AND team = ?) OR visibility = 'user')"
	conditions = append(conditions, visCondition)
	args = append(args, agentID, team)

	query := fmt.Sprintf(`SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE %s ORDER BY updated_at DESC`,
		strings.Join(conditions, " AND "))
	if extraFilters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", extraFilters.Limit)
	}
	return s.queryMemories(ctx, query, args)
}

func (s *SQLiteStore) GetMemoriesByIDs(ctx context.Context, ids []string) ([]*model.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE id IN (%s) AND status != ?`,
		strings.Join(placeholders, ","))
	args = append(args, model.StatusDeleted)
	return s.queryMemories(ctx, query, args)
}

func (s *SQLiteStore) IncrementAccessCount(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE memories SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`, timeNow().UTC(), id)
	return err
}

func (s *SQLiteStore) UpdateLastAccessed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE memories SET last_accessed = ? WHERE id = ?`, timeNow().UTC(), id)
	return err
}

func (s *SQLiteStore) CreateAgent(ctx context.Context, a *model.Agent) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO agents (id, name, user_id, team, api_key_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.UserID, a.Team, a.APIKeyHash, a.CreatedAt.UTC())
	return err
}

func (s *SQLiteStore) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	query := `SELECT id, name, user_id, team, api_key_hash, created_at FROM agents WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)
	return s.scanAgent(row)
}

func (s *SQLiteStore) ListAgents(ctx context.Context, userID string) ([]*model.Agent, error) {
	var query string
	var args []interface{}
	if userID != "" {
		query = `SELECT id, name, user_id, team, api_key_hash, created_at FROM agents WHERE user_id = ? ORDER BY created_at DESC`
		args = []interface{}{userID}
	} else {
		query = `SELECT id, name, user_id, team, api_key_hash, created_at FROM agents ORDER BY created_at DESC`
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer rows.Close()
	var agents []*model.Agent
	for rows.Next() {
		a, err := s.scanAgentFromRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *SQLiteStore) DeleteAgent(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete agent %s: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) GetAgentByAPIKeyHash(ctx context.Context, hash string) (*model.Agent, error) {
	query := `SELECT id, name, user_id, team, api_key_hash, created_at FROM agents WHERE api_key_hash = ?`
	row := s.db.QueryRowContext(ctx, query, hash)
	return s.scanAgent(row)
}

func (s *SQLiteStore) CreateLog(ctx context.Context, log *model.MemoryLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO memory_logs (id, memory_id, agent_id, user_id, action, details, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.MemoryID, log.AgentID, log.UserID, log.Action, log.Details, log.CreatedAt.UTC())
	return err
}

func (s *SQLiteStore) BatchCreateMemories(ctx context.Context, memories []*model.Memory) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO memories (id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	defer stmt.Close()
	for _, m := range memories {
		tagsJSON, _ := json.Marshal(m.Tags)
		mergedJSON, _ := json.Marshal(m.MergedFrom)
		if _, err := stmt.ExecContext(ctx,
			m.ID, m.UserID, m.AgentID, m.Team, m.Visibility, m.Content, m.Category,
			m.Priority, m.Source, m.Confidence, m.TTL, string(tagsJSON), m.Version, m.Status,
			m.CreatedAt.UTC(), m.UpdatedAt.UTC(), m.LastAccessed.UTC(), m.AccessCount, string(mergedJSON)); err != nil {
			return fmt.Errorf("insert memory %s in batch: %w", m.ID, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetMemoryCountByAgent(ctx context.Context, agentID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories WHERE agent_id = ? AND status = ?`, agentID, model.StatusActive).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count memories for agent %s: %w", agentID, err)
	}
	return count, nil
}

func (s *SQLiteStore) GetMemoriesByStatus(ctx context.Context, status string, limit int) ([]*model.Memory, error) {
	query := `SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE status = ? ORDER BY updated_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s.queryMemories(ctx, query, []interface{}{status})
}

func (s *SQLiteStore) GetTopAccessedMemories(ctx context.Context, userID string, limit int) ([]*model.Memory, error) {
	query := `SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE user_id = ? AND status = ?
		ORDER BY access_count DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s.queryMemories(ctx, query, []interface{}{userID, model.StatusActive})
}

func (s *SQLiteStore) GetZeroAccessMemories(ctx context.Context, userID string, limit int) ([]*model.Memory, error) {
	query := `SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories WHERE user_id = ? AND status = ? AND access_count = 0
		ORDER BY updated_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s.queryMemories(ctx, query, []interface{}{userID, model.StatusActive})
}

func (s *SQLiteStore) GetStaleMemories(ctx context.Context, userID string, hoursThreshold int, limit int) ([]*model.Memory, error) {
	query := `SELECT id, user_id, agent_id, team, visibility, content, category,
		priority, source, confidence, ttl, tags, version, status, created_at, updated_at,
		last_accessed, access_count, merged_from FROM memories
		WHERE user_id = ? AND status = ? AND ttl != 'permanent'
		AND last_accessed < datetime('now', ? || ' hours')
		ORDER BY last_accessed ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s.queryMemories(ctx, query, []interface{}{userID, model.StatusActive, fmt.Sprintf("-%d", hoursThreshold)})
}

func (s *SQLiteStore) UpdateMemoryStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE memories SET status=?, updated_at=? WHERE id = ?`, status, timeNow().UTC(), id)
	return err
}

// --- Helper functions ---

func buildFilterConditions(filter model.MemoryFilter) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}
	conditions = append(conditions, "status != ?")
	args = append(args, model.StatusDeleted)
	if filter.UserID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Team != "" {
		conditions = append(conditions, "team = ?")
		args = append(args, filter.Team)
	}
	if filter.Visibility != "" {
		conditions = append(conditions, "visibility = ?")
		args = append(args, filter.Visibility)
	}
	if filter.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, filter.Category)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	return conditions, args
}

func (s *SQLiteStore) queryMemories(ctx context.Context, query string, args []interface{}) ([]*model.Memory, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()
	var memories []*model.Memory
	for rows.Next() {
		m, err := s.scanMemoryFromRows(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *SQLiteStore) scanMemory(row *sql.Row) (*model.Memory, error) {
	m := &model.Memory{}
	var tagsJSON, mergedJSON, createdAt, updatedAt, lastAccessed string
	err := row.Scan(&m.ID, &m.UserID, &m.AgentID, &m.Team, &m.Visibility, &m.Content, &m.Category,
		&m.Priority, &m.Source, &m.Confidence, &m.TTL, &tagsJSON, &m.Version, &m.Status,
		&createdAt, &updatedAt, &lastAccessed, &m.AccessCount, &mergedJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}
	return m, parseMemoryFields(m, tagsJSON, mergedJSON, createdAt, updatedAt, lastAccessed)
}

func (s *SQLiteStore) scanMemoryFromRows(rows *sql.Rows) (*model.Memory, error) {
	m := &model.Memory{}
	var tagsJSON, mergedJSON, createdAt, updatedAt, lastAccessed string
	err := rows.Scan(&m.ID, &m.UserID, &m.AgentID, &m.Team, &m.Visibility, &m.Content, &m.Category,
		&m.Priority, &m.Source, &m.Confidence, &m.TTL, &tagsJSON, &m.Version, &m.Status,
		&createdAt, &updatedAt, &lastAccessed, &m.AccessCount, &mergedJSON)
	if err != nil {
		return nil, fmt.Errorf("scan memory row: %w", err)
	}
	return m, parseMemoryFields(m, tagsJSON, mergedJSON, createdAt, updatedAt, lastAccessed)
}

func parseMemoryFields(m *model.Memory, tagsJSON, mergedJSON, createdAt, updatedAt, lastAccessed string) error {
	if err := json.Unmarshal([]byte(tagsJSON), &m.Tags); err != nil {
		return fmt.Errorf("unmarshal tags: %w", err)
	}
	if err := json.Unmarshal([]byte(mergedJSON), &m.MergedFrom); err != nil {
		return fmt.Errorf("unmarshal merged_from: %w", err)
	}
	var err error
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}
	m.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return fmt.Errorf("parse updated_at: %w", err)
	}
	m.LastAccessed, err = parseTime(lastAccessed)
	if err != nil {
		return fmt.Errorf("parse last_accessed: %w", err)
	}
	return nil
}

func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05", time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

func (s *SQLiteStore) scanAgent(row *sql.Row) (*model.Agent, error) {
	a := &model.Agent{}
	var createdAt string
	err := row.Scan(&a.ID, &a.Name, &a.UserID, &a.Team, &a.APIKeyHash, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	a.CreatedAt, _ = parseTime(createdAt)
	return a, nil
}

func (s *SQLiteStore) scanAgentFromRows(rows *sql.Rows) (*model.Agent, error) {
	a := &model.Agent{}
	var createdAt string
	err := rows.Scan(&a.ID, &a.Name, &a.UserID, &a.Team, &a.APIKeyHash, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan agent row: %w", err)
	}
	a.CreatedAt, _ = parseTime(createdAt)
	return a, nil
}

var _ DAL = (*SQLiteStore)(nil)
