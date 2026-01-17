package server

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

type ShutdownConfig struct {
	ReadinessDelay time.Duration
	GracePeriod    time.Duration
	ForcePeriod    time.Duration
}

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
		slog.Info("Shutdown signal received")
	case err := <-errCh:
		return err
	}

	slog.Info("Draining readiness")
	select {
	case <-time.After(cfg.ReadinessDelay):
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(sigCtx, cfg.GracePeriod)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("Graceful shutdown failed, forcing exit", "err", err)

		select {
		case <-time.After(cfg.ForcePeriod):
		case <-sigCtx.Done():
		}

		return err
	}

	slog.Info("HTTP server shut down gracefully")
	return nil
}
