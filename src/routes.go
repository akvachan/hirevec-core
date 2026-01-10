// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import (
	"fmt"
	"net/http"
)

var (
	PostionRoute    = "/api/v0/positions/{id}"
	PositionsRoute  = "/api/v0/positions/"
	CandidateRoute  = "/api/v0/candidates/{id}"
	CandidatesRoute = "/api/v0/candidates/"
)

func RegisterRoute(
	router *http.ServeMux,
	method string,
	route string,
	handler func(http.ResponseWriter, *http.Request),
) {
	router.HandleFunc(fmt.Sprintf("%v %v", method, route), handler)
}

func RegisterRoutes() *http.ServeMux {
	router := http.NewServeMux()

	RegisterRoute(router, http.MethodGet, PositionsRoute, GetPositionsHandler)
	RegisterRoute(router, http.MethodGet, PostionRoute, GetPositionHandler)
	RegisterRoute(router, http.MethodGet, CandidatesRoute, GetCandidatesHandler)
	RegisterRoute(router, http.MethodGet, CandidateRoute, GetCandidateHandler)

	return router
}
