// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	DefaultRefreshTokenExpiration = 30 * 24 * time.Hour
	DefaultAccessTokenExpiration  = 30 * time.Minute
)

const (
	TokenAudience = "api.hirevec.com"
	TokenIssuer   = "api.hirevec.com"
)

var (
	ErrFailedCreateAccessToken        = errors.New("failed to create access token")
	ErrFailedCreateAppleOIDCProvider  = errors.New("failed to create Apple OIDC provider")
	ErrFailedCreateGoogleOIDCProvider = errors.New("failed to create Google OIDC provider")
	ErrFailedCreateRefreshToken       = errors.New("failed to create refresh token")
	ErrFailedExchangeToken            = errors.New("failed to exchange tokens")
	ErrFailedLoadAsymmetricKey        = errors.New("failed to load asymmetric key")
	ErrFailedLoadSymmetricKey         = errors.New("failed to load symmetric key")
	ErrFailedParseClaims              = errors.New("failed to parse claims")
	ErrFailedParseJTI                 = errors.New("failed to parse jti")
	ErrFailedParseProvider            = errors.New("failed to parse provider")
	ErrFailedParseScope               = errors.New("failed to parse scope")
	ErrFailedParseTokenType           = errors.New("failed to parse token type")
	ErrFailedSetProvider              = errors.New("failed to set provider")
	ErrFailedSetTokenType             = errors.New("failed to set token type")
	ErrIDTokenRequired                = errors.New("id_token required")
	ErrInvalidAccessToken             = errors.New("invalid access token")
	ErrInvalidIDToken                 = errors.New("invalid id_token")
	ErrInvalidProvider                = errors.New("invalid provider")
	ErrInvalidRefreshToken            = errors.New("invalid refresh token")
	ErrInvalidRole                    = errors.New("invalid role")
	ErrInvalidSubject                 = errors.New("invalid subject")
	ErrInvalidTokenType               = errors.New("invalid token type")
	ErrInvalidScopeValueType          = errors.New("invalid scope value type provided")
)

type Provider string

const (
	ProviderApple  Provider = "apple"
	ProviderGoogle Provider = "google"
)

func (p Provider) Raw() string {
	switch p {
	case ProviderApple:
		return "apple"
	case ProviderGoogle:
		return "google"
	default:
		return ""
	}
}

type ScopeType []ScopeValueType

func (s ScopeType) Raw() string {
	var result []string
	for _, role := range s {
		result = append(result, string(role))
	}
	return strings.Join(result, " ")
}

func NewScope(scope string) (ScopeType, error) {
	var result ScopeType
	for _, role := range strings.Fields(scope) {
		scopeValue, err := ToScopeValue(role)
		if err != nil {
			return result, ErrInvalidScopeValueType
		}
		result = append(result, scopeValue)
	}
	return result, nil
}

type ScopeValueType string

const (
	ScopeValueTypeCandidate  ScopeValueType = "role:candidate"
	ScopeValueTypeRecruiter  ScopeValueType = "role:recruiter"
	ScopeValueTypeOnboarding ScopeValueType = "role:onboarding"
)

func ToScopeValue(str string) (ScopeValueType, error) {
	switch str {
	case "role:candidate":
		return ScopeValueTypeCandidate, nil
	case "role:recruiter":
		return ScopeValueTypeRecruiter, nil
	case "role:onboarding":
		return ScopeValueTypeOnboarding, nil
	default:
		return "", ErrInvalidScopeValueType
	}
}

type IssuedTokenType string

