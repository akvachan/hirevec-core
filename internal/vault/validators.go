// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package vault deals with authentication and authorization.
package vault

import (
	"html"
	"regexp"
	"strings"
)

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
