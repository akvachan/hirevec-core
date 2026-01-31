// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package auth deals with authentication and authorization
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"aidanwoods.dev/go-paseto"
)

var (
	secretKey              paseto.V4SymmetricKey
	publicKey              paseto.V4AsymmetricPublicKey
	privateKey             paseto.V4AsymmetricSecretKey
	accessTokenParser      paseto.Parser
	refreshTokenParser     paseto.Parser
	AccessTokenExpiration  = 30 * time.Minute    // 30 minutes
	RefreshTokenExpiration = 30 * 24 * time.Hour // 30 days
)

var stateStore = &StateStore{
	states: make(map[string]time.Time),
}

type IssuedTokenType string

const (
	refreshToken IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	accessToken  IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
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
	UserID   uint32
	Provider string
	JTI      string
}

type AccessTokenClaims struct {
	UserID   uint32
	Provider string
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    uint32 `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func InitPaseto() {
	// For private APIs
	hirevecSecretKey, exists := os.LookupEnv("HIREVEC_SECRET_KEY")
	if !exists {
		panic("HIREVEC_SECRET_KEY is not set")
	}

	key, err := paseto.V4SymmetricKeyFromHex(hirevecSecretKey)
	if err != nil {
		panic("could not instantiate a secret key")
	}
	secretKey = key

	// For public APIs
	hirevecPrivateKey, exists := os.LookupEnv("HIREVEC_PRIVATE_KEY")
	if !exists {
		panic("HIREVEC_PRIVATE_KEY is not set")
	}

	privKey, err := paseto.NewV4AsymmetricSecretKeyFromHex(hirevecPrivateKey)
	if err != nil {
		panic("could not instantiate a private key")
	}
	privateKey = privKey
	publicKey = privKey.Public()

	accessTokenParser = paseto.NewParser()
	accessTokenParser.AddRule(paseto.ForAudience("hirevec-api"))
	accessTokenParser.AddRule(paseto.IssuedBy("hirevec"))
	accessTokenParser.AddRule(paseto.NotExpired())
	accessTokenParser.AddRule(paseto.NotBeforeNbf())

	refreshTokenParser = paseto.NewParser()
	refreshTokenParser.AddRule(paseto.ForAudience("hirevec-api"))
	refreshTokenParser.AddRule(paseto.IssuedBy("hirevec"))
	refreshTokenParser.AddRule(paseto.NotExpired())
	refreshTokenParser.AddRule(paseto.NotBeforeNbf())
}

func ParseAccessToken(tokenString string) (*AccessTokenClaims, error) {
	parsedToken, err := accessTokenParser.ParseV4Public(publicKey, tokenString, nil)
	if err != nil {
		return nil, errors.New("invalid access token")
	}

	userID, err := parsedToken.GetSubject()
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	id, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		return nil, errors.New("could not parse user ID")
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, errors.New("could not parse provider")
	}
	if provider != "apple" && provider != "google" {
		return nil, errors.New("invalid provider")
	}

	return &AccessTokenClaims{
		UserID:   uint32(id),
		Provider: provider,
	}, nil
}

func ParseRefreshToken(tokenString string) (*RefreshTokenClaims, error) {
	parsedToken, err := refreshTokenParser.ParseV4Local(secretKey, tokenString, nil)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	userID, err := parsedToken.GetSubject()
	if err != nil || userID == "" {
		return nil, errors.New("invalid subject")
	}
	id, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		return nil, errors.New("could not parse user ID")
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, errors.New("could not parse provider")
	}
	if provider != "apple" && provider != "google" {
		return nil, errors.New("invalid provider")
	}

	tokenType, err := parsedToken.GetString("type")
	if err != nil {
		return nil, errors.New("could not parse type")
	}
	if tokenType != "refresh" {
		return nil, errors.New("invalid token type")
	}

	jti, err := parsedToken.GetJti()
	if err != nil {
		return nil, errors.New("could not parse jti")
	}
	if jti == "" {
		return nil, errors.New("invalid refresh token")
	}

	return &RefreshTokenClaims{
		UserID:   uint32(id),
		Provider: provider,
		JTI:      jti,
	}, nil
}

func GetPublicKey() []byte {
	return publicKey.ExportBytes()
}

func CreateAccessToken(userID uint32, provider string, scope string) (string, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience("hirevec-api")
	token.SetIssuer("hirevec")
	token.SetSubject(fmt.Sprintf("%d", userID))
	token.SetExpiration(now.Add(AccessTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("token_type", accessToken); err != nil {
		return "", errors.New("could not set token type")
	}

	if err := token.Set("provider", provider); err != nil {
		return "", errors.New("could not set provider")
	}

	token.SetString("scope", scope)

	return token.V4Sign(privateKey, nil), nil
}

func CreateRefreshToken(
	userID uint32,
	provider string,
	jti string,
) (string, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience("hirevec-api")
	token.SetIssuer("hirevec")
	token.SetSubject(fmt.Sprintf("%d", userID))
	token.SetExpiration(now.Add(RefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(jti)

	if err := token.Set("token_type", refreshToken); err != nil {
		return "", errors.New("could not set token type")
	}

	if err := token.Set("provider", provider); err != nil {
		return "", errors.New("could not set provider")
	}

	return token.V4Encrypt(secretKey, nil), nil
}

func CreateTokenPair(
	userID uint32,
	provider string,
	jti string,
	scope string,
) (TokenPair, error) {
	accessToken, err := CreateAccessToken(userID, provider, scope)
	if err != nil {
		return TokenPair{}, errors.New("could not create an access token")
	}

	refreshToken, err := CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return TokenPair{}, errors.New("could not create a refresh token")
	}

	return TokenPair{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(AccessTokenExpiration.Abs().Seconds()),
		RefreshToken: refreshToken,
		Scope:        scope,
	}, nil
}

// GenerateStateToken creates and stores a state token
func GenerateStateToken() (string, error) {
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

// ValidateAndDeleteState checks if state exists and deletes it (one-time use)
func ValidateAndDeleteState(state string) bool {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	expiry, exists := stateStore.states[state]
	if !exists {
		return false
	}
	delete(stateStore.states, state)

	return !time.Now().After(expiry)
}

func CleanupExpiredStates() {
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()

	now := time.Now()
	for state, expiry := range stateStore.states {
		if now.After(expiry) {
			delete(stateStore.states, state)
		}
	}
}

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpiredStates()
		}
	}()
}
