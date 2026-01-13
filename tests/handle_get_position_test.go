// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type Position struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
}

type GetPositionSuccessResponse struct {
	Status string   `json:"status"`
	Data   Position `json:"data"`
}

type GetPositionFailResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func createPosition(t *testing.T, position Position) int {
	var id int
	err := testDB.QueryRow(
		`
		INSERT INTO general.positions (title, description, company) 
		VALUES ($1,$2,$3) 
		RETURNING id
		`,
		position.Title,
		position.Description,
		position.Company,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create position: %v", err)
	}
	return id
}

func TestGetPositionHandler_Success(t *testing.T) {
	t.Cleanup(truncateAll)

	pos := Position{
		Title:       "Test Title",
		Description: "Test Description",
		Company:     "Test Company",
	}
	posID := createPosition(t, pos)

	resp, err := http.Get(fmt.Sprintf("%s/positions/%d", baseURL, posID))
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

	var apiResp GetPositionSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Errorf("expected status 'success', got %v", apiResp.Status)
	}
	if apiResp.Data.Title != pos.Title {
		t.Errorf("expected title %v, got %v", pos.Title, apiResp.Data.Title)
	}
	if apiResp.Data.Company != pos.Company {
		t.Errorf("expected company %v, got %v", pos.Company, apiResp.Data.Company)
	}
	if apiResp.Data.Description != pos.Description {
		t.Errorf("expected description %q, got %q", pos.Description, apiResp.Data.Description)
	}
}

func TestGetPositionHandler_InvalidID(t *testing.T) {
	t.Cleanup(truncateAll)

	resp, err := http.Get(fmt.Sprintf("%s/positions/%s", baseURL, "abc"))
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

	var apiResp GetPositionFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}

	if apiResp.Message != "invalid id" {
		t.Errorf("expected message 'invalid id', got %q", apiResp.Message)
	}
}

func TestGetPositionHandler_NotFound(t *testing.T) {
	t.Cleanup(truncateAll)

	nonExistentID := 999999

	resp, err := http.Get(fmt.Sprintf("%s/positions/%d", baseURL, nonExistentID))
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

	var apiResp GetPositionFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}

	if apiResp.Message != "position not found" {
		t.Errorf("expected message 'position not found', got %q", apiResp.Message)
	}
}
