package hirevec

import (
	"errors"
	"log/slog"
	"net/http"
)

// TODO ./todos/261225-135914-ImplementBasicHandlers.md
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

	query := GetPositionByIDQuery

	rows, err := HirevecDatabase.Query(query, id)
	if err != nil {
		slog.Error("could not perform a query")
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&position); err != nil {
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
