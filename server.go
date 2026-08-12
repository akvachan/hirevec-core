// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"bytes"
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
	ServerBaseURL               *url.URL
	RequestReadTimeout          time.Duration
	RequestWriteTimeout         time.Duration
	GracePeriod                 time.Duration
	UseGoogleSSO                bool
	UseAppleSSO                 bool
	TEIBaseURL                  *url.URL
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
	BaseURL *url.URL
	Store   Store
	Vault   Vault
	Server  http.Server
	Mux     *http.ServeMux
}

const (
	DefaultRequestReadTimeout  = 1000 * time.Millisecond
	DefaultRequestWriteTimeout = 1000 * time.Millisecond
	DefaultGracePeriod         = 5000 * time.Millisecond
)

func NewAPI(ctx context.Context, c APIConfig, s Store, v Vault) *API {
	slog.Debug("initializing server")

	api := API{
		BaseURL: c.ServerBaseURL,
		Store:   s,
		Vault:   v,
		Mux:     http.NewServeMux(),
	}

	api.Server = http.Server{
		Addr:         c.ServerBaseURL.Host,
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
		TEIBaseURL:      c.TEIBaseURL.Host,
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

type ProblemType string

type ProblemSource struct {
	Pointer   string `json:"pointer,omitempty"`
	Parameter string `json:"parameter,omitempty"`
	Header    string `json:"header,omitempty"`
	Cookie    string `json:"cookie,omitempty"`
}

type Problem struct {
	Type   ProblemType   `json:"type,omitempty"`
	Detail string        `json:"detail,omitempty"`
	Source ProblemSource `json:"source,omitempty"`
}

func JSON(w http.ResponseWriter, responseBody any, status int) {
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

// TODO: Validate cursor format as well
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
	ProblemTypeBearerTokenRequired ProblemType = "urn:hirevec:bearer-token-required"
	ProblemTypeInvalidAccessToken  ProblemType = "urn:hirevec:invalid-access-token"
	ProblemTypeUnauthorized        ProblemType = "urn:hirevec:unauthorized"
)

func (a *API) MiddlewareAuth(roles map[Role]bool) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			bearer, found := strings.CutPrefix(authHeader, "Bearer ")
			if !found || bearer == "" {
				JSON(w, Problem{
					Type:   ProblemTypeBearerTokenRequired,
					Detail: "Bearer token is required",
					Source: ProblemSource{Header: "Authorization"},
				}, http.StatusUnauthorized)
				return
			}

			claims, err := a.Vault.ParseAccessToken(bearer)
			if err != nil {
				JSON(w, Problem{
					Type:   ProblemTypeInvalidAccessToken,
					Detail: "Invalid access token",
					Source: ProblemSource{Header: "Authorization"},
				}, http.StatusBadRequest)
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
					JSON(w, Problem{
						Type:   ProblemTypeUnauthorized,
						Source: ProblemSource{Header: "Authorization"},
					}, http.StatusUnauthorized)
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ContextKeyClaims, claims)))
		}
	}
}

const ProblemTypeUnsupportedMediaType ProblemType = "urn:hirevec:unsupported-media-type"

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
	RouteCandidate       Route = "/candidates/{id}"
	RouteCandidates      Route = "/candidates"
	RouteMatches         Route = "/matches"
	RoutePosition        Route = "/positions/{id}"
	RoutePositions       Route = "/positions"
	RouteReactions       Route = "/reactions"
	RouteRecommendation  Route = "/recommendations/{id}"
	RouteRecommendations Route = "/recommendations"
	RouteRecruiter       Route = "/recruiters/{id}"
	RouteRecruiters      Route = "/recruiters"
	RouteUser            Route = "/users/{id}"
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
		Roles:   []Role{RoleRecruiter, RoleCandidate},
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
		Roles:   []Role{RoleRecruiter, RoleCandidate},
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
		Roles:   []Role{RoleRecruiter, RoleCandidate},
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
		Route:   RouteRecommendation,
		Handler: a.HandlerGetRecommendation(),
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

