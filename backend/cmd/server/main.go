// Package main implements the agent-memory server.
//
// agent-memory is a self-hosted AI agent memory management framework supporting
// multi-agent isolation, semantic search, auto-classification, dedup, and TTL lifecycle.
//
// Frontend assets are embedded via Go embed from backend/cmd/server/web/
// (copied from frontend/ during build by Makefile).
//
// GitHub: https://github.com/lomehong/agent-memory
package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/lomehong/agent-memory/internal/api"
	"github.com/lomehong/agent-memory/internal/auth"
	"github.com/lomehong/agent-memory/internal/config"
	"github.com/lomehong/agent-memory/internal/core"
	"github.com/lomehong/agent-memory/internal/embedding"
	"github.com/lomehong/agent-memory/internal/model"
	"github.com/lomehong/agent-memory/internal/storage"
)

//go:embed all:web
var webFS embed.FS

func main() {
	startTime := time.Now()
	configPath := "config.yaml"
	for i, arg := range os.Args[1:] {
		if (arg == "-config" || arg == "--config") && i+1 < len(os.Args) {
			configPath = os.Args[i+2]
		}
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	logger.Info().Msg("starting agent-memory server")

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatal().Err(err).Str("path", configPath).Msg("config load failed")
	}
	logger.Info().Str("addr", cfg.Server.Addr()).Msg("config loaded")

	// Init SQLite.
	db, err := storage.Init(cfg.Storage.SQLitePath, &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("database init failed")
	}
	defer db.Close()
	logger.Info().Msg("database initialized")

	// Init vector store.
	var vectorStore storage.VectorStore
	switch cfg.Storage.Vector.Provider {
	case "qdrant":
		vectorStore = storage.NewQdrantStore(cfg.Storage.Vector.Host, cfg.Storage.Vector.Port, "memories", cfg.Embedding.Dimensions, &logger)
		logger.Info().Str("host", cfg.Storage.Vector.Host).Msg("qdrant vector store initialized")
	default:
		vectorStore = storage.NewMemoryVectorStore(cfg.Embedding.Dimensions, &logger)
		logger.Info().Msg("in-memory vector store initialized (dev mode)")
	}

	// Init embedding.
	var embedProvider embedding.EmbeddingProvider
	switch cfg.Embedding.Provider {
	case "onnx":
		embedProvider = embedding.NewONNXEmbedding(cfg.Embedding.ModelPath, cfg.Embedding.Model, cfg.Embedding.Dimensions, &logger)
	case "openai":
		embedProvider = embedding.NewOpenAIEmbedding(cfg.Embedding.OpenAIAPIKey, cfg.Embedding.OpenAIBaseURL, cfg.Embedding.Model, cfg.Embedding.Dimensions, &logger)
	case "mock":
		embedProvider = embedding.NewMockEmbedding(cfg.Embedding.Dimensions)
	default:
		embedProvider = embedding.NewMockEmbedding(cfg.Embedding.Dimensions)
	}
	logger.Info().Str("provider", embedProvider.Name()).Int("dims", embedProvider.Dim()).Msg("embedding initialized")

	// Init core services.
	writer := core.NewWriter(db, embedProvider, vectorStore, cfg, &logger)
	retriever := core.NewRetriever(db, embedProvider, vectorStore, cfg, &logger)
	ttlMgr := core.NewTTLManager(db, cfg, &logger)
	compressor := core.NewCompressor(db, embedProvider, vectorStore, cfg, &logger)

	// Seed agents from config.
	seedAgents(cfg, db, &logger)

	// Setup router.
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(api.CORS)
	r.Use(api.RequestLogger(&logger))
	r.Use(api.Recovery(&logger))

	r.Route("/api/v1", func(r chi.Router) {
		var requestCount uint64
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddUint64(&requestCount, 1)
				next.ServeHTTP(w, r)
			})
		})

		r.Get("/health", handleHealth())
		r.Get("/metrics", handleMetrics(db, startTime, &requestCount))
		r.Get("/system/config", handleSystemConfig(cfg, startTime))

		// Auth routes (no middleware)
		authMgr := auth.NewAuthManager(&cfg.Web, &logger)
		authHandler := api.NewAuthHandler(authMgr, &logger)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/logout", authHandler.Logout)
		r.Get("/auth/me", authHandler.Me)

		r.Group(func(r chi.Router) {
			r.Use(api.APIKeyOrJWTAuth(db, authMgr, &logger))

			r.Post("/memories", handleCreateMemory(writer))
			r.Get("/memories/search", handleSearch(retriever))
			r.Get("/memories", handleListMemories(retriever))
			r.Get("/memories/{id}", handleGetMemory(retriever))
			r.Put("/memories/{id}", handleUpdateMemory(db))
			r.Delete("/memories/{id}", handleDeleteMemory(db))
			r.Post("/memories/batch", handleBatchWrite(writer))
			r.Post("/memories/compress", handleCompress(compressor))
			r.Get("/memories/report", handleReport(retriever))
			r.Post("/memories/export", handleExport(db))

			r.Post("/agents", handleCreateAgent(db))
			r.Get("/agents", handleListAgents(db))
			r.Get("/agents/{id}", handleGetAgent(db))
			r.Delete("/agents/{id}", handleDeleteAgent(db))
		})
	})

	// Serve embedded web dashboard
	webSubFS, err := fs.Sub(webFS, "web")
	if err != nil {
		logger.Fatal().Err(err).Msg("embed web failed")
	}
	r.Handle("/*", http.FileServer(http.FS(webSubFS)))

	// Start TTL scan loop.
	bgCtx, cancelTTL := context.WithCancel(context.Background())
	defer cancelTTL()
	go ttlMgr.StartScanLoop(bgCtx)

	// Start server.
	srv := &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		logger.Info().Str("addr", srv.Addr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info().Msg("server stopped")
}

