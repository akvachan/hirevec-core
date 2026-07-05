// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/akvachan/hirevec-core"
)

const GeneralCommandsInfo = `hvcli is a tool for managing Hirevec account

Usage:

	hvcli <command> [arguments]

The commands are:

	dev               run development-related commands
	help              show help information
	login             login into your account
	matches           show matches
	negative          react negatively to a recommendation
	positive          react positively to a recommendation
	recommendations   show recommendations
	register          register a new account
`

const DevCommandsInfo = `Usage: 

	hvcli dev <command>

The commands are:

	hashpassword  hash provided password using server-compliant hashing algorithm
	quickstart    ingest some test data and setup test user auto-login
`

const DefaultBaseURL = "http://localhost:8888"

type CLIConfig struct {
	BaseURL      string `json:"base_url"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return path.Join(home, ".hvcli.json")
}

func LoadOrCreateConfig() CLIConfig {
	var cfg CLIConfig
	data, err := os.ReadFile(GetConfigPath())
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return cfg
}

func SaveToken(access, refresh string) error {
	cfg := LoadOrCreateConfig()
	cfg.AccessToken = access
	cfg.RefreshToken = refresh

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(GetConfigPath(), data, 0o600)
}

func Prompt(label string) string {
	fmt.Printf("%s: ", label)
	var s string
	fmt.Scanln(&s)

	return strings.TrimSpace(s)
}

func LoginAndSaveToken(ctx context.Context, client hirevec.Client, email string, password string) error {
	var tokens hirevec.TokenPair
	var err error
	if email == "" || password == "" {
		tokens, err = client.Login(ctx, Prompt("Email"), Prompt("Password"))
		if err != nil {
			return err
		}
	} else {
		tokens, err = client.Login(ctx, email, password)
		if err != nil {
			return err
		}
	}

	if err = SaveToken(tokens.AccessToken, tokens.RefreshToken); err != nil {
		return err
	}

	return nil
}

func RegisterAndSaveToken(ctx context.Context, client hirevec.Client) error {
	tokens, err := client.Register(ctx, Prompt("Email"), Prompt("Full Name"), Prompt("Password"))
	if err != nil {
		return err
	}

	if err = SaveToken(tokens.AccessToken, tokens.RefreshToken); err != nil {
		return err
	}

	return nil
}

func QuickStartAndLogin(ctx context.Context, client hirevec.Client, prompt bool) error {
	var err error
	var db *sql.DB
	if url := os.Getenv("POSTGRESQL_DATABASE_URL"); url != "" {
		db, err = hirevec.ConnectPostgreSQL(url)
		if err != nil {
			return err
		}
	} else {
		db, err = hirevec.ConnectSQLite()
		if err != nil {
			return err
		}
	}

	if err := hirevec.ExecMigration(db, hirevec.PathQuickStartMigration); err != nil {
		return err
	}

	if prompt {
		if err := LoginAndSaveToken(ctx, client, "", ""); err != nil {
			return err
		}
	} else {
		if err := LoginAndSaveToken(ctx, client, "alex.chen.demo@example.com", "test"); err != nil {
			return err
		}
	}

	return nil
}

type ClientRequestType string

const (
	ClientRequestTypeGetMeRecommendations ClientRequestType = "GetMeRecommendations"
	ClientRequestTypeCreateMeReaction     ClientRequestType = "CreateMeReaction"
	ClientRequestTypeGetMeMatches         ClientRequestType = "GetMeMatches"
)

type OutputFormat string

const (
	OutputFormatJSON OutputFormat = "json"
)

func PrintClientResponse[T any](response T, requestType ClientRequestType, format OutputFormat) {
	switch format {
	case OutputFormatJSON:
		jsonData, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling response to JSON: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))

	default:
		fmt.Fprintf(os.Stderr, "Unsupported output format: %s\n", format)
	}
}

func main() {
	_ = hirevec.Loadenv(".env")

	if len(os.Args) < 2 {
		fmt.Println(GeneralCommandsInfo)
		os.Exit(69)
	}

	cfg := LoadOrCreateConfig()
	client := hirevec.NewClient(cfg.BaseURL)
	client.AccessToken = cfg.AccessToken
	client.RefreshToken = cfg.RefreshToken
	ctx := context.Background()

	switch os.Args[1] {
	case "login":
		if err := LoginAndSaveToken(ctx, client, "", ""); err != nil {
			fmt.Printf("Failed to login: %v\n", err)
			os.Exit(69)
		}

	case "register":
		if err := RegisterAndSaveToken(ctx, client); err != nil {
			fmt.Printf("Failed to register: %v\n", err)
			os.Exit(69)
		}

	case "dev":
		if len(os.Args) < 3 {
			fmt.Println(DevCommandsInfo)
			os.Exit(69)
		}

		switch os.Args[2] {
		case "quickstart":
			// TODO: Support login prompt on quickstart
			// https://github.com/akvachan/hirevec-core/issues/38
			if err := QuickStartAndLogin(ctx, client, false); err != nil {
				fmt.Printf("Failed to quick start: %v\n", err)
				os.Exit(69)
			}

		case "hashpassword":
			if len(os.Args) < 4 {
				fmt.Println("Usage: hvcli dev hashpassword PASSWORD")
				os.Exit(69)
			}

			hash, err := hirevec.HashPassword(os.Args[3])
			if err != nil {
				fmt.Printf("Failed to hash password: %v\n", err)
				os.Exit(69)
			}
			fmt.Println(hash)
		}

	// TODO: Support cursors and page limit
	// https://github.com/akvachan/hirevec-core/issues/39
	case "recommendations":
		// TODO: Implement access token refresh if it is expired
		// https://github.com/akvachan/hirevec-core/issues/41
		response, err := client.GetMeRecommendations(ctx, "", "", hirevec.DefaultPageSize, true)
		if err != nil {
			fmt.Printf("Failed to fetch recommendations: %v\n", err)
			os.Exit(69)
		}

		PrintClientResponse(response, ClientRequestTypeGetMeRecommendations, OutputFormatJSON)

	case "positive":
		if len(os.Args) < 3 {
			fmt.Println("Usage: hvcli positive RECOMMENDATION_ID")
			os.Exit(69)
		}

		response, err := client.CreateMeReaction(ctx, os.Args[2], hirevec.ReactionTypePositive)
		if err != nil {
			fmt.Printf("Failed to create reaction: %v\n", err)
			os.Exit(69)
		}

		PrintClientResponse(response, ClientRequestTypeCreateMeReaction, OutputFormatJSON)

	case "negative":
		if len(os.Args) < 3 {
			fmt.Println("Usage: hvcli negative RECOMMENDATION_ID")
			os.Exit(69)
		}

		response, err := client.CreateMeReaction(ctx, os.Args[2], hirevec.ReactionTypeNegative)
		if err != nil {
			fmt.Printf("Failed to create reaction: %v\n", err)
			os.Exit(69)
		}

		PrintClientResponse(response, ClientRequestTypeCreateMeReaction, OutputFormatJSON)

	// TODO: Support cursors and page limit
	// https://github.com/akvachan/hirevec-core/issues/40
	case "matches":
		response, err := client.GetMeMatches(ctx, "", 0)
		if err != nil {
			fmt.Printf("Failed to fetch matches: %v\n", err)
			os.Exit(69)
		}

		PrintClientResponse(response, ClientRequestTypeGetMeMatches, OutputFormatJSON)

	case "help":
		fmt.Println(GeneralCommandsInfo)

	default:
		fmt.Printf("Unknown command: %v\n", os.Args[1])
		os.Exit(69)
	}
}
