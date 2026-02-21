// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"log/slog"
	"os"
)

const DefaultLogLevel = slog.LevelError

type LoggerConfig struct {
	Level slog.Level
}

func InitLogger(config LoggerConfig) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: config.Level}))
	slog.SetDefault(logger)
}