// --- Agent seeding ---

func seedAgents(cfg *config.Config, db storage.DAL, logger *zerolog.Logger) {
	ctx := context.Background()
	for _, entry := range cfg.Agents {
		if entry.APIKey == "" || strings.Contains(entry.APIKey, "${") {
			continue
		}
		existing, _ := db.GetAgent(ctx, entry.ID)
		if existing != nil {
			// Update existing agent if config changed
			updated := existing.UserID != entry.UserID || existing.Team != entry.Team || existing.Name != entry.Name
			if updated {
				existing.UserID = entry.UserID
				existing.Team = entry.Team
				existing.Name = entry.Name
				if err := db.UpdateAgent(ctx, existing); err != nil {
					logger.Error().Err(err).Str("id", entry.ID).Msg("agent update failed")
				} else {
					logger.Info().Str("id", entry.ID).Str("name", entry.Name).Msg("agent updated")
				}
			}
			continue
		}
		agent := &model.Agent{
			ID: entry.ID, Name: entry.Name, Team: entry.Team,
			UserID: entry.UserID, APIKeyHash: hashKey(entry.APIKey),
			CreatedAt: time.Now().UTC(),
		}
		if err := db.CreateAgent(ctx, agent); err != nil {
			logger.Error().Err(err).Str("id", entry.ID).Msg("seed agent failed")
			continue
		}
		logger.Info().Str("id", entry.ID).Str("name", entry.Name).Msg("agent seeded")
	}
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

// --- Handlers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSystemConfig(cfg *config.Config, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, _ := os.Stat(cfg.Storage.SQLitePath)
		var dbSize int64
		if info != nil {
			dbSize = info.Size()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"version":        "v1.0.0",
			"uptime_seconds": int(time.Since(startTime).Seconds()),
			"storage": map[string]interface{}{
				"sqlite_path":      cfg.Storage.SQLitePath,
				"vector_provider":  cfg.Storage.Vector.Provider,
				"db_size_bytes":    dbSize,
			},
			"embedding": map[string]interface{}{
				"provider":   cfg.Embedding.Provider,
				"model":      cfg.Embedding.Model,
				"dimensions": cfg.Embedding.Dimensions,
			},
			"dedup_thresholds": cfg.Dedup.Thresholds,
			"ttl": map[string]interface{}{
				"scan_interval_hours":  cfg.TTL.ScanIntervalHours,
				"degrade_multiplier":   cfg.TTL.DegradeMultiplier,
				"archive_multiplier":   cfg.TTL.ArchiveMultiplier,
			},
			"scoring": map[string]interface{}{
				"similarity_weight":    cfg.Search.Scoring.SimilarityWeight,
				"priority_weight":      cfg.Search.Scoring.PriorityWeight,
				"access_count_weight":  cfg.Search.Scoring.AccessCountWeight,
				"category_weight":      cfg.Search.Scoring.CategoryWeight,
				"urgency_weight":       cfg.Search.Scoring.UrgencyWeight,
			},
			"governance": map[string]interface{}{
				"compress_threshold":      cfg.Governance.CompressThreshold,
				"max_memories_per_agent":  cfg.Governance.MaxMemoriesPerAgent,
				"max_content_length":      cfg.Governance.MaxContentLength,
			},
		})
	}
}

func handleMetrics(db storage.DAL, startTime time.Time, requestCount *uint64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		uptime := time.Since(startTime).Seconds()
		fmt.Fprintf(w, "# HELP agent_memory_uptime_seconds Server uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE agent_memory_uptime_seconds gauge\n")
		fmt.Fprintf(w, "agent_memory_uptime_seconds %.0f\n\n", uptime)
		fmt.Fprintf(w, "# HELP agent_memory_api_requests_total Total API requests served\n")
		fmt.Fprintf(w, "# TYPE agent_memory_api_requests_total counter\n")
		fmt.Fprintf(w, "agent_memory_api_requests_total %d\n\n", atomic.LoadUint64(requestCount))
		for _, status := range []string{"active", "degraded", "archived"} {
			mem, err := db.GetMemoriesByStatus(ctx, status, 1)
			if err == nil {
				fmt.Fprintf(w, "# HELP agent_memory_memories_%s Number of %s memories\n", status, status)
				fmt.Fprintf(w, "# TYPE agent_memory_memories_%s gauge\n", status)
				fmt.Fprintf(w, "agent_memory_memories_%s %d\n", status, len(mem))
		}
		}
	}
}

