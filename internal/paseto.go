// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"slices"
	"strings"
	"sync"
	"time"

	"aidanwoods.dev/go-paseto"
)

type (
	ScopeType []ScopeValueType

	ScopeValueType string

	ClaimType string

	IssuedTokenType string

	StateStore struct {
		mu     sync.RWMutex
		states map[string]time.Time
	}

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

var stateStore = &StateStore{
	states: make(map[string]time.Time),
}

const (
	IssuedTokenTypeRefreshToken   IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	IssuedTokenTypeAccessToken    IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
	DefaultRefreshTokenExpiration                 = 30 * 24 * time.Hour
	DefaultAccessTokenExpiration                  = 30 * time.Minute
	ScopeValueTypeCandidate       ScopeValueType  = "role:candidate"
	ScopeValueTypeRecruiter       ScopeValueType  = "role:recruiter"
	ScopeValueTypeAdmin           ScopeValueType  = "role:admin"
	ScopeValueTypeOnboarding      ScopeValueType  = "role:onboarding"
	TokenAudience                                 = "api.hirevec.com"
	TokenIssuer                                   = "api.hirevec.com"
)

func ToScopeValue(str string) (ScopeValueType, error) {
	switch str {
	case "role:candidate":
		return ScopeValueTypeCandidate, nil
	case "role:recruiter":
		return ScopeValueTypeRecruiter, nil
	case "role:admin":
		return ScopeValueTypeAdmin, nil
	case "role:onboarding":
		return ScopeValueTypeOnboarding, nil
	default:
		return "", ErrInvalidScopeValueType
	}
}

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

func (v PasetoVault) ParseAccessToken(tokenString string) (*AccessTokenClaims, error) {
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

func (v PasetoVault) ParseRefreshToken(tokenString string) (*RefreshTokenClaims, error) {
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

func (v PasetoVault) GetPublicKey() []byte {
	return v.V4AsymetricPublicKey.ExportBytes()
}

func (v PasetoVault) CreateAccessToken(userID string, provider string, scope ScopeType) (*AccessToken, error) {
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

func (v PasetoVault) CreateRefreshToken(userID string, provider string, jti string) (*RefreshToken, error) {
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

func (v PasetoVault) CreateTokenPair(userID string, provider string, jti string, scope ScopeType) (*TokenPair, error) {
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

func (v PasetoVault) GetScopeForRoles(roles []string) (ScopeType, error) {
	scope := make([]ScopeValueType, 0, len(roles))

	for _, r := range roles {
		switch r {
		case "candidate", "recruiter", "admin":
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
