// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ParamLocation string

const (
	ParamLocationQuery ParamLocation = "query"
	ParamLocationPath  ParamLocation = "path"
)

type ParamType string

const (
	ParamTypeBoolean ParamType = "boolean"
	ParamTypeInteger ParamType = "integer"
	ParamTypeFloat   ParamType = "float"
	ParamTypeString  ParamType = "string"
	ParamTypeArray   ParamType = "array"
)

type ParamFormat string

const (
	ParamFormatUUID     ParamFormat = "uuid"
	ParamFormatDateTime ParamFormat = "date-time"
	ParamFormatDate     ParamFormat = "date"
	ParamFormatEmail    ParamFormat = "email"
	ParamFormatURI      ParamFormat = "uri"
	ParamFormatPassword ParamFormat = "password"
)

type Param struct {
	Name     string        `json:"name"`
	Location ParamLocation `json:"location"`
	Type     ParamType     `json:"type"`
	Required bool          `json:"required"`
}

// RelType is an enum that implements [RFC5988](https://www.rfc-editor.org/rfc/rfc5988.txt).
type RelType string

const (
	// Conveys an identifier for the link's context.
	RelTypeSelf RelType = "self"

	// Refers to a parent document in a hierarchy of documents.
	RelTypeUp RelType = "up"

	// Refers to the previous resource in an ordered series of resources.
	RelTypePrevious RelType = "previous"

	// Refers to the next resource in a ordered series of resources.
	RelTypeNext RelType = "next"

	// An IRI that refers to the furthest preceding resource in a series of resources.
	RelTypeFirst RelType = "first"

	// An IRI that refers to the furthest following resource in a series of resources.
	RelTypeLast RelType = "last"

	// Refers to an index.
	RelTypeIndex RelType = "index"

	// Refers to a resource offering help (more information, links to other sources information, etc.).
	RelTypeHelp RelType = "help"

	// Refers to a resource that can be used to edit the link's context.
	RelTypeEdit RelType = "edit"

	RelTypeRecommendation RelType = "/rels/recommendations"
)

type Link struct {
	Rel    RelType `json:"rel"`
	Name   string  `json:"name"`
	Method Method  `json:"method"`
	Href   Href    `json:"href"`
	Params []Param `json:"params,omitempty"`
	Docs   string  `json:"docs,omitempty"`
}

type ResponseStatus string

const (
	ResponseStatusSuccess = "success"
	ResponseStatusError   = "error"
	ResponseStatusFail    = "fail"
)

type SuccessResponse struct {
	Status ResponseStatus `json:"status"`
	Data   any            `json:"data"`
	Links  []Link         `json:"_links,omitempty"`
}

type ErrorResponse struct {
	Status  ResponseStatus `json:"status"`
	Message string         `json:"message"`
}

type FailResponse struct {
	Status ResponseStatus `json:"status"`
	Data   any            `json:"data"`
	Links  []Link         `json:"_links,omitempty"`
}

type AuthErrorCode string

const (
	AuthInvalidRequest       AuthErrorCode = "invalid_request"
	AuthInvalidGrant         AuthErrorCode = "invalid_grant"
	AuthInvalidClient        AuthErrorCode = "invalid_client"
	AuthUnsupportedGrantType AuthErrorCode = "unsupported_grant_type"
)

type AuthErrorResponse struct {
	Error            AuthErrorCode `json:"error"`
	ErrorDescription string        `json:"error_description,omitempty"`
	ErrorURI         string        `json:"error_uri,omitempty"`
	Links            []Link        `json:"_links,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("could not encode response data", "err", err)
	}
}

func SetDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func SetAuthHeaders(w http.ResponseWriter) {
	SetDefaultHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func Success(w http.ResponseWriter, status int, data any, links ...Link) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, SuccessResponse{ResponseStatusSuccess, data, links})
}

func Error(w http.ResponseWriter, status int, message string) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, ErrorResponse{ResponseStatusError, message})
}

func Fail(w http.ResponseWriter, status int, data any, links ...Link) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, FailResponse{ResponseStatusFail, data, links})
}

func AuthAccessToken(w http.ResponseWriter, accessToken AccessToken, links ...Link) {
	SetAuthHeaders(w)

	data := struct {
		AccessToken
		Links []Link `json:"_links,omitempty"`
	}{
		accessToken,
		links,
	}
	WriteJSON(w, http.StatusOK, data)
}

func AuthTokenPair(w http.ResponseWriter, tokenPair TokenPair, links ...Link) {
	SetAuthHeaders(w)

	data := struct {
		TokenPair
		Links []Link `json:"_links,omitempty"`
	}{
		tokenPair,
		links,
	}
	WriteJSON(w, http.StatusOK, data)
}

func AuthError(w http.ResponseWriter, code AuthErrorCode, description string, links ...Link) {
	SetAuthHeaders(w)
	WriteJSON(w, http.StatusBadRequest, AuthErrorResponse{Error: code, ErrorDescription: description, Links: links})
}

func Unauthorized(w http.ResponseWriter, code AuthErrorCode, description string, links ...Link) {
	SetAuthHeaders(w)
	w.Header().Set("WWW-Authenticate", "Bearer")
	WriteJSON(w, http.StatusUnauthorized, AuthErrorResponse{Error: code, ErrorDescription: description, Links: links})
}
