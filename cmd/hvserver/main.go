// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/akvachan/hirevec-core"
)

func main() {
	hirevec.InitLogger(
		hirevec.MustParseEnv("HIREVEC_LOG_LEVEL", hirevec.ParseLogLevel, hirevec.DefaultLogLevel),
		hirevec.MustParseEnv("HIREVEC_LOG_JSON", strconv.ParseBool, false),
	)

	if err := hirevec.Loadenv(".env"); err != nil {
		slog.Warn("could not load .env, using system environment", "err", err)
	}

	if err := hirevec.RunApp(hirevec.AppConfig{
		LogLevel: hirevec.MustParseEnv("HIREVEC_LOG_LEVEL", hirevec.ParseLogLevel, hirevec.DefaultLogLevel),
		LogJSON:  hirevec.MustParseEnv("HIREVEC_LOG_JSON", strconv.ParseBool, false),

		ServerBaseURL:       hirevec.MustParseEnv("HIREVEC_BASE_URL", url.Parse, &url.URL{Host: "localhost:8888"}),
		RequestReadTimeout:  hirevec.MustParseEnv("HIREVEC_REQUEST_READ_TIMEOUT", time.ParseDuration, hirevec.DefaultRequestReadTimeout),
		RequestWriteTimeout: hirevec.MustParseEnv("HIREVEC_REQUEST_WRITE_TIMEOUT", time.ParseDuration, hirevec.DefaultRequestWriteTimeout),
		GracePeriod:         hirevec.MustParseEnv("HIREVEC_GRACE_PERIOD", time.ParseDuration, hirevec.DefaultGracePeriod),

		SymmetricKey:  os.Getenv("HIREVEC_SYMMETRIC_KEY"),
		AsymmetricKey: os.Getenv("HIREVEC_ASYMMETRIC_KEY"),

		PostgreSQLURL: hirevec.MustParseEnv("HIREVEC_POSTGRESQL_URL", url.Parse, &url.URL{Host: ""}),

		LlamaCppURL:             hirevec.MustParseEnv("HIREVEC_LLAMACPP_URL", url.Parse, &url.URL{Host: ""}),
		LlamaCppAPIKey:          os.Getenv("HIREVEC_LLAMACPP_API_KEY"),
		LlamaCppEmbeddingsModel: os.Getenv("HIREVEC_LLAMACPP_EMBEDDINGS_MODEL"),
		LlamaCppEmbeddingsDim:   hirevec.MustParseEnv("HIREVEC_LLAMACPP_EMBEDDINGS_DIM", hirevec.ParseUint, 768),
		LlamaCppRerankerModel:   os.Getenv("HIREVEC_LLAMACPP_RERANKER_MODEL"),

		EmbeddingsJobFrequency:      hirevec.MustParseEnv("HIREVEC_EMBEDDINGS_JOB_FREQUENCY", time.ParseDuration, hirevec.DefaultEmbeddingsJobFrequency),
		RecommendationsJobFrequency: hirevec.MustParseEnv("HIREVEC_RECOMMENDATIONS_JOB_FREQUENCY", time.ParseDuration, hirevec.DefaultRecommendationsJobFrequency),

		DevelopmentMode:                  hirevec.MustParseEnv("HIREVEC_DEVELOPMENT_MODE", strconv.ParseBool, false),
		DevelopmentModeDropTablesOnExit:  hirevec.MustParseEnv("HIREVEC_DEVELOPMENT_MODE_DROP_TABLES_ON_EXIT", strconv.ParseBool, false),
		DevelopmentModeTokensNeverExpire: hirevec.MustParseEnv("HIREVEC_DEVELOPMENT_MODE_TOKENS_NEVER_EXPIRE", strconv.ParseBool, false),
	}); err != nil {
		slog.Error(
			"failed to run app",
			"err", err,
		)
	}
}
