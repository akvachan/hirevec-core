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
	"html"
	"log/slog"
	"net"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	DefaultReadTimeout   = 2000 * time.Millisecond
	DefaultWriteTimeout  = 2000 * time.Millisecond
	DefaultGracePeriod   = 5000 * time.Millisecond
	DefaultPageSizeLimit = 50
	PageSizeMaxLimit     = 100
)

var (
	ErrAboutForbiddenChars          = errors.New("about contains forbidden characters")
	ErrAboutTooLong                 = errors.New("about too long")
	ErrAboutTooShort                = errors.New("about too short")
	ErrEmailNotVerified             = errors.New("email not verified")
	ErrExtraDataDecoded             = errors.New("extra data decoded")
	ErrFailedBindAddress            = errors.New("failed to bind address")
	ErrFailedDecode                 = errors.New("failed to decode")
	ErrFailedGenerateUsernameSuffix = errors.New("failed to generate random username suffix")
	ErrFailedShutdownServer         = errors.New("failed to shutdown server")
	ErrNameForbiddenChars           = errors.New("name contains forbidden characters")
	ErrNameTooLong                  = errors.New("name too long")
	ErrNameTooShort                 = errors.New("name too short")
	ErrInvalidHideReactedParamValue = errors.New("invalid value for hide_reacted query parameter")
	ErrFailedCloseRequestBody       = errors.New("failed to close request body")
)

type ServerConfig struct {
	Protocol     string
	Host         string
	Port         uint16
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	GracePeriod  time.Duration
}

func RunServer(ctx context.Context, c ServerConfig, s StoreInterface, v VaultInterface) error {
	server, err := NewServer(ctx, c, s, v)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%v", c.Host, c.Port))
	if err != nil {
		return ErrFailedBindAddress
	}

	return WaitAndShutdown(ctx, server, StartServer(server, listener), c.GracePeriod)
}

func NewServer(ctx context.Context, c ServerConfig, s StoreInterface, v VaultInterface) (*http.Server, error) {
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%v", c.Host, c.Port),
		ReadTimeout:  c.ReadTimeout,
		WriteTimeout: c.WriteTimeout,
		Handler:      GetRootMux(s, v),
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}, nil
}

func StartServer(server *http.Server, ln net.Listener) chan error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info(
			"HTTP server starting",
			"addr", server.Addr,
		)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info(
		"HTTP server ready",
		"addr", server.Addr,
	)
	return errCh
}

func WaitAndShutdown(ctx context.Context, server *http.Server, errCh chan error, gracePeriod time.Duration) error {
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
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed, forcing close", "err", err)
		server.Close()
		return ErrFailedShutdownServer
	}

	slog.Info("HTTP server shutdown complete")
	return nil
}

type ContextKey string

const (
	ContextKeyUserID ContextKey = "user_id"
	ContextKeyClaims ContextKey = "claims"
)

type ResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

// Chain wraps handler into a sequence of middlewares, each middleware is applied in the same order it is provided.
func Chain(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

func PanicHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"panic recovered",
					"err", err,
				)
				Error(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	}
}

func GetUserID(r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(ContextKeyUserID).(string)
	return userID, ok
}

func GetClaims(r *http.Request) (*AccessTokenClaims, bool) {
	claims, ok := r.Context().Value(ContextKeyClaims).(*AccessTokenClaims)
	return claims, ok
}

func GetPagination(r *http.Request) Page {
	p := Page{
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  DefaultPageSizeLimit,
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			p.Limit = min(parsed, PageSizeMaxLimit)
		}
	}
	return p
}

func GetRecommendationsQueryParams(r *http.Request) (RecommendationsQueryParams, error) {
	hideReacted := r.URL.Query().Get("hide_reacted")
	var params RecommendationsQueryParams
	switch hideReacted {
	case "true":
		params.HideReacted = true
	case "false":
		params.HideReacted = false
	case "":
		params.HideReacted = false
	default:
		return params, ErrInvalidHideReactedParamValue
	}
	return params, nil
}

