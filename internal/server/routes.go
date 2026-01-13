// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

// Package server implements basic routing, middleware, handlers and validation
package server

import (
	"fmt"
	"net/http"
)

var HirevecServer *http.Server

var (
	routePosition           = "/api/v0/positions/{id}"
	routePositions          = "/api/v0/positions/"
	routeCandidate          = "/api/v0/candidates/{id}"
	routeCandidates         = "/api/v0/candidates/"
	routeCandidatesReaction = "/api/v0/candidates/{id}/reactions"
	routeRecruitersReaction = "/api/v0/recruiters/{id}/reactions"
	routeMatches            = "/api/v0/matches/"
)

func registerRoute(
	router *http.ServeMux,
	method string,
	route string,
	handler func(http.ResponseWriter, *http.Request),
) {
	router.HandleFunc(fmt.Sprintf("%v %v", method, route), handler)
}

func registerRoutes() *http.ServeMux {
	r := http.NewServeMux()

	registerRoute(r, http.MethodGet, routePositions, handleGetPositions)
	registerRoute(r, http.MethodGet, routePosition, handleGetPosition)
	registerRoute(r, http.MethodGet, routeCandidates, handleGetCandidates)
	registerRoute(r, http.MethodGet, routeCandidate, handleGetCandidate)
	registerRoute(r, http.MethodPost, routeCandidatesReaction, handlePostCandidateReaction)
	registerRoute(r, http.MethodPost, routeRecruitersReaction, handlePostRecruiterReaction)
	registerRoute(r, http.MethodPost, routeMatches, handlePostMatch)

	return r
}
