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

	"github.com/akvachan/hirevec-backend/internal/store"
	"github.com/akvachan/hirevec-backend/internal/vault"
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

var allowedAPIVersions = []apiVersion{
	Unversioned,
	V1,
}

// Route defines the configuration for a single API endpoint.
type Route struct {
	// An HTTP Method (GET, POST, PATCH, PUT).
	Methods []string

	// An HTTP Path without leading or trailing slashes.
	Path string

	// API version the route belongs to.
	Version apiVersion

	// Handler function.
	Handler http.HandlerFunc

	// A slice of Middleware.
	Middleware []Middleware

	// A short explanation of the endpoint for documentation purposes.
	Description string
}

// NewRouter initializes a new router with a specific prefix and mounts it onto the provided root mux.
func NewRouter(prefix string) *router {
	prefix = strings.Trim(prefix, "/")
	r := &router{
		mux:    http.NewServeMux(),
		prefix: prefix,
	}
	http.DefaultServeMux.Handle("/"+prefix+"/", r.mux)
	return r
}

// AddRoutes registers multiple route definitions with the router.
//
// It constructs the final URL pattern following the format: METHOD /prefix/vN/path or /prefix/path if Unversioned is provided.
func (r *router) AddRoutes(routes ...Route) error {
	for _, route := range routes {
		path := strings.Trim(route.Path, "/")

		for _, method := range route.Methods {
			if route.Handler == nil {
				return ErrHandlerRequired(path)
			}
			if route.Description == "" {
				return ErrDescriptionRequired(path)
			}
			if !slices.Contains(allowedMethods, method) {
				return ErrMethodNotAllowed(method, path)
			}
			if !route.Version.IsValid() {
				return ErrInvalidAPIVersion(path, route.Version)
			}

			var pattern string
			if route.Version == Unversioned {
				pattern = fmt.Sprintf("%s /%s/%s", method, r.prefix, path)
			} else {
				pattern = fmt.Sprintf("%s /%s/v%d/%s", method, r.prefix, route.Version, path)
			}
			handler := Chain(route.Handler, route.Middleware...)
			r.mux.Handle(pattern, handler)
		}
	}
	return nil
}

// AssembleTree assembles the complete application routing tree.
func AssembleTree(localStore store.Store, localVault vault.Vault) error {
	apiRouter := NewRouter("api")

	// A standard rate limiter for all endpoints.
	standardRateLimiter := NewRateLimiter(60, time.Minute)

	if err := apiRouter.AddRoutes(
		Route{
			Methods:     []string{http.MethodGet},
			Path:        "auth/keys",
			Version:     V1,
			Handler:     GetPublicKeys(localVault),
			Middleware:  PublicMiddleware(standardRateLimiter),
			Description: "Get server public keys",
		},
		Route{
			Methods:     []string{http.MethodPost},
			Path:        "auth/token",
			Version:     V1,
			Handler:     CreateAccessToken(localStore, localVault),
			Middleware:  PublicMiddleware(standardRateLimiter),
			Description: "Get new access token",
		},
		Route{
			Methods:     []string{http.MethodGet, http.MethodPost},
			Path:        "auth/login/{provider}",
			Version:     V1,
			Handler:     Login(localVault),
			Middleware:  PublicMiddleware(standardRateLimiter),
			Description: "Authorize via provider",
		},
		Route{
			Methods:     []string{http.MethodGet, http.MethodPost},
			Path:        "auth/callback/{provider}",
			Version:     V1,
			Handler:     RedirectProvider(localStore, localVault),
			Middleware:  PublicMiddleware(standardRateLimiter),
			Description: "Internal OAuth callback",
		},
		Route{
			Methods:     []string{http.MethodPost},
			Path:        "/me/reactions",
			Version:     V1,
			Handler:     CreateCandidate(localStore, localVault),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "Create a new candidate",
		},
		Route{
			Methods:     []string{http.MethodPost},
			Path:        "/me/matches",
			Version:     V1,
			Handler:     CreateCandidate(localStore, localVault),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "Create a new candidate",
		},
		Route{
			Methods:     []string{http.MethodPost},
			Path:        "candidates",
			Version:     V1,
			Handler:     CreateCandidate(localStore, localVault),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "Create a new candidate",
		},
		// Route{
		// 	Methods:     []string{http.MethodPost},
		// 	Path:        "recruiters",
		// 	Version:     V1,
		// 	Handler:     CreateRecruiter(localStore),
		// 	Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
		// 	Description: "Create a new recruiter",
		// },
		Route{
			Methods:     []string{http.MethodGet},
			Path:        "positions",
			Version:     V1,
			Handler:     GetPositions(localStore),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "List all positions",
		},
		Route{
			Methods:     []string{http.MethodGet},
			Path:        "positions/{id}",
			Version:     V1,
			Handler:     GetPosition(localStore),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "Get position by ID",
		},
		Route{
			Methods:     []string{http.MethodGet},
			Path:        "candidates/{id}",
			Version:     V1,
			Handler:     GetCandidate(localStore),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "Get candidate by ID",
		},
		Route{
			Methods:     []string{http.MethodGet},
			Path:        "candidates",
			Version:     V1,
			Handler:     GetCandidates(localStore),
			Middleware:  ProtectedMiddleware(localVault, standardRateLimiter),
			Description: "List all candidates",
		},
	); err != nil {
		return ErrFailedToAddRoutes(err)
	}

	return nil
}
