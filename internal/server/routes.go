// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"fmt"
	"net/http"
	"strings"
)

// router wraps a standard http.ServeMux to provide prefixed and versioned routing.
type router struct {
	mux    *http.ServeMux
	prefix string
}

// apiVersion represents a numerical version of the API.
type apiVersion uint8

const (
	// NoVersion is a sentinel value used to indicate that a route should not have a /vN/ segment in its URL.
	NoVersion apiVersion = 255

	// v0 represents the initial development version of the API.
	v0 apiVersion = 0
)

// route defines the configuration for a single API endpoint.
type route struct {
	// An HTTP method (GET, POST, PATCH, PUT).
	method string

	// An HTTP path without leading or trailing slashes.
	path string

	// API version the route belongs to.
	apiVersion apiVersion

	// Handler function.
	handler http.HandlerFunc

	// A slice of middleware.
	middleware []middleware

	// A short explanation of the endpoint for documentation purposes.
	description string
}

// newRouter initializes a new router with a specific prefix and mounts it onto the provided root mux.
func newRouter(rootMux *http.ServeMux, prefix string) *router {
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

// addRoutes registers multiple route definitions with the router.
//
// It constructs the final URL pattern following the format: METHOD /prefix/vN/path.
func (r *router) addRoutes(routes ...route) {
	for _, route := range routes {
		if route.handler == nil {
			panic("handler cannot be nil")
		}
		if strings.HasPrefix(route.path, "/") {
			panic("path cannot have a leading / (slash)")
		}
		if strings.HasSuffix(route.path, "/") {
			panic("path cannot have a trailing / (slash)")
		}
		if route.description == "" {
			panic("description cannot be empty")
		}
		var pattern string
		if route.apiVersion == NoVersion {
			pattern = fmt.Sprintf("%s /%s/%s", route.method, r.prefix, route.path)
		} else {
			pattern = fmt.Sprintf("%s /%s/v%d/%s", route.method, r.prefix, route.apiVersion, route.path)
		}
		handler := chain(route.handler, route.middleware...)
		r.mux.Handle(pattern, handler)
	}
}

// GetRootRouter assembles the complete application routing tree.
func GetRootRouter() *http.ServeMux {
	rootMux := http.NewServeMux()
	apiRouter := newRouter(rootMux, "api")
	apiRouter.addRoutes(
		route{
			method:      http.MethodGet,
			path:        "health",
			apiVersion:  v0,
			handler:     handleHealth,
			middleware:  middlewareGroupPublic,
			description: "Health check endpoint",
		},
		route{
			method:      http.MethodGet,
			path:        "positions",
			apiVersion:  v0,
			handler:     handleGetPositions,
			middleware:  middlewareGroupProtected,
			description: "List all positions",
		},
		route{
			method:      http.MethodGet,
			path:        "positions/{id}",
			apiVersion:  v0,
			handler:     handleGetPosition,
			middleware:  middlewareGroupProtected,
			description: "Get position by ID",
		},
		route{
			method:      http.MethodGet,
			path:        "candidates",
			apiVersion:  v0,
			handler:     handleGetCandidates,
			middleware:  middlewareGroupProtected,
			description: "List all candidates",
		},
		route{
			method:      http.MethodGet,
			path:        "candidates/{id}",
			apiVersion:  v0,
			handler:     handleGetCandidate,
			middleware:  middlewareGroupProtected,
			description: "Get candidate by ID",
		},
	)
	return rootMux
}
