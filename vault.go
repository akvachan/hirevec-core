// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
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
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"uuid"

	"aidanwoods.dev/go-paseto"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type Provider string

const (
	ProviderApple  Provider = "apple"
	ProviderGoogle Provider = "google"
	ProviderEmail  Provider = "email"
)

const DefaultProvider = ProviderGoogle

var ErrInvalidProvider = errors.New("invalid provider")

func StringToProvider(str string) (Provider, error) {
	switch str {
	case "apple":
		return ProviderApple, nil
	case "google":
		return ProviderGoogle, nil
	case "email":
		return ProviderEmail, nil
	default:
		return "", ErrInvalidProvider
	}
}

func (p Provider) ToString() string {
	return string(p)
}

type Role string

const (
	RoleCandidate Role = "candidate"
	RoleRecruiter Role = "recruiter"

	// TODO: Create admin role with access of editing all resources
	// RoleAdmin Role = "admin"
)

var ErrInvalidRole = errors.New("invalid role")

func StringToRole(roleStr string) (Role, error) {
	if roleStr == string(RoleCandidate) {
		return RoleCandidate, nil
	} else if roleStr == string(RoleRecruiter) {
		return RoleRecruiter, nil
	} else {
		return Role(""), ErrInvalidRole
	}
}

const (
	DefaultAccessTokenExpiration = 30 * time.Minute // 30 minutes
	DefaultStateTokenExpiration  = 10 * time.Minute // 10 minutes
	DefaultVerifierExpiration    = 10 * time.Minute // 10 minutes
)

const (
	TokenAudience      = "hirevec.com"
	TokenIssuer        = "hirevec.com"
	StateTokenAudience = "oauth-state"
)

type OIDCConfig struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

type VaultConfig struct {
	ServerBaseURL                    *url.URL
	SymmetricKey                     string
	AsymmetricKey                    string
	UseGoogleSSO                     bool
	UseAppleSSO                      bool
	GoogleClientID                   string
	GoogleClientSecret               string
	AppleClientID                    string
	AppleClientSecret                string
	AccessTokenExpiration            time.Duration
	DevelopmentMode                  bool
	DevelopmentModeTokensNeverExpire bool
}

type Vault struct {
	AccessTokenParser     paseto.Parser
	RefreshTokenParser    paseto.Parser
	StateTokenParser      paseto.Parser
	V4AsymmetricPublicKey paseto.V4AsymmetricPublicKey
	V4AsymmetricSecretKey paseto.V4AsymmetricSecretKey
	V4SymmetricKey        paseto.V4SymmetricKey
	UseGoogleSSO          bool
	UseAppleSSO           bool
	GoogleOIDCConfig      *OIDCConfig
	AppleOIDCConfig       *OIDCConfig
	AccessTokenExpiration time.Duration
	StateTokenExpiration  time.Duration
	VerifierExpiration    time.Duration
}

