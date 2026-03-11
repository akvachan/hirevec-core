// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"fmt"
	"net/http"
	"time"
)

type (
	Method string

	PublicRouteConfig struct {
		Mux     *http.ServeMux
		Method  Method
		Route   string
		Handler http.HandlerFunc
	}

	ProtectedRouteConfig struct {
		Mux            *http.ServeMux
		Method         Method
		Route          string
		Handler        http.HandlerFunc
		RequiredScopes []ScopeValueType
	}
)

const (
	RouteHealth          = "/v1/health"
	RoutePublicKeys      = "/v1/auth/keys"
	RouteToken           = "/v1/auth/token"
	RouteLogin           = "/v1/auth/login/{provider}"
	RouteCallback        = "/v1/auth/callback/{provider}"
	RoutePositions       = "/v1/positions"
	RoutePosition        = "/v1/positions/{id}"
	RouteCandidates      = "/v1/candidates"
	RouteCandidate       = "/v1/candidates/{id}"
	RouteRecommendations = "/v1/me/recommendations"
	// RouteMatches                = "/v1/me/matches"
	// RouteReactions              = "/v1/me/reactions"
	// RouteProfile                = "/v1/me/profile"
	// RouteStats                  = "/v1/me/stats"
	MethodGet  Method = http.MethodGet
	MethodPost Method = http.MethodPost
)

func PublicRoute(s Store, v Vault, cfg PublicRouteConfig) {
	routeWithMethod := fmt.Sprintf("%s %s", cfg.Method, cfg.Route)
	rlcfg := NewRateLimiterConfig(60, time.Minute)
	basic := Chain(
		cfg.Handler,
		Logger,
		ErrorHandler,
		RateLimiter(rlcfg),
		MaxBytesLimiter,
	)

	cfg.Mux.Handle(routeWithMethod, basic)
}

func ProtectedRoute(s Store, v Vault, cfg ProtectedRouteConfig) {
	routeWithMethod := fmt.Sprintf("%s %s", cfg.Method, cfg.Route)
	rlcfg := NewRateLimiterConfig(120, time.Minute)
	basic := Chain(
		cfg.Handler,
		Logger,
		ErrorHandler,
		RateLimiter(rlcfg),
		MaxBytesLimiter,
		Authentication(v, cfg.RequiredScopes),
	)

	cfg.Mux.Handle(routeWithMethod, basic)
}

func GetRootMux(s Store, v Vault) http.Handler {
	mux := http.NewServeMux()
	pcfg := NewPaginatorConfig(DefaultPageSizeLimit, PageSizeMaxLimit)

	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodGet,
			RouteHealth,
			Health,
		},
	)
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodGet,
			RoutePublicKeys,
			PublicKeys(v),
		},
	)
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodPost,
			RouteToken,
			CreateAccessToken(s, v),
		},
	)
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodGet,
			RouteLogin,
			Login(v),
		},
	)
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodPost,
			RouteLogin,
			Login(v),
		})
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodGet,
			RouteCallback,
			RedirectProvider(s, v),
		})
	PublicRoute(
		s, v,
		PublicRouteConfig{
			mux,
			MethodPost,
			RouteCallback,
			RedirectProvider(s, v),
		},
	)

	ProtectedRoute(
		s, v,
		ProtectedRouteConfig{
			mux,
			MethodGet,
			RoutePosition,
			GetPosition(s),
			[]ScopeValueType{ScopeValueTypeAdmin},
		},
	)
	ProtectedRoute(
		s, v,
		ProtectedRouteConfig{
			mux,
			MethodGet,
			RoutePositions,
			Chain(
				GetPositions(s),
				Paginator(pcfg),
			),
			[]ScopeValueType{ScopeValueTypeAdmin},
		},
	)
	ProtectedRoute(
		s, v,
		ProtectedRouteConfig{
			mux,
			MethodGet,
			RouteCandidate,
			GetCandidate(s),
			[]ScopeValueType{ScopeValueTypeAdmin},
		},
	)
	ProtectedRoute(
		s, v,
		ProtectedRouteConfig{
			mux,
			MethodGet,
			RouteCandidates,
			Chain(
				GetCandidates(s),
				Paginator(pcfg),
			),
			[]ScopeValueType{ScopeValueTypeAdmin},
		},
	)
	ProtectedRoute(
		s, v,
		ProtectedRouteConfig{
			mux,
			MethodGet,
			RouteRecommendations,
			Chain(
				GetRecommendations(s),
				Paginator(pcfg),
			),
			[]ScopeValueType{ScopeValueTypeAdmin, ScopeValueTypeCandidate, ScopeValueTypeRecruiter},
		},
	)

	return mux
}
