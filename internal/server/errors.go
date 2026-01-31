// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"errors"
)

var (
	ErrExtraDataDecoded = errors.New("extra data decoded")
	ErrCouldNotDecode   = errors.New("could not decode")
	ErrCouldNotParseSerialID 	= errors.New("id must be an integer")
	ErrNotPositiveSerialID = errors.New("id must be a positive integer")
	ErrCouldNotParseLimit = errors.New("limit must be zero or a positive integer")
	ErrCouldNotParseOffset = errors.New("offset must be zero or a positive integer")
	ErrNameTooLong = errors.New("name length must be smaller than 128 characters")
	ErrNameTooShort = errors.New("name length must be bigger than 1 character")
	ErrNameHasForbiddenChars = errors.New("name contains forbidden characters")
)
