// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/akvachan/hirevec-backend/internal/db"
	"github.com/akvachan/hirevec-backend/internal/models"
)

type SuccessAPIResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
}

type ErrorAPIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type FailAPIResponse struct {
	Status  string `json:"status"`
	Message any    `json:"message"`
}

func writeSuccessResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		SuccessAPIResponse{
			Status: "success",
			Data:   data,
		},
	)
}

func writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		ErrorAPIResponse{
			Status:  "error",
			Message: message,
		},
	)
}

func writeFailResponse(w http.ResponseWriter, status int, message any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		FailAPIResponse{
			Status:  "fail",
			Message: message,
		},
	)
}

func decodeJSON(r *http.Request, outData any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(outData); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("extra data")
	}
	return nil
}

// GetMainHandler is a function that assembles all routes and applies middleware.
func GetMainHandler() http.Handler {
	mainRouter := registerRoutes()
	mainHandler := getMaxBytesMiddleware(mainRouter)
	mainHandler = getLoggingMiddleware()(mainHandler)
	return mainHandler
}

func handleGetPosition(w http.ResponseWriter, r *http.Request) {
	id, err := validateSerialID(r.PathValue("id"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	var result json.RawMessage
	err = db.SelectPositionByID(&result, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeFailResponse(w, http.StatusNotFound, "position not found")
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusOK, result)
}

func handleGetPositions(w http.ResponseWriter, r *http.Request) {
	limit, err := validateLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	offset, err := validateOffset(r.URL.Query().Get("offset"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	var result json.RawMessage
	err = db.SelectPositions(&result, models.Paginator{Limit: limit, Offset: offset})
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusOK, result)
}

func handleGetCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := validateSerialID(r.PathValue("id"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	var result json.RawMessage
	err = db.SelectCandidateByID(&result, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeFailResponse(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusOK, result)
}

func handleGetCandidates(w http.ResponseWriter, r *http.Request) {
	limit, err := validateLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	offset, err := validateOffset(r.URL.Query().Get("offset"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	var result json.RawMessage
	err = db.SelectCandidates(&result, models.Paginator{Limit: limit, Offset: offset})
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusOK, result)
}

func handlePostCandidateReaction(w http.ResponseWriter, r *http.Request) {
	var req models.PostCandidatesReactionRequest

	if err := decodeJSON(r, &req); err != nil {
		writeFailResponse(w, http.StatusBadRequest, "malformed request")
		return
	}

	cid, err := validateSerialID(r.PathValue("id"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	pid, err := validateSerialID(req.PositionID)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	rtype, err := validateReactionType(req.ReactionType)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	err = db.InsertCandidateReaction(models.CandidateReaction{CandidateID: cid, PositionID: pid, ReactionType: rtype})
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusCreated, nil)
}

func handlePostRecruiterReaction(w http.ResponseWriter, r *http.Request) {
	var req models.PostRecruitersReactionRequest

	if err := decodeJSON(r, &req); err != nil {
		writeFailResponse(w, http.StatusBadRequest, "malformed request")
		return
	}

	rid, err := validateSerialID(r.PathValue("id"))
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	pid, err := validateSerialID(req.PositionID)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	cid, err := validateSerialID(req.CandidateID)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	rtype, err := validateReactionType(req.ReactionType)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	err = db.InsertRecruiterReaction(models.RecruiterReaction{RecruiterID: rid, CandidateID: cid, PositionID: pid, ReactionType: rtype})
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusCreated, nil)
}

func handlePostMatch(w http.ResponseWriter, r *http.Request) {
	var req models.PostMatchRequest

	if err := decodeJSON(r, &req); err != nil {
		writeFailResponse(w, http.StatusBadRequest, "malformed request")
		return
	}

	pid, err := validateSerialID(req.PositionID)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	cid, err := validateSerialID(req.CandidateID)
	if err != nil {
		writeFailResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	err = db.InsertMatch(models.Match{CandidateID: cid, PositionID: pid})
	if err != nil {
		slog.Error("query failed", "err", err)
		writeErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeSuccessResponse(w, http.StatusCreated, nil)
}
