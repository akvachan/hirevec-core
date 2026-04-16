// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import "os"

func Getenv(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func main() {
	if err := RunApp(
		AppConfig{
			Host:                Getenv("HIREVEC_CORE_HOST", "localhost"),
			Port:                Getenv("HIREVEC_CORE_PORT", "8080"),
			LogLevel:            Getenv("HIREVEC_CORE_LOG_LEVEL", "DEBUG"),
			RequestReadTimeout:  Getenv("HIREVEC_CORE_REQUEST_READ_TIMEOUT", "5000"),  // milliseconds
			RequestWriteTimeout: Getenv("HIREVEC_CORE_REQUEST_WRITE_TIMEOUT", "5000"), // milliseconds
			GracePeriod:         Getenv("HIREVEC_CORE_GRACE_PERIOD", "5000"),          // milliseconds
			GoogleClientID:      Getenv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret:  Getenv("GOOGLE_CLIENT_SECRET", ""),
			AppleClientID:       Getenv("APPLE_CLIENT_ID", ""),
			AppleClientSecret:   Getenv("APPLE_CLIENT_SECRET", ""),
		},
	); err != nil {
		print("app crashed:", err)
	}
}