func NewVault(ctx context.Context, c VaultConfig) (Vault, error) {
	slog.Debug("initializing vault")

	accessTokenParser := paseto.NewParser()
	accessTokenParser.AddRule(paseto.ForAudience(TokenAudience))
	accessTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))

	refreshTokenParser := paseto.NewParser()
	refreshTokenParser.AddRule(paseto.ForAudience(TokenAudience))
	refreshTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))

	stateTokenParser := paseto.NewParser()
	stateTokenParser.AddRule(paseto.ForAudience(StateTokenAudience))
	stateTokenParser.AddRule(paseto.IssuedBy(TokenIssuer))

	if c.DevelopmentMode && c.DevelopmentModeTokensNeverExpire {
		slog.Warn("ignoring token expiration validation", "development_mode", c.DevelopmentMode)
	} else {
		accessTokenParser.AddRule(paseto.NotExpired())
		accessTokenParser.AddRule(paseto.NotBeforeNbf())

		stateTokenParser.AddRule(paseto.NotExpired())
		stateTokenParser.AddRule(paseto.NotBeforeNbf())
	}

	sk, err := LoadOrCreateSymmetricKey(c.SymmetricKey)
	if err != nil {
		slog.Error("failed to init symmetric key", "err", err)
		return Vault{}, fmt.Errorf("failed to init symmetric key: %w", err)
	}

	ak, err := LoadOrCreateAsymmetricKey(c.AsymmetricKey)
	if err != nil {
		slog.Error("failed to init asymmetric key", "err", err)
		return Vault{}, fmt.Errorf("failed to init asymmetric key: %w", err)
	}

	var googleOIDCConfig *OIDCConfig
	if c.UseGoogleSSO {
		slog.Debug("connecting to SSO provider", "provider", "google")
		googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			return Vault{}, fmt.Errorf("failed to initialize Google provider: %w", err)
		}
		callbackURL := strings.Replace(string(RouteCallback), "{provider}", "google", 1)
		googleOIDCConfig = &OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     c.GoogleClientID,
				ClientSecret: c.GoogleClientSecret,
				RedirectURL:  fmt.Sprintf("%s%s", c.ServerBaseURL, callbackURL),
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
				Endpoint:     googleProvider.Endpoint(),
			},
			Verifier: googleProvider.Verifier(&oidc.Config{
				ClientID: c.GoogleClientID,
			}),
		}
	}

	var appleOIDCConfig *OIDCConfig
	if c.UseAppleSSO {
		slog.Debug("connecting to SSO provider", "provider", "apple")
		appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
		if err != nil {
			return Vault{}, fmt.Errorf("failed to initialize Apple provider: %w", err)
		}
		callbackURL := strings.Replace(string(RouteCallback), "{provider}", "apple", 1)
		appleOIDCConfig = &OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     c.AppleClientID,
				ClientSecret: c.AppleClientSecret,
				RedirectURL:  fmt.Sprintf("%s%s", c.ServerBaseURL, callbackURL),
				Scopes:       []string{oidc.ScopeOpenID, "name", "email"},
				Endpoint:     appleProvider.Endpoint(),
			},
			Verifier: appleProvider.Verifier(&oidc.Config{
				ClientID: c.AppleClientID,
			}),
		}
	}

	accessTokenExpiration := DefaultAccessTokenExpiration
	if c.AccessTokenExpiration != 0 {
		accessTokenExpiration = c.AccessTokenExpiration
	}

	vault := Vault{
		AccessTokenParser:     accessTokenParser,
		RefreshTokenParser:    refreshTokenParser,
		StateTokenParser:      stateTokenParser,
		V4AsymmetricSecretKey: ak,
		V4AsymmetricPublicKey: ak.Public(),
		V4SymmetricKey:        sk,
		UseGoogleSSO:          c.UseGoogleSSO,
		UseAppleSSO:           c.UseAppleSSO,
		GoogleOIDCConfig:      googleOIDCConfig,
		AppleOIDCConfig:       appleOIDCConfig,
		AccessTokenExpiration: accessTokenExpiration,
		StateTokenExpiration:  DefaultStateTokenExpiration,
		VerifierExpiration:    DefaultVerifierExpiration,
	}

	return vault, nil
}

