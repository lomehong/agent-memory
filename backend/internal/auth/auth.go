package auth

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lomehong/agent-memory/internal/config"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	jwt.RegisteredClaims
	Username string `json:"sub"`
	Role     string `json:"role"`
}

type AdminUser struct {
	Username     string
	PasswordHash string
}

type rateLimit struct {
	count    int
	expireAt time.Time
}

type AuthManager struct {
	jwtSecret   []byte
	tokenTTL    time.Duration
	admins      map[string]*AdminUser
	rateLimiter map[string]*rateLimit
	maxRate     int
	mu          sync.RWMutex
	logger      *zerolog.Logger
}

func NewAuthManager(cfg *config.WebConfig, logger *zerolog.Logger) *AuthManager {
	secret := []byte(cfg.JWTSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		rand.Read(secret)
		logger.Warn().Msg("JWT secret not configured, using random key (tokens will not survive restart)")
	}

	ttl := time.Duration(cfg.TokenTTLHours) * time.Hour
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	maxRate := cfg.LoginRateLimit
	if maxRate <= 0 {
		maxRate = 5
	}

	admins := make(map[string]*AdminUser)
	for _, entry := range cfg.Admins {
		hash := entry.PasswordHash
		if hash != "" && hash[0] != '$' {
			h, err := bcrypt.GenerateFromPassword([]byte(hash), bcrypt.DefaultCost)
			if err != nil {
				logger.Error().Err(err).Str("username", entry.Username).Msg("bcrypt failed for admin")
				continue
			}
			hash = string(h)
			logger.Info().Str("username", entry.Username).Msg("admin password auto-bcrypted")
		}
		admins[entry.Username] = &AdminUser{Username: entry.Username, PasswordHash: hash}
	}

	if len(admins) == 0 {
		h, _ := bcrypt.GenerateFromPassword([]byte("<admin-password>"), bcrypt.DefaultCost)
		admins["admin"] = &AdminUser{Username: "admin", PasswordHash: string(h)}
		logger.Warn().Msg("no admins configured, using default admin credentials (see docs)")
	}

	return &AuthManager{
		jwtSecret:   secret,
		tokenTTL:    ttl,
		admins:      admins,
		rateLimiter: make(map[string]*rateLimit),
		maxRate:     maxRate,
		logger:      logger,
	}
}

func (am *AuthManager) Login(username, password, clientIP string) (token string, expiresAt time.Time, err error) {
	am.mu.Lock()
	// Rate limit check
	now := time.Now()
	rl, ok := am.rateLimiter[clientIP]
	if ok && now.Before(rl.expireAt) {
		if rl.count >= am.maxRate {
			am.mu.Unlock()
			return "", time.Time{}, fmt.Errorf("rate limit exceeded")
		}
		rl.count++
	} else {
		am.rateLimiter[clientIP] = &rateLimit{count: 1, expireAt: now.Add(time.Minute)}
	}
	am.mu.Unlock()

	am.mu.RLock()
	admin, ok := am.admins[username]
	am.mu.RUnlock()

	if !ok {
		return "", time.Time{}, fmt.Errorf("用户名或密码错误")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		return "", time.Time{}, fmt.Errorf("用户名或密码错误")
	}

	expiresAt = time.Now().Add(am.tokenTTL)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Username: username,
		Role:     "admin",
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := t.SignedString(am.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token generation failed")
	}
	return tokenStr, expiresAt, nil
}

func (am *AuthManager) ValidateToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return am.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func (am *AuthManager) GetAdmins() []*AdminUser {
	am.mu.RLock()
	defer am.mu.RUnlock()
	result := make([]*AdminUser, 0, len(am.admins))
	for _, a := range am.admins {
		result = append(result, a)
	}
	return result
}
