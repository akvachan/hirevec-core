// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package common implements common helper functions for the scripts
package common

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
)

func Loadenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error(
				"failed to properly close file",
				"err", err,
			)
			os.Exit(0)
		}
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		value = strings.Trim(value, `"'`)

		err = os.Setenv(key, value)
		if err != nil {
			return err
		}
	}

	return scanner.Err()
}

func Getenv(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}
