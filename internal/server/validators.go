// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

const (
	// PageSizeDefaultLimit is used when the client does not provide a limit parameter.
	PageSizeDefaultLimit = 50

	// PageSizeMaxLimit prevents clients from requesting excessively large datasets.
	PageSizeMaxLimit = 100
)

// ValidateSerialID converts a string ID to a positive integer.
//
// It returns an error if the string is not an integer or if the ID is non-positive.
func ValidateSerialID(strID string) (uint32, error) {
	id, err := strconv.ParseUint(strID, 10, 32)
	if err != nil {
		return 0, ErrFailedToParseSerialID
	}
	if id == 0 {
		return 0, ErrInvalidID
	}
	return uint32(id), nil
}

// ValidateLimit parses the limit query parameter.
//
// It returns an error if the limit is not zero or a positive integer.
//
// It automatically caps the limit to the maximum limit allowed.
func ValidateLimit(strLimit string) (uint8, error) {
	if strLimit == "" {
		return PageSizeDefaultLimit, nil
	}

	limit, err := strconv.ParseUint(strLimit, 10, 8)
	if err != nil {
		return 0, ErrFailedToParseLimit
	}

	if limit > PageSizeMaxLimit {
		limit = PageSizeMaxLimit
	}

	return uint8(limit), nil
}

// ValidateOffset parses the offset query parameter for pagination.
//
// It returns an error if the offset is not zero or a positive integer.
func ValidateOffset(strOffset string) (uint8, error) {
	if strOffset == "" {
		return 0, nil
	}

	offset, err := strconv.ParseUint(strOffset, 10, 8)
	if err != nil {
		return 0, ErrFailedToParseOffset
	}

	return uint8(offset), nil
}

func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)

	reTags := regexp.MustCompile(`<[^>]*>`)
	name = reTags.ReplaceAllString(name, "")

	reValid := regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	if !reValid.MatchString(name) {
		return "", ErrNameHasForbiddenChars
	}

	if len(name) < 1 {
		return "", ErrNameTooShort
	}
	if len(name) > 128 {
		return "", ErrNameTooLong
	}

	return html.EscapeString(name), nil
}

func ValidateAbout(about string) (string, error) {
	about = strings.TrimSpace(about)

	reTags := regexp.MustCompile(`<[^>]*>`)
	about = reTags.ReplaceAllString(about, "")

	reValid := regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	if !reValid.MatchString(about) {
		return "", ErrAboutHasForbiddenChars 
	}

	if len(about) < 1 {
		return "", ErrAboutTooShort 
	}
	if len(about) > 500 {
		return "", ErrAboutTooLong 
	}

	return html.EscapeString(about), nil
}
