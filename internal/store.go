// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type (
	Store interface {
		CreateCandidate(Candidate) error
		CreateMatch(Match) error
		CreateReaction(Reaction) error
		CreateRecommendation(positionID, candidateID string) (recID string, err error)
		CreateRecruiter(Recruiter) error
		CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
		CreateUser(User) (userID string, err error)
		GetCandidate(id string) (*Candidate, error)
		GetCandidateByUserID(userID string) (Candidate, error)
		GetPosition(id string) (*Position, error)
		GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error)
		GetUserRoles(userID string, provider Provider) (roles []string, err error)
		ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
	}

	PostgresStore struct {
		Postgres *sql.DB
	}

	PagedResult[T any] struct {
		Items   []T
		HasPrev bool
		HasNext bool
	}

	StoreConfig struct {
		PostgresHost     string
		PostgresPort     uint16
		PostgresDB       string
		PostgresUser     string
		PostgresPassword string
	}
)

func NewPostgresStore(c StoreConfig) (*PostgresStore, error) {
	dbConnString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s",
		c.PostgresHost,
		c.PostgresPort,
		c.PostgresUser,
		c.PostgresPassword,
		c.PostgresDB,
	)
	database, err := sql.Open("pgx", dbConnString)
	if err != nil {
		return nil, ErrFailedConnectDB
	}
	return &PostgresStore{Postgres: database}, nil
}

func (s PostgresStore) GetCandidate(id string) (*Candidate, error) {
	var c Candidate

	err := s.Postgres.QueryRow(
		`
		SELECT id, about
		FROM v1.candidates
		WHERE id = $1
		`,
		id,
	).Scan(
		&c.ID,
		&c.About,
	)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s PostgresStore) GetPosition(id string) (*Position, error) {
	var p Position

	err := s.Postgres.QueryRow(
		`
		SELECT id, recruiter_id, title, description, company
		FROM v1.positions
		WHERE id = $1
		`,
		id,
	).Scan(
		&p.ID,
		&p.RecruiterID,
		&p.Title,
		&p.Description,
		&p.Company,
	)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// GetCandidateByUserID fetches a candidate by their associated user ID.
func (s PostgresStore) GetCandidateByUserID(userID string) (Candidate, error) {
	var c Candidate
	query := `
        SELECT id, user_id, about
        FROM v1.candidates
        WHERE user_id = $1
        LIMIT 1
    `

	err := s.Postgres.QueryRow(query, userID).Scan(&c.ID, &c.UserID, &c.About)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, err
	}

	return c, nil
}

// GetUserByProvider retrieves an existing user and his role based on their provider details.
func (s PostgresStore) GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error) {
	var isCandidate, isRecruiter bool

	err = s.Postgres.QueryRow(
		`
		SELECT
				u.id,
				EXISTS (
						SELECT 1 FROM v1.candidates c WHERE c.user_id = u.id
				) AS is_candidate,
				EXISTS (
						SELECT 1 FROM v1.recruiters r WHERE r.user_id = u.id
				) AS is_recruiter
		FROM v1.users u
		WHERE u.provider = $1
			AND u.provider_user_id = $2
		`,
		provider,
		providerUserID,
	).Scan(&userID, &isCandidate, &isRecruiter)

	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("%w: providerUserID=%s", ErrUserNotFound, providerUserID)
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
		return userID, nil, ErrUserNoRole
	}

	return userID, roles, nil
}

// GetUserRoles fetches user roles by user's ID and provider.
func (s PostgresStore) GetUserRoles(userID string, provider Provider) (roles []string, err error) {
	var isCandidate, isRecruiter, isAdmin bool

	err = s.Postgres.QueryRow(
		`
		SELECT
				u.id,
				EXISTS (
						SELECT 1 FROM v1.candidates c WHERE c.user_id = u.id
				) AS is_candidate,
				EXISTS (
						SELECT 1 FROM v1.recruiters r WHERE r.user_id = u.id
				) AS is_recruiter
				EXISTS (
						SELECT 1 FROM v1.admins r WHERE r.user_id = u.id
				) AS is_admin
		FROM v1.users u
		WHERE u.user_id = $1
			AND u.provider = $2
		`,
		userID,
		provider,
	).Scan(&isCandidate, &isRecruiter, &isAdmin)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: userID=%s", ErrUserNotFound, userID)
	}
	if err != nil {
		return nil, err
	}

	if isCandidate {
		roles = append(roles, "candidate")
	}
	if isRecruiter {
		roles = append(roles, "recruiter")
	}
	if isAdmin {
		roles = append(roles, "admin")
	}

	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}

	return roles, nil
}

// CreateUser generates a unique username and inserts a new user record.
func (s PostgresStore) CreateUser(u User) (userID string, err error) {
	if u.FullName == "" {
		return "", ErrFullNameRequired
	}

	if u.UserName == "" {
		return "", ErrUserNameRequired
	}

	suffix := make([]byte, 2)
	_, err = rand.Read(suffix)
	if err != nil {
		return "", ErrFailedGenerateUsernameSuffix
	}

	err = s.Postgres.QueryRow(
		`
		INSERT INTO v1.users (
			provider,
			provider_user_id, 
			email,
			full_name,
			user_name
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
		`,
		u.Provider,
		u.ProviderUserID,
		u.Email,
		u.FullName,
		u.UserName,
	).Scan(&userID)
	return userID, err
}

// CreateReaction records a reaction (from a candidate or recruiter) to a recommendation.
func (s PostgresStore) CreateReaction(r Reaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.reactions (
			recommendation_id,
			reactor_type,
			reactor_id,
			reaction_type
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (recommendation_id, reactor_type, reactor_id) DO UPDATE
		SET reaction_type = EXCLUDED.reaction_type,
		    created_at = NOW()
		`,
		r.RecommendationID,
		r.ReactorType,
		r.ReactorID,
		r.ReactionType,
	)
	return err
}

// CreateCandidate creates a candidate
func (s PostgresStore) CreateCandidate(c Candidate) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.candidates (
			user_id,
			about
		)
		VALUES ($1, $2)
		`,
		c.UserID,
		c.About,
	)
	return err
}

// CreateRecruiter creates a recruiter
func (s PostgresStore) CreateRecruiter(r Recruiter) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.recruiters (
			user_id
		)
		VALUES ($1)
		`,
		r.UserID,
	)
	return err
}

// CreateMatch creates a new match record between a candidate and a position when mutual interest is established.
func (s PostgresStore) CreateMatch(m Match) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.matches (
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
	 	FROM v1.refresh_tokens 
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
		INSERT INTO v1.refresh_tokens (
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

// CreateRecommendation inserts a new recommendation for a candidate and a position.
func (s PostgresStore) CreateRecommendation(positionID, candidateID string) (recID string, err error) {
	query := `
		INSERT INTO v1.recommendations (position_id, candidate_id)
		VALUES ($1, $2)
		ON CONFLICT (position_id, candidate_id) DO NOTHING
		RETURNING id
	`
	err = s.Postgres.QueryRow(query, positionID, candidateID).Scan(&recID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRecommendationExists
		}
		return "", err
	}

	if recID == "" {
		return "", ErrRecommendationExists
	}

	return recID, nil
}
