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

func ParseUint16WithDefault(value string, defaultValue uint16) uint16 {
	parsedValue, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return defaultValue
	}
	return uint16(parsedValue)
}

func ParseIntWithDefault(value string, defaultValue int) int {
	parsedValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return int(parsedValue)
}

type AppConfig struct {
	Host                string
	Port                uint16
	LogLevel            slog.Level
	RequestReadTimeout  time.Duration
	RequestWriteTimeout time.Duration
	GracePeriod         time.Duration
	PostgresDatabaseURL string
	TestTokenUserID     string
	TestTokenProvider   string
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

	InitLogger(c.LogLevel)

	vault, err := NewVault(
		ctx,
		VaultConfig{
			ServerHost:         c.Host,
			ServerPort:         c.Port,
			SymmetricKey:       c.SymmetricKey,
			AsymmetricKey:      c.AsymmetricKey,
			GoogleClientID:     c.GoogleClientID,
			GoogleClientSecret: c.GoogleClientSecret,
			AppleClientID:      c.AppleClientID,
			AppleClientSecret:  c.AppleClientSecret,
		},
	)
	if err != nil {
		return fmt.Errorf("vault init failed: %w", err)
	}

	store, err := NewStore(StoreConfig{
		PostgresDatabaseURL: c.PostgresDatabaseURL,
	})
	if err != nil {
		return fmt.Errorf("store init failed: %w", err)
	}

	return RunServer(
		ctx,
		ServerConfig{
			Host:         c.Host,
			Port:         c.Port,
			ReadTimeout:  c.RequestReadTimeout,
			WriteTimeout: c.RequestWriteTimeout,
			GracePeriod:  c.GracePeriod,
		},
		*store,
		*vault,
	)
}
