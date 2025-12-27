// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import (
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

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

func MaxBytesMiddleware(h *http.ServeMux) http.Handler {
	return http.MaxBytesHandler(h, MaxBytesHandler)
}