const (
	ProblemTypeInvalidRequestBody   ProblemType = "urn:hirevec:invalid-request-body"
	ProblemTypeInvalidGrantType     ProblemType = "urn:hirevec:invalid-grant-type"
	ProblemTypeRefreshTokenRequired ProblemType = "urn:hirevec:refresh-token-required"
	ProblemTypeInvalidRefreshToken  ProblemType = "urn:hirevec:invalid-refresh-token"
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
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		if body.GrantType != "refresh_token" {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidGrantType,
				Detail: "grant_type must be refresh_token",
				Source: ProblemSource{Pointer: "/grant_type"},
			}, http.StatusBadRequest)
			return
		}

		if body.RefreshToken == "" {
			JSON(w, Problem{
				Type:   ProblemTypeRefreshTokenRequired,
				Detail: "refresh_token is required",
				Source: ProblemSource{Pointer: "/refresh_token"},
			}, http.StatusBadRequest)
			return
		}

		claims, err := a.Vault.ParseRefreshToken(body.RefreshToken)
		if err != nil {
			slog.Error(
				"failed to parse refresh token",
				"ip", r.RemoteAddr,
				"err", err,
			)
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRefreshToken,
				Detail: "Invalid refresh token",
				Source: ProblemSource{Pointer: "/refresh_token"},
			}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidRefreshToken,
					Detail: "Invalid refresh token",
					Source: ProblemSource{Pointer: "/refresh_token"},
				}, http.StatusBadRequest)
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
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRefreshToken,
				Detail: "Invalid refresh token",
				Source: ProblemSource{Pointer: "/meta/refresh_token"},
			}, http.StatusBadRequest)
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

		accessToken, err := a.Vault.CreateAccessToken(claims.UserID, claims.Provider, roles)
		if err != nil {
			slog.Error("failed to create access token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		JSON(w, accessToken, http.StatusOK)
	}
}

type RequestBodyLoginViaEmail struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

