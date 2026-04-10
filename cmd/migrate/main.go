package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/core"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
)

type Mem0Export struct {
	ID        string `json:"id"`
	Memory    string `json:"memory"`
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()

	configPath := "config.yaml"
	exportFile := "mem0_export.json"
	for i, arg := range os.Args[1:] {
		if (arg == "-config" || arg == "--config") && i+1 < len(os.Args) {
			configPath = os.Args[i+2]
		}
		if (arg == "-input" || arg == "--input") && i+1 < len(os.Args) {
			exportFile = os.Args[i+2]
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("config load failed")
	}

	db, err := storage.Init(cfg.Storage.SQLitePath, &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("db init failed")
	}
	defer db.Close()

	vectorStore := storage.NewMemoryVectorStore(cfg.Embedding.Dimensions, &logger)
	embedProvider := embedding.NewMockEmbedding(cfg.Embedding.Dimensions)
	writer := core.NewWriter(db, embedProvider, vectorStore, cfg, &logger)

	data, err := os.ReadFile(exportFile)
	if err != nil {
		logger.Fatal().Err(err).Str("file", exportFile).Msg("read failed")
	}

	var exports []Mem0Export
	if err := json.Unmarshal(data, &exports); err != nil {
		logger.Fatal().Err(err).Msg("parse failed")
	}

	logger.Info().Int("total", len(exports)).Msg("migration starting")

	ctx := context.Background()
	imported, merged, errors := 0, 0, 0

	for _, exp := range exports {
		if exp.Memory == "" {
			continue
		}

		category := core.InferCategory(exp.Memory)
		visibility := core.InferVisibility(exp.Memory, category)

		req := model.MemoryCreateReq{
			Content:    exp.Memory,
			Visibility: visibility,
			Source:     fmt.Sprintf("mem0_export:%s", exp.ID),
			Tags:       []string{"migrated"},
		}

		userID := exp.UserID
		if userID == "" {
			userID = "default"
		}

		result, err := writer.Write(ctx, userID, "migrate", "", req)
		if err != nil {
			if result != nil && result.Suggestion != nil && result.Suggestion.DedupHit {
				merged++
			} else {
				errors++
			}
			continue
		}
		imported++
		if imported%50 == 0 {
			logger.Info().Int("imported", imported).Int("merged", merged).Msg("progress")
		}
	}

	report := map[string]interface{}{
		"total": imported + merged + errors, "imported": imported,
		"merged": merged, "errors": errors,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(reportJSON))
	logger.Info().Int("imported", imported).Int("merged", merged).Int("errors", errors).Msg("migration completed")
}
