// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"

	"github.com/akvachan/hirevec-backend/internal"
	"github.com/akvachan/hirevec-backend/internal/utils"
)

func main() {
	if err := utils.Loadenv(".dev.env"); err != nil {
		slog.Warn(
			"could not load .env, using system environment",
			"err", err,
		)
	}

	if err := app.Run(
		app.AppConfig{
			// Server
			Host:         os.Getenv("HOST"),
			Port:         os.Getenv("PORT"),
			ReadTimeout:  os.Getenv("REQUEST_READ_TIMEOUT"),
			WriteTimeout: os.Getenv("REQUEST_WRITE_TIMEOUT"),
			GracePeriod:  os.Getenv("GRACE_PERIOD"),

			// DB
			PostgresHost:     os.Getenv("PG_HOST"),
			PostgresPort:     os.Getenv("PG_PORT"),
			PostgresDB:       os.Getenv("PG_DB"),
			PostgresUser:     os.Getenv("PG_USER"),
			PostgresPassword: os.Getenv("PG_PASSWORD"),

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
