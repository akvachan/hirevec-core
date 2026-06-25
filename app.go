// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	DefaultLogLevel = slog.LevelDebug
)

func InitLogger(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func ParseDurationWithDefault(value string, defaultValue time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func ParseLogLevelWithDefault(value string, defaultValue slog.Level) slog.Level {
	switch value {
	case "INFO":
		return slog.LevelInfo
	case "ERROR":
		return slog.LevelError
	case "WARN":
		return slog.LevelWarn
	case "DEBUG":
		return slog.LevelDebug
	default:
		return defaultValue
	}
}

func ParseIntWithDefault(value string, defaultValue int) int {
	parsedValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return int(parsedValue)
}

func ParseBoolWithDefault(value string, defaultValue bool) bool {
	parsedValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}

type AppConfig struct {
	ServerBaseURL               string
	LogLevel                    slog.Level
	RequestReadTimeout          time.Duration
	RequestWriteTimeout         time.Duration
	GracePeriod                 time.Duration
	PostgreSQLDatabaseURL       string
	TEIBaseURL                  string
	TEIAPIKey                   string
	EmbeddingsJobFrequency      time.Duration
	RecommendationsJobFrequency time.Duration
	SymmetricKey                string
	AsymmetricKey               string
	GoogleClientID              string
	GoogleClientSecret          string
	AppleClientID               string
	AppleClientSecret           string
}

func RunApp(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	InitLogger(c.LogLevel)

	useGoogleSSO := true
	useAppleSSO := true
	if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
		useGoogleSSO = false
	}
	if c.AppleClientID == "" || c.AppleClientSecret == "" {
		useAppleSSO = false
	}

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
		return fmt.Errorf("vault init failed: %w", err)
	}

	dbProvider := DatabaseProviderSQLite
	if c.PostgreSQLDatabaseURL != "" {
		dbProvider = DatabaseProviderPostgreSQL
	}

	store, err := NewStore(StoreConfig{
		DatabaseProvider:      dbProvider,
		PostgreSQLDatabaseURL: c.PostgreSQLDatabaseURL,
	})
	if err != nil {
		return fmt.Errorf("store init failed: %w", err)
	}

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
			UseEmbeddings:               (dbProvider != DatabaseProviderSQLite) && (c.TEIBaseURL != "" && c.TEIAPIKey != ""),
			UseReranker:                 (c.TEIBaseURL != "" && c.TEIAPIKey != ""),
			EmbeddingsJobFrequency:      c.EmbeddingsJobFrequency,
			RecommendationsJobFrequency: c.RecommendationsJobFrequency,
		},
		store,
		vault,
	)
}