func Authentication(v VaultInterface, allowedScopes []ScopeValueType) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var bearer string

			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				var found bool
				bearer, found = strings.CutPrefix(authHeader, "Bearer ")
				if !found || bearer == "" {
					Unauthorized(w, AuthInvalidClient, "Bearer token is required")
					return
				}
			}

			claims, err := v.ParseAccessToken(bearer)
			if err != nil || claims == nil {
				AuthError(w, AuthInvalidGrant, "invalid access token")
				return
			}

			allowed := make(map[ScopeValueType]bool, len(allowedScopes))
			for _, s := range allowedScopes {
				allowed[s] = true
			}

			for _, s := range claims.Scope {
				if _, ok := allowed[s]; !ok {
					AuthError(w, AuthInvalidGrant, "unauthorized")
					return
				}
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

func Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	}
}

func MaxBytesLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1_000_000)
		next.ServeHTTP(w, r)
	}
}

type (
	Method string

	RouteConfig struct {
		Mux            *http.ServeMux
		Method         Method
		Route          string
		Handler        http.HandlerFunc
		RequiredScopes []ScopeValueType // required for protected routes
	}
)

const (
	MethodGet  Method = http.MethodGet
	MethodPost Method = http.MethodPost
)

const (
	RouteOpenAPI              = "/openapi.yaml"
	RouteHealth               = "/health"
	RoutePublicKeys           = "/v1/auth/keys"
	RouteToken                = "/v1/auth/token"
	RouteLogin                = "/v1/auth/login/{provider}"
	RouteCallback             = "/v1/auth/callback/{provider}"
	RouteGetMyRecommendations = "/v1/me/recommendations"
	RouteGetMyReactions       = "/v1/me/reactions"
	RouteGetMyMatches         = "/v1/me/matches"
	RouteCreateMyReaction     = "/v1/me/recommendations/{id}/reaction"
)

func Route(method Method, route string) string {
	return fmt.Sprintf("%s %s", method, route)
}

func BaseMiddleware(handler http.HandlerFunc) http.Handler {
	return Chain(
		handler,
		Logger,
		PanicHandler,
		MaxBytesLimiter,
	)
}

func PublicRoute(s StoreInterface, v VaultInterface, cfg RouteConfig) {
	handler := BaseMiddleware(cfg.Handler)

	cfg.Mux.Handle(
		Route(cfg.Method, cfg.Route),
		handler,
	)
}

func ProtectedRoute(s StoreInterface, v VaultInterface, cfg RouteConfig) {
	handler := Chain(
		cfg.Handler,
		Logger,
		PanicHandler,
		MaxBytesLimiter,
		Authentication(v, cfg.RequiredScopes),
	)

	cfg.Mux.Handle(
		Route(cfg.Method, cfg.Route),
		handler,
	)
}

func GetRootMux(s StoreInterface, v VaultInterface) http.Handler {
	mux := http.NewServeMux()

	// Public routes
	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteOpenAPI,
		Handler: OpenAPI,
	})
	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteHealth,
		Handler: Health,
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RoutePublicKeys,
		Handler: PublicKeys(v),
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodPost,
		Route:   RouteToken,
		Handler: CreateAccessToken(s, v),
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteLogin,
		Handler: Login(v),
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodPost,
		Route:   RouteLogin,
		Handler: Login(v),
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteCallback,
		Handler: RedirectProvider(s, v),
	})

	PublicRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodPost,
		Route:   RouteCallback,
		Handler: RedirectProvider(s, v),
	})

	ProtectedRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteGetMyRecommendations,
		Handler: GetMyRecommendations(s),
		RequiredScopes: []ScopeValueType{
			ScopeValueTypeCandidate, ScopeValueTypeRecruiter,
		},
	})

	ProtectedRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteGetMyReactions,
		Handler: GetMyReactions(s),
		RequiredScopes: []ScopeValueType{
			ScopeValueTypeCandidate, ScopeValueTypeRecruiter,
		},
	})

	ProtectedRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteGetMyMatches,
		Handler: GetMyMatches(s),
		RequiredScopes: []ScopeValueType{
			ScopeValueTypeCandidate, ScopeValueTypeRecruiter,
		},
	})

	ProtectedRoute(s, v, RouteConfig{
		Mux:     mux,
		Method:  MethodPost,
		Route:   RouteCreateMyReaction,
		Handler: CreateMyReaction(s),
		RequiredScopes: []ScopeValueType{
			ScopeValueTypeCandidate, ScopeValueTypeRecruiter,
		},
	})

	return mux
}