const (
	IssuedTokenTypeRefreshToken IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	IssuedTokenTypeAccessToken  IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

type (
	VaultInterface interface {
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

	VaultConfig struct {
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

	VaultImpl struct {
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

	OIDCConfig struct {
		OAuth2Config *oauth2.Config
		Verifier     *oidc.IDTokenVerifier
	}
)

func NewVault(ctx context.Context, cfg VaultConfig) (*VaultImpl, error) {
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

	return &VaultImpl{
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

type StateStore struct {
	mu     sync.RWMutex
	states map[string]time.Time
}

var stateStore = &StateStore{
	states: make(map[string]time.Time),
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
		return "", ErrFailedExchangeToken
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
		return "", ErrFailedExchangeToken
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v VaultImpl) VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*User, error) {
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

func (v VaultImpl) VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*User, error) {
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

type (
	PasetoKey struct {
		Version uint8  `json:"version"`
		Kid     uint32 `json:"kid"`
		Key     []byte `json:"key"`
	}

	PublicPasetoKeys struct {
		Keys []PasetoKey `json:"keys"`
	}

	RefreshTokenClaims struct {
		UserID   string
		Provider string
		JTI      string
	}

	AccessTokenClaims struct {
		UserID   string
		Provider string
		Scope    ScopeType
	}

	AccessToken struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   uint32 `json:"expires_in"`
		Scope       string `json:"scope"`
		UserID      string `json:"user_id"`
	}

	RefreshToken struct {
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    uint32 `json:"expires_in"`
		UserID       string `json:"user_id"`
	}

	TokenPair struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    uint32 `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		UserID       string `json:"user_id"`
	}
)

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
		return nil, ErrFailedParseProvider
	}
	if provider != "apple" && provider != "google" {
		return nil, ErrInvalidProvider
	}

	scope, err := parsedToken.GetString("scope")
	if err != nil {
		return nil, ErrFailedParseScope
	}

	validScope, err := NewScope(scope)
	if err != nil {
		return nil, ErrInvalidScopeValueType
	}

	return &AccessTokenClaims{
		UserID:   userID,
		Provider: provider,
		Scope:    validScope,
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
		return nil, ErrFailedParseProvider
	}
	if provider != "apple" && provider != "google" {
		return nil, ErrInvalidProvider
	}

	tokenType, err := parsedToken.GetString("type")
	if err != nil {
		return nil, ErrFailedParseTokenType
	}
	if tokenType != "refresh" {
		return nil, ErrInvalidTokenType
	}

	jti, err := parsedToken.GetJti()
	if err != nil || jti == "" {
		return nil, ErrFailedParseJTI
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

func (v VaultImpl) CreateAccessToken(userID string, provider string, scope ScopeType) (*AccessToken, error) {
	now := time.Now().UTC()

	var expiration time.Duration
	switch {
	case slices.Contains(scope, ScopeValueTypeOnboarding):
		expiration = 24 * time.Hour
	case v.AccessTokenExpiration != 0:
		expiration = v.AccessTokenExpiration
	default:
		expiration = DefaultAccessTokenExpiration
	}

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(userID)
	token.SetExpiration(now.Add(expiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("token_type", IssuedTokenTypeAccessToken); err != nil {
		return nil, ErrFailedSetTokenType
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, ErrFailedSetProvider
	}

	rawScope := scope.Raw()
	token.SetString("scope", rawScope)

	return &AccessToken{
		AccessToken: token.V4Sign(v.V4AsymmetricSecretKey, nil),
		TokenType:   "Bearer",
		ExpiresIn:   uint32(expiration.Abs().Seconds()),
		Scope:       rawScope,
		UserID:      userID,
	}, nil
}

func (v VaultImpl) CreateRefreshToken(userID string, provider string, jti string) (*RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(userID)
	token.SetExpiration(now.Add(DefaultRefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(jti)

	if err := token.Set("token_type", IssuedTokenTypeRefreshToken); err != nil {
		return nil, ErrFailedSetTokenType
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, ErrFailedSetProvider
	}

	var expiresIn uint32
	if v.RefreshTokenExpiration != 0 {
		expiresIn = uint32(v.RefreshTokenExpiration.Abs().Seconds())
	} else {
		expiresIn = uint32(DefaultRefreshTokenExpiration.Abs().Seconds())
	}

	return &RefreshToken{
		RefreshToken: token.V4Encrypt(v.V4SymmetricKey, nil),
		ExpiresIn:    expiresIn,
		UserID:       userID,
	}, nil
}

func (v VaultImpl) CreateTokenPair(userID string, provider string, jti string, scope ScopeType) (*TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, scope)
	if err != nil {
		return nil, ErrFailedCreateAccessToken
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return nil, ErrFailedCreateRefreshToken
	}

	return &TokenPair{
		AccessToken:  accessToken.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(DefaultAccessTokenExpiration.Abs().Seconds()),
		RefreshToken: refreshToken.RefreshToken,
		Scope:        scope.Raw(),
		UserID:       userID,
	}, nil
}

func (v VaultImpl) GetScopeForRoles(roles []string) (ScopeType, error) {
	scope := make([]ScopeValueType, 0, len(roles))

	for _, r := range roles {
		switch r {
		case "candidate", "recruiter":
			scopeValue, err := ToScopeValue("role:" + r)
			if err != nil {
				return scope, ErrInvalidScopeValueType
			}
			scope = append(scope, scopeValue)
		default:
			return scope, ErrInvalidRole
		}
	}

	return scope, nil
}
