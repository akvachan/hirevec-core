// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

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
	hirevec.HirevecLogger = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)
	slog.SetDefault(hirevec.HirevecLogger)

	hirevec.LoadDotEnv(".env")
	url := os.Getenv("DATABASE_URL")

	database, err := sql.Open("pgx", url)
	if err != nil {
		slog.Error(fmt.Sprintf("unable to connect to database: %v", err))
		os.Exit(1)
	}
	defer database.Close()

	server := &http.Server{
		Addr:         hirevec.Addr,
		Handler:      hirevec.MainHandler(),
		ReadTimeout:  hirevec.ReadTimeout,
		WriteTimeout: hirevec.WriteTimout,
	}
	defer server.Close()

	hirevec.HirevecDatabase = database
	hirevec.HirevecServer = server

	slog.Info(fmt.Sprintf("server listening on %v", server.Addr))
	err = server.ListenAndServe()
	if err != nil {
		slog.Error(fmt.Sprintf("server crashed: %v", err))
	}
}
