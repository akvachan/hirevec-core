// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/akvachan/hirevec-backend/cmd/common"
)

var (
	initSQLPath   = path.Join("cmd", "setup", "init.sql")
	devSQLPath    = path.Join("cmd", "setup", "dev.sql")
	sentinelTable = "general.users"
)

var requiredVars = []string{
	"POSTGRES_USER",
	"POSTGRES_PASSWORD",
	"POSTGRES_DB",
}

var log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

func main() {
	dev := flag.Bool("dev", false, "apply additional dev SQL")
	flag.Parse()

	if err := common.Loadenv(".env"); err != nil {
		log.Warn("failed to load .env, using system environment", "err", err)
	}

	checkPostgres()
	checkEnvVars()
	createUserAndDB()
	initDB()

	if *dev {
		ingestData()
	}
}

func checkPostgres() {
	if _, err := exec.LookPath("psql"); err != nil {
		var hint string
		switch runtime.GOOS {
		case "darwin":
			hint = "brew install postgresql"
		case "linux":
			hint = "sudo apt install postgresql-client"
		default:
			hint = "https://www.postgresql.org/download/"
		}
		common.Exit("psql not found", "hint", hint)
	}

	out, _ := exec.Command("psql", "--version").Output()
	log.Info("psql found", "version", strings.TrimSpace(string(out)))

	host := common.Getenv("POSTGRES_HOST", "localhost")
	port := common.Getenv("POSTGRES_PORT", "5432")

	if path, err := exec.LookPath("pg_isready"); err == nil {
		if err := exec.Command(path, "-h", host, "-p", port).Run(); err != nil {
			common.Exit("postgres not reachable, start it first", "host", host, "port", port)
		}
		log.Info("postgres is reachable", "host", host, "port", port)
	}
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
	log.Info("all required environment variables are set")
}

func createUserAndDB() {
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	superuser := detectSuperuser()
	log.Info("provisioning user via superuser", "user", user, "db", dbName)

	runSuper(
		superuser, "create role",
		`
		do $$ 
		begin
		if not exists (select from pg_roles where rolname = '`+user+`') then
			create role `+user+` with login password '`+password+`';
		end if;
		end $$;
		`,
	)

	out, err := psqlSuper(superuser, "-tAc",
		"select 1 from pg_database where datname = '"+dbName+"';",
	).Output()
	if err != nil {
		common.Exit("failed to check database existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		runSuper(superuser, "create database", "CREATE DATABASE "+dbName+" OWNER "+user+";")
	} else {
		log.Info("database already exists, skipping creation", "db", dbName)
	}

	runSuper(superuser, "grant privileges", "GRANT ALL PRIVILEGES ON DATABASE "+dbName+" TO "+user+";")

	log.Info("role and database ready", "user", user, "db", dbName)
}

func initDB() {
	out, err := psqlApp("-c", "select to_regclass('"+sentinelTable+"');").Output()
	if err != nil {
		common.Exit("failed to query database", "err", err)
	}

	if strings.Contains(string(out), sentinelTable) {
		log.Info("database already initialized, skipping init.sql")
		return
	}

	log.Info("initializing database", "file", initSQLPath)
	cmd := psqlApp("-f", initSQLPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		common.Exit("database initialization failed", "err", err)
	}
	log.Info("database initialized")
}

func detectSuperuser() string {
	if v := os.Getenv("POSTGRES_SUPERUSER"); v != "" {
		return v
	}

	host := common.Getenv("POSTGRES_HOST", "localhost")
	port := common.Getenv("POSTGRES_PORT", "5432")

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
		common.Exit("provisioning failed", "op", op, "err", err)
	}
}

func psqlSuper(superuser string, args ...string) *exec.Cmd {
	base := []string{
		"-h", common.Getenv("POSTGRES_HOST", "localhost"),
		"-p", common.Getenv("POSTGRES_PORT", "5432"),
		"-U", superuser,
		"-d", "postgres",
	}
	return exec.Command("psql", append(base, args...)...)
}

func psqlApp(args ...string) *exec.Cmd {
	base := []string{
		"-h", common.Getenv("POSTGRES_HOST", "localhost"),
		"-p", common.Getenv("POSTGRES_PORT", "5432"),
		"-U", os.Getenv("POSTGRES_USER"),
		"-d", os.Getenv("POSTGRES_DB"),
	}
	cmd := exec.Command("psql", append(base, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("POSTGRES_PASSWORD"))
	return cmd
}

func ingestData() {
	if _, err := os.Stat(devSQLPath); err == nil {
		log.Info("applying dev SQL", "file", devSQLPath)
		cmd := psqlApp("-f", devSQLPath)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			common.Exit("dev SQL execution failed", "err", err)
		}
	} else {
		log.Warn("dev flag set but dev.sql not found, skipping")
	}
}
