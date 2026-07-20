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
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	reader := bytes.NewReader(requestBody)
	request, err := http.NewRequest(http.MethodPost, aiConfig.TEIBaseURL, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to init new request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+aiConfig.TEIAPIKey)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.Error("failed to close response body", "err", err)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding endpoint returned %d", response.StatusCode)
	}

	var parsed EmbeddingsResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
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

//go:embed migrations/*
var EmbeddedStatic embed.FS

var (
	RegexHTMLTag = regexp.MustCompile(`(?s)<[^>]*>`)
	RegexJSTag   = regexp.MustCompile(`(?is)<script.*?>.*?</script>`)
	RegexSQL     = regexp.MustCompile(`(?i)\b(select|insert|update|delete|drop|truncate|alter)\b`)
)

const DefaultEmbeddingsBatchSize = 64

func (a *API) RunEmbeddingsJob(c AIConfig) error {
	slog.Debug("running embeddings job")

	// TODO: Test this store method, rethink it once more
	// https://github.com/akvachan/hirevec-core/issues/21
	ids, texts, err := a.Store.FetchPendingEmbeddingsMetadata(DefaultEmbeddingsBatchSize)
	if err != nil {
		return fmt.Errorf("failed to fetch pending emebeddings metadata: %w")
	}
	if len(ids) == 0 {
		slog.Debug("empty pending embeddings metadata, skipping run...")
		return nil
	}

	// TODO: Test this store method, rethink it once more
	// https://github.com/akvachan/hirevec-core/issues/21
	batchOut, err := CreateEmbeddings(c, texts)
	if err != nil {
		// TODO: Test this store method, rethink it once more
		// https://github.com/akvachan/hirevec-core/issues/21
		if err := a.Store.MarkEmbeddingsStatus(ids, EmbeddingStatusPending); err != nil {
			return fmt.Errorf("failed to mark embeddings status: %w", err)
		}
	}

	tx, err := a.Store.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// TODO: Test this store method, rethink it once more
	// https://github.com/akvachan/hirevec-core/issues/21
	if err := a.Store.UpsertEmbeddingsTx(tx, ids, batchOut); err != nil {
		if err := tx.Rollback(); err != nil {
			slog.Error("failed to rollback transaction", "err", err)
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		slog.Error("failed to upsert embeddings", "err", err)
		return fmt.Errorf("failed to upsert embeddings: %w", err)
	}

	// TODO: Test this store method, rethink it once more
	// https://github.com/akvachan/hirevec-core/issues/21
	if err := a.Store.MarkEmbeddingsStatusTx(tx, ids, EmbeddingStatusDone); err != nil {
		if err := tx.Rollback(); err != nil {
			slog.Error("failed to rollback transaction", "err", err)
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		slog.Error("failed to mark ebeddings status", "err", err)
		return fmt.Errorf("failed to mark ebeddings status: %w", err)
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
	slog.Debug("running recommendations job")

	candidateIDs, err := a.Store.GetCandidates(
		DefaultCandidatesBatchSize,
		DefaultRecommendationsJobFrequency,
	)
	if err != nil {
		return fmt.Errorf("failed to get candidates: %w", err)
	}
	if len(candidateIDs) == 0 {
		slog.Debug("candidates list is empty, skipping run...")
		return nil
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
			slog.Debug("positions list is empty, skipping candidate...", "candidateID", candidateIDs[i])
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
		for i := range limit {
			if _, err := a.Store.CreateRecommendation(positionIDs[i], candidateIDs[i]); err != nil {
				return fmt.Errorf("failed to create recommendation: %w", err)
			}
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
		Mux:   http.NewServeMux(),
	}

	api.Server = http.Server{
		Addr:         c.ServerBaseURL,
		ReadTimeout:  c.RequestReadTimeout,
		WriteTimeout: c.RequestWriteTimeout,
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		Handler:      api.Mux,
	}

	api.RegisterRoutes()

	return &api
}

var ErrFailedShutdownServer = errors.New("failed to shutdown server")

func (a *API) WaitAndShutdown(ctx context.Context, errCh chan error, gracePeriod time.Duration) error {
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case <-errCh:
		return ErrFailedShutdownServer
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), gracePeriod)
	defer cancel()

	slog.Info("starting graceful shutdown", "timeout", gracePeriod)
	if err := a.Server.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to gracefully shutdown, forcing close", "err", err)
		if err := a.Server.Close(); err != nil {
			slog.Error("failed to force close server", "err", err)
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

	slog.Debug("creating listener", "addr", api.Server.Addr)
	listener, err := net.Listen("tcp", api.Server.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	slog.Debug("starting server", "addr", api.Server.Addr)
	errCh := make(chan error, 1)
	go func() {
		if err := api.Server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
				if err := api.RunEmbeddingsJob(aiConfig); err != nil {
					slog.Error("failed to run embeddings job", "err", err)
				}
			}
		}()
	}

	go func() {
		for range time.Tick(c.RecommendationsJobFrequency) {
			if err := api.RunRecommendationsJob(aiConfig); err != nil {
				slog.Error("failed to run recommendations job", "err", err)
			}
		}
	}()

	if err := api.WaitAndShutdown(ctx, errCh, c.GracePeriod); err != nil {
		slog.Error("failed to wait and shutdown", "err", err)
		return fmt.Errorf("failed to wait and shutdown: %w", err)
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
}

// Error represents a single error object returned when a request fails.
type Error struct {
	// Code is an application-specific error identifier used for programmatic handling.
	Code string `json:"code,omitempty"`

	// Message provides a detailed explanation of the error.
	Message string `json:"detail,omitempty"`

	// Source identifies the specific field or parameter that caused the error.
	Source ErrorSource `json:"source,omitempty"`
}

// ErrorSource identifies where an error originated in the request.
type ErrorSource struct {
	// Pointer is a JSON Pointer to the offending value in the request body.
	Body string `json:"body,omitempty"`

	// Parameter is the query string parameter that caused the error.
	Parameter string `json:"parameter,omitempty"`

	// Header is a string indicating the name of a single request header which caused the error.
	Header string `json:"header,omitempty"`

	// Cookie is a string indicating the name of a cookie which caused the error.
	Cookie string `json:"cookie,omitempty"`
}

// Links contains URLs to other related pages.
type Links struct {
	// Next is a link to the next page in a paginated response.
	Next string `json:"next,omitempty"`

	// Previous is a link to the previous page in a paginated response.
	Previous string `json:"previous,omitempty"`
}

// Meta contains that contains data that does not belong to Data, Errors, or Links.
type Meta struct {
	// Page contains metadata about current page
	Page Page `json:"page,omitempty"`
}

// Envelope is a wrapper that contains zero or more fields.
type Envelope struct {
	// Data contains any main data to be consumed.
	Data any `json:"data,omitempty"`

	// Errors contains one or more errors when the request fails.
	Errors []Error `json:"errors,omitempty"`

	// Links contains top-level navigation or pagination URLs.
	Links Links `json:"links,omitempty"`

	// Meta contains additional information that is not links or errors or data.
	Meta Meta `json:"meta,omitempty"`
}

func JSON(w http.ResponseWriter, responseBody Envelope, status int) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(responseBody); err != nil {
		slog.Error("failed to encode response", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Debug("failed to write response to client", "err", err)
		w.Header().Del("Content-Type")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
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
				w.WriteHeader(http.StatusInternalServerError)
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

func GetPageFromQuery(r *http.Request) Page {
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

const (
	ErrorCodeBearerTokenRequired = "bearer_token_required"
	ErrorCodeInvalidAccessToken  = "invalid_access_token"
	ErrorCodeUnauthorized        = "unauthorized"
)

func (a *API) MiddlewareAuth(roles map[Role]bool) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			bearer, found := strings.CutPrefix(authHeader, "Bearer ")
			if !found || bearer == "" {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeBearerTokenRequired,
					Message: "Bearer token is required",
					Source:  ErrorSource{Header: "Accept"},
				}}}, http.StatusUnauthorized)
				return
			}

			claims, err := a.Vault.ParseAccessToken(bearer)
			if err != nil {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidAccessToken,
					Message: "Invalid access token",
					Source:  ErrorSource{Header: "Accept"},
				}}}, http.StatusBadRequest)
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
					JSON(w, Envelope{Errors: []Error{{
						Code:   ErrorCodeUnauthorized,
						Source: ErrorSource{Header: "Accept"},
					}}}, http.StatusUnauthorized)
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ContextKeyClaims, claims)))
		}
	}
}

