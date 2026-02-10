// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"fmt"
)

var (
	ErrAboutHasForbiddenChars			= fmt.Errorf("`about` field contains forbidden characters")
	ErrAboutTooLong								= fmt.Errorf("`about` field length must be smaller than 500 characters")
	ErrAboutTooShort							= fmt.Errorf("`about` field length must be bigger than 1 character")
	ErrDescriptionRequired				= func(path string) error { return fmt.Errorf("description cannot be empty for route: %s", path) }
	ErrExtraDataDecoded						= fmt.Errorf("extra data decoded")
	ErrFailedToAddRoutes         	= func(err error) error { return fmt.Errorf("failed to add routes: %w", err) }
	ErrFailedToAssembleRouteTree 	= func(err error) error { return fmt.Errorf("failed to assemble route tree: %w", err) }
	ErrFailedToBindAddress				= func(host string, err error) error { return fmt.Errorf("failed to bind to %s: %w", host, err) }
	ErrFailedToDecode							= fmt.Errorf("could not decode")
	ErrFailedToParseLimit					= fmt.Errorf("limit must be zero or a positive integer")
	ErrFailedToParseOffset				= fmt.Errorf("offset must be zero or a positive integer")
	ErrFailedToParseSerialID			= fmt.Errorf("id must be an integer")
	ErrFailedToShutdownServer			= func(err error) error { return fmt.Errorf("failed to shutdown server: %w", err) }
	ErrHandlerRequired						= func(path string) error { return fmt.Errorf("handler cannot be nil for route: %s", path) }
	ErrInvalidAPIVersion 					= func(path string, version apiVersion) error { return fmt.Errorf("invalid API version %d for path: %s. Allowed versions: %v", version, path, allowedAPIVersions) }
	ErrInvalidID									= fmt.Errorf("id must be a positive integer")
	ErrMethodNotAllowed						= func(method string, path string) error { return fmt.Errorf("method %s not allowed for path: %s. Allowed methods: %v", method, path, allowedMethods) }
)
