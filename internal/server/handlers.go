// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/akvachan/hirevec-backend/internal/auth"
	"github.com/akvachan/hirevec-backend/internal/db"

	"golang.org/x/oauth2"
)

type createCandidateReactionBody struct {
	PositionID   uint32          `json:"position_id"`
	ReactionType db.ReactionType `json:"reaction_type"`
}

type createRecruiterReactionBody struct {
	PositionID   uint32          `json:"position_id"`
	CandidateID  uint32          `json:"candidate_id"`
	ReactionType db.ReactionType `json:"reaction_type"`
}

type createMatchBody struct {
	PositionID  uint32 `json:"position_id"`
	CandidateID uint32 `json:"candidate_id"`
}

type createTokenBody struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type successResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
}

type errorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type failResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type authErrorCode string

const (
	invalidRequest       authErrorCode = "invalid_request"
	invalidGrant         authErrorCode = "invalid_grant"
	unsupportedGrantType authErrorCode = "unsupported_grant_type"
)

type authErrorResponse struct {
	Error            authErrorCode `json:"error"`
	ErrorDescription string        `json:"error_description,omitempty"`
	ErrorURI         string        `json:"error_uri,omitempty"`
}

func WriteSuccessResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(successResponse{Status: "success", Data: data})
}

func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Status: "error", Message: message})
}

func WriteFailResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(failResponse{Status: "fail", Data: data})
}

func WriteAuthSuccessResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func WriteAuthErrorResponse(w http.ResponseWriter, errorCode authErrorCode, errorDescription string, errorURI string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(authErrorResponse{
		Error:            errorCode,
		ErrorDescription: errorDescription,
		ErrorURI:         errorURI,
	})
}

func decodeRequestBody[T any](r *http.Request) (*T, error) {
	var data T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&data)
	if err != nil {
		return nil, ErrCouldNotDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}
	return &data, err
}

func GetPosition(w http.ResponseWriter, r *http.Request) {
	id, err := ValidateSerialID(r.PathValue("id"))
	if err != nil {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
		return
	}

	jsonResponse, err := db.GetPosition(id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "position not found"})
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusOK, jsonResponse)
}

func GetPositions(w http.ResponseWriter, r *http.Request) {
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

	jsonResponse, err := db.GetPositions(db.Paginator{Limit: limit, Offset: offset})
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusOK, jsonResponse)
}

func GetCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := ValidateSerialID(r.PathValue("id"))
	if err != nil {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
		return
	}

	jsonResponse, err := db.GetCandidate(id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteFailResponse(w, http.StatusNotFound, map[string]string{"id": "position not found"})
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusOK, jsonResponse)
}

func GetCandidates(w http.ResponseWriter, r *http.Request) {
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

	jsonResponse, err := db.GetCandidates(db.Paginator{Limit: limit, Offset: offset})
	if err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusOK, jsonResponse)
}

func CreateCandidateReaction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if id == 0 {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
		return
	}
	if err != nil {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
		return
	}

	req, err := decodeRequestBody[createCandidateReactionBody](r)
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

	if err := db.CreateCandidateReaction(
		db.CandidateReaction{
			CandidateID:  uint32(id),
			PositionID:   req.PositionID,
			ReactionType: req.ReactionType,
		},
	); err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusCreated, nil)
}

func CreateRecruiterReaction(w http.ResponseWriter, r *http.Request) {
	rid, err := ValidateSerialID(r.PathValue("id"))
	if err != nil {
		WriteFailResponse(w, http.StatusBadRequest, map[string]string{"id": "invalid id"})
		return
	}

	req, err := decodeRequestBody[createRecruiterReactionBody](r)
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

	if err := db.CreateRecruiterReaction(
		db.RecruiterReaction{
			RecruiterID:  rid,
			CandidateID:  req.CandidateID,
			PositionID:   req.PositionID,
			ReactionType: req.ReactionType,
		},
	); err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusCreated, nil)
}

func CreateMatch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequestBody[createMatchBody](r)
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

	if err := db.CreateMatch(db.Match{CandidateID: req.CandidateID, PositionID: req.PositionID}); err != nil {
		slog.Error("query failed", "err", err)
		WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	WriteSuccessResponse(w, http.StatusCreated, nil)
}

func GetPublicKeys(w http.ResponseWriter, _ *http.Request) {
	publicKey := auth.GetPublicKey()

	keys := []auth.PasetoKey{
		{
			Version: 4,
			Kid:     1,
			Key:     publicKey,
		},
	}
	WriteSuccessResponse(w, http.StatusOK, auth.PublicPasetoKeys{Keys: keys})
}

func TokenEndpoint(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequestBody[createTokenBody](r)
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "", "")
		return
	}
	if req.GrantType != "refresh_token" {
		WriteAuthErrorResponse(w, unsupportedGrantType, "grant_type must be refresh_token", "")
		return
	}
	if req.RefreshToken == "" {
		WriteAuthErrorResponse(w, invalidGrant, "refresh_token is required", "")
		return
	}

	claims, err := auth.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		slog.Error("refresh token parsing failed", "err", err)
		WriteAuthErrorResponse(w, invalidGrant, "invalid refresh token", "")
		return
	}

	isRefreshTokenRevoked, err := db.ValidateActiveSession(claims.JTI)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteAuthErrorResponse(w, invalidGrant, "invalid refresh token", "")
			return
		}
		slog.Error("db validation failed", "err", err, "jti", claims.JTI)
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}
	if isRefreshTokenRevoked {
		slog.Warn("revoked token reuse attempt", "jti", claims.JTI, "user_id", claims.UserID, "ip", r.RemoteAddr)
		WriteAuthErrorResponse(w, invalidGrant, "invalid refresh token", "")
		return
	}

	accessToken, err := auth.CreateAccessToken(claims.UserID, claims.Provider, "")
	if err != nil {
		slog.Error("token creation failed", "err", err, "user_id", claims.UserID)
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}

	WriteAuthSuccessResponse(w, accessToken)
}

