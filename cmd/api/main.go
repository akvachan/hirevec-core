package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/akvachan/hirevec-backend/cmd/common"
)

func main() {
	if err := common.Loadenv(".env"); err != nil {
		slog.Warn("failed to load .env, using system environment", "err", err)
	}

	if len(os.Args) < 2 {
		fmt.Println("usage: go run main.go <path>")
		fmt.Println("example: go run main.go /v1/positions?limit=1")
		os.Exit(1)
	}

	path := os.Args[1]

	protocol := common.Getenv("PROTOCOL", "http")
	host := common.Getenv("HOST", "localhost")
	port := common.Getenv("PORT", "8080")

	url := fmt.Sprintf("%s://%s:%s%s", protocol, host, port, path)

	bearerToken := os.Getenv("ACCESS_TOKEN")
	if bearerToken == "" {
		slog.Error("access token is not set")
		os.Exit(1)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Error("failed to create request", "url", url, "err", err)
		os.Exit(1)
	}

	req.Header.Add("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("failed to get url", "url", url, "err", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var data any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Error("failed to decode json", "err", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(data); err != nil {
		slog.Error("failed to encode json", "err", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code", "status", resp.StatusCode)
	}

	fmt.Println(buf.String())
}
