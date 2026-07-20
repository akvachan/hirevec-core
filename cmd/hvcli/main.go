// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/akvachan/hirevec-core"
)

const GeneralCommandsInfo = `hvcli is a tool for managing Hirevec server

Usage:

	hvcli <command> [arguments]

The commands are:

	dev               run development-related commands
	help              show help information

`

const DevCommandsInfo = `Usage: 

	hvcli dev <command>

The commands are:

	hashpassword  hash provided password using server-compliant hashing algorithm
	ingest        ingest some test data

`

func IngestTestData(ctx context.Context) error {
	var err error
	var db *sql.DB
	if url := os.Getenv("POSTGRESQL_DATABASE_URL"); url != "" {
		db, err = hirevec.ConnectPostgreSQL(url)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
	} else {
		db, err = hirevec.ConnectSQLite()
		if err != nil {
			return fmt.Errorf("failed to connect to SQLite: %w", err)
		}
	}

	if err := hirevec.ExecMigration(db, hirevec.PathDevIngestMigration); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	return nil
}

func main() {
	_ = hirevec.Loadenv(".env")

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, GeneralCommandsInfo)
		os.Exit(69)
	}

	ctx := context.Background()

	switch os.Args[1] {

	case "dev":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stderr, DevCommandsInfo)
			os.Exit(69)
		}

		switch os.Args[2] {
		case "ingest":
			if err := IngestTestData(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to ingest: %s\n", err)
				os.Exit(69)
			}

		case "hashpassword":
			if len(os.Args) < 4 {
				fmt.Fprint(os.Stderr, "Usage: hvcli dev hashpassword PASSWORD\n")
				os.Exit(69)
			}

			hash, err := hirevec.HashPassword(os.Args[3])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to hash password: %s\n", err)
				os.Exit(69)
			}

			fmt.Println(hash)
		}

	case "help":
		fmt.Println(GeneralCommandsInfo)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %v\n", os.Args[1])
		os.Exit(69)
	}
}