const ErrorCodeUnsupportedMediaType = "unsupported_media_type"

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
	Method      Method
	Handler     http.HandlerFunc
	Middlewares []Middleware
	Route       Route
	Roles       []Role
}

func (a *API) PublicRoute(c RouteConfig) {
	handler := MiddlewareChain(
		c.Handler,
		a.MiddlewareLogging,
		a.MiddlewarePanicRecovery,
		a.MiddlewareMaxBytesLimit,
	)
	handler = MiddlewareChain(handler, c.Middlewares...)

	a.Mux.Handle(
		fmt.Sprintf("%s %s", c.Method, c.Route),
		handler,
	)
}

func (a *API) PrivateRoute(c RouteConfig) {
	rolesMap := make(map[Role]bool)
	for _, role := range c.Roles {
		rolesMap[role] = true
	}

	handler := MiddlewareChain(
		c.Handler,
		a.MiddlewareLogging,
		a.MiddlewarePanicRecovery,
		a.MiddlewareMaxBytesLimit,
		a.MiddlewareAuth(rolesMap),
	)
	handler = MiddlewareChain(handler, c.Middlewares...)

	a.Mux.Handle(
		fmt.Sprintf("%s %s", c.Method, c.Route),
		handler,
	)
}

type Route string

const (
	// TODO: Docs
	// RouteOpenAPI Route = "/openapi.json"
	// RouteDocs    Route = "/docs"

	// Misc
	RouteHealth Route = "/health"

	// Authentication and authorization
	RouteCallback         Route = "/callback"
	RouteLoginViaEmail    Route = "/login"
	RouteLoginViaProvider Route = "/login/{provider}"
	RouteRefresh          Route = "/refresh"

	// Resources and collections
	RouteCandidate       Route = "/candidate"
	RouteCandidates      Route = "/candidates"
	RouteMatches         Route = "/matches"
	RoutePosition        Route = "/positions/{id}"
	RoutePositions       Route = "/positions"
	RouteReactions       Route = "/reactions"
	RouteRecommendations Route = "/recommendations"
	RouteRecruiter       Route = "/recruiter"
	RouteRecruiters      Route = "/recruiters"
	RouteUser            Route = "/user"
	RouteUsers           Route = "/users"
)

