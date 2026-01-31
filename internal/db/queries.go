// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package db provides an interface to the database
package db

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GetPosition retrieves a single position from the database by its unique identifier.
func GetPosition(positionID uint32) (json.RawMessage, error) {
	var j json.RawMessage
	err := HirevecDatabase.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.positions t
		WHERE t.id = $1
		`,
		positionID,
	).Scan(&j)
	return j, err
}

// GetPositions retrieves a paginated list of all positions, ordered by ID.
func GetPositions(p Paginator) (json.RawMessage, error) {
	var j json.RawMessage
	err := HirevecDatabase.QueryRow(
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
	return j, err
}

// GetCandidate retrieves a single candidate's details by their ID.
func GetCandidate(candidateID uint32) (json.RawMessage, error) {
	var j json.RawMessage
	err := HirevecDatabase.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.candidates t
		WHERE t.id = $1
		`,
		candidateID,
	).Scan(&j)
	return j, err
}

// GetCandidates retrieves a paginated list of candidates, ordered by ID.
func GetCandidates(p Paginator) (json.RawMessage, error) {
	var j json.RawMessage
	err := HirevecDatabase.QueryRow(
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
	return j, err
}

// GetUserByProvider retrieves an existing user based on their provider details.
func GetUserByProvider(provider string, providerUserID string) (userID uint32, err error) {
	err = HirevecDatabase.QueryRow(
		`
		SELECT id 
		FROM general.users 
		WHERE provider = $1 
		AND provider_user_id = $2
		`,
		provider,
		providerUserID,
	).Scan(&userID)
	return userID, err
}

// CreateUser generates a unique username and inserts a new user record.
func CreateUser(user User) (userID uint32, err error) {
	if user.FirstName == "" || user.LastName == "" || user.FullName == "" {
		return 0, errors.New("empty names provided")
	}

	suffix := make([]byte, 2)
	_, err = rand.Read(suffix)
	if err != nil {
		return 0, errors.New("could not generate a random suffix")
	}

	userName := fmt.Sprintf("%s_%s_%s",
		strings.ToLower(user.FirstName),
		strings.ToLower(user.LastName),
		hex.EncodeToString(suffix),
	)

	err = HirevecDatabase.QueryRow(
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
func CreateCandidateReaction(r CandidateReaction) error {
	_, err := HirevecDatabase.Exec(
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

// CreateRecruiterReaction records a recruiter's reaction to a specific candidate for a position.
func CreateRecruiterReaction(r RecruiterReaction) error {
	_, err := HirevecDatabase.Exec(
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
func CreateMatch(m Match) error {
	_, err := HirevecDatabase.Exec(
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
func ValidateActiveSession(jti string) (isSessionRevoked bool, err error) {
	err = HirevecDatabase.QueryRow(
		`
		SELECT revoked 
	 	FROM general.refresh_tokens 
	 	WHERE jti = $1 
		AND expires_at > NOW()
		`,
		jti,
	).Scan(&isSessionRevoked)
	return isSessionRevoked, err
}

// CreateRefreshToken creates a new refresh token record.
func CreateRefreshToken(userID uint32, expiresAt time.Time) (jti string, err error) {
	err = HirevecDatabase.QueryRow(
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
