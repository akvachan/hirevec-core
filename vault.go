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
	"os"
	"strings"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	DefaultRefreshTokenExpiration = 30 * 24 * time.Hour
	DefaultAccessTokenExpiration  = 30 * time.Minute
	DefaultStateTokenExpiration   = 10 * time.Minute
	DefaultVerifierExpiration     = 10 * time.Minute
	DefaultProvider               = ProviderGoogle
)

const (
	TokenAudience      = "api.hirevec.com"
	TokenIssuer        = "api.hirevec.com"
	StateTokenAudience = "oauth-state"
)

const (
	skFile = ".sk"
	akFile = ".ak"
)

var (
	ErrFailedSaveSymmetricKey         = errors.New("failed to create or load symmetric key")
	ErrFailedSaveAsymmetricKey        = errors.New("failed to create or load asymmetric key")
	ErrFailedSetScope                 = errors.New("failed to set scope")
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
	ErrFailedParseCSRF                = errors.New("failed to parse CSRF")
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
	ErrInvalidScopeValue              = errors.New("invalid scope value provided")
	ErrInvalidStateToken              = errors.New("invalid state token")
)

type Provider string

const (
	ProviderApple  Provider = "apple"
	ProviderGoogle Provider = "google"
)

func ToProvider(str string, def Provider) (Provider, error) {
	switch str {
	case "apple":
		return ProviderApple, nil
	case "google":
		return ProviderGoogle, nil
	case "":
		return def, nil
	default:
		return "", ErrInvalidProvider
	}
}

func (p Provider) Raw() string {
	return string(p)
}

type Role string

const (
	RoleCandidate  Role = "candidate"
	RoleRecruiter  Role = "recruiter"
	RoleOnboarding Role = "onboarding"
)

type Scope []ScopeValue

func (s Scope) Raw() string {
	var result []string
	for _, role := range s {
		result = append(result, string(role))
	}
	return strings.Join(result, " ")
}

type ScopeValue string

const (
	ScopeValueCandidate  ScopeValue = "role:candidate"
	ScopeValueRecruiter  ScopeValue = "role:recruiter"
	ScopeValueOnboarding ScopeValue = "role:onboarding"
)

func ToScopeValue(str string) (ScopeValue, error) {
	switch str {
	case "role:candidate":
		return ScopeValueCandidate, nil
	case "role:recruiter":
		return ScopeValueRecruiter, nil
	case "role:onboarding":
		return ScopeValueOnboarding, nil
	default:
		return "", ErrInvalidScopeValue
	}
}

type IssuedTokenType string

const (
	IssuedTokenTypeRefreshToken IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	IssuedTokenTypeAccessToken  IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

type VaultInterface interface {
	CreateAccessToken(userID ULID, provider Provider, roles map[Role]ULID) (*AccessToken, error)
	CreateAuthCodeURL(state string, verifier string, provider Provider) (string, error)
	CreateRefreshToken(userID ULID, provider Provider, jti ULID) (*RefreshToken, error)
	CreateTokenPair(userID ULID, provider Provider, jti ULID, roles map[Role]ULID) (*TokenPair, error)
	CreateStateToken(provider Provider) (string, error)
	ExchangeAppleCodeForIDToken(ctx context.Context, code string, verifier *http.Cookie) (string, error)
	ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifier *http.Cookie) (string, error)
	ParseAccessToken(token string) (*AccessTokenClaims, error)
	ParseRefreshToken(token string) (*RefreshTokenClaims, error)
	ParseStateToken(token string) (*StateTokenClaims, error)
	VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*User, error)
	VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*User, error)
}

type VaultConfig struct {
	ServerHost             string
	ServerPort             string
	GoogleClientID         string
	GoogleClientSecret     string
	AppleClientID          string
	AppleClientSecret      string
	RefreshTokenExpiration time.Duration
	AccessTokenExpiration  time.Duration
}

