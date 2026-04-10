package scoring

import (
	"math"

	"github.com/lomehong/agent-memory/internal/model"
)

// ScoreParams contains all parameters for multi-dimensional scoring.
// Corresponds to DESIGN-009.
type ScoreParams struct {
	Similarity      float64 // cosine similarity [0, 1]
	Priority        int     // memory priority [1-5], 1=highest
	AccessCount     int     // number of times accessed
	Category        string  // memory category
	RecencyHours    float64 // hours since last update
	CategoryWeight  float64 // weight from config for this category
	TTL             string  // TTL level (for urgency)
	UrgencyBoost    float64 // boost value for urgency
	UrgencyDays     int     // threshold days for urgency
}

// Weights defines the five scoring dimensions from DESIGN-009.
type Weights struct {
	SimilarityWeight  float64
	PriorityWeight    float64
	AccessCountWeight float64
	CategoryWeight    float64
	UrgencyWeight     float64
}

// DefaultWeights returns DESIGN-009 default weights.
func DefaultWeights() Weights {
	return Weights{
		SimilarityWeight:  0.40,
		PriorityWeight:    0.25,
		AccessCountWeight: 0.15,
		CategoryWeight:    0.10,
		UrgencyWeight:     0.10,
	}
}

// Score computes a composite score from five dimensions.
// score = Σ(normalized_dimension * weight)
func Score(p ScoreParams) float64 {
	w := DefaultWeights()

	simScore := normalizeSimilarity(p.Similarity)
	priScore := normalizePriority(p.Priority)
	accScore := normalizeAccessCount(p.AccessCount)
	catScore := normalizeCategory(p.Category, p.CategoryWeight)
	urgScore := normalizeUrgency(p.TTL, p.RecencyHours, p.UrgencyBoost, p.UrgencyDays)

	total := simScore*w.SimilarityWeight +
		priScore*w.PriorityWeight +
		accScore*w.AccessCountWeight +
		catScore*w.CategoryWeight +
		urgScore*w.UrgencyWeight

	return clamp01(total)
}

// ScoreWithWeights computes score with custom weights.
func ScoreWithWeights(p ScoreParams, w Weights) float64 {
	total := normalizeSimilarity(p.Similarity)*w.SimilarityWeight +
		normalizePriority(p.Priority)*w.PriorityWeight +
		normalizeAccessCount(p.AccessCount)*w.AccessCountWeight +
		normalizeCategory(p.Category, p.CategoryWeight)*w.CategoryWeight +
		normalizeUrgency(p.TTL, p.RecencyHours, p.UrgencyBoost, p.UrgencyDays)*w.UrgencyWeight

	return clamp01(total)
}

// normalizeSimilarity: already [0,1].
func normalizeSimilarity(sim float64) float64 {
	if sim < 0 { return 0 }
	if sim > 1 { return 1 }
	return sim
}

// normalizePriority: maps 1-5 to 1.0-0.0 (inverted, 1 is highest priority).
func normalizePriority(pri int) float64 {
	if pri <= 1 { return 1.0 }
	if pri >= 5 { return 0.0 }
	return 1.0 - float64(pri-1)/4.0
}

// normalizeAccessCount: logarithmic scaling, 0->0, 1->0.3, 50->0.8, 100+->1.0.
func normalizeAccessCount(count int) float64 {
	if count <= 0 { return 0 }
	if count >= 100 { return 1.0 }
	return 0.3 + 0.7*math.Log(float64(count+1))/math.Log(101)
}

// normalizeCategory: uses config weight (already normalized externally).
func normalizeCategory(category string, configWeight float64) float64 {
	if configWeight > 0 {
		return configWeight
	}
	switch category {
	case model.CategoryIdentity: return 1.0
	case model.CategoryPrinciple: return 0.8
	case model.CategoryKnowledge: return 0.6
	case model.CategoryWorking: return 0.4
	default: return 0.5
	}
}

// normalizeUrgency: working memories approaching TTL expiry get a boost.
// Corresponds to DESIGN-009 urgency dimension.
func normalizeUrgency(ttl string, recencyHours float64, boost float64, thresholdDays int) float64 {
	if ttl == model.TTLPermanent || ttl == "" {
		return 0
	}
	if thresholdDays <= 0 {
		thresholdDays = 7
	}
	if boost <= 0 {
		boost = 0.2
	}
	// Calculate remaining hours based on TTL level
	remainingHours := ttlRemainingHours(ttl) - recencyHours
	if remainingHours <= 0 {
		return boost // Already expired, max urgency
	}
	thresholdHours := float64(thresholdDays) * 24
	if remainingHours <= thresholdHours {
		// Linear ramp from 0 to boost as remaining time approaches 0
		return boost * (1.0 - remainingHours/thresholdHours)
	}
	return 0
}

func ttlRemainingHours(ttl string) float64 {
	switch ttl {
	case model.TTLYear: return 365 * 24
	case model.TTLMonth: return 30 * 24
	case model.TTLWeek: return 7 * 24
	case model.TTLSession: return 24
	default: return 30 * 24
	}
}

func clamp01(v float64) float64 {
	if v < 0 { return 0 }
	if v > 1 { return 1 }
	return v
}
