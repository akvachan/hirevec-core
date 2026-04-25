// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package server

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/akvachan/hirevec-core/internal"
)

func main() {
	if err := hirevec.Loadenv(".env"); err != nil {
		print("could not load .env, using system environment")
	}

	if err := hirevec.RunApp(
		hirevec.AppConfig{
			Host:                hirevec.Getenv("HIREVEC_HOST", "localhost"),
			Port:                hirevec.ParseUint16WithDefault(os.Getenv("HIREVEC_PORT"), 8080),
			RequestReadTimeout:  hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_REQUEST_READ_TIMEOUT"), 2000*time.Millisecond),
			RequestWriteTimeout: hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_REQUEST_WRITE_TIMEOUT"), 2000*time.Millisecond),
			GracePeriod:         hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_GRACE_PERIOD"), 5000*time.Millisecond),
			LogLevel:            hirevec.ParseLogLevelWithDefault(os.Getenv("HIREVEC_LOG_LEVEL"), slog.LevelDebug),
			PostgresDatabaseURL: hirevec.Getenv("POSTGRES_DATABASE_URL", fmt.Sprintf("postgres://%s@localhost:5432/postgres?sslmode=disable", os.Getenv("USER"))),
			SymmetricKey:        os.Getenv("SYMMETRIC_KEY"),
			AsymmetricKey:       os.Getenv("ASYMMETRIC_KEY"),
			GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
			GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
			AppleClientID:       os.Getenv("APPLE_CLIENT_ID"),
			AppleClientSecret:   os.Getenv("APPLE_CLIENT_SECRET"),
		},
	); err != nil {
		print("app crashed: ", err.Error())
	}
}
