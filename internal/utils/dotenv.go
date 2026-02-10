// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package utils implements miscellaneous helpful routines.
package utils

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
)

// Loadenv reads a configuration file at the specified path and loads
// its contents into the process's environment variables.
//
// The file should follow the KEY=VALUE format. The function ignores empty lines,
// lines starting with '#', and handles optional quotes around values.
func Loadenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error(
				"could not properly close file",
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
