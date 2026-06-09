// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

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
	"net/mail"
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
	ErrTextForbiddenChars            = errors.New("text contains forbidden characters")
	ErrTextTooLong                   = errors.New("text too long")
	ErrTextTooShort                  = errors.New("text too short")
	ErrEmailNotVerified              = errors.New("email not verified")
	ErrEmbeddingsCountMismatch       = errors.New("mismatched IDs and embeddings count")
	ErrExtraDataDecoded              = errors.New("extra data decoded")
	ErrFailedEncodeEmbeddingsRequest = errors.New("failed to encode embeddings request")
	ErrFailedShutdownServer          = errors.New("failed to shutdown server")
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

type EmbeddingEntity struct {
	Embedding []float32 `json:"embedding"`
}

type EmbeddingsRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
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
		return nil, fmt.Errorf(
			"embedding endpoint returned %d",
			resp.StatusCode,
		)
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

func LoadBM25Cache() error {
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

	tx, err := s.db.Begin()
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

		if len(positionIDs) == 0 {
			slog.Debug(
				"failed to find suitable positions",
				"candidateID", candidateIDs[i],
			)
			continue
		}

		if c.UseReranker {
			positionIDs, err = Rerank(c, candidateTexts[i], positionIDs)
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

	slog.Debug("starting server", "addr", server.Addr)
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	slog.Info("server ready", "addr", server.Addr)

	if err := LoadLanguageData(); err != nil {
		return err
	}

	if err := LoadBM25Cache(); err != nil {
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
		Handler:      ServeMux(s, v),
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
		slog.Error(
			"failed to gracefully shutdown, forcing close",
			"err", err,
		)
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

func PanicRecoverer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"recovered from panic",
					"panic", err,
					"method", r.Method,
					"path", r.URL.Path,
				)

				Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}
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

func Authentication(v Vault, roles map[Role]bool) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			bearer, found := strings.CutPrefix(authHeader, "Bearer ")
			if !found || bearer == "" {
				Unauthorized(w, AuthInvalidClient, "Bearer token is required")
				return
			}

			claims, err := v.ParseAccessToken(bearer)
			if err != nil {
				AuthError(w, AuthInvalidGrant, "invalid access token")
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
	MethodPost  Method = http.MethodPost
	MethodGet   Method = http.MethodGet
	MethodPatch Method = http.MethodPatch
)

type RouteConfig struct {
	Method  Method
	Handler http.HandlerFunc
	Routes  []Route
	Roles   []Role
}

func PublicRoute(c RouteConfig) {
	handler := Chain(
		c.Handler,
		Logger,
		PanicRecoverer,
		MaxBytesLimiter,
	)

	for _, route := range c.Routes {
		DefaultServeMux.Handle(
			fmt.Sprintf("%s %s", c.Method, route),
			handler,
		)
	}
}

func ProtectedRoute(c RouteConfig, v Vault) {
	rolesMap := make(map[Role]bool)
	for _, role := range c.Roles {
		rolesMap[role] = true
	}

	handler := Chain(
		c.Handler,
		Logger,
		PanicRecoverer,
		MaxBytesLimiter,
		Authentication(v, rolesMap),
	)

	for _, route := range c.Routes {
		DefaultServeMux.Handle(
			fmt.Sprintf("%s %s", c.Method, route),
			handler,
		)
	}
}

type Route string

// Routes
const (
	// Unversioned (stable) routes
	RouteHealth        Route = "/health"
	RouteAccessToken   Route = "/auth/token"
	RouteAuthorize     Route = "/auth/authorize"
	RouteOAuthCallback Route = "/auth/callback"

	// Versioned routes
	RouteV1Users             Route = "/v1/users"
	RouteV1UsersMe           Route = "/v1/users/me"
	RouteV1Candidates        Route = "/v1/candidates"
	RouteV1Recruiters        Route = "/v1/recruiters"
	RouteV1MeRecommendations Route = "/v1/me/recommendations"
	RouteV1MeReactions       Route = "/v1/me/reactions"
	RouteV1MeMatches         Route = "/v1/me/matches"
	RouteV1MeReaction        Route = "/v1/me/recommendations/{id}/reaction"

	// Latest version (default) routes
	RouteUsers             Route = "/users"
	RouteUsersMe           Route = "/users/me"
	RouteCandidates        Route = "/candidates"
	RouteRecruiters        Route = "/recruiters"
	RouteMeRecommendations Route = "/me/recommendations"
	RouteMeReactions       Route = "/me/reactions"
	RouteMeMatches         Route = "/me/matches"
	RouteMeReaction        Route = "/me/recommendations/{id}/reaction"
)

var DefaultServeMux = http.NewServeMux()

func ServeMux(s Store, v Vault) http.Handler {
	slog.Debug("registering routes")

	PublicRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteHealth},
		Handler: Health,
	})

	PublicRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteAuthorize},
		Handler: HandlerAuthorize(s, v),
	})

	PublicRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteAuthorize},
		Handler: HandlerAuthorize(s, v),
	})

	PublicRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteAccessToken},
		Handler: HandlerCreateAccessToken(s, v),
	})

	PublicRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteOAuthCallback},
		Handler: HandlerOAuthCallback(s, v),
	})

	PublicRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteOAuthCallback},
		Handler: HandlerOAuthCallback(s, v),
	})

	PublicRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteV1Users, RouteUsers},
		Handler: HandlerCreateUser(s, v),
	})

	ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteV1UsersMe, RouteUsersMe},
		Handler: HandlerGetUsersMe(s, v),
	}, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodPatch,
		Routes:  []Route{RouteV1UsersMe, RouteUsersMe},
		Handler: HandlerPatchUsersMe(s, v),
	}, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodDelete,
	// 	Route:   RouteUsersMe,
	// 	Handler: HandlerDeleteUsersMe(s, v),
	// }, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteV1Candidates, RouteCandidates},
		Handler: HandlerCreateCandidate(s, v),
	}, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RouteCandidatesMe,
	// 	Handler: HandlerGetCandidatesMe(s, v),
	// 	Roles:   []Role{RoleCandidate},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodPatch,
	// 	Route:   RouteCandidatesMe,
	// 	Handler: HandlerDeleteCandidatesMe(s, v),
	// 	Roles:   []Role{RoleCandidate},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodDelete,
	// 	Route:   RouteCandidatesMe,
	// 	Handler: HandlerDeleteCandidatesMe(s, v),
	// 	Roles:   []Role{RoleCandidate},
	// }, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteV1Recruiters, RouteRecruiters},
		Handler: HandlerCreateRecruiter(s, v),
	}, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RouteRecruitersMe,
	// 	Handler: HandlerGetRecruitersMe(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodPatch,
	// 	Route:   RouteRecruitersMe,
	// 	Handler: HandlerPatchRecruitersMe(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodDelete,
	// 	Route:   RouteRecruitersMe,
	// 	Handler: HandlerDeleteRecruitersMe(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodPost,
	// 	Route:   RoutePositions,
	// 	Handler: HandlerCreatePosition(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RoutePosition,
	// 	Handler: HandlerGetPosition(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodPatch,
	// 	Route:   RoutePosition,
	// 	Handler: HandlerPatchPosition(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodDelete,
	// 	Route:   RoutePosition,
	// 	Handler: HandlerDeletePosition(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	// ProtectedRoute(RouteConfig{
	// 	Method:  MethodGet,
	// 	Route:   RouteMePositions,
	// 	Handler: HandlerGetMePositions(s, v),
	// 	Roles:   []Role{RoleRecruiter},
	// }, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteV1MeRecommendations, RouteMeRecommendations},
		Handler: HandlerGetMeRecommendations(s),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	}, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteV1MeReactions, RouteMeReactions},
		Handler: HandlerGetMeReactions(s),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	}, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodGet,
		Routes:  []Route{RouteV1MeMatches, RouteMeMatches},
		Handler: HandlerGetMeMatches(s),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	}, v)

	ProtectedRoute(RouteConfig{
		Method:  MethodPost,
		Routes:  []Route{RouteV1MeReaction, RouteMeReaction},
		Handler: HandlerCreateMeReaction(s),
		Roles:   []Role{RoleCandidate, RoleRecruiter},
	}, v)

	return DefaultServeMux
}

