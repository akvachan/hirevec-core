package hirevecbackend

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/akvachan/hirevec-backend/src"
	_ "github.com/jackc/pgx/v5"
)

func main() {
	hirevecDatabase, _ := sql.Open("postgres", "user=myname dbname=dbname sslmode=disable")
	defer hirevecDatabase.Close()
	chan

	mux := http.NewServeMux()
	mux.HandleFunc("GET api/v0/positions/{id}", api.GetPosition)
	mux.HandleFunc("GET api/v0/candidates/{id}", api.GetCandidate)
	mux.HandleFunc("GET api/v0/matches/{id}", api.GetMatch)
	mux.HandleFunc("GET api/v0/likes/{id}", api.GetLike)
	mux.HandleFunc("GET api/v0/dislikes/{id}", api.GetDislike)
	mux.HandleFunc("GET api/v0/swipes/{id}", api.GetSwipe)
	mux.HandleFunc("POST api/v0/positions/", api.CreatePosition)
	mux.HandleFunc("POST api/v0/candidates/", api.CreateCandidate)
	mux.HandleFunc("POST api/v0/matches/", api.CreateMatch)
	mux.HandleFunc("POST api/v0/likes/", api.CreateLike)
	mux.HandleFunc("POST api/v0/dislikes/", api.CreateDislike)
	mux.HandleFunc("POST api/v0/swipes/", api.CreateSwipe)

	handler := http.MaxBytesHandler(mux, 1*constants.Megabyte)

	server := &http.Server{
		Addr:         "localhost:8000",
		Handler:      handler,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}

	server.ListenAndServe()
}
