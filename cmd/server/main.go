// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"

	"github.com/akvachan/hirevec-backend"
	"github.com/akvachan/hirevec-backend/cmd/common"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := common.Loadenv(".env"); err != nil {
		slog.Warn("failed to load .env, using system environment")
	}

	if err := hirevec.RunApp(
		hirevec.AppConfig{
			// Server
			Protocol:            os.Getenv("PROTOCOL"),
			Host:                os.Getenv("HOST"),
			Port:                os.Getenv("PORT"),
			RequestReadTimeout:  os.Getenv("REQUEST_READ_TIMEOUT"),
			RequestWriteTimeout: os.Getenv("REQUEST_WRITE_TIMEOUT"),
			GracePeriod:         os.Getenv("GRACE_PERIOD"),

			// DB
			PostgresHost:     os.Getenv("POSTGRES_HOST"),
			PostgresPort:     os.Getenv("POSTGRES_PORT"),
			PostgresDB:       os.Getenv("POSTGRES_DB"),
			PostgresUser:     os.Getenv("POSTGRES_USER"),
			PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),

			// Logger
			LogLevel: os.Getenv("LOG_LEVEL"),

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
