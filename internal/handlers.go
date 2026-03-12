// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

type (
	// FailData defines [JSend](https://github.com/omniti-labs/jsend) request failure data.
	FailData map[string]string

	// ResponseStatus defines JSend status codes.
	ResponseStatus string

	// AuthErrorCode defienes OAuth2 error codes, see [RFC6749](https://www.rfc-editor.org/rfc/rfc6749.txt).
	AuthErrorCode string

	// ErrorCode defines JSend error codes.
	ErrorCode uint16

	// RelType defines link relation type, see [RFC5988](https://www.rfc-editor.org/rfc/rfc5988.txt).
	RelType string

	// Link defines a [HAL](https://datatracker.ietf.org/doc/html/draft-kelly-json-hal-11) link object.
	Link struct {
		Href      string `json:"href"`
		Name      string `json:"name,omitempty"`
		Templated bool   `json:"templated,omitempty"`
	}

	// Links defines a group of HAL links.
	Links map[RelType]Link

	// SuccessResponse defines a successful JSend HTTP response.
	SuccessResponse struct {
		Status ResponseStatus `json:"status"`
		Data   any            `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}

	// ErrorResponse defines an erroneous JSend HTTP response.
	ErrorResponse struct {
		Status  ResponseStatus `json:"status"`
		Message string         `json:"message"`
		Code    ErrorCode      `json:"code,omitempty"`
	}

	// FailResponse defines an HTTP request validation failure.
	FailResponse struct {
		Status ResponseStatus `json:"status"`
		Data   any            `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}

	// AuthErrorResponse defines OAuth2 error response.
	AuthErrorResponse struct {
		Error            AuthErrorCode `json:"error"`
		ErrorDescription string        `json:"error_description,omitempty"`
		ErrorURI         string        `json:"error_uri,omitempty"`
		Links            Links         `json:"_links,omitempty"`
	}
)

var (
	adjectives = []string{
		"fast", "lazy", "clever", "curious", "brave", "mighty", "silent", "noisy", "happy", "grumpy",
	}

	nouns = []string{
		"lion", "tiger", "panda", "fox", "eagle", "shark", "wolf", "dragon", "otter", "koala",
	}
)

const (
	// All went well, and (usually) some data was returned.
	ResponseStatusSuccess = "success"

	// There was a problem with the data submitted, or some pre-condition of the API call wasn't satisfied.
	ResponseStatusFail = "fail"

	// An error occurred in processing the request, i.e. an exception was thrown.
	ResponseStatusError = "error"

	/*
		The request is missing a required parameter, includes an
		unsupported parameter value (other than grant type),
		repeats a parameter, includes multiple credentials,
		utilizes more than one mechanism for authenticating the
		client, or is otherwise malformed.
	*/
	AuthInvalidRequest AuthErrorCode = "invalid_request"

	/*
		The provided authorization grant (e.g., authorization
		code, resource owner credentials) or refresh token is
		invalid, expired, revoked, does not match the redirection
		URI used in the authorization request, or was issued to
		another client.
	*/
	AuthInvalidGrant AuthErrorCode = "invalid_grant"

	/*
		Client authentication failed (e.g., unknown client, no
		client authentication included, or unsupported
		authentication method).  The authorization server MAY
		return an HTTP 401 (Unauthorized) status code to indicate
		which HTTP authentication schemes are supported.  If the
		client attempted to authenticate via the "Authorization"
		request header field, the authorization server MUST
		respond with an HTTP 401 (Unauthorized) status code and
		include the "WWW-Authenticate" response header field
		matching the authentication scheme used by the client.
	*/
	AuthInvalidClient AuthErrorCode = "invalid_client"

	/*
		The authenticated client is not authorized to use this
		authorization grant type.
	*/
	AuthUnauthorizedClient AuthErrorCode = "unauthorized_client"

	/*
		The authorization grant type is not supported by the
		authorization server.
	*/
	AuthUnsupportedGrantType AuthErrorCode = "unsupported_grant_type"

	// Conveys an identifier for the link's context.
	RelTypeSelf RelType = "self"

	// Refers to a parent document in a hierarchy of documents.
	RelTypeUp RelType = "up"

	// Refers to the previous resource in an ordered series of resources.
	RelTypePrevious RelType = "previous"

	// Refers to the next resource in a ordered series of resources.
	RelTypeNext RelType = "next"

	// An IRI that refers to the furthest preceding resource in a series of resources.
	RelTypeFirst RelType = "first"

	// An IRI that refers to the furthest following resource in a series of resources.
	RelTypeLast RelType = "last"

	// Refers to an index.
	RelTypeIndex RelType = "index"

	// Refers to a resource offering help (more information, links to other sources information, etc.).
	RelTypeHelp RelType = "help"

	// Refers to a resource that can be used to edit the link's context.
	RelTypeEdit RelType = "edit"

	// Refers to a custom recommendations relation.
	RelTypeRecommendation RelType = "/rels/recommendations"
)

