// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package store provides an interface to the storage components
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/akvachan/hirevec-backend/internal/store/db/models"
)

type Store interface {
	CreateCandidate(models.Candidate) error
	CreateCandidateReaction(models.CandidateReaction) error
	CreateMatch(models.Match) error
	CreateRecruiterReaction(models.RecruiterReaction) error
	CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
	CreateUser(models.User) (userID string, err error)
	GetCandidate(id uint32) (json.RawMessage, error)
	GetCandidates(models.Paginator) (json.RawMessage, error)
	GetPosition(id uint32) (json.RawMessage, error)
	GetPositions(models.Paginator) (json.RawMessage, error)
	GetUserByProvider(provider models.Provider, providerUserID string) (userID string, roles []string, err error)
	ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
}

type StoreImpl struct {
	Postgres *sql.DB
}

type StoreConfig struct {
	PostgresHost     string
	PostgresPort     uint16
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
}

func NewStore(c StoreConfig) (*StoreImpl, error) {
	dbConnString := fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s",
		c.PostgresUser,
		c.PostgresPassword,
		c.PostgresHost,
		c.PostgresPort,
		c.PostgresDB,
	)
	database, err := sql.Open("pgx", dbConnString)
	if err != nil {
		return nil, ErrFailedToConnectDB(err)
	}
	return &StoreImpl{Postgres: database}, nil
}
