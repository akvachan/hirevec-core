package utils

import (
	"log/slog"
	"strconv"
	"time"
)

func ParseTimeWithDefault(value string, defaultValue time.Duration) time.Duration {
	parsedReadTimeout, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		slog.Warn(
			"failed to parse time, using default",
			"value", value,
			"default", defaultValue,
		)
		return defaultValue
	}
	return time.Duration(parsedReadTimeout) * time.Millisecond
}

func ParseLogLevelWithDefault(value string, defaultValue slog.Level) slog.Level {
	switch value {
	case "INFO":
		return slog.LevelInfo
	case "ERROR":
		return slog.LevelError
	case "WARN":
		return slog.LevelWarn
	case "DEBUG":
		return slog.LevelDebug
	default:
		slog.Warn(
			"failed to parse log level, using default",
			"value", value,
			"default", defaultValue,
		)
		return defaultValue
	}
}

func ParseUint16WithDefault(value string, defaultValue uint16) uint16 {
	parsedValue, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		slog.Warn(
			"failed to parse uint, using default",
			"value", value,
			"default", defaultValue,
		)
		return defaultValue
	}
	return uint16(parsedValue)
}
