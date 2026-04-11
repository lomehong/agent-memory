package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
// Corresponds to DESIGN-018.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Web        WebConfig        `yaml:"web"`
	Agents     []AgentEntry     `yaml:"agents"`
	Storage    StorageConfig    `yaml:"storage"`
	Embedding  EmbeddingConfig  `yaml:"embedding"`
	Dedup      DedupConfig      `yaml:"dedup"`
	Search     SearchConfig     `yaml:"search"`
	TTL        TTLConfig        `yaml:"ttl"`
	Governance GovernanceConfig `yaml:"governance"`
	Logging    LoggingConfig    `yaml:"logging"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// AgentEntry defines a pre-registered agent. Corresponds to REQ-020.
type AgentEntry struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	UserID string `yaml:"user_id"` // 用于记忆隔离的用户标识
	Team   string `yaml:"team"`
	APIKey string `yaml:"api_key"`
}

type StorageConfig struct {
	SQLitePath string       `yaml:"sqlite_path"`
	Vector     VectorConfig `yaml:"vector"`
}

type VectorConfig struct {
	Provider string `yaml:"provider"` // qdrant | memory
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
}

type EmbeddingConfig struct {
	Provider     string `yaml:"provider"`      // local | mock | openai
	Model        string `yaml:"model"`
	ModelPath    string `yaml:"model_path"`
	Dimensions   int    `yaml:"dimensions"`
	OpenAIAPIKey  string `yaml:"openai_api_key"`
	OpenAIBaseURL string `yaml:"openai_base_url"`
}

// DedupConfig defines deduplication thresholds per category.
// Corresponds to DESIGN-006.
type DedupConfig struct {
	Thresholds map[string]float64 `yaml:"thresholds"` // category -> threshold
}

// Threshold returns the dedup threshold for a given category, with fallback.
func (d DedupConfig) Threshold(category string) float64 {
	if d.Thresholds != nil {
		if t, ok := d.Thresholds[category]; ok && t > 0 {
			return t
		}
	}
	// Defaults from DESIGN-006
	switch category {
	case "identity":
		return 0.95
	case "principle":
		return 0.90
	case "knowledge":
		return 0.85
	case "working":
		return 0.70
	default:
		return 0.85
	}
}

// SearchConfig defines search and scoring parameters.
// Corresponds to DESIGN-009.
type SearchConfig struct {
	Scoring         ScoringWeights  `yaml:"scoring"`
	CategoryWeights map[string]float64 `yaml:"category_weights"`
	Urgency         UrgencyConfig   `yaml:"urgency"`
	DefaultPageSize int             `yaml:"default_page_size"`
	MaxPageSize     int             `yaml:"max_page_size"`
}

type ScoringWeights struct {
	SimilarityWeight  float64 `yaml:"similarity_weight"`
	PriorityWeight    float64 `yaml:"priority_weight"`
	AccessCountWeight float64 `yaml:"access_count_weight"`
	CategoryWeight    float64 `yaml:"category_weight"`
	UrgencyWeight     float64 `yaml:"urgency_weight"`
}

func (w ScoringWeights) Similarity() float64 {
	if w.SimilarityWeight > 0 {
		return w.SimilarityWeight
	}
	return 0.40
}
func (w ScoringWeights) Priority() float64 {
	if w.PriorityWeight > 0 {
		return w.PriorityWeight
	}
	return 0.25
}
func (w ScoringWeights) AccessCount() float64 {
	if w.AccessCountWeight > 0 {
		return w.AccessCountWeight
	}
	return 0.15
}
func (w ScoringWeights) Category() float64 {
	if w.CategoryWeight > 0 {
		return w.CategoryWeight
	}
	return 0.10
}
func (w ScoringWeights) Urgency() float64 {
	if w.UrgencyWeight > 0 {
		return w.UrgencyWeight
	}
	return 0.10
}

type UrgencyConfig struct {
	ThresholdDays int     `yaml:"threshold_days"`
	Boost         float64 `yaml:"boost"`
}

func (u UrgencyConfig) GetThresholdDays() int {
	if u.ThresholdDays > 0 {
		return u.ThresholdDays
	}
	return 7
}
func (u UrgencyConfig) GetBoost() float64 {
	if u.Boost > 0 {
		return u.Boost
	}
	return 0.2
}

// CategoryWeight returns the weight for a category in scoring.
func (s SearchConfig) CategoryWeight(category string) float64 {
	if s.CategoryWeights != nil {
		if w, ok := s.CategoryWeights[category]; ok {
			return w
		}
	}
	switch category {
	case "identity":
		return 1.0
	case "principle":
		return 0.8
	case "knowledge":
		return 0.6
	case "working":
		return 0.4
	default:
		return 0.5
	}
}

func (s SearchConfig) GetDefaultPageSize() int {
	if s.DefaultPageSize > 0 {
		return s.DefaultPageSize
	}
	return 10
}
func (s SearchConfig) GetMaxPageSize() int {
	if s.MaxPageSize > 0 {
		return s.MaxPageSize
	}
	return 50
}

// TTLConfig defines TTL lifecycle parameters.
// Corresponds to DESIGN-010.
type TTLConfig struct {
	ScanIntervalHours int `yaml:"scan_interval_hours"`
	DegradeMultiplier int `yaml:"degrade_multiplier"`
	ArchiveMultiplier int `yaml:"archive_multiplier"`
}

func (t TTLConfig) GetScanInterval() time.Duration {
	h := t.ScanIntervalHours
	if h <= 0 {
		h = 6
	}
	return time.Duration(h) * time.Hour
}
func (t TTLConfig) GetDegradeMultiplier() int {
	if t.DegradeMultiplier > 0 {
		return t.DegradeMultiplier
	}
	return 2
}
func (t TTLConfig) GetArchiveMultiplier() int {
	if t.ArchiveMultiplier > 0 {
		return t.ArchiveMultiplier
	}
	return 3
}

// TTLDuration maps TTL level string to duration.
// Corresponds to REQ-008.
func TTLDuration(level string) time.Duration {
	switch level {
	case "permanent":
		return 0
	case "year":
		return 365 * 24 * time.Hour
	case "month":
		return 30 * 24 * time.Hour
	case "week":
		return 7 * 24 * time.Hour
	case "session":
		return 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

type GovernanceConfig struct {
	CompressThreshold  float64 `yaml:"compress_threshold"`
	AutoDeleteDays     int     `yaml:"auto_delete_days"`
	MaxMemoriesPerAgent int    `yaml:"max_memories_per_agent"`
	MaxTagsPerMemory   int     `yaml:"max_tags_per_memory"`
	MaxContentLength   int     `yaml:"max_content_length"`
}

func (g GovernanceConfig) GetCompressThreshold() float64 {
	if g.CompressThreshold > 0 {
		return g.CompressThreshold
	}
	return 0.85
}
func (g GovernanceConfig) GetMaxMemoriesPerAgent() int {
	if g.MaxMemoriesPerAgent > 0 {
		return g.MaxMemoriesPerAgent
	}
	return 10000
}
func (g GovernanceConfig) GetMaxTagsPerMemory() int {
	if g.MaxTagsPerMemory > 0 {
		return g.MaxTagsPerMemory
	}
	return 20
}
func (g GovernanceConfig) GetMaxContentLength() int {
	if g.MaxContentLength > 0 {
		return g.MaxContentLength
	}
	return 10000
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type WebConfig struct {
	JWTSecret      string       `yaml:"jwt_secret"`
	TokenTTLHours  int          `yaml:"token_ttl_hours"`
	Admins         []AdminEntry `yaml:"admins"`
	LoginRateLimit int          `yaml:"login_rate_limit"`
}

type AdminEntry struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type MonitoringConfig struct {
	Enabled     bool   `yaml:"enabled"`
	MetricsPath string `yaml:"metrics_path"`
}

// envVarRe matches ${VAR} or ${VAR:-default} patterns.
var envVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

func expandEnvVars(data []byte) []byte {
	return envVarRe.ReplaceAllFunc(data, func(match []byte) []byte {
		submatch := envVarRe.FindSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		varName := string(submatch[1])
		varDefault := ""
		if len(submatch) >= 4 {
			varDefault = string(submatch[3])
		}
		if val, ok := os.LookupEnv(varName); ok {
			return []byte(val)
		}
		return []byte(varDefault)
	})
}

// Load reads and parses the config file, expanding environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	data = expandEnvVars(data)

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply defaults.
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8100
	}
	if cfg.Storage.SQLitePath == "" {
		cfg.Storage.SQLitePath = "./data/memories.db"
	}
	if cfg.Embedding.Dimensions == 0 {
		cfg.Embedding.Dimensions = 384
	}
	if cfg.Embedding.Provider == "" {
		cfg.Embedding.Provider = "mock"
	}
	if cfg.Storage.Vector.Provider == "" {
		cfg.Storage.Vector.Provider = "memory"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	return &cfg, nil
}

// NormalizeLevel converts level string to lowercase.
func NormalizeLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
