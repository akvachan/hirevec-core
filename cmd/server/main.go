// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

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
	hirevecUtils.LoadDotEnv(".dev.env")
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
	defer database.Close()

	// Set up server
	ongoingCtx, stopOngoing := context.WithCancel(context.Background())
	defer stopOngoing()

	server := &http.Server{
		Addr:         "localhost:8080",
		Handler:      hirevecServer.GetMainHandler(),
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
		slog.Error("Server exited with error", "err", err)
	}
}
