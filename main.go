// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	hirevec "github.com/akvachan/hirevec-backend/src"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Set up environment variables
	hirevec.LoadDotEnv(".env")

	// Set up logger
	hirevec.HirevecLogger = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)
	slog.SetDefault(hirevec.HirevecLogger)

	// Set up database
	database, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error(fmt.Sprintf("unable to connect to database: %v", err))
		os.Exit(1)
	}
	hirevec.HirevecDatabase = database
	defer database.Close()

	// Set up server
	server := &http.Server{
		Addr:         hirevec.Addr,
		Handler:      hirevec.MainHandler(),
		ReadTimeout:  hirevec.ReadTimeout,
		WriteTimeout: hirevec.WriteTimout,
	}
	hirevec.HirevecServer = server
	defer server.Close()

	// Start server
	slog.Info(fmt.Sprintf("server listening on %v", server.Addr))
	_ = server.ListenAndServe()
}
