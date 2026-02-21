// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"net/http"
)

func AssembleTree(localStore Store, localVault Vault) http.Handler {
	rootMux := http.NewServeMux()

	getPublicKeys := PublicMiddleware(GetPublicKeys(localVault))
	createAccessToken := PublicMiddleware(CreateAccessToken(localStore, localVault))
	login := PublicMiddleware(Login(localVault))
	callback := PublicMiddleware(RedirectProvider(localStore, localVault))
	getCandidates := ProtectedMiddleware(GetCandidates(localStore))
	getCandidate := ProtectedMiddleware(GetCandidate(localStore))
	createCandidate := ProtectedMiddleware(CreateCandidate(localStore, localVault))
	getPositions := ProtectedMiddleware(GetPositions(localStore))
	getPosition := ProtectedMiddleware(GetPosition(localStore))

	rootMux.Handle("GET 	/api/v1/auth/keys", getPublicKeys)
	rootMux.Handle("GET 	/api/v1/auth/token", createAccessToken)
	rootMux.Handle("GET 	/api/v1/auth/login/{provider}", login)
	rootMux.Handle("POST 	/api/v1/auth/login/{provider}", login)
	rootMux.Handle("GET 	/api/v1/auth/callback/{provider}", callback)
	rootMux.Handle("POST 	/api/v1/auth/callback/{provider}", callback)
	rootMux.Handle("GET 	/api/v1/candidates", getCandidates)
	rootMux.Handle("GET 	/api/v1/candidates/{id}", getCandidate)
	rootMux.Handle("POST 	/api/v1/candidates", createCandidate)
	rootMux.Handle("GET 	/api/v1/positions", getPositions)
	rootMux.Handle("GET 	/api/v1/positions/{id}", getPosition)

	return rootMux
}
