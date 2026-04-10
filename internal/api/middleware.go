package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/lomehong/agent-memory/internal/auth"
	"github.com/lomehong/agent-memory/internal/storage"
	"github.com/rs/zerolog"
)

type contextKey string

const (
	agentContextKey  contextKey = "agent"
	userIDContextKey contextKey = "user_id"
)

// AgentInfo holds agent information extracted from the request context.
type AgentInfo struct {
	ID     string
	Name   string
	UserID string
	Team   string
}

// APIKeyOrJWTAuth is middleware that supports both X-API-Key and Bearer JWT authentication.
func APIKeyOrJWTAuth(dal storage.DAL, authMgr *auth.AuthManager, logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			// Try JWT first if Authorization header exists
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr := parseBearerToken(authHeader)
				claims, err := authMgr.ValidateToken(tokenStr)
				if err != nil {
					http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
					return
				}
				info := AgentInfo{
					ID:     claims.Username,
					Name:   claims.Username,
					UserID: claims.Username,
					Team:   "admin",
				}
				ctx := context.WithValue(r.Context(), agentContextKey, info)
				ctx = context.WithValue(ctx, userIDContextKey, claims.Username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fall back to API Key
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, `{"error":"missing X-API-Key header or Bearer token"}`, http.StatusUnauthorized)
				return
			}

			keyHash := hashAPIKey(apiKey)

			agent, err := dal.GetAgentByAPIKeyHash(r.Context(), keyHash)
			if err != nil {
				logger.Error().Err(err).Msg("failed to look up agent by API key")
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}
			if agent == nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			info := AgentInfo{
				ID:     agent.ID,
				Name:   agent.Name,
				UserID: agent.UserID,
				Team:   agent.Team,
			}
			ctx := context.WithValue(r.Context(), agentContextKey, info)
			ctx = context.WithValue(ctx, userIDContextKey, agent.UserID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth is kept as an alias for backward compatibility.
var APIKeyAuth = APIKeyOrJWTAuth

// GetAgentInfo extracts agent info from the request context.
func GetAgentInfo(r *http.Request) *AgentInfo {
	info, ok := r.Context().Value(agentContextKey).(AgentInfo)
	if !ok {
		return nil
	}
	return &info
}

// GetUserID extracts user ID from the request context.
func GetUserID(r *http.Request) string {
	uid, ok := r.Context().Value(userIDContextKey).(string)
	if !ok {
		return ""
	}
	return uid
}

// hashAPIKey creates a SHA-256 hash of the API key.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

// CORS middleware adds CORS headers.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs each HTTP request.
func RequestLogger(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.statusCode).
				Dur("duration", duration).
				Str("remote", r.RemoteAddr).
				Msg("request")
		})
	}
}

// Recovery middleware recovers from panics with full stack trace.
func Recovery(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					logger.Error().
						Str("panic", fmt.Sprintf("%v", err)).
						Str("path", r.URL.Path).
						Str("method", r.Method).
						Str("stack", string(buf)).
						Msg("panic recovered")
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

// parseBearerToken extracts token from Authorization header.
func parseBearerToken(authHeader string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return authHeader
}
