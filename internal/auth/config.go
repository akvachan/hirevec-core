// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package auth deals with authentication and authorization
package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

var (
	GoogleOIDC *OIDCConfig
	AppleOIDC  *OIDCConfig
)

var host string

func InitOAuth(ctx context.Context) error {
	envHost, exists := os.LookupEnv("HOST")
	if !exists {
		return fmt.Errorf("HOST is not set")
	}
	host = strings.TrimSuffix(envHost, "/")

	if err := initOAuthApple(ctx); err != nil {
		return err
	}
	if err := initOAuthGoogle(ctx); err != nil {
		return err
	}

	return nil
}

func initOAuthApple(ctx context.Context) error {
	googleClientID, exists := os.LookupEnv("GOOGLE_CLIENT_ID")
	if !exists {
		return fmt.Errorf("GOOGLE_CLIENT_ID is not set")
	}

	googleClientSecret, exists := os.LookupEnv("GOOGLE_CLIENT_SECRET")
	if !exists {
		return fmt.Errorf("GOOGLE_CLIENT_SECRET is not set")
	}

	googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return fmt.Errorf("failed to create Google OIDC provider: %w", err)
	}

	GoogleOIDC = &OIDCConfig{
		OAuth2Config: &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/callback/google", host),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
			Endpoint:     googleProvider.Endpoint(),
		},
		Verifier: googleProvider.Verifier(&oidc.Config{ClientID: googleClientID}),
	}

	return nil
}

func initOAuthGoogle(ctx context.Context) error {
	appleClientID, exists := os.LookupEnv("APPLE_CLIENT_ID")
	if !exists {
		return fmt.Errorf("APPLE_CLIENT_ID is not set")
	}

	appleClientSecret, exists := os.LookupEnv("APPLE_CLIENT_SECRET")
	if !exists {
		return fmt.Errorf("APPLE_CLIENT_SECRET is not set")
	}

	appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
	if err != nil {
		return fmt.Errorf("failed to create Apple OIDC provider: %w", err)
	}

	AppleOIDC = &OIDCConfig{
		OAuth2Config: &oauth2.Config{
			ClientID:     appleClientID,
			ClientSecret: appleClientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/callback/apple", host),
			Scopes:       []string{oidc.ScopeOpenID, "name", "email"},
			Endpoint:     appleProvider.Endpoint(),
		},
		Verifier: appleProvider.Verifier(&oidc.Config{ClientID: appleClientID}),
	}

	return nil
}
