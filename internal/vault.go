// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Vault interface {
	CleanupExpiredStateTokens()
	CreateAccessToken(userID string, provider string, scope ScopeType) (*AccessToken, error)
	CreateAuthCodeURL(state string, verifier string, provider string) (string, error)
	CreateRefreshToken(userID string, provider string, jti string) (*RefreshToken, error)
	CreateStateToken() (string, error)
	CreateTokenPair(userID string, provider string, jti string, scope ScopeType) (*TokenPair, error)
	ExchangeAppleCodeForIDToken(ctx context.Context, code string, verifier *http.Cookie) (string, error)
	ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifier *http.Cookie) (string, error)
	GetPublicKey() []byte
	GetScopeForRoles(roles []string) (ScopeType, error)
	ParseAccessToken(token string) (*AccessTokenClaims, error)
	ParseRefreshToken(token string) (*RefreshTokenClaims, error)
	ValidateAndDeleteStateToken(state string) bool
	VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*User, error)
	VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*User, error)
}

type VaultConfig struct {
	Host                   string
	Port                   string
	SymmetricKeyHex        string
	AsymmetricKeyHex       string
	GoogleClientID         string
	GoogleClientSecret     string
	AppleClientID          string
	AppleClientSecret      string
	RefreshTokenExpiration time.Duration
	AccessTokenExpiration  time.Duration
}

type PasetoVault struct {
	AccessTokenParser      paseto.Parser
	RefreshTokenParser     paseto.Parser
	V4AsymetricPublicKey   paseto.V4AsymmetricPublicKey
	V4AsymmetricSecretKey  paseto.V4AsymmetricSecretKey
	V4SymmetricKey         paseto.V4SymmetricKey
	GoogleOIDCConfig       OIDCConfig
	AppleOIDCConfig        OIDCConfig
	RefreshTokenExpiration time.Duration
	AccessTokenExpiration  time.Duration
}

type OIDCConfig struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

func NewPasetoVault(ctx context.Context, cfg VaultConfig) (*PasetoVault, error) {
	accessTokenParser := paseto.NewParser()
	accessTokenParser.AddRule(paseto.ForAudience(TokenAudience))
	accessTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))
	accessTokenParser.AddRule(paseto.NotExpired())
	accessTokenParser.AddRule(paseto.NotBeforeNbf())

	refreshTokenParser := paseto.NewParser()
	refreshTokenParser.AddRule(paseto.ForAudience(TokenAudience))
	refreshTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))
	refreshTokenParser.AddRule(paseto.NotExpired())
	refreshTokenParser.AddRule(paseto.NotBeforeNbf())

	symmetricKey, err := paseto.V4SymmetricKeyFromHex(cfg.SymmetricKeyHex)
	if err != nil {
		slog.Error(
			"Failed to load a symmetric key",
			"err", err,
		)
		return nil, ErrFailedLoadSymmetricKey
	}

	asymmetricKey, err := paseto.NewV4AsymmetricSecretKeyFromHex(cfg.AsymmetricKeyHex)
	if err != nil {
		slog.Error(
			"Failed to load an asymmetric key",
			"err", err,
			"key", cfg.AsymmetricKeyHex,
		)
		return nil, ErrFailedLoadAsymmetricKey
	}

	googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, ErrFailedCreateGoogleOIDCProvider
	}

	appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
	if err != nil {
		return nil, ErrFailedCreateAppleOIDCProvider
	}

	return &PasetoVault{
		AccessTokenParser:     accessTokenParser,
		RefreshTokenParser:    refreshTokenParser,
		V4AsymmetricSecretKey: asymmetricKey,
		V4AsymetricPublicKey:  asymmetricKey.Public(),
		V4SymmetricKey:        symmetricKey,
		GoogleOIDCConfig: OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				RedirectURL:  fmt.Sprintf("%s:%s/oauth2/callback/google", cfg.Host, cfg.Port),
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
				Endpoint:     googleProvider.Endpoint(),
			},
			Verifier: googleProvider.Verifier(&oidc.Config{ClientID: cfg.GoogleClientID}),
		},
		AppleOIDCConfig: OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     cfg.AppleClientID,
				ClientSecret: cfg.AppleClientSecret,
				RedirectURL:  fmt.Sprintf("%s/oauth2/callback/apple", cfg.Host),
				Scopes:       []string{oidc.ScopeOpenID, "name", "email"},
				Endpoint:     appleProvider.Endpoint(),
			},
			Verifier: appleProvider.Verifier(&oidc.Config{ClientID: cfg.AppleClientID}),
		},
		RefreshTokenExpiration: cfg.RefreshTokenExpiration,
		AccessTokenExpiration:  cfg.AccessTokenExpiration,
	}, nil
}

// CreateStateToken creates and stores a state token
func (v PasetoVault) CreateStateToken() (string, error) {
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
func (v PasetoVault) ValidateAndDeleteStateToken(state string) bool {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	expiry, exists := stateStore.states[state]
	if !exists {
		return false
	}
	delete(stateStore.states, state)

	return !time.Now().After(expiry)
}

func (v PasetoVault) CleanupExpiredStateTokens() {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	now := time.Now()
	for state, expiry := range stateStore.states {
		if now.After(expiry) {
			delete(stateStore.states, state)
		}
	}
}

func (v PasetoVault) CreateAuthCodeURL(state string, verifier string, provider string) (string, error) {
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

func (v PasetoVault) ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.GoogleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", ErrFailedExchangeToken
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v PasetoVault) ExchangeAppleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.AppleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", ErrFailedExchangeToken
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v PasetoVault) VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*User, error) {
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
		return nil, ErrFailedParseClaims
	}
	if !claims.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	name, err := ValidateName(claims.Name)
	if err != nil {
		return nil, err
	}

	return &User{
		Provider:       ProviderGoogle,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FullName:       name,
	}, nil
}

func (v PasetoVault) VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*User, error) {
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
		return nil, ErrFailedParseClaims
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

	return &User{
		Provider:       ProviderApple,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FullName:       fullName,
	}, nil
}
