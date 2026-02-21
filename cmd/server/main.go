// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"

	hirevec "github.com/akvachan/hirevec-backend/internal"
)

func main() {
	if err := hirevec.Loadenv(".env"); err != nil {
		slog.Warn(
			"could not load .env, using system environment",
			"err", err,
		)
	}

	if err := hirevec.RunApp(
		hirevec.AppConfig{
			// Server
			Host:         os.Getenv("HOST"),
			Port:         os.Getenv("PORT"),
			ReadTimeout:  os.Getenv("REQUEST_READ_TIMEOUT"),
			WriteTimeout: os.Getenv("REQUEST_WRITE_TIMEOUT"),
			GracePeriod:  os.Getenv("GRACE_PERIOD"),

			// DB
			PostgresHost:     os.Getenv("POSTGRES_HOST"),
			PostgresPort:     os.Getenv("POSTGRES_PORT"),
			PostgresDB:       os.Getenv("POSTGRES_DB"),
			PostgresUser:     os.Getenv("POSTGRES_USER"),
			PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),

			// Logger
			LogLevel: os.Getenv("LOG_LEVEL"),

			// Crypto
			SymmetricKeyHex:    os.Getenv("SYMMETRIC_KEY"),
			AsymmetricKeyHex:   os.Getenv("ASYMMETRIC_KEY"),
			GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			AppleClientID:      os.Getenv("APPLE_CLIENT_ID"),
			AppleClientSecret:  os.Getenv("APPLE_CLIENT_SECRET"),
		},
	); err != nil {
		slog.Error(
			"app crashed",
			"err", err,
		)
		os.Exit(1)
	}
}
