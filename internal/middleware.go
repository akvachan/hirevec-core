// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ContextKey string

const (
	ContextKeyUserID ContextKey = "user_id"
	ContextKeyClaims ContextKey = "claims"
)

type RateLimiter struct {
	MaxRequests uint
	Window      time.Duration
	Visitors    map[string]*Visitor
	Mu          sync.RWMutex
}

type Visitor struct {
	Tokens     uint
	LastRefill time.Time
}

func NewRateLimiter(maxRequests uint, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		MaxRequests: maxRequests,
		Window:      window,
		Visitors:    make(map[string]*Visitor),
	}
	go rl.cleanupVisitors()
	return rl
}

func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.Mu.Lock()
		for ip, v := range rl.Visitors {
			if time.Since(v.LastRefill) > rl.Window*2 {
				delete(rl.Visitors, ip)
			}
		}
		rl.Mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.Mu.Lock()
	defer rl.Mu.Unlock()

	now := time.Now()
	v, exists := rl.Visitors[ip]

	if !exists {
		rl.Visitors[ip] = &Visitor{
			Tokens:     rl.MaxRequests - 1,
			LastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(v.LastRefill)
	if elapsed >= rl.Window {
		v.Tokens = rl.MaxRequests
		v.LastRefill = now
	}

	if v.Tokens > 0 {
		v.Tokens--
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

func PublicMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return Chain(
		handler,
		ErrorHandling,
		Logging,
		RateLimit(NewRateLimiter(100, time.Minute)),
		MaxBytes,
		// Authentication(v),
		// Authorization,
	)
}

func ProtectedMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return Chain(
		handler,
		ErrorHandling,
		Logging,
		RateLimit(NewRateLimiter(100, time.Minute)),
		MaxBytes,
		// Authentication(v),
		// Authorization,
	)
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

func Chain(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

type ResponseWriter struct {
	http.ResponseWriter
	Status int
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.Status = code
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
	userID, ok := ctx.Value(ContextKeyUserID).(string)
	return userID, ok
}

func GetClaims(ctx context.Context) (*AccessTokenClaims, bool) {
	claims, ok := ctx.Value(ContextKeyClaims).(*AccessTokenClaims)
	return claims, ok
}

func Authentication(v Vault) Middleware {
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
			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// func Authorization(next http.HandlerFunc) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		claims, ok := GetClaims(r.Context())
// 		if !ok {
// 			WriteUnauthorizedResponse(w, AuthInvalidRequest, "missing claims in context")
// 			return
// 		}
//
// 		next.ServeHTTP(w, r)
// 	}
// }

func Logging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &ResponseWriter{ResponseWriter: w, Status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.Status,
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