const (
	ProblemTypePasswordRequired   ProblemType = "urn:hirevec:password-required"
	ProblemTypeInvalidCredentials ProblemType = "urn:hirevec:invalid-credentials"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerLoginViaEmail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBodyLoginViaEmail](r)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		if body.Email == "" {
			JSON(w, Problem{
				Type:   ProblemTypeEmailRequired,
				Detail: "Email is required",
				Source: ProblemSource{Pointer: "/email"},
			}, http.StatusBadRequest)
			return
		}

		if body.Password == "" {
			JSON(w, Problem{
				Type:   ProblemTypePasswordRequired,
				Detail: "Password is required",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return
		}

		user, roles, err := a.Store.GetUserAndRolesByEmail(body.Email, ProviderEmail)
		switch {
		case errors.Is(err, ErrUserNotFound):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidCredentials,
				Detail: "Invalid credentials",
			}, http.StatusUnauthorized)
			return

		case errors.Is(err, ErrUserNoRole):
			accessToken, err := a.Vault.CreateAccessToken(user.ID, user.Provider, map[Role]ULID{})
			if err != nil {
				slog.Error("failed to create access token", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			JSON(w, accessToken, http.StatusOK)

		case err != nil:
			slog.Error("failed to get user by email", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !IsValidPassword(user.PasswordHash, body.Password) {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidCredentials,
				Detail: "Invalid credentials",
			}, http.StatusUnauthorized)
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

		JSON(w, tokenPair, http.StatusOK)
	}
}

const ProblemTypeInvalidProvider ProblemType = "urn:hirevec:invalid-provider"

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerLoginViaProvider() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := Provider(r.PathValue("provider"))
		if provider != ProviderGoogle && provider != ProviderApple {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidProvider,
				Detail: "Provider must be google or apple",
				Source: ProblemSource{Parameter: "provider"},
			}, http.StatusBadRequest)
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
	ProblemTypeMissingState          ProblemType = "urn:hirevec:missing-state"
	ProblemTypeInvalidState          ProblemType = "urn:hirevec:invalid-state"
	ProblemTypeInvalidCSRF           ProblemType = "urn:hirevec:invalid-csrf"
	ProblemTypeMissingVerifier       ProblemType = "urn:hirevec:missing-verifier"
	ProblemTypeAuthorizationProvider ProblemType = "urn:hirevec:authorization-provider-error"
	ProblemTypeMissingCode           ProblemType = "urn:hirevec:missing-code"
	ProblemTypeIDTokenRequired       ProblemType = "urn:hirevec:id-token-required"
	ProblemTypeInvalidIDToken        ProblemType = "urn:hirevec:invalid-id-token"
	ProblemTypeFailedParseClaims     ProblemType = "urn:hirevec:failed-parse-claims"
	ProblemTypeUnverifiedEmail       ProblemType = "urn:hirevec:unverified-email"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerSSOCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), a.Vault.StateTokenExpiration)
		defer cancel()

		stateString := r.URL.Query().Get("state")
		if stateString == "" {
			JSON(w, Problem{
				Type:   ProblemTypeMissingState,
				Detail: "Missing state",
				Source: ProblemSource{Parameter: "state"},
			}, http.StatusBadRequest)
			return
		}

		state, err := a.Vault.ParseStateToken(stateString)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidState,
				Detail: "Invalid state",
				Source: ProblemSource{Parameter: "state"},
			}, http.StatusBadRequest)
			return
		}

		csrfCookie, err := r.Cookie("oauth_csrf")
		if err != nil || csrfCookie.Value != state.CSRF {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidCSRF,
				Detail: "Invalid CSRF",
				Source: ProblemSource{Cookie: "oauth_csrf"},
			}, http.StatusBadRequest)
			return
		}

		verifierCookie, err := r.Cookie("oauth_verifier")
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeMissingVerifier,
				Detail: "Missing Verifier",
				Source: ProblemSource{Cookie: "oauth_verifier"},
			}, http.StatusBadRequest)
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			JSON(w, Problem{
				Type:   ProblemTypeAuthorizationProvider,
				Detail: "Authorization provider returned error",
				Source: ProblemSource{Parameter: "error"},
			}, http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			JSON(w, Problem{
				Type:   ProblemTypeMissingCode,
				Detail: "Missing authorization code",
				Source: ProblemSource{Parameter: "code"},
			}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeIDTokenRequired,
					Detail: "id_token is required",
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeIDTokenRequired,
					Detail: "id_token is required",
				}, http.StatusBadRequest)
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
			JSON(w, Problem{
				Type:   ProblemTypeIDTokenRequired,
				Detail: "Invalid provider; must be one of: google, apple",
				Source: ProblemSource{Parameter: "provider"},
			}, http.StatusBadRequest)
			return
		}

		switch {
		case errors.Is(idTokenErr, ErrInvalidIDToken):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidIDToken,
				Detail: "Invalid id_token",
			}, http.StatusBadRequest)
			return

		case errors.Is(idTokenErr, ErrFailedParseClaims):
			JSON(w, Problem{
				Type:   ProblemTypeFailedParseClaims,
				Detail: "Failed to parse claims",
			}, http.StatusBadRequest)
			return

		case errors.Is(idTokenErr, ErrEmailNotVerified):
			JSON(w, Problem{
				Type:   ProblemTypeUnverifiedEmail,
				Detail: "Unverified email",
			}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserFullNameFormat,
					Detail: ErrorMessageUserFullNameWrongSize,
				}, http.StatusBadRequest)
				return
			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserFullNameFormat,
					Detail: ErrorMessageUserFullNameForbiddenChars,
				}, http.StatusBadRequest)
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
			JSON(w, accessToken, http.StatusOK)
			return

		case errors.Is(err, ErrUserNoRole):
			accessToken, err := a.Vault.CreateAccessToken(userID, idToken.Provider, map[Role]ULID{})
			if err != nil {
				slog.Error("failed to create access token", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Cache-Control", "no-store")
			JSON(w, accessToken, http.StatusOK)
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
		JSON(w, tokenPair, http.StatusOK)
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
	JSON(w, ResponseDataHealth{Status: "ok"}, http.StatusOK)
}

const ProblemTypeInvalidIncludeReacted ProblemType = "urn:hirevec:invalid-include-reacted"

var (
	ErrNoNextPageToLink = errors.New("failed to process page due to missing next page")
	ErrEmptyCursor      = errors.New("failed to process page due to cursor being empty")
	ErrZeroLimit        = errors.New("failed to process page due to limit being zero")
)

func (a *API) AddNextLink(w http.ResponseWriter, route Route, nextPage Page) error {
	if !nextPage.HasNext {
		return ErrNoNextPageToLink
	}
	if nextPage.Cursor == "" {
		return ErrEmptyCursor
	}
	if nextPage.Limit == 0 {
		return ErrZeroLimit
	}

	u := a.BaseURL.JoinPath(string(route))

	query := url.Values{}
	query.Set("cursor", nextPage.Cursor)
	query.Set("limit", strconv.Itoa(nextPage.Limit))
	u.RawQuery = query.Encode()

	w.Header().Add("Link", fmt.Sprintf(`<%s>; rel="next"`, u.String()))

	return nil
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

		roleStr := r.URL.Query().Get("role")
		role, err := StringToRole(roleStr)
		if err != nil && roleStr != "" {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidReactionType,
				Detail: "Invalid role; must be one of: recruiter, candidate",
				Source: ProblemSource{Parameter: "role"},
			}, http.StatusBadRequest)
			return
		}

		includeReactedStr := r.URL.Query().Get("include_reacted")
		includeReacted, err := strconv.ParseBool(includeReactedStr)
		if err != nil && includeReactedStr != "" {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidIncludeReacted,
				Detail: "Invalid include_reacted; must be one of: 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False",
				Source: ProblemSource{Parameter: "include_reacted"},
			}, http.StatusBadRequest)
			return
		}

		if role == RoleRecruiter && !isRecruiter {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}
		if role == RoleCandidate && !isCandidate {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: candidate",
			}, http.StatusForbidden)
			return
		}

		if (role == RoleRecruiter || role == "") && isRecruiter {
			recommendations, nextPage, err := a.Store.GetRecommendationsForRecruiter(recruiterID, GetPageFromQuery(r), includeReacted)
			if err != nil {
				slog.Error("failed to fetch recommendations", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteRecommendations, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, recommendations, http.StatusOK)
			return

		} else if (role == RoleCandidate || role == "") && isCandidate {
			recommendations, nextPage, err := a.Store.GetRecommendationsForCandidate(candidateID, GetPageFromQuery(r), includeReacted)
			if err != nil {
				slog.Error("failed to fetch recommendations", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteRecommendations, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, recommendations, http.StatusOK)
			return

		} else {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing one of required roles: recruiter, candidate",
			}, http.StatusForbidden)
			return
		}
	}
}

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetRecommendation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recommendationIDStr := r.PathValue("id")
		recommendationID := ULID(recommendationIDStr)
		if recommendationID == "" {
			JSON(w, Problem{
				Type:   ProblemTypePositionIDRequired,
				Detail: "Recommendation ID is required",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusBadRequest)
			return
		}

		recommendation, err := a.Store.GetRecommendation(recommendationID)
		if err != nil {
			if errors.Is(err, ErrRecommendationNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeRecommendationNotFound,
					Detail: "Recommendation not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, recommendation, http.StatusOK)
	}
}

type RequestCreateReaction struct {
	RecommendationID ULID         `json:"recommendation_id"`
	ReactionType     ReactionType `json:"reaction_type"`
}

const (
	ProblemTypeRecommendationIDRequired ProblemType = "urn:hirevec:recommendation-id-required"
	ProblemTypeRecommendationNotFound   ProblemType = "urn:hirevec:recommendation-not-found"
	ProblemTypeReactionExists           ProblemType = "urn:hirevec:reaction-exists"
	ProblemTypeInvalidReactionType      ProblemType = "urn:hirevec:invalid-reaction-type"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateReaction() http.HandlerFunc {
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing one of required roles: candidate, recruiter",
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestCreateReaction](r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if body.RecommendationID == "" {
			JSON(w, Problem{
				Type:   ProblemTypeRecommendationIDRequired,
				Detail: "Recommendation ID is required",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusBadRequest)
			return
		}
		if !body.ReactionType.IsValid() {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidReactionType,
				Detail: "Invalid reaction type; must be one of: positive, negative, neutral",
				Source: ProblemSource{Pointer: "/reaction_type"},
			}, http.StatusBadRequest)
			return
		}

		var reactorType ReactorType
		var reactorID ULID
		if isCandidate {
			reactorType = ReactorTypeCandidate
			reactorID = candidateID
		} else if isRecruiter {
			reactorType = ReactorTypeRecruiter
			reactorID = recruiterID
		}

		err = a.Store.CreateReaction(body.RecommendationID, reactorType, reactorID, body.ReactionType)
		switch {
		case errors.Is(err, ErrReactionAlreadyExists):
			JSON(w, Problem{
				Type:   ProblemTypeReactionExists,
				Detail: "Reaction already exists; reactions are immutable",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusConflict)
			return
		case errors.Is(err, ErrRecommendationNotFound):
			JSON(w, Problem{
				Type:   ProblemTypeRecommendationNotFound,
				Detail: "Recommendation not found",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusNotFound)
			return
		case errors.Is(err, ErrUnauthorizedReactor):
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		case err != nil:
			slog.Error("failed to record reaction", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
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

		candidateID, isCandidate := claims.Roles[RoleCandidate]
		recruiterID, isRecruiter := claims.Roles[RoleRecruiter]

		roleStr := r.URL.Query().Get("role")
		role, err := StringToRole(roleStr)
		if err != nil && roleStr != "" {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidReactionType,
				Detail: "Invalid role; must be one of: recruiter, candidate",
				Source: ProblemSource{Parameter: "role"},
			}, http.StatusBadRequest)
			return
		}

		if role == RoleRecruiter && !isRecruiter {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}
		if role == RoleCandidate && !isCandidate {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: candidate",
			}, http.StatusForbidden)
			return
		}

		if (role == RoleRecruiter || role == "") && isRecruiter {
			reactions, nextPage, err := a.Store.GetReactions(recruiterID, ReactorTypeRecruiter, GetPageFromQuery(r))
			if err != nil {
				slog.Error("failed to fetch reactions", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteReactions, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, reactions, http.StatusOK)
			return

		} else if (role == RoleCandidate || role == "") && isCandidate {
			reactions, nextPage, err := a.Store.GetReactions(candidateID, ReactorTypeCandidate, GetPageFromQuery(r))
			if err != nil {
				slog.Error("failed to fetch reactions", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteReactions, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, reactions, http.StatusOK)
			return

		} else {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing one of required roles: recruiter, candidate",
			}, http.StatusForbidden)
			return
		}
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

		candidateID, isCandidate := claims.Roles[RoleCandidate]
		recruiterID, isRecruiter := claims.Roles[RoleRecruiter]

		roleStr := r.URL.Query().Get("role")
		role, err := StringToRole(roleStr)
		if err != nil && roleStr != "" {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidReactionType,
				Detail: "Invalid role; must be one of: recruiter, candidate",
				Source: ProblemSource{Parameter: "role"},
			}, http.StatusBadRequest)
			return
		}

		if role == RoleRecruiter && !isRecruiter {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}
		if role == RoleCandidate && !isCandidate {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: candidate",
			}, http.StatusForbidden)
			return
		}

		if (role == RoleRecruiter || role == "") && isRecruiter {
			matches, nextPage, err := a.Store.GetMatchesForRecruiter(recruiterID, GetPageFromQuery(r))
			if err != nil {
				slog.Error("failed to fetch matches", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteMatches, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, matches, http.StatusOK)
			return

		} else if (role == RoleCandidate || role == "") && isCandidate {
			matches, nextPage, err := a.Store.GetMatchesForCandidate(candidateID, GetPageFromQuery(r))
			if err != nil {
				slog.Error("failed to fetch matches", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			err = a.AddNextLink(w, RouteMatches, nextPage)
			if errors.Is(err, ErrZeroLimit) {
				slog.Error("zero limit in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if errors.Is(err, ErrEmptyCursor) {
				slog.Error("empty cursor in next page", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			JSON(w, matches, http.StatusOK)
			return

		} else {
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing one of required roles: recruiter, candidate",
			}, http.StatusForbidden)
			return
		}
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
	ProblemTypeEmailRequired             ProblemType = "urn:hirevec:email-required"
	ProblemTypeInvalidUserEmailFormat    ProblemType = "urn:hirevec:invalid-user-email-format"
	ProblemTypeInvalidUserPasswordFormat ProblemType = "urn:hirevec:invalid-user-password-format"
	ProblemTypeInvalidUserFullNameFormat ProblemType = "urn:hirevec:invalid-user-full-name-format"
	ProblemTypeUserAlreadyExists         ProblemType = "urn:hirevec:user-exists"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerCreateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := DecodeRequestBody[RequestBodyCreateUser](r)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		if body.Email == "" {
			JSON(w, Problem{
				Type:   ProblemTypeEmailRequired,
				Detail: "Email is required",
				Source: ProblemSource{Pointer: "/email"},
			}, http.StatusBadRequest)
			return
		}

		email, err := mail.ParseAddress(body.Email)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserEmailFormat,
				Detail: "Invalid email",
				Source: ProblemSource{Pointer: "/email"},
			}, http.StatusBadRequest)
			return
		}

		switch err = ValidateUserPassword(body.Password); {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserPasswordFormat,
				Detail: "Password must be between 8 and 128 characters",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoLower):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserPasswordFormat,
				Detail: "Password must contain at least one lowercase letter",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoUpper):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserPasswordFormat,
				Detail: "Password must contain at least one uppercase letter",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoDigit):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserPasswordFormat,
				Detail: "Password must contain at least one digit",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPasswordHasNoSpecial):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserPasswordFormat,
				Detail: "Password must contain at least one special character",
				Source: ProblemSource{Pointer: "/password"},
			}, http.StatusBadRequest)
			return

		case err != nil:
			slog.Error("failed to validate password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		fullName, err := NormalizeAndValidateUserFullName(body.FullName)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserFullNameFormat,
				Detail: ErrorMessageUserFullNameWrongSize,
				Source: ProblemSource{Pointer: "/full_name"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrTextForbiddenChars):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidUserFullNameFormat,
				Detail: ErrorMessageUserFullNameForbiddenChars,
				Source: ProblemSource{Pointer: "/full_name"},
			}, http.StatusBadRequest)
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
			JSON(w, Problem{
				Type:   ProblemTypeUserAlreadyExists,
				Detail: "User with the provided email already exists",
				Source: ProblemSource{Pointer: "data/email"},
			}, http.StatusConflict)
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
		JSON(w, tokenPair, http.StatusCreated)
	}
}

