// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package common implements common helper functions for the scripts
package common

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

func RunPsql(cmd *exec.Cmd, op string) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	emitPsqlOutput(stderr.String(), op)
	emitPsqlOutput(stdout.String(), op)

	if err != nil {
		Exit("query failed", "op", op, "err", err)
	}
}

func Psql(args ...string) *exec.Cmd {
	base := []string{
		"-h", Getenv("POSTGRES_HOST", "localhost"),
		"-p", Getenv("POSTGRES_PORT", "5432"),
		"-U", os.Getenv("POSTGRES_USER"),
		"-d", os.Getenv("POSTGRES_DB"),
	}
	cmd := exec.Command("psql", append(base, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("POSTGRES_PASSWORD"))
	return cmd
}

func RunPsqlSuper(superuser, db, op, stmt string) {
	cmd := PsqlSuper(superuser, db, "-c", stmt)
	RunPsql(cmd, op)
}

func PsqlSuper(superuser, db string, args ...string) *exec.Cmd {
	base := []string{
		"-h", Getenv("POSTGRES_HOST", "localhost"),
		"-p", Getenv("POSTGRES_PORT", "5432"),
		"-U", superuser,
		"-d", db,
	}
	return exec.Command("psql", append(base, args...)...)
}

// emitPsqlOutput parses postgres output lines and routes them to the right slog level.
func emitPsqlOutput(output, op string) {
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "ERROR:"), strings.HasPrefix(line, "FATAL:"), strings.HasPrefix(line, "PANIC:"):
			Log.Error("postgres error", "op", op, "msg", line)
		case strings.HasPrefix(line, "WARNING:"):
			Log.Warn("postgres warning", "op", op, "msg", line)
		case strings.HasPrefix(line, "NOTICE:"), strings.HasPrefix(line, "INFO:"), strings.HasPrefix(line, "HINT:"):
			Log.Info("postgres info", "op", op, "msg", line)
		case strings.HasPrefix(line, "DEBUG:"):
			Log.Debug("postgres debug", "op", op, "msg", line)
		default:
			Log.Debug("postgres debug", "op", op, "msg", line)
		}
	}
}
