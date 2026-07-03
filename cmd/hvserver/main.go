// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/akvachan/hirevec-core"
)

func main() {
	hirevec.InitLogger(hirevec.GetenvAndParse("HIREVEC_LOG_LEVEL", hirevec.ParseLogLevel, hirevec.DefaultLogLevel))

	if err := hirevec.Loadenv(".env"); err != nil {
		slog.Warn("could not load .env, using system environment", "err", err)
	}

	if err := hirevec.RunApp(hirevec.AppConfig{
		ServerBaseURL:               hirevec.Getenv("HIREVEC_BASE_URL", "localhost:8888"),
		RequestReadTimeout:          hirevec.GetenvAndParse("HIREVEC_REQUEST_READ_TIMEOUT", time.ParseDuration, hirevec.DefaultRequestReadTimeout),
		RequestWriteTimeout:         hirevec.GetenvAndParse("HIREVEC_REQUEST_WRITE_TIMEOUT", time.ParseDuration, hirevec.DefaultRequestWriteTimeout),
		GracePeriod:                 hirevec.GetenvAndParse("HIREVEC_GRACE_PERIOD", time.ParseDuration, hirevec.DefaultGracePeriod),
		LogLevel:                    hirevec.GetenvAndParse("HIREVEC_LOG_LEVEL", hirevec.ParseLogLevel, hirevec.DefaultLogLevel),
		EmbeddingsJobFrequency:      hirevec.GetenvAndParse("HIREVEC_EMBEDDINGS_JOB_FREQUENCY", time.ParseDuration, hirevec.DefaultEmbeddingsJobFrequency),
		RecommendationsJobFrequency: hirevec.GetenvAndParse("HIREVEC_RECOMMENDATIONS_JOB_FREQUENCY", time.ParseDuration, hirevec.DefaultRecommendationsJobFrequency),
		PostgreSQLDatabaseURL:       os.Getenv("POSTGRESQL_DATABASE_URL"),
		TEIBaseURL:                  os.Getenv("TEI_BASE_URL"),
		TEIAPIKey:                   os.Getenv("TEI_API_KEY"),
		SymmetricKey:                os.Getenv("HIREVEC_SYMMETRIC_KEY"),
		AsymmetricKey:               os.Getenv("HIREVEC_ASYMMETRIC_KEY"),
		GoogleClientID:              os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:          os.Getenv("GOOGLE_CLIENT_SECRET"),
		AppleClientID:               os.Getenv("APPLE_CLIENT_ID"),
		AppleClientSecret:           os.Getenv("APPLE_CLIENT_SECRET"),
	}); err != nil {
		slog.Error(
			"failed to run app",
			"err", err,
		)
	}
}
