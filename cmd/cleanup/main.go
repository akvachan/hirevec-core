// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/akvachan/hirevec-core/cmd/common"
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
	common.CheckEnvVars(requiredVars)

	user := os.Getenv("POSTGRES_USER")
	dbName := os.Getenv("POSTGRES_DB")
	superuser := common.DetectSuperuser()

	log.Info("starting cleanup", "superuser", superuser, "user", user, "db", dbName)

	dropDB(superuser, dbName)
	dropRole(superuser, user)

	log.Info("cleanup complete")
}

func dropDB(superuser string, dbName string) {
	out, err := common.PsqlSuper(superuser, "postgres", "-tAc",
		"select 1 from pg_database where datname = '"+dbName+"';",
	).Output()
	if err != nil {
		common.Exit("failed to check database existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		log.Info("database does not exist, skipping", "db", dbName)
		return
	}

	common.RunPsqlSuper(superuser, "postgres", "terminate connections",
		"select pg_terminate_backend(pid) from pg_stat_activity where datname = '"+dbName+"' and pid <> pg_backend_pid();",
	)

	common.RunPsqlSuper(superuser, "postgres", "drop database", "DROP DATABASE "+dbName+";")
	log.Info("database dropped", "db", dbName)
}

func dropRole(superuser, user string) {
	out, err := common.PsqlSuper(superuser, "postgres", "-tAc",
		"select 1 from pg_roles where rolname = '"+user+"';",
	).Output()
	if err != nil {
		common.Exit("failed to check role existence", "err", err)
	}
	if strings.TrimSpace(string(out)) != "1" {
		log.Info("role does not exist, skipping", "role", user)
		return
	}

	common.RunPsqlSuper(superuser, "postgres", "drop role", "DROP ROLE "+user+";")
	log.Info("role dropped", "role", user)
}
