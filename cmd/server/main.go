// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"

	"github.com/akvachan/hirevec-core"
	"github.com/akvachan/hirevec-core/cmd/common"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := common.Loadenv(".env"); err != nil {
		slog.Warn("failed to load .env, using system environment")
	}

	if err := hirevec.RunApp(
		hirevec.AppConfig{
			// Server
			Protocol:            common.Getenv("PROTOCOL", "http"),
			Host:                common.Getenv("HOST", "localhost"),
			Port:                common.Getenv("PORT", "8080"),
			RequestReadTimeout:  common.Getenv("REQUEST_READ_TIMEOUT", "2000"),
			RequestWriteTimeout: common.Getenv("REQUEST_WRITE_TIMEOUT", "2000"),
			GracePeriod:         common.Getenv("GRACE_PERIOD", "5000"),

			// DB
			PostgresHost:     common.Getenv("POSTGRES_HOST", "localhost"),
			PostgresPort:     common.Getenv("POSTGRES_PORT", "5432"),
			PostgresDB:       common.Getenv("POSTGRES_DB", "hirevec"),
			PostgresUser:     os.Getenv("POSTGRES_USER"),
			PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),

			// Logger
			LogLevel: common.Getenv("LOG_LEVEL", "DEBUG"),

			// Crypto
			SymmetricKey:       os.Getenv("SYMMETRIC_KEY"),
			AsymmetricKey:      os.Getenv("ASYMMETRIC_KEY"),
			GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			AppleClientID:      os.Getenv("APPLE_CLIENT_ID"),
			AppleClientSecret:  os.Getenv("APPLE_CLIENT_SECRET"),
		},
	); err != nil {
		common.Exit("app crashed", "err", err)
	}
}
