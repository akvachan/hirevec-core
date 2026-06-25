// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	ErrTextForbiddenChars   = errors.New("text contains forbidden characters")
	ErrTextTooLong          = errors.New("text too long")
	ErrTextTooShort         = errors.New("text too short")
	ErrEmailNotVerified     = errors.New("email not verified")
	ErrExtraDataDecoded     = errors.New("extra data decoded")
	ErrFailedShutdownServer = errors.New("failed to shutdown server")
	ErrPasswordHasNoLower   = errors.New("password has no lower letter")
	ErrPasswordHasNoUpper   = errors.New("password has no upper letter")
	ErrPasswordHasNoDigit   = errors.New("password has no digit")
	ErrPasswordHasNoSpecial = errors.New("password has no special character")
	ErrPositionTitleHasURL  = errors.New("position title must not contain URLs")
)

type APIConfig struct {
	ServerBaseURL               string
	RequestReadTimeout          time.Duration
	RequestWriteTimeout         time.Duration
	GracePeriod                 time.Duration
	UseGoogleSSO                bool
	UseAppleSSO                 bool
	TEIBaseURL                  string
	TEIAPIKey                   string
	EmbeddingsModel             string
	RerankerModel               string
	UseEmbeddings               bool
	UseReranker                 bool
	EmbeddingsJobFrequency      time.Duration
	RecommendationsJobFrequency time.Duration
}

type EmbeddingEntity struct {
	Embedding []float32 `json:"embedding"`
}

type EmbeddingsResponse struct {
	Data []EmbeddingEntity `json:"data"`
}

type EmbeddingsRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

func CreateEmbeddings(aiConfig AIConfig, input []string) ([]EmbeddingEntity, error) {
	requestBody, err := json.Marshal(EmbeddingsRequest{
		Input: input,
		Model: aiConfig.EmbeddingsModel,
	})
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(requestBody)
	request, err := http.NewRequest(http.MethodPost, aiConfig.TEIBaseURL, reader)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+aiConfig.TEIAPIKey)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.Error(
				"failed to close response body",
				"err", err,
			)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"embedding endpoint returned %d",
			response.StatusCode,
		)
	}

	var parsed EmbeddingsResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	if len(parsed.Data) != len(input) {
		return nil, fmt.Errorf(
			"embedding count mismatch (got %d, expected %d)",
			len(parsed.Data),
			len(input),
		)
	}

	return parsed.Data, nil
}

//go:embed assets/*
var EmbeddedAssets embed.FS

var (
	RegexHTMLTag = regexp.MustCompile(`(?s)<[^>]*>`)
	RegexJSTag   = regexp.MustCompile(`(?is)<script.*?>.*?</script>`)
	RegexSQL     = regexp.MustCompile(`(?i)\b(select|insert|update|delete|drop|truncate|alter)\b`)
)

const DefaultEmbeddingsBatchSize = 64

func (a *API) RunEmbeddingsJob(c AIConfig) error {
	// TODO: test this store method, rethink it once more
	ids, texts, err := a.Store.FetchPendingEmbeddingsMetadata(DefaultEmbeddingsBatchSize)
	if err != nil || len(ids) == 0 {
		return err
	}

	// TODO: test this store method, rethink it once more
	batchOut, err := CreateEmbeddings(c, texts)
	if err != nil {
		// TODO: test this store method, rethink it once more
		return a.Store.MarkEmbeddingsStatus(ids, EmbeddingStatusPending)
	}

	tx, err := a.Store.DB.Begin()
	if err != nil {
		return err
	}

	// TODO: test this store method, rethink it once more
	if err := a.Store.UpsertEmbeddingsTx(tx, ids, batchOut); err != nil {
		if err := tx.Rollback(); err != nil {
			slog.Error(
				"failed to rollback transaction",
				"method", "UpsertEmbeddingsTx",
				"err", err,
			)
			return err
		}
		slog.Error(
			"failed to upsert embeddings",
			"method", "UpsertEmbeddingsTx",
			"err", err,
		)
		return err
	}

	// TODO: test this store method, rethink it once more
	if err := a.Store.MarkEmbeddingsStatusTx(tx, ids, EmbeddingStatusDone); err != nil {
		if err := tx.Rollback(); err != nil {
			slog.Error(
				"failed to rollback transaction",
				"method", "MarkEmbeddingsStatusTx",
				"err", err,
			)
			return err
		}
		slog.Error(
			"failed to mark ebeddings status",
			"method", "MarkEmbeddingsStatusTx",
			"err", err,
		)
		return err
	}

	return tx.Commit()
}

type AIConfig struct {
	UseEmbeddings   bool
	UseReranker     bool
	TEIBaseURL      string
	TEIAPIKey       string
	EmbeddingsModel string
	RerankerModel   string
}

func Rerank(c AIConfig, candidateID ULID, positions []ULID) ([]ULID, error) {
	return nil, nil
}

const (
	DefaultCandidatesBatchSize       = 32
	DefaultRecommendationsDailyLimit = 32
	DefaultTopPositions              = 128
)

func (a *API) RunRecommendationsJob(c AIConfig) error {
	candidateIDs, err := a.Store.GetCandidates(
		DefaultCandidatesBatchSize,
		DefaultRecommendationsJobFrequency,
	)
	if err != nil {
		return err
	}
	if len(candidateIDs) == 0 {
		slog.Warn("candidates list is empty")
	}

	for i := range len(candidateIDs) {
		var positionIDs []ULID

		if c.UseEmbeddings {
			positionIDs, err = a.Store.GetPositionsForCandidateViaEmbeddings(candidateIDs[i], DefaultTopPositions)
			if err != nil {
				slog.Error(
					"failed to find similar positions via embeddings",
					"candidateID", candidateIDs[i],
					"err", err,
				)
				continue
			}
		} else {
			positionIDs, err = a.Store.GetPositionsForCandidateViaFTS(candidateIDs[i], DefaultTopPositions)
			if err != nil {
				slog.Error(
					"failed to find similar positions via FTS",
					"candidateID", candidateIDs[i],
					"err", err,
				)
				continue
			}
		}

		if len(positionIDs) == 0 {
			slog.Warn(
				"positions list is empty",
				"candidateID", candidateIDs[i],
			)
			continue
		}

		if c.UseReranker {
			positionIDs, err = Rerank(c, candidateIDs[i], positionIDs)
			if err != nil {
				slog.Error(
					"failed to rerank",
					"candidateID", candidateIDs[i],
					"err", err,
				)
				continue
			}
		}

		limit := min(len(positionIDs), int(DefaultRecommendationsDailyLimit))
		recommendations := make([]Recommendation, limit)
		for i := range limit {
			recommendationID, err := NewRecommendationULID()
			if err != nil {
				return err
			}
			recommendations[i] = Recommendation{
				ID:          recommendationID,
				PositionID:  positionIDs[i],
				CandidateID: candidateIDs[i],
			}
		}

		if err := a.Store.CreateRecommendations(recommendations); err != nil {
			return err
		}
	}

	return nil
}

type API struct {
	Store  Store
	Vault  Vault
	Server http.Server
	Mux    *http.ServeMux
}

const (
	DefaultRequestReadTimeout  = 1000 * time.Millisecond
	DefaultRequestWriteTimeout = 1000 * time.Millisecond
	DefaultGracePeriod         = 5000 * time.Millisecond
)

func NewAPI(ctx context.Context, c APIConfig, s Store, v Vault) *API {
	slog.Debug("initializing server")

	api := API{
		Store: s,
		Vault: v,
	}

	localMux := http.NewServeMux()
	api.Mux = localMux

	api.Server = http.Server{
		Addr:         c.ServerBaseURL,
		ReadTimeout:  c.RequestReadTimeout,
		WriteTimeout: c.RequestWriteTimeout,
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		Handler:      localMux,
	}

	api.RegisterRoutes()

	return &api
}