type VaultImpl struct {
	AccessTokenParser      paseto.Parser
	RefreshTokenParser     paseto.Parser
	StateTokenParser       paseto.Parser
	V4AsymmetricPublicKey  paseto.V4AsymmetricPublicKey
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

	stateTokenParser := paseto.NewParser()
	stateTokenParser.AddRule(paseto.ForAudience(StateTokenAudience))
	stateTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))
	stateTokenParser.AddRule(paseto.NotExpired())
	stateTokenParser.AddRule(paseto.NotBeforeNbf())

	sk, err := loadOrCreateSymmetricKey()
	if err != nil {
		slog.Error("failed to init symmetric key", "err", err)
		return nil, ErrFailedSaveSymmetricKey
	}

	ak, err := loadOrCreateAsymmetricKey()
	if err != nil {
		slog.Error("failed to init asymmetric key", "err", err)
		return nil, ErrFailedSaveAsymmetricKey
	}

	googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, ErrFailedCreateGoogleOIDCProvider
	}

	appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
	if err != nil {
		return nil, ErrFailedCreateAppleOIDCProvider
	}

	vault := VaultImpl{
		AccessTokenParser:     accessTokenParser,
		RefreshTokenParser:    refreshTokenParser,
		StateTokenParser:      stateTokenParser,
		V4AsymmetricSecretKey: ak,
		V4AsymmetricPublicKey: ak.Public(),
		V4SymmetricKey:        sk,
		GoogleOIDCConfig: OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				RedirectURL:  fmt.Sprintf("%s/oauth/callback", cfg.ServerHost),
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
				Endpoint:     googleProvider.Endpoint(),
			},
			Verifier: googleProvider.Verifier(&oidc.Config{ClientID: cfg.GoogleClientID}),
		},
		AppleOIDCConfig: OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     cfg.AppleClientID,
				ClientSecret: cfg.AppleClientSecret,
				RedirectURL:  fmt.Sprintf("%s/oauth/callback", cfg.ServerHost),
				Scopes:       []string{oidc.ScopeOpenID, "name", "email"},
				Endpoint:     appleProvider.Endpoint(),
			},
			Verifier: appleProvider.Verifier(&oidc.Config{ClientID: cfg.AppleClientID}),
		},
		RefreshTokenExpiration: cfg.RefreshTokenExpiration,
		AccessTokenExpiration:  cfg.AccessTokenExpiration,
	}

	return &vault, nil
}

type StateTokenClaims struct {
	Provider Provider `json:"provider"`
	CSRF     string   `json:"csrf"`
}

func (v VaultImpl) CreateStateToken(provider Provider) (string, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetIssuedAt(now)
	token.SetNotBefore(now)
	token.SetExpiration(now.Add(DefaultStateTokenExpiration))

	if err := token.Set("provider", provider); err != nil {
		return "", ErrFailedSetProvider
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := base64.URLEncoding.EncodeToString(b)
	token.SetString("csrf", state)

	token.SetAudience(StateTokenAudience)
	token.SetIssuer(TokenIssuer)

	return token.V4Encrypt(v.V4SymmetricKey, nil), nil
}

func (v VaultImpl) ParseStateToken(raw string) (*StateTokenClaims, error) {
	token, err := v.StateTokenParser.ParseV4Local(v.V4SymmetricKey, raw, nil)
	if err != nil {
		return nil, ErrInvalidStateToken
	}

	provider, err := token.GetString("provider")
	if err != nil {
		return nil, ErrFailedParseProvider
	}
	validProvider, err := ToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
	}

	csrf, err := token.GetString("csrf")
	if err != nil {
		return nil, ErrFailedParseCSRF
	}

	return &StateTokenClaims{
		Provider: validProvider,
		CSRF:     csrf,
	}, nil
}

