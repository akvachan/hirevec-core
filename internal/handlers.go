// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type (
	// FailData defines [JSend](https://github.com/omniti-labs/jsend) request failure data.
	FailData map[string]string

	// ResponseStatus defines JSend status codes.
	ResponseStatus string

	// ErrorCode defines JSend error codes.
	ErrorCode uint16

	// RelType defines link relation type, see [RFC5988](https://www.rfc-editor.org/rfc/rfc5988.txt).
	RelType string

	// Link defines a [HAL](https://datatracker.ietf.org/doc/html/draft-kelly-json-hal-11) link object.
	Link struct {
		Href      string `json:"href"`
		Name      string `json:"name,omitempty"`
		Templated bool   `json:"templated,omitempty"`
	}

	// Links defines a group of HAL links.
	Links map[RelType]Link

	// SuccessResponse defines a successful JSend HTTP response.
	SuccessResponse struct {
		Status ResponseStatus `json:"status"`
		Data   any            `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}

	// ErrorResponse defines an erroneous JSend HTTP response.
	ErrorResponse struct {
		Status  ResponseStatus `json:"status"`
		Message string         `json:"message"`
		Code    ErrorCode      `json:"code,omitempty"`
	}

	// FailResponse defines an HTTP request validation failure.
	FailResponse struct {
		Status ResponseStatus `json:"status"`
		Data   any            `json:"data,omitempty"`
		Links  Links          `json:"_links,omitempty"`
	}
)

var (
	adjectives = []string{
		"fast", "lazy", "clever", "curious", "brave", "mighty", "silent", "noisy", "happy", "grumpy",
	}

	nouns = []string{
		"lion", "tiger", "panda", "fox", "eagle", "shark", "wolf", "dragon", "otter", "koala",
	}
)

const (
	// All went well, and (usually) some data was returned.
	ResponseStatusSuccess = "success"

	// There was a problem with the data submitted, or some pre-condition of the API call wasn't satisfied.
	ResponseStatusFail = "fail"

	// An error occurred in processing the request, i.e. an exception was thrown.
	ResponseStatusError = "error"

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

	// Refers to a custom recommendations relation.
	RelTypeRecommendation RelType = "/rels/recommendations"
)

// WriteJSON implements a helper for writing HTTP status and encoding data.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode response data", "err", err)
	}
}

func SetDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func Success(w http.ResponseWriter, status int, data any, links Links) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, SuccessResponse{ResponseStatusSuccess, data, links})
}

func Error(w http.ResponseWriter, status int, message string) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, ErrorResponse{Status: ResponseStatusError, Message: message})
}

func Fail(w http.ResponseWriter, status int, data any) {
	SetDefaultHeaders(w)
	WriteJSON(w, status, FailResponse{Status: ResponseStatusFail, Data: data})
}

func DecodeRequestBody[T any](r *http.Request) (data *T, err error) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err = dec.Decode(data)
	if err != nil {
		return nil, ErrFailedDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}
	return data, err
}

// GenerateUsername creates username with a cryptographically random suffix
func GenerateUsername() (string, error) {
	randInt := func(n int) int {
		if n <= 0 {
			return 0
		}
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		return int(b[0]) % n
	}
	adj := adjectives[randInt(len(adjectives))]
	noun := nouns[randInt(len(nouns))]

	suffix := make([]byte, 2)
	_, err := rand.Read(suffix)
	if err != nil {
		return "", ErrFailedGenerateUsernameSuffix
	}

	username := fmt.Sprintf("%s_%s%s", adj, noun, hex.EncodeToString(suffix))
	username = strings.ToLower(username)

	return username, nil
}

func Health(w http.ResponseWriter, r *http.Request) {
	Success(w, http.StatusOK, nil, nil)
}

func GetPosition(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links := Links{}

		position, err := s.GetPosition(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			links[RelTypeUp] = Link{Href: RoutePositions}

			Fail(w, http.StatusNotFound, FailData{"id": "position not found"})
			return
		}
		if err != nil {
			slog.Error("query failed", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		links[RelTypeSelf] = Link{Href: r.URL.Path}
		links[RelTypeUp] = Link{Href: RoutePositions}

		Success(w, http.StatusOK, position, links)
	}
}

func GetCandidate(s Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links := Links{}

		candidate, err := s.GetCandidate(r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			links[RelTypeUp] = Link{Href: RoutePositions}

			Fail(w, http.StatusNotFound, FailData{"id": "candidate not found"})
			return
		}
		if err != nil {
			slog.Error("query failed", "err", err)
			Error(w, http.StatusInternalServerError, "internal server error")
			return
		}

		links[RelTypeSelf] = Link{Href: r.URL.Path}
		links[RelTypeUp] = Link{Href: RouteCandidates}

		Success(w, http.StatusOK, candidate, links)
	}
}
