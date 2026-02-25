// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

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
