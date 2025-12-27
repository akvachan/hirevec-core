// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import (
	"fmt"
	"net/http"
)

var (
	GetPositionRoute     = "/api/v0/positions/{id}"
	GetPositionsRoute    = "/api/v0/positions/"
	GetCandidateRoute    = "/api/v0/candidates/{id}"
	GetCandidatesRoute   = "/api/v0/candidates/"
	GetMatchRoute        = "/api/v0/matches/{id}"
	GetLikeRoute         = "/api/v0/likes/{id}"
	GetDislikeRoute      = "/api/v0/dislikes/{id}"
	GetSwipeRoute        = "/api/v0/swipes/{id}"
	CreatePositionRoute  = "/api/v0/positions/"
	CreateCandidateRoute = "/api/v0/candidates/"
	CreateMatchRoute     = "/api/v0/matches/"
	CreateLikeRoute      = "/api/v0/likes/"
	CreateDislikeRoute   = "/api/v0/dislikes/"
	CreateSwipeRoute     = "/api/v0/swipes/"
)

func Endpoint(method string, route string) string {
	return fmt.Sprintf("%v %v", method, route)
}

func RegisterRoutes() *http.ServeMux {
	router := http.NewServeMux()

	router.HandleFunc(
		Endpoint(http.MethodGet, GetPositionRoute),
		GetPositionHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetPositionsRoute),
		GetPositionsHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetCandidateRoute),
		GetCandidateHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetCandidatesRoute),
		GetCandidatesHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetMatchRoute),
		GetMatchHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetLikeRoute),
		GetLikeHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetDislikeRoute),
		GetDislikeHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodGet, GetSwipeRoute),
		GetSwipeHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreatePositionRoute),
		CreatePositionHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreateCandidateRoute),
		CreateCandidateHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreateMatchRoute),
		CreateMatchHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreateLikeRoute),
		CreateLikeHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreateDislikeRoute),
		CreateDislikeHandler,
	)

	router.HandleFunc(
		Endpoint(http.MethodPost, CreateSwipeRoute),
		CreateSwipeHandler,
	)

	return router
}
