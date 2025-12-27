// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

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

// TODO ./todos/261225-135914-ImplementBasicHandlers.md

func GetPosition(w http.ResponseWriter, r *http.Request) {
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

func GetPositions(w http.ResponseWriter, r *http.Request) {
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
