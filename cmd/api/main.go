// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/akvachan/hirevec-core/cmd/common"
)

var log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

func main() {
	if err := common.Loadenv(".env"); err != nil {
		log.Warn("failed to load .env, using system environment", "err", err)
	}

	if len(os.Args) < 2 {
		fmt.Println("usage: go run main.go <path> [method] [json-body]")
		fmt.Println("examples:")
		fmt.Println("  go run main.go v1/positions?limit=1")
		fmt.Println("  go run main.go v1/positions POST '{\"title\":\"Engineer\"}'")
		fmt.Println("  go run main.go v1/positions/123 PATCH '{\"title\":\"Senior Engineer\"}'")
		fmt.Println("  go run main.go v1/positions/123 DELETE")
		os.Exit(1)
	}

	path := os.Args[1]
	path, _ = strings.CutPrefix(path, "/")

	method := "GET"
	if len(os.Args) >= 3 {
		method = strings.ToUpper(os.Args[2])
	}

	validMethods := map[string]bool{"GET": true, "POST": true, "PATCH": true, "DELETE": true, "PUT": true}
	if !validMethods[method] {
		common.Exit("unsupported HTTP method", "method", method)
	}

	var bodyReader *bytes.Reader
	if len(os.Args) >= 4 {
		rawBody := os.Args[3]
		var jsonCheck any
		if err := json.Unmarshal([]byte(rawBody), &jsonCheck); err != nil {
			common.Exit("invalid JSON body", "err", err)
		}
		bodyReader = bytes.NewReader([]byte(rawBody))
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	protocol := common.Getenv("PROTOCOL", "http")
	host := common.Getenv("HOST", "localhost")
	port := common.Getenv("PORT", "8080")
	url := fmt.Sprintf("%s://%s:%s/%s", protocol, host, port, path)

	bearerToken := os.Getenv("ACCESS_TOKEN")
	if bearerToken == "" {
		common.Exit("access token is not set")
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		common.Exit("failed to create request", "url", url, "method", method, "err", err)
	}

	req.Header.Add("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Accept", "application/json")
	if bodyReader.Size() > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		common.Exit("failed to send request", "url", url, "err", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.Exit("failed to read response body", "err", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Error("unexpected status code", "status", resp.StatusCode)
	}

	if len(body) == 0 {
		fmt.Println("(empty response body)")
		return
	}

	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Println(string(body))
		return
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		common.Exit("failed to encode json", "err", err)
	}

	fmt.Println(buf.String())
}