func (a *API) RegisterRoutes() {
	slog.Debug("registering routes")

	// TODO: Document routes in openapi.json
	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	// a.PublicRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RouteOpenAPI,
	// 	Handler: a.HandlerOpenAPI,
	// })
	//
	// a.PublicRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RouteDocs,
	// 	Handler: a.HandlerDocs,
	// })

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteLoginViaProvider,
		Handler: a.HandlerLoginViaProvider(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteLoginViaProvider,
		Handler: a.HandlerLoginViaProvider(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteCallback,
		Handler: a.HandlerSSOCallback(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteCallback,
		Handler: a.HandlerSSOCallback(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteHealth,
		Handler: a.HandlerHealth,
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteLoginViaEmail,
		Handler: a.HandlerLoginViaEmail(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteRefresh,
		Handler: a.HandlerRefresh(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PublicRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteUsers,
		Handler: a.HandlerCreateUser(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteUser,
		Handler: a.HandlerGetUser(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RouteUser,
		Handler: a.HandlerPatchUser(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteUser,
		Handler: a.HandlerDeleteUser(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteCandidates,
		Handler: a.HandlerCreateCandidate(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteCandidate,
		Handler: a.HandlerGetCandidate(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RouteCandidate,
		Handler: a.HandlerPatchCandidate(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteCandidate,
		Handler: a.HandlerDeleteCandidate(),
		Roles:   []Role{RoleCandidate},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteRecruiters,
		Handler: a.HandlerCreateRecruiter(),
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteRecruiter,
		Handler: a.HandlerGetRecruiter(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RouteRecruiter,
		Handler: a.HandlerDeleteRecruiter(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RoutePositions,
		Handler: a.HandlerCreatePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RoutePositions,
		Handler: a.HandlerGetPositions(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RoutePosition,
		Handler: a.HandlerGetPosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPatch,
		Route:   RoutePosition,
		Handler: a.HandlerPatchPosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodDelete,
		Route:   RoutePosition,
		Handler: a.HandlerDeletePosition(),
		Roles:   []Role{RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteRecommendations,
		Handler: a.HandlerGetRecommendations(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteReactions,
		Handler: a.HandlerGetReactions(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodPost,
		Route:   RouteReactions,
		Handler: a.HandlerCreateReaction(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})

	// TODO: Document the route in openapi.json
	// https://github.com/akvachan/hirevec-core/issues/33
	a.PrivateRoute(RouteConfig{
		Method:  MethodGet,
		Route:   RouteMatches,
		Handler: a.HandlerGetMatches(),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	})
}

func (a *API) CreateAccessToken(w http.ResponseWriter, userID ULID, provider Provider, roles map[Role]ULID) {
	accessToken, err := a.Vault.CreateAccessToken(userID, provider, roles)
	if err != nil {
		slog.Error("failed to create access token", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	JSON(w, Envelope{Data: map[string]any{
		"access_token": accessToken.AccessToken,
		"token_type":   accessToken.TokenType,
		"expires_in":   accessToken.ExpiresIn,
		"scope":        accessToken.Scope,
		"user_id":      accessToken.UserID,
	}}, http.StatusOK)
}

const (
	ErrorCodeInvalidRequestBody   = "invalid_request_body"
	ErrorCodeInvalidGrantType     = "invalid_grant_type"
	ErrorCodeRefreshTokenRequired = "refresh_token_required"
	ErrorCodeInvalidRefreshToken  = "invalid_refresh_token"
)

type RequestBodyRefresh struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerRefresh() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBodyRefresh](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		if body.GrantType != "refresh_token" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidGrantType,
				Message: "grant_type must be refresh_token",
				Source:  ErrorSource{Body: "/data/grant_type"},
			}}}, http.StatusBadRequest)
			return
		}

		if body.RefreshToken == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeRefreshTokenRequired,
				Message: "refresh_token is required",
				Source:  ErrorSource{Body: "/data/refresh_token"},
			}}}, http.StatusBadRequest)
			return
		}

		claims, err := a.Vault.ParseRefreshToken(body.RefreshToken)
		if err != nil {
			slog.Error(
				"failed to parse refresh token",
				"ip", r.RemoteAddr,
				"err", err,
			)
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRefreshToken,
				Message: "Invalid refresh token",
				Source:  ErrorSource{Body: "/data/refresh_token"},
			}}}, http.StatusBadRequest)
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
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidRefreshToken,
					Message: "Invalid refresh token",
					Source:  ErrorSource{Body: "/data/refresh_token"},
				}}}, http.StatusBadRequest)
				return
			}
			slog.Error(
				"failed to validate refresh token",
				"err", err,
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if isRevoked {
			slog.Warn(
				"revoked token reuse attempt",
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRefreshToken,
				Message: "Invalid refresh token",
				Source:  ErrorSource{Body: "/data/meta/refresh_token"},
			}}}, http.StatusBadRequest)
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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		a.CreateAccessToken(w, claims.UserID, claims.Provider, roles)
	}
}

type RequestBodyLoginViaEmail struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

