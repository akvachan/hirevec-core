package tests

import (
	"log/slog"
	"net/http"
)

const url string = "http://localhost:8000/api/v0"

func simpleTests() {
	slog.Info("Running test #1...")
	path := url + "/positions/1"
	resp, err := http.Get(path)
	if err != nil {
		slog.Error("Test #1 failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
}

func main() {
	simpleTests()
}
