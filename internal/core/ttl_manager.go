package core

import (
	"context"
	"fmt"
	"time"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/rs/zerolog"
)

// TTLManager manages memory lifecycle through TTL-based expiration.
// Corresponds to DESIGN-010.
type TTLManager struct {
	dal    storage.DAL
	config *config.Config
	logger *zerolog.Logger
}

func NewTTLManager(dal storage.DAL, cfg *config.Config, logger *zerolog.Logger) *TTLManager {
	return &TTLManager{dal: dal, config: cfg, logger: logger}
}

// Scan scans for expired memories and updates their status per DESIGN-010:
//   - active → degraded (after TTL * DegradeMultiplier without access)
//   - degraded → archived (after TTL * ArchiveMultiplier since degradation)
func (tm *TTLManager) Scan(ctx context.Context) error {
	start := time.Now()
	now := time.Now().UTC()

	tm.logger.Info().Msg("TTL scan started")

	// Get all active and degraded memories.
	activeMemories, err := tm.dal.ListMemories(ctx, model.MemoryFilter{Status: model.StatusActive, Limit: 10000})
	if err != nil {
		return fmt.Errorf("list active: %w", err)
	}

	degradedCount := 0
	for _, m := range activeMemories {
		if m.TTL == model.TTLPermanent || m.TTL == "" {
			continue
		}

		ttlDuration := config.TTLDuration(m.TTL)
		if ttlDuration == 0 {
			continue
		}

		idleSinceAccess := now.Sub(m.LastAccessed)
		degradeThreshold := ttlDuration * time.Duration(tm.config.TTL.GetDegradeMultiplier())

		if idleSinceAccess >= degradeThreshold {
			if err := tm.dal.UpdateMemoryStatus(ctx, m.ID, model.StatusDegraded); err != nil {
				tm.logger.Error().Err(err).Str("id", m.ID).Msg("degrade failed")
				continue
			}
			degradedCount++
			tm.logAction(ctx, m.ID, "degraded", fmt.Sprintf("idle %v >= degrade threshold %v (ttl=%s)", idleSinceAccess, degradeThreshold, m.TTL))
		}
	}

	// Now check degraded → archived.
	archivedCount := 0
	degradedMemories, err := tm.dal.GetMemoriesByStatus(ctx, model.StatusDegraded, 10000)
	if err != nil {
		tm.logger.Warn().Err(err).Msg("list degraded failed")
	} else {
		for _, m := range degradedMemories {
			ttlDuration := config.TTLDuration(m.TTL)
			if ttlDuration == 0 {
				continue
			}
			idleSinceUpdate := now.Sub(m.UpdatedAt)
			archiveThreshold := ttlDuration * time.Duration(tm.config.TTL.GetArchiveMultiplier())

			if idleSinceUpdate >= archiveThreshold {
				if err := tm.dal.UpdateMemoryStatus(ctx, m.ID, model.StatusArchived); err != nil {
					tm.logger.Error().Err(err).Str("id", m.ID).Msg("archive failed")
					continue
				}
				archivedCount++
				tm.logAction(ctx, m.ID, "archived", fmt.Sprintf("idle since degrade %v >= archive threshold %v", idleSinceUpdate, archiveThreshold))
			}
		}
	}

	duration := time.Since(start)
	tm.logger.Info().
		Int("scanned", len(activeMemories)).
		Int("degraded", degradedCount).
		Int("archived", archivedCount).
		Dur("duration", duration).
		Msg("TTL scan completed")

	return nil
}

// StartScanLoop starts a background goroutine that periodically scans for expired memories.
func (tm *TTLManager) StartScanLoop(ctx context.Context) {
	interval := tm.config.TTL.GetScanInterval()
	tm.logger.Info().Dur("interval", interval).Msg("TTL scan loop started")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			tm.logger.Info().Msg("TTL scan loop stopped")
			return
		case <-ticker.C:
			if err := tm.Scan(ctx); err != nil {
				tm.logger.Error().Err(err).Msg("TTL scan failed")
			}
		}
	}
}

func (tm *TTLManager) logAction(ctx context.Context, memoryID, action, details string) {
	log := &model.MemoryLog{
		ID: fmt.Sprintf("ttl-%s-%d", memoryID[:8], time.Now().UnixNano()),
		MemoryID: memoryID, Action: "ttl_" + action, Details: details, CreatedAt: time.Now().UTC(),
	}
	_ = tm.dal.CreateLog(ctx, log)
}
