package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akvachan/hirevec-core"
)

const DefaultBaseURL = "http://localhost:8888"

type CLIConfig struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".hvcli.json")
}

func SaveToken(access, refresh string) error {
	cfg := CLIConfig{AccessToken: access, RefreshToken: refresh}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetConfigPath(), data, 0o600)
}

func PrintJSON(data any) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

func ExecuteWithRefresh(
	ctx context.Context,
	client hirevec.Client,
	fn func() (*hirevec.JSONAPIDocument, error),
) (*hirevec.JSONAPIDocument, error) {
	doc, err := fn()

	isUnauthorized := false
	if err != nil && strings.Contains(err.Error(), "401") {
		isUnauthorized = true
	} else if doc != nil {
		for _, e := range doc.Errors {
			if e.Status == "401" || e.Status == "403" && strings.Contains(e.Detail, "token") {
				isUnauthorized = true
				break
			}
		}
	}

	if isUnauthorized {
		if client.RefreshToken == "" {
			fmt.Println("Session expired (no refresh token found). Please run 'hv-cli quick-start' to re-login.")
			os.Exit(1)
		}

		fmt.Println("Access token expired. Refreshing session...")

		newToken, refreshErr := client.GetAccessToken(ctx)
		if refreshErr != nil {
			fmt.Println("Refresh token has expired or is invalid. Please run 'hv-cli quick-start' to re-login.")
			os.Exit(1)
		}

		client.AccessToken = newToken.AccessToken
		saveErr := SaveToken(client.AccessToken, client.RefreshToken)
		if saveErr != nil {
			fmt.Printf("Failed to save tokens: %v\n", saveErr)
			os.Exit(1)
		}

		return fn()
	}

	return doc, err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: hv-cli <command> [args]")
		fmt.Println("Commands:")
		fmt.Println("  quick-start          - Register a test user, create profiles, and ingest test positions")
		fmt.Println("  recommendations      - List your recommendations")
		fmt.Println("  positive <id>        - React positively to a recommendation")
		fmt.Println("  negative <id>        - React negatively to a recommendation")
		fmt.Println("  matches              - List your matches")
		os.Exit(1)
	}

	ctx := context.Background()

	client := hirevec.NewClient(DefaultBaseURL)
	data, err := os.ReadFile(GetConfigPath())
	if err == nil {
		var cfg CLIConfig
		if err := json.Unmarshal(data, &cfg); err == nil {
			client.AccessToken = cfg.AccessToken
			client.RefreshToken = cfg.RefreshToken
		}
	}

	command := os.Args[1]

	switch command {
	case "quick-start":
		fmt.Println("Starting quick-start process...")

		email := "testuser@example.com"
		password := "SuperSecret123!"
		fullName := "Test User"

		fmt.Println("Registering test user...")
		tokenPair, err := client.Register(ctx, email, fullName, password)
		if err != nil {
			fmt.Printf("Failed to register (might already exist): %v\n", err)
			tokenPair, err = client.Login(ctx, email, password)
			if err != nil {
				fmt.Printf("Failed to login: %v\n", err)
				os.Exit(1)
			}
		}

		client.AccessToken = tokenPair.AccessToken
		client.RefreshToken = tokenPair.RefreshToken
		err = SaveToken(client.AccessToken, client.RefreshToken)
		if err != nil {
			fmt.Printf("Failed to save tokens: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Creating candidate profile...")
		cToken, err := client.CreateMeCandidateProfile(ctx, "I am a passionate software engineer looking for Go backend roles.")
		if err != nil {
			fmt.Printf("Failed to create candidate profile: %v\n", err)
			os.Exit(1)
		}

		client.AccessToken = cToken.AccessToken
		_ = SaveToken(client.AccessToken, client.RefreshToken)

		fmt.Println("Creating recruiter profile...")
		rToken, err := client.CreateMeRecruiterProfile(ctx)
		if err != nil {
			fmt.Printf("Failed to create recruiter profile: %v\n", err)
			os.Exit(1)
		}

		client.AccessToken = rToken.AccessToken
		_ = SaveToken(client.AccessToken, client.RefreshToken)

		fmt.Println("Ingesting positions...")
		_, err = client.CreateMePosition(ctx, "Senior Go Engineer", "Looking for a Go expert to build distributed systems.", "TechCorp")
		if err != nil {
			fmt.Printf("Failed to create position 1: %v\n", err)
			os.Exit(1)
		}

		_, err = client.CreateMePosition(ctx, "Backend Developer", "Maintain and scale our core API using Go and PostgreSQL.", "Startup Inc")
		if err != nil {
			fmt.Printf("Failed to create position 2: %v\n", err)
			os.Exit(1)
		}

		_, err = client.CreateMePosition(ctx, "Staff Software Engineer", "Lead the backend architecture transition.", "BigData Co")
		if err != nil {
			fmt.Printf("Failed to create position 3: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\nQuick start complete! You are logged in as", email)

	case "recommendations":
		doc, err := ExecuteWithRefresh(ctx, client, func() (*hirevec.JSONAPIDocument, error) {
			return client.GetMeRecommendations(ctx, "done", "done", 10, true)
		})
		if err != nil {
			fmt.Printf("Failed to fetch recommendations: %v\n", err)
			os.Exit(1)
		}

		PrintJSON(doc.Data)

	case "positive":
		if len(os.Args) < 3 {
			fmt.Println("Missing recommendation ID. Usage: hv-cli positive <id>")
			os.Exit(1)
		}
		recID := os.Args[2]

		_, err := ExecuteWithRefresh(ctx, client, func() (*hirevec.JSONAPIDocument, error) {
			return client.CreateMeReaction(ctx, recID, hirevec.ReactionTypePositive)
		})
		if err != nil {
			fmt.Printf("Failed to record positive reaction: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Positively reacted to recommendation: %s\n", recID)

	case "negative":
		if len(os.Args) < 3 {
			fmt.Println("Missing recommendation ID. Usage: hv-cli negative <id>")
			os.Exit(1)
		}
		recID := os.Args[2]

		_, err := ExecuteWithRefresh(ctx, client, func() (*hirevec.JSONAPIDocument, error) {
			return client.CreateMeReaction(ctx, recID, hirevec.ReactionTypeNegative)
		})
		if err != nil {
			fmt.Printf("Failed to record negative reaction: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Negatively reacted to recommendation: %s\n", recID)

	case "matches":
		doc, err := ExecuteWithRefresh(ctx, client, func() (*hirevec.JSONAPIDocument, error) {
			return client.GetMeMatches(ctx, "", 10)
		})
		if err != nil {
			fmt.Printf("Failed to fetch matches: %v\n", err)
			os.Exit(1)
		}

		PrintJSON(doc.Data)

	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
