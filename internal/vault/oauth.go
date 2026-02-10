// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package vault deals with authentication and authorization.
package vault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/akvachan/hirevec-backend/internal/store/db/models"
)

type OIDCConfig struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

// CreateStateToken creates and stores a state token
func (v VaultImpl) CreateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := base64.URLEncoding.EncodeToString(b)

	stateStore.mu.Lock()
	stateStore.states[state] = time.Now().Add(10 * time.Minute)
	stateStore.mu.Unlock()

	return state, nil
}

// ValidateAndDeleteStateToken checks if state exists and deletes it (one-time use)
func (v VaultImpl) ValidateAndDeleteStateToken(state string) bool {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	expiry, exists := stateStore.states[state]
	if !exists {
		return false
	}
	delete(stateStore.states, state)

	return !time.Now().After(expiry)
}

func (v VaultImpl) CleanupExpiredStateTokens() {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	now := time.Now()
	for state, expiry := range stateStore.states {
		if now.After(expiry) {
			delete(stateStore.states, state)
		}
	}
}

func (v VaultImpl) CreateAuthCodeURL(state string, verifier string, provider string) (string, error) {
	var config *oauth2.Config

	switch provider {
	case "google":
		config = v.GoogleOIDCConfig.OAuth2Config
	case "apple":
		config = v.AppleOIDCConfig.OAuth2Config
	default:
		return "", ErrInvalidProvider
	}

	url := config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	return url, nil
}

func (v VaultImpl) ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.GoogleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", ErrFailedToExchangeToken(err)
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v VaultImpl) ExchangeAppleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.AppleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", ErrFailedToExchangeToken(err)
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v VaultImpl) VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*models.User, error) {
	idToken, err := v.GoogleOIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, ErrInvalidIDToken
	}

	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, ErrFailedToParseClaims
	}
	if !claims.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	name, err := ValidateName(claims.Name)
	if err != nil {
		return nil, err
	}

	firstName, err := ValidateName(claims.GivenName)
	if err != nil {
		return nil, err
	}

	lastName, err := ValidateName(claims.FamilyName)
	if err != nil {
		return nil, err
	}

	return &models.User{
		Provider:       models.Google,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FirstName:      firstName,
		LastName:       lastName,
		FullName:       name,
	}, nil
}

func (v VaultImpl) VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*models.User, error) {
	idToken, err := v.AppleOIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, ErrInvalidIDToken
	}

	var claims struct {
		Sub            string `json:"sub"`
		Email          string `json:"email"`
		EmailVerified  string `json:"email_verified"`
		IsPrivateEmail string `json:"is_private_email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, ErrFailedToParseClaims
	}

	var firstName, lastName, fullName string
	if userJSON != "" {
		var appleUser struct {
			Name struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"name"`
		}
		if err := json.Unmarshal([]byte(userJSON), &appleUser); err == nil {
			firstName = appleUser.Name.FirstName
			lastName = appleUser.Name.LastName
			fullName = fmt.Sprintf("%s %s", firstName, lastName)
		}
	}

	return &models.User{
		Provider:       models.Apple,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FirstName:      firstName,
		LastName:       lastName,
		FullName:       fullName,
	}, nil
}