type (
	// FailData defines [JSend](https://github.com/omniti-labs/jsend) request failure data.
	FailData map[string]string

	// ResponseStatus defines JSend status codes.
	ResponseStatus string

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

	Links    map[RelType]Link
	Embedded map[string]any
	Props    map[string]any

	// Resource is a flat HAL Resource Object. _links, _embedded, and all
	Resource struct {
		Links    Links    `json:"_links,omitempty"`
		Embedded Embedded `json:"_embedded,omitempty"`
		Props    Props    `json:"-"`
	}

	ErrorResponse struct {
		Status  ResponseStatus `json:"status"`
		Message string         `json:"message"`
		Code    ErrorCode      `json:"code,omitempty"`
	}

	FailResponse struct {
		Status ResponseStatus `json:"status"`
		Data   FailData       `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}

	RecommendationsQueryParams struct {
		HideReacted bool `json:"hide_reacted"`
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
)

func (res Resource) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(res.Props)+2)
	for k, v := range res.Props {
		m[k] = v
	}
	if len(res.Links) > 0 {
		m["_links"] = res.Links
	}
	if len(res.Embedded) > 0 {
		m["_embedded"] = res.Embedded
	}
	return json.Marshal(m)
}

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

func Success(w http.ResponseWriter, status int, res Resource) {
	SetDefaultHeaders(w)
	res.Props["status"] = "success"
	WriteJSON(w, status, res)
}

func Error(w http.ResponseWriter, status int, message string) {
	type ErrorResponse struct {
		Status  ResponseStatus `json:"status"`
		Message string         `json:"message"`
		Code    ErrorCode      `json:"code,omitempty"`
	}
	SetDefaultHeaders(w)
	WriteJSON(w, status, ErrorResponse{Status: ResponseStatusError, Message: message})
}

func Fail(w http.ResponseWriter, status int, data FailData) {
	type FailResponse struct {
		Status ResponseStatus `json:"status"`
		Data   FailData       `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}

	SetDefaultHeaders(w)
	WriteJSON(w, status, FailResponse{Status: ResponseStatusFail, Data: data})
}

func DecodeRequestBody[T any](r *http.Request) (*T, error) {
	var data T

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&data); err != nil {
		slog.Debug(err.Error())
		return nil, ErrFailedDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}

	if err := r.Body.Close(); err != nil {
		slog.Debug(err.Error())
		return nil, ErrFailedCloseRequestBody
	}

	return &data, nil
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

func OpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, path.Join("docs", "openapi.yaml"))
}

func Health(w http.ResponseWriter, r *http.Request) {
	Success(w, http.StatusOK, Resource{Props: Props{}})
}

// Returns position recommendations for the authenticated candidate.
func GetMyRecommendations(s StoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r)
		if !ok {
			Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		candidate, err := s.GetCandidateByUserID(userID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				Error(w, http.StatusNotFound, "candidate profile not found")
				return
			}
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch candidate profile")
			return
		}

		page := GetPagination(r)
		params, err := GetRecommendationsQueryParams(r)
		if errors.Is(err, ErrInvalidHideReactedParamValue) {
			Fail(w, http.StatusBadRequest, FailData{"hide_reacted": "must be one of: false, true"})
			return
		}

		posRcm, nextCursor, err := s.GetPositionRecommendations(candidate.ID, page, params)
		if err != nil {
			slog.Error("failed to fetch recommendations", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch recommendations")
			return
		}

		page.Count = len(posRcm)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf:          Link{Href: "/v1/me/recommendations"},
			RelType("reactions"): Link{Href: "/v1/me/reactions"},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{Href: fmt.Sprintf("/v1/me/recommendations?cursor=%s&limit=%d", nextCursor, page.Limit)}
		}

		positions := make([]Resource, len(posRcm))
		for i, rec := range posRcm {
			positions[i] = Resource{
				Links: Links{
					RelTypeSelf:         Link{Href: "/v1/me/recommendations/" + rec.RecommendationID},
					RelType("reaction"): Link{Href: "/v1/me/recommendations/" + rec.RecommendationID + "/reaction"},
				},
				Props: Props{
					"recommendation_id": rec.RecommendationID,
					"position_id":       rec.PositionID,
					"title":             rec.Title,
					"company":           rec.Company,
					"description":       rec.Description,
				},
			}
		}

		embedded := Embedded{}
		if len(posRcm) > 0 {
			embedded = Embedded{"positions": positions}
		}

		Success(w, http.StatusOK, Resource{
			Links:    links,
			Embedded: embedded,
			Props:    Props{"page": page},
		})
	}
}

