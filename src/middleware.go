// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import (
	"net/http"
	"time"
)

// responseWriter is a struct that is used as a place where ServeHTTP can write data to.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// LoggingMiddleware is a middleware that logs all incoming requests.
func LoggingMiddleware() func(http.Handler) http.Handler {
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

// MaxBytesMiddleware is a middleware that rejects all requests that are bigger than a certain size.
// Additionally it sets maximum response size as well.
func MaxBytesMiddleware(h *http.ServeMux) http.Handler {
	return http.MaxBytesHandler(h, MaxBytesHandler)
}
