// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package hirevec implements internal server features
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

func InitLogger(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func ParseDurationWithDefault(value string, defaultValue time.Duration) time.Duration {
	parsedReadTimeout, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return time.Duration(parsedReadTimeout) * time.Millisecond
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
	ServerBaseURL       string
	LogLevel            slog.Level
	RequestReadTimeout  time.Duration
	RequestWriteTimeout time.Duration
	GracePeriod         time.Duration
	PostgresDatabaseURL string
	TEIBaseURL          string
	TEIAPIKey           string
	SymmetricKey        string
	AsymmetricKey       string
	GoogleClientID      string
	GoogleClientSecret  string
	AppleClientID       string
	AppleClientSecret   string
}

func RunApp(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.SetLogLoggerLevel(c.LogLevel)

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

	usePostgres := true
	if c.PostgresDatabaseURL == "" {
		usePostgres = false
	}

	store, err := NewStore(StoreConfig{
		UsePostgres:         usePostgres,
		PostgresDatabaseURL: c.PostgresDatabaseURL,
	})
	if err != nil {
		return fmt.Errorf("store init failed: %w", err)
	}

	useEmbeddings := true
	useReranker := true
	if !usePostgres {
		useEmbeddings = false
	}
	if c.TEIBaseURL == "" || c.TEIAPIKey == "" {
		useEmbeddings = false
		useReranker = false
	}

	return RunServer(
		ctx,
		ServerConfig{
			ServerBaseURL:       c.ServerBaseURL,
			RequestReadTimeout:  c.RequestReadTimeout,
			RequestWriteTimeout: c.RequestWriteTimeout,
			GracePeriod:         c.GracePeriod,
			UseGoogleSSO:        useGoogleSSO,
			UseAppleSSO:         useAppleSSO,
			TEIBaseURL:          c.TEIBaseURL,
			TEIAPIKey:           c.TEIAPIKey,
			UseEmbeddings:       useEmbeddings,
			UseReranker:         useReranker,
		},
		*store,
		*vault,
	)
}