const ProblemTypeRecruiterExists ProblemType = "urn:hirevec:recruiter-exists"

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
				JSON(w, Problem{
					Type:   ProblemTypeRecruiterExists,
					Detail: "Recruiter already exists",
					Source: ProblemSource{Pointer: "data/user_id"},
				}, http.StatusConflict)
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
		JSON(w, accessToken, http.StatusCreated)
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
	ProblemTypeCandidateAlreadyExists      ProblemType = "urn:hirevec:candiate-already-exists"
	ProblemTypeInvalidCandidateAboutFormat ProblemType = "urn:hirevec:invalid-candidate-about-format"
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
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		about, err := NormalizeAndValidateCandidateAbout(body.About)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidCandidateAboutFormat,
				Detail: "About must be up to 1024 characters",
				Source: ProblemSource{Pointer: "data/about"},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate about", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err = a.Store.CreateCandidate(claims.UserID, about); err != nil {
			if errors.Is(err, ErrCandidateAlreadyExists) {
				JSON(w, Problem{
					Type:   ProblemTypeCandidateAlreadyExists,
					Detail: "Candidate already exists",
					Source: ProblemSource{Pointer: "data/user_id"},
				}, http.StatusConflict)
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
		JSON(w, accessToken, http.StatusCreated)
	}
}

