// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"time"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

type GetCandidatesSuccessResponse struct {
	Status string      `json:"status"`
	Data   []Candidate `json:"data"`
}

type GetCandidatesFailResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func TestGetCandidatesHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	uniq := time.Now().UnixNano()
	user1 := User{
		Email: fmt.Sprintf("alice-%d@example.com", uniq),
		UserName: fmt.Sprintf("alice-%d", uniq),
		FullName: "Alice",
	}
	userID1 := createUser(t, user1)
	candidate1 := Candidate{
		UserID: userID1,
		About:  "Alice is great",
	}
	_ = createCandidate(t, candidate1)

	user2 := User{
		Email: fmt.Sprintf("bob-%d@example.com", uniq),
		UserName: fmt.Sprintf("bob-%d", uniq),
		FullName: "Bob",
	}
	userID2 := createUser(t, user2)
	candidate2 := Candidate{
		UserID: userID2,
		About:  "Bob is awesome",
	}
	_ = createCandidate(t, candidate2)


	params := url.Values{}
	params.Set("limit", "10")
	params.Set("offset", "0")
	resp, err := http.Get(fmt.Sprintf("%s/candidates?%s", baseURL, params.Encode()))
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

	var apiResp GetCandidatesSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Errorf("expected status 'success', got %v", apiResp.Status)
	}

	if len(apiResp.Data) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(apiResp.Data))
	}

	if apiResp.Data[0].UserID != userID1 || apiResp.Data[0].About != "Alice is great" {
		t.Errorf("unexpected first candidate: %+v", apiResp.Data[0])
	}

	if apiResp.Data[1].UserID != userID2 || apiResp.Data[1].About != "Bob is awesome" {
		t.Errorf("unexpected second candidate: %+v", apiResp.Data[1])
	}
}

func TestGetCandidatesHandler_InvalidLimit(t *testing.T) {
	t.Cleanup(truncateAll)

	params := url.Values{}
	params.Set("limit", "abc")
	params.Set("offset", "0")
	resp, err := http.Get(fmt.Sprintf("%s/candidates?%s", baseURL, params.Encode()))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp GetCandidatesFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}

func TestGetCandidatesHandler_InvalidOffset(t *testing.T) {
	t.Cleanup(truncateAll)

	params := url.Values{}
	params.Set("limit", "10")
	params.Set("offset", "-5")
	resp, err := http.Get(fmt.Sprintf("%s/candidates?%s", baseURL, params.Encode()))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp GetCandidatesFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}

func TestGetCandidatesHandler_EmptyList(t *testing.T) {
	t.Cleanup(truncateAll)

	params := url.Values{}
	params.Set("limit", "10")
	params.Set("offset", "0")
	resp, err := http.Get(fmt.Sprintf("%s/candidates?%s", baseURL, params.Encode()))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var apiResp GetCandidatesSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(apiResp.Data) != 0 {
		t.Errorf("expected empty candidate list, got %d", len(apiResp.Data))
	}
}
