// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

// Package server implements basic routing, middleware, handlers and validation
package server

import (
	"log/slog"
	"net/http"
	"time"
)

const (
	bit      int64 = 1
	kilobyte int64 = bit * 1024
	megabyte int64 = kilobyte * 1024
)

const maxBytesHandler = 1 * megabyte

var HirevecLogger *slog.Logger

// responseWriter is a struct that is used as a place where ServeHTTP can write data to.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// getLoggingMiddleware is a middleware that logs all incoming requests.
func getLoggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseWriter{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			HirevecLogger.Info(
				"request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration", time.Since(start),
			)
		})
	}
}

// getMaxBytesMiddleware is a middleware that rejects all requests that are bigger than a certain size.
// Additionally it sets maximum response size as well.
func getMaxBytesMiddleware(h *http.ServeMux) http.Handler {
	return http.MaxBytesHandler(h, maxBytesHandler)
}