func (a *API) WaitAndShutdown(ctx context.Context, errCh chan error, gracePeriod time.Duration) error {
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case <-errCh:
		return ErrFailedShutdownServer
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), gracePeriod)
	defer cancel()

	slog.Info(
		"starting graceful shutdown",
		"timeout", gracePeriod,
	)
	if err := a.Server.Shutdown(shutdownCtx); err != nil {
		slog.Error(
			"failed to gracefully shutdown, forcing close",
			"err", err,
		)
		if err := a.Server.Close(); err != nil {
			slog.Error(
				"failed to force close server",
				"err", err,
			)
		}
		return ErrFailedShutdownServer
	}
	slog.Info("HTTP server shutdown complete")

	return nil
}

const (
	DefaultEmbeddingsJobFrequency      = 1 * time.Hour
	DefaultRecommendationsJobFrequency = 24 * time.Hour
)

func RunAPI(ctx context.Context, c APIConfig, s Store, v Vault) error {
	api := NewAPI(ctx, c, s, v)

	slog.Debug(
		"creating listener",
		"addr", api.Server.Addr,
	)
	listener, err := net.Listen("tcp", api.Server.Addr)
	if err != nil {
		return err
	}

	slog.Debug(
		"starting server",
		"addr", api.Server.Addr,
	)
	errCh := make(chan error, 1)
	go func() {
		if err := api.Server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info("server ready", "addr", api.Server.Addr)

	aiConfig := AIConfig{
		UseEmbeddings:   c.UseEmbeddings,
		UseReranker:     c.UseReranker,
		TEIBaseURL:      c.TEIBaseURL,
		TEIAPIKey:       c.TEIAPIKey,
		EmbeddingsModel: c.EmbeddingsModel,
		RerankerModel:   c.RerankerModel,
	}

	if c.UseEmbeddings {
		go func() {
			for range time.Tick(c.EmbeddingsJobFrequency) {
				slog.Info("running embeddings job")
				if err := api.RunEmbeddingsJob(aiConfig); err != nil {
					slog.Error(
						"failed to run embeddings job",
						"err", err,
					)
				}
			}
		}()
	}

	go func() {
		for range time.Tick(c.RecommendationsJobFrequency) {
			slog.Info("running recommendations job")
			if err := api.RunRecommendationsJob(aiConfig); err != nil {
				slog.Error(
					"failed to run recommendations job",
					"err", err,
				)
			}
		}
	}()

	if err := api.WaitAndShutdown(ctx, errCh, c.GracePeriod); err != nil {
		slog.Error(
			"failed to wait and shutdown",
			"err", err,
		)
		return err
	}

	return nil
}

type ContextKey string

const ContextKeyClaims ContextKey = "claims"

type ResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

func MiddlewareChain(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

func (a *API) MiddlewarePanicRecovery(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"recovered from panic",
					"panic", err,
					"method", r.Method,
					"path", r.URL.Path,
				)

				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

func GetClaims(r *http.Request) (AccessTokenClaims, bool) {
	claims, ok := r.Context().Value(ContextKeyClaims).(AccessTokenClaims)
	return claims, ok
}

const (
	DefaultPageSize = 32
	MaxPageSize     = 128
)

func GetPagination(r *http.Request) Page {
	p := Page{
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  DefaultPageSize,
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			p.Limit = min(parsed, MaxPageSize)
		}
	}
	return p
}

// OAuth2ErrorCode represents a standardized OAuth 2.0 error code used to identify the reason a request failed.
// See https://www.rfc-editor.org/info/rfc6749.
type OAuth2ErrorCode string

const (
	// OAuth2InvalidRequest indicates that the request is malformed, missing required parameters, or otherwise invalid.
	OAuth2InvalidRequest OAuth2ErrorCode = "invalid_request"

	// OAuth2InvalidGrant indicates that the provided authorization grant, refresh token, or credentials are invalid or expired.
	OAuth2InvalidGrant OAuth2ErrorCode = "invalid_grant"

	// OAuth2InvalidClient indicates that client authentication failed or the client credentials are invalid.
	OAuth2InvalidClient OAuth2ErrorCode = "invalid_client"

	// OAuth2UnauthorizedClient indicates that the authenticated client is not permitted to use the requested grant type.
	OAuth2UnauthorizedClient OAuth2ErrorCode = "unauthorized_client"

	// OAuth2UnsupportedGrantType indicates that the authorization server does not support the requested grant type.
	OAuth2UnsupportedGrantType OAuth2ErrorCode = "unsupported_grant_type"
)

// OAuth2ErrorResponse represents a standard OAuth 2.0 error response returned by the authorization server.
type OAuth2ErrorResponse struct {
	// Error contains the OAuth 2.0 error code describing the failure.
	Error OAuth2ErrorCode `json:"error"`

	// ErrorDescription provides a human-readable explanation of the error.
	ErrorDescription string `json:"error_description,omitempty"`

	// ErrorURI provides a URL to documentation with additional details about the error.
	ErrorURI string `json:"error_uri,omitempty"`
}

func OAuth2AccessToken(w http.ResponseWriter, accessToken AccessToken) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(accessToken); err != nil {
		slog.Error("failed to encode OAuth2 access token", "err", err)
	}
}

func OAuth2TokenPair(w http.ResponseWriter, tokenPair TokenPair) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tokenPair); err != nil {
		slog.Error("failed to encode OAuth2 token pair", "err", err)
	}
}

func OAuth2Error(w http.ResponseWriter, code OAuth2ErrorCode, description string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(OAuth2ErrorResponse{
		Error:            code,
		ErrorDescription: description,
	}); err != nil {
		slog.Error("failed to encode OAuth2 token pair", "err", err)
	}
}

func OAuth2Unauthorized(w http.ResponseWriter, code OAuth2ErrorCode, description string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(OAuth2ErrorResponse{
		Error:            code,
		ErrorDescription: description,
	}); err != nil {
		slog.Error("failed to encode OAuth2 token pair", "err", err)
	}
}

func (a *API) MiddlewareAuthentication(roles map[Role]bool) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			bearer, found := strings.CutPrefix(authHeader, "Bearer ")
			if !found || bearer == "" {
				OAuth2Unauthorized(w, OAuth2InvalidClient, "Bearer token is required")
				return
			}

			claims, err := a.Vault.ParseAccessToken(bearer)
			if err != nil {
				OAuth2Error(w, OAuth2InvalidGrant, "invalid access token")
				return
			}

			if len(roles) > 0 {
				authorized := false
				for role := range claims.Roles {
					if roles[role] {
						authorized = true
						break
					}
				}
				if !authorized {
					OAuth2Error(w, OAuth2InvalidGrant, "unauthorized")
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ContextKeyClaims, claims)))
		}
	}
}

func (a *API) MiddlewareLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		responseWriter := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(responseWriter, r)
		slog.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", responseWriter.status,
			"duration", time.Since(start),
		)
	}
}

func (a *API) MiddlewareMaxBytesLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1_000_000)
		next.ServeHTTP(w, r)
	}
}

type Method string

const (
	MethodPost   Method = http.MethodPost
	MethodGet    Method = http.MethodGet
	MethodPatch  Method = http.MethodPatch
	MethodDelete Method = http.MethodDelete
)

type RouteConfig struct {
	Method  Method
	Handler http.HandlerFunc
	Route   Route
	Roles   []Role
}

func (a *API) PublicRoute(c RouteConfig) {
	handler := MiddlewareChain(
		c.Handler,
		a.MiddlewareLogging,
		a.MiddlewarePanicRecovery,
		a.MiddlewareMaxBytesLimit,
	)

	a.Mux.Handle(
		fmt.Sprintf("%s %s", c.Method, c.Route),
		handler,
	)
}

