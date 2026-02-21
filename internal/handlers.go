// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

type CreateCandidateReactionBody struct {
	PositionID   uint32       `json:"position_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

type CreateCandidateBody struct {
	About string `json:"about"`
}

type CreateRecruiterReactionBody struct {
	PositionID   uint32       `json:"position_id"`
	CandidateID  uint32       `json:"candidate_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

type CreateMatchBody struct {
	PositionID  uint32 `json:"position_id"`
	CandidateID uint32 `json:"candidate_id"`
}

type CreateTokenBody struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type SuccessResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
}

type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type FailResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type AuthErrorCode string

const (
	AuthInvalidRequest       AuthErrorCode = "invalid_request"
	AuthInvalidGrant         AuthErrorCode = "invalid_grant"
	AuthInvalidClient        AuthErrorCode = "invalid_client"
	AuthUnsupportedGrantType AuthErrorCode = "unsupported_grant_type"
)

type AuthErrorResponse struct {
	Error            AuthErrorCode `json:"error"`
	ErrorDescription string        `json:"error_description,omitempty"`
	ErrorURI         string        `json:"error_uri,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data any, headers map[string]string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error(
			"could not encode response data",
			"err", err,
		)
	}
}

func WriteSuccessResponse(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, SuccessResponse{Status: "success", Data: data}, nil)
}

func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Status: "error", Message: message}, nil)
}

func WriteFailResponse(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, FailResponse{Status: "fail", Data: data}, nil)
}

func WriteAuthSuccessResponse(w http.ResponseWriter, data any) {
	WriteJSON(
		w,
		http.StatusOK,
		data,
		map[string]string{
			"Cache-Control": "no-store",
			"Pragma":        "no-cache",
		},
	)
}

func WriteAuthErrorResponse(w http.ResponseWriter, code AuthErrorCode, description string) {
	WriteJSON(
		w,
		http.StatusBadRequest,
		AuthErrorResponse{
			Error:            code,
			ErrorDescription: description,
		},
		map[string]string{
			"Cache-Control": "no-store",
			"Pragma":        "no-cache",
		},
	)
}

func WriteUnauthorizedResponse(w http.ResponseWriter, code AuthErrorCode, description string) {
	WriteJSON(
		w,
		http.StatusUnauthorized,
		AuthErrorResponse{
			Error:            code,
			ErrorDescription: description,
		},
		map[string]string{
			"WWW-Authenticate": "Bearer",
		},
	)
}

func DecodeRequestBody[T any](r *http.Request) (data *T, err error) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err = dec.Decode(data)
	if err != nil {
		return nil, ErrFailedToDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}
	return data, err
}

func GetPosition(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := ValidateSerialID(r.PathValue("id"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
			return
		}

		jsonResponse, err := s.GetPosition(id)
		if errors.Is(err, sql.ErrNoRows) {
			WriteFailResponse(w, http.StatusNotFound, map[string]string{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusOK, jsonResponse)
	}
}

func GetPositions(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := ValidateLimit(r.URL.Query().Get("limit"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"limit": "invalid limit"})
			return
		}

		offset, err := ValidateOffset(r.URL.Query().Get("offset"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"limit": "invalid offset"})
			return
		}

		jsonResponse, err := s.GetPositions(Paginator{Limit: limit, Offset: offset})
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusOK, jsonResponse)
	}
}

func GetCandidate(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := ValidateSerialID(r.PathValue("id"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
			return
		}

		jsonResponse, err := s.GetCandidate(id)
		if errors.Is(err, sql.ErrNoRows) {
			WriteFailResponse(w, http.StatusNotFound, map[string]string{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusOK, jsonResponse)
	}
}

func GetCandidates(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := ValidateLimit(r.URL.Query().Get("limit"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"limit": "invalid limit"})
			return
		}

		offset, err := ValidateOffset(r.URL.Query().Get("offset"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"offset": "invalid offset"})
			return
		}

		jsonResponse, err := s.GetCandidates(Paginator{Limit: limit, Offset: offset})
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusOK, jsonResponse)
	}
}

func CreateCandidateReaction(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, err := ValidateSerialID(r.PathValue("id"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
			return
		}

		req, err := DecodeRequestBody[CreateCandidateReactionBody](r)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request")
			return
		}
		if req.PositionID == 0 {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"position_id": "invalid position id"})
			return
		}
		if !req.ReactionType.IsValid() {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"reaction_type": "invalid reaction type"})
			return
		}

		if err := s.CreateCandidateReaction(
			CandidateReaction{
				CandidateID:  uint32(cid),
				PositionID:   req.PositionID,
				ReactionType: req.ReactionType,
			},
		); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusCreated, nil)
	}
}

func CreateRecruiterReaction(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid, err := ValidateSerialID(r.PathValue("id"))
		if err != nil {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
			return
		}

		req, err := DecodeRequestBody[CreateRecruiterReactionBody](r)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request")
			return
		}
		if req.PositionID == 0 {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"position_id": "invalid position id"})
			return
		}
		if req.CandidateID == 0 {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"candidate_id": "invalid candidate id"})
			return
		}
		if !req.ReactionType.IsValid() {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"reaction_type": "invalid reaction type"})
			return
		}

		if err := s.CreateRecruiterReaction(
			RecruiterReaction{
				RecruiterID:  rid,
				CandidateID:  req.CandidateID,
				PositionID:   req.PositionID,
				ReactionType: req.ReactionType,
			},
		); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusCreated, nil)
	}
}

func CreateCandidate(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[CreateCandidateBody](r)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request")
			return
		}
		about, err := ValidateAbout(req.About)
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		claims, ok := GetClaims(r.Context())
		if !ok {
			slog.Error(
				"could not retrieve context",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		if err := s.CreateCandidate(
			Candidate{
				UserID: claims.UserID,
				About:  about,
			}); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		CreateTokenPair(s, v, w, claims.UserID, claims.Provider, []string{"candidate"})
	}
}

func CreateMatch(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[CreateMatchBody](r)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, "invalid request")
			return
		}
		if req.PositionID == 0 {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"position_id": "position_id must be non-zero"})
			return
		}
		if req.CandidateID == 0 {
			WriteFailResponse(w, http.StatusBadRequest, map[string]string{"candidate_id": "candidate_id must be non-zero"})
			return
		}

		if err := s.CreateMatch(Match{CandidateID: req.CandidateID, PositionID: req.PositionID}); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		WriteSuccessResponse(w, http.StatusCreated, nil)
	}
}

func GetPublicKeys(v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicKey := v.GetPublicKey()

		keys := []PasetoKey{
			{
				Version: 4,
				Kid:     1,
				Key:     publicKey,
			},
		}
		WriteSuccessResponse(w, http.StatusOK, PublicPasetoKeys{Keys: keys})
	}
}

func CreateAccessToken(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[CreateTokenBody](r)
		if err != nil {
			WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid request body")
			return
		}
		if req.GrantType != "refresh_token" {
			WriteAuthErrorResponse(w, AuthUnsupportedGrantType, "grant_type must be refresh_token")
			return
		}
		if req.RefreshToken == "" {
			WriteAuthErrorResponse(w, AuthInvalidGrant, "refresh_token is required")
			return
		}

		claims, err := v.ParseRefreshToken(req.RefreshToken)
		if err != nil {
			slog.Error(
				"refresh token parsing failed",
				"err", err,
			)
			WriteAuthErrorResponse(w, AuthInvalidGrant, "invalid refresh token")
			return
		}

		isRefreshTokenRevoked, err := s.ValidateActiveSession(claims.JTI)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				WriteAuthErrorResponse(w, AuthInvalidGrant, "invalid refresh token")
				return
			}
			slog.Error(
				"db validation failed",
				"err", err,
				"jti", claims.JTI,
			)
			WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
			return
		}
		if isRefreshTokenRevoked {
			slog.Warn(
				"revoked token reuse attempt",
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			WriteAuthErrorResponse(w, AuthInvalidGrant, "invalid refresh token")
			return
		}

		accessToken, err := v.CreateAccessToken(claims.UserID, claims.Provider, "")
		if err != nil {
			slog.Error(
				"token creation failed",
				"err", err,
				"user_id", claims.UserID,
			)
			WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
			return
		}

		WriteAuthSuccessResponse(w, accessToken)
	}
}

func Login(v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.PathValue("provider")

		state, err := v.CreateStateToken()
		if err != nil {
			slog.Error(
				"generation of state token failed",
				"err", err,
			)
			WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
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
			WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid provider")
			return
		}
		if err != nil {
			slog.Error(
				"generation of auth code url failed",
				"err", err,
			)
			WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
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
			WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid provider")
			return
		}
	}
}

func GoogleCallback(s Store, v Vault, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid state")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !v.ValidateAndDeleteStateToken(stateQuery) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid state")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid oauth_verifier")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "authorization provider error")
		return
	}

	DeleteCookies(w, []string{"oauth_state", "oauth_verifier"})

	code := r.URL.Query().Get("code")
	if code == "" {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid code")
		return
	}

	rawIDToken, err := v.ExchangeGoogleCodeForIDToken(ctx, code, verifierCookie)
	if errors.Is(err, ErrIDTokenRequired) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "id_token is required")
		return
	}
	if err != nil {
		slog.Error(
			"oauth token exchange failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseGoogleIDToken(ctx, rawIDToken)
	if errors.Is(err, ErrInvalidIDToken) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedToParseClaims) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if errors.Is(err, ErrEmailNotVerified) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "email not verified")
		return
	}
	if err != nil {
		slog.Error(
			"id_token verification failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	FinishAuthFlow(s, v, w, *user)
}

func AppleCallback(s Store, v Vault, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid state")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !v.ValidateAndDeleteStateToken(stateQuery) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid state")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid oauth_verifier")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "authorization provider error")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid code")
		return
	}

	rawIDToken, err := v.ExchangeAppleCodeForIDToken(ctx, code, verifierCookie)
	if errors.Is(err, ErrIDTokenRequired) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "id_token is required")
		return
	}
	if err != nil {
		slog.Error(
			"oauth token exchange failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseAppleIDToken(ctx, rawIDToken, r.FormValue("user"))
	if errors.Is(err, ErrInvalidIDToken) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedToParseClaims) {
		WriteAuthErrorResponse(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if err != nil {
		slog.Error(
			"id_token verification failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	FinishAuthFlow(s, v, w, *user)
}

func FinishAuthFlow(s Store, v Vault, w http.ResponseWriter, user User) {
	userID, roles, err := s.GetUserByProvider(user.Provider, user.ProviderUserID)

	if errors.Is(err, ErrUserDoesNotExist) {
		userID, err := s.CreateUser(user)
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
			return
		}
		CreateOnboardingToken(v, w, userID, user.Provider.Raw())
		return
	}
	if errors.Is(err, ErrUserDoesNotHaveARole) {
		CreateOnboardingToken(v, w, userID, user.Provider.Raw())
		return
	}
	if err != nil {
		slog.Error(
			"query failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	CreateTokenPair(s, v, w, userID, user.Provider.Raw(), roles)
}

func CreateOnboardingToken(v Vault, w http.ResponseWriter, userID string, provider string) {
	accessToken, err := v.CreateAccessToken(userID, provider, "onboarding")
	if err != nil {
		slog.Error(
			"failed to create access token",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}
	WriteAuthSuccessResponse(w, accessToken)
}

func CreateTokenPair(s Store, v Vault, w http.ResponseWriter, userID string, provider string, roles []string) {
	scope, err := v.GetScopeForRoles(roles)
	if err != nil {
		slog.Error(
			"failed to get scope for roles",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	jti, err := s.CreateRefreshToken(userID, time.Now().UTC().Add(RefreshTokenExpiration.Abs()))
	if err != nil {
		slog.Error(
			"query failed",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	tokenPair, err := v.CreateTokenPair(userID, provider, jti, scope)
	if err != nil {
		slog.Error(
			"failed to create token pair",
			"err", err,
		)
		WriteAuthErrorResponse(w, AuthInvalidRequest, "internal server error")
		return
	}

	WriteAuthSuccessResponse(w, tokenPair)
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
