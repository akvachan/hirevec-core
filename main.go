// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	hirevec "github.com/akvachan/hirevec-backend/src"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Set up environment variables
	hirevec.LoadDotEnv(".dev.env")

	// Set up logger
	hirevec.HirevecLogger = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)
	slog.SetDefault(hirevec.HirevecLogger)

	// Set up database
	dsn := os.Getenv("DEV_DATABASE_URL")
	if dsn == "" {
		fmt.Println("DEV_DATABASE_URL is not set")
		os.Exit(1)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error(fmt.Sprintf("unable to connect to database: %v", err))
		os.Exit(1)
	}
	hirevec.HirevecDatabase = database
	defer database.Close()

	addr := "localhost:8080"

	// Set up server
	server := &http.Server{
		Addr:         addr,
		Handler:      hirevec.GetMainHandler(),
		ReadTimeout:  hirevec.ReadTimeout,
		WriteTimeout: hirevec.WriteTimout,
	}
	hirevec.HirevecServer = server
	defer server.Close()

	slog.Info(fmt.Sprintf("server listening on %v", server.Addr))
	go func() {
		_ = server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