func handleCreateMemory(writer *core.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req model.MemoryCreateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		result, err := writer.Write(r.Context(), info.UserID, info.ID, info.Team, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func handleSearch(retriever *core.Retriever) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		query := r.URL.Query().Get("query")
		if query == "" {
			writeError(w, http.StatusBadRequest, "query required")
			return
		}
		opts := model.SearchOpts{Category: r.URL.Query().Get("category")}
		if k, _ := strconv.Atoi(r.URL.Query().Get("top_k")); k > 0 { opts.TopK = k }

		results, err := retriever.Search(r.Context(), info.UserID, info.ID, info.Team, query, opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"results": results, "count": len(results)})
	}
}

func handleListMemories(retriever *core.Retriever) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		filter := model.MemoryFilter{
			Category: r.URL.Query().Get("category"),
			Status:   r.URL.Query().Get("status"),
		}
		if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 { filter.Limit = l }
		if o, _ := strconv.Atoi(r.URL.Query().Get("offset")); o > 0 { filter.Offset = o }

		// Get total count (without limit/offset)
		countFilter := model.MemoryFilter{Category: filter.Category, Status: filter.Status}
		allMemories, _ := retriever.ListMemories(r.Context(), info.UserID, info.ID, info.Team, countFilter)
		total := len(allMemories)

		memories, err := retriever.ListMemories(r.Context(), info.UserID, info.ID, info.Team, filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"memories": memories, "count": len(memories), "total": total})
	}
}

func handleGetMemory(retriever *core.Retriever) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := chi.URLParam(r, "id")
		mem, err := retriever.GetMemory(r.Context(), info.UserID, info.ID, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, mem)
	}
}

func handleUpdateMemory(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := chi.URLParam(r, "id")
		mem, err := db.GetMemory(r.Context(), id)
		if err != nil || mem == nil {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		if mem.UserID != info.UserID || (mem.Visibility == model.VisibilityPrivate && mem.AgentID != info.ID) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		var req model.MemoryUpdateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Content != nil { mem.Content = *req.Content }
		if req.Category != nil { mem.Category = *req.Category }
		if req.Priority != nil { mem.Priority = *req.Priority }
		if req.Visibility != nil { mem.Visibility = *req.Visibility }
		if req.TTL != nil { mem.TTL = *req.TTL }
		if req.Tags != nil { mem.Tags = req.Tags }
		if req.Status != nil { mem.Status = *req.Status }
		mem.Version++
		mem.UpdatedAt = time.Now().UTC()
		if err := db.UpdateMemory(r.Context(), mem); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, mem)
	}
}

func handleDeleteMemory(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := chi.URLParam(r, "id")
		mem, err := db.GetMemory(r.Context(), id)
		if err != nil || mem == nil {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		if mem.UserID != info.UserID {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		if err := db.DeleteMemory(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}
}

func handleBatchWrite(writer *core.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var reqs []model.MemoryCreateReq
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		memories, errs := writer.BatchWrite(r.Context(), info.UserID, info.ID, info.Team, reqs)
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"memories": memories, "count": len(memories), "errors": len(errs),
		})
	}
}

func handleCompress(compressor *core.Compressor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		report, err := compressor.Compress(r.Context(), info.UserID, info.ID, info.Team)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func handleReport(retriever *core.Retriever) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		report, err := retriever.GetHealthReport(r.Context(), info.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func handleExport(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		memories, err := db.ListMemories(r.Context(), model.MemoryFilter{UserID: info.UserID, Limit: 10000})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result := make([]model.Memory, len(memories))
		for i, m := range memories {
			result[i] = *m
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=\"memories-export-"+info.UserID+".json\"")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"user_id":    info.UserID,
			"count":      len(result),
			"memories":   result,
		})
	}
}

func handleCreateAgent(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req model.AgentRegisterReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			writeError(w, http.StatusBadRequest, "X-API-Key required")
			return
		}
		team := req.Team
		if team == "" { team = "default" }
		agent := &model.Agent{
			ID: strings.ToLower(req.Name) + "-" + fmt.Sprintf("%d", time.Now().UnixNano()),
			Name: req.Name, Team: team, UserID: info.UserID,
			APIKeyHash: hashKey(apiKey), CreatedAt: time.Now().UTC(),
		}
		if err := db.CreateAgent(r.Context(), agent); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, agent)
	}
}

func handleListAgents(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := api.GetAgentInfo(r)
		if info == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		agents, err := db.ListAgents(r.Context(), info.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"agents": agents, "count": len(agents)})
	}
}

func handleGetAgent(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		agent, err := db.GetAgent(r.Context(), id)
		if err != nil || agent == nil {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func handleDeleteAgent(db storage.DAL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.DeleteAgent(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}
}

var _ = runtime.GC // ensure runtime is used
