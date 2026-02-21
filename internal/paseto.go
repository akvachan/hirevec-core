// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"strings"
	"sync"
	"time"

	"aidanwoods.dev/go-paseto"
)

var stateStore = &StateStore{
	states: make(map[string]time.Time),
}

type IssuedTokenType string

const (
	refreshToken           IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	accessToken            IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
	RefreshTokenExpiration                 = 30 * 24 * time.Hour
	AccessTokenExpiration                  = 30 * time.Minute
)

type StateStore struct {
	mu     sync.RWMutex
	states map[string]time.Time
}

// PasetoKey defines the public key structure within PublicPasetoKeys.
type PasetoKey struct {
	Version uint8  `json:"version"`
	Kid     uint32 `json:"kid"`
	Key     []byte `json:"key"`
}

// PublicPasetoKeys defines the API response from the endpoint that serves public keys.
type PublicPasetoKeys struct {
	Keys []PasetoKey `json:"keys"`
}

type RefreshTokenClaims struct {
	UserID   string
	Provider string
	JTI      string
}

type AccessTokenClaims struct {
	UserID   string
	Provider string
	Scope    string
}

type AccessToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   uint32 `json:"expires_in"`
	Scope       string `json:"scope"`
	UserID      string `json:"user_id"`
}

type RefreshToken struct {
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    uint32 `json:"expires_in"`
	UserID       string `json:"user_id"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    uint32 `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	UserID       string `json:"user_id"`
}

func (v VaultImpl) ParseAccessToken(tokenString string) (*AccessTokenClaims, error) {
	parsedToken, err := v.AccessTokenParser.ParseV4Public(v.V4AsymetricPublicKey, tokenString, nil)
	if err != nil {
		return nil, ErrInvalidAccessToken
	}

	userID, err := parsedToken.GetSubject()
	if err != nil {
		return nil, ErrInvalidSubject
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, ErrFailedToParseProvider
	}
	if provider != "apple" && provider != "google" {
		return nil, ErrInvalidProvider
	}

	scope, err := parsedToken.GetString("scope")
	if err != nil {
		return nil, ErrFailedToParseScope
	}

	return &AccessTokenClaims{
		UserID:   userID,
		Provider: provider,
		Scope:    scope,
	}, nil
}

func (v VaultImpl) ParseRefreshToken(tokenString string) (*RefreshTokenClaims, error) {
	parsedToken, err := v.RefreshTokenParser.ParseV4Local(v.V4SymmetricKey, tokenString, nil)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	userID, err := parsedToken.GetSubject()
	if err != nil || userID == "" {
		return nil, ErrInvalidSubject
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, ErrFailedToParseProvider
	}
	if provider != "apple" && provider != "google" {
		return nil, ErrInvalidProvider
	}

	tokenType, err := parsedToken.GetString("type")
	if err != nil {
		return nil, ErrFailedToParseTokenType
	}
	if tokenType != "refresh" {
		return nil, ErrInvalidTokenType
	}

	jti, err := parsedToken.GetJti()
	if err != nil || jti == "" {
		return nil, ErrFailedToParseJTI
	}

	return &RefreshTokenClaims{
		UserID:   userID,
		Provider: provider,
		JTI:      jti,
	}, nil
}

func (v VaultImpl) GetPublicKey() []byte {
	return v.V4AsymetricPublicKey.ExportBytes()
}

func (v VaultImpl) CreateAccessToken(userID string, provider string, scope string) (*AccessToken, error) {
	now := time.Now().UTC()

	var expiration time.Duration
	if scope == "onboarding" {
		expiration = 24 * time.Hour
	} else {
		expiration = AccessTokenExpiration
	}

	token := paseto.NewToken()
	token.SetAudience("hirevec-api")
	token.SetIssuer("hirevec")
	token.SetSubject(userID)
	token.SetExpiration(now.Add(expiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("token_type", accessToken); err != nil {
		return nil, ErrFailedToSetTokenType
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, ErrFailedToSetProvider
	}

	token.SetString("scope", scope)

	return &AccessToken{
		AccessToken: token.V4Sign(v.V4AsymmetricSecretKey, nil),
		TokenType:   "Bearer",
		ExpiresIn:   uint32(expiration.Abs().Seconds()),
		Scope:       scope,
		UserID:      userID,
	}, nil
}

func (v VaultImpl) CreateRefreshToken(userID string, provider string, jti string) (*RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience("hirevec-api")
	token.SetIssuer("hirevec")
	token.SetSubject(userID)
	token.SetExpiration(now.Add(RefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(jti)

	if err := token.Set("token_type", refreshToken); err != nil {
		return nil, ErrFailedToSetTokenType
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, ErrFailedToSetProvider
	}

	return &RefreshToken{
		RefreshToken: token.V4Encrypt(v.V4SymmetricKey, nil),
		ExpiresIn:    uint32(RefreshTokenExpiration.Abs().Seconds()),
		UserID:       userID,
	}, nil
}

func (v VaultImpl) CreateTokenPair(userID string, provider string, jti string, scope string) (*TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, scope)
	if err != nil {
		return nil, ErrFailedToCreateAccessToken(err)
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return nil, ErrFailedToCreateRefreshToken(err)
	}

	return &TokenPair{
		AccessToken:  accessToken.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(AccessTokenExpiration.Abs().Seconds()),
		RefreshToken: refreshToken.RefreshToken,
		Scope:        scope,
		UserID:       userID,
	}, nil
}

func (v VaultImpl) GetScopeForRoles(roles []string) (string, error) {
	scopes := make([]string, 0, len(roles))

	for _, r := range roles {
		switch r {
		case "candidate", "recruiter":
			scopes = append(scopes, "role:"+r)
		default:
			return "", ErrInvalidRole
		}
	}

	return strings.Join(scopes, " "), nil
}
