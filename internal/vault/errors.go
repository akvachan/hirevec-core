// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package vault deals with authentication and authorization.
package vault

import (
	"fmt"
)

var (
	ErrEmailNotVerified                 = fmt.Errorf("email not verified")
	ErrFailedToCreateAccessToken        = func(err error) error { return fmt.Errorf("failed to create access token: %w", err) }
	ErrFailedToCreateAppleOIDCProvider  = func(err error) error { return fmt.Errorf("failed to create Apple OIDC provider: %w", err) }
	ErrFailedToCreateGoogleOIDCProvider = func(err error) error { return fmt.Errorf("failed to create Google OIDC provider: %w", err) }
	ErrFailedToCreateRefreshToken       = func(err error) error { return fmt.Errorf("failed to create refresh token: %w", err) }
	ErrFailedToExchangeToken            = func(err error) error { return fmt.Errorf("token exchange failed: %v", err) }
	ErrFailedToLoadAsymmetricKey        = fmt.Errorf("failed to load an asymmetric key")
	ErrFailedToLoadSymmetricKey         = fmt.Errorf("failed to load a symmetric key")
	ErrFailedToParseClaims              = fmt.Errorf("failed to parse claims")
	ErrFailedToParseJTI                 = fmt.Errorf("failed to parse JTI from claims")
	ErrFailedToParseProvider            = fmt.Errorf("failed to parse provider from claims")
	ErrFailedToParseTokenType           = fmt.Errorf("failed to parse token type from claims")
	ErrFailedToParseUserID              = fmt.Errorf("failed to parse user ID from subject")
	ErrFailedToSetProvider              = fmt.Errorf("failed to set provider in claims")
	ErrFailedToSetScope                 = fmt.Errorf("failed to set scope in claims")
	ErrFailedToSetTokenType             = fmt.Errorf("failed to set token type in claims")
	ErrIDTokenRequired                  = fmt.Errorf("no id_token field in oauth2 token")
	ErrInvalidAccessToken               = fmt.Errorf("invalid access token")
	ErrInvalidIDToken                   = fmt.Errorf("invalid id_token")
	ErrInvalidProvider                  = fmt.Errorf("invalid provider")
	ErrInvalidRefreshToken              = fmt.Errorf("invalid refresh token")
	ErrInvalidSubject                   = fmt.Errorf("invalid subject")
	ErrInvalidTokenType                 = fmt.Errorf("invalid token type")
	ErrNameHasForbiddenChars            = fmt.Errorf("`name` field contains forbidden characters")
	ErrNameTooLong                      = fmt.Errorf("`name` field length must be smaller than 128 characters")
	ErrNameTooShort                     = fmt.Errorf("`name` field length must be bigger than 1 character")
	ErrInvalidRole                      = fmt.Errorf("invalid role")
	ErrFailedToParseScope               = fmt.Errorf("failed to parse scope from claims")
)
