// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	hirevecDB "github.com/akvachan/hirevec-backend/internal/db"
	hirevecServer "github.com/akvachan/hirevec-backend/internal/server"
	hirevecUtils "github.com/akvachan/hirevec-backend/internal/utils"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	testDB  *sql.DB
	baseURL string
)

func TestMain(m *testing.M) {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	err = hirevecUtils.LoadDotEnv(filepath.Join(root, "..", ".dev.env"))
	if err != nil {
		panic(err)
	}

	hirevecServer.HirevecLogger = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelError,
		}),
	)
	slog.SetDefault(hirevecServer.HirevecLogger)

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		slog.Error("TEST_DATABASE_URL is not set")
		os.Exit(1)
	}

	testDB, err = sql.Open("pgx", dsn)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to connect to test DB: %v", err))
		os.Exit(1)
	}
	if err := testDB.Ping(); err != nil {
		slog.Error(fmt.Sprintf("failed to ping test DB: %v", err))
		os.Exit(1)
	}

	hirevecDB.HirevecDatabase = testDB

	ts := httptest.NewServer(hirevecServer.GetMainHandler())
	defer ts.Close()
	baseURL = ts.URL + "/api/v0"

	code := m.Run()
	os.Exit(code)
}

func truncateAll() {
	tables := []string{
		"matches",
		"recruiters_reactions",
		"candidates_reactions",
		"positions",
		"recruiters",
		"candidates",
		"users",
	}
	for _, tbl := range tables {
		if _, err := testDB.Exec(fmt.Sprintf("TRUNCATE TABLE general.%s CASCADE", tbl)); err != nil {
			slog.Error(fmt.Sprintf("failed to clean up DB: %v", err))
			os.Exit(1)
		}
	}
}
