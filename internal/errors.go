// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"fmt"
)

var (
	ErrAboutHasForbiddenChars           = fmt.Errorf("`about` field contains forbidden characters")
	ErrAboutTooLong                     = fmt.Errorf("`about` field length must be smaller than 500 characters")
	ErrAboutTooShort                    = fmt.Errorf("`about` field length must be bigger than 1 character")
	ErrDescriptionRequired              = func(path string) error { return fmt.Errorf("description cannot be empty for route: %s", path) }
	ErrEmailNotVerified                 = fmt.Errorf("email not verified")
	ErrExtraDataDecoded                 = fmt.Errorf("extra data decoded")
	ErrFailedToAddRoutes                = func(err error) error { return fmt.Errorf("failed to add routes: %w", err) }
	ErrFailedToAssembleRouteTree        = func(err error) error { return fmt.Errorf("failed to assemble route tree: %w", err) }
	ErrFailedToBindAddress              = func(host string, err error) error { return fmt.Errorf("failed to bind to %s: %w", host, err) }
	ErrFailedToConnectDB                = func(err error) error { return fmt.Errorf("failed to connect to database: %w", err) }
	ErrFailedToCreateAccessToken        = func(err error) error { return fmt.Errorf("failed to create access token: %w", err) }
	ErrFailedToCreateAppleOIDCProvider  = func(err error) error { return fmt.Errorf("failed to create Apple OIDC provider: %w", err) }
	ErrFailedToCreateGoogleOIDCProvider = func(err error) error { return fmt.Errorf("failed to create Google OIDC provider: %w", err) }
	ErrFailedToCreateRefreshToken       = func(err error) error { return fmt.Errorf("failed to create refresh token: %w", err) }
	ErrFailedToDecode                   = fmt.Errorf("could not decode")
	ErrFailedToExchangeToken            = func(err error) error { return fmt.Errorf("token exchange failed: %v", err) }
	ErrFailedToGenerateUsernameSuffix   = func(err error) error { return fmt.Errorf("failed to generate a username suffix: %w", err) }
	ErrFailedToLoadAsymmetricKey        = fmt.Errorf("failed to load an asymmetric key")
	ErrFailedToLoadSymmetricKey         = fmt.Errorf("failed to load a symmetric key")
	ErrFailedToParseClaims              = fmt.Errorf("failed to parse claims")
	ErrFailedToParseJTI                 = fmt.Errorf("failed to parse JTI from claims")
	ErrFailedToParseLimit               = fmt.Errorf("limit must be zero or a positive integer")
	ErrFailedToParseOffset              = fmt.Errorf("offset must be zero or a positive integer")
	ErrFailedToParseProvider            = fmt.Errorf("failed to parse provider from claims")
	ErrFailedToParseScope               = fmt.Errorf("failed to parse scope from claims")
	ErrFailedToParseSerialID            = fmt.Errorf("id must be an integer")
	ErrFailedToParseTokenType           = fmt.Errorf("failed to parse token type from claims")
	ErrFailedToParseUserID              = fmt.Errorf("failed to parse user ID from subject")
	ErrFailedToSetProvider              = fmt.Errorf("failed to set provider in claims")
	ErrFailedToSetScope                 = fmt.Errorf("failed to set scope in claims")
	ErrFailedToSetTokenType             = fmt.Errorf("failed to set token type in claims")
	ErrFailedToShutdownServer           = func(err error) error { return fmt.Errorf("failed to shutdown server: %w", err) }
	ErrHandlerRequired                  = func(path string) error { return fmt.Errorf("handler cannot be nil for route: %s", path) }
	ErrIDTokenRequired                  = fmt.Errorf("no id_token field in oauth2 token")
	ErrInvalidAccessToken               = fmt.Errorf("invalid access token")
	ErrInvalidID                        = fmt.Errorf("id must be a positive integer")
	ErrInvalidIDToken                   = fmt.Errorf("invalid id_token")
	ErrInvalidProvider                  = fmt.Errorf("invalid provider")
	ErrInvalidRefreshToken              = fmt.Errorf("invalid refresh token")
	ErrInvalidRole                      = fmt.Errorf("invalid role")
	ErrInvalidSubject                   = fmt.Errorf("invalid subject")
	ErrInvalidTokenType                 = fmt.Errorf("invalid token type")
	ErrNameHasForbiddenChars            = fmt.Errorf("`name` field contains forbidden characters")
	ErrNameTooLong                      = fmt.Errorf("`name` field length must be smaller than 128 characters")
	ErrNameTooShort                     = fmt.Errorf("`name` field length must be bigger than 1 character")
	ErrNamesRequired                    = fmt.Errorf("empty names provided")
	ErrUserDoesNotExist                 = fmt.Errorf("user with the  providerUserID does not exist")
	ErrUserDoesNotHaveARole             = fmt.Errorf("user does not have any role")
)
