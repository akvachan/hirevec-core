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

