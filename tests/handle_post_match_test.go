// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type Match struct {
	CandidateID int `json:"candidate_id"`
	PositionID  int `json:"position_id"`
}

type PostMatchRequest struct {
	CandidateID string `json:"candidate_id"`
	PositionID  string `json:"position_id"`
}

type PostMatchSuccessResponse struct {
	Status string `json:"status"`
}

type PostMatchFailResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func TestPostMatchHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	user := User{
		Email:    fmt.Sprintf("alice-%d@example.com", uniq),
		UserName: fmt.Sprintf("alice-%d", uniq),
		FullName: "Alice",
	}
	userID := createUser(t, user)

	candidate := Candidate{
		UserID: userID,
		About:  "Alice is great",
	}
	candidateID := createCandidate(t, candidate)

	position := Position{
		Title:       "Backend Engineer",
		Description: "Go Developer",
		Company:     "Acme Corp",
	}
	positionID := createPosition(t, position)

	reqBody := PostMatchRequest{
		PositionID:  fmt.Sprintf("%d", positionID),
		CandidateID: fmt.Sprintf("%d", candidateID),
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/matches/", baseURL), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var apiResp PostMatchSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Errorf("expected status 'success', got %q", apiResp.Status)
	}
}

func TestPostMatchHandler_InvalidCandidateID(t *testing.T) {
	t.Cleanup(truncateAll)

	positionID := createPosition(t, Position{
		Title:       "Backend Engineer",
		Description: "Go Developer",
		Company:     "Acme Corp",
	})

	reqBody := PostMatchRequest{
		CandidateID: "-1",
		PositionID:  fmt.Sprintf("%d", positionID),
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/matches/", baseURL), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp PostMatchFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}

func TestPostMatchHandler_InvalidPositionID(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	user := User{
		Email:    fmt.Sprintf("alice-%d@example.com", uniq),
		UserName: fmt.Sprintf("alice-%d", uniq),
		FullName: "Alice",
	}
	userID := createUser(t, user)
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Alice is great",
	})

	reqBody := PostMatchRequest{
		CandidateID: fmt.Sprintf("%d", candidateID),
		PositionID:  "-1",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/matches/", baseURL), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp PostMatchFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}