// Records a candidate's reaction to a position recommendation.
func CreateMyReaction(s StoreInterface) http.HandlerFunc {
	type RequestBody struct {
		ReactionType ReactionType `json:"reaction_type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r)
		if !ok {
			Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		candidate, err := s.GetCandidateByUserID(userID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				Error(w, http.StatusNotFound, "candidate profile not found")
				return
			}
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch candidate profile")
			return
		}

		recommendationID := r.PathValue("id")
		if recommendationID == "" {
			Fail(w, http.StatusBadRequest, FailData{"id": "recommendation id is required"})
			return
		}

		rec, err := s.GetRecommendation(recommendationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				Fail(w, http.StatusNotFound, FailData{"id": "recommendation not found"})
				return
			}
			slog.Error("failed to fetch recommendation", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch recommendation")
			return
		}
		if rec.CandidateID != candidate.ID {
			Fail(w, http.StatusForbidden, FailData{"id": "forbidden for this id"})
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			Fail(w, http.StatusBadRequest, FailData{"body": "invalid request body"})
			return
		}
		if !body.ReactionType.IsValid() {
			Fail(w, http.StatusBadRequest, FailData{"reaction_type": "must be one of: positive, negative, neutral"})
			return
		}

		if err := s.CreateReaction(Reaction{
			RecommendationID: recommendationID,
			ReactorType:      ReactorTypeCandidate,
			ReactorID:        candidate.ID,
			ReactionType:     body.ReactionType,
		}); err != nil {
			if errors.Is(err, ErrReactionAlreadyExists) {
				Fail(w, http.StatusConflict, FailData{"request": "reaction already exists; reactions are immutable"})
				return
			}
			slog.Error("failed to record reaction", "err", err)
			Error(w, http.StatusInternalServerError, "failed to record reaction")
			return
		}

		Success(w, http.StatusCreated, Resource{
			Links: Links{
				RelTypeSelf:          Link{Href: "/v1/me/recommendations/" + recommendationID + "/reaction"},
				RelTypeUp:            Link{Href: "/v1/me/recommendations"},
				RelType("reactions"): Link{Href: "/v1/me/reactions"},
				RelType("matches"):   Link{Href: "/v1/me/matches"},
			},
			Props: Props{
				"status": "success",
			},
		})
	}
}

// Returns all reactions made by the authenticated candidate.
func GetMyReactions(s StoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r)
		if !ok {
			Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		candidate, err := s.GetCandidateByUserID(userID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				Error(w, http.StatusNotFound, "candidate profile not found")
				return
			}
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch candidate profile")
			return
		}

		page := GetPagination(r)

		reactions, nextCursor, err := s.GetReactionsByCandidateID(candidate.ID, page)
		if err != nil {
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch reactions")
			return
		}

		page.Count = len(reactions)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: "/v1/me/reactions"},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{Href: "/v1/me/reactions?cursor=" + nextCursor}
		}

		// Each embedded reaction links back to the recommendation it was made on.
		embedded := make([]Resource, len(reactions))
		for i, rx := range reactions {
			embedded[i] = Resource{
				Links: Links{
					RelTypeSelf: Link{Href: "/v1/me/recommendations/" + rx.RecommendationID + "/reaction"},
				},
				Props: Props{
					"recommendation_id": rx.RecommendationID,
					"reactor_type":      rx.ReactorType,
					"reactor_id":        rx.ReactorID,
					"reaction_type":     rx.ReactionType,
					"reacted_at":        rx.ReactedAt,
				},
			}
		}

		Success(w, http.StatusOK, Resource{
			Links:    links,
			Embedded: Embedded{"reactions": embedded},
			Props:    Props{"page": page},
		})
	}
}

// Returns all mutual matches for the authenticated candidate.
func GetMyMatches(s StoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r)
		if !ok {
			Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		candidate, err := s.GetCandidateByUserID(userID)
		if err != nil {
			if errors.Is(err, ErrCandidateNotFound) {
				Error(w, http.StatusNotFound, "candidate profile not found")
				return
			}
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch candidate profile")
			return
		}

		page := GetPagination(r)

		matches, nextCursor, err := s.GetMatchesByCandidateID(candidate.ID, page)
		if err != nil {
			slog.Error("failed to fetch matches", "err", err)
			Error(w, http.StatusInternalServerError, "failed to fetch matches")
			return
		}

		page.Count = len(matches)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: "/v1/me/matches"},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{Href: fmt.Sprintf("/v1/me/matches?cursor=%s&limit=%d", nextCursor, page.Limit)}
		}

		embedded := make([]Resource, len(matches))
		for i, m := range matches {
			embedded[i] = Resource{
				Links: Links{
					RelTypeSelf: Link{Href: "/v1/positions/" + m.PositionID},
				},
				Props: Props{
					"position_id": m.PositionID,
					"title":       m.Title,
					"description": m.Description,
					"company":     m.Company,
					"matched_at":  m.MatchedAt,
				},
			}
		}

		Success(w, http.StatusOK, Resource{
			Links:    links,
			Embedded: Embedded{"matches": embedded},
			Props:    Props{"page": page},
		})
	}
}

func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)

	reTags := regexp.MustCompile(`<[^>]*>`)
	name = reTags.ReplaceAllString(name, "")

	reValid := regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	if !reValid.MatchString(name) {
		return "", ErrNameForbiddenChars
	}

	if len(name) < 1 {
		return "", ErrNameTooShort
	}
	if len(name) > 128 {
		return "", ErrNameTooLong
	}

	return html.EscapeString(name), nil
}

func ValidateAbout(about string) (string, error) {
	about = strings.TrimSpace(about)

	reTags := regexp.MustCompile(`<[^>]*>`)
	about = reTags.ReplaceAllString(about, "")

	reValid := regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	if !reValid.MatchString(about) {
		return "", ErrAboutForbiddenChars
	}

	if len(about) < 1 {
		return "", ErrAboutTooShort
	}
	if len(about) > 500 {
		return "", ErrAboutTooLong
	}

	return html.EscapeString(about), nil
}

// AuthErrorCode defienes OAuth2 error codes, see [RFC6749](https://www.rfc-editor.org/rfc/rfc6749.txt).
type AuthErrorCode string

const (
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
)

// AuthErrorResponse defines OAuth2 error response.
type AuthErrorResponse struct {
	Error            AuthErrorCode `json:"error"`
	ErrorDescription string        `json:"error_description,omitempty"`
	ErrorURI         string        `json:"error_uri,omitempty"`
}

func SetAuthHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func SetUnauthorizedHeaders(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
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

func PublicKeys(v VaultInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicKey := v.GetPublicKey()

		keys := []PasetoKey{
			{
				Version: 4,
				Kid:     1,
				Key:     publicKey,
			},
		}

		WriteJSON(w, http.StatusOK, PublicPasetoKeys{Keys: keys})
	}
}

func CreateAccessToken(s StoreInterface, v VaultInterface) http.HandlerFunc {
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

func Login(v VaultInterface) http.HandlerFunc {
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

func RedirectProvider(s StoreInterface, v VaultInterface) http.HandlerFunc {
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

func GoogleCallback(s StoreInterface, v VaultInterface, w http.ResponseWriter, r *http.Request) {
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

func AppleCallback(s StoreInterface, v VaultInterface, w http.ResponseWriter, r *http.Request) {
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

func FinishAuthFlow(s StoreInterface, v VaultInterface, w http.ResponseWriter, user User) {
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

func CreateOnboardingToken(v VaultInterface, w http.ResponseWriter, userID string, provider string) {
	accessToken, err := v.CreateAccessToken(userID, provider, ScopeType{ScopeValueTypeOnboarding})
	if err != nil {
		slog.Error("failed to create access token", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	AuthAccessToken(w, *accessToken)
}

func CreateTokenPair(s StoreInterface, v VaultInterface, w http.ResponseWriter, userID string, provider string, roles []string) {
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
