// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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

var (
	ErrFailedParseClaims = errors.New("failed to parse claims")
	ErrIDTokenRequired   = errors.New("id_token required")
	ErrInvalidIDToken    = errors.New("invalid id_token")
	ErrInvalidProvider   = errors.New("invalid provider")
	ErrInvalidRole       = errors.New("invalid role")
	ErrInvalidTokenType  = errors.New("invalid token type")
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

type (
	ScopeValue string
	Scope      []ScopeValue
)

func (s Scope) Raw() string {
	var result []string
	for _, role := range s {
		result = append(result, string(role))
	}
	return strings.Join(result, " ")
}

type IssuedTokenType string

const (
	IssuedTokenTypeRefreshToken IssuedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
	IssuedTokenTypeAccessToken  IssuedTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

type VaultConfig struct {
	ServerHost             string
	ServerPort             uint16
	SymmetricKey           string
	AsymmetricKey          string
	GoogleClientID         string
	GoogleClientSecret     string
	AppleClientID          string
	AppleClientSecret      string
	RefreshTokenExpiration time.Duration
	AccessTokenExpiration  time.Duration
}

type Vault struct {
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

func NewVault(ctx context.Context, cfg VaultConfig) (*Vault, error) {
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

	sk, err := LoadOrCreateSymmetricKey(cfg.SymmetricKey)
	if err != nil {
		slog.Error("failed to init symmetric key", "err", err)
		return nil, err
	}

	ak, err := LoadOrCreateAsymmetricKey(cfg.AsymmetricKey)
	if err != nil {
		slog.Error("failed to init asymmetric key", "err", err)
		return nil, err
	}

	slog.Debug("connecting to SSO provider", "provider", "google")
	googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, err
	}

	slog.Debug("connecting to SSO provider", "provider", "apple")
	appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
	if err != nil {
		return nil, err
	}

	accessTokenExpiration := DefaultAccessTokenExpiration
	if cfg.AccessTokenExpiration != 0 {
		accessTokenExpiration = cfg.AccessTokenExpiration
	}

	refreshTokenExpiration := DefaultRefreshTokenExpiration
	if cfg.AccessTokenExpiration != 0 {
		refreshTokenExpiration = cfg.RefreshTokenExpiration
	}

	slog.Debug("initializing vault")
	vault := Vault{
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
		RefreshTokenExpiration: refreshTokenExpiration,
		AccessTokenExpiration:  accessTokenExpiration,
	}

	return &vault, nil
}

type StateTokenClaims struct {
	Provider Provider `json:"provider"`
	CSRF     string   `json:"csrf"`
}

func (v Vault) CreateStateToken(provider Provider) (string, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetIssuedAt(now)
	token.SetNotBefore(now)
	token.SetExpiration(now.Add(v.AccessTokenExpiration))

	if err := token.Set("provider", provider); err != nil {
		return "", err
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

func (v Vault) ParseStateToken(raw string) (*StateTokenClaims, error) {
	token, err := v.StateTokenParser.ParseV4Local(v.V4SymmetricKey, raw, nil)
	if err != nil {
		return nil, err
	}

	provider, err := token.GetString("provider")
	if err != nil {
		return nil, err
	}
	validProvider, err := ToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
	}

	csrf, err := token.GetString("csrf")
	if err != nil {
		return nil, err
	}

	return &StateTokenClaims{
		Provider: validProvider,
		CSRF:     csrf,
	}, nil
}

func (v Vault) CreateAuthCodeURL(state string, verifier string, provider Provider) (string, error) {
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

func (v Vault) ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.GoogleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", err
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v Vault) ExchangeAppleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.AppleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", err
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

func (v Vault) VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (*User, error) {
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

func (v Vault) VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (*User, error) {
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

func (v Vault) ParseAccessToken(tokenString string) (*AccessTokenClaims, error) {
	parsedToken, err := v.AccessTokenParser.ParseV4Public(v.V4AsymmetricPublicKey, tokenString, nil)
	if err != nil {
		return nil, err
	}

	userID, err := parsedToken.GetSubject()
	if err != nil {
		return nil, err
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, err
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

func (v Vault) ParseRefreshToken(tokenString string) (*RefreshTokenClaims, error) {
	parsedToken, err := v.RefreshTokenParser.ParseV4Local(v.V4SymmetricKey, tokenString, nil)
	if err != nil {
		return nil, err
	}

	userID, err := parsedToken.GetSubject()
	if err != nil || userID == "" {
		return nil, err
	}

	provider, err := parsedToken.GetString("provider")
	if err != nil {
		return nil, err
	}
	validProvider, err := ToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
	}

	tokenType, err := parsedToken.GetString("type")
	if err != nil {
		return nil, err
	}
	if tokenType != "refresh" {
		return nil, ErrInvalidTokenType
	}

	jti, err := parsedToken.GetJti()
	if err != nil || jti == "" {
		return nil, err
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

func (v Vault) CreateAccessToken(userID ULID, provider Provider, roles map[Role]ULID) (*AccessToken, error) {
	now := time.Now().UTC()

	expiration := v.AccessTokenExpiration
	if _, hasOnboardingRole := roles[RoleOnboarding]; hasOnboardingRole {
		expiration = 24 * time.Hour
	}

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(expiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("token_type", IssuedTokenTypeAccessToken); err != nil {
		return nil, err
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, err
	}

	scope, err := RolesToScope(roles)
	if err != nil {
		return nil, err
	}
	if err := token.Set("scope", scope); err != nil {
		return nil, err
	}

	var candidateID, recruiterID ULID
	for role, id := range roles {
		switch role {
		case RoleCandidate:
			if err := token.Set("candidate_id", id); err != nil {
				return nil, err
			}
			candidateID = id

		case RoleRecruiter:
			if err := token.Set("recruiter_id", id); err != nil {
				return nil, err
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

func (v Vault) CreateRefreshToken(userID ULID, provider Provider, jti ULID) (*RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(v.RefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(string(jti))

	if err := token.Set("token_type", IssuedTokenTypeRefreshToken); err != nil {
		return nil, err
	}

	if err := token.Set("provider", provider); err != nil {
		return nil, err
	}

	return &RefreshToken{
		RefreshToken: token.V4Encrypt(v.V4SymmetricKey, nil),
		ExpiresIn:    uint32(v.RefreshTokenExpiration.Abs().Seconds()),
		UserID:       userID,
	}, nil
}

func (v Vault) CreateTokenPair(userID ULID, provider Provider, jti ULID, roles map[Role]ULID) (*TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, roles)
	if err != nil {
		return nil, err
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return nil, err
	}

	scope, err := RolesToScope(roles)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(v.AccessTokenExpiration.Abs().Seconds()),
		RefreshToken: refreshToken.RefreshToken,
		Scope:        scope.Raw(),
		UserID:       userID,
	}, nil
}

const envFile = ".env"

func UpsertEnvKey(filename, key, value string) error {
	var lines []string
	found := false

	data, err := os.ReadFile(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(filename, []byte(key+"="+value+"\n"), 0o644)
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, key+"=") {
			lines = append(lines, key+"="+value)
			found = true
		} else {
			lines = append(lines, line)
		}
	}

	if !found {
		lines = append(lines, key+"="+value)
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return os.WriteFile(filename, []byte(output), 0o644)
}

func LoadOrCreateSymmetricKey(val string) (paseto.V4SymmetricKey, error) {
	if val != "" {
		slog.Debug("loading symmetric key")
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return paseto.V4SymmetricKey{}, err
		}
		return paseto.V4SymmetricKeyFromBytes(decoded)
	}

	key := paseto.NewV4SymmetricKey()
	hexVal := key.ExportHex()

	if err := UpsertEnvKey(envFile, "HIREVEC_SYMMETRIC_KEY", hexVal); err != nil {
		return key, err
	}

	_ = os.Setenv("HIREVEC_SYMMETRIC_KEY", hexVal)

	return key, nil
}

func LoadOrCreateAsymmetricKey(val string) (paseto.V4AsymmetricSecretKey, error) {
	if val != "" {
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return paseto.V4AsymmetricSecretKey{}, err
		}
		return paseto.NewV4AsymmetricSecretKeyFromBytes(decoded)
	}

	key := paseto.NewV4AsymmetricSecretKey()
	hexVal := key.ExportHex()

	if err := UpsertEnvKey(envFile, "HIREVEC_ASYMMETRIC_KEY", hexVal); err != nil {
		return key, err
	}

	_ = os.Setenv("HIREVEC_ASYMMETRIC_KEY", hexVal)

	return key, nil
}

func Getenv(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return defaultValue
	}
	return value
}

func Loadenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			slog.Error(
				"failed to properly close file",
				"err", err,
			)
			os.Exit(0)
		}
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		value = strings.Trim(value, `"'`)

		err = os.Setenv(key, value)
		if err != nil {
			return err
		}
	}

	return scanner.Err()
}