const (
	ErrorCodePasswordRequired   = "password_required"
	ErrorCodeInvalidCredentials = "invalid_credentials"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerLoginViaEmail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBodyLoginViaEmail](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		missingEmail := body.Email == ""
		missingPassword := body.Password == ""
		if missingPassword || missingEmail {
			errors := make([]Error, 0, 2)
			if missingEmail {
				errors = append(errors, Error{
					Code:    ErrorCodeEmailRequired,
					Message: "Email is required",
					Source:  ErrorSource{Body: "/data/email"},
				})
			}
			if missingPassword {
				errors = append(errors, Error{
					Code:    ErrorCodePasswordRequired,
					Message: "Password is required",
					Source:  ErrorSource{Body: "/data/password"},
				})
			}
			JSON(w, Envelope{Errors: errors}, http.StatusBadRequest)
			return
		}

		user, roles, err := a.Store.GetUserAndRolesByEmail(body.Email, ProviderEmail)
		switch {
		case errors.Is(err, ErrUserNotFound):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidCredentials,
				Message: "Invalid credentials",
			}}}, http.StatusUnauthorized)
			return

		case errors.Is(err, ErrUserNoRole):
			accessToken, err := a.Vault.CreateAccessToken(user.ID, user.Provider, map[Role]ULID{})
			if err != nil {
				slog.Error("failed to create access token", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			JSON(w, Envelope{Data: accessToken}, http.StatusOK)

		case err != nil:
			slog.Error("failed to get user by email", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !IsValidPassword(user.PasswordHash, body.Password) {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidCredentials,
				Message: "Invalid credentials",
			}}}, http.StatusUnauthorized)
			return
		}

		jti, err := a.Store.CreateRefreshToken(user.ID)
		if err != nil {
			slog.Error("failed to create refresh token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tokenPair, err := a.Vault.CreateTokenPair(user.ID, user.Provider, jti, roles)
		if err != nil {
			slog.Error("failed to create token pair", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, Envelope{Data: tokenPair}, http.StatusOK)
	}
}

const ErrorCodeInvalidProvider = "invalid_provider"

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerLoginViaProvider() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := Provider(r.PathValue("provider"))
		if provider != ProviderGoogle && provider != ProviderApple {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidProvider,
				Message: "Provider must be google or apple",
				Source:  ErrorSource{Parameter: "provider"},
			}}}, http.StatusBadRequest)
			return
		}

		state, err := a.Vault.CreateStateToken(provider)
		if err != nil {
			slog.Error("failed to generate state token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		parsed, err := a.Vault.ParseStateToken(state)
		if err != nil {
			slog.Error("failed to parse state token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
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
			slog.Error("failed to generate auth code URL", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

var ErrEmailNotVerified = errors.New("email not verified")

const (
	ErrorCodeMissingState          = "missing_state"
	ErrorCodeInvalidState          = "invalid_state"
	ErrorCodeInvalidCSRF           = "invalid_csrf"
	ErrorCodeMissingVerifier       = "missing_verifier"
	ErrorCodeAuthorizationProvider = "authorization_provider_error"
	ErrorCodeMissingCode           = "missing_code"
	ErrorCodeIDTokenRequired       = "id_token_required"
	ErrorCodeInvalidIDToken        = "invalid_id_token"
	ErrorCodeFailedParseClaims     = "failed_parse_claims"
	ErrorCodeUnverifiedEmail       = "unverified_email"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerSSOCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), a.Vault.StateTokenExpiration)
		defer cancel()

		stateString := r.URL.Query().Get("state")
		if stateString == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingState,
				Message: "Missing state",
				Source:  ErrorSource{Parameter: "state"},
			}}}, http.StatusBadRequest)
			return
		}

		state, err := a.Vault.ParseStateToken(stateString)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidState,
				Message: "Invalid state",
				Source:  ErrorSource{Parameter: "state"},
			}}}, http.StatusBadRequest)
			return
		}

		csrfCookie, err := r.Cookie("oauth_csrf")
		if err != nil || csrfCookie.Value != state.CSRF {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidCSRF,
				Message: "Invalid CSRF",
				Source:  ErrorSource{Cookie: "oauth_csrf"},
			}}}, http.StatusBadRequest)
			return
		}

		verifierCookie, err := r.Cookie("oauth_verifier")
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingVerifier,
				Message: "Missing Verifier",
				Source:  ErrorSource{Cookie: "oauth_verifier"},
			}}}, http.StatusBadRequest)
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeAuthorizationProvider,
				Message: "Authorization provider returned error",
				Source:  ErrorSource{Parameter: "error"},
			}}}, http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingCode,
				Message: "Missing authorization code",
				Source:  ErrorSource{Parameter: "code"},
			}}}, http.StatusBadRequest)
			return
		}

		// Delete cookies
		for _, name := range [2]string{"oauth_csrf", "oauth_verifier"} {
			http.SetCookie(w, &http.Cookie{
				Name:     name,
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		var idToken IDToken
		var idTokenErr error

		switch Provider(r.PathValue("provider")) {
		case ProviderGoogle:
			rawIDToken, err := a.Vault.ExchangeGoogleCodeForIDToken(ctx, code, verifierCookie)
			if errors.Is(err, ErrIDTokenRequired) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeIDTokenRequired,
					Message: "id_token is required",
				}}}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to exchange Google code", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			idToken, idTokenErr = a.Vault.VerifyAndParseGoogleIDToken(ctx, rawIDToken)

		case ProviderApple:
			idTokenString, err := a.Vault.ExchangeAppleCodeForIDToken(
				ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeIDTokenRequired,
					Message: "id_token is required",
				}}}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to exchange Apple code", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			idToken, idTokenErr = a.Vault.VerifyAndParseAppleIDToken(
				ctx,
				idTokenString,
				r.FormValue("user"),
			)

		default:
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeIDTokenRequired,
				Message: "Invalid provider; must be one of: google, apple",
				Source:  ErrorSource{Parameter: "provider"},
			}}}, http.StatusBadRequest)
			return
		}

		switch {
		case errors.Is(idTokenErr, ErrInvalidIDToken):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidIDToken,
				Message: "Invalid id_token",
			}}}, http.StatusBadRequest)
			return

		case errors.Is(idTokenErr, ErrFailedParseClaims):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeFailedParseClaims,
				Message: "Failed to parse claims",
			}}}, http.StatusBadRequest)
			return

		case errors.Is(idTokenErr, ErrEmailNotVerified):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeUnverifiedEmail,
				Message: "Unverified email",
			}}}, http.StatusBadRequest)
			return

		case idTokenErr != nil:
			slog.Error("failed to verify Google ID token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		userID, roles, err := a.Store.GetUserIDAndRolesByProvider(idToken.Provider, idToken.ProviderUserID)
		switch {
		case errors.Is(err, ErrUserNotFound):
			userName, err := GenerateUserName()
			if err != nil {
				slog.Error("failed to generate user name", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			fullName, err := NormalizeAndValidateUserFullName(idToken.FullName)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserFullNameFormat,
					Message: ErrorMessageUserFullNameWrongSize,
				}}}, http.StatusBadRequest)
				return
			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserFullNameFormat,
					Message: ErrorMessageUserFullNameForbiddenChars,
				}}}, http.StatusBadRequest)
				return
			case err != nil:
				slog.Error("failed to normalize and validate full name", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			userID, err := a.Store.CreateUser(idToken.Provider, idToken.ProviderUserID, idToken.Email, fullName, userName, "")
			if err != nil {
				slog.Error("failed to create user", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			accessToken, err := a.Vault.CreateAccessToken(userID, idToken.Provider, map[Role]ULID{})
			if err != nil {
				slog.Error("failed to create access token", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Cache-Control", "no-store")
			JSON(w, Envelope{Data: accessToken}, http.StatusOK)
			return

		case errors.Is(err, ErrUserNoRole):
			accessToken, err := a.Vault.CreateAccessToken(userID, idToken.Provider, map[Role]ULID{})
			if err != nil {
				slog.Error("failed to create access token", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Cache-Control", "no-store")
			JSON(w, Envelope{Data: accessToken}, http.StatusOK)
			return

		case err != nil:
			slog.Error("failed to get user by provider", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jti, err := a.Store.CreateRefreshToken(userID)
		if err != nil {
			slog.Error("failed to create refresh token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tokenPair, err := a.Vault.CreateTokenPair(userID, idToken.Provider, jti, roles)
		if err != nil {
			slog.Error("failed to create token pair", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		JSON(w, Envelope{Data: tokenPair}, http.StatusOK)
		return
	}
}

var RegexFullName = regexp.MustCompile(`^[\pL][\pL\s'’-]{2,512}\z`)

const (
	DefaultUserFullNameMinLength = 2
	DefaultUserFullNameMaxLength = 512
)

var (
	ErrTextForbiddenChars = errors.New("text contains forbidden characters")
	ErrTextTooShort       = errors.New("text too short")
	ErrTextTooLong        = errors.New("text too long")
)

// TODO: Return an array of errors instead of one by one.
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
	ErrorMessageUserFullNameWrongSize      = fmt.Sprintf("full_name must be between %v and %v characters", DefaultUserFullNameMinLength, DefaultUserFullNameMaxLength)
	ErrorMessageUserFullNameForbiddenChars = "full_name must be a valid 'passport-style' full name. It must start with a letter and can only contain letters, spaces, apostrophes, or hyphens"
)

var ErrExtraDataDecoded = errors.New("extra data decoded")

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
		return data, fmt.Errorf("failed to close body: %w", err)
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
			return 0, fmt.Errorf("failed to generate random bytes: %w", err)
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

////go:embed openapi.json
// var OpenAPISchema []byte
//
// func (a *API) HandlerOpenAPI(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Content-Type", "application/json")
// 	w.Write(OpenAPISchema)
// }
//
// const SwaggerUIHTML = `
// <!DOCTYPE html>
// <html lang="en">
// <head>
//     <meta charset="utf-8" />
//     <meta name="viewport" content="width=device-width, initial-scale=1" />
//     <meta name="description" content="SwaggerUI" />
//     <title>Docs</title>
//     <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
// </head>
// <body>
//     <div id="swagger-ui"></div>
//     <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
//     <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js" crossorigin></script>
//     <script>
//         window.onload = () => {
//             window.ui = SwaggerUIBundle({
//                 url: '/openapi.json',
//                 dom_id: '#swagger-ui',
//                 deepLinking: true,
//                 presets: [
//                     SwaggerUIBundle.presets.apis,
//                     SwaggerUIStandalonePreset
//                 ],
//                 layout: "BaseLayout"
//             });
//         };
//     </script>
// </body>
// </html>
// `
//
// func (a *API) HandlerDocs(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Content-Type", "text/html; charset=utf-8")
// 	fmt.Fprint(w, SwaggerUIHTML)
// }

type ResponseDataHealth struct {
	Status string `json:"status"`
}

func (a *API) HandlerHealth(w http.ResponseWriter, r *http.Request) {
	JSON(w, Envelope{Data: ResponseDataHealth{
		Status: "ok",
	}}, http.StatusOK)
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetRecommendations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, isCandidate := claims.Roles[RoleCandidate]
		recruiterID, isRecruiter := claims.Roles[RoleRecruiter]
		if !isCandidate && !isRecruiter {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing one of required roles: recruiter, candidate",
			}}}, http.StatusForbidden)
			return
		}

		q := r.URL.Query()
		page := GetPageFromQuery(r)

		page.Count = 0
		positionCursor := q.Get("pos_cursor")
		positionNextCursor, candidateNextCursor := "done", "done"
		var data struct {
			RecommendationsForCandidate []RecommendationForCandidate `json:"position_recommendations"`
			RecommendationsForRecruiter []RecommendationForRecruiter `json:"candidate_recommendations"`
		}
		if isCandidate && positionCursor != "done" {
			recommendations, cursor, err := a.Store.GetRecommendationsForCandidate(
				candidateID,
				Page{Cursor: positionCursor, Limit: page.Limit},
				true,
			)
			if err != nil {
				slog.Error("failed to fetch position recommendations", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			positionNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recommendations)
			data.RecommendationsForCandidate = recommendations
		}

		candidateCursor := q.Get("can_cursor")
		if isRecruiter && candidateCursor != "done" {
			recommendations, cursor, err := a.Store.GetRecommendationsForRecruiter(
				recruiterID,
				Page{Cursor: candidateCursor, Limit: page.Limit},
				true,
			)
			if err != nil {
				slog.Error("failed to fetch candidate recommendations", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			candidateNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recommendations)
			data.RecommendationsForRecruiter = recommendations
		}

		var links Links
		if positionNextCursor != "done" || candidateNextCursor != "done" {
			nextHref := fmt.Sprintf(
				"%s?pos_cursor=%s&can_cursor=%s&limit=%d",
				RouteRecommendations, positionNextCursor, candidateNextCursor, page.Limit,
			)
			links.Next = nextHref
		}

		JSON(w, Envelope{
			Data:  data,
			Meta:  Meta{Page: page},
			Links: links,
		}, http.StatusOK)
	}
}

type RequestCreateReaction struct {
	RecommendationID ULID         `json:"recommendation_id"`
	ReactionType     ReactionType `json:"reaction_type"`
}

const (
	ErrorCodeRecommendationIDRequired = "recommendation_id_required"
	ErrorCodeRecommendationNotFound   = "recommendation_not_found"
	ErrorCodeReactionExists           = "reaction_exists"
	ErrorCodeInvalidReactionType      = "invalid_reaction_type"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateReaction() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestCreateReaction](r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !body.ReactionType.IsValid() {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidReactionType,
				Message: "Must be one of: positive, negative, neutral",
				Source:  ErrorSource{Body: "/data/reaction_type"},
			}}}, http.StatusBadRequest)
			return
		}

		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		if body.RecommendationID == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeRecommendationIDRequired,
				Message: "Recommendation ID is required",
				Source:  ErrorSource{Parameter: "id"},
			}}}, http.StatusBadRequest)
			return
		}

		recommendation, err := a.Store.GetRecommendation(body.RecommendationID)
		if err != nil {
			if errors.Is(err, ErrRecommendationNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeRecommendationNotFound,
					Message: "Recommendation not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch recommendation", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if recommendation.CandidateID != candidateID {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := a.Store.CreateReaction(body.RecommendationID, ReactorTypeCandidate, candidateID, body.ReactionType); err != nil {
			if errors.Is(err, ErrReactionAlreadyExists) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeReactionExists,
					Message: "Reaction already exists; reactions are immutable",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusConflict)
				return
			}
			slog.Error("failed to record reaction", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetReactions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		page := GetPageFromQuery(r)

		reactions, nextCursor, err := a.Store.GetReactionsByCandidateID(candidateID, page)
		if err != nil {
			slog.Error("failed to fetch reactions", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		page.Count = len(reactions)
		page.HasNext = nextCursor != ""

		var links Links
		if nextCursor != "" {
			links.Next = fmt.Sprintf(
				"%s?cursor=%s",
				RouteReactions,
				nextCursor,
			)
		}

		JSON(w, Envelope{
			Data:  reactions,
			Links: links,
			Meta:  Meta{Page: page},
		}, http.StatusOK)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetMatches() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		page := GetPageFromQuery(r)
		matches, nextCursor, err := a.Store.GetMatchesByCandidateID(candidateID, page)
		if err != nil {
			slog.Error("failed to fetch matches", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		page.Count = len(matches)
		page.HasNext = nextCursor != ""

		var links Links
		if nextCursor != "" {
			links.Next = fmt.Sprintf(
				"%s?cursor=%s&limit=%d",
				RouteMatches,
				nextCursor,
				page.Limit,
			)
		}

		JSON(w, Envelope{
			Data:  matches,
			Links: links,
			Meta:  Meta{Page: page},
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

var (
	ErrPasswordHasNoUpper   = errors.New("password has no upper letter")
	ErrPasswordHasNoDigit   = errors.New("password has no digit")
	ErrPasswordHasNoSpecial = errors.New("password has no special character")
	ErrPasswordHasNoLower   = errors.New("password has no lower letter")
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

type RequestBodyCreateUser struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

const (
	ErrorCodeEmailRequired             = "email_required"
	ErrorCodeInvalidUserEmailFormat    = "invalid_user_email_format"
	ErrorCodeInvalidUserPasswordFormat = "invalid_user_password_format"
	ErrorCodeInvalidUserFullNameFormat = "invalid_user_full_name_format"
	ErrorCodeUserAlreadyExists         = "user_exists"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBodyCreateUser](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		if body.Email == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeEmailRequired,
				Message: "Email is required",
				Source:  ErrorSource{Body: "/data/email"},
			}}}, http.StatusBadRequest)
			return
		}

		email, err := mail.ParseAddress(body.Email)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserEmailFormat,
				Message: "Invalid email",
				Source:  ErrorSource{Body: "/data/email"},
			}}}, http.StatusBadRequest)
			return
		}

		switch err = ValidateUserPassword(body.Password); {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserPasswordFormat,
				Message: "Password must be between 8 and 128 characters",
				Source:  ErrorSource{Body: "/data/password"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoLower):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserPasswordFormat,
				Message: "Password must contain at least one lowercase letter",
				Source:  ErrorSource{Body: "/data/password"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoUpper):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserPasswordFormat,
				Message: "Password must contain at least one uppercase letter",
				Source:  ErrorSource{Body: "/data/password"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoDigit):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserPasswordFormat,
				Message: "Password must contain at least one digit",
				Source:  ErrorSource{Body: "/data/password"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoSpecial):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserPasswordFormat,
				Message: "Password must contain at least one special character",
				Source:  ErrorSource{Body: "/data/password"},
			}}}, http.StatusBadRequest)
			return

		case err != nil:
			slog.Error("failed to validate password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		fullName, err := NormalizeAndValidateUserFullName(body.FullName)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserFullNameFormat,
				Message: ErrorMessageUserFullNameWrongSize,
				Source:  ErrorSource{Body: "/data/full_name"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrTextForbiddenChars):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidUserFullNameFormat,
				Message: ErrorMessageUserFullNameForbiddenChars,
				Source:  ErrorSource{Body: "/data/full_name"},
			}}}, http.StatusBadRequest)
			return

		case err != nil:
			slog.Error("failed to validate full name", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		exists, err := a.Store.UserExistsByEmail(email.Address, ProviderEmail)
		if err != nil {
			slog.Error("failed to check user existance", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if exists {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeUserAlreadyExists,
				Message: "User with the provided email already exists",
				Source:  ErrorSource{Body: "data/email"},
			}}}, http.StatusConflict)
			return
		}

		userName, err := GenerateUserName()
		if err != nil {
			slog.Error("failed to generate a user name", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		passwordHash, err := HashPassword(body.Password)
		if err != nil {
			slog.Error("failed to hash password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		userID, err := a.Store.CreateUser(ProviderEmail, "", email.Address, fullName, userName, passwordHash)
		if err != nil {
			slog.Error("failed to create user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jti, err := a.Store.CreateRefreshToken(userID)
		if err != nil {
			slog.Error("failed to create refresh token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tokenPair, err := a.Vault.CreateTokenPair(userID, ProviderEmail, jti, map[Role]ULID{})
		if err != nil {
			slog.Error("failed to create token pair", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		JSON(w, Envelope{Data: tokenPair}, http.StatusCreated)
	}
}

const ErrorCodeRecruiterExists = "recruiter_exists"

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateRecruiter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err := a.Store.CreateRecruiter(claims.UserID); err != nil {
			if errors.Is(err, ErrRecruiterAlreadyExists) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeRecruiterExists,
					Message: "Recruiter already exists",
					Source:  ErrorSource{Body: "data/user_id"},
				}}}, http.StatusConflict)
				return
			}
			slog.Error("failed to create recruiter", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		roles, err := a.Store.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error("failed to get user roles", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		accessToken, err := a.Vault.CreateAccessToken(claims.UserID, claims.Provider, roles)
		if err != nil {
			slog.Error("failed to create access token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		JSON(w, Envelope{Data: accessToken}, http.StatusCreated)
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

type RequestBodyCreateCandidate struct {
	About string `json:"about"`
}

const (
	ErrorCodeCandidateAlreadyExists      = "candiate_already_exists"
	ErrorCodeInvalidCandidateAboutFormat = "invalid_candidate_about_format"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateCandidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		body, err := DecodeRequestBody[RequestBodyCreateCandidate](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		about, err := NormalizeAndValidateCandidateAbout(body.About)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidCandidateAboutFormat,
				Message: "About must be up to 1024 characters",
				Source:  ErrorSource{Body: "data/about"},
			}}}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate about", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err = a.Store.CreateCandidate(claims.UserID, about); err != nil {
			if errors.Is(err, ErrCandidateAlreadyExists) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeCandidateAlreadyExists,
					Message: "Candidate already exists",
					Source:  ErrorSource{Body: "data/user_id"},
				}}}, http.StatusConflict)
				return
			}
			slog.Error("failed to create candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		roles, err := a.Store.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error("failed to get user roles", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		accessToken, err := a.Vault.CreateAccessToken(claims.UserID, claims.Provider, roles)
		if err != nil {
			slog.Error("failed to create access token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		JSON(w, Envelope{Data: accessToken}, http.StatusCreated)
	}
}

const (
	ErrorCodeInternalServerError = "internal_server_error"
	ErrorCodeUserNotFound        = "user_not_found"
	ErrorCodeForbidden           = "forbidden"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if errors.Is(err, ErrUserNotFound) {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeUserNotFound,
				Message: "User not found",
			}}}, http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("failed to get user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, Envelope{Data: user}, http.StatusOK)
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
	ErrorMessageUserNameWrongSize      = "user_name must be between 4 and 32 characters"
	ErrorMessageUserNameForbiddenChars = "user_name can only contain underscores, latin characters and numbers"
)

type RequestBodyPatchUser struct {
	FullName *string `json:"full_name"`
	UserName *string `json:"user_name"`
}

const (
	ErrorCodeResourceTypeMismatch      = "resource_type_mismatch"
	ErrorCodeResourceIDMismatch        = "resource_id_mismatch"
	ErrorCodeInvalidUserUserNameFormat = "invalid_user_user_name_format"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerPatchUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		body, err := DecodeRequestBody[RequestBodyPatchUser](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeUserNotFound,
					Message: "User not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		changed := false
		if body.FullName != nil {
			fullName, err := NormalizeAndValidateUserFullName(*body.FullName)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserFullNameFormat,
					Message: ErrorMessageUserFullNameWrongSize,
					Source:  ErrorSource{Body: "data/full_name"},
				}}}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserFullNameFormat,
					Message: ErrorMessageUserFullNameForbiddenChars,
					Source:  ErrorSource{Body: "data/full_name"},
				}}}, http.StatusBadRequest)
				return

			case err != nil:
				slog.Error("failed to validate full name", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
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
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserUserNameFormat,
					Message: ErrorMessageUserNameWrongSize,
					Source:  ErrorSource{Body: "data/user_name"},
				}}}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidUserUserNameFormat,
					Message: ErrorMessageUserNameForbiddenChars,
					Source:  ErrorSource{Body: "data/user_name"},
				}}}, http.StatusBadRequest)
				return

			case err != nil:
				slog.Error("failed to validate user name", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if user.UserName != userName {
				user.UserName = userName
				changed = true
			}
		}

		if !changed {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err = a.Store.UpdateUser(user.ID, user.FullName, user.UserName); err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeUserNotFound,
					Message: "User not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerDeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		userIDStr := r.PathValue(("id"))
		userID := ULID(userIDStr)
		if userID != claims.UserID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have permission to access or modify the requested resource",
			}}}, http.StatusForbidden)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeUserNotFound,
					Message: "User not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = a.Store.DeleteUser(user.ID); err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeUserNotFound,
					Message: "User not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

