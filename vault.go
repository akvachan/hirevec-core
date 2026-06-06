// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

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
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

const (
	DefaultRefreshTokenExpiration = 30 * 24 * time.Hour
	DefaultAccessTokenExpiration  = 30 * time.Minute
	DefaultStateTokenExpiration   = 10 * time.Minute
	DefaultVerifierExpiration     = 10 * time.Minute
	DefaultProvider               = ProviderEmail
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
	ProviderEmail  Provider = "email"
)

func StringToProvider(str string, def Provider) (Provider, error) {
	switch str {
	case "apple":
		return ProviderApple, nil
	case "google":
		return ProviderGoogle, nil
	case "email":
		return ProviderEmail, nil
	case "":
		return DefaultProvider, nil
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
)

type Scope string

const (
	ScopeRoleCandidate Scope = "role:candidate"
	ScopeRoleRecruiter Scope = "role:recruiter"
)

type VaultConfig struct {
	ServerBaseURL          string
	SymmetricKey           string
	AsymmetricKey          string
	UseGoogleSSO           bool
	UseAppleSSO            bool
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
	UseGoogleSSO           bool
	UseAppleSSO            bool
	GoogleOIDCConfig       *OIDCConfig
	AppleOIDCConfig        *OIDCConfig
	RefreshTokenExpiration time.Duration
	AccessTokenExpiration  time.Duration
	StateTokenExpiration   time.Duration
	VerifierExpiration     time.Duration
}

type OIDCConfig struct {
	OAuth2Config *oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

func NewVault(ctx context.Context, c VaultConfig) (*Vault, error) {
	slog.Debug("initializing vault")

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

	sk, err := LoadOrCreateSymmetricKey(c.SymmetricKey)
	if err != nil {
		slog.Error(
			"failed to init symmetric key",
			"err", err,
		)
		return nil, err
	}

	ak, err := LoadOrCreateAsymmetricKey(c.AsymmetricKey)
	if err != nil {
		slog.Error(
			"failed to init asymmetric key",
			"err", err,
		)
		return nil, err
	}

	var googleOIDCConfig *OIDCConfig
	if c.UseGoogleSSO {
		slog.Debug("connecting to SSO provider", "provider", "google")
		googleProvider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			return nil, err
		}
		googleOIDCConfig = &OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     c.GoogleClientID,
				ClientSecret: c.GoogleClientSecret,
				RedirectURL:  fmt.Sprintf("%s%s", c.ServerBaseURL, RouteOAuthCallback),
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
				Endpoint:     googleProvider.Endpoint(),
			},
			Verifier: googleProvider.Verifier(
				&oidc.Config{ClientID: c.GoogleClientID},
			),
		}
	}

	var appleOIDCConfig *OIDCConfig
	if c.UseAppleSSO {
		slog.Debug("connecting to SSO provider", "provider", "apple")
		appleProvider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
		if err != nil {
			return nil, err
		}
		appleOIDCConfig = &OIDCConfig{
			OAuth2Config: &oauth2.Config{
				ClientID:     c.AppleClientID,
				ClientSecret: c.AppleClientSecret,
				RedirectURL:  fmt.Sprintf("%s%s", c.ServerBaseURL, RouteOAuthCallback),
				Scopes:       []string{oidc.ScopeOpenID, "name", "email"},
				Endpoint:     appleProvider.Endpoint(),
			},
			Verifier: appleProvider.Verifier(&oidc.Config{ClientID: c.AppleClientID}),
		}
	}

	accessTokenExpiration := DefaultAccessTokenExpiration
	if c.AccessTokenExpiration != 0 {
		accessTokenExpiration = c.AccessTokenExpiration
	}

	refreshTokenExpiration := DefaultRefreshTokenExpiration
	if c.RefreshTokenExpiration != 0 {
		refreshTokenExpiration = c.RefreshTokenExpiration
	}

	vault := Vault{
		AccessTokenParser:      accessTokenParser,
		RefreshTokenParser:     refreshTokenParser,
		StateTokenParser:       stateTokenParser,
		V4AsymmetricSecretKey:  ak,
		V4AsymmetricPublicKey:  ak.Public(),
		V4SymmetricKey:         sk,
		UseGoogleSSO:           c.UseGoogleSSO,
		UseAppleSSO:            c.UseAppleSSO,
		GoogleOIDCConfig:       googleOIDCConfig,
		AppleOIDCConfig:        appleOIDCConfig,
		RefreshTokenExpiration: refreshTokenExpiration,
		AccessTokenExpiration:  accessTokenExpiration,
		StateTokenExpiration:   DefaultStateTokenExpiration,
		VerifierExpiration:     DefaultVerifierExpiration,
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
	token.SetExpiration(now.Add(v.StateTokenExpiration))

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
	validProvider, err := StringToProvider(provider, DefaultProvider)
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

func (v Vault) CreateAuthCodeURL(
	state string,
	verifier string,
	provider Provider,
) (string, error) {
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

func (v Vault) ExchangeGoogleCodeForIDToken(
	ctx context.Context,
	code string,
	verifierCookie *http.Cookie,
) (string, error) {
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

func (v Vault) ExchangeAppleCodeForIDToken(
	ctx context.Context,
	code string,
	verifierCookie *http.Cookie,
) (string, error) {
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

func (v Vault) VerifyAndParseGoogleIDToken(
	ctx context.Context,
	rawIDToken string,
) (*User, error) {
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

func (v Vault) VerifyAndParseAppleIDToken(
	ctx context.Context,
	rawIDToken string,
	userJSON string,
) (*User, error) {
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

func (v Vault) ParseAccessToken(
	tokenString string,
) (*AccessTokenClaims, error) {
	parsedToken, err := v.AccessTokenParser.ParseV4Public(
		v.V4AsymmetricPublicKey,
		tokenString,
		nil,
	)
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
	validProvider, err := StringToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
	}

	roles := make(map[Role]ULID, 3)
	recruiterID, _ := parsedToken.GetString("recruiter_id")
	candidateID, _ := parsedToken.GetString("candidate_id")

	if recruiterID != "" {
		roles[RoleRecruiter] = ULID(recruiterID)
	}
	if candidateID != "" {
		roles[RoleCandidate] = ULID(candidateID)
	}

	return &AccessTokenClaims{
		UserID:   ULID(userID),
		Provider: validProvider,
		Roles:    roles,
	}, nil
}

func (v Vault) ParseRefreshToken(
	tokenString string,
) (*RefreshTokenClaims, error) {
	parsedToken, err := v.RefreshTokenParser.ParseV4Local(
		v.V4SymmetricKey,
		tokenString,
		nil,
	)
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
	validProvider, err := StringToProvider(provider, DefaultProvider)
	if err != nil {
		return nil, ErrInvalidProvider
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

func (v Vault) CreateAccessToken(
	userID ULID,
	provider Provider,
	roles map[Role]ULID,
) (*AccessToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(v.AccessTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)

	if err := token.Set("provider", provider); err != nil {
		return nil, err
	}

	var scope string
	var scopeBuilder strings.Builder

	candidateID, ok := roles[RoleCandidate]
	if ok {
		if err := token.Set("candidate_id", candidateID); err != nil {
			return nil, err
		}
		scopeBuilder.Write([]byte(ScopeRoleCandidate))
		scopeBuilder.WriteString(" ")
	}

	recruiterID, ok := roles[RoleRecruiter]
	if ok {
		if err := token.Set("recruiter_id", recruiterID); err != nil {
			return nil, err
		}
		scopeBuilder.Write([]byte(ScopeRoleRecruiter))
		scopeBuilder.WriteString(" ")
	}

	scope = scopeBuilder.String()
	token.SetString("scope", scope)

	return &AccessToken{
		AccessToken: token.V4Sign(v.V4AsymmetricSecretKey, nil),
		TokenType:   "Bearer",
		ExpiresIn:   uint32(v.AccessTokenExpiration.Seconds()),
		Scope:       scope,
		UserID:      userID,
		CandidateID: candidateID,
		RecruiterID: recruiterID,
	}, nil
}

func (v Vault) CreateRefreshToken(
	userID ULID,
	provider Provider,
	jti ULID,
) (*RefreshToken, error) {
	now := time.Now().UTC()

	token := paseto.NewToken()
	token.SetAudience(TokenAudience)
	token.SetIssuer(TokenIssuer)
	token.SetSubject(string(userID))
	token.SetExpiration(now.Add(v.RefreshTokenExpiration))
	token.SetNotBefore(now)
	token.SetIssuedAt(now)
	token.SetJti(string(jti))

	if err := token.Set("provider", provider); err != nil {
		return nil, err
	}

	return &RefreshToken{
		RefreshToken: token.V4Encrypt(v.V4SymmetricKey, nil),
		ExpiresIn:    uint32(v.RefreshTokenExpiration.Seconds()),
		UserID:       userID,
	}, nil
}

func (v Vault) CreateTokenPair(
	userID ULID,
	provider Provider,
	jti ULID,
	roles map[Role]ULID,
) (*TokenPair, error) {
	accessToken, err := v.CreateAccessToken(userID, provider, roles)
	if err != nil {
		return nil, err
	}

	refreshToken, err := v.CreateRefreshToken(userID, provider, jti)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    uint32(v.AccessTokenExpiration.Seconds()),
		RefreshToken: refreshToken.RefreshToken,
		Scope:        accessToken.Scope,
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

func LoadOrCreateSymmetricKey(
	val string,
) (paseto.V4SymmetricKey, error) {
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

	if err := UpsertEnvKey(
		envFile,
		"HIREVEC_SYMMETRIC_KEY",
		hexVal,
	); err != nil {
		return key, err
	}

	return key, nil
}

func LoadOrCreateAsymmetricKey(
	val string,
) (paseto.V4AsymmetricSecretKey, error) {
	if val != "" {
		decoded, err := hex.DecodeString(val)
		if err != nil {
			return paseto.V4AsymmetricSecretKey{}, err
		}
		return paseto.NewV4AsymmetricSecretKeyFromBytes(decoded)
	}

	key := paseto.NewV4AsymmetricSecretKey()
	hexVal := key.ExportHex()

	if err := UpsertEnvKey(
		envFile, "HIREVEC_ASYMMETRIC_KEY",
		hexVal,
	); err != nil {
		return key, err
	}

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

func (v Vault) IsValidPassword(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(password),
	) == nil
}

func (v Vault) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	return string(hash), err
}
