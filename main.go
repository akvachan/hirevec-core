// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"

	hirevec "github.com/akvachan/hirevec-backend/src"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	path := path.Join(".env")
	hirevec.LoadDotEnv(path)
	url := os.Getenv("DATABASE_URL")

	database, err := sql.Open("pgx", url)
	if err != nil {
		slog.Error(fmt.Sprintf("unable to connect to database: %v", err))
		os.Exit(1)
	}
	defer database.Close()

	router := http.NewServeMux()
	router.HandleFunc("GET /api/v0/positions/{id}", hirevec.GetPosition)
	router.HandleFunc("GET /api/v0/positions/", hirevec.GetPositions)
	router.HandleFunc("GET /api/v0/candidates/{id}", hirevec.GetCandidate)
	router.HandleFunc("GET /api/v0/matches/{id}", hirevec.GetMatch)
	router.HandleFunc("GET /api/v0/likes/{id}", hirevec.GetLike)
	router.HandleFunc("GET /api/v0/dislikes/{id}", hirevec.GetDislike)
	router.HandleFunc("GET /api/v0/swipes/{id}", hirevec.GetSwipe)
	router.HandleFunc("POST /api/v0/positions/", hirevec.CreatePosition)
	router.HandleFunc("POST /api/v0/candidates/", hirevec.CreateCandidate)
	router.HandleFunc("POST /api/v0/matches/", hirevec.CreateMatch)
	router.HandleFunc("POST /api/v0/likes/", hirevec.CreateLike)
	router.HandleFunc("POST /api/v0/dislikes/", hirevec.CreateDislike)
	router.HandleFunc("POST /api/v0/swipes/", hirevec.CreateSwipe)
	handler := http.MaxBytesHandler(router, hirevec.MaxBytesHandler)

	server := &http.Server{
		Addr:         hirevec.Addr,
		Handler:      handler,
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
