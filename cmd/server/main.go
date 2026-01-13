// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	hirevecDB "github.com/akvachan/hirevec-backend/internal/db"
	hirevecServer "github.com/akvachan/hirevec-backend/internal/server"
	hirevecUtils "github.com/akvachan/hirevec-backend/internal/utils"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	ReadTimeout = 2 * time.Second
	WriteTimout = 2 * time.Second
)

func main() {
	// Set up environment variables
	hirevecUtils.LoadDotEnv(".dev.env")
	dsn := os.Getenv("DEV_DATABASE_URL")
	if dsn == "" {
		fmt.Println("DEV_DATABASE_URL is not set")
		os.Exit(1)
	}

	// Set up logger
	hirevecServer.HirevecLogger = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)
	slog.SetDefault(hirevecServer.HirevecLogger)

	// Set up database
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error(fmt.Sprintf("unable to connect to database: %v", err))
		os.Exit(1)
	}
	hirevecDB.HirevecDatabase = database
	defer database.Close()

	// Set up server
	addr := "localhost:8080"
	server := &http.Server{
		Addr:         addr,
		Handler:      hirevecServer.GetMainHandler(),
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimout,
	}
	hirevecServer.HirevecServer = server
	slog.Info(fmt.Sprintf("server listening on %v", server.Addr))
	_ = server.ListenAndServe()
}
