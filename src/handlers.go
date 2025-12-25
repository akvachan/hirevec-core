package hirevec 

import (
	"errors"
	"log/slog"
	"net/http"

	"hirevec/database"
)

var hirevecDatabase = database.HirevecDatabase

func validateID(id string) error {
	if id == "" {
		return errors.New("id cannot be empty")
	}
	return nil
}

func GetPosition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := validateID(id); err != nil {
		slog.Error("could not extract id from the path")
		return
	}

	query := database.GetPositionByID
	rows, err := hirevecDatabase.Query(query, id)
	if err != nil {
		slog.Error("could not perform a query")
	}
	defer rows.Close()

	for rows.Next() {
		var positionID int
		var title string
		var description string
		var company string

		if err := rows.Scan(&positionID, &title, &description, &company); err != nil {
			slog.Error("could not extract all needed columns")
		}
	}

	if !rows.NextResultSet() {
		slog.Error("expected more result sets")
	}
}

func GetCandidate(w http.ResponseWriter, r *http.Request) {}

func GetMatch(w http.ResponseWriter, r *http.Request) {}

func GetLike(w http.ResponseWriter, r *http.Request) {}

func GetDislike(w http.ResponseWriter, r *http.Request) {}

func GetSwipe(w http.ResponseWriter, r *http.Request) {}

func CreatePosition(w http.ResponseWriter, r *http.Request) {}

func CreateCandidate(w http.ResponseWriter, r *http.Request) {}

func CreateMatch(w http.ResponseWriter, r *http.Request) {}

func CreateLike(w http.ResponseWriter, r *http.Request) {}

func CreateDislike(w http.ResponseWriter, r *http.Request) {}

func CreateSwipe(w http.ResponseWriter, r *http.Request) {}
