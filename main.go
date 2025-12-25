package main

import (
	"database/sql"
	"net/http"
	"time"

	hirevec "github.com/akvachan/hirevec-backend/src"
	_ 			"github.com/jackc/pgx/v5"
)

func main() {
	hirevecDatabase, _ := sql.Open("postgres", "user=myname dbname=dbname sslmode=disable")
	hirevec.HirevecDatabase = hirevecDatabase
	defer hirevecDatabase.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET api/v0/positions/{id}", hirevec.GetPosition)
	mux.HandleFunc("GET api/v0/candidates/{id}", hirevec.GetCandidate)
	mux.HandleFunc("GET api/v0/matches/{id}", hirevec.GetMatch)
	mux.HandleFunc("GET api/v0/likes/{id}", hirevec.GetLike)
	mux.HandleFunc("GET api/v0/dislikes/{id}", hirevec.GetDislike)
	mux.HandleFunc("GET api/v0/swipes/{id}", hirevec.GetSwipe)
	mux.HandleFunc("POST api/v0/positions/", hirevec.CreatePosition)
	mux.HandleFunc("POST api/v0/candidates/", hirevec.CreateCandidate)
	mux.HandleFunc("POST api/v0/matches/", hirevec.CreateMatch)
	mux.HandleFunc("POST api/v0/likes/", hirevec.CreateLike)
	mux.HandleFunc("POST api/v0/dislikes/", hirevec.CreateDislike)
	mux.HandleFunc("POST api/v0/swipes/", hirevec.CreateSwipe)

	handler := http.MaxBytesHandler(mux, 1*hirevec.Megabyte)

	server := &http.Server{
		Addr:         "localhost:8000",
		Handler:      handler,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}

	server.ListenAndServe()
}
