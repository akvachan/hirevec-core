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

type CandidateReaction struct {
	CandidateID  int `json:"candidate_id"`
	PositionID   int `json:"position_id"`
	ReactionType string `json:"reaction_type"`
}

type PostCandidatesReactionRequest struct {
	PositionID   string `json:"position_id"`
	ReactionType string `json:"reaction_type"`
}

func TestPostCandidateReactionHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()

	userID := createUser(t, User{
		Email:    fmt.Sprintf("alice-%d@example.com", uniq),
		UserName: fmt.Sprintf("alice-%d", uniq),
		FullName: "Alice",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Alice candidate",
	})

	positionID := createPosition(t, Position{
		Title: "Test Position", Description: "Desc", Company: "Acme",
	})

	reqBody := PostCandidatesReactionRequest{
		PositionID:   fmt.Sprintf("%d", positionID),
		ReactionType: "like",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/candidates/%d/reactions", baseURL, candidateID), "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var rtype string
	err = testDB.QueryRow(
		`SELECT reaction_type FROM general.candidates_reactions WHERE candidate_id=$1 AND position_id=$2`,
		candidateID, positionID,
	).Scan(&rtype)
	if err != nil {
		t.Fatalf("failed to query reaction: %v", err)
	}
	if rtype != "like" {
		t.Errorf("expected reaction type 'like', got %q", rtype)
	}
}

func TestPostCandidateReactionHandler_InvalidCandidateID(t *testing.T) {
	t.Cleanup(truncateAll)

	reqBody := PostCandidatesReactionRequest{
		PositionID:   "1",
		ReactionType: "like",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/candidates/%s/reactions", baseURL, "abc"), "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostCandidateReactionHandler_InvalidPositionID(t *testing.T) {
	t.Cleanup(truncateAll)

	userID := createUser(t, User{
		Email:    fmt.Sprintf("bob-%d@example.com", time.Now().UnixNano()),
		UserName: fmt.Sprintf("bob-%d", time.Now().UnixNano()),
		FullName: "Bob",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Bob candidate",
	})

	reqBody := PostCandidatesReactionRequest{
		PositionID:   "abc",
		ReactionType: "like",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/candidates/%d/reactions", baseURL, candidateID), "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostCandidateReactionHandler_InvalidReactionType(t *testing.T) {
	t.Cleanup(truncateAll)

	userID := createUser(t, User{
		Email:    fmt.Sprintf("carol-%d@example.com", time.Now().UnixNano()),
		UserName: fmt.Sprintf("carol-%d", time.Now().UnixNano()),
		FullName: "Carol",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Carol candidate",
	})

	positionID := createPosition(t, Position{
		Title: "Test Position", Description: "Desc", Company: "Acme",
	})

	reqBody := PostCandidatesReactionRequest{
		PositionID:   fmt.Sprintf("%d", positionID),
		ReactionType: "super-like",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post(fmt.Sprintf("%s/candidates/%d/reactions", baseURL, candidateID), "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostCandidateReactionHandler_MalformedJSON(t *testing.T) {
	t.Cleanup(truncateAll)

	userID := createUser(t, User{
		Email:    fmt.Sprintf("dave-%d@example.com", time.Now().UnixNano()),
		UserName: fmt.Sprintf("dave-%d", time.Now().UnixNano()),
		FullName: "Dave",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Dave candidate",
	})

	bodyBytes := []byte(`{ "position_id": 1, "reaction_type": `) 

	resp, err := http.Post(fmt.Sprintf("%s/candidates/%d/reactions", baseURL, candidateID), "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}
