// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
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

func InitLogger(level slog.Level, json bool) error {
	if json {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	}

	return nil
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

func ParseEmbeddingsDim(value string) (uint32, error) {
	parsedValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse embeddings dim: %w", err)
	}
	return uint32(parsedValue), nil
}

func ParseUserPassword(value string) (string, error) {
	if err := ValidateUserPassword(value); err != nil {
		return "", fmt.Errorf("failed to parse user password: %w", err)
	}
	return value, nil
}

type AppConfig struct {
	LogLevel slog.Level
	LogJSON  bool

	ServerBaseURL       *url.URL
	RequestReadTimeout  time.Duration
	RequestWriteTimeout time.Duration
	GracePeriod         time.Duration

	SymmetricKey  string
	AsymmetricKey string

	PostgreSQLURL *url.URL

	LlamaCppURL             *url.URL
	LlamaCppAPIKey          string
	LlamaCppEmbeddingsModel string
	LlamaCppEmbeddingsDim   uint32
	LlamaCppRerankerModel   string

	EmbeddingsJobFrequency      time.Duration
	RecommendationsJobFrequency time.Duration

	DevelopmentMode                  bool
	DevelopmentModeDropTablesOnExit  bool
	DevelopmentModeTokensNeverExpire bool
	DevelopmentModeUserFullName      string
	DevelopmentModeUserName          string
	DevelopmentModeUserEmail         *mail.Address
	DevelopmentModeUserPassword      string
}

func RunApp(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := InitLogger(c.LogLevel, c.LogJSON); err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}

	if c.DevelopmentMode {
		slog.Warn("development mode activated")
		if c.DevelopmentModeDropTablesOnExit {
			slog.Warn("dropping tables on exit", "development_mode", c.DevelopmentMode)
		}

		if c.DevelopmentModeUserPassword == "" {
			return fmt.Errorf("failed to proceed in development mode without user password")
		}
	}

	vault, err := NewVault(
		ctx,
		VaultConfig{
			ServerBaseURL:                    c.ServerBaseURL,
			SymmetricKey:                     c.SymmetricKey,
			AsymmetricKey:                    c.AsymmetricKey,
			DevelopmentMode:                  c.DevelopmentMode,
			DevelopmentModeTokensNeverExpire: c.DevelopmentModeTokensNeverExpire,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to init vault: %w", err)
	}

	dbProvider := DatabaseProviderSQLite
	if c.PostgreSQLURL.String() != "" {
		dbProvider = DatabaseProviderPostgreSQL
	}

	store, err := NewStore(StoreConfig{
		DatabaseProvider:      dbProvider,
		PostgreSQLDatabaseURL: c.PostgreSQLURL,

		LlamaCppEmbeddingsDim: c.LlamaCppEmbeddingsDim,

		DevelopmentMode:                 c.DevelopmentMode,
		DevelopmentModeDropTablesOnExit: c.DevelopmentModeDropTablesOnExit,
		DevelopmentModeUserFullName:     c.DevelopmentModeUserFullName,
		DevelopmentModeUserName:         c.DevelopmentModeUserName,
		DevelopmentModeUserEmail:        c.DevelopmentModeUserEmail,
		DevelopmentModeUserPassword:     c.DevelopmentModeUserPassword,
	})
	if err != nil {
		return fmt.Errorf("failed to init store: %w", err)
	}

	useLlamaCpp := false
	if c.LlamaCppURL.String() != "" {
		if c.LlamaCppAPIKey != "" {
			useLlamaCpp = true
		} else {
			slog.Warn("refusing to use llama-cpp server due to missing API key")
		}
	}

	return RunAPI(
		ctx,
		APIConfig{
			ServerURL:           c.ServerBaseURL,
			RequestReadTimeout:  c.RequestReadTimeout,
			RequestWriteTimeout: c.RequestWriteTimeout,
			GracePeriod:         c.GracePeriod,

			LlamaCppURL:             c.LlamaCppURL.String(),
			LlamaCppAPIKey:          c.LlamaCppAPIKey,
			LlamaCppRerankerModel:   c.LlamaCppRerankerModel,
			LlamaCppEmbeddingsDim:   c.LlamaCppEmbeddingsDim,
			LlamaCppEmbeddingsModel: c.LlamaCppEmbeddingsModel,

			UseEmbeddings:               (dbProvider != DatabaseProviderSQLite) && useLlamaCpp,
			UseReranker:                 useLlamaCpp,
			EmbeddingsJobFrequency:      c.EmbeddingsJobFrequency,
			RecommendationsJobFrequency: c.RecommendationsJobFrequency,

			DevelopmentMode:                 c.DevelopmentMode,
			DevelopmentModeDropTablesOnExit: c.DevelopmentModeDropTablesOnExit,
		},
		store,
		vault,
	)
}