const (
	ErrorCodeMissingRequiredRole = "missing_required_role"
	ErrorCodeCandidateNotFound   = "candidate_not_found"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetCandidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		urlCandidateID := ULID(r.PathValue("id"))
		if urlCandidateID != candidateID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have permission to access or modify the requested resource",
			}}}, http.StatusForbidden)
			return
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeCandidateNotFound,
					Message: "Candidate not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, Envelope{Data: candidate}, http.StatusOK)
	}
}

type RequestBodyPatchCandidate struct {
	About *string `json:"about"`
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerPatchCandidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		urlCandidateIDStr := r.PathValue("id")
		urlCandidateID := ULID(urlCandidateIDStr)
		if urlCandidateID != candidateID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have permission to access or modify the requested resource",
			}}}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBodyPatchCandidate](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeCandidateNotFound,
					Message: "Candidate not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		changed := false
		if body.About != nil {
			about, err := NormalizeAndValidateCandidateAbout(*body.About)
			if errors.Is(err, ErrTextTooLong) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidCandidateAboutFormat,
					Message: "About must be up to 1024 characters",
					Source:  ErrorSource{Body: "data/about"},
				}}}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate about", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if candidate.About != about {
				candidate.About = about
				changed = true
			}
		}

		if !changed {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err = a.Store.UpdateCandidate(candidate.ID, candidate.About); err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeCandidateNotFound,
					Message: "Candidate not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerDeleteCandidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: candidate",
			}}}, http.StatusForbidden)
			return
		}

		if err := a.Store.DeleteCandidate(candidateID); err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeCandidateNotFound,
					Message: "Candidate not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

