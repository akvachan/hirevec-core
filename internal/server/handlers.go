// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package server implements the HTTP transport layer, providing RESTful endpoints.
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

// successAPIResponse represents a successful JSend-style response.
type successAPIResponse struct {
	Status string `json:"status"` // Always "success"
	Data   any    `json:"data,omitempty"`
}

// errorAPIResponse represents a critical server error response.
type errorAPIResponse struct {
	Status  string `json:"status"` // Always "error"
	Message string `json:"message"`
}

// failAPIResponse represents a client-side validation failure response.
type failAPIResponse struct {
	Status  string `json:"status"` // Always "fail"
	Message any    `json:"message"`
}

// writeSuccessResponse helper writes a JSON success response with the provided HTTP status and data.
func writeSuccessResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		successAPIResponse{
			Status: "success",
			Data:   data,
		},
	)
}

// writeErrorResponse helper writes a JSON error response for server-side issues.
func writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		errorAPIResponse{
			Status:  "error",
			Message: message,
		},
	)
}

// writeFailResponse helper writes a JSON failure response for invalid client input.
func writeFailResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(
		failAPIResponse{
			Status:  "fail",
			Message: message,
		},
	)
}

// decodeJSON reads the request body and decodes it into outData,
// enforcing strict field matching and checking for trailing data.
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

// handleHealth provides a simple liveness check for the server.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeSuccessResponse(w, http.StatusOK, nil)
}

// handleGetPosition retrieves a specific position by the "id" path parameter.
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

// handleGetPositions retrieves a list of positions using limit and offset query parameters.
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

// handleGetCandidate retrieves a specific candidate by the "id" path parameter.
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

// handleGetCandidates retrieves a list of candidates using limit and offset query parameters.
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

// handlePostCandidateReaction processes a candidate's reaction to a position.
func handlePostCandidateReaction(w http.ResponseWriter, r *http.Request) {
	var req models.PostCandidateReactionRequest

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

// handlePostRecruiterReaction processes a recruiter's reaction to a candidate for a specific position.
func handlePostRecruiterReaction(w http.ResponseWriter, r *http.Request) {
	var req models.PostRecruiterReactionRequest

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

// handlePostMatch manually creates a match record between a candidate and a position.
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
