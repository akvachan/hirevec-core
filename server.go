// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"bufio"
	"bytes"
	"cmp"
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
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/oauth2"
)

var (
	ErrAboutForbiddenChars           = errors.New("about contains forbidden characters")
	ErrAboutTooLong                  = errors.New("about too long")
	ErrAboutTooShort                 = errors.New("about too short")
	ErrEmailNotVerified              = errors.New("email not verified")
	ErrExtraDataDecoded              = errors.New("extra data decoded")
	ErrFailedShutdownServer          = errors.New("failed to shutdown server")
	ErrFailedEncodeEmbeddingsRequest = errors.New("failed to encode embeddings request")
	ErrNameForbiddenChars            = errors.New("name contains forbidden characters")
	ErrEmbeddingsCountMismatch       = errors.New("mismatched IDs and embeddings count")
	ErrNameTooLong                   = errors.New("name too long")
	ErrNameTooShort                  = errors.New("name too short")
	ErrFailedCreateEmbeddingsRequest = errors.New("embedding endpoint returned non-200 status")
)

type ServerConfig struct {
	ServerBaseURL       string
	RequestReadTimeout  time.Duration
	RequestWriteTimeout time.Duration
	GracePeriod         time.Duration
	UseGoogleSSO        bool
	UseAppleSSO         bool
	TEIBaseURL          string
	TEIAPIKey           string
	EmbeddingsModel     string
	RerankerModel       string
	UseEmbeddings       bool
	UseReranker         bool
}

type EmbeddingsBatchOut struct {
	IDs        []ULID
	Embeddings [][]float32
}

type EmbeddingsRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type EmbeddingEntity struct {
	Embedding []float32 `json:"embedding"`
}

type EmbeddingsResponse struct {
	Data []EmbeddingEntity `json:"data"`
}

