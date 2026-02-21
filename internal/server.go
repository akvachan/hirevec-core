// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	DefaultReadTimeout  = 2000 * time.Millisecond
	DefaultWriteTimeout = 2000 * time.Millisecond
	DefaultGracePeriod  = 5000 * time.Millisecond
)

type ServerConfig struct {
	Host         string
	Port         uint16
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	GracePeriod  time.Duration
}

func RunServer(ctx context.Context, c ServerConfig, s Store, v Vault) error {
	server, err := NewServer(ctx, c, s, v)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%v", c.Host, c.Port))
	if err != nil {
		return ErrFailedToBindAddress(c.Host, err)
	}

	return WaitAndShutdown(ctx, server, StartServer(server, listener), c.GracePeriod)
}

func NewServer(ctx context.Context, c ServerConfig, s Store, v Vault) (*http.Server, error) {
	return &http.Server{
		Addr:         c.Host,
		ReadTimeout:  c.ReadTimeout,
		WriteTimeout: c.WriteTimeout,
		Handler:      AssembleTree(s, v),
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}, nil
}

func StartServer(server *http.Server, ln net.Listener) chan error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info(
			"HTTP server starting",
			"addr", server.Addr,
		)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info(
		"HTTP server ready",
		"addr", server.Addr,
	)
	return errCh
}

func WaitAndShutdown(ctx context.Context, server *http.Server, errCh chan error, gracePeriod time.Duration) error {
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return ErrFailedToShutdownServer(err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), gracePeriod)
	defer cancel()

	slog.Info(
		"starting graceful shutdown",
		"timeout", gracePeriod,
	)
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed, forcing close", "err", err)
		server.Close()
		return ErrFailedToShutdownServer(err)
	}

	slog.Info("HTTP server shutdown complete")
	return nil
}