const ErrorCodeRecruiterNotFound = "recruiter_not_found"

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetRecruiter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		recruiter, err := a.Store.GetRecruiter(recruiterID)
		if err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeRecruiterNotFound,
					Message: "Recruiter not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get recruiter", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, Envelope{Data: recruiter}, http.StatusOK)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerDeleteRecruiter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		if err := a.Store.DeleteRecruiter(recruiterID); err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeRecruiterNotFound,
					Message: "Recruiter not found",
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete recruiter", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

var RegexUrl = regexp.MustCompile(`https?://|www\.`)

const (
	DefaultPositionTitleMinLength = 4
	DefaultPositionTitleMaxLength = 64
)

var ErrPositionTitleHasURL = errors.New("position title must not contain URLs")

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

type RequestBodyCreatePosition struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
}

const (
	ErrorCodeInvalidPositionTitleFormat       = "invalid_position_title_format"
	ErrorCodeInvalidPositionDescriptionFormat = "invalid_position_description_format"
	ErrorCodeInvalidPositionCompanyFormat     = "invalid_position_company_format"
	ErrorCodePositionExists                   = "position_exists"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreatePosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBodyCreatePosition](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		title, err := NormalizeAndValidatePositionTitle(body.Title)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidPositionTitleFormat,
				Message: "Title must be between 4 and 64 characters",
				Source:  ErrorSource{Body: "data/title"},
			}}}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPositionTitleHasURL):
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidPositionTitleFormat,
				Message: "Title cannot contain a URL",
				Source:  ErrorSource{Body: "data/title"},
			}}}, http.StatusBadRequest)
			return

		case err != nil:
			slog.Error("failed to validate position title", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		description, err := NormalizeAndValidatePositionDescription(body.Description)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidPositionDescriptionFormat,
				Message: "Description must be up to 2048 characters",
				Source:  ErrorSource{Body: "data/description"},
			}}}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate description", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		company, err := NormalizeAndValidatePositionCompanyName(body.Company)
		if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidPositionCompanyFormat,
				Message: "Company name must be between 2 and 512 characters",
				Source:  ErrorSource{Body: "data/company"},
			}}}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate company name", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err = a.Store.CreatePosition(recruiterID, title, description, company, true); err != nil {
			if errors.Is(err, ErrPositionAlreadyExists) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionExists,
					Message: "Position with the same title, description and company already exists",
				}}}, http.StatusConflict)
				return
			}
			slog.Error("failed to create position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetPositions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		page := GetPageFromQuery(r)
		positions, nextCursor, err := a.Store.GetPositions(recruiterID, page)
		if err != nil {
			slog.Error("failed to fetch positions", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var links Links
		if nextCursor != "" {
			links.Next = fmt.Sprintf("%s?cursor=%s", RoutePositions, nextCursor)
		}

		JSON(w, Envelope{
			Data:  positions,
			Links: links,
		}, http.StatusOK)
	}
}

const (
	ErrorCodePositionIDRequired = "position_id_required"
	ErrorCodePositionNotFound   = "position_not_found"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetPosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodePositionIDRequired,
				Message: "Position ID is required",
				Source:  ErrorSource{Parameter: "id"},
			}}}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionNotFound,
					Message: "Position not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have access to this resource",
			}}}, http.StatusForbidden)
			return
		}

		JSON(w, Envelope{Data: position}, http.StatusOK)
	}
}

