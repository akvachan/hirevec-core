// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type GetPositionsSuccessResponse struct {
	Status string     `json:"status"`
	Data   []Position `json:"data"`
}

type GetPositionsFailResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func createPositions(t *testing.T, count int) []Position {
	t.Helper()

	positions := make([]Position, 0, count)
	for i := range count {
		pos := Position{
			Title:       fmt.Sprintf("Title %d", i),
			Description: fmt.Sprintf("Description %d", i),
			Company:     fmt.Sprintf("Company %d", i),
		}
		createPosition(t, pos)
		positions = append(positions, pos)
	}
	return positions
}

func TestGetPositionsHandler_Success_Default(t *testing.T) {
	t.Cleanup(truncateAll)

	expected := createPositions(t, 3)

	resp, err := http.Get(fmt.Sprintf("%s/positions", baseURL))
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

	var apiResp GetPositionsSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Fatalf("expected status 'success', got %q", apiResp.Status)
	}

	if len(apiResp.Data) != len(expected) {
		t.Fatalf("expected %d positions, got %d", len(expected), len(apiResp.Data))
	}
}

func TestGetPositionsHandler_Success_WithLimitOffset(t *testing.T) {
	t.Cleanup(truncateAll)

	createPositions(t, 5)

	resp, err := http.Get(fmt.Sprintf("%s/positions?limit=2&offset=1", baseURL))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var apiResp GetPositionsSuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "success" {
		t.Fatalf("expected status 'success', got %q", apiResp.Status)
	}

	if len(apiResp.Data) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(apiResp.Data))
	}
}

func TestGetPositionsHandler_InvalidLimit(t *testing.T) {
	t.Cleanup(truncateAll)

	resp, err := http.Get(fmt.Sprintf("%s/positions?limit=abc", baseURL))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp GetPositionsFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}

func TestGetPositionsHandler_InvalidOffset(t *testing.T) {
	t.Cleanup(truncateAll)

	resp, err := http.Get(fmt.Sprintf("%s/positions?offset=-1", baseURL))
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var apiResp GetPositionsFailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if apiResp.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", apiResp.Status)
	}
}
