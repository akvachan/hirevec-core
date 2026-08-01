// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	DefaultLogLevel = slog.LevelWarn
)

func InitLogger(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

var ErrUnknownLogLevel = errors.New("unknown log level")

func ParseLogLevel(value string) (slog.Level, error) {
	switch value {
	case "INFO":
		return slog.LevelInfo, nil
	case "ERROR":
		return slog.LevelError, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "DEBUG":
		return slog.LevelDebug, nil
	default:
		return -1, ErrUnknownLogLevel
	}
}

func ParseInt(value string) (int, error) {
	parsedValue, err := strconv.ParseInt(value, 10, 64)
	return int(parsedValue), fmt.Errorf("failed to parse int: %w", err)
}

type AppConfig struct {
	ServerBaseURL               *url.URL
	LogLevel                    slog.Level
	RequestReadTimeout          time.Duration
	RequestWriteTimeout         time.Duration
	GracePeriod                 time.Duration
	EmbeddingsJobFrequency      time.Duration
	RecommendationsJobFrequency time.Duration
	SymmetricKey                string
	AsymmetricKey               string
	AppleClientID               string
	AppleClientSecret           string
	GoogleClientID              string
	GoogleClientSecret          string
	PostgreSQLDatabaseURL       *url.URL
	TEIAPIKey                   string
	TEIBaseURL                  *url.URL
}

func RunApp(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	InitLogger(c.LogLevel)

	useGoogleSSO := (c.GoogleClientID != "" || c.GoogleClientSecret != "")
	useAppleSSO := (c.AppleClientID != "" || c.AppleClientSecret != "")

	vault, err := NewVault(
		ctx,
		VaultConfig{
			ServerBaseURL:      c.ServerBaseURL,
			SymmetricKey:       c.SymmetricKey,
			AsymmetricKey:      c.AsymmetricKey,
			UseGoogleSSO:       useGoogleSSO,
			UseAppleSSO:        useAppleSSO,
			GoogleClientID:     c.GoogleClientID,
			GoogleClientSecret: c.GoogleClientSecret,
			AppleClientID:      c.AppleClientID,
			AppleClientSecret:  c.AppleClientSecret,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to init vault: %w", err)
	}

	dbProvider := DatabaseProviderSQLite
	if c.PostgreSQLDatabaseURL.String() != "" {
		dbProvider = DatabaseProviderPostgreSQL
	}

	store, err := NewStore(StoreConfig{
		DatabaseProvider:      dbProvider,
		PostgreSQLDatabaseURL: c.PostgreSQLDatabaseURL.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to init store: %w", err)
	}

	teiBaseURL := c.TEIBaseURL.String()
	return RunAPI(
		ctx,
		APIConfig{
			ServerBaseURL:               c.ServerBaseURL,
			RequestReadTimeout:          c.RequestReadTimeout,
			RequestWriteTimeout:         c.RequestWriteTimeout,
			GracePeriod:                 c.GracePeriod,
			UseGoogleSSO:                useGoogleSSO,
			UseAppleSSO:                 useAppleSSO,
			TEIBaseURL:                  c.TEIBaseURL,
			TEIAPIKey:                   c.TEIAPIKey,
			UseEmbeddings:               (dbProvider != DatabaseProviderSQLite) && (teiBaseURL != "" && c.TEIAPIKey != ""),
			UseReranker:                 (teiBaseURL != "" && c.TEIAPIKey != ""),
			EmbeddingsJobFrequency:      c.EmbeddingsJobFrequency,
			RecommendationsJobFrequency: c.RecommendationsJobFrequency,
		},
		store,
		vault,
	)
}
