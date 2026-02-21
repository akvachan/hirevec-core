// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store interface {
	CreateCandidate(Candidate) error
	CreateCandidateReaction(CandidateReaction) error
	CreateMatch(Match) error
	CreateRecruiterReaction(RecruiterReaction) error
	CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
	CreateUser(User) (userID string, err error)
	GetCandidate(id uint32) (json.RawMessage, error)
	GetCandidates(Paginator) (json.RawMessage, error)
	GetPosition(id uint32) (json.RawMessage, error)
	GetPositions(Paginator) (json.RawMessage, error)
	GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error)
	ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
}

type StoreConfig struct {
	PostgresHost     string
	PostgresPort     uint16
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
}

type PostgresStore struct {
	Postgres *sql.DB
}

func NewPostgresStore(c StoreConfig) (*PostgresStore, error) {
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
	return &PostgresStore{Postgres: database}, nil
}

// GetPosition retrieves a single position from the database by its unique identifier.
func (s PostgresStore) GetPosition(id uint32) (j json.RawMessage, err error) {
	return j, s.Postgres.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.positions t
		WHERE t.id = $1
		`,
		id,
	).Scan(&j)
}

// GetPositions retrieves a paginated list of all positions, ordered by ID.
func (s PostgresStore) GetPositions(p Paginator) (j json.RawMessage, err error) {
	return j, s.Postgres.QueryRow(
		`
		SELECT COALESCE(json_agg(t), '[]'::json)
		FROM (
			SELECT *
			FROM general.positions
			ORDER BY id
			LIMIT $1 OFFSET $2
		) t
		`,
		p.Limit,
		p.Offset,
	).Scan(&j)
}

// GetCandidate retrieves a single candidate's details by their ID.
func (s PostgresStore) GetCandidate(id uint32) (j json.RawMessage, err error) {
	return j, s.Postgres.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.candidates t
		WHERE t.id = $1
		`,
		id,
	).Scan(&j)
}

// GetCandidates retrieves a paginated list of candidates, ordered by ID.
func (s PostgresStore) GetCandidates(p Paginator) (j json.RawMessage, err error) {
	return j, s.Postgres.QueryRow(
		`
		SELECT COALESCE(json_agg(t), '[]'::json)
		FROM (
			SELECT *
			FROM general.candidates
			ORDER BY id 
			LIMIT $1 OFFSET $2
		) t
		`,
		p.Limit,
		p.Offset,
	).Scan(&j)
}

// GetUserByProvider retrieves an existing user and his role based on their provider details.
func (s PostgresStore) GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error) {
	var isCandidate, isRecruiter bool

	err = s.Postgres.QueryRow(
		`
        SELECT
            u.id,
            EXISTS (
                SELECT 1 FROM general.candidates c WHERE c.user_id = u.id
            ) AS is_candidate,
            EXISTS (
                SELECT 1 FROM general.recruiters r WHERE r.user_id = u.id
            ) AS is_recruiter
        FROM general.users u
        WHERE u.provider = $1
          AND u.provider_user_id = $2
        `,
		provider,
		providerUserID,
	).Scan(&userID, &isCandidate, &isRecruiter)

	if err == sql.ErrNoRows {
		return "", nil, ErrUserDoesNotExist
	}
	if err != nil {
		return "", nil, err
	}

	if isCandidate {
		roles = append(roles, "candidate")
	}
	if isRecruiter {
		roles = append(roles, "recruiter")
	}

	if len(roles) == 0 {
		return userID, nil, ErrUserDoesNotHaveARole
	}

	return userID, roles, nil
}

// CreateUser generates a unique username and inserts a new user record.
func (s PostgresStore) CreateUser(user User) (userID string, err error) {
	if user.FirstName == "" || user.LastName == "" || user.FullName == "" {
		return "", ErrNamesRequired
	}

	suffix := make([]byte, 2)
	_, err = rand.Read(suffix)
	if err != nil {
		return "", ErrFailedToGenerateUsernameSuffix(err)
	}

	userName := fmt.Sprintf("%s_%s_%s",
		strings.ToLower(user.FirstName),
		strings.ToLower(user.LastName),
		hex.EncodeToString(suffix),
	)

	err = s.Postgres.QueryRow(
		`
		INSERT INTO general.users (
			provider,
			provider_user_id, 
			email,
			full_name,
			user_name
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
		`,
		user.Provider,
		user.ProviderUserID,
		user.Email,
		user.FullName,
		userName,
	).Scan(&userID)
	return userID, err
}

// CreateCandidateReaction records a candidate's interest or reaction to a specific job position.
func (s PostgresStore) CreateCandidateReaction(r CandidateReaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO general.candidates_reactions (
			candidate_id,
			position_id,
			reaction_type
		)
		VALUES ($1, $2, $3)
		`,
		r.CandidateID,
		r.PositionID,
		r.ReactionType,
	)
	return err
}

// CreateCandidate creates a candidate
func (s PostgresStore) CreateCandidate(r Candidate) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO general.candidate (
			user_id,
			about
		)
		VALUES ($1, $2)
		`,
		r.UserID,
		r.About,
	)
	return err
}

// CreateRecruiterReaction records a recruiter's reaction to a specific candidate for a position.
func (s PostgresStore) CreateRecruiterReaction(r RecruiterReaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO general.recruiters_reactions (
			recruiter_id,
			position_id,
			candidate_id,
			reaction_type
		)
		VALUES ($1, $2, $3, $4)
		`,
		r.RecruiterID,
		r.PositionID,
		r.CandidateID,
		r.ReactionType,
	)
	return err
}

// CreateMatch creates a new match record between a candidate and a position when mutual interest is established.
func (s PostgresStore) CreateMatch(m Match) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO general.matches (
			candidate_id,
			position_id
		)
		VALUES ($1, $2)
		`,
		m.CandidateID,
		m.PositionID,
	)
	return err
}

// ValidateActiveSession checks if the JTI exists and is not expired.
func (s PostgresStore) ValidateActiveSession(jti string) (isSessionRevoked bool, err error) {
	return isSessionRevoked, s.Postgres.QueryRow(
		`
		SELECT revoked 
	 	FROM general.refresh_tokens 
	 	WHERE jti = $1 
		AND expires_at > NOW()
		`,
		jti,
	).Scan(&isSessionRevoked)
}

// CreateRefreshToken creates a new refresh token record.
func (s PostgresStore) CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error) {
	err = s.Postgres.QueryRow(
		`
		INSERT INTO general.refresh_tokens (
			user_id,
			expires_at
		)
		VALUES ($1, $2) 
		RETURNING jti
		`,
		userID,
		expiresAt,
	).Scan(&jti)
	return jti, err
}
