// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

func Health(w http.ResponseWriter, r *http.Request) {
	link := Link{
		Rel:    RelTypeSelf,
		Name:   "health",
		Method: MethodGet,
		Href:   RouteHealth.Href,
	}
	Success(w, http.StatusOK, nil, link)
}

func GetPosition(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse, err := s.GetPosition(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			Fail(w, http.StatusNotFound, map[string]string{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusOK, jsonResponse)
	}
}

func GetPositions(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := ValidateLimit(r.URL.Query().Get("limit"))
		if err != nil {
			Fail(w, http.StatusBadRequest, map[string]string{"limit": "invalid limit"})
			return
		}

		offset, err := ValidateOffset(r.URL.Query().Get("offset"))
		if err != nil {
			Fail(w, http.StatusBadRequest, map[string]string{"limit": "invalid offset"})
			return
		}

		jsonResponse, err := s.GetPositions(Paginator{Limit: limit, Offset: offset})
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusOK, jsonResponse)
	}
}

func GetCandidate(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse, err := s.GetCandidate(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			Fail(w, http.StatusNotFound, map[string]string{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusOK, jsonResponse)
	}
}

func GetCandidates(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := ValidateLimit(r.URL.Query().Get("limit"))
		if err != nil {
			Fail(w, http.StatusBadRequest, map[string]string{"limit": "invalid limit"})
			return
		}

		offset, err := ValidateOffset(r.URL.Query().Get("offset"))
		if err != nil {
			Fail(w, http.StatusBadRequest, map[string]string{"offset": "invalid offset"})
			return
		}

		jsonResponse, err := s.GetCandidates(Paginator{Limit: limit, Offset: offset})
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusOK, jsonResponse)
	}
}

func CreateCandidateReaction(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBodyCreateCandidateReaction](r)
		if err != nil {
			Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		if !req.ReactionType.IsValid() {
			Fail(w, http.StatusBadRequest, map[string]string{"reaction_type": "invalid reaction type"})
			return
		}

		if err := s.CreateCandidateReaction(
			CandidateReaction{
				CandidateID:  r.PathValue("id"),
				PositionID:   req.PositionID,
				ReactionType: req.ReactionType,
			},
		); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusCreated, nil)
	}
}

func CreateRecruiterReaction(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBodyCreateRecruiterReaction](r)
		if err != nil {
			Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		if !req.ReactionType.IsValid() {
			Fail(w, http.StatusBadRequest, map[string]string{"reaction_type": "invalid reaction type"})
			return
		}

		if err := s.CreateRecruiterReaction(
			RecruiterReaction{
				RecruiterID:  r.PathValue("id"),
				CandidateID:  req.CandidateID,
				PositionID:   req.PositionID,
				ReactionType: req.ReactionType,
			},
		); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusCreated, nil)
	}
}

func CreateCandidate(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBodyCreateCandidate](r)
		if err != nil {
			Error(w, http.StatusBadRequest, "invalid request")
			return
		}
		about, err := ValidateAbout(req.About)
		if err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		claims, ok := GetClaims(r.Context())
		if !ok {
			slog.Error(
				"could not retrieve context",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
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
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		CreateTokenPair(s, v, w, claims.UserID, claims.Provider, []string{"candidate"})
	}
}

func CreateMatch(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBodyCreateMatch](r)
		if err != nil {
			Error(w, http.StatusBadRequest, "invalid request")
			return
		}

		if err := s.CreateMatch(Match{CandidateID: req.CandidateID, PositionID: req.PositionID}); err != nil {
			slog.Error(
				"query failed",
				"err", err,
			)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusCreated, nil)
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
		Success(w, http.StatusOK, PublicPasetoKeys{Keys: keys})
	}
}

func CreateAccessToken(s Store, v Vault) http.HandlerFunc {
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
			slog.Error(
				"refresh token parsing failed",
				"err", err,
			)
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

		accessToken, err := v.CreateAccessToken(claims.UserID, claims.Provider, "")
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
			slog.Error(
				"generation of state token failed",
				"err", err,
			)
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
			slog.Error(
				"generation of auth code url failed",
				"err", err,
			)
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
		slog.Error(
			"oauth token exchange failed",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseGoogleIDToken(ctx, rawIDToken)
	if errors.Is(err, ErrInvalidIDToken) {
		AuthError(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedToParseClaims) {
		AuthError(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if errors.Is(err, ErrEmailNotVerified) {
		AuthError(w, AuthInvalidRequest, "email not verified")
		return
	}
	if err != nil {
		slog.Error(
			"id_token verification failed",
			"err", err,
		)
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
		slog.Error(
			"oauth token exchange failed",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	user, err := v.VerifyAndParseAppleIDToken(ctx, rawIDToken, r.FormValue("user"))
	if errors.Is(err, ErrInvalidIDToken) {
		AuthError(w, AuthInvalidRequest, "invalid id_token")
		return
	}
	if errors.Is(err, ErrFailedToParseClaims) {
		AuthError(w, AuthInvalidRequest, "failed to parse claims")
		return
	}
	if err != nil {
		slog.Error(
			"id_token verification failed",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
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
			AuthError(w, AuthInvalidRequest, "internal server error")
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
		AuthError(w, AuthInvalidRequest, "internal server error")
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
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	AuthAccessToken(w, *accessToken)
}

func CreateTokenPair(s Store, v Vault, w http.ResponseWriter, userID string, provider string, roles []string) {
	scope, err := v.GetScopeForRoles(roles)
	if err != nil {
		slog.Error(
			"failed to get scope for roles",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	jti, err := s.CreateRefreshToken(userID, time.Now().UTC().Add(RefreshTokenExpiration.Abs()))
	if err != nil {
		slog.Error(
			"query failed",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	tokenPair, err := v.CreateTokenPair(userID, provider, jti, scope)
	if err != nil {
		slog.Error(
			"failed to create token pair",
			"err", err,
		)
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
