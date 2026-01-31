// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package auth deals with authentication and authorization
package auth

import "errors"

func ValidateProvider(provider string) (bool, error) {
	if provider != "apple" && provider != "google" {
		return false, errors.New("invalid provider")
	}
	return true, nil
}