func (v Vault) CreateStateToken(provider Provider) (string, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetIssuedAt(now)
	token.SetNotBefore(now)
	token.SetExpiration(now.Add(v.StateTokenExpiration))

	if err := token.Set("provider", provider); err != nil {
		return "", fmt.Errorf("failed to set provider: %w", err)
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	state := base64.URLEncoding.EncodeToString(b)
	token.SetString("csrf", state)
	token.SetAudience(StateTokenAudience)
	token.SetIssuer(TokenIssuer)

	return token.V4Encrypt(v.V4SymmetricKey, nil), nil
}

type StateTokenClaims struct {
	CSRF string `json:"csrf"`
}

func (v Vault) ParseStateToken(raw string) (StateTokenClaims, error) {
	token, err := v.StateTokenParser.ParseV4Local(v.V4SymmetricKey, raw, nil)
	if err != nil {
		return StateTokenClaims{}, fmt.Errorf("failed to parse token: %w", err)
	}

	csrf, err := token.GetString("csrf")
	if err != nil {
		return StateTokenClaims{}, fmt.Errorf("failed to get CSRF: %w", err)
	}

	return StateTokenClaims{
		CSRF: csrf,
	}, nil
}

var (
	ErrGoogleOIDCConfigNotInitialized = errors.New("configuration Google OIDC is not initialized")
	ErrAppleOIDCConfigNotInitialized  = errors.New("configuration Apple OIDC is not initialized")
)

func (v Vault) CreateAuthCodeURL(state string, verifier string, provider Provider) (string, error) {
	var config *oauth2.Config
	switch provider {
	case ProviderGoogle:
		if v.GoogleOIDCConfig != nil {
			config = v.GoogleOIDCConfig.OAuth2Config
		} else {
			return "", ErrGoogleOIDCConfigNotInitialized
		}
	case ProviderApple:
		if v.AppleOIDCConfig != nil {
			config = v.AppleOIDCConfig.OAuth2Config
		} else {
			return "", ErrAppleOIDCConfigNotInitialized
		}
	default:
		return "", ErrInvalidProvider
	}
	return config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

var ErrIDTokenRequired = errors.New("id_token is required")

func (v Vault) ExchangeGoogleCodeForIDToken(ctx context.Context, code string, verifierCookie *http.Cookie) (string, error) {
	tok, err := v.GoogleOIDCConfig.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
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
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", ErrIDTokenRequired
	}

	return rawIDToken, nil
}

type GoogleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

type IDToken struct {
	Provider       Provider
	ProviderUserID string
	Email          string
	FullName       string
}

var (
	ErrFailedParseClaims = errors.New("failed to parse claims")
	ErrInvalidIDToken    = errors.New("invalid id_token")
)

func (v Vault) VerifyAndParseGoogleIDToken(ctx context.Context, rawIDToken string) (IDToken, error) {
	idToken, err := v.GoogleOIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return IDToken{}, ErrInvalidIDToken
	}

	var claims GoogleClaims
	if err := idToken.Claims(&claims); err != nil {
		return IDToken{}, ErrFailedParseClaims
	}

	if !claims.EmailVerified {
		return IDToken{}, ErrEmailNotVerified
	}

	return IDToken{
		ProviderGoogle,
		claims.Sub,
		claims.Email,
		claims.Name,
	}, nil
}

type AppleClaims struct {
	Sub            string          `json:"sub"`
	Email          string          `json:"email"`
	EmailVerified  json.RawMessage `json:"email_verified"`
	IsPrivateEmail string          `json:"is_private_email"`
}