// See [RFC6749](https://www.rfc-editor.org/info/rfc6749).
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
	WriteJSON(w, accessToken, http.StatusOK)
}

func AuthTokenPair(w http.ResponseWriter, tokenPair TokenPair) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	WriteJSON(w, tokenPair, http.StatusOK)
}

func AuthError(w http.ResponseWriter, code AuthErrorCode, desc string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	WriteJSON(w, AuthErrorResponse{
		Error:            code,
		ErrorDescription: desc,
	}, http.StatusBadRequest)
}

func Unauthorized(w http.ResponseWriter, code AuthErrorCode, desc string) {
	SetDefaultHeaders(w)
	SetAuthHeaders(w)
	SetUnauthorizedHeaders(w)
	WriteJSON(w, AuthErrorResponse{
		Error:            code,
		ErrorDescription: desc,
	}, http.StatusUnauthorized)
}

func HandlerCreateAccessToken(s Store, v Vault) http.HandlerFunc {
	type RequestBody struct {
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBody](r)
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
				"failed to parse refresh token",
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
				"err", err,
			)
			AuthError(w, AuthInvalidGrant, "invalid refresh token")
			return
		}

		isRevoked, err := s.IsRevokedRefreshToken(claims.JTI)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn(
					"refresh token not found",
					"jti", claims.JTI,
					"user_id", claims.UserID,
					"ip", r.RemoteAddr,
				)
				AuthError(w, AuthInvalidGrant, "invalid refresh token")
				return
			}
			slog.Error(
				"failed to validate refresh token",
				"err", err,
				"jti", claims.JTI,
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		if isRevoked {
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
				"failed to get user roles",
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
				"err", err,
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
				"failed to create access token",
				"user_id", claims.UserID,
				"ip", r.RemoteAddr,
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		AuthAccessToken(w, *accessToken)
	}
}

