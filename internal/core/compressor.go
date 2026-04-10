package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/rs/zerolog"
)

// Compressor handles memory compression.
// Corresponds to DESIGN-011.
type Compressor struct {
	dal       storage.DAL
	embedding embedding.EmbeddingProvider
	vector    storage.VectorStore
	config    *config.Config
	logger    *zerolog.Logger
}

func NewCompressor(dal storage.DAL, embedding embedding.EmbeddingProvider, vector storage.VectorStore, cfg *config.Config, logger *zerolog.Logger) *Compressor {
	return &Compressor{dal: dal, embedding: embedding, vector: vector, config: cfg, logger: logger}
}

// Compress finds and merges similar memories.
// Corresponds to DESIGN-011 algorithm.
func (c *Compressor) Compress(ctx context.Context, userID, agentID, team string) (*model.CompressReport, error) {
	report := &model.CompressReport{}

	// Get all active working memories visible to this agent.
	filter := model.MemoryFilter{Category: model.CategoryWorking, Limit: 1000}
	memories, err := c.dal.ListVisibleMemories(ctx, userID, agentID, team, filter)
	if err != nil {
		return nil, fmt.Errorf("list working memories: %w", err)
	}
	report.TotalScanned = len(memories)
	if len(memories) == 0 {
		return report, nil
	}

	// Generate embeddings.
	embeddings := make(map[string][]float32)
	for _, m := range memories {
		vec, err := c.embedding.Embed(ctx, m.Content)
		if err != nil {
			c.logger.Warn().Err(err).Str("id", m.ID).Msg("embed failed for compression")
			report.Errors++
			continue
		}
		embeddings[m.ID] = vec
	}

	// Find similar pairs.
	threshold := c.config.Governance.GetCompressThreshold()
	processed := make(map[string]bool)

	for i, m1 := range memories {
		if processed[m1.ID] {
			continue
		}
		vec1, ok := embeddings[m1.ID]
		if !ok {
			continue
		}

		var similar []*model.Memory
		for j := i + 1; j < len(memories); j++ {
			m2 := memories[j]
			if processed[m2.ID] {
				continue
			}
			vec2, ok := embeddings[m2.ID]
			if !ok {
				continue
			}
			if cosineSim(vec1, vec2) >= threshold {
				similar = append(similar, m2)
			}
		}

		if len(similar) > 0 {
			merged, err := c.mergeMemories(ctx, m1, similar)
			if err != nil {
				c.logger.Error().Err(err).Str("id", m1.ID).Msg("merge failed")
				report.Errors++
				continue
			}

			for _, src := range similar {
				if err := c.dal.UpdateMemoryStatus(ctx, src.ID, model.StatusArchived); err != nil {
					report.Errors++
					continue
				}
				_ = c.vector.Delete(ctx, src.ID)
				processed[src.ID] = true
				report.Archived++
			}

			// Update merged vector.
			newVec, _ := c.embedding.Embed(ctx, merged.Content)
			if newVec != nil {
				_ = c.vector.Upsert(ctx, merged.ID, newVec, vectorMetadata(merged))
			}

			sourceIDs := []string{m1.ID}
			for _, s := range similar {
				sourceIDs = append(sourceIDs, s.ID)
			}
			report.Merged++
			report.Details = append(report.Details, model.CompressDetail{
				Action: "merge", SourceIDs: sourceIDs, TargetID: m1.ID,
				Reason: fmt.Sprintf("merged %d similar working memories", len(similar)+1),
			})
			processed[m1.ID] = true
		}
	}

	c.logger.Info().
		Int("scanned", report.TotalScanned).
		Int("merged", report.Merged).
		Int("archived", report.Archived).
		Msg("compression completed")

	return report, nil
}

func (c *Compressor) mergeMemories(ctx context.Context, primary *model.Memory, others []*model.Memory) (*model.Memory, error) {
	now := time.Now().UTC()

	var contents []string
	contents = append(contents, primary.Content)
	for _, m := range others {
		contents = append(contents, m.Content)
	}

	seen := make(map[string]bool)
	var unique []string
	for _, content := range contents {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			unique = append(unique, line)
		}
	}
	mergedContent := strings.Join(unique, "\n")
	if len(mergedContent) > c.config.Governance.GetMaxContentLength() {
		mergedContent = mergedContent[:c.config.Governance.GetMaxContentLength()-3] + "..."
	}

	tagSet := make(map[string]bool)
	for _, t := range primary.Tags { tagSet[t] = true }
	for _, m := range others {
		for _, t := range m.Tags { tagSet[t] = true }
	}
	var mergedTags []string
	for t := range tagSet { mergedTags = append(mergedTags, t) }

	var mergedFrom []string
	mergedFrom = append(mergedFrom, primary.MergedFrom...)
	for _, m := range others { mergedFrom = append(mergedFrom, m.ID) }

	for _, m := range others {
		if m.Priority < primary.Priority {
			primary.Priority = m.Priority
		}
	}

	primary.Content = mergedContent
	primary.Tags = mergedTags
	primary.Version++
	primary.UpdatedAt = now
	primary.MergedFrom = mergedFrom

	if err := c.dal.UpdateMemory(ctx, primary); err != nil {
		return nil, fmt.Errorf("update merged: %w", err)
	}
	return primary, nil
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 { return 0 }
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 { return 0 }
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 { return 0 }
	z := x
	for i := 0; i < 10; i++ { z = (z + x/z) / 2 }
	return z
}

func vectorMetadata(m *model.Memory) map[string]string {
	return map[string]string{
		"user_id": m.UserID, "agent_id": m.AgentID,
		"team": m.Team, "category": m.Category, "visibility": m.Visibility,
	}
}