const (
	ProblemTypeInternalServerError ProblemType = "urn:hirevec:internal-server-error"
	ProblemTypeUserNotFound        ProblemType = "urn:hirevec:user-not-found"
	ProblemTypeForbidden           ProblemType = "urn:hirevec:forbidden"
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

		userID := ULID(r.PathValue("id"))

		// Infer userID from claims
		if userID == "me" {
			userID = claims.UserID
		}

		user, err := a.Store.GetUser(userID)
		if errors.Is(err, ErrUserNotFound) {
			JSON(w, Problem{
				Type:   ProblemTypeUserNotFound,
				Detail: "User not found",
			}, http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("failed to get user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, user, http.StatusOK)
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
	ProblemTypeResourceTypeMismatch      ProblemType = "urn:hirevec:resource-type-mismatch"
	ProblemTypeResourceIDMismatch        ProblemType = "urn:hirevec:resource-id-mismatch"
	ProblemTypeInvalidUserUserNameFormat ProblemType = "urn:hirevec:invalid-user-user-name-format"
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
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeUserNotFound,
					Detail: "User not found",
				}, http.StatusNotFound)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserFullNameFormat,
					Detail: ErrorMessageUserFullNameWrongSize,
					Source: ProblemSource{Pointer: "data/full_name"},
				}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserFullNameFormat,
					Detail: ErrorMessageUserFullNameForbiddenChars,
					Source: ProblemSource{Pointer: "data/full_name"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserUserNameFormat,
					Detail: ErrorMessageUserNameWrongSize,
					Source: ProblemSource{Pointer: "data/user_name"},
				}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrTextForbiddenChars):
				JSON(w, Problem{
					Type:   ProblemTypeInvalidUserUserNameFormat,
					Detail: ErrorMessageUserNameForbiddenChars,
					Source: ProblemSource{Pointer: "data/user_name"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeUserNotFound,
					Detail: "User not found",
				}, http.StatusNotFound)
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
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		}

		user, err := a.Store.GetUser(claims.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeUserNotFound,
					Detail: "User not found",
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get user", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = a.Store.DeleteUser(user.ID); err != nil {
			if errors.Is(err, ErrUserNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeUserNotFound,
					Detail: "User not found",
				}, http.StatusNotFound)
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
	ProblemTypeMissingRequiredRole ProblemType = "urn:hirevec:missing-required-role"
	ProblemTypeCandidateNotFound   ProblemType = "urn:hirevec:candidate-not-found"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetCandidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidateID := ULID(r.PathValue("id"))

		// Infer candidateID from claims
		if candidateID == "me" {
			claims, ok := GetClaims(r)
			if !ok {
				slog.Error("failed to access claims")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			claimsCandidateID, ok := claims.Roles[RoleCandidate]
			if !ok {
				JSON(w, Problem{
					Type:   ProblemTypeMissingRequiredRole,
					Detail: "Missing required role: candidate",
				}, http.StatusForbidden)
				return
			}
			candidateID = claimsCandidateID
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeCandidateNotFound,
					Detail: "Candidate not found",
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, candidate, http.StatusOK)
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: candidate",
			}, http.StatusForbidden)
			return
		}

		urlCandidateIDStr := r.PathValue("id")
		urlCandidateID := ULID(urlCandidateIDStr)
		if urlCandidateID != candidateID {
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBodyPatchCandidate](r)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		candidate, err := a.Store.GetCandidate(candidateID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeCandidateNotFound,
					Detail: "Candidate not found",
				}, http.StatusNotFound)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidCandidateAboutFormat,
					Detail: "About must be up to 1024 characters",
					Source: ProblemSource{Pointer: "data/about"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeCandidateNotFound,
					Detail: "Candidate not found",
				}, http.StatusNotFound)
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: candidate",
			}, http.StatusForbidden)
			return
		}

		urlCandidateIDStr := r.PathValue("id")
		urlCandidateID := ULID(urlCandidateIDStr)
		if urlCandidateID != candidateID {
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		}

		if err := a.Store.DeleteCandidate(candidateID); err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeCandidateNotFound,
					Detail: "Candidate not found",
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete candidate", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