func HandlerAuthorize(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerRaw := r.URL.Query().Get("provider")
		provider, err := StringToProvider(providerRaw, ProviderEmail)
		if err != nil {
			AuthError(
				w,
				AuthInvalidRequest,
				"invalid provider; must be one of: google, apple, email",
			)
			return
		}

		if provider == ProviderEmail {
			var req struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				AuthError(w, AuthInvalidRequest, "invalid request body")
				return
			}
			if req.Email == "" || req.Password == "" {
				AuthError(w, AuthInvalidRequest, "email and password required")
				return
			}

			user, roles, err := s.GetUserByEmail(req.Email, ProviderEmail)
			if errors.Is(err, ErrUserNotFound) {
				Unauthorized(w, AuthInvalidRequest, "invalid credentials")
				return
			}
			if user != nil {
				if !v.IsValidPassword(user.PasswordHash, req.Password) {
					Unauthorized(w, AuthInvalidRequest, "invalid credentials")
					return
				}
				if errors.Is(err, ErrUserNoRole) {
					// TODO: indicate next actions according to HAL
					CreateAccessToken(v, w,
						user.ID,
						user.Provider,
						map[Role]ULID{},
					)
					return
				}
			}
			if err != nil {
				slog.Error(
					"failed to get user by email",
					"err", err,
				)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

			CreateTokenPair(s, v, w,
				user.ID,
				user.Provider,
				roles,
			)
			return
		}

		state, err := v.CreateStateToken(provider)
		if err != nil {
			slog.Error(
				"failed to generate state token",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		parsed, err := v.ParseStateToken(state)
		if err != nil {
			slog.Error(
				"failed to parse state token",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_csrf",
			Value:    parsed.CSRF,
			Path:     "/",
			MaxAge:   int(v.StateTokenExpiration),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		verifier := oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_verifier",
			Value:    verifier,
			Path:     "/",
			MaxAge:   int(v.VerifierExpiration),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		url, err := v.CreateAuthCodeURL(state, verifier, provider)
		if err != nil {
			slog.Error(
				"failed to generate auth code URL",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		http.Redirect(w, r,
			url,
			http.StatusTemporaryRedirect,
		)
	}
}

func HandlerOAuthCallback(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(
			r.Context(),
			v.StateTokenExpiration,
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

		DeleteCookies(w, [2]string{"oauth_csrf", "oauth_verifier"})

		var user *User
		switch state.Provider {
		case ProviderGoogle:
			rawIDToken, err := v.ExchangeGoogleCodeForIDToken(ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				AuthError(w, AuthInvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error(
					"failed to exchange Google code",
					"err", err,
				)
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
				slog.Error(
					"failed to verify Google ID token",
					"err", err,
				)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

		case ProviderApple:
			rawIDToken, err := v.ExchangeAppleCodeForIDToken(ctx,
				code,
				verifierCookie,
			)
			if errors.Is(err, ErrIDTokenRequired) {
				AuthError(w, AuthInvalidRequest, "id_token is required")
				return
			}
			if err != nil {
				slog.Error(
					"failed to exchange Apple code",
					"err", err,
				)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

			user, err = v.VerifyAndParseAppleIDToken(ctx,
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
				slog.Error(
					"failed to verify Apple ID token",
					"err", err,
				)
				AuthError(w, AuthInvalidRequest, "internal server error")
				return
			}

		default:
			AuthError(w,
				AuthInvalidRequest,
				"invalid provider; must be one of: google, apple",
			)
			return
		}

		FinishAuthFlow(s, v, w, *user)
	}
}

const (
	FailDataFullNameSize           = "full_name must be between 2 and 128 characters"
	FailDataFullNameForbiddenChars = "full_name must be a valid 'passport-style' given name. It must start with a letter and can only contain letters, spaces, apostrophes, or hyphens"
)

func FinishAuthFlow(s Store, v Vault, w http.ResponseWriter, user User) {
	userID, roles, err := s.GetUserByProvider(user.Provider, user.ProviderUserID)

	if errors.Is(err, ErrUserNotFound) {
		userID, ulidErr := NewUserULID()
		if ulidErr != nil {
			slog.Error(
				"failed to generate user ULID",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		user.UserName, err = GenerateUsername()
		if err != nil {
			slog.Error(
				"failed to generate username",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		user.FullName, err = ValidateName(user.FullName)
		if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
			AuthError(w, AuthInvalidRequest, FailDataFullNameSize)
			return
		}
		if errors.Is(err, ErrTextForbiddenChars) {
			AuthError(w, AuthInvalidRequest, FailDataFullNameForbiddenChars)
		}
		if err != nil {
			slog.Error(
				"failed to validate full_name",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}

		err = s.CreateUser(user)
		if err != nil {
			slog.Error(
				"failed to create user",
				"err", err,
			)
			AuthError(w, AuthInvalidRequest, "internal server error")
			return
		}
		// TODO: indicate next actions according to HAL
		CreateAccessToken(v, w,
			userID,
			user.Provider,
			map[Role]ULID{},
		)
		return
	}
	if errors.Is(err, ErrUserNoRole) {
		// TODO: indicate next actions according to HAL
		CreateAccessToken(v, w,
			userID,
			user.Provider,
			map[Role]ULID{},
		)
		return
	}
	if err != nil {
		slog.Error(
			"failed to get user by provider",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	CreateTokenPair(s, v, w,
		userID,
		user.Provider,
		roles,
	)
}

func CreateAccessToken(
	v Vault,
	w http.ResponseWriter,
	userID ULID,
	provider Provider,
	roles map[Role]ULID,
) {
	accessToken, err := v.CreateAccessToken(userID, provider, roles)
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
		slog.Error(
			"failed to generate JTI ULID",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	err = s.CreateRefreshToken(
		jti,
		userID,
		time.Now().UTC().Add(DefaultRefreshTokenExpiration),
	)
	if err != nil {
		slog.Error(
			"failed to create refresh token",
			"err", err,
		)
		AuthError(w, AuthInvalidRequest, "internal server error")
		return
	}

	tokenPair, err := v.CreateTokenPair(userID, provider, jti, roles)
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

func WriteJSON[T any](w http.ResponseWriter, data T, status int) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error(
			"failed to encode response data",
			"err", err,
		)
	}
}

func SetDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func HALSuccess(w http.ResponseWriter, res Resource, status int) {
	SetDefaultHeaders(w)
	res.Props["status"] = ResponseStatusSuccess
	WriteJSON(w, res, status)
}

type SuccessResponse[T any] struct {
	Status ResponseStatus `json:"status"`
	Data   T              `json:"data,omitempty"`
}

func Success[T any](w http.ResponseWriter, data T, status int) {
	SetDefaultHeaders(w)
	WriteJSON(w, SuccessResponse[T]{
		Status: ResponseStatusSuccess,
		Data:   data,
	}, status)
}

func EmptySuccess(w http.ResponseWriter, status int) {
	SetDefaultHeaders(w)
	WriteJSON(w, map[string]string{"status": ResponseStatusSuccess}, status)
}

type ErrorResponse struct {
	Status  ResponseStatus `json:"status"`
	Message string         `json:"message"`
	Code    ErrorCode      `json:"code,omitempty"`
}

func Error(w http.ResponseWriter, message string, status int) {
	SetDefaultHeaders(w)
	WriteJSON(w, ErrorResponse{
		Status:  ResponseStatusError,
		Message: message,
	}, status)
}

type FailResponse struct {
	Status ResponseStatus `json:"status"`
	Data   FailData       `json:"data,omitempty"`
}

func Fail(w http.ResponseWriter, data FailData, status int) {
	SetDefaultHeaders(w)
	WriteJSON(w, FailResponse{
		Status: ResponseStatusFail,
		Data:   data,
	}, status)
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

var (
	// adjectives is an array for creating random usernames, used in conjunction with nouns
	adjectives = [...]string{
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

	// nouns is an array for creating random usernames, used in conjunction with adjectives
	nouns = [...]string{
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

	suffix := make([]byte, 4)
	_, err := rand.Read(suffix)
	if err != nil {
		return "", err
	}

	userName := fmt.Sprintf("%s_%s%s", adj, noun, hex.EncodeToString(suffix))
	userName = strings.ToLower(userName)

	return userName, nil
}

func Health(w http.ResponseWriter, r *http.Request) {
	EmptySuccess(w, http.StatusOK)
}

func HandlerGetMeRecommendations(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
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
			slog.Error("failed to determine user's role")
			Error(w, "internal server error", http.StatusInternalServerError)
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
				slog.Error(
					"failed to fetch position recommendations",
					"err", err,
				)
				Error(w, "internal server error", http.StatusInternalServerError)
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
								RouteV1MeRecommendations,
								rec.RecommendationID),
						},
						RelType("reaction"): Link{
							Href: fmt.Sprintf(
								"%s/%s/reaction",
								RouteV1MeRecommendations,
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
				slog.Error(
					"failed to fetch candidate recommendations",
					"err", err,
				)
				Error(w, "internal server error", http.StatusInternalServerError)
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
								RouteV1MeRecommendations,
								rec.RecommendationID,
							),
						},
						RelType("reaction"): Link{
							Href: fmt.Sprintf(
								"%s/%s/reaction",
								RouteV1MeRecommendations,
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

		selfHref := RouteV1MeRecommendations
		if excludeReacted {
			selfHref += "?exclude_reacted=true"
		}
		links := Links{
			RelTypeSelf:          Link{Href: string(selfHref)},
			RelType("reactions"): Link{Href: string(RouteV1MeReactions)},
		}
		if page.HasNext {
			nextHref := fmt.Sprintf(
				"%s?pos_cursor=%s&can_cursor=%s&limit=%d",
				RouteV1MeRecommendations, posNextCursor, canNextCursor, page.Limit,
			)
			if excludeReacted {
				nextHref += "&exclude_reacted=true"
			}
			links[RelTypeNext] = Link{Href: nextHref}
		}

		HALSuccess(w, Resource{
			Links:    links,
			Embedded: embedded,
			Props:    Props{"page": page},
		}, http.StatusOK)
	}
}

func HandlerCreateMeReaction(s Store) http.HandlerFunc {
	type RequestBody struct {
		ReactionType ReactionType `json:"reaction_type"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		recommendationID := ULID(r.PathValue("id"))
		if recommendationID == "" {
			Fail(w, FailData{"id": "recommendation id is required"}, http.StatusBadRequest)
			return
		}

		rec, err := s.GetRecommendation(recommendationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				Fail(w, FailData{"id": "recommendation not found"}, http.StatusNotFound)
				return
			}
			slog.Error(
				"failed to fetch recommendation",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if rec.CandidateID != candidateID {
			Fail(w, FailData{"reaction": "reaction forbidden"}, http.StatusForbidden)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			Fail(w, FailData{"body": "invalid request body"}, http.StatusBadRequest)
			return
		}
		if !body.ReactionType.IsValid() {
			Fail(w, FailData{"reaction_type": "must be one of: positive, negative, neutral"}, http.StatusBadRequest)
			return
		}

		if err := s.CreateReaction(Reaction{
			RecommendationID: recommendationID,
			ReactorType:      ReactorTypeCandidate,
			ReactorID:        candidateID,
			ReactionType:     body.ReactionType,
		}); err != nil {
			if errors.Is(err, ErrReactionAlreadyExists) {
				Fail(w, FailData{"id": "reaction already exists; reactions are immutable"}, http.StatusConflict)
				return
			}
			slog.Error(
				"failed to record reaction",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		HALSuccess(w, Resource{
			Links: Links{
				RelTypeSelf: Link{
					Href: fmt.Sprintf(
						"%s/%s/reaction",
						RouteV1MeRecommendations,
						recommendationID,
					),
				},
				RelTypeUp:            Link{Href: string(RouteV1MeRecommendations)},
				RelType("reactions"): Link{Href: string(RouteV1MeReactions)},
				RelType("matches"):   Link{Href: string(RouteV1MeMatches)},
			},
		}, http.StatusCreated)
	}
}

func HandlerGetMeReactions(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		page := GetPagination(r)

		reactions, nextCursor, err := s.GetReactionsByCandidateID(
			candidateID,
			page,
		)
		if err != nil {
			slog.Error(
				"failed to fetch candidate",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		page.Count = len(reactions)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: string(RouteV1MeReactions)},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{
				Href: fmt.Sprintf(
					"%s?cursor=%s",
					RouteV1MeReactions,
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
							RouteV1MeRecommendations,
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

		HALSuccess(w, Resource{
			Links:    links,
			Embedded: Embedded{"reactions": embedded},
			Props:    Props{"page": page},
		}, http.StatusOK)
	}
}

func HandlerGetMeMatches(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		candidateID, ok := claims.Roles[RoleCandidate]
		if !ok {
			slog.Error("failed to access candidate's ID within claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		page := GetPagination(r)

		matches, nextCursor, err := s.GetMatchesByCandidateID(candidateID, page)
		if err != nil {
			slog.Error(
				"failed to fetch matches",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		page.Count = len(matches)
		page.HasNext = nextCursor != ""

		links := Links{
			RelTypeSelf: Link{Href: string(RouteV1MeMatches)},
		}
		if nextCursor != "" {
			links[RelTypeNext] = Link{
				Href: fmt.Sprintf(
					"%s?cursor=%s&limit=%d",
					RouteV1MeMatches,
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
					"created_at":  m.CreatedAt,
				},
			}
		}

		HALSuccess(w, Resource{
			Links:    links,
			Embedded: Embedded{"matches": embedded},
			Props:    Props{"page": page},
		}, http.StatusOK)
	}
}

var nameRegex = regexp.MustCompile(`^[\pL][\pL\s'’-]{2,128}\z`)

func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ")
	if len(name) < 2 {
		return "", ErrTextTooShort
	}
	if len(name) > 128 {
		return "", ErrTextTooLong
	}
	if !nameRegex.MatchString(name) {
		return "", ErrTextForbiddenChars
	}
	return name, nil
}

func HandlerCreateUser(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			FullName string `json:"full_name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Fail(w, FailData{"body": "invalid request body"}, http.StatusBadRequest)
			return
		}
		if req.Email == "" {
			Fail(w, FailData{"email": "email is required"}, http.StatusBadRequest)
			return
		}

		email, err := mail.ParseAddress(req.Email)
		if err != nil {
			Fail(w, FailData{"email": "invalid email"}, http.StatusBadRequest)
			return
		}

		fullName, err := ValidateName(req.FullName)
		if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
			Fail(w, FailData{"full_name": FailDataFullNameSize}, http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrTextForbiddenChars) {
			Fail(w, FailData{"full_name": FailDataFullNameForbiddenChars}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error(
				"failed to validate name",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		exists, err := s.UserExistsByEmail(email.Address, ProviderEmail)
		if exists {
			Fail(w, FailData{"email": "user already exists"}, http.StatusConflict)
			return
		}
		if !exists && err != nil {
			slog.Error(
				"failed to check user existance",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		userName, err := GenerateUsername()
		if err != nil {
			slog.Error(
				"failed to generate a username",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		ulid, err := NewUserULID()
		if err != nil {
			slog.Error(
				"failed to generate a user ULID",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		passwordHash, err := v.HashPassword(req.Password)
		if err != nil {
			slog.Error(
				"failed to hash password",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		user := User{
			ID:             ulid,
			Provider:       ProviderEmail,
			ProviderUserID: "",
			Email:          email.Address,
			FullName:       fullName,
			UserName:       userName,
			PasswordHash:   passwordHash,
		}
		err = s.CreateUser(user)
		if err != nil {
			slog.Error(
				"failed to create user",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: indicate next actions according to HAL
		CreateAccessToken(v, w,
			user.ID,
			user.Provider,
			map[Role]ULID{},
		)
	}
}

func HandlerCreateRecruiter(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ulid, err := NewRecruiterULID()
		if err != nil {
			slog.Error(
				"failed to generate a recruiter ULID",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		err = s.CreateRecruiter(Recruiter{ulid, claims.UserID})
		if errors.Is(err, ErrRecruiterAlreadyExists) {
			Fail(w, FailData{"user_id": "recruiter already exists"}, http.StatusConflict)
			return
		}
		if err != nil {
			slog.Error(
				"failed to create recruiter",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		roles, err := s.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error(
				"failed to get user roles",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		_, ok = roles[RoleCandidate]
		if ok {
			CreateAccessToken(v, w,
				claims.UserID,
				claims.Provider,
				roles,
			)
			return
		}

		CreateTokenPair(s, v, w,
			claims.UserID,
			claims.Provider,
			roles,
		)
	}
}

func ValidateAbout(about string) (string, error) {
	about = strings.TrimSpace(about)
	reTags := regexp.MustCompile(`<[^>]*>`)
	about = reTags.ReplaceAllString(about, "")
	if len(about) > 1024 {
		return "", ErrTextTooLong
	}
	return html.EscapeString(about), nil
}

func HandlerCreateCandidate(s Store, v Vault) http.HandlerFunc {
	type RequestBody struct {
		About string `json:"about"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			Fail(w, FailData{"body": "invalid request body"}, http.StatusBadRequest)
			return
		}

		about, err := ValidateAbout(req.About)
		if errors.Is(err, ErrTextTooLong) {
			Fail(w, FailData{"about": "about must be up to 1024 characters"}, http.StatusBadRequest)
			return
		}
		if err != nil {
			slog.Error(
				"failed to create candidate",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		ulid, err := NewCandidateULID()
		if err != nil {
			slog.Error(
				"failed to generate a candidate ULID",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		err = s.CreateCandidate(Candidate{
			ID:     ulid,
			UserID: claims.UserID,
			About:  about,
		})
		if errors.Is(err, ErrCandidateAlreadyExists) {
			Fail(w, FailData{"user_id": "candidate already exists"}, http.StatusConflict)
			return
		}
		if err != nil {
			slog.Error(
				"failed to create candidate",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		roles, err := s.GetUserRoles(claims.UserID, claims.Provider)
		if err != nil {
			slog.Error(
				"failed to get user roles",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if _, ok = roles[RoleRecruiter]; ok {
			CreateAccessToken(v, w,
				claims.UserID,
				claims.Provider,
				roles,
			)
			return
		}

		CreateTokenPair(s, v, w,
			claims.UserID,
			claims.Provider,
			roles,
		)
	}
}

func HandlerGetUsersMe(s Store, v Vault) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		user, err := s.GetUser(claims.UserID)
		if errors.Is(err, ErrUserNotFound) {
			Fail(w, FailData{"user": "user not found"}, http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error(
				"failed to get user",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: Implement HAL for this success
		Success(w, map[string]User{
			"user": *user,
		}, http.StatusOK)
	}
}

var userNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func ValidateUserName(userName string) (string, error) {
	userName = strings.TrimSpace(userName)
	if len(userName) < 4 {
		return "", ErrTextTooShort
	}
	if len(userName) > 32 {
		return "", ErrTextTooLong
	}
	if !userNameRegex.MatchString(userName) {
		return "", ErrTextForbiddenChars
	}
	return userName, nil
}

const (
	FailDataUserNameSize           = "user_name must be between 4 and 32 characters"
	FailDataUserNameForbiddenChars = "user_name can only contain underscores, latin characters and numbers"
)

func HandlerPatchUsersMe(s Store, v Vault) http.HandlerFunc {
	type RequestBody struct {
		UserName *string `json:"user_name,omitempty"`
		FullName *string `json:"full_name,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r)
		if !ok {
			slog.Error("failed to access claims")
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		body, err := DecodeRequestBody[RequestBody](r)
		if err != nil {
			slog.Debug("err", err)
			Fail(w, FailData{"body": "invalid request body"}, http.StatusBadRequest)
			return
		}

		user, err := s.GetUser(claims.UserID)
		if errors.Is(err, ErrUserNotFound) {
			Fail(w, FailData{"user": "user not found"}, http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error(
				"failed to get user",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		changed := false
		if body.FullName != nil {
			validated, err := ValidateName(*body.FullName)
			if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
				Fail(w, FailData{"full_name": FailDataFullNameSize}, http.StatusBadRequest)
				return
			}
			if errors.Is(err, ErrTextForbiddenChars) {
				Fail(w, FailData{"full_name": FailDataFullNameForbiddenChars}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error(
					"failed to validate full_name",
					"err", err,
				)
				Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if user.FullName != validated {
				user.FullName = validated
				changed = true
			}
		}

		if body.UserName != nil {
			validated, err := ValidateUserName(*body.UserName)
			if errors.Is(err, ErrTextTooShort) || errors.Is(err, ErrTextTooLong) {
				Fail(w, FailData{"user_name": FailDataUserNameSize}, http.StatusBadRequest)
				return
			}
			if errors.Is(err, ErrTextForbiddenChars) {
				Fail(w, FailData{"user_name": FailDataUserNameForbiddenChars}, http.StatusBadRequest)
				return
			}
			if err != nil {
				slog.Error(
					"failed to validate user_name",
					"err", err,
				)
				Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if user.UserName != validated {
				user.UserName = validated
				changed = true
			}
		}

		if !changed {
			// TODO: Implement HAL for this success
			Success(w, map[string]User{
				"user": *user,
			}, http.StatusOK)
			return
		}

		updatedUser, err := s.UpdateUser(*user)
		if errors.Is(err, ErrUserNotFound) {
			Fail(w, FailData{"user": "user not found"}, http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error(
				"failed to update user",
				"err", err,
			)
			Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: Impelment HAL for this success
		Success(w, map[string]User{
			"user": *updatedUser,
		}, http.StatusOK)
	}
}