func (v Vault) VerifyAndParseAppleIDToken(ctx context.Context, rawIDToken string, userJSON string) (IDToken, error) {
	idToken, err := v.AppleOIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return IDToken{}, ErrInvalidIDToken
	}

	var claims AppleClaims
	if err := idToken.Claims(&claims); err != nil {
		return IDToken{}, ErrFailedParseClaims
	}

	var verified bool
	if len(claims.EmailVerified) > 0 {
		if err := json.Unmarshal(claims.EmailVerified, &verified); err != nil {
			var verifiedStr string
			if json.Unmarshal(claims.EmailVerified, &verifiedStr) == nil {
				verified = (strings.ToLower(verifiedStr) == "true")
			}
		}
	}
	if !verified {
		return IDToken{}, ErrEmailNotVerified
	}

	email, err := mail.ParseAddress(claims.Email)
	if err != nil {
		return IDToken{}, fmt.Errorf("failed to parse address: %w", err)
	}

	var fullName string
	if userJSON != "" {
		var appleUser struct {
			Name struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"name"`
		}
		if err := json.Unmarshal([]byte(userJSON), &appleUser); err == nil {
			fullName = fmt.Sprintf("%s %s", appleUser.Name.FirstName, appleUser.Name.LastName)
		}
	}

	return IDToken{
		ProviderApple,
		claims.Sub,
		email.Address,
		fullName,
	}, nil
}

type AccessTokenClaims struct {
	UserID   uuid.UUID
	Provider Provider
	Roles    map[Role]uuid.UUID
}

func (v Vault) ParseAccessToken(tokenString string) (AccessTokenClaims, error) {
	parsedToken, err := v.AccessTokenParser.ParseV4Public(
		v.V4AsymmetricPublicKey,
		tokenString,
		nil,
	)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("failed to parse token: %w", err)
	}

	userIDRaw, err := parsedToken.GetSubject()
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("failed to get subject: %w", err)
	}
	userID, err := uuid.Parse(userIDRaw)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("failed to parse userID: %w", err)
	}

	stringProvider, err := parsedToken.GetString("provider")
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("failed to get provider: %w", err)
	}
	provider, err := StringToProvider(stringProvider)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("failed to convert string to provider: %w", err)
	}

	roles := make(map[Role]uuid.UUID, 3)
	recruiterID, _ := parsedToken.GetString("recruiter_id")
	candidateID, _ := parsedToken.GetString("candidate_id")
	if recruiterID != "" {
		if roles[RoleRecruiter], err = uuid.Parse(recruiterID); err != nil {
			return AccessTokenClaims{}, fmt.Errorf("failed to parse recruiterID: %w", err)
		}
	}
	if candidateID != "" {
		if roles[RoleCandidate], err = uuid.Parse(candidateID); err != nil {
			return AccessTokenClaims{}, fmt.Errorf("failed to parse candidateID: %w", err)
		}
	}

	return AccessTokenClaims{
		UserID:   userID,
		Provider: provider,
		Roles:    roles,
	}, nil
}

type RefreshTokenClaims struct {
	UserID   uuid.UUID
	Provider Provider
	JTI      uuid.UUID
}

func (v Vault) ParseRefreshToken(tokenString string) (RefreshTokenClaims, error) {
	parsedToken, err := v.RefreshTokenParser.ParseV4Local(
		v.V4SymmetricKey,
		tokenString,
		nil,
	)
	if err != nil {
		slog.Error("failed to parse refresh token", "err", err)
		return RefreshTokenClaims{}, fmt.Errorf("failed to parse token: %w", err)
	}

	userIDRaw, err := parsedToken.GetSubject()
	if err != nil {
		return RefreshTokenClaims{}, fmt.Errorf("failed to get subject: %w", err)
	}
	userID, err := uuid.Parse(userIDRaw)
	if err != nil {
		return RefreshTokenClaims{}, fmt.Errorf("failed to parse userID: %w", err)
	}

	stringProvider, err := parsedToken.GetString("provider")
	if err != nil {
		return RefreshTokenClaims{}, fmt.Errorf("failed to get provider: %w", err)
	}
	provider, err := StringToProvider(stringProvider)
	if err != nil {
		return RefreshTokenClaims{}, ErrInvalidProvider
	}

	jtiRaw, err := parsedToken.GetJti()
	if err != nil {
		return RefreshTokenClaims{}, fmt.Errorf("failed to get JTI: %w", err)
	}
	jti, err := uuid.Parse(jtiRaw)
	if err != nil {
		return RefreshTokenClaims{}, fmt.Errorf("failed to parse JTI: %w", err)
	}

	return RefreshTokenClaims{
		UserID:   userID,
		Provider: provider,
		JTI:      jti,
	}, nil
}

type Scope string

const (
	ScopeRoleCandidate Scope = "role:candidate"
	ScopeRoleRecruiter Scope = "role:recruiter"
)

type AccessToken struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   uint32    `json:"expires_in"`
	Scope       string    `json:"scope"`
	UserID      uuid.UUID `json:"user_id"`
}

func (v Vault) CreateAccessToken(userID uuid.UUID, provider Provider, roles map[Role]uuid.UUID) (AccessToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(userID.String())
	token.SetExpiration(now.Add(v.AccessTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("provider", provider); err != nil {
		return AccessToken{}, fmt.Errorf("failed to set provider: %w", err)
	}

	var scopes []string

	if _, ok := roles[RoleCandidate]; ok {
		if err := token.Set("candidate_id", roles[RoleCandidate]); err != nil {
			return AccessToken{}, fmt.Errorf("failed to set candidate_id: %w", err)
		}
		scopes = append(scopes, string(ScopeRoleCandidate))
	}

	if _, ok := roles[RoleRecruiter]; ok {
		if err := token.Set("recruiter_id", roles[RoleRecruiter]); err != nil {
			return AccessToken{}, fmt.Errorf("failed to set recruiter_id: %w", err)
		}
		scopes = append(scopes, string(ScopeRoleRecruiter))
	}

	scopeStr := strings.Join(scopes, " ")
	token.SetString("scope", scopeStr)

	return AccessToken{
		AccessToken: token.V4Sign(v.V4AsymmetricSecretKey, nil),
		TokenType:   "Bearer",
		ExpiresIn:   uint32(v.AccessTokenExpiration.Seconds()),
		Scope:       scopeStr,
		UserID:      userID,
	}, nil
}

type RefreshToken struct {
	RefreshToken string    `json:"refresh_token"`
	UserID       uuid.UUID `json:"user_id"`
}

func (v Vault) CreateRefreshToken(userID uuid.UUID, provider Provider, jti uuid.UUID) (RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(userID.String())
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(jti.String())

	if err := token.Set("provider", provider); err != nil {
		return RefreshToken{}, fmt.Errorf("failed to set provider: %w", err)
	}

	return RefreshToken{
		RefreshToken: token.V4Encrypt(v.V4SymmetricKey, nil),
		UserID:       userID,
	}, nil
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    uint32    `json:"expires_in"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	UserID       uuid.UUID `json:"user_id"`
}

func (v Vault) CreateTokenPair(userID uuid.UUID, provider Provider, jti uuid.UUID, roles map[Role]uuid.UUID) (TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, roles)
	if err != nil {
		return TokenPair{}, fmt.Errorf("failed to create access token: %w", err)
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return TokenPair{}, fmt.Errorf("failed to create refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:  accessToken.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(v.AccessTokenExpiration.Seconds()),
		RefreshToken: refreshToken.RefreshToken,
		Scope:        accessToken.Scope,
		UserID:       userID,
	}, nil
}

const envFile = ".env"

var envFileLock sync.Mutex

func UpsertEnvKey(filename string, key string, value string) error {
	envFileLock.Lock()
	defer envFileLock.Unlock()

	data, err := os.ReadFile(filename)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read file: %w", err)
		}
		if err := os.WriteFile(filename, []byte(key+"="+value+"\n"), 0o600); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
	}

	found := false
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
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

	return os.WriteFile(filename, []byte(output), 0o600)
}