func (a *API) ProtectedRoute(c RouteConfig) {
	rolesMap := make(map[Role]bool)
	for _, role := range c.Roles {
		rolesMap[role] = true
	}

	handler := MiddlewareChain(
		c.Handler,
		a.MiddlewareLogging,
		a.MiddlewarePanicRecovery,
		a.MiddlewareMaxBytesLimit,
		a.MiddlewareAuthentication(rolesMap),
	)

	a.Mux.Handle(
		fmt.Sprintf("%s %s", c.Method, c.Route),
		handler,
	)
}

type Route string

// Routes
const (
	RouteHealth            Route = "/health"
	RouteOAuth2AccessToken Route = "/oauth2/token"
	RouteOAuth2Authorize   Route = "/oauth2/authorize"
	RouteOAuth2Callback    Route = "/oauth2/callback"
)

const (
	RouteV1Me                       Route = "/v1/me"
	RouteV1MeCandidate              Route = "/v1/me/candidate"
	RouteV1MeMatches                Route = "/v1/me/matches"
	RouteV1MePositions              Route = "/v1/me/positions"
	RouteV1MePosition               Route = "/v1/me/positions/{id}"
	RouteV1MeReactions              Route = "/v1/me/reactions"
	RouteV1MeRecommendations        Route = "/v1/me/recommendations"
	RouteV1MeRecommendationReaction Route = "/v1/me/recommendations/{id}/reaction"
	RouteV1MeRecruiter              Route = "/v1/me/recruiter"
)

