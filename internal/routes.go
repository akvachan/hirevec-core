// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"fmt"
	"net/http"
)

type Method string

const (
	MethodGet    Method = http.MethodGet
	MethodPost   Method = http.MethodPost
	MethodPut    Method = http.MethodPut
	MethodPatch  Method = http.MethodPatch
	MethodDelete Method = http.MethodDelete
)

type Href string

type Route struct {
	Method Method
	Href   Href
}

func (r Route) String() string {
	return fmt.Sprintf("%s %s", r.Method, r.Href)
}

var (
	RouteHealth            = Route{MethodGet, Href("/v1/health")}
	RoutePublicKeys        = Route{MethodGet, Href("/v1/auth/keys")}
	RouteCreateAccessToken = Route{MethodPost, Href("/v1/auth/token")}
	RouteGetLogin          = Route{MethodGet, Href("/v1/auth/login/{provider}")}
	RouteCreateLogin       = Route{MethodPost, Href("/v1/auth/login/{provider}")}
	RouteGetCallback       = Route{MethodGet, Href("/v1/auth/callback/{provider}")}
	RouteCreateCallback    = Route{MethodPost, Href("/v1/auth/callback/{provider}")}

	// DEPRECATED:
	RouteGetPositions     = Route{MethodGet, Href("/v1/positions/{id}")}
	RouteGetCandidates    = Route{MethodGet, Href("/v1/candidates/{id}")}
	RouteCreateCandidates = Route{MethodPost, Href("/v1/candidates")}
)

func GetRootMux(s Store, v Vault) http.Handler {
	mux := http.NewServeMux()

	var (
		health            = Public(Health)
		publicKeys        = Public(GetPublicKeys(v))
		createAccessToken = Public(CreateAccessToken(s, v))
		login             = Public(Login(v))
		callback          = Public(RedirectProvider(s, v))
	)

	mux.Handle(RouteHealth.String(), health)
	mux.Handle(RoutePublicKeys.String(), publicKeys)
	mux.Handle(RouteCreateAccessToken.String(), createAccessToken)
	mux.Handle(RouteGetLogin.String(), login)
	mux.Handle(RouteCreateLogin.String(), login)
	mux.Handle(RouteGetCallback.String(), callback)
	mux.Handle(RouteCreateCallback.String(), callback)

	return mux
}
