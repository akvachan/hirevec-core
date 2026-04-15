// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/akvachan/hirevec-backend"
	"github.com/akvachan/hirevec-backend/cmd/common"
)

func main() {
	if err := hirevec.RunApp(
		hirevec.AppConfig{
			Host:                common.Getenv("HIREVEC_CORE_HOST", "localhost"),
			Port:                common.Getenv("HIREVEC_CORE_PORT", "8080"),
			LogLevel:            common.Getenv("HIREVEC_CORE_LOG_LEVEL", "DEBUG"),
			RequestReadTimeout:  common.Getenv("HIREVEC_CORE_REQUEST_READ_TIMEOUT", "5000"),  // milliseconds
			RequestWriteTimeout: common.Getenv("HIREVEC_CORE_REQUEST_WRITE_TIMEOUT", "5000"), // milliseconds
			GracePeriod:         common.Getenv("HIREVEC_CORE_GRACE_PERIOD", "5000"),          // milliseconds
			GoogleClientID:      common.Getenv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret:  common.Getenv("GOOGLE_CLIENT_SECRET", ""),
			AppleClientID:       common.Getenv("APPLE_CLIENT_ID", ""),
			AppleClientSecret:   common.Getenv("APPLE_CLIENT_SECRET", ""),
		},
	); err != nil {
		common.Logger.Error("app crashed", "err", err)
	}
}