// WriteJSON implements a helper for writing HTTP status and encoding data.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode response data", "err", err)
	}
}

func SetDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func SetAuthHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func SetUnauthorizedHeaders(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
}

func Success(w http.ResponseWriter, status int, data any, links Links) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, SuccessResponse{ResponseStatusSuccess, data, links})
}

func Error(w http.ResponseWriter, status int, message string) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, ErrorResponse{Status: ResponseStatusError, Message: message})
}

func Fail(w http.ResponseWriter, status int, data any) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, FailResponse{Status: ResponseStatusFail, Data: data})
}

func AuthAccessToken(w http.ResponseWriter, accessToken AccessToken) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	WriteJSON(w, http.StatusOK, accessToken)
}

func AuthTokenPair(w http.ResponseWriter, tokenPair TokenPair) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	WriteJSON(w, http.StatusOK, tokenPair)
}

func AuthError(w http.ResponseWriter, code AuthErrorCode, description string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	WriteJSON(w, http.StatusBadRequest, AuthErrorResponse{Error: code, ErrorDescription: description})
}

func Unauthorized(w http.ResponseWriter, code AuthErrorCode, description string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	SetUnauthorizedHeaders(w)
	WriteJSON(w, http.StatusUnauthorized, AuthErrorResponse{Error: code, ErrorDescription: description})
}

func DecodeRequestBody[T any](r *http.Request) (data *T, err error) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err = dec.Decode(data)
	if err != nil {
		return nil, ErrFailedDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}
	return data, err
}

func Health(w http.ResponseWriter, r *http.Request) {
	Success(w, http.StatusOK, nil, nil)
}

