// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
)

func validateID(id string) error {
	if len(id) > 10 {
		return errors.New("id out of range")
	}
	return nil
}

func GetPositionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := validateID(id); err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid id"})
		return
	}

	query := GetPositionByIDQuery
	var result json.RawMessage
	err := HirevecDatabase.QueryRow(query, id).Scan(&result)
	if len(result) == 0 {
		WriteResponse(w, http.StatusNotFound, APIResponse{Error: "position not found"})
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusInternalServerError, APIResponse{Error: "internal server error"})
		return
	}

	WriteResponse(w, http.StatusOK, APIResponse{Data: result})
}

func GetPositionsHandler(w http.ResponseWriter, r *http.Request) {
	limit := PageSizeDefaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			slog.Error("query failed", "err", err)
			WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid limit"})
			return
		}
		if parsed > PageSizeMaxLimit {
			parsed = PageSizeMaxLimit
		}
		limit = parsed
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err != nil || parsed < 0 {
			slog.Error("query failed", "err", err)
			WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid offset"})
			return
		}
		offset = parsed
	}

	query := GetPositionsQuery
	var result json.RawMessage
	err := HirevecDatabase.QueryRow(query, limit, offset).Scan(&result)
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusInternalServerError, APIResponse{Error: "internal server error"})
		return
	}

	WriteResponse(w, http.StatusOK, APIResponse{Data: result})
}

func GetCandidateHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := validateID(id); err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid id"})
		return
	}

	query := GetCandidateByIDQuery
	var result json.RawMessage
	err := HirevecDatabase.QueryRow(query, id).Scan(&result)
	if len(result) == 0 {
		WriteResponse(w, http.StatusNotFound, APIResponse{Error: "candidate not found"})
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusInternalServerError, APIResponse{Error: "internal server error"})
		return
	}

	WriteResponse(w, http.StatusOK, APIResponse{Data: result})
}

func GetCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	limit := PageSizeDefaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			slog.Error("query failed", "err", err)
			WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid limit"})
			return
		}
		if parsed > PageSizeMaxLimit {
			parsed = PageSizeMaxLimit
		}
		limit = parsed
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err != nil || parsed < 0 {
			slog.Error("query failed", "err", err)
			WriteResponse(w, http.StatusBadRequest, APIResponse{Error: "invalid offset"})
			return
		}
		offset = parsed
	}

	query := GetCandidatesQuery
	var result json.RawMessage
	err := HirevecDatabase.QueryRow(query, limit, offset).Scan(&result)
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteResponse(w, http.StatusInternalServerError, APIResponse{Error: "internal server error"})
		return
	}

	WriteResponse(w, http.StatusOK, APIResponse{Data: result})
}

func GetMatchHandler(w http.ResponseWriter, r *http.Request) {}

func GetLikeHandler(w http.ResponseWriter, r *http.Request) {}

func GetDislikeHandler(w http.ResponseWriter, r *http.Request) {}

func GetSwipeHandler(w http.ResponseWriter, r *http.Request) {}

func CreatePositionHandler(w http.ResponseWriter, r *http.Request) {}

func CreateCandidateHandler(w http.ResponseWriter, r *http.Request) {}

func CreateMatchHandler(w http.ResponseWriter, r *http.Request) {}

func CreateLikeHandler(w http.ResponseWriter, r *http.Request) {}

func CreateDislikeHandler(w http.ResponseWriter, r *http.Request) {}

func CreateSwipeHandler(w http.ResponseWriter, r *http.Request) {}

// MainHandler is a function that assembles all routes and applies middleware.
func MainHandler() http.Handler {
	mainRouter := RegisterRoutes()
	mainHandler := MaxBytesMiddleware(mainRouter)
	mainHandler = LoggingMiddleware()(mainHandler)
	return mainHandler
}
