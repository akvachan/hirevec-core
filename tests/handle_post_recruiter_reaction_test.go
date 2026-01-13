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

type Recruiter struct {
	UserID int `json:"user_id"`
}

type PostRecruitersReactionRequest struct {
	CandidateID  string `json:"candidate_id"`
	PositionID   string `json:"position_id"`
	ReactionType string `json:"reaction_type"`
}

func createRecruiter(t *testing.T, r Recruiter) int {
	var id int
	err := testDB.QueryRow(
		`INSERT INTO general.recruiters (user_id) VALUES ($1) RETURNING id`,
		r.UserID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create recruiter: %v", err)
	}
	return id
}

func doJSONPost(t *testing.T, url string, body any) *http.Response {
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	return resp
}

func TestPostRecruiterReactionHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()

	userID := createUser(t, User{
		Email:    fmt.Sprintf("candidate-%d@example.com", uniq),
		UserName: fmt.Sprintf("candidate-%d", uniq),
		FullName: "Candidate Bob",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Candidate Bob is awesome",
	})

	userID2 := createUser(t, User{
		Email:    fmt.Sprintf("recruiter-%d@example.com", uniq),
		UserName: fmt.Sprintf("recruiter-%d", uniq),
		FullName: "Recruiter Alice",
	})
	recruiterID := createRecruiter(t, Recruiter{
		UserID: userID2,
	})

	positionID := createPosition(t, Position{
		Title:       "Backend Engineer",
		Description: "Go & PostgreSQL",
		Company:     "Acme Corp",
	})

	reqBody := PostRecruitersReactionRequest{
		CandidateID:  fmt.Sprintf("%d", candidateID),
		PositionID:   fmt.Sprintf("%d", positionID),
		ReactionType: "like",
	}

	resp := doJSONPost(t, fmt.Sprintf("%s/recruiters/%d/reactions", baseURL, recruiterID), reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var apiResp struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Errorf("expected status 'success', got %q", apiResp.Status)
	}
}

func TestPostRecruiterReactionHandler_InvalidRecruiterID(t *testing.T) {
	t.Cleanup(truncateAll)

	reqBody := PostRecruitersReactionRequest{
		CandidateID:  "1",
		PositionID:   "1",
		ReactionType: "like",
	}

	resp := doJSONPost(t, fmt.Sprintf("%s/recruiters/%s/reactions", baseURL, "abc"), reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostRecruiterReactionHandler_InvalidCandidateID(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	userID := createUser(t, User{
		Email:    fmt.Sprintf("recruiter-%d@example.com", uniq),
		UserName: fmt.Sprintf("recruiter-%d", uniq),
		FullName: "Recruiter Alice",
	})
	recruiterID := createRecruiter(t, Recruiter{UserID: userID})

	reqBody := PostRecruitersReactionRequest{
		CandidateID:  "abc",
		PositionID:   "1",
		ReactionType: "like",
	}

	resp := doJSONPost(t, fmt.Sprintf("%s/recruiters/%d/reactions", baseURL, recruiterID), reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostRecruiterReactionHandler_InvalidPositionID(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	userID := createUser(t, User{
		Email:    fmt.Sprintf("candidate-%d@example.com", uniq),
		UserName: fmt.Sprintf("candidate-%d", uniq),
		FullName: "Candidate Bob",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Candidate Bob is awesome",
	})

	userID2 := createUser(t, User{
		Email:    fmt.Sprintf("recruiter-%d@example.com", uniq),
		UserName: fmt.Sprintf("recruiter-%d", uniq),
		FullName: "Recruiter Alice",
	})
	recruiterID := createRecruiter(t, Recruiter{UserID: userID2})

	reqBody := PostRecruitersReactionRequest{
		CandidateID:  fmt.Sprintf("%d", candidateID),
		PositionID:   "abc",
		ReactionType: "like",
	}

	resp := doJSONPost(t, fmt.Sprintf("%s/recruiters/%d/reactions", baseURL, recruiterID), reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostRecruiterReactionHandler_InvalidReactionType(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	userID := createUser(t, User{
		Email:    fmt.Sprintf("candidate-%d@example.com", uniq),
		UserName: fmt.Sprintf("candidate-%d", uniq),
		FullName: "Candidate Bob",
	})
	candidateID := createCandidate(t, Candidate{
		UserID: userID,
		About:  "Candidate Bob is awesome",
	})

	userID2 := createUser(t, User{
		Email:    fmt.Sprintf("recruiter-%d@example.com", uniq),
		UserName: fmt.Sprintf("recruiter-%d", uniq),
		FullName: "Recruiter Alice",
	})
	recruiterID := createRecruiter(t, Recruiter{UserID: userID2})

	positionID := createPosition(t, Position{
		Title:       "Backend Engineer",
		Description: "Go & PostgreSQL",
		Company:     "Acme Corp",
	})

	reqBody := PostRecruitersReactionRequest{
		CandidateID:  fmt.Sprintf("%d", candidateID),
		PositionID:   fmt.Sprintf("%d", positionID),
		ReactionType: "maybe",
	}

	resp := doJSONPost(t, fmt.Sprintf("%s/recruiters/%d/reactions", baseURL, recruiterID), reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestPostRecruiterReactionHandler_MalformedJSON(t *testing.T) {
	t.Cleanup(truncateAll)

	userID2 := createUser(t, User{
		Email:    fmt.Sprintf("recruiter-%d@example.com", time.Now().UnixNano()),
		UserName: fmt.Sprintf("recruiter-%d", time.Now().UnixNano()),
		FullName: "Recruiter Alice",
	})
	recruiterID := createRecruiter(t, Recruiter{UserID: userID2})

	resp, err := http.Post(
		fmt.Sprintf("%s/recruiters/%d/reactions", baseURL, recruiterID),
		"application/json",
		bytes.NewReader([]byte(`{bad json}`)),
	)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

