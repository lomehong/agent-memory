package core

import (
	"math"
	"time"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/model"
)

// HeatScorer calculates heat scores for memories.
// Heat score combines frequency and recency of access.
type HeatScorer struct {
	config *config.Config
}

// NewHeatScorer creates a new heat scorer.
func NewHeatScorer(cfg *config.Config) *HeatScorer {
	return &HeatScorer{config: cfg}
}

// Score calculates the heat score for a memory.
// Formula: freqScore = log(accessCount + 1) * 10
//          recencyScore = max(0, 100 - daysSinceAccess * 5)
//          final = freqScore * 0.6 + recencyScore * 0.4
func (h *HeatScorer) Score(memory *model.Memory) float64 {
	freqWeight := h.config.Evolution.Heat.FrequencyWeight
	recencyWeight := h.config.Evolution.Heat.RecencyWeight
	if freqWeight == 0 {
		freqWeight = 0.6
	}
	if recencyWeight == 0 {
		recencyWeight = 0.4
	}

	// Frequency score: logarithmic scale
	freqScore := math.Log(float64(memory.AccessCount+1)) * 10

	// Recency score: linear decay
	daysSinceAccess := time.Since(memory.LastAccessed).Hours() / 24
	recencyScore := math.Max(0, 100-daysSinceAccess*5)

	// Combined score
	finalScore := freqScore*freqWeight + recencyScore*recencyWeight

	return finalScore
}
