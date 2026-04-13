package core

import (
	"testing"
	"time"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/model"
)

// --- HeatScorer Tests ---

func TestHeatScorer_Score(t *testing.T) {
	cfg := &config.Config{
		Evolution: config.EvolutionConfig{
			Heat: config.HeatConfig{
				FrequencyWeight: 0.6,
				RecencyWeight:   0.4,
			},
		},
	}
	scorer := NewHeatScorer(cfg)

	now := time.Now()

	tests := []struct {
		name        string
		memory      model.Memory
		minScore    float64
		maxScore    float64
	}{
		{
			name: "brand new memory with no access",
			memory: model.Memory{
				AccessCount:  0,
				LastAccessed: now,
				CreatedAt:    now,
			},
			minScore: 30,
			maxScore: 50,
		},
		{
			name: "highly accessed recent memory",
			memory: model.Memory{
				AccessCount:  100,
				LastAccessed: now,
				CreatedAt:    now,
			},
			minScore: 60,
			maxScore: 75,
		},
		{
			name: "old rarely accessed memory",
			memory: model.Memory{
				AccessCount:  0,
				LastAccessed: now.AddDate(0, 0, -30),
				CreatedAt:    now.AddDate(0, 0, -30),
			},
			minScore: 0,
			maxScore: 20,
		},
		{
			name: "moderately accessed, somewhat old",
			memory: model.Memory{
				AccessCount:  10,
				LastAccessed: now.AddDate(0, 0, -10),
				CreatedAt:    now.AddDate(0, 0, -15),
			},
			minScore: 20,
			maxScore: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.Score(&tt.memory)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("Score %.2f out of expected range [%.2f, %.2f]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestHeatScorer_DefaultConfig(t *testing.T) {
	cfg := &config.Config{} // empty config
	scorer := NewHeatScorer(cfg)

	mem := model.Memory{
		AccessCount:  5,
		LastAccessed: time.Now(),
		CreatedAt:    time.Now().AddDate(0, 0, -7),
	}

	score := scorer.Score(&mem)
	if score <= 0 || score > 100 {
		t.Errorf("Score should be in [0, 100], got %.2f", score)
	}
}

// --- Config Defaults Tests ---

func TestDreamConfigDefaults(t *testing.T) {
	cfg := config.DreamConfig{}
	if cfg.GetDefaultLookbackDays() != 7 {
		t.Errorf("Expected default lookback 7, got %d", cfg.GetDefaultLookbackDays())
	}
	if cfg.GetPatternThreshold() != 0.7 {
		t.Errorf("Expected pattern threshold 0.7, got %.2f", cfg.GetPatternThreshold())
	}
	if cfg.GetInsightDedupThreshold() != 0.85 {
		t.Errorf("Expected dedup threshold 0.85, got %.2f", cfg.GetInsightDedupThreshold())
	}
	if cfg.GetMaxSourceMemories() != 20 {
		t.Errorf("Expected max source 20, got %d", cfg.GetMaxSourceMemories())
	}
}

func TestReviewConfigDefaults(t *testing.T) {
	cfg := config.ReviewConfig{}
	if cfg.GetMinConfidence() != 0.5 {
		t.Errorf("Expected min confidence 0.5, got %.2f", cfg.GetMinConfidence())
	}
	if cfg.GetStaleThresholdDays() != 30 {
		t.Errorf("Expected stale threshold 30, got %d", cfg.GetStaleThresholdDays())
	}
	keywords := cfg.GetErrorKeywords()
	if len(keywords) == 0 {
		t.Error("Expected default error keywords")
	}
}

func TestLLMConfigDefaults(t *testing.T) {
	cfg := config.LLMConfig{}
	if cfg.GetTimeoutSeconds() != 10 {
		t.Errorf("Expected timeout 10, got %d", cfg.GetTimeoutSeconds())
	}
	if cfg.GetMaxTokens() != 2000 {
		t.Errorf("Expected max tokens 2000, got %d", cfg.GetMaxTokens())
	}
}

func TestHeatConfigDefaults(t *testing.T) {
	cfg := config.HeatConfig{}
	if cfg.GetHeatThreshold() != 30 {
		t.Errorf("Expected heat threshold 30, got %.2f", cfg.GetHeatThreshold())
	}
	if cfg.GetExtensionMultiplier() != 1.5 {
		t.Errorf("Expected extension multiplier 1.5, got %.2f", cfg.GetExtensionMultiplier())
	}
}