func Login(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	var config *oauth2.Config

	switch provider {
	case "google":
		config = auth.GoogleOIDC.OAuth2Config
	case "apple":
		config = auth.AppleOIDC.OAuth2Config
	default:
		WriteAuthErrorResponse(w, invalidRequest, "invalid provider", "")
		return
	}

	state, err := auth.GenerateStateToken()
	if err != nil {
		slog.Error("generation of state token failed", "err", err)
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}

	tenMinutes := int((10 * time.Minute).Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   tenMinutes,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

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

	url := config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func RedirectionEndpoint(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")

	switch provider {
	case "google":
		GoogleCallback(w, r)
	case "apple":
		AppleCallback(w, r)
	default:
		WriteAuthErrorResponse(w, invalidRequest, "invalid provider", "")
	}
}

func GoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "invalid state", "")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !auth.ValidateAndDeleteState(stateQuery) {
		WriteAuthErrorResponse(w, invalidRequest, "invalid state", "")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "invalid oauth_verifier", "")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		WriteAuthErrorResponse(w, invalidRequest, "authorization provider error", "")
		return
	}

	deleteCookies := -1
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   deleteCookies,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    "",
		Path:     "/",
		MaxAge:   deleteCookies,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		WriteAuthErrorResponse(w, invalidRequest, "invalid code", "")
		return
	}

	tok, err := auth.GoogleOIDC.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		slog.Warn("oauth token exchange failed", "err", err)
		WriteAuthErrorResponse(w, invalidRequest, "oauth token exchange failed", "")
		return
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		WriteAuthErrorResponse(w, invalidRequest, "id_token is required", "")
		return
	}

	idToken, err := auth.GoogleOIDC.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("id_token verification failed", "err", err)
		WriteAuthErrorResponse(w, invalidRequest, "invalid id_token", "")
		return
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
		WriteAuthErrorResponse(w, invalidRequest, "failed to parse claims", "")
		return
	}

	if !claims.EmailVerified {
		WriteAuthErrorResponse(w, invalidRequest, "email not verified", "")
		return
	}

	user := db.User{
		Provider:       db.Google,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FirstName:      claims.GivenName,
		LastName:       claims.FamilyName,
		FullName:       claims.Name,
	}
	finishOAuthFlow(w, user)
}

func AppleCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		WriteAuthErrorResponse(w, invalidRequest, "invalid state", "")
		return
	}
	stateQuery := r.URL.Query().Get("state")
	if stateCookie.Value != stateQuery || !auth.ValidateAndDeleteState(stateQuery) {
		WriteAuthErrorResponse(w, invalidRequest, "invalid state", "")
		return
	}

	verifierCookie, err := r.Cookie("oauth_verifier")
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "invalid oauth_verifier", "")
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		WriteAuthErrorResponse(w, invalidRequest, "authorization provider error", "")
		return
	}

	deleteCookies := -1
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Path:     "/",
		MaxAge:   deleteCookies,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Path:     "/",
		MaxAge:   deleteCookies,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		WriteAuthErrorResponse(w, invalidRequest, "invalid code", "")
		return
	}

	oauth2Token, err := auth.AppleOIDC.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(verifierCookie.Value),
	)
	if err != nil {
		slog.Warn("oauth token exchange failed", "err", err)
		WriteAuthErrorResponse(w, invalidRequest, "oauth token exchange failed", "")
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		WriteAuthErrorResponse(w, invalidRequest, "id_token is required", "")
		return
	}

	idToken, err := auth.AppleOIDC.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("id_token verification failed", "err", err)
		WriteAuthErrorResponse(w, invalidRequest, "invalid id_token", "")
		return
	}

	var claims struct {
		Sub            string `json:"sub"`
		Email          string `json:"email"`
		EmailVerified  string `json:"email_verified"`
		IsPrivateEmail string `json:"is_private_email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "failed to parse claims", "")
		return
	}

	var firstName, lastName, fullName string
	userJSON := r.FormValue("user")
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

	user := db.User{
		Provider:       db.Apple,
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		FirstName:      firstName,
		LastName:       lastName,
		FullName:       fullName,
	}
	finishOAuthFlow(w, user)
}

func finishOAuthFlow(w http.ResponseWriter, user db.User) {
	userProvider := user.Provider
	isValidProvider := userProvider.IsValid()
	if !isValidProvider {
		WriteAuthErrorResponse(w, invalidRequest, "invalid provider", "")
	}

	var userID uint32
	userID, err := db.GetUserByProvider(userProvider.Raw(), user.ProviderUserID)
	if errors.Is(err, sql.ErrNoRows) {
		userID, err = db.CreateUser(user)
		if err != nil {
			WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
			return
		}
	}
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}

	jti, err := db.CreateRefreshToken(userID, time.Now().UTC().Add(auth.RefreshTokenExpiration))
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}

	tokenPair, err := auth.CreateTokenPair(userID, userProvider.Raw(), jti, "")
	if err != nil {
		WriteAuthErrorResponse(w, invalidRequest, "internal server error", "")
		return
	}

	WriteAuthSuccessResponse(w, tokenPair)
}
