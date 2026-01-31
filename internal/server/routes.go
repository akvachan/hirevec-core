// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// router wraps a standard http.ServeMux to provide prefixed and versioned routing.
type router struct {
	mux    *http.ServeMux
	prefix string
}

// apiVersion represents a numerical version of the API.
type apiVersion uint8

const (
	// Unversioned is a sentinel value used to indicate that a route should not have a /vN/ segment in its URL.
	Unversioned apiVersion = 0

	// V1 represents the initial development version of the API.
	V1 apiVersion = 1
)

func (v apiVersion) IsValid() bool {
	switch v {
	case Unversioned:
		return true
	case V1:
		return true
	default:
		return false
	}
}

var allowedMethods = []string{
	http.MethodDelete,
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
}

// Route defines the configuration for a single API endpoint.
type Route struct {
	// An HTTP Method (GET, POST, PATCH, PUT).
	Methods []string

	// An HTTP Path without leading or trailing slashes.
	Path string

	// API version the route belongs to.
	APIVersion apiVersion

	// Handler function.
	Handler http.HandlerFunc

	// A slice of Middleware.
	Middleware []Middleware

	// A short explanation of the endpoint for documentation purposes.
	Description string
}

// NewRouter initializes a new router with a specific prefix and mounts it onto the provided root mux.
func NewRouter(rootMux *http.ServeMux, prefix string) *router {
	if strings.HasPrefix(prefix, "/") {
		panic("prefix cannot have a leading / (slash)")
	}
	if strings.HasSuffix(prefix, "/") {
		panic("prefix cannot have a trailing / (slash)")
	}
	r := &router{
		mux:    http.NewServeMux(),
		prefix: prefix,
	}
	rootMux.Handle("/"+prefix+"/", r.mux)
	return r
}

// AddRoutes registers multiple route definitions with the router.
//
// It constructs the final URL pattern following the format: METHOD /prefix/vN/path or /prefix/path if Unversioned is provided.
func (r *router) AddRoutes(routes ...Route) {
	for _, route := range routes {
		for _, method := range route.Methods {
			if route.Handler == nil {
				panic("handler cannot be nil")
			}
			if strings.HasPrefix(route.Path, "/") {
				panic("path cannot have a leading / (slash)")
			}
			if strings.HasSuffix(route.Path, "/") {
				panic("path cannot have a trailing / (slash)")
			}
			if route.Description == "" {
				panic("description cannot be empty")
			}
			if !slices.Contains(allowedMethods, method) {
				panic(fmt.Sprintf("method %v not allowed, allowed methods:  %v", method, allowedMethods))
			}
			if !route.APIVersion.IsValid() {
				panic("API version %v not allowed, allowed versions: V1, Unversioned")
			}
			var pattern string
			if route.APIVersion == Unversioned {
				pattern = fmt.Sprintf("%s /%s/%s", method, r.prefix, route.Path)
			} else {
				pattern = fmt.Sprintf("%s /%s/v%d/%s", method, r.prefix, route.APIVersion, route.Path)
			}
			handler := Chain(route.Handler, route.Middleware...)
			r.mux.Handle(pattern, handler)
		}
	}
}

// GetRootRouter assembles the complete application routing tree.
func GetRootRouter() *http.ServeMux {
	rootMux := http.NewServeMux()
	apiRouter := NewRouter(rootMux, "api")
	apiRouter.AddRoutes(
		// Route{
		// 	Methods:    []string{http.MethodGet},
		// 	Path:       "health",
		// 	APIVersion: V1,
		// 	Handler:    CheckHealth,
		// 	 append(
		// 		GroupPublic,
		// 		RateLimit(60, time.Minute),
		// 	),
		// 	Description: "Health check endpoint",
		// },

		// OAuth endpoints
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "auth/keys",
			APIVersion: V1,
			Handler:    GetPublicKeys,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Get server public keys",
		},
		Route{
			Methods:    []string{http.MethodPost},
			Path:       "oauth2/token",
			APIVersion: V1,
			Handler:    TokenEndpoint,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Get new access token",
		},
		Route{
			Methods:    []string{http.MethodGet, http.MethodPost},
			Path:       "oauth2/login/{provider}",
			APIVersion: V1,
			Handler:    Login,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Hour),
			),
			Description: "Authorize via provider",
		},
		Route{
			Methods:    []string{http.MethodGet, http.MethodPost},
			Path:       "oauth2/callback/{provider}",
			APIVersion: V1,
			Handler:    RedirectionEndpoint,
			Middleware: append(
				GroupPublic,
				RateLimit(60, time.Minute),
			),
			Description: "Internal OAuth callback",
		},

		// Protected resources
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "positions",
			APIVersion: V1,
			Handler:    GetPositions,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Hour),
			),
			Description: "List all positions",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "positions/{id}",
			APIVersion: V1,
			Handler:    GetPosition,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Minute),
			),
			Description: "Get position by ID",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "candidates/{id}",
			APIVersion: V1,
			Handler:    GetCandidate,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Minute),
			),
			Description: "Get candidate by ID",
		},
		Route{
			Methods:    []string{http.MethodGet},
			Path:       "candidates",
			APIVersion: V1,
			Handler:    GetCandidates,
			Middleware: append(
				GroupProtected,
				RateLimit(60, time.Hour),
			),
			Description: "List all candidates",
		},
	)

	return rootMux
}
