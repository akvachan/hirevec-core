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

const DefaultLogLevel = slog.LevelError

type LoggerConfig struct {
	Level slog.Level
}

func InitLogger(config LoggerConfig) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: config.Level}))
	slog.SetDefault(logger)
}

func ParseDurationWithDefault(value string, defaultValue time.Duration) time.Duration {
	parsedReadTimeout, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		slog.Warn(
			"failed to parse duration, using default",
			"value", value,
			"default", defaultValue,
		)
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
		slog.Warn(
			"failed to parse log level, using default",
			"value", value,
			"default", defaultValue,
		)
		return defaultValue
	}
}

func ParseUint16WithDefault(value string, defaultValue uint16) uint16 {
	parsedValue, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		slog.Warn(
			"failed to parse uint, using default",
			"value", value,
			"default", defaultValue,
		)
		return defaultValue
	}
	return uint16(parsedValue)
}

type AppConfig struct {
	Protocol            string
	Host                string
	Port                string
	RequestReadTimeout  string
	RequestWriteTimeout string
	GracePeriod         string
	PostgresHost        string
	PostgresPort        string
	PostgresDB          string
	PostgresUser        string
	PostgresPassword    string
	LogLevel            string
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

	InitLogger(
		LoggerConfig{
			Level: ParseLogLevelWithDefault(c.LogLevel, DefaultLogLevel),
		},
	)

	vault, err := NewVault(
		ctx,
		VaultConfig{
			Host:               c.Host,
			Port:               c.Port,
			GoogleClientID:     c.GoogleClientID,
			GoogleClientSecret: c.GoogleClientSecret,
			AppleClientID:      c.AppleClientID,
			AppleClientSecret:  c.AppleClientSecret,
			SymmetricKeyHex:    c.SymmetricKey,
			AsymmetricKeyHex:   c.AsymmetricKey,
		},
	)
	if err != nil {
		return fmt.Errorf("vault init failed: %w", err)
	}

	store, err := NewStore(
		StoreConfig{
			PostgresHost:     c.PostgresHost,
			PostgresPort:     ParseUint16WithDefault(c.PostgresPort, 5432),
			PostgresDB:       c.PostgresDB,
			PostgresUser:     c.PostgresUser,
			PostgresPassword: c.PostgresPassword,
		},
	)
	if err != nil {
		return fmt.Errorf("store init failed: %w", err)
	}

	return RunServer(
		ctx,
		ServerConfig{
			Protocol:     c.Protocol,
			Host:         c.Host,
			Port:         ParseUint16WithDefault(c.Port, 8080),
			ReadTimeout:  ParseDurationWithDefault(c.RequestReadTimeout, DefaultReadTimeout),
			WriteTimeout: ParseDurationWithDefault(c.RequestWriteTimeout, DefaultWriteTimeout),
			GracePeriod:  ParseDurationWithDefault(c.GracePeriod, DefaultGracePeriod),
		},
		store,
		vault,
	)
}
