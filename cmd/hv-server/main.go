// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"log/slog"
	"os"

	"github.com/akvachan/hirevec-core"
)

func main() {
	hirevec.InitLogger(slog.LevelWarn)

	if err := hirevec.Loadenv(".env"); err != nil {
		slog.Warn("could not load .env, using system environment", "err", err)
	}

	if err := hirevec.RunApp(
		hirevec.AppConfig{
			ServerBaseURL:               hirevec.Getenv("HIREVEC_BASE_URL", "localhost:8888"),
			RequestReadTimeout:          hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_REQUEST_READ_TIMEOUT"), hirevec.DefaultRequestReadTimeout),
			RequestWriteTimeout:         hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_REQUEST_WRITE_TIMEOUT"), hirevec.DefaultRequestWriteTimeout),
			GracePeriod:                 hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_GRACE_PERIOD"), hirevec.DefaultGracePeriod),
			LogLevel:                    hirevec.ParseLogLevelWithDefault(os.Getenv("HIREVEC_LOG_LEVEL"), hirevec.DefaultLogLevel),
			PostgreSQLDatabaseURL:       os.Getenv("POSTGRESQL_DATABASE_URL"),
			TEIBaseURL:                  os.Getenv("TEI_BASE_URL"),
			TEIAPIKey:                   os.Getenv("TEI_API_KEY"),
			EmbeddingsJobFrequency:      hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_EMBEDDINGS_JOB_FREQUENCY"), hirevec.DefaultEmbeddingsJobFrequency),
			RecommendationsJobFrequency: hirevec.ParseDurationWithDefault(os.Getenv("HIREVEC_RECOMMENDATIONS_JOB_FREQUENCY"), hirevec.DefaultRecommendationsJobFrequency),
			SymmetricKey:                os.Getenv("HIREVEC_SYMMETRIC_KEY"),
			AsymmetricKey:               os.Getenv("HIREVEC_ASYMMETRIC_KEY"),
			GoogleClientID:              os.Getenv("GOOGLE_CLIENT_ID"),
			GoogleClientSecret:          os.Getenv("GOOGLE_CLIENT_SECRET"),
			AppleClientID:               os.Getenv("APPLE_CLIENT_ID"),
			AppleClientSecret:           os.Getenv("APPLE_CLIENT_SECRET"),
		},
	); err != nil {
		slog.Error(
			"failed to run app",
			"err", err,
		)
	}
}
