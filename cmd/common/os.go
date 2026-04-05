// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package common implements common helper functions for the scripts
package common

import (
	"os"
	"os/exec"
	"strings"
)

func OsUsername() (string, error) {
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func DetectSuperuser() string {
	if v := os.Getenv("POSTGRES_SUPERUSER"); v != "" {
		return v
	}

	host := Getenv("POSTGRES_HOST", "localhost")
	port := Getenv("POSTGRES_PORT", "5432")

	candidates := []string{"postgres"}
	if u, err := OsUsername(); err == nil && u != "postgres" {
		candidates = append(candidates, u)
	}

	for _, u := range candidates {
		cmd := exec.Command("psql", "-h", host, "-p", port, "-U", u, "-d", "postgres", "-c", "SELECT 1;")
		if err := cmd.Run(); err == nil {
			return u
		}
	}

	if u, err := OsUsername(); err == nil {
		return u
	}
	return "postgres"
}

func CheckEnvVars(requiredVars []string) {
	var missing []string
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		Exit("missing required environment variables", "vars", missing)
	}
}
