// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package server implements the HTTP transport layer, providing RESTful endpoints.
package server

import (
	"errors"
	"strconv"

	"github.com/akvachan/hirevec-backend/internal/models"
)

const (
	// pageSizeDefaultLimit is used when the client does not provide a limit parameter.
	pageSizeDefaultLimit = 50

	// pageSizeMaxLimit prevents clients from requesting excessively large datasets.
	pageSizeMaxLimit = 100
)

// validateSerialID converts a string ID to a positive integer.
//
// It returns an error if the string is not an integer or if the ID is non-positive.
func validateSerialID(strID string) (int, error) {
	id, err := strconv.Atoi(strID)
	if err != nil {
		return 0, errors.New("id must be an integer")
	}
	if id <= 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return id, nil
}

// validateLimit parses the limit query parameter.
//
// It returns an error if the limit is not zero or a positive integer.
//
// It automatically caps the limit to the maximum limit allowed.
func validateLimit(strLimit string) (int, error) {
	if strLimit == "" {
		return pageSizeDefaultLimit, nil
	}

	limit, err := strconv.Atoi(strLimit)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}

	if limit <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if limit > pageSizeMaxLimit {
		limit = pageSizeMaxLimit
	}

	return limit, nil
}

// validateOffset parses the offset query parameter for pagination.
//
// It returns an error if the offset is not zero or a positive integer.
func validateOffset(strOffset string) (int, error) {
	if strOffset == "" {
		return 0, nil
	}

	offset, err := strconv.Atoi(strOffset)
	if err != nil {
		return 0, errors.New("offset must be an integer")
	}
	if offset < 0 {
		return 0, errors.New("offset must be zero or a positive integer")
	}
	return offset, nil
}

// validateReactionType checks if the provided reaction matches the allowed models.
//
// It returns an error if reaction type is not valid.
func validateReactionType(rtype models.ReactionType) (string, error) {
	switch rtype {
	case models.Positive, models.Negative:
		return string(rtype), nil
	default:
		return "", errors.New("invalid reaction type")
	}
}