type RequestBodyPatchPosition struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Company     *string `json:"company"`
	IsActive    *bool   `json:"is_active"`
}

func (a *API) HandlerPatchPosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodePositionIDRequired,
				Message: "Position ID is required",
				Source:  ErrorSource{Parameter: "id"},
			}}}, http.StatusBadRequest)
			return
		}

		body, err := DecodeRequestBody[RequestBodyPatchPosition](r)
		if err != nil {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeInvalidRequestBody,
				Message: "Invalid request body",
			}}}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionNotFound,
					Message: "Position not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have access to this resource",
			}}}, http.StatusForbidden)
			return
		}

		changed := false
		if body.Title != nil {
			title, err := NormalizeAndValidatePositionTitle(*body.Title)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidPositionTitleFormat,
					Message: "Title must be between 4 and 64 characters",
					Source:  ErrorSource{Body: "data/title"},
				}}}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrPositionTitleHasURL):
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidPositionTitleFormat,
					Message: "Title cannot contain a URL",
					Source:  ErrorSource{Body: "data/title"},
				}}}, http.StatusBadRequest)
				return

			case err != nil:
				slog.Error("failed to validate position title", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
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
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidPositionDescriptionFormat,
					Message: "Description must be up to 2048 characters",
					Source:  ErrorSource{Body: "data/description"},
				}}}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate description", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
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
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodeInvalidPositionCompanyFormat,
					Message: "Company name must be between 2 and 512 characters",
					Source:  ErrorSource{Body: "data/company"},
				}}}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error("failed to validate company name", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
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
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err = a.Store.UpdatePosition(
			position.ID,
			position.Title,
			position.Description,
			position.Company,
			position.IsActive,
		); err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionNotFound,
					Message: "Position not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to update position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) HandlerDeletePosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		recruiterID, ok := claims.Roles[RoleRecruiter]
		if !ok {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeMissingRequiredRole,
				Message: "Missing required role: recruiter",
			}}}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodePositionIDRequired,
				Message: "Position ID is required",
				Source:  ErrorSource{Parameter: "id"},
			}}}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionNotFound,
					Message: "Position not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, Envelope{Errors: []Error{{
				Code:    ErrorCodeForbidden,
				Message: "You do not have access to this resource",
			}}}, http.StatusForbidden)
			return
		}

		err = a.Store.DeletePosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Envelope{Errors: []Error{{
					Code:    ErrorCodePositionNotFound,
					Message: "Position not found",
					Source:  ErrorSource{Parameter: "id"},
				}}}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
