package api

import (
	"encoding/json"
	"net/http"

	"github.com/lomehong/agent-memory/internal/auth"
	"github.com/rs/zerolog"
)

type AuthHandler struct {
	authMgr *auth.AuthManager
	logger  *zerolog.Logger
}

func NewAuthHandler(authMgr *auth.AuthManager, logger *zerolog.Logger) *AuthHandler {
	return &AuthHandler{authMgr: authMgr, logger: logger}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	clientIP := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		clientIP = xff
	}

	token, expiresAt, err := h.authMgr.Login(req.Username, req.Password, clientIP)
	if err != nil {
		h.logger.Warn().Err(err).Str("username", req.Username).Str("ip", clientIP).Msg("login failed")
		http.Error(w, `{"error":"用户名或密码错误"}`, http.StatusUnauthorized)
		return
	}

	WriteJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// JWT is stateless; logout is a no-op client-side action
	WriteJSON(w, http.StatusOK, map[string]string{"message": "已登出"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
		return
	}
	tokenStr := parseBearerToken(authHeader)
	if tokenStr == "" {
		http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
		return
	}

	claims, err := h.authMgr.ValidateToken(tokenStr)
	if err != nil {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"username": claims.Username,
		"role":     claims.Role,
	})
}
