// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type (
	ContextKey string

	Visitor struct {
		Tokens     uint
		LastRefill time.Time
	}

	RateLimiterConfig struct {
		MaxRequests uint
		Window      time.Duration
		Visitors    map[string]*Visitor
		Mu          sync.RWMutex
	}

	PaginatorConfig struct {
		DefaultLimit uint64
		MaxLimit     uint64
	}

	Pagination struct {
		Limit  uint64  `json:"limit"`
		After  *string `json:"after,omitempty"`
		Before *string `json:"before,omitempty"`
	}

	Middleware func(http.HandlerFunc) http.HandlerFunc

	ResponseWriter struct {
		http.ResponseWriter
		status int
	}
)

const (
	DefaultPageSizeLimit            = 50
	PageSizeMaxLimit                = 100
	ContextKeyPagination ContextKey = "pagination"
	ContextKeyUserID     ContextKey = "user_id"
	ContextKeyClaims     ContextKey = "claims"
)

func NewRateLimiterConfig(maxRequests uint, window time.Duration) *RateLimiterConfig {
	rl := &RateLimiterConfig{
		MaxRequests: maxRequests,
		Window:      window,
		Visitors:    make(map[string]*Visitor),
	}
	go rl.cleanupVisitors()
	return rl
}

func (rl *RateLimiterConfig) cleanupVisitors() {
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

func (rl *RateLimiterConfig) allow(ip string) bool {
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

func RateLimiter(rlc *RateLimiterConfig) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				ip = strings.Split(forwardedFor, ",")[0]
			}

			if !rlc.allow(ip) {
				Error(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

func NewPaginatorConfig(defaultLimit uint64, maxLimit uint64) PaginatorConfig {
	return PaginatorConfig{
		defaultLimit,
		maxLimit,
	}
}

func GetPagination(r *http.Request) Pagination {
	p, ok := r.Context().Value(ContextKeyPagination).(Pagination)
	if !ok {
		p = Pagination{
			Limit: DefaultPageSizeLimit,
		}
	}

	q := r.URL.Query()

	p.Before = nil
	p.After = nil

	if before := q.Get("before"); before != "" {
		p.Before = &before
	}
	if after := q.Get("after"); after != "" {
		p.After = &after
	}

	return p
}

func Paginator(pc PaginatorConfig) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			limitStr := r.URL.Query().Get("limit")
			limit, err := strconv.ParseUint(limitStr, 10, 64)
			if err != nil || limit == 0 {
				limit = pc.DefaultLimit
			}
			if limit > pc.MaxLimit {
				limit = pc.MaxLimit
			}

			after := r.URL.Query().Get("after")
			before := r.URL.Query().Get("before")
			if after != "" && before != "" {
				Fail(w, http.StatusBadRequest, FailData{"pagination": "cannot use both before and after"}, nil)
				return
			}

			p := Pagination{
				Limit:  limit,
				After:  &after,
				Before: &before,
			}
			ctx := context.WithValue(r.Context(), ContextKeyPagination, p)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// Chain wraps handler into a sequence of middlewares, each middleware is applied in the same order it is provided.
func Chain(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func ErrorHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"panic recovered",
					"err", err,
				)
				Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	}
}

func GetUserID(r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(ContextKeyUserID).(string)
	return userID, ok
}

func GetClaims(r *http.Request) (*AccessTokenClaims, bool) {
	claims, ok := r.Context().Value(ContextKeyClaims).(*AccessTokenClaims)
	return claims, ok
}

func Authentication(v Vault, allowedForScopeValues []ScopeValueType) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var bearer string

			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				var found bool
				bearer, found = strings.CutPrefix(authHeader, "Bearer ")
				if !found || bearer == "" {
					Unauthorized(w, AuthInvalidClient, "Bearer token is required", nil)
					return
				}
			}

			claims, err := v.ParseAccessToken(bearer)
			if err != nil || claims == nil {
				AuthError(w, AuthInvalidGrant, "invalid access token")
				return
			}

			for _, scopeValue := range claims.Scope {
				if !slices.Contains(allowedForScopeValues, scopeValue) {
					AuthError(w, AuthInvalidGrant, "permission denied")
					return
				}
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

func Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
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

func MaxBytesLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1_000_000)
		next.ServeHTTP(w, r)
	}
}