const ProblemTypeRecruiterNotFound ProblemType = "urn:hirevec:recruiter-not-found"

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetRecruiter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recruiterID := ULID(r.PathValue("id"))

		// Infer candidateID from claims
		if recruiterID == "me" {
			claims, ok := GetClaims(r)
			if !ok {
				slog.Error("failed to access claims")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			claimsRecruiterID, ok := claims.Roles[RoleRecruiter]
			if !ok {
				JSON(w, Problem{
					Type:   ProblemTypeMissingRequiredRole,
					Detail: "Missing required role: recruiter",
				}, http.StatusForbidden)
				return
			}
			recruiterID = claimsRecruiterID
		}

		recruiter, err := a.Store.GetRecruiter(recruiterID)
		if err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeRecruiterNotFound,
					Detail: "Recruiter not found",
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to get recruiter", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, recruiter, http.StatusOK)
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}

		if err := a.Store.DeleteRecruiter(recruiterID); err != nil {
			if errors.Is(err, ErrRecruiterNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypeRecruiterNotFound,
					Detail: "Recruiter not found",
				}, http.StatusNotFound)
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
	ProblemTypeInvalidPositionTitleFormat       ProblemType = "urn:hirevec:invalid-position-title-format"
	ProblemTypeInvalidPositionDescriptionFormat ProblemType = "urn:hirevec:invalid-position-description-format"
	ProblemTypeInvalidPositionCompanyFormat     ProblemType = "urn:hirevec:invalid-position-company-format"
	ProblemTypePositionExists                   ProblemType = "urn:hirevec:position-exists"
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBodyCreatePosition](r)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		title, err := NormalizeAndValidatePositionTitle(body.Title)
		switch {
		case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidPositionTitleFormat,
				Detail: "Title must be between 4 and 64 characters",
				Source: ProblemSource{Pointer: "data/title"},
			}, http.StatusBadRequest)
			return

		case errors.Is(err, ErrPositionTitleHasURL):
			JSON(w, Problem{
				Type:   ProblemTypeInvalidPositionTitleFormat,
				Detail: "Title cannot contain a URL",
				Source: ProblemSource{Pointer: "data/title"},
			}, http.StatusBadRequest)
			return

		case err != nil:
			slog.Error("failed to validate position title", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		description, err := NormalizeAndValidatePositionDescription(body.Description)
		if errors.Is(err, ErrTextTooLong) {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidPositionDescriptionFormat,
				Detail: "Description must be up to 2048 characters",
				Source: ProblemSource{Pointer: "data/description"},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate description", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		company, err := NormalizeAndValidatePositionCompanyName(body.Company)
		if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidPositionCompanyFormat,
				Detail: "Company name must be between 2 and 512 characters",
				Source: ProblemSource{Pointer: "data/company"},
			}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error("failed to validate company name", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err = a.Store.CreatePosition(recruiterID, title, description, company, true); err != nil {
			if errors.Is(err, ErrPositionAlreadyExists) {
				JSON(w, Problem{
					Type:   ProblemTypePositionExists,
					Detail: "Position with the same title, description and company already exists",
				}, http.StatusConflict)
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}

		positions, page, err := a.Store.GetPositions(recruiterID, GetPageFromQuery(r))
		if err != nil {
			slog.Error("failed to fetch positions", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		a.AddNextLink(w, RoutePositions, page)

		JSON(w, positions, http.StatusOK)
	}
}

const (
	ProblemTypePositionIDRequired ProblemType = "urn:hirevec:position-id-required"
	ProblemTypePositionNotFound   ProblemType = "urn:hirevec:position-not-found"
)

// TODO: Write integration tests for this handler
// https://github.com/akvachan/hirevec-core/issues/34
func (a *API) HandlerGetPosition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Problem{
				Type:   ProblemTypePositionIDRequired,
				Detail: "Position ID is required",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypePositionNotFound,
					Detail: "Position not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		JSON(w, position, http.StatusOK)
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
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Problem{
				Type:   ProblemTypePositionIDRequired,
				Detail: "Position ID is required",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusBadRequest)
			return
		}

		body, err := DecodeRequestBody[RequestBodyPatchPosition](r)
		if err != nil {
			JSON(w, Problem{
				Type:   ProblemTypeInvalidRequestBody,
				Detail: "Invalid request body",
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypePositionNotFound,
					Detail: "Position not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		}

		changed := false
		if body.Title != nil {
			title, err := NormalizeAndValidatePositionTitle(*body.Title)
			switch {
			case errors.Is(err, ErrTextTooShort), errors.Is(err, ErrTextTooLong):
				JSON(w, Problem{
					Type:   ProblemTypeInvalidPositionTitleFormat,
					Detail: "Title must be between 4 and 64 characters",
					Source: ProblemSource{Pointer: "data/title"},
				}, http.StatusBadRequest)
				return

			case errors.Is(err, ErrPositionTitleHasURL):
				JSON(w, Problem{
					Type:   ProblemTypeInvalidPositionTitleFormat,
					Detail: "Title cannot contain a URL",
					Source: ProblemSource{Pointer: "data/title"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidPositionDescriptionFormat,
					Detail: "Description must be up to 2048 characters",
					Source: ProblemSource{Pointer: "data/description"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypeInvalidPositionCompanyFormat,
					Detail: "Company name must be between 2 and 512 characters",
					Source: ProblemSource{Pointer: "data/company"},
				}, http.StatusBadRequest)
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
				JSON(w, Problem{
					Type:   ProblemTypePositionNotFound,
					Detail: "Position not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
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
			JSON(w, Problem{
				Type:   ProblemTypeMissingRequiredRole,
				Detail: "Missing required role: recruiter",
			}, http.StatusForbidden)
			return
		}

		positionIDStr := r.PathValue("id")
		positionID := ULID(positionIDStr)
		if positionID == "" {
			JSON(w, Problem{
				Type:   ProblemTypePositionIDRequired,
				Detail: "Position ID is required",
				Source: ProblemSource{Parameter: "id"},
			}, http.StatusBadRequest)
			return
		}

		position, err := a.Store.GetPosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypePositionNotFound,
					Detail: "Position not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to fetch position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if position.RecruiterID != recruiterID {
			JSON(w, Problem{
				Type:   ProblemTypeForbidden,
				Detail: "You do not have access to this resource",
			}, http.StatusForbidden)
			return
		}

		err = a.Store.DeletePosition(positionID)
		if err != nil {
			if errors.Is(err, ErrPositionNotFound) {
				JSON(w, Problem{
					Type:   ProblemTypePositionNotFound,
					Detail: "Position not found",
					Source: ProblemSource{Parameter: "id"},
				}, http.StatusNotFound)
				return
			}
			slog.Error("failed to delete position", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
