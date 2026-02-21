// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package hirevec implements internal server features
package hirevec

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
)

type AppConfig struct {
	Host               string
	Port               string
	ReadTimeout        string
	WriteTimeout       string
	GracePeriod        string
	PostgresHost       string
	PostgresPort       string
	PostgresDB         string
	PostgresUser       string
	PostgresPassword   string
	LogLevel           string
	SymmetricKeyHex    string
	AsymmetricKeyHex   string
	GoogleClientID     string
	GoogleClientSecret string
	AppleClientID      string
	AppleClientSecret  string
}

func RunApp(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	InitLogger(
		LoggerConfig{
			Level: ParseLogLevelWithDefault(c.LogLevel, DefaultLogLevel),
		},
	)

	vault, err := NewPasetoVault(
		ctx,
		VaultConfig{
			Host:               c.Host,
			Port:               c.Port,
			GoogleClientID:     c.GoogleClientID,
			GoogleClientSecret: c.GoogleClientSecret,
			AppleClientID:      c.AppleClientID,
			AppleClientSecret:  c.AppleClientSecret,
			SymmetricKeyHex:    c.SymmetricKeyHex,
			AsymmetricKeyHex:   c.AsymmetricKeyHex,
		},
	)
	if err != nil {
		return fmt.Errorf("vault init failed: %w", err)
	}

	store, err := NewPostgresStore(
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
			Host:         c.Host,
			Port:         ParseUint16WithDefault(c.Port, 8080),
			ReadTimeout:  ParseTimeWithDefault(c.ReadTimeout, DefaultReadTimeout),
			WriteTimeout: ParseTimeWithDefault(c.WriteTimeout, DefaultWriteTimeout),
			GracePeriod:  ParseTimeWithDefault(c.GracePeriod, DefaultGracePeriod),
		},
		store,
		vault,
	)
}