func CreateEmbeddings(c AIConfig, input []string) ([]EmbeddingEntity, error) {
	reqBody, err := json.Marshal(EmbeddingsRequest{
		Input: input,
		Model: c.EmbeddingsModel,
	})
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(reqBody)
	req, err := http.NewRequest(http.MethodPost, c.TEIBaseURL, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.TEIAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrFailedCreateEmbeddingsRequest
	}

	var parsed EmbeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
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

var (
	DefaultStopCharsPath = path.Join("data", "stopchars.txt")
	DefaultStopWordsPath = path.Join("data", "stopwords.txt")
	DefaultLemmasPath    = path.Join("data", "lemmas.csv")
)

var (
	DefaultStopChars map[rune]bool     = make(map[rune]bool)
	DefaultStopWords map[string]bool   = make(map[string]bool)
	DefaultLemmas    map[string]string = make(map[string]string)
)

func LoadStopChars() error {
	slog.Debug("loading stop characters")

	f, err := os.Open(DefaultStopCharsPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		line := scanner.Text()
		for _, r := range line {
			if r != '\n' && r != '\r' {
				DefaultStopChars[r] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func LoadStopWords() error {
	slog.Debug("loading stop words")

	f, err := os.Open(DefaultStopWordsPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		DefaultStopWords[word] = true
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func LoadLemmas() error {
	slog.Debug("loading lemmas")

	f, err := os.Open(DefaultLemmasPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ",", 2)
		word := strings.TrimSpace(parts[0])
		lemma := strings.TrimSpace(parts[1])
		DefaultLemmas[word] = lemma
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func LoadLanguageData() error {
	if err := LoadStopChars(); err != nil {
		return err
	}
	if err := LoadStopWords(); err != nil {
		return err
	}
	if err := LoadLemmas(); err != nil {
		return err
	}
	return nil
}

var (
	HTMLTagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	ScriptTagRe = regexp.MustCompile(`(?is)<script.*?>.*?</script>`)
	SQLRe       = regexp.MustCompile(`(?i)\b(select|insert|update|delete|drop|truncate|alter)\b`)
)

func PreprocessText(text string) []string {
	text = strings.ToLower(text)
	text = ScriptTagRe.ReplaceAllString(text, " ")
	text = HTMLTagRe.ReplaceAllString(text, " ")
	text = SQLRe.ReplaceAllString(text, " ")
	text += " "

	var result []string
	token := make([]rune, 0, 32)

	for _, r := range text {
		if DefaultStopChars[r] || unicode.IsSpace(r) {
			if len(token) > 0 {
				word := string(token)
				if !DefaultStopWords[word] {
					if lemma, ok := DefaultLemmas[word]; ok {
						word = lemma
					}
					result = append(result, word)
				}
				token = token[:0] // reset token
			}
			continue
		}

		token = append(token, r)
	}

	return result
}

const DefaultEmbeddingsBatchSize = 64

func RunEmbeddingsJob(c AIConfig, s Store) error {
	// TODO: test this store method, rethink it once more
	ids, texts, err := s.FetchPendingEmbeddingsMetadata(DefaultEmbeddingsBatchSize)
	if err != nil || len(ids) == 0 {
		return err
	}

	// TODO: test this store method, rethink it once more
	batchOut, err := CreateEmbeddings(c, texts)
	if err != nil {
		// TODO: test this store method, rethink it once more
		return s.MarkEmbeddingsStatus(ids, EmbeddingStatusPending)
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}

	// TODO: test this store method, rethink it once more
	if err := s.UpsertEmbeddings(tx, ids, batchOut); err != nil {
		tx.Rollback()
		return err
	}

	// TODO: test this store method, rethink it once more
	if err := s.MarkEmbeddingsStatusTx(tx, ids, EmbeddingStatusDone); err != nil {
		tx.Rollback()
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

func Rerank(
	c AIConfig,
	candidate string,
	positions []ULID,
) ([]ULID, error) {
	return nil, nil
}

const (
	DefaultCandidatesBatchSize       = 32
	DefaultRecommendationsDailyLimit = 32
)

const DefaultNearestNeighbors = 128

func RunRecommendationsJob(c AIConfig, s Store) error {
	// TODO: fix the reference table
	candidateIDs, candidateTexts, err := s.GetCandidates(
		DefaultCandidatesBatchSize,
		DefaultRecommendationsJobFrequency,
	)
	if err != nil {
		return err
	}

	for i := range len(candidateIDs) {
		var positionIDs []ULID

		if c.UseEmbeddings {
			// TODO: review this store function
			positionIDs, err = s.GetPositionsForCandidateWithEmbedding(
				candidateIDs[i],
				DefaultNearestNeighbors,
			)
			if err != nil {
				slog.Error(
					"failed to find similar positions using embeddings",
					"candidateID", candidateIDs[i],
					"err", err,
				)
				continue
			}
		} else {
			// use BM25
		}

		if len(candidateIDs) == 0 {
			slog.Debug(
				"failed to find suitable positions",
				"candidateID", candidateIDs[i],
			)
			continue
		}

		if c.UseReranker {
			positionIDs, err = Rerank(c, candidateTexts[i], positionIDs)
			if err != nil {
				slog.Error("rerank failed", "candidateID", candidateIDs[i], "err", err)
				continue
			}
		}

		limit := min(len(positionIDs), int(DefaultRecommendationsDailyLimit))
		recommendations := make([]Recommendation, 0, limit)
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

		if err := s.CreateRecommendations(recommendations); err != nil {
			return err
		}
	}

	return nil
}

const (
	DefaultEmbeddingsJobFrequency      = 1 * time.Hour
	DefaultRecommendationsJobFrequency = 24 * time.Hour
)

func RunServer(ctx context.Context, c ServerConfig, s Store, v Vault) error {
	server, err := NewServer(ctx, c, s, v)
	if err != nil {
		return err
	}

	slog.Debug("creating listener", "addr", server.Addr)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	slog.Info("HTTP server starting", "addr", server.Addr)
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info("HTTP server ready", "addr", server.Addr)

	if err := LoadLanguageData(); err != nil {
		return err
	}

	if err := LoadBM25Data(); err != nil {
		return err
	}

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
			for range time.Tick(DefaultEmbeddingsJobFrequency) {
				slog.Info("running embeddings job")
				RunEmbeddingsJob(aiConfig, s)
			}
		}()
	}

	go func() {
		for range time.Tick(DefaultRecommendationsJobFrequency) {
			slog.Info("running recommendations job")
			RunRecommendationsJob(aiConfig, s)
		}
	}()

	return WaitAndShutdown(ctx, server, errCh, c.GracePeriod)
}

func NewServer(
	ctx context.Context,
	c ServerConfig,
	s Store,
	v Vault,
) (*http.Server, error) {
	slog.Debug("initializing server")
	return &http.Server{
		Addr:         c.ServerBaseURL,
		ReadTimeout:  c.RequestReadTimeout,
		WriteTimeout: c.RequestWriteTimeout,
		Handler:      RootMux(s, v),
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}, nil
}

func WaitAndShutdown(
	ctx context.Context,
	server *http.Server,
	errCh chan error,
	gracePeriod time.Duration,
) error {
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

func Chain(handl http.HandlerFunc, mdws ...Middleware) http.HandlerFunc {
	wrapped := handl
	for i := len(mdws) - 1; i >= 0; i-- {
		wrapped = mdws[i](wrapped)
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

func GetUserID(r *http.Request) (ULID, bool) {
	userID, ok := r.Context().Value(ContextKeyUserID).(ULID)
	return userID, ok
}

func GetClaims(r *http.Request) (*AccessTokenClaims, bool) {
	claims, ok := r.Context().Value(ContextKeyClaims).(*AccessTokenClaims)
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

func Authentication(v Vault, allowedRoles []Role) Middleware {
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

			allowed := make(map[Role]bool, len(allowedRoles))
			for _, s := range allowedRoles {
				allowed[s] = true
			}

			for role := range claims.Roles {
				if _, ok := allowed[role]; !ok {
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

type Method string

const (
	MethodGet  Method = http.MethodGet
	MethodPost Method = http.MethodPost
)

type RouteConfig struct {
	Mux           *http.ServeMux
	Method        Method
	Route         string
	Handler       http.HandlerFunc
	RequiredRoles []Role
}

const (
	RouteHealth            = "/health"
	RouteOAuthToken        = "/oauth/token"
	RouteOAuthAuthorize    = "/oauth/authorize"
	RouteOAuthCallback     = "/oauth/callback"
	RouteMeRecommendations = "/v1/me/recommendations"
	RouteMeReactions       = "/v1/me/reactions"
	RouteMeMatches         = "/v1/me/matches"
	RouteMeReaction        = "/v1/me/recommendations/{id}/reaction"
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

func PublicRoute(cfg RouteConfig) {
	handler := BaseMiddleware(cfg.Handler)

	cfg.Mux.Handle(
		Route(cfg.Method, cfg.Route),
		handler,
	)
}

func ProtectedRoute(cfg RouteConfig, v Vault) {
	handler := Chain(
		cfg.Handler,
		Logger,
		PanicHandler,
		MaxBytesLimiter,
		Authentication(v, cfg.RequiredRoles),
	)

	cfg.Mux.Handle(
		Route(cfg.Method, cfg.Route),
		handler,
	)
}

func RootMux(s Store, v Vault) http.Handler {
	mux := http.NewServeMux()

	PublicRoute(RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteHealth,
		Handler: Health,
	})

	if v.UseGoogleSSO || v.UseAppleSSO {
		PublicRoute(RouteConfig{
			Mux:     mux,
			Method:  MethodPost,
			Route:   RouteOAuthToken,
			Handler: OAuthToken(s, v),
		})

		PublicRoute(RouteConfig{
			Mux:     mux,
			Method:  MethodGet,
			Route:   RouteOAuthAuthorize,
			Handler: OAuthAuthorize(v),
		})

		PublicRoute(RouteConfig{
			Mux:     mux,
			Method:  MethodPost,
			Route:   RouteOAuthAuthorize,
			Handler: OAuthAuthorize(v),
		})

		PublicRoute(RouteConfig{
			Mux:     mux,
			Method:  MethodGet,
			Route:   RouteOAuthCallback,
			Handler: OAuthCallback(s, v),
		})

		PublicRoute(RouteConfig{
			Mux:     mux,
			Method:  MethodPost,
			Route:   RouteOAuthCallback,
			Handler: OAuthCallback(s, v),
		})
	} else {
		slog.Warn(`SSO is not configured, /oauth/authorize
			and /oauth/callback routes are disabled`)
	}

	ProtectedRoute(RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteMeRecommendations,
		Handler: GetMeRecommendations(s),
		RequiredRoles: []Role{
			RoleCandidate, RoleRecruiter,
		},
	}, v)

	ProtectedRoute(RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteMeReactions,
		Handler: GetMeReactions(s),
		RequiredRoles: []Role{
			RoleCandidate, RoleRecruiter,
		},
	}, v)

	ProtectedRoute(RouteConfig{
		Mux:     mux,
		Method:  MethodGet,
		Route:   RouteMeMatches,
		Handler: GetMeMatches(s),
		RequiredRoles: []Role{
			RoleCandidate, RoleRecruiter,
		},
	}, v)

	ProtectedRoute(RouteConfig{
		Mux:     mux,
		Method:  MethodPost,
		Route:   RouteMeReaction,
		Handler: CreateMeReaction(s),
		RequiredRoles: []Role{
			RoleCandidate, RoleRecruiter,
		},
	}, v)

	return mux
}

// [RFC6749](https://www.rfc-editor.org/rfc/rfc6749.txt).
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

func AuthError(w http.ResponseWriter, code AuthErrorCode, desc string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)

	resp := AuthErrorResponse{Error: code, ErrorDescription: desc}
	WriteJSON(w, http.StatusBadRequest, resp)
}

func Unauthorized(w http.ResponseWriter, code AuthErrorCode, desc string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	SetUnauthorizedHeaders(w)

	resp := AuthErrorResponse{Error: code, ErrorDescription: desc}
	WriteJSON(w, http.StatusUnauthorized, resp)
}

func OAuthToken(s Store, v Vault) http.HandlerFunc {
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

		isRefreshTokenRevoked, err := s.IsActiveSession(claims.JTI)
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

		roles, err := s.GetUserRoles(
			claims.UserID,
			Provider(claims.Provider),
		)
		if err != nil {
			slog.Error(
				"failed to get roles for the user",
				"err", err,
				"user_id", claims.UserID,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		accessToken, err := v.CreateAccessToken(
			claims.UserID,
			claims.Provider,
			roles,
		)
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

func OAuthAuthorize(v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerRaw := r.URL.Query().Get("provider")
		provider, err := ToProvider(providerRaw, DefaultProvider)
		if err != nil {
			AuthError(
				w,
				AuthInvalidRequest,
				"invalid provider; must be one of: google, apple",
			)
			return
		}

		state, err := v.CreateStateToken(provider)
		if err != nil {
			slog.Error("generation of state token failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		parsed, err := v.ParseStateToken(state)
		if err != nil {
			slog.Error("failed to parse state token", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_csrf",
			Value:    parsed.CSRF,
			Path:     "/",
			MaxAge:   int(DefaultStateTokenExpiration.Seconds()),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		verifier := oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_verifier",
			Value:    verifier,
			Path:     "/",
			MaxAge:   int(DefaultVerifierExpiration.Seconds()),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		url, err := v.CreateAuthCodeURL(state, verifier, provider)
		if err != nil {
			slog.Error("generation of auth code url failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func OAuthCallback(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(
			r.Context(),
			DefaultStateTokenExpiration,
		)
		defer cancel()

		stateRaw := r.URL.Query().Get("state")
		if stateRaw == "" {
			AuthError(w, AuthInvalidRequest, "missing state")
			return
		}

		state, err := v.ParseStateToken(stateRaw)
		if err != nil {
			AuthError(w, AuthInvalidRequest, "invalid state token")
			return
		}

		csrfCookie, err := r.Cookie("oauth_csrf")
		if err != nil || csrfCookie.Value != state.CSRF {
			AuthError(w, AuthInvalidRequest, "invalid CSRF token")
			return
		}

		verifierCookie, err := r.Cookie("oauth_verifier")
		if err != nil {
			AuthError(w, AuthInvalidRequest, "missing PKCE verifier")
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			AuthError(w, AuthInvalidRequest, "authorization provider error")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			AuthError(w, AuthInvalidRequest, "missing authorization code")
			return
		}

		DeleteCookies(w, []string{"oauth_csrf", "oauth_verifier"})

		var user *User
		switch state.Provider {
		case ProviderGoogle:
			rawIDToken, err := v.ExchangeGoogleCodeForIDToken(
				ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				AuthError(w, AuthInvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error("Google code exchange failed", "err", err)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

			user, err = v.VerifyAndParseGoogleIDToken(ctx, rawIDToken)
			if errors.Is(err, ErrInvalidIDToken) {
				AuthError(w, AuthInvalidRequest, "invalid id_token")
				return
			}
			if errors.Is(err, ErrFailedParseClaims) {
				AuthError(w, AuthInvalidRequest, "failed to parse claims")
				return
			}
			if errors.Is(err, ErrEmailNotVerified) {
				AuthError(w, AuthInvalidRequest, "unverified provider email")
				return
			}
			if err != nil {
				slog.Error("Google ID token verification failed", "err", err)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

		case ProviderApple:
			rawIDToken, err := v.ExchangeAppleCodeForIDToken(
				ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				AuthError(w, AuthInvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error("Apple code exchange failed", "err", err)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

			user, err = v.VerifyAndParseAppleIDToken(
				ctx,
				rawIDToken,
				r.FormValue("user"),
			)
			if errors.Is(err, ErrInvalidIDToken) {
				AuthError(w, AuthInvalidRequest, "invalid id_token")
				return
			}
			if errors.Is(err, ErrFailedParseClaims) {
				AuthError(w, AuthInvalidRequest, "failed to parse claims")
				return
			}
			if err != nil {
				slog.Error("Apple ID token verification failed", "err", err)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

		default:
			AuthError(
				w,
				AuthInvalidRequest,
				"invalid provider; must be one of: google, apple",
			)
			return
		}

		FinishAuthFlow(s, v, w, *user)
	}
}

func FinishAuthFlow(s Store, v Vault, w http.ResponseWriter, user User) {
	userID, roles, err := s.GetUserByProvider(user.Provider, user.ProviderUserID)

	if errors.Is(err, ErrUserNotFound) {
		userID, ulidErr := NewUserULID()
		if ulidErr != nil {
			slog.Error("ULID generation failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		user.UserName, err = GenerateUsername()
		if err != nil {
			slog.Error("username generation failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		err := s.CreateUser(user)
		if err != nil {
			slog.Error("query failed", "err", err)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		CreateOnboardingToken(v, w, userID, user.Provider)
		return
	}
	if errors.Is(err, ErrUserNoRole) {
		CreateOnboardingToken(v, w, userID, user.Provider)
		return
	}
	if err != nil {
		slog.Error("query failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	CreateTokenPair(s, v, w, userID, user.Provider, roles)
}

func CreateOnboardingToken(
	v Vault,
	w http.ResponseWriter,
	userID ULID, provider Provider,
) {
	accessToken, err := v.CreateAccessToken(
		userID,
		provider,
		map[Role]ULID{RoleOnboarding: userID},
	)
	if err != nil {
		slog.Error("failed to create access token", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	AuthAccessToken(w, *accessToken)
}

func CreateTokenPair(
	s Store,
	v Vault,
	w http.ResponseWriter,
	userID ULID,
	provider Provider,
	roles map[Role]ULID,
) {
	jti, err := NewJTIULID()
	if err != nil {
		slog.Error("ULID generation failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	err = s.CreateRefreshToken(jti,
		userID,
		time.Now().UTC().Add(DefaultRefreshTokenExpiration.Abs()),
	)
	if err != nil {
		slog.Error("query failed", "err", err)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	tokenPair, err := v.CreateTokenPair(userID, provider, jti, roles)
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

type (
	// [JSend](https://github.com/omniti-labs/jsend)
	FailData       map[string]string
	ResponseStatus string
	ErrorCode      uint16
)

type (
	// [HAL](https://datatracker.ietf.org/doc/html/draft-kelly-json-hal-11)
	Link struct {
		Href      string `json:"href"`
		Name      string `json:"name,omitempty"`
		Templated bool   `json:"templated,omitempty"`
	}

	RelType string

	Links    map[RelType]Link
	Embedded map[string]any
	Props    map[string]any

	Resource struct {
		Links    Links    `json:"_links,omitempty"`
		Embedded Embedded `json:"_embedded,omitempty"`
		Props    Props    `json:"-"`
	}
)

type (
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
)

var (
	adjectives = []string{
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

	nouns = []string{
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

const (
	// All went well, and (usually) some data was returned.
	ResponseStatusSuccess = "success"

	// There was a problem with the data submitted or some pre-condition failed
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

	// An IRI that refers to the furthest preceding resource in a series.
	RelTypeFirst RelType = "first"

	// An IRI that refers to the furthest following resource in a series.
	RelTypeLast RelType = "last"

	// Refers to an index.
	RelTypeIndex RelType = "index"

	// Refers to a resource offering help.
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
	WriteJSON(w, status, ErrorResponse{
		Status:  ResponseStatusError,
		Message: message,
	})
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
		return nil, err
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}

	if err := r.Body.Close(); err != nil {
		return nil, err
	}

	return &data, nil
}

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
		return "", err
	}

	username := fmt.Sprintf("%s_%s%s", adj, noun, hex.EncodeToString(suffix))
	username = strings.ToLower(username)

	return username, nil
}

func Health(w http.ResponseWriter, r *http.Request) {
	Success(w, http.StatusOK, Resource{Props: Props{}})
}

func GetMeRecommendations(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, http.StatusInternalServerError, "internal server error")
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
		posNextCursor, canNextCursor := "done", "done"
		page.Count = 0
		embedded := Embedded{}

		candidateID, isCandidate := claims.Roles[RoleCandidate]
		recruiterID, isRecruiter := claims.Roles[RoleRecruiter]
		if !isCandidate && !isRecruiter {
			slog.Error("user has neither candidate nor recruiter role")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		posCursor := q.Get("pos_cursor")
		if isCandidate && q.Get("exclude_positions") != "true" && posCursor != "done" {
			recs, cursor, err := s.GetPositionRecommendations(
				candidateID,
				Page{Cursor: posCursor, Limit: page.Limit},
				excludeReacted,
			)
			if err != nil {
				slog.Error("failed to fetch position recommendations", "err", err)
				Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			posNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recs)
			positions := make([]Resource, len(recs))
			for i, rec := range recs {
				positions[i] = Resource{
					Links: Links{
						RelTypeSelf: Link{
							Href: fmt.Sprintf(
								"%s/%s",
								RouteMeRecommendations,
								rec.RecommendationID),
						},
						RelType("reaction"): Link{
							Href: fmt.Sprintf(
								"%s/%s/reaction",
								RouteMeRecommendations,
								rec.RecommendationID),
						},
					}, Props: Props{
						"recommendation_id": rec.RecommendationID,
						"position_id":       rec.PositionID,
						"title":             rec.Title,
						"company":           rec.Company,
						"description":       rec.Description,
					},
				}
			}
			if len(positions) > 0 {
				embedded["positions"] = positions
			}
		}

		canCursor := q.Get("can_cursor")
		if isRecruiter &&
			q.Get("exclude_candidates") != "true" &&
			canCursor != "done" {
			recs, cursor, err := s.GetCandidateRecommendations(
				recruiterID,
				Page{Cursor: canCursor, Limit: page.Limit},
				excludeReacted,
			)
			if err != nil {
				slog.Error("failed to fetch candidate recommendations", "err", err)
				Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			canNextCursor = cmp.Or(string(cursor), "done")
			page.Count += len(recs)
			candidates := make([]Resource, len(recs))
			for i, rec := range recs {
				candidates[i] = Resource{
					Links: Links{
						RelTypeSelf: Link{
							Href: fmt.Sprintf(
								"%s/%s",
								RouteMeRecommendations,
								rec.RecommendationID,
							),
						},
						RelType("reaction"): Link{
							Href: fmt.Sprintf(
								"%s/%s/reaction",
								RouteMeRecommendations,
								rec.RecommendationID,
							),
						},
					},
					Props: Props{
						"recommendation_id": rec.RecommendationID,
						"candidate_id":      rec.CandidateID,
						"full_name":         rec.FullName,
						"about":             rec.About,
					},
				}
			}
			if len(candidates) > 0 {
				embedded["candidates"] = candidates
			}
		}

		page.HasNext = posNextCursor != "done" || canNextCursor != "done"

		selfHref := RouteMeRecommendations
		if excludeReacted {
			selfHref += "?exclude_reacted=true"
		}
		links := Links{
			RelTypeSelf:          Link{Href: selfHref},
			RelType("reactions"): Link{Href: RouteMeReactions},
		}
		if page.HasNext {
			nextHref := fmt.Sprintf(
				"%s?pos_cursor=%s&can_cursor=%s&limit=%d",
				RouteMeRecommendations, posNextCursor, canNextCursor, page.Limit,
			)
			if excludeReacted {
				nextHref += "&exclude_reacted=true"
			}
			links[RelTypeNext] = Link{Href: nextHref}
		}

		Success(w, http.StatusOK, Resource{
			Links:    links,
			Embedded: embedded,
			Props:    Props{"page": page},
		})
	}
}

func CreateMeReaction(s Store) http.HandlerFunc {
	type RequestBody struct {
		ReactionType ReactionType `json:"reaction_type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		recommendationID := ULID(r.PathValue("id"))
		if recommendationID == "" {
			Fail(
				w,
				http.StatusBadRequest,
				FailData{"id": "recommendation id is required"},
			)
			return
		}

		rec, err := s.GetRecommendation(recommendationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				Fail(
					w,
					http.StatusNotFound,
					FailData{"id": "recommendation not found"},
				)
				return
			}
			slog.Error("failed to fetch recommendation", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if rec.CandidateID != candidateID {
			Fail(w, http.StatusForbidden, FailData{"id": "forbidden for this id"})
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			Fail(w, http.StatusBadRequest, FailData{"body": "invalid request body"})
			return
		}
		if !body.ReactionType.IsValid() {
			Fail(
				w,
				http.StatusBadRequest,
				FailData{
					"reaction_type": "must be one of: positive, negative, neutral",
				},
			)
			return
		}

		if err := s.CreateReaction(Reaction{
			RecommendationID: recommendationID,
			ReactorType:      ReactorTypeCandidate,
			ReactorID:        candidateID,
			ReactionType:     body.ReactionType,
		}); err != nil {
			if errors.Is(err, ErrReactionAlreadyExists) {
				Fail(
					w,
					http.StatusConflict,
					FailData{
						"request": "reaction already exists; reactions are immutable",
					},
				)
				return
			}
			slog.Error("failed to record reaction", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		Success(w, http.StatusCreated, Resource{
			Links: Links{
				RelTypeSelf: Link{
					Href: fmt.Sprintf(
						"%s/%s/reaction",
						RouteMeRecommendations,
						recommendationID,
					),
				},
				RelTypeUp:            Link{Href: RouteMeRecommendations},
				RelType("reactions"): Link{Href: RouteMeReactions},
				RelType("matches"):   Link{Href: RouteMeMatches},
			},
			Props: Props{
				"status": "success",
			},
		})
	}
}

func GetMeReactions(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		page := GetPagination(r)

		reactions, nextCursor, err := s.GetReactionsByCandidateID(
			candidateID,
			page,
		)
		if err != nil {
			slog.Error("failed to fetch candidate profile", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		page.Count = len(reactions)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: RouteMeReactions},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{
				Href: fmt.Sprintf(
					"%s?cursor=%s",
					RouteMeReactions,
					nextCursor,
				),
			}
		}

		embedded := make([]Resource, len(reactions))
		for i, rx := range reactions {
			embedded[i] = Resource{
				Links: Links{
					RelTypeSelf: Link{
						Href: fmt.Sprintf(
							"%s/%s/reaction",
							RouteMeRecommendations,
							rx.RecommendationID,
						),
					},
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

func GetMeMatches(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		page := GetPagination(r)

		matches, nextCursor, err := s.GetMatchesByCandidateID(candidateID, page)
		if err != nil {
			slog.Error("failed to fetch matches", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		page.Count = len(matches)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: RouteMeMatches},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{
				Href: fmt.Sprintf(
					"%s?cursor=%s&limit=%d",
					RouteMeMatches,
					nextCursor,
					page.Limit,
				),
			}
		}

		embedded := make([]Resource, len(matches))
		for i, m := range matches {
			embedded[i] = Resource{
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