func (v VaultImpl) CreateAuthCodeURL(state string, verifier string, provider Provider) (string, error) {
	var config *oauth2.Config

	switch provider {
	case ProviderGoogle:
		config = v.GoogleOIDCConfig.OAuth2Config
	case ProviderApple:
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
	RefreshTokenClaims struct {
		UserID   ULID
		Provider Provider
		JTI      ULID
	}

	AccessTokenClaims struct {
		UserID   ULID
		Provider Provider
		Roles    map[Role]ULID
	}

	AccessToken struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   uint32 `json:"expires_in"`
		Scope       string `json:"scope"`
		UserID      ULID   `json:"user_id"`
		CandidateID ULID   `json:"candidate_id,omitempty"`
		RecruiterID ULID   `json:"recruiter_id,omitempty"`
	}

	RefreshToken struct {
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    uint32 `json:"expires_in"`
		UserID       ULID   `json:"user_id"`
	}

	TokenPair struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    uint32 `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		UserID       ULID   `json:"user_id"`
	}
)

func (v VaultImpl) ParseAccessToken(tokenString string) (*AccessTokenClaims, error) {
	parsedToken, err := v.AccessTokenParser.ParseV4Public(v.V4AsymmetricPublicKey, tokenString, nil)
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
	validProvider, err := ToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
	}

	roles := make(map[Role]ULID)
	candidateID, err := parsedToken.GetString("candidate_id")
	if candidateID != "" {
		roles[RoleCandidate] = ULID(candidateID)
	}
	recruiterID, err := parsedToken.GetString("recruiter_id")
	if candidateID != "" {
		roles[RoleRecruiter] = ULID(recruiterID)
	}

	return &AccessTokenClaims{
		UserID:   ULID(userID),
		Provider: validProvider,
		Roles:    roles,
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
	validProvider, err := ToProvider(provider, DefaultProvider)
	if err != nil {
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
		UserID:   ULID(userID),
		Provider: validProvider,
		JTI:      ULID(jti),
	}, nil
}

func RolesToScope(roles map[Role]ULID) (*Scope, error) {
	var scope Scope
	for role := range roles {
		switch {
		case role == RoleCandidate:
			scope = append(scope, ScopeValue("role:"+string(RoleCandidate)))
		case role == RoleOnboarding:
			scope = append(scope, ScopeValue("role:"+string(RoleOnboarding)))
		case role == RoleRecruiter:
			scope = append(scope, ScopeValue("role:"+string(RoleRecruiter)))
		default:
			return nil, ErrInvalidRole
		}
	}
	return &scope, nil
}

func (v VaultImpl) CreateAccessToken(userID ULID, provider Provider, roles map[Role]ULID) (*AccessToken, error) {
	now := time.Now().UTC()

	_, hasOnboarding := roles[RoleOnboarding]
	var expiration time.Duration
	switch {
	case hasOnboarding:
		expiration = 24 * time.Hour
	case v.AccessTokenExpiration != 0:
		expiration = v.AccessTokenExpiration
	default:
		expiration = DefaultAccessTokenExpiration
	}

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(expiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("token_type", IssuedTokenTypeAccessToken); err != nil {
		return nil, ErrFailedSetTokenType
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, ErrFailedSetProvider
	}

	scope, err := RolesToScope(roles)
	if err != nil {
		return nil, ErrFailedSetScope
	}
	if err := token.Set("scope", scope); err != nil {
		return nil, ErrFailedSetScope
	}

	var candidateID, recruiterID ULID
	for role, id := range roles {
		switch role {
		case RoleCandidate:
			if err := token.Set("candidate_id", id); err != nil {
				return nil, ErrFailedSetScope
			}
			candidateID = id

		case RoleRecruiter:
			if err := token.Set("recruiter_id", id); err != nil {
				return nil, ErrFailedSetScope
			}
			recruiterID = id
		}
	}

	return &AccessToken{
		AccessToken: token.V4Sign(v.V4AsymmetricSecretKey, nil),
		TokenType:   "Bearer",
		ExpiresIn:   uint32(expiration.Abs().Seconds()),
		Scope:       scope.Raw(),
		UserID:      userID,
		CandidateID: candidateID,
		RecruiterID: recruiterID,
	}, nil
}

func (v VaultImpl) CreateRefreshToken(userID ULID, provider Provider, jti ULID) (*RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(DefaultRefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(string(jti))

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

func (v VaultImpl) CreateTokenPair(userID ULID, provider Provider, jti ULID, roles map[Role]ULID) (*TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, roles)
	if err != nil {
		return nil, ErrFailedCreateAccessToken
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return nil, ErrFailedCreateRefreshToken
	}

	scope, err := RolesToScope(roles)
	if err != nil {
		return nil, ErrFailedParseScope
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

func loadOrCreateSymmetricKey() (paseto.V4SymmetricKey, error) {
	if data, err := os.ReadFile(skFile); err == nil {
		key, err := paseto.V4SymmetricKeyFromBytes(data)
		if err != nil {
			return key, err
		}
		return key, nil
	}

	key := paseto.NewV4SymmetricKey()
	f, err := os.Create(skFile)
	defer f.Close()
	if err != nil {
		return key, err
	}
	fmt.Fprint(f, key.ExportHex())

	return key, nil
}

func loadOrCreateAsymmetricKey() (paseto.V4AsymmetricSecretKey, error) {
	if data, err := os.ReadFile(akFile); err == nil {
		key, err := paseto.NewV4AsymmetricSecretKeyFromBytes(data)
		if err != nil {
			return key, err
		}
		return key, nil
	}

	key := paseto.NewV4AsymmetricSecretKey()
	f, err := os.Create(akFile)
	defer f.Close()
	if err != nil {
		return key, err
	}
	fmt.Fprint(f, key.ExportHex())

	return key, nil
}