func GetPosition(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links := Links{}

		position, err := s.GetPosition(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			links[RelTypeUp] = Link{Href: RoutePositions}

			Fail(w, http.StatusNotFound, FailData{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error("query failed", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		links[RelTypeSelf] = Link{Href: r.URL.Path}
		links[RelTypeUp] = Link{Href: RoutePositions}

		Success(w, http.StatusOK, position, links)
	}
}

func GetCandidate(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links := Links{}

		candidate, err := s.GetCandidate(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			links[RelTypeUp] = Link{Href: RoutePositions}

			Fail(w, http.StatusNotFound, FailData{"id": "candidate not found"})
			return
		}
		if err != nil {
			slog.Error("query failed", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		links[RelTypeSelf] = Link{Href: r.URL.Path}
		links[RelTypeUp] = Link{Href: RouteCandidates}

		Success(w, http.StatusOK, candidate, links)
	}
}

func PublicKeys(v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicKey := v.GetPublicKey()

		keys := []PasetoKey{
			{
				Version: 4,
				Kid:     1,
				Key:     publicKey,
			},
		}
		Success(w, http.StatusOK, PublicPasetoKeys{Keys: keys}, nil)
	}
}

func CreateAccessToken(s Store, v Vault) http.HandlerFunc {
	type RequestBodyCreateToken struct {
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBodyCreateToken](r)
		if err != nil {
			AuthError(w, AuthInvalidRequest, "invalid request body")
			return
		}
		if req.GrantType != "refresh_token" {
			AuthError(w, AuthUnsupportedGrantType, "grant_type must be refresh_token")
			return
		}
		if req.RefreshToken == "" {
			AuthError(w, AuthInvalidGrant, "refresh_token is required")
			return
		}

		claims, err := v.ParseRefreshToken(req.RefreshToken)
		if err != nil {
			slog.Error("refresh token parsing failed", "err", err)
			AuthError(w, AuthInvalidGrant, "invalid refresh token")
			return
		}

		isRefreshTokenRevoked, err := s.ValidateActiveSession(claims.JTI)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				AuthError(w, AuthInvalidGrant, "invalid refresh token")
				return
			}
			slog.Error(
				"db validation failed",
				"err", err,
				"jti", claims.JTI,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		if isRefreshTokenRevoked {
			slog.Warn(
				"revoked token reuse attempt",
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			AuthError(w, AuthInvalidGrant, "invalid refresh token")
			return
		}

		roles, err := s.GetUserRoles(claims.UserID, Provider(claims.Provider))
		if err != nil {
			slog.Error(
				"failed to get roles for the user",
				"err", err,
				"user_id", claims.UserID,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		scope, err := v.GetScopeForRoles(roles)
		if err != nil {
			slog.Error(
				"failed to get scope for the user with the following roles",
				"err", err,
				"user_id", claims.UserID,
				"roles", roles,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		accessToken, err := v.CreateAccessToken(claims.UserID, claims.Provider, scope)
		if err != nil {
			slog.Error(
				"token creation failed",
				"err", err,
				"user_id", claims.UserID,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		AuthAccessToken(w, *accessToken)
	}
}

func Login(v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.PathValue("provider")

		state, err := v.CreateStateToken()
		if err != nil {
			slog.Error("generation of state token failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		tenMinutes := int((10 * time.Minute).Seconds())

		// State token is used to prevent CSRF attacks and is stored in a secure, HttpOnly cookie with a short expiration time
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_state",
			Value:    state,
			Path:     "/",
			MaxAge:   tenMinutes,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		// PKCE verifier is used to prevent authorization code interception attacks
		verifier := oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_verifier",
			Value:    verifier,
			Path:     "/",
			MaxAge:   tenMinutes,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		url, err := v.CreateAuthCodeURL(state, verifier, provider)
		if errors.Is(err, ErrInvalidProvider) {
			AuthError(w, AuthInvalidRequest, "invalid provider")
			return
		}
		if err != nil {
			slog.Error("generation of auth code url failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func RedirectProvider(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.PathValue("provider")

		switch provider {
		case "google":
			GoogleCallback(s, v, w, r)
			return
		case "apple":
			AppleCallback(s, v, w, r)
			return
		default:
			AuthError(w, AuthInvalidRequest, "invalid provider")
			return
		}
	}
}

func GoogleCallback(s Store, v Vault, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		AuthError(w, AuthInvalidRequest, "invalid state")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !v.ValidateAndDeleteStateToken(stateQuery) {
		AuthError(w, AuthInvalidRequest, "invalid state")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		AuthError(w, AuthInvalidRequest, "invalid oauth_verifier")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		AuthError(w, AuthInvalidRequest, "authorization provider error")
		return
	}

	DeleteCookies(w, []string{"oauth_state", "oauth_verifier"})

	code := r.URL.Query().Get("code")
	if code == "" {
		AuthError(w, AuthInvalidRequest, "invalid code")
		return
	}

	rawIDToken, err := v.ExchangeGoogleCodeForIDToken(ctx, code, verifierCookie)
	if errors.Is(err, ErrIDTokenRequired) {
		AuthError(w, AuthInvalidRequest, "id_token is required")
		return
	}
	if err != nil {
		slog.Error("oauth token exchange failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseGoogleIDToken(ctx, rawIDToken)
	if errors.Is(err, ErrInvalidIDToken) {
		AuthError(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedParseClaims) {
		AuthError(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if errors.Is(err, ErrEmailNotVerified) {
		AuthError(w, AuthInvalidRequest, "email not verified")
		return
	}
	if err != nil {
		slog.Error("id_token verification failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	FinishAuthFlow(s, v, w, *user)
}

func AppleCallback(s Store, v Vault, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		AuthError(w, AuthInvalidRequest, "invalid state")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !v.ValidateAndDeleteStateToken(stateQuery) {
		AuthError(w, AuthInvalidRequest, "invalid state")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		AuthError(w, AuthInvalidRequest, "invalid oauth_verifier")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		AuthError(w, AuthInvalidRequest, "authorization provider error")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		AuthError(w, AuthInvalidRequest, "invalid code")
		return
	}

	rawIDToken, err := v.ExchangeAppleCodeForIDToken(ctx, code, verifierCookie)
	if errors.Is(err, ErrIDTokenRequired) {
		AuthError(w, AuthInvalidRequest, "id_token is required")
		return
	}
	if err != nil {
		slog.Error("oauth token exchange failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseAppleIDToken(ctx, rawIDToken, r.FormValue("user"))
	if errors.Is(err, ErrInvalidIDToken) {
		AuthError(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedParseClaims) {
		AuthError(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if err != nil {
		slog.Error("id_token verification failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	FinishAuthFlow(s, v, w, *user)
}

func FinishAuthFlow(s Store, v Vault, w http.ResponseWriter, user User) {
	userID, roles, err := s.GetUserByProvider(user.Provider, user.ProviderUserID)

	if errors.Is(err, ErrUserNotFound) {
		user.UserName, err = GenerateUsername()
		if err != nil {
			slog.Error("username generation failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		userID, err := s.CreateUser(user)
		if err != nil {
			slog.Error("query failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		CreateOnboardingToken(v, w, userID, user.Provider.Raw())
		return
	}
	if errors.Is(err, ErrUserNoRole) {
		CreateOnboardingToken(v, w, userID, user.Provider.Raw())
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	CreateTokenPair(s, v, w, userID, user.Provider.Raw(), roles)
}

func CreateOnboardingToken(v Vault, w http.ResponseWriter, userID string, provider string) {
	accessToken, err := v.CreateAccessToken(userID, provider, ScopeType{ScopeValueTypeOnboarding})
	if err != nil {
		slog.Error("failed to create access token", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	AuthAccessToken(w, *accessToken)
}

func CreateTokenPair(s Store, v Vault, w http.ResponseWriter, userID string, provider string, roles []string) {
	scope, err := v.GetScopeForRoles(roles)
	if err != nil {
		slog.Error("failed to get scope for roles", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	jti, err := s.CreateRefreshToken(userID, time.Now().UTC().Add(DefaultRefreshTokenExpiration.Abs()))
	if err != nil {
		slog.Error("query failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	tokenPair, err := v.CreateTokenPair(userID, provider, jti, scope)
	if err != nil {
		slog.Error("failed to create token pair", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	AuthTokenPair(w, *tokenPair)
}

func DeleteCookies(w http.ResponseWriter, names []string) {
	for _, name := range names {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// GenerateUsername creates username with a cryptographically random suffix
func GenerateUsername() (string, error) {
	randInt := func(n int) int {
		if n <= 0 {
			return 0
		}
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		return int(b[0]) % n
	}
	adj := adjectives[randInt(len(adjectives))]
	noun := nouns[randInt(len(nouns))]

	suffix := make([]byte, 2)
	_, err := rand.Read(suffix)
	if err != nil {
		return "", ErrFailedGenerateUsernameSuffix
	}

	username := fmt.Sprintf("%s_%s%s", adj, noun, hex.EncodeToString(suffix))
	username = strings.ToLower(username)

	return username, nil
}

func linksPages() {
}