func (a *API) RegisterRoutes() {
	slog.Debug("registering routes")

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteHealth,
		Handler: a.HandlerHealth,
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteOAuth2Authorize,
		Handler: a.HandlerOAuth2Authorize(),
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteOAuth2Authorize,
		Handler: a.HandlerOAuth2Authorize(),
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteOAuth2AccessToken,
		Handler: a.HandlerOAuth2CreateAccessToken(),
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteOAuth2Callback,
		Handler: a.HandlerOAuth2Callback(),
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteOAuth2Callback,
		Handler: a.HandlerOAuth2Callback(),
	})

	// TODO: Document the route in openapi.json
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteV1Me,
		Handler: a.HandlerV1CreateMe(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1Me,
		Handler: a.HandlerV1GetMe(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RouteV1Me,
		Handler: a.HandlerV1PatchMe(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteV1Me,
		Handler: a.HandlerV1DeleteMe(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteV1MeCandidate,
		Handler: a.HandlerV1CreateMeCandidateProfile(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MeCandidate,
		Handler: a.HandlerV1GetMeCandidateProfile(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RouteV1MeCandidate,
		Handler: a.HandlerV1PatchMeCandidateProfile(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteV1MeCandidate,
		Handler: a.HandlerV1DeleteMeCandidateProfile(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteV1MeRecruiter,
		Handler: a.HandlerV1CreateMeRecruiterProfile(),
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MeRecruiter,
		Handler: a.HandlerV1GetMeRecruiterProfile(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteV1MeRecruiter,
		Handler: a.HandlerV1DeleteMeRecruiterProfile(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteV1MePositions,
		Handler: a.HandlerV1CreateMePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MePositions,
		Handler: a.HandlerV1GetMePositions(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MePosition,
		Handler: a.HandlerV1GetMePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RouteV1MePosition,
		Handler: a.HandlerV1PatchMePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteV1MePosition,
		Handler: a.HandlerV1DeleteMePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MeRecommendations,
		Handler: a.HandlerV1GetMeRecommendations(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MeReactions,
		Handler: a.HandlerV1GetMeReactions(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteV1MeRecommendationReaction,
		Handler: a.HandlerV1CreateMeReaction(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	a.ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteV1MeMatches,
		Handler: a.HandlerV1GetMeMatches(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})
}

func (a *API) OAuth2CreateAccessToken(w http.ResponseWriter, userID ULID, provider Provider, roles map[Role]ULID) {
	accessToken, err := a.Vault.CreateAccessToken(userID, provider, roles)
	if err != nil {
		slog.Error(
			"failed to create access token",
			"err", err,
		)
		OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
		return
	}
	OAuth2AccessToken(w, accessToken)
}

// TODO: Write tests for this handler
func (a *API) HandlerOAuth2CreateAccessToken() http.HandlerFunc {
	type RequestBody struct {
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			OAuth2Error(w, OAuth2InvalidRequest, "invalid request body")
			return
		}
		if body.GrantType != "refresh_token" {
			OAuth2Error(w, OAuth2UnsupportedGrantType, "grant_type must be refresh_token")
			return
		}
		if body.RefreshToken == "" {
			OAuth2Error(w, OAuth2InvalidGrant, "refresh_token is required")
			return
		}

		claims, err := a.Vault.ParseRefreshToken(body.RefreshToken)
		if err != nil {
			slog.Error(
				"failed to parse refresh token",
				"ip", r.RemoteAddr,
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidGrant, "invalid refresh token")
			return
		}

		isRevoked, err := a.Store.IsRevokedRefreshToken(claims.JTI)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn(
					"refresh token not found",
					"jti", claims.JTI,
					"user_id", claims.UserID,
					"ip", r.RemoteAddr,
				)
				OAuth2Error(w, OAuth2InvalidGrant, "invalid refresh token")
				return
			}
			slog.Error(
				"failed to validate refresh token",
				"err", err,
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}
		if isRevoked {
			slog.Warn(
				"revoked token reuse attempt",
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			OAuth2Error(w, OAuth2InvalidGrant, "invalid refresh token")
			return
		}

		roles, err := a.Store.GetUserRoles(
			claims.UserID,
			Provider(claims.Provider),
		)
		if err != nil {
			slog.Error(
				"failed to get user roles",
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		a.OAuth2CreateAccessToken(
			w,
			claims.UserID,
			claims.Provider,
			roles,
		)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerOAuth2Authorize() http.HandlerFunc {
	type RequstBodyEmailAuthorization struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		providerString := r.URL.Query().Get("provider")
		provider, err := StringToProvider(providerString, ProviderEmail)
		if err != nil {
			OAuth2Error(
				w,
				OAuth2InvalidRequest,
				"invalid provider; must be one of: google, apple, email",
			)
			return
		}

		if provider == ProviderEmail {
			body, err := DecodeRequestBody[RequstBodyEmailAuthorization](r)
			if err != nil {
				OAuth2Error(w, OAuth2InvalidRequest, "invalid request body")
				return
			}
			if body.Email == "" || body.Password == "" {
				OAuth2Error(w, OAuth2InvalidRequest, "email and password required")
				return
			}

			user, roles, err := a.Store.GetUserAndRolesByEmail(body.Email, ProviderEmail)
			switch {

			case errors.Is(err, ErrUserNotFound):
				OAuth2Unauthorized(w, OAuth2InvalidRequest, "invalid credentials")
				return

			case errors.Is(err, ErrUserNoRole):
				a.OAuth2CreateAccessToken(w, user.ID, user.Provider, map[Role]ULID{})
				return

			case err != nil:
				slog.Error("failed to get user by email", "err", err)
				OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
				return

			}

			if !a.Vault.IsValidPassword(user.PasswordHash, body.Password) {
				OAuth2Unauthorized(w, OAuth2InvalidRequest, "invalid credentials")
				return
			}

			a.OAuth2CreateTokenPair(w, user.ID, user.Provider, roles)
			return
		}

		state, err := a.Vault.CreateStateToken(provider)
		if err != nil {
			slog.Error(
				"failed to generate state token",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		parsed, err := a.Vault.ParseStateToken(state)
		if err != nil {
			slog.Error(
				"failed to parse state token",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_csrf",
			Value:    parsed.CSRF,
			Path:     "/",
			MaxAge:   int(a.Vault.StateTokenExpiration),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		verifier := oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_verifier",
			Value:    verifier,
			Path:     "/",
			MaxAge:   int(a.Vault.VerifierExpiration),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		url, err := a.Vault.CreateAuthCodeURL(state, verifier, provider)
		if err != nil {
			slog.Error(
				"failed to generate auth code URL",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		http.Redirect(
			w, r,
			url,
			http.StatusTemporaryRedirect,
		)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerOAuth2Callback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(
			r.Context(),
			a.Vault.StateTokenExpiration,
		)
		defer cancel()

		stateString := r.URL.Query().Get("state")
		if stateString == "" {
			OAuth2Error(w, OAuth2InvalidRequest, "missing state")
			return
		}

		state, err := a.Vault.ParseStateToken(stateString)
		if err != nil {
			OAuth2Error(w, OAuth2InvalidRequest, "invalid state token")
			return
		}

		csrfCookie, err := r.Cookie("oauth_csrf")
		if err != nil || csrfCookie.Value != state.CSRF {
			OAuth2Error(w, OAuth2InvalidRequest, "invalid CSRF token")
			return
		}

		verifierCookie, err := r.Cookie("oauth_verifier")
		if err != nil {
			OAuth2Error(w, OAuth2InvalidRequest, "missing PKCE verifier")
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			OAuth2Error(w, OAuth2InvalidRequest, "authorization provider error")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			OAuth2Error(w, OAuth2InvalidRequest, "missing authorization code")
			return
		}

		DeleteCookies(w, [2]string{"oauth_csrf", "oauth_verifier"})

		var user User
		switch state.Provider {
		case ProviderGoogle:
			rawIDToken, err := a.Vault.ExchangeGoogleCodeForIDToken(
				ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				OAuth2Error(w, OAuth2InvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error(
					"failed to exchange Google code",
					"err", err,
				)
				OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
				return
			}

			user, err = a.Vault.VerifyAndParseGoogleIDToken(ctx, rawIDToken)
			switch {

			case errors.Is(err, ErrInvalidIDToken):
				OAuth2Error(w, OAuth2InvalidRequest, "invalid id_token")
				return

			case errors.Is(err, ErrFailedParseClaims):
				OAuth2Error(w, OAuth2InvalidRequest, "failed to parse claims")
				return

			case errors.Is(err, ErrEmailNotVerified):
				OAuth2Error(w, OAuth2InvalidRequest, "unverified provider email")
				return

			case err != nil:
				slog.Error(
					"failed to verify Google ID token",
					"err", err,
				)
				OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
				return
			}

		case ProviderApple:
			idTokenString, err := a.Vault.ExchangeAppleCodeForIDToken(
				ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				OAuth2Error(w, OAuth2InvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error(
					"failed to exchange Apple code",
					"err", err,
				)
				OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
				return
			}

			user, err = a.Vault.VerifyAndParseAppleIDToken(
				ctx,
				idTokenString,
				r.FormValue("user"),
			)
			switch {

			case errors.Is(err, ErrInvalidIDToken):
				OAuth2Error(w, OAuth2InvalidRequest, "invalid id_token")
				return

			case errors.Is(err, ErrFailedParseClaims):
				OAuth2Error(w, OAuth2InvalidRequest, "failed to parse claims")
				return

			case errors.Is(err, ErrEmailNotVerified):
				OAuth2Error(w, OAuth2InvalidRequest, "unverified provider email")
				return

			case err != nil:
				slog.Error(
					"failed to verify Apple ID token",
					"err", err,
				)
				OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
				return
			}

		default:
			OAuth2Error(
				w,
				OAuth2InvalidRequest,
				"invalid provider; must be one of: google, apple",
			)
			return
		}

		a.FinishAuthFlow(w, user)
	}
}

var RegexFullName = regexp.MustCompile(`^[\pL][\pL\s'’-]{2,512}\z`)

const (
	DefaultUserFullNameMinLength = 2
	DefaultUserFullNameMaxLength = 512
)

func NormalizeAndValidateUserFullName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ")
	if len(name) < DefaultUserFullNameMinLength {
		return "", ErrTextTooShort
	}
	if len(name) > DefaultUserFullNameMaxLength {
		return "", ErrTextTooLong
	}
	if !RegexFullName.MatchString(name) {
		return "", ErrTextForbiddenChars
	}
	return name, nil
}

var (
	FailMessageUserFullNameWrongSize      = fmt.Sprintf("full_name must be between %v and %v characters", DefaultUserFullNameMinLength, DefaultUserFullNameMaxLength)
	FailMessageUserFullNameForbiddenChars = "full_name must be a valid 'passport-style' full name. It must start with a letter and can only contain letters, spaces, apostrophes, or hyphens"
)

func (a *API) FinishAuthFlow(w http.ResponseWriter, user User) {
	userID, roles, err := a.Store.GetUserAndRolesByProvider(user.Provider, user.ProviderUserID)
	switch {

	case errors.Is(err, ErrUserNotFound):
		userID, ulidErr := NewUserULID()
		if ulidErr != nil {
			slog.Error(
				"failed to generate user ULID",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		user.UserName, err = GenerateUserName()
		if err != nil {
			slog.Error(
				"failed to generate user name",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		user.FullName, err = NormalizeAndValidateUserFullName(user.FullName)
		switch {

		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			OAuth2Error(w, OAuth2InvalidRequest, FailMessageUserFullNameWrongSize)
			return

		case errors.Is(err, ErrTextForbiddenChars):
			OAuth2Error(w, OAuth2InvalidRequest, FailMessageUserFullNameForbiddenChars)
			return

		case err != nil:
			slog.Error(
				"failed to validate full name",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}

		err = a.Store.CreateUser(user)
		if err != nil {
			slog.Error(
				"failed to create user",
				"err", err,
			)
			OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
			return
		}
		a.OAuth2CreateAccessToken(
			w,
			userID,
			user.Provider,
			map[Role]ULID{},
		)
		return

	case errors.Is(err, ErrUserNoRole):
		a.OAuth2CreateAccessToken(
			w,
			userID,
			user.Provider,
			map[Role]ULID{},
		)
		return

	case err != nil:
		slog.Error(
			"failed to get user by provider",
			"err", err,
		)
		OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
		return
	}

	a.OAuth2CreateTokenPair(
		w,
		userID,
		user.Provider,
		roles,
	)
}

func (a *API) OAuth2CreateTokenPair(w http.ResponseWriter, userID ULID, provider Provider, roles map[Role]ULID) {
	jti, err := NewJTIULID()
	if err != nil {
		slog.Error(
			"failed to generate JTI ULID",
			"err", err,
		)
		OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
		return
	}

	err = a.Store.CreateRefreshToken(
		jti,
		userID,
		time.Now().UTC().Add(DefaultRefreshTokenExpiration),
	)
	if err != nil {
		slog.Error(
			"failed to create refresh token",
			"err", err,
		)
		OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
		return
	}

	tokenPair, err := a.Vault.CreateTokenPair(userID, provider, jti, roles)
	if err != nil {
		slog.Error(
			"failed to create token pair",
			"err", err,
		)
		OAuth2Error(w, OAuth2InvalidRequest, "internal server error")
		return
	}

	OAuth2TokenPair(w, tokenPair)
}

func DeleteCookies(w http.ResponseWriter, names [2]string) {
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

// JSONAPIMediaType is the official media type used for JSON:API request and response payloads.
// See https://jsonapi.org/format/.
const JSONAPIMediaType = "application/vnd.api+json"

// JSONAPIError represents a single error object returned when a request fails.
type JSONAPIError struct {
	// Status is the HTTP status code associated with the error, represented as a string.
	Status string `json:"status,omitempty"`

	// Code is an application-specific error identifier used for programmatic handling.
	Code string `json:"code,omitempty"`

	// Title is a short, human-readable summary of the error.
	Title string `json:"title,omitempty"`

	// Detail provides a more detailed explanation of the error.
	Detail string `json:"detail,omitempty"`

	// Source identifies the specific field or parameter that caused the error.
	Source *JSONAPIErrorSource `json:"source,omitempty"`

	// Meta contains additional custom information about the error.
	Meta map[string]any `json:"meta,omitempty"`
}

// JSONAPIErrorSource identifies where an error originated in the request.
type JSONAPIErrorSource struct {
	// Pointer is a JSON Pointer to the offending value in the request body.
	Pointer string `json:"pointer,omitempty"`

	// Parameter is the query string parameter that caused the error.
	Parameter string `json:"parameter,omitempty"`
}

// JSONAPIResource represents a JSON:API resource object such as a user,
// article, order, or any other domain entity.
type JSONAPIResource struct {
	// Type is the resource type, typically matching the collection name.
	Type string `json:"type"`

	// ID is the unique identifier of the resource.
	ID string `json:"id"`

	// Attributes contains the resource's actual data fields.
	Attributes map[string]any `json:"attributes,omitempty"`

	// Relationships contains references to related resources.
	Relationships map[string]any `json:"relationships,omitempty"`

	// Links contains URLs related to this resource.
	Links map[string]any `json:"links,omitempty"`

	// Meta contains additional resource-specific metadata.
	Meta map[string]any `json:"meta,omitempty"`
}

// JSONAPIDocument is the top-level JSON:API response document.
type JSONAPIDocument struct {
	// Data contains the primary resource(s) returned by the request.
	Data any `json:"data,omitempty"`

	// Errors contains one or more errors when the request fails.
	Errors []JSONAPIError `json:"errors,omitempty"`

	// Meta contains response-level metadata.
	Meta map[string]any `json:"meta,omitempty"`

	// Included contains related resources that are side-loaded with the response
	// to reduce the need for additional API requests.
	Included []JSONAPIResource `json:"included,omitempty"`

	// Links contains top-level navigation or pagination URLs.
	Links map[string]any `json:"links,omitempty"`
}

func JSON(w http.ResponseWriter, document JSONAPIDocument, status int) {
	w.Header().Set("Content-Type", JSONAPIMediaType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(document); err != nil {
		slog.Error("failed to encode json:api response", "err", err)
	}
}

func DecodeRequestBody[T any](r *http.Request) (T, error) {
	var data T

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&data); err != nil {
		return data, err
	}
	if decoder.More() {
		return data, ErrExtraDataDecoded
	}

	if err := r.Body.Close(); err != nil {
		return data, err
	}

	return data, nil
}

var (
	// UserNameAdjectives is an array for creating random user names, used in conjunction with nouns
	UserNameAdjectives = [...]string{
		"fast",
		"lazy",
		"clever",
		"curious",
		"brave",
		"mighty",
		"silent",
		"noisy",
		"happy",
		"grumpy",
	}

	// UserNameNouns is an array for creating random user names, used in conjunction with adjectives
	UserNameNouns = [...]string{
		"lion",
		"tiger",
		"panda",
		"fox",
		"eagle",
		"shark",
		"wolf",
		"dragon",
		"otter",
		"koala",
	}
)

func GenerateUserName() (string, error) {
	randomInteger := func(n int) (int, error) {
		if n <= 0 {
			return 0, nil
		}
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			return 0, err
		}
		return int(b[0]) % n, nil
	}

	indexAdjectives, err := randomInteger(len(UserNameAdjectives))
	if err != nil {
		return "", fmt.Errorf("failed to pull secure adjective seed: %w", err)
	}
	adjective := UserNameAdjectives[indexAdjectives]

	indexNouns, err := randomInteger(len(UserNameNouns))
	if err != nil {
		return "", fmt.Errorf("failed to pull secure noun seed: %w", err)
	}
	noun := UserNameNouns[indexNouns]

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("failed to pull secure suffix seed: %w", err)
	}

	userName := strings.ToLower(fmt.Sprintf("%s_%s%s", adjective, noun, hex.EncodeToString(suffix)))
	return userName, nil
}

// TODO: Write tests for this handler
func (a *API) HandlerHealth(w http.ResponseWriter, r *http.Request) {
	JSON(w, JSONAPIDocument{
		Meta: map[string]any{
			"status": "ok",
		},
	}, http.StatusOK)
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1GetMeRecommendations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, isCandidate := claims.Roles[RoleCandidate]
		recruiterID, isRecruiter := claims.Roles[RoleRecruiter]
		if !isCandidate && !isRecruiter {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing one of required roles: recruiter, candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		q := r.URL.Query()
		page := GetPagination(r)
		var excludeReacted bool
		switch q.Get("exclude_reacted") {
		case "true":
			excludeReacted = true
		case "false":
			excludeReacted = false
		default:
			excludeReacted = true
		}

		page.Count = 0
		positionCursor := q.Get("pos_cursor")
		positionNextCursor, candidateNextCursor := "done", "done"
		data := make([]JSONAPIResource, 0)
		if isCandidate && q.Get("exclude_positions") != "true" && positionCursor != "done" {
			recommendations, cursor, err := a.Store.GetPositionRecommendations(
				candidateID,
				Page{Cursor: positionCursor, Limit: page.Limit},
				excludeReacted,
			)
			if err != nil {
				slog.Error("failed to fetch position recommendations", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}

			positionNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recommendations)
			for _, rec := range recommendations {
				data = append(data, JSONAPIResource{
					Type: "recommendations",
					ID:   string(rec.RecommendationID),
					Attributes: map[string]any{
						"position_id": rec.PositionID,
						"title":       rec.Title,
						"company":     rec.Company,
						"description": rec.Description,
					},
					Links: map[string]any{
						"self":     fmt.Sprintf("%s/%s", RouteV1MeRecommendations, rec.RecommendationID),
						"reaction": fmt.Sprintf("%s/%s/reaction", RouteV1MeRecommendations, rec.RecommendationID),
					},
				})
			}
		}

		candidateCursor := q.Get("can_cursor")
		if isRecruiter && q.Get("exclude_candidates") != "true" && candidateCursor != "done" {
			recommendations, cursor, err := a.Store.GetCandidateRecommendations(
				recruiterID,
				Page{Cursor: candidateCursor, Limit: page.Limit},
				excludeReacted,
			)
			if err != nil {
				slog.Error("failed to fetch candidate recommendations", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}

			candidateNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recommendations)
			for _, rec := range recommendations {
				data = append(data, JSONAPIResource{
					Type: "recommendations",
					ID:   string(rec.RecommendationID),
					Attributes: map[string]any{
						"candidate_id": rec.CandidateID,
						"full_name":    rec.FullName,
						"about":        rec.About,
					},
					Links: map[string]any{
						"self":     fmt.Sprintf("%s/%s", RouteV1MeRecommendations, rec.RecommendationID),
						"reaction": fmt.Sprintf("%s/%s/reaction", RouteV1MeRecommendations, rec.RecommendationID),
					},
				})
			}
		}

		page.HasNext = positionNextCursor != "done" || candidateNextCursor != "done"
		selfHref := string(RouteV1MeRecommendations)
		if excludeReacted {
			selfHref += "?exclude_reacted=true"
		}

		links := map[string]any{
			"self":      selfHref,
			"reactions": string(RouteV1MeReactions),
		}
		if page.HasNext {
			nextHref := fmt.Sprintf(
				"%s?pos_cursor=%s&can_cursor=%s&limit=%d",
				RouteV1MeRecommendations, positionNextCursor, candidateNextCursor, page.Limit,
			)
			if excludeReacted {
				nextHref += "&exclude_reacted=true"
			}
			links["next"] = nextHref
		}

		JSON(w, JSONAPIDocument{
			Data:  data,
			Meta:  map[string]any{"page": page},
			Links: links,
		}, http.StatusOK)
	}
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1CreateMeReaction() http.HandlerFunc {
	type RequestBody struct {
		ReactionType ReactionType `json:"reaction_type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		recommendationIDStr := r.PathValue("id")
		recommendationID := ULID(recommendationIDStr)
		if recommendationID == "" {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "recommendation id is required",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(recommendationIDStr, ULIDPrefixRecommendation) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid recommendation id",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}

		recommendation, err := a.Store.GetRecommendation(recommendationID)
		if err != nil {
			if errors.Is(err, ErrRecommendationNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "recommendation not found",
						Source: &JSONAPIErrorSource{Parameter: "id"},
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch recommendation", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		if recommendation.CandidateID != candidateID {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "you do not have access to this resource",
				}},
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		if !body.ReactionType.IsValid() {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "must be one of: positive, negative, neutral",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/reaction_type"},
				}},
			}, http.StatusBadRequest)
			return
		}

		if err := a.Store.CreateReaction(Reaction{
			RecommendationID: recommendationID,
			ReactorType:      ReactorTypeCandidate,
			ReactorID:        candidateID,
			ReactionType:     body.ReactionType,
		}); err != nil {
			if errors.Is(err, ErrReactionAlreadyExists) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "409",
						Title:  "Conflict",
						Detail: "reaction already exists; reactions are immutable",
						Source: &JSONAPIErrorSource{Parameter: "id"},
					}},
				}, http.StatusConflict)
				return
			}
			slog.Error("failed to record reaction", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "reactions",
				ID:   string(recommendationID),
				Attributes: map[string]any{
					"reaction_type": body.ReactionType,
				},
				Links: map[string]any{
					"self":      fmt.Sprintf("%s/%s/reaction", RouteV1MeRecommendations, recommendationID),
					"up":        string(RouteV1MeRecommendations),
					"reactions": string(RouteV1MeReactions),
					"matches":   string(RouteV1MeMatches),
				},
			},
		}, http.StatusCreated)
	}
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1GetMeReactions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		page := GetPagination(r)

		reactions, nextCursor, err := a.Store.GetReactionsByCandidateID(
			candidateID,
			page,
		)
		if err != nil {
			slog.Error("failed to fetch reactions", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		page.Count = len(reactions)
		page.HasNext = nextCursor != ""

		links := map[string]any{
			"self": string(RouteV1MeReactions),
		}
		if nextCursor != "" {
			links["next"] = fmt.Sprintf(
				"%s?cursor=%s",
				RouteV1MeReactions,
				nextCursor,
			)
		}

		data := make([]JSONAPIResource, len(reactions))
		for i, reaction := range reactions {
			data[i] = JSONAPIResource{
				Type: "reactions",
				ID:   string(reaction.RecommendationID),
				Attributes: map[string]any{
					"recommendation_id": reaction.RecommendationID,
					"reactor_type":      reaction.ReactorType,
					"reactor_id":        reaction.ReactorID,
					"reaction_type":     reaction.ReactionType,
					"reacted_at":        reaction.ReactedAt,
				},
				Links: map[string]any{
					"self": fmt.Sprintf(
						"%s/%s/reaction",
						RouteV1MeRecommendations,
						reaction.RecommendationID,
					),
				},
			}
		}

		JSON(w, JSONAPIDocument{
			Data:  data,
			Links: links,
			Meta:  map[string]any{"page": page},
		}, http.StatusOK)
	}
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1GetMeMatches() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		page := GetPagination(r)

		matches, nextCursor, err := a.Store.GetMatchesByCandidateID(candidateID, page)
		if err != nil {
			slog.Error("failed to fetch matches", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		page.Count = len(matches)
		page.HasNext = nextCursor != ""

		links := map[string]any{
			"self": string(RouteV1MeMatches),
		}
		if nextCursor != "" {
			links["next"] = fmt.Sprintf(
				"%s?cursor=%s&limit=%d",
				RouteV1MeMatches,
				nextCursor,
				page.Limit,
			)
		}

		data := make([]JSONAPIResource, len(matches))
		for i, m := range matches {
			data[i] = JSONAPIResource{
				Type: "matches",
				ID:   string(m.PositionID),
				Attributes: map[string]any{
					"title":       m.Title,
					"description": m.Description,
					"company":     m.Company,
					"created_at":  m.CreatedAt,
				},
			}
		}

		JSON(w, JSONAPIDocument{
			Data:  data,
			Links: links,
			Meta:  map[string]any{"page": page},
		}, http.StatusOK)
	}
}

var (
	RegexPasswordHasLower   = regexp.MustCompile(`[a-z]`)
	RegexPasswordHasUpper   = regexp.MustCompile(`[A-Z]`)
	RegexPasswordHasDigit   = regexp.MustCompile(`\d`)
	RegexPasswordHasSpecial = regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>/?]`)
)

const (
	DefaultUserPasswordMinLength = 8
	DefaultUserPasswordMaxLength = 128
)

func ValidateUserPassword(password string) error {
	if len(password) < DefaultUserPasswordMinLength {
		return ErrTextTooShort
	}
	if len(password) > DefaultUserPasswordMaxLength {
		return ErrTextTooLong
	}
	if !RegexPasswordHasLower.MatchString(password) {
		return ErrPasswordHasNoLower
	}
	if !RegexPasswordHasUpper.MatchString(password) {
		return ErrPasswordHasNoUpper
	}
	if !RegexPasswordHasDigit.MatchString(password) {
		return ErrPasswordHasNoDigit
	}
	if !RegexPasswordHasSpecial.MatchString(password) {
		return ErrPasswordHasNoSpecial
	}
	return nil
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1CreateMe() http.HandlerFunc {
	type RequestBody struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Password string `json:"password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		if body.Email == "" {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "email is required",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/email"},
				}},
			}, http.StatusBadRequest)
			return
		}

		email, err := mail.ParseAddress(body.Email)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid email",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/email"},
				}},
			}, http.StatusBadRequest)
			return
		}

		err = ValidateUserPassword(body.Password)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "password must be between 8 and 128 characters",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrPasswordHasNoLower):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "password must contain at least one lowercase letter",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrPasswordHasNoUpper):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "password must contain at least one uppercase letter",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrPasswordHasNoDigit):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "password must contain at least one digit",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrPasswordHasNoSpecial):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "password must contain at least one special character",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
				}},
			}, http.StatusBadRequest)
			return
		case err != nil:
			slog.Error("failed to validate password", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		fullName, err := NormalizeAndValidateUserFullName(body.FullName)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: FailMessageUserFullNameWrongSize,
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/full_name"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrTextForbiddenChars):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: FailMessageUserFullNameForbiddenChars,
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/full_name"},
				}},
			}, http.StatusBadRequest)
			return
		case err != nil:
			slog.Error("failed to validate full name", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		exists, err := a.Store.UserExistsByEmail(email.Address, ProviderEmail)
		if err != nil {
			slog.Error("failed to check user existance", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}
		if exists {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "409",
					Title:  "Conflict",
					Detail: "user already exists",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/email"},
				}},
			}, http.StatusConflict)
			return
		}

		userName, err := GenerateUserName()
		if err != nil {
			slog.Error("failed to generate a user name", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		userID, err := NewUserULID()
		if err != nil {
			slog.Error("failed to generate a user ULID", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		passwordHash, err := a.Vault.HashPassword(body.Password)
		if err != nil {
			slog.Error("failed to hash password", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		user := User{
			ID:           userID,
			Provider:     ProviderEmail,
			Email:        email.Address,
			FullName:     fullName,
			UserName:     userName,
			PasswordHash: passwordHash,
		}
		err = a.Store.CreateUser(user)
		if err != nil {
			slog.Error("failed to create user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		a.OAuth2CreateTokenPair(
			w,
			user.ID,
			user.Provider,
			map[Role]ULID{},
		)
	}
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1CreateMeRecruiterProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recruiterID, err := NewRecruiterULID()
		if err != nil {
			slog.Error("failed to generate a recruiter ULID", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		err = a.Store.CreateRecruiter(Recruiter{recruiterID, claims.UserID})
		if err != nil {
			if errors.Is(err, ErrRecruiterAlreadyExists) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "409",
						Title:  "Conflict",
						Detail: "recruiter already exists",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/user_id"},
					}},
				}, http.StatusConflict)
				return
			}
			slog.Error("failed to create recruiter", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		roles, err := a.Store.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error("failed to get user roles", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		a.OAuth2CreateAccessToken(
			w,
			claims.UserID,
			claims.Provider,
			roles,
		)
	}
}

var RegexHasTags = regexp.MustCompile(`<[^>]*>`)

const (
	DefaultCandidateAboutMaxLength = 1024
)

func NormalizeAndValidateCandidateAbout(about string) (string, error) {
	about = strings.TrimSpace(about)
	about = RegexHasTags.ReplaceAllString(about, "")
	if len(about) > DefaultCandidateAboutMaxLength {
		return "", ErrTextTooLong
	}
	return html.EscapeString(about), nil
}

// TODO: Write integration tests for this handler
func (a *API) HandlerV1CreateMeCandidateProfile() http.HandlerFunc {
	type RequestBody struct {
		About string `json:"about"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		about, err := NormalizeAndValidateCandidateAbout(body.About)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "about must be up to 1024 characters",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/about"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate about", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, err := NewCandidateULID()
		if err != nil {
			slog.Error("failed to generate a candidate ULID", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		err = a.Store.CreateCandidate(Candidate{
			ID:     candidateID,
			UserID: claims.UserID,
			About:  about,
		})
		if err != nil {
			if errors.Is(err, ErrCandidateAlreadyExists) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "409",
						Title:  "Conflict",
						Detail: "candidate already exists",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/user_id"},
					}},
				}, http.StatusConflict)
				return
			}
			slog.Error("failed to create candidate", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		roles, err := a.Store.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error("failed to get user roles", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		a.OAuth2CreateAccessToken(
			w,
			claims.UserID,
			claims.Provider,
			roles,
		)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1GetMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "user not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "users",
				ID:   string(user.ID),
				Attributes: map[string]any{
					"provider":   user.Provider,
					"email":      user.Email,
					"full_name":  user.FullName,
					"user_name":  user.UserName,
					"updated_at": user.UpdatedAt,
				},
				Links: map[string]any{
					"self": string(RouteV1Me),
				},
			},
		}, http.StatusOK)
	}
}

var RegexUserName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

const (
	DefaultUserNameMinLength = 4
	DefaultUesrNameMaxLength = 32
)

func NormalizeAndValidateUserName(userName string) (string, error) {
	userName = strings.TrimSpace(userName)
	if len(userName) < DefaultUserNameMinLength {
		return "", ErrTextTooShort
	}
	if len(userName) > DefaultUesrNameMaxLength {
		return "", ErrTextTooLong
	}
	if !RegexUserName.MatchString(userName) {
		return "", ErrTextForbiddenChars
	}
	return userName, nil
}

const (
	FailMessageUserNameWrongSize      = "user_name must be between 4 and 32 characters"
	FailMessageUserNameForbiddenChars = "user_name can only contain underscores, latin characters and numbers"
)

// TODO: Write tests for this handler
func (a *API) HandlerV1PatchMe() http.HandlerFunc {
	type RequestBody struct {
		UserName *string `json:"user_name,omitempty"`
		FullName *string `json:"full_name,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "user not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		changed := false
		if body.FullName != nil {
			fullName, err := NormalizeAndValidateUserFullName(*body.FullName)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: FailMessageUserFullNameWrongSize,
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/full_name"},
					}},
				}, http.StatusBadRequest)
				return
			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: FailMessageUserFullNameForbiddenChars,
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/full_name"},
					}},
				}, http.StatusBadRequest)
				return
			case err != nil:
				slog.Error("failed to validate full name", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}

			if user.FullName != fullName {
				user.FullName = fullName
				changed = true
			}
		}

		if body.UserName != nil {
			userName, err := NormalizeAndValidateUserName(*body.UserName)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: FailMessageUserNameWrongSize,
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/user_name"},
					}},
				}, http.StatusBadRequest)
				return
			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: FailMessageUserNameForbiddenChars,
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/user_name"},
					}},
				}, http.StatusBadRequest)
				return
			case err != nil:
				slog.Error("failed to validate user name", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}

			if user.UserName != userName {
				user.UserName = userName
				changed = true
			}
		}

		if !changed {
			JSON(w, JSONAPIDocument{
				Meta: map[string]any{
					"status": "success",
				},
			}, http.StatusOK)
			return
		}

		err = a.Store.UpdateUser(user)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "user not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1DeleteMe() http.HandlerFunc {
	type RequestBodyPassword struct {
		Password string `json:"password"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "user not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		if user.Provider == ProviderEmail {
			body, err := DecodeRequestBody[RequestBodyPassword](r)
			if err != nil {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "invalid request body",
					}},
				}, http.StatusBadRequest)
				return
			}
			if body.Password == "" {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "password is required",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
					}},
				}, http.StatusBadRequest)
				return
			}
			if !a.Vault.IsValidPassword(user.PasswordHash, body.Password) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "401",
						Title:  "Unauthorized",
						Detail: "incorrect password",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/password"},
					}},
				}, http.StatusUnauthorized)
				return
			}
		} else {
			// TODO: Add user deletion support for SSO
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "account deletion is not supported for this provider",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/provider"},
				}},
			}, http.StatusForbidden)
			return
		}

		err = a.Store.DeleteUser(user.ID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "user not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete user", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1GetMeCandidateProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "candidate not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get candidate", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "candidates",
				ID:   string(candidateID),
				Attributes: map[string]any{
					"user_id":             candidate.UserID,
					"about":               candidate.About,
					"last_recommended_at": candidate.LastRecommendedAt,
				},
				Links: map[string]any{
					"self": string(RouteV1MeCandidate),
				},
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1PatchMeCandidateProfile() http.HandlerFunc {
	type RequestBody struct {
		About *string `json:"about,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "candidate not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get candidate", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		changed := false
		if body.About != nil {
			about, err := NormalizeAndValidateCandidateAbout(*body.About)
			if errors.Is(err, ErrTextTooLong) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "about must be up to 1024 characters",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/about"},
					}},
				}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate about", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}

			if candidate.About != about {
				candidate.About = about
				changed = true
			}
		}

		if !changed {
			JSON(w, JSONAPIDocument{
				Meta: map[string]any{
					"status": "success",
				},
			}, http.StatusOK)
			return
		}

		err = a.Store.UpdateCandidate(candidate)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "candidate not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update candidate", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1DeleteMeCandidateProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: candidate",
				}},
			}, http.StatusForbidden)
			return
		}

		err := a.Store.DeleteCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "candidate not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete candidate", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1GetMeRecruiterProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		recruiter, err := a.Store.GetRecruiter(recruiterID)
		if err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "recruiter not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get recruiter", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "recruiters",
				ID:   string(recruiterID),
				Attributes: map[string]any{
					"user_id": recruiter.UserID,
				},
				Links: map[string]any{
					"self": string(RouteV1MeRecruiter),
				},
			},
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1DeleteMeRecruiterProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		err := a.Store.DeleteRecruiter(recruiterID)
		if err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "recruiter not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete recruiter", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

var RegexUrl = regexp.MustCompile(`https?://|www\.`)

const (
	DefaultPositionTitleMinLength = 4
	DefaultPositionTitleMaxLength = 64
)

func NormalizeAndValidatePositionTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")
	if len(title) < DefaultPositionTitleMinLength {
		return "", ErrTextTooShort
	}
	if len(title) > DefaultPositionTitleMaxLength {
		return "", ErrTextTooLong
	}
	if _, err := url.ParseRequestURI(title); err == nil || RegexUrl.MatchString(title) {
		return "", ErrPositionTitleHasURL
	}
	return title, nil
}

var RegexTags = regexp.MustCompile(`<[^>]*>`)

const (
	DefaultPositionDescriptionMaxLength = 2048
)

func NormalizeAndValidatePositionDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	description = RegexTags.ReplaceAllString(description, "")
	if len(description) > DefaultPositionDescriptionMaxLength {
		return "", ErrTextTooLong
	}
	return html.EscapeString(description), nil
}

const (
	DefaultPositionCompanyNameMinLength = 2
	DefaultPositionCompanyNameMaxLength = 512
)

func NormalizeAndValidatePositionCompanyName(company string) (string, error) {
	company = strings.TrimSpace(company)
	if len(company) < DefaultPositionCompanyNameMinLength {
		return "", ErrTextTooShort
	}
	if len(company) > DefaultPositionCompanyNameMaxLength {
		return "", ErrTextTooLong
	}
	return company, nil
}

// TODO: Write tests for this handler
func (a *API) HandlerV1CreateMePosition() http.HandlerFunc {
	type RequestBody struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Company     string `json:"company"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		title, err := NormalizeAndValidatePositionTitle(body.Title)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "title must be between 4 and 64 characters",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/title"},
				}},
			}, http.StatusBadRequest)
			return
		case errors.Is(err, ErrPositionTitleHasURL):
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "title cannot contain a URL",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/title"},
				}},
			}, http.StatusBadRequest)
			return
		case err != nil:
			slog.Error("failed to validate position title", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		description, err := NormalizeAndValidatePositionDescription(body.Description)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "description must be up to 2048 characters",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/description"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate description", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		company, err := NormalizeAndValidatePositionCompanyName(body.Company)
		if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "company name must be between 2 and 512 characters",
					Source: &JSONAPIErrorSource{Pointer: "/data/attributes/company"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate company name", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		positionID, err := NewPositionULID()
		if err != nil {
			slog.Error("failed to generate position ULID", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		err = a.Store.CreatePosition(Position{
			positionID,
			recruiterID,
			title,
			description,
			company,
			true,
		})
		if err != nil {
			if errors.Is(err, ErrPositionAlreadyExists) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "409",
						Title:  "Conflict",
						Detail: "position with the same title, description and company already exists",
					}},
				}, http.StatusConflict)
				return
			}
			slog.Error("failed to create position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "positions",
				ID:   string(positionID),
				Attributes: map[string]any{
					"is_active": true,
				},
				Links: map[string]any{
					"self": fmt.Sprintf("%s/%s", RouteV1MePositions, positionID),
				},
			},
		}, http.StatusCreated)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1GetMePositions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		page := GetPagination(r)
		positions, nextCursor, err := a.Store.GetPositions(recruiterID, page)
		if err != nil {
			slog.Error("failed to fetch positions", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		links := map[string]any{
			"self": string(RouteV1MePositions),
		}
		if nextCursor != "" {
			links["next"] = fmt.Sprintf("%s?cursor=%s", RouteV1MePositions, nextCursor)
		}

		data := make([]JSONAPIResource, len(positions))
		for i, pos := range positions {
			data[i] = JSONAPIResource{
				Type: "positions",
				ID:   string(pos.ID),
				Attributes: map[string]any{
					"title":       pos.Title,
					"description": pos.Description,
					"company":     pos.Company,
					"is_active":   pos.IsActive,
				},
				Links: map[string]any{
					"self": fmt.Sprintf("%s/%s", RouteV1MePositions, pos.ID),
				},
			}
		}

		JSON(w, JSONAPIDocument{
			Data:  data,
			Links: links,
		}, http.StatusOK)
	}
}

// TODO: Write tests for this handler
func (a *API) HandlerV1GetMePosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "position id is required",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(positionIDStr, ULIDPrefixPosition) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid position id",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "position not found",
						Source: &JSONAPIErrorSource{Parameter: "id"},
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "you do not have access to this resource",
				}},
			}, http.StatusForbidden)
			return
		}

		JSON(w, JSONAPIDocument{
			Data: JSONAPIResource{
				Type: "positions",
				ID:   string(position.ID),
				Attributes: map[string]any{
					"title":       position.Title,
					"description": position.Description,
					"company":     position.Company,
					"is_active":   position.IsActive,
				},
				Links: map[string]any{
					"self": fmt.Sprintf("%s/%s", RouteV1MePositions, position.ID),
				},
			},
		}, http.StatusOK)
	}
}

func (a *API) HandlerV1PatchMePosition() http.HandlerFunc {
	type RequestBody struct {
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Company     *string `json:"company,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "position id is required",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(positionIDStr, ULIDPrefixPosition) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid position id",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid request body",
				}},
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "position not found",
						Source: &JSONAPIErrorSource{Parameter: "id"},
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "you do not have access to this resource",
				}},
			}, http.StatusForbidden)
			return
		}

		changed := false

		if body.Title != nil {
			title, err := NormalizeAndValidatePositionTitle(*body.Title)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "title must be between 4 and 64 characters",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/title"},
					}},
				}, http.StatusBadRequest)
				return
			case errors.Is(err, ErrPositionTitleHasURL):
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "title cannot contain a URL",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/title"},
					}},
				}, http.StatusBadRequest)
				return
			case err != nil:
				slog.Error("failed to validate position title", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}
			if position.Title != title {
				position.Title = title
				changed = true
			}
		}

		if body.Description != nil {
			description, err := NormalizeAndValidatePositionDescription(*body.Description)
			if errors.Is(err, ErrTextTooLong) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "description must be up to 2048 characters",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/description"},
					}},
				}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate description", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}
			if position.Description != description {
				position.Description = description
				changed = true
			}
		}

		if body.Company != nil {
			company, err := NormalizeAndValidatePositionCompanyName(*body.Company)
			if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "400",
						Title:  "Bad Request",
						Detail: "company name must be between 2 and 512 characters",
						Source: &JSONAPIErrorSource{Pointer: "/data/attributes/company"},
					}},
				}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate company name", "err", err)
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "500",
						Title:  "Internal Server Error",
					}},
				}, http.StatusInternalServerError)
				return
			}
			if position.Company != company {
				position.Company = company
				changed = true
			}
		}

		if body.IsActive != nil {
			if position.IsActive != *body.IsActive {
				position.IsActive = *body.IsActive
				changed = true
			}
		}

		if !changed {
			JSON(w, JSONAPIDocument{
				Meta: map[string]any{
					"status": "success",
				},
			}, http.StatusOK)
			return
		}

		err = a.Store.UpdatePosition(position)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "position not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}

func (a *API) HandlerV1DeleteMePosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "missing required role: recruiter",
				}},
			}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "position id is required",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(positionIDStr, ULIDPrefixPosition) {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "400",
					Title:  "Bad Request",
					Detail: "invalid position id",
					Source: &JSONAPIErrorSource{Parameter: "id"},
				}},
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "position not found",
						Source: &JSONAPIErrorSource{Parameter: "id"},
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "403",
					Title:  "Forbidden",
					Detail: "you do not have access to this resource",
				}},
			}, http.StatusForbidden)
			return
		}

		err = a.Store.DeletePosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, JSONAPIDocument{
					Errors: []JSONAPIError{{
						Status: "404",
						Title:  "Not Found",
						Detail: "position not found",
					}},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete position", "err", err)
			JSON(w, JSONAPIDocument{
				Errors: []JSONAPIError{{
					Status: "500",
					Title:  "Internal Server Error",
				}},
			}, http.StatusInternalServerError)
			return
		}

		JSON(w, JSONAPIDocument{
			Meta: map[string]any{
				"status": "success",
			},
		}, http.StatusOK)
	}
}
