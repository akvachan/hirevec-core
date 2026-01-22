// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package utils implements miscellaneous helpful routines.
package utils

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv reads a configuration file at the specified path and loads
// its contents into the process's environment variables.
//
// The file should follow the KEY=VALUE format. The function ignores empty lines,
// lines starting with '#', and handles optional quotes around values.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

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
