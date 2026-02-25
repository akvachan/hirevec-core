// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type (
	Store interface {
		CreateCandidate(Candidate) error
		CreateCandidateReaction(CandidateReaction) error
		CreateMatch(Match) error
		CreateRecruiterReaction(RecruiterReaction) error
		CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
		CreateUser(User) (userID string, err error)
		GetCandidate(id string) (*Candidate, error)
		GetCandidates(limit uint64, beforeID *string, afterID *string) (candidates []Candidate, hasPrev bool, hasNext bool, err error)
		GetPosition(id string) (*Position, error)
		GetPositions(limit uint64, beforeID *string, afterID *string) (positions []Position, hasPrev bool, hasNext bool, err error)
		GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error)
		ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
	}

	PostgresStore struct {
		Postgres *sql.DB
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
		return nil, ErrFailedToConnectDB(err)
	}
	return &PostgresStore{Postgres: database}, nil
}

func (s PostgresStore) GetPosition(id string) (*Position, error) {
	var p Position

	err := s.Postgres.QueryRow(
		`
		SELECT id, title, description, company
		FROM v1.positions
		WHERE id = $1
		`,
		id,
	).Scan(
		&p.ID,
		&p.Title,
		&p.Description,
		&p.Company,
	)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

func (s PostgresStore) GetPositions(
	limit uint64,
	beforeID *string,
	afterID *string,
) (positions []Position, hasPrev bool, hasNext bool, err error) {
	query := `
        SELECT id, title, description, company
        FROM v1.positions
        WHERE 1=1
    `
	args := []any{}
	argPos := 1

	order := "ASC"

	if afterID != nil {
		query += fmt.Sprintf(" AND id > $%d", argPos)
		args = append(args, *afterID)
		argPos++
		order = "ASC"
	}

	if beforeID != nil {
		query += fmt.Sprintf(" AND id < $%d", argPos)
		args = append(args, *beforeID)
		argPos++
		order = "DESC"
	}

	query += fmt.Sprintf(" ORDER BY id %s LIMIT $%d", order, argPos)
	args = append(args, limit+1)

	rows, err := s.Postgres.Query(query, args...)
	if err != nil {
		return nil, false, false, err
	}

	positions = make([]Position, 0, limit+1)
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Company); err != nil {
			return nil, false, false, err
		}
		positions = append(positions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, false, false, err
	}

	hasMore := uint64(len(positions)) > limit
	if hasMore {
		positions = positions[:limit]
	}

	if beforeID != nil {
		for i, j := 0, len(positions)-1; i < j; i, j = i+1, j-1 {
			positions[i], positions[j] = positions[j], positions[i]
		}
	}

	switch {
	case afterID != nil:
		hasPrev = true
		hasNext = hasMore

	case beforeID != nil:
		hasPrev = hasMore
		hasNext = true

	default:
		hasPrev = false
		hasNext = hasMore
	}

	return positions, hasPrev, hasNext, nil
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

func (s PostgresStore) GetCandidates(
	limit uint64,
	beforeID *string,
	afterID *string,
) (candidates []Candidate, hasPrev bool, hasNext bool, err error) {
	query := `
        SELECT id, about
        FROM v1.candidates
        WHERE 1=1
    `
	args := []any{}
	argPos := 1
	order := "ASC"

	if afterID != nil {
		query += fmt.Sprintf(" AND id > $%d", argPos)
		args = append(args, *afterID)
		argPos++
		order = "ASC"
	}

	if beforeID != nil {
		query += fmt.Sprintf(" AND id < $%d", argPos)
		args = append(args, *beforeID)
		argPos++
		order = "DESC"
	}

	query += fmt.Sprintf(" ORDER BY id %s LIMIT $%d", order, argPos)
	args = append(args, limit+1)

	rows, err := s.Postgres.Query(query, args...)
	if err != nil {
		return nil, false, false, err
	}

	candidates = make([]Candidate, 0, limit+1)
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.About); err != nil {
			return nil, false, false, err
		}
		candidates = append(candidates, c)
	}

	if err := rows.Err(); err != nil {
		return nil, false, false, err
	}

	hasMore := uint64(len(candidates)) > limit
	if hasMore {
		candidates = candidates[:limit]
	}

	if beforeID != nil {
		for i, j := 0, len(candidates)-1; i < j; i, j = i+1, j-1 {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		}
	}

	switch {
	case afterID != nil:
		hasPrev = true
		hasNext = hasMore

	case beforeID != nil:
		hasPrev = hasMore
		hasNext = true

	default:
		hasPrev = false
		hasNext = hasMore
	}

	return candidates, hasPrev, hasNext, nil
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
func (s PostgresStore) CreateUser(u User) (userID string, err error) {
	if u.FirstName == "" || u.LastName == "" || u.FullName == "" {
		return "", ErrNamesRequired
	}

	suffix := make([]byte, 2)
	_, err = rand.Read(suffix)
	if err != nil {
		return "", ErrFailedToGenerateUsernameSuffix(err)
	}

	userName := fmt.Sprintf("%s_%s_%s",
		strings.ToLower(u.FirstName),
		strings.ToLower(u.LastName),
		hex.EncodeToString(suffix),
	)

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
		userName,
	).Scan(&userID)
	return userID, err
}

// CreateCandidateReaction records a candidate's interest or reaction to a specific job position.
func (s PostgresStore) CreateCandidateReaction(r CandidateReaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.candidates_reactions (
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
func (s PostgresStore) CreateCandidate(c Candidate) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.candidate (
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

// CreateRecruiterReaction records a recruiter's reaction to a specific candidate for a position.
func (s PostgresStore) CreateRecruiterReaction(r RecruiterReaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.recruiters_reactions (
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
