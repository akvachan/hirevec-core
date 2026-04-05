// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/akvachan/hirevec-core/cmd/common"
)

var (
	initSQLPath   = path.Join("cmd", "setup", "init.sql")
	devSQLPath    = path.Join("cmd", "setup", "dev.sql")
	sentinelTable = "v1.users"
)

var requiredVars = []string{
	"POSTGRES_USER",
	"POSTGRES_PASSWORD",
	"POSTGRES_DB",
}

func main() {
	dev := flag.Bool("dev", false, "apply additional dev SQL")
	flag.Parse()

	if err := common.Loadenv(".env"); err != nil {
		common.Log.Warn("failed to load .env, using system environment", "err", err)
	}
	common.CheckEnvVars(requiredVars)

	checkPostgres()
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
	common.Log.Info("psql found", "version", strings.TrimSpace(string(out)))

	host := common.Getenv("POSTGRES_HOST", "localhost")
	port := common.Getenv("POSTGRES_PORT", "5432")

	if path, err := exec.LookPath("pg_isready"); err == nil {
		if err := exec.Command(path, "-h", host, "-p", port).Run(); err != nil {
			common.Exit("postgres not reachable, start it first", "host", host, "port", port)
		}
		common.Log.Info("postgres is reachable", "host", host, "port", port)
	}
}

func createUserAndDB() {
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	superuser := common.DetectSuperuser()
	common.Log.Info("provisioning user via superuser", "user", user, "db", dbName)

	common.RunPsqlSuper(superuser, "postgres", "create role",
		`
		do $$ 
		begin
		if not exists (select from pg_roles where rolname = '`+user+`') then
			create role `+user+` with login password '`+password+`';
		end if;
		end $$;
		`,
	)

	out, err := common.PsqlSuper(superuser, "postgres", "-tAc",
		"select 1 from pg_database where datname = '"+dbName+"';",
	).Output()
	if err != nil {
		common.Exit("failed to check database existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		common.RunPsqlSuper(superuser, "postgres", "create database", "create database "+dbName+" owner "+user+";")
	} else {
		common.Log.Info("database already exists, skipping creation", "db", dbName)
	}

	common.RunPsqlSuper(superuser, "postgres", "grant privileges", "grant all privileges on database "+dbName+" TO "+user+";")
	common.RunPsqlSuper(superuser, dbName, "creating extension", "create extension if not exists vector;")

	common.Log.Info("role and database ready", "user", user, "db", dbName)
}

func initDB() {
	out, err := common.Psql("-c", "select to_regclass('"+sentinelTable+"');").Output()
	if err != nil {
		common.Exit("failed to query database", "err", err)
	}

	if strings.Contains(string(out), sentinelTable) {
		common.Log.Info("database already initialized, skipping init.sql")
		return
	}

	common.Log.Info("initializing database", "file", initSQLPath)
	common.RunPsql(common.Psql("-f", initSQLPath), "init db")
	common.Log.Info("database initialized")
}

func ingestData() {
	if _, err := os.Stat(devSQLPath); err == nil {
		common.Log.Info("applying dev SQL", "file", devSQLPath)
		common.RunPsql(common.Psql("-f", devSQLPath), "ingest data")
		common.Log.Info("ingesting embeddings")
	} else {
		common.Log.Warn("dev flag set but dev.sql not found, skipping")
	}
}
