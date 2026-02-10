// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/akvachan/hirevec-backend/internal/vault"
)

type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
	contextKeyClaims contextKey = "claims"
)

type RateLimiter struct {
	MaxRequests uint
	Window      time.Duration
	visitors    map[string]*visitor
	mu          sync.RWMutex
}

type visitor struct {
	tokens     uint
	lastRefill time.Time
}

func NewRateLimiter(maxRequests uint, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		MaxRequests: maxRequests,
		Window:      window,
		visitors:    make(map[string]*visitor),
	}
	go rl.cleanupVisitors()
	return rl
}

func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastRefill) > rl.Window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]

	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:     rl.MaxRequests - 1,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(v.lastRefill)
	if elapsed >= rl.Window {
		v.tokens = rl.MaxRequests
		v.lastRefill = now
	}

	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
}

func RateLimit(rl *RateLimiter) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				ip = strings.Split(forwardedFor, ",")[0]
			}

			if !rl.allow(ip) {
				WriteErrorResponse(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

func PublicMiddleware(rl *RateLimiter) []Middleware {
	return []Middleware{
		ErrorHandling,
		Logging,
		RateLimit(rl),
		MaxBytes,
	}
}

func ProtectedMiddleware(v vault.Vault, rl *RateLimiter) []Middleware {
	return []Middleware{
		ErrorHandling,
		Logging,
		RateLimit(rl),
		MaxBytes,
		Authentication(v),
		Authorization,
	}
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

func Chain(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func ErrorHandling(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"panic recovered",
					"err", err,
				)
				WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	}
}

func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	return userID, ok
}

func GetClaims(ctx context.Context) (*vault.AccessTokenClaims, bool) {
	claims, ok := ctx.Value(contextKeyClaims).(*vault.AccessTokenClaims)
	return claims, ok
}

func Authentication(v vault.Vault) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			bearer, found := strings.CutPrefix(authHeader, "Bearer ")
			if !found || bearer == "" {
				WriteUnauthorizedResponse(w, AuthInvalidClient, "Bearer token is required")
				return
			}
			claims, err := v.ParseAccessToken(bearer)
			if err != nil {
				WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid access token")
				return
			}
			ctx := context.WithValue(r.Context(), contextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, contextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

func Authorization(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		if !ok {
			WriteUnauthorizedResponse(w, AuthInvalidRequest, "missing claims in context")
			return
		}

		next.ServeHTTP(w, r)
	}
}

func Logging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	}
}

func MaxBytes(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1_000_000)
		next.ServeHTTP(w, r)
	}
}
