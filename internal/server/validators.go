// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

// Package server implements basic routing, middleware, handlers and validation
package server

import (
	"errors"
	"strconv"

	"github.com/akvachan/hirevec-backend/internal/models"
)

const (
	pageSizeDefaultLimit = 50
	pageSizeMaxLimit     = 100
)

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

func validateReactionType(rtype models.ReactionType) (string, error) {
	switch rtype {
	case models.Like, models.Dislike:
		return string(rtype), nil
	default:
		return "", errors.New("invalid reaction type")
	}
}
