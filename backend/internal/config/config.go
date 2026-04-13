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
	Evolution  EvolutionConfig   `yaml:"evolution"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// AgentEntry defines a pre-registered agent. Corresponds to REQ-020.
// DESIGN-021: user_id is the unique identity for memory isolation.
// api_key is for permission validation only, not identity distinction.
type AgentEntry struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	UserID string `yaml:"user_id"` // 身份隔离标识，每个 Agent 必须独立

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

// EvolutionConfig defines memory evolution parameters.
// Corresponds to REQ-030: Dream & Review evolution.
type EvolutionConfig struct {
	Dream  DreamConfig  `yaml:"dream"`
	Review ReviewConfig `yaml:"review"`
	LLM    LLMConfig    `yaml:"llm"`
	Heat   HeatConfig   `yaml:"heat"`
}

// DreamConfig defines dream evolution parameters.
type DreamConfig struct {
	Enabled             bool    `yaml:"enabled"`
	MinMemories         int     `yaml:"min_memories"`           // 最少记忆数才触发，默认 3
	MaxInsights         int     `yaml:"max_insights"`           // 单次最多生成洞察数，默认 10
	DefaultLookbackDays int     `yaml:"default_lookback_days"`  // 默认回溯天数，默认 7
	PatternThreshold    float64 `yaml:"pattern_threshold"`     // 重复模式相似度阈值，默认 0.7
	InsightDedupThreshold float64 `yaml:"insight_dedup_threshold"` // 洞察去重阈值，默认 0.85
	MaxSourceMemories   int     `yaml:"max_source_memories"`   // 单条洞察最大关联源记忆数，默认 20
	RunIntervalHours    int     `yaml:"run_interval_hours"`
}

func (d DreamConfig) GetDefaultLookbackDays() int {
	if d.DefaultLookbackDays > 0 { return d.DefaultLookbackDays }
	return 7
}
func (d DreamConfig) GetPatternThreshold() float64 {
	if d.PatternThreshold > 0 { return d.PatternThreshold }
	return 0.7
}
func (d DreamConfig) GetInsightDedupThreshold() float64 {
	if d.InsightDedupThreshold > 0 { return d.InsightDedupThreshold }
	return 0.85
}
func (d DreamConfig) GetMaxSourceMemories() int {
	if d.MaxSourceMemories > 0 { return d.MaxSourceMemories }
	return 20
}

// ReviewConfig defines memory review parameters.
type ReviewConfig struct {
	Enabled            bool     `yaml:"enabled"`
	MinConfidence      float64  `yaml:"min_confidence"`        // 最低置信度，默认 0.5
	StaleThresholdDays int     `yaml:"stale_threshold_days"` // 陈旧记忆天数阈值，默认 30
	ErrorKeywords      []string `yaml:"error_keywords"`       // 五问分析错误关键词
	RunIntervalHours   int     `yaml:"run_interval_hours"`
}

func (r ReviewConfig) GetMinConfidence() float64 {
	if r.MinConfidence > 0 { return r.MinConfidence }
	return 0.5
}
func (r ReviewConfig) GetStaleThresholdDays() int {
	if r.StaleThresholdDays > 0 { return r.StaleThresholdDays }
	return 30
}
func (r ReviewConfig) GetErrorKeywords() []string {
	if len(r.ErrorKeywords) > 0 { return r.ErrorKeywords }
	return []string{"失败", "错误", "bug", "问题", "异常", "error", "fail", "超时", "timeout"}
}

// LLMConfig defines LLM client parameters for evolution.
type LLMConfig struct {
	Enabled        bool   `yaml:"enabled"`
	APIKey         string `yaml:"api_key"`
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	MaxTokens      int    `yaml:"max_tokens"`
}

func (l LLMConfig) GetTimeoutSeconds() int {
	if l.TimeoutSeconds > 0 { return l.TimeoutSeconds }
	return 10
}
func (l LLMConfig) GetMaxTokens() int {
	if l.MaxTokens > 0 { return l.MaxTokens }
	return 2000
}

// HeatConfig defines heat scoring parameters.
type HeatConfig struct {
	FrequencyWeight    float64 `yaml:"frequency_weight"`     // 访问频率权重，默认 0.6
	RecencyWeight      float64 `yaml:"recency_weight"`       // 时间衰减权重，默认 0.4
	HeatThreshold      float64 `yaml:"heat_threshold"`       // 降级热度阈值，默认 30
	ExtensionMultiplier float64 `yaml:"extension_multiplier"` // 高热度 TTL 延长倍数，默认 1.5
}

func (h HeatConfig) GetHeatThreshold() float64 {
	if h.HeatThreshold > 0 { return h.HeatThreshold }
	return 30
}
func (h HeatConfig) GetExtensionMultiplier() float64 {
	if h.ExtensionMultiplier > 0 { return h.ExtensionMultiplier }
	return 1.5
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
