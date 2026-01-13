// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type Candidate struct {
	UserID int    `json:"user_id"`
	About  string `json:"about"`
}

type User struct {
	Email string `json:"email"`
	UserName string `json:"user_name"`
	FullName string `json:"full_name"`
}

type GetCandidateSuccessResponse struct {
	Status string    `json:"status"`
	Data   Candidate `json:"data"`
}

type GetCandidateFailResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func createUser(t *testing.T, u User) int {
	var id int
	err := testDB.QueryRow(
		`INSERT INTO general.users (email, user_name, full_name) VALUES ($1,$2,$3) RETURNING id`,
		u.Email, u.UserName, u.FullName,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return id
}

func createCandidate(t *testing.T, c Candidate) int {
	var id int
	err := testDB.QueryRow(
		`INSERT INTO general.candidates (user_id, about) VALUES ($1,$2) RETURNING id`,
		c.UserID, c.About,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create candidate: %v", err)
	}
	return id
}

func TestGetCandidateHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	user := User{
		Email: fmt.Sprintf("alice-%d@example.com", uniq),
		UserName: fmt.Sprintf("alice-%d", uniq),
		FullName: "Alice",
	}
	userID := createUser(t, user)

	candidate := Candidate{
		UserID: userID,
		About:  "Backend engineer with Go experience",
	}
	candidateID := createCandidate(t, candidate)

	resp, err := http.Get(fmt.Sprintf("%s/candidates/%d", baseURL, candidateID))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var apiResp GetCandidateSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Errorf("expected status 'success', got %q", apiResp.Status)
	}

	if apiResp.Data.UserID != candidate.UserID {
		t.Errorf("expected user_id %d, got %d", candidate.UserID, apiResp.Data.UserID)
	}

	if apiResp.Data.About != candidate.About {
		t.Errorf("expected about %q, got %q", candidate.About, apiResp.Data.About)
	}
}

func TestGetCandidateHandler_InvalidID(t *testing.T) {
	t.Cleanup(truncateAll)

	resp, err := http.Get(fmt.Sprintf("%s/candidates/%s", baseURL, "abc"))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var apiResp GetCandidateFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}

func TestGetCandidateHandler_NotFound(t *testing.T) {
	t.Cleanup(truncateAll)

	nonExistentID := 999999

	resp, err := http.Get(fmt.Sprintf("%s/candidates/%d", baseURL, nonExistentID))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var apiResp GetCandidateFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}

	if apiResp.Message != "not found" {
		t.Errorf("expected message 'not found', got %q", apiResp.Message)
	}
}
