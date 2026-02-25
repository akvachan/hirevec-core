// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"fmt"
	"net/http"
	"time"
)

type Method string

const (
	RouteHealth            = "/v1/health"
	RoutePublicKeys        = "/v1/auth/keys"
	RouteToken             = "/v1/auth/token"
	RouteLogin             = "/v1/auth/login/{provider}"
	RouteCallback          = "/v1/auth/callback/{provider}"
	RoutePositions         = "/v1/positions"
	RoutePosition          = "/v1/positions/{id}"
	RouteCandidates        = "/v1/candidates"
	RouteCandidate         = "/v1/candidates/{id}"
	MethodGet       Method = http.MethodGet
	MethodPost      Method = http.MethodPost
)

func PublicRoute(m *http.ServeMux, method Method, route string, handler http.HandlerFunc, mdw ...Middleware) {
	routeWithMethod := fmt.Sprintf("%s %s", method, route)
	rlcfg := NewRateLimiterConfig(60, time.Minute)
	basic := Chain(
		handler,
		Logger,
		ErrorHandler,
		RateLimiter(rlcfg),
		MaxBytesLimiter,
	)

	m.Handle(routeWithMethod, Chain(basic, mdw...))
}

func ProtectedRoute(mux *http.ServeMux, method Method, route string, handler http.HandlerFunc, mdw ...Middleware) {
	routeWithMethod := fmt.Sprintf("%s %s", method, route)
	rlcfg := NewRateLimiterConfig(120, time.Minute)
	basic := Chain(
		handler,
		Logger,
		ErrorHandler,
		RateLimiter(rlcfg),
		MaxBytesLimiter,
	)

	mux.Handle(routeWithMethod, Chain(basic, mdw...))
}

func GetRootMux(s Store, v Vault) http.Handler {
	mux := http.NewServeMux()
	pcfg := NewPaginatorConfig(PageSizeDefaultLimit, PageSizeMaxLimit)

	PublicRoute(mux, MethodGet, RouteHealth, Health)
	PublicRoute(mux, MethodGet, RoutePublicKeys, PublicKeys(v))
	PublicRoute(mux, MethodPost, RouteToken, CreateAccessToken(s, v))
	PublicRoute(mux, MethodGet, RouteLogin, Login(v))
	PublicRoute(mux, MethodPost, RouteLogin, Login(v))
	PublicRoute(mux, MethodGet, RouteCallback, RedirectProvider(s, v))
	PublicRoute(mux, MethodPost, RouteCallback, RedirectProvider(s, v))

	ProtectedRoute(mux, MethodGet, RoutePosition, GetPosition(s))
	ProtectedRoute(mux, MethodGet, RoutePositions, GetPositions(s), Paginator(pcfg))
	ProtectedRoute(mux, MethodGet, RouteCandidate, GetCandidate(s))
	ProtectedRoute(mux, MethodGet, RouteCandidates, GetCandidates(s), Paginator(pcfg))

	return mux
}
