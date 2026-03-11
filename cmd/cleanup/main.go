// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/akvachan/hirevec-backend/cmd/common"
)

var requiredVars = []string{
	"POSTGRES_USER",
	"POSTGRES_DB",
}

var log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

func main() {
	if err := common.Loadenv(".env"); err != nil {
		log.Warn("failed to load .env, using system environment", "err", err)
	}
	checkEnvVars()

	user := os.Getenv("POSTGRES_USER")
	dbName := os.Getenv("POSTGRES_DB")
	superuser := detectSuperuser()

	log.Info("starting cleanup", "superuser", superuser, "user", user, "db", dbName)

	dropDB(superuser, dbName)
	dropRole(superuser, user)

	log.Info("cleanup complete")
}

func checkEnvVars() {
	var missing []string
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		log.Error("missing required environment variables", "vars", missing)
		os.Exit(1)
	}
}

func dropDB(superuser string, dbName string) {
	out, err := psqlSuper(superuser, "-tAc",
		"SELECT 1 FROM pg_database WHERE datname = '"+dbName+"';",
	).Output()
	if err != nil {
		die("failed to check database existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		log.Info("database does not exist, skipping", "db", dbName)
		return
	}

	runSuper(superuser, "terminate connections",
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '"+dbName+"' AND pid <> pg_backend_pid();",
	)

	runSuper(superuser, "drop database", "DROP DATABASE "+dbName+";")
	log.Info("database dropped", "db", dbName)
}

func dropRole(superuser, user string) {
	out, err := psqlSuper(superuser, "-tAc",
		"SELECT 1 FROM pg_roles WHERE rolname = '"+user+"';",
	).Output()
	if err != nil {
		die("failed to check role existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		log.Info("role does not exist, skipping", "role", user)
		return
	}

	runSuper(superuser, "drop role", "DROP ROLE "+user+";")
	log.Info("role dropped", "role", user)
}

func detectSuperuser() string {
	if v := os.Getenv("POSTGRES_SUPERUSER"); v != "" {
		return v
	}

	host := envOr("POSTGRES_HOST", "localhost")
	port := envOr("POSTGRES_PORT", "5432")

	candidates := []string{"postgres"}
	if u, err := osUsername(); err == nil && u != "postgres" {
		candidates = append(candidates, u)
	}

	for _, u := range candidates {
		cmd := exec.Command("psql", "-h", host, "-p", port, "-U", u, "-d", "postgres", "-c", "SELECT 1;")
		if err := cmd.Run(); err == nil {
			return u
		}
	}

	if u, err := osUsername(); err == nil {
		return u
	}
	return "postgres"
}

func osUsername() (string, error) {
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runSuper(superuser, op, stmt string) {
	cmd := psqlSuper(superuser, "-c", stmt)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		die("cleanup failed", "op", op, "err", err)
	}
}

func psqlSuper(superuser string, args ...string) *exec.Cmd {
	base := []string{
		"-h", envOr("POSTGRES_HOST", "localhost"),
		"-p", envOr("POSTGRES_PORT", "5432"),
		"-U", superuser,
		"-d", "postgres",
	}
	return exec.Command("psql", append(base, args...)...)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func die(msg string, args ...any) {
	log.Error(msg, args...)
	os.Exit(1)
}
