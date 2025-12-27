// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

type GetPositionsSuccessResponse struct {
	Data []struct {
		PositionID  int    `json:"position_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Company     string `json:"company"`
	} `json:"data"`
}

const url string = "http://localhost:8888/api/v0"

// TODO ./todos/261225-155400-ImplementBasicTests.md
func TestGetPositionSuccess(t *testing.T) {
	path := url + "/positions/1"
	resp, err := http.Get(path)
	if err != nil {
		t.Errorf("could not retrieve position: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status code mismatch: %v != 200", resp.StatusCode)
		return
	}

	var response GetPositionsSuccessResponse
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&response); err != nil || dec.More() {
		t.Errorf("malformed response: %v", err)
		return
	}

	expectedDataLen := 1
	if len(response.Data) != expectedDataLen {
		t.Errorf("requested %v positions but got: %v", expectedDataLen, response.Data)
	}
}
