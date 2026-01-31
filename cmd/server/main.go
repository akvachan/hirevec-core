// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	hirevecDB "github.com/akvachan/hirevec-backend/internal/db"
	hirevecServer "github.com/akvachan/hirevec-backend/internal/server"
	hirevecUtils "github.com/akvachan/hirevec-backend/internal/utils"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Set up environment variables
	err := hirevecUtils.LoadDotEnv(".dev.env")
	if err != nil {
		slog.Error(fmt.Sprintf("environment variables could not be loaded: %v", err))
	}
	dsn := os.Getenv("DEV_DB_URL")
	if dsn == "" {
		fmt.Println("DEV_DB_URL is not set")
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
	defer func() {
		if err := database.Close(); err != nil {
			slog.Error(fmt.Sprintf("could not proprely close database connection: %v", err))
			os.Exit(1)
		}
	}()

	// Set up server
	ongoingCtx, stopOngoing := context.WithCancel(context.Background())
	defer stopOngoing()

	server := &http.Server{
		Addr:         "localhost:8080",
		Handler:      hirevecServer.GetRootRouter(),
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ongoingCtx
		},
	}
	hirevecServer.HirevecServer = server

	err = hirevecServer.RunHTTPServer(
		context.Background(),
		server,
		hirevecServer.ShutdownConfig{
			ReadinessDelay: 5 * time.Second,
			GracePeriod:    5 * time.Second,
			ForcePeriod:    2 * time.Second,
		},
	)

	stopOngoing()
	if err != nil {
		slog.Error("server exited with error", "err", err)
	}
}