func LoadOrCreateSymmetricKey(val string) (paseto.V4SymmetricKey, error) {
	if val != "" {
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return paseto.V4SymmetricKey{}, fmt.Errorf("failed to decode string: %w", err)
		}
		return paseto.V4SymmetricKeyFromBytes(decoded)
	}

	key := paseto.NewV4SymmetricKey()
	if err := UpsertEnvKey(
		envFile,
		"HIREVEC_SYMMETRIC_KEY",
		key.ExportHex(),
	); err != nil {
		return key, fmt.Errorf("failed to upsert environment variable: %w", err)
	}

	return key, nil
}

func LoadOrCreateAsymmetricKey(val string) (paseto.V4AsymmetricSecretKey, error) {
	if val != "" {
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return paseto.V4AsymmetricSecretKey{}, fmt.Errorf("failed to decode string: %w", err)
		}
		return paseto.NewV4AsymmetricSecretKeyFromBytes(decoded)
	}

	key := paseto.NewV4AsymmetricSecretKey()
	if err := UpsertEnvKey(
		envFile, "HIREVEC_ASYMMETRIC_KEY",
		key.ExportHex(),
	); err != nil {
		return key, fmt.Errorf("failed to upsert environment variable: %w", err)
	}

	return key, nil
}

func MustParseEnv[T any](key string, parser func(string) (T, error), defaultValue T) T {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return defaultValue
	}

	parsed, err := parser(value)
	if err != nil {
		panic(fmt.Errorf("failed to parse environment variable %q: %w", key, err))
	}

	return parsed
}

func Loadenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Error("failed to close file", "err", err)
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
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if err = os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", key, err)
		}
	}

	return scanner.Err()
}

func IsValidPassword(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(password),
	) == nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash from password: %w", err)
	}
	return string(hash), nil
}
