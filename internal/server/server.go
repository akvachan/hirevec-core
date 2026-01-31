// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// HirevecServer holds the active instance of the HTTP server.
var HirevecServer *http.Server

// HirevecLogger is the global structured logger for the server package.
var HirevecLogger *slog.Logger

// ShutdownConfig defines the timing parameters for a controlled server exit.
type ShutdownConfig struct {
	// ReadinessDelay is the time to wait after a shutdown signal.
	ReadinessDelay time.Duration

	// GracePeriod is the maximum time allowed for existing active requests to complete.
	GracePeriod time.Duration

	// ForcePeriod is a final timeout used if the graceful shutdown fails.
	ForcePeriod time.Duration
}

// RunHTTPServer starts the HTTP server in a background goroutine and manages its lifecycle.
func RunHTTPServer(
	ctx context.Context,
	server *http.Server,
	cfg ShutdownConfig,
) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		slog.Info("HTTP server starting", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-sigCtx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	slog.Info("draining readiness")
	select {
	case <-time.After(cfg.ReadinessDelay):
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(sigCtx, cfg.GracePeriod)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown failed, forcing exit", "err", err)

		select {
		case <-time.After(cfg.ForcePeriod):
		case <-sigCtx.Done():
		}

		return err
	}

	slog.Info("HTTP server shut down gracefully")
	return nil
}

// GetRootRouter assembles the complete application routing tree.
func GetRootRouter() *http.ServeMux {
	rootMux := http.NewServeMux()
	apiRouter := NewRouter(rootMux, "api")
	apiRouter.AddRoutes(
		// Route{
		// 	Methods:    []string{http.MethodGet},
		// 	Path:       "health",
		// 	APIVersion: V1,
		// 	Handler:    CheckHealth,
		// 	 append(
		// 		GroupPublic,
		// 		RateLimit(60, time.Minute),
		// 	),
		// 	Description: "Health check endpoint",
		// },

		// OAuth endpoints
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "auth/keys",
			APIVersion: V1,
			Handler:    GetPublicKeys,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Get server public keys",
		},
		Route{
			Methods:    []string{http.MethodPost},
			Path:       "oauth2/token",
			APIVersion: V1,
			Handler:    TokenEndpoint,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Get new access token",
		},
		Route{
			Methods:    []string{http.MethodGet, http.MethodPost},
			Path:       "oauth2/login/{provider}",
			APIVersion: V1,
			Handler:    Login,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Hour),
			),
			Description: "Authorize via provider",
		},
		Route{
			Methods:    []string{http.MethodGet, http.MethodPost},
			Path:       "oauth2/callback/{provider}",
			APIVersion: V1,
			Handler:    RedirectionEndpoint,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Internal OAuth callback",
		},

		// Protected resources
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "positions",
			APIVersion: V1,
			Handler:    GetPositions,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Hour),
			),
			Description: "List all positions",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "positions/{id}",
			APIVersion: V1,
			Handler:    GetPosition,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Minute),
			),
			Description: "Get position by ID",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "candidates/{id}",
			APIVersion: V1,
			Handler:    GetCandidate,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Minute),
			),
			Description: "Get candidate by ID",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "candidates",
			APIVersion: V1,
			Handler:    GetCandidates,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Hour),
			),
			Description: "List all candidates",
		},
	)

	return rootMux
}
