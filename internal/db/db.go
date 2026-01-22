// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package db implements queries for the database.
package db

import (
	"database/sql"
	"encoding/json"

	"github.com/akvachan/hirevec-backend/internal/models"
)

// HirevecDatabase is the global database connection pool.
var HirevecDatabase *sql.DB

// SelectPositionByID retrieves a single position from the database by its unique identifier and scans the result as a JSON object into outJSON.
func SelectPositionByID(outJSON *json.RawMessage, id int) error {
	return HirevecDatabase.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.positions t
		WHERE t.id = $1
		`,
		id,
	).Scan(outJSON)
}

// SelectPositions retrieves a paginated list of all positions, ordered by ID.
func SelectPositions(outJSON *json.RawMessage, p models.Paginator) error {
	return HirevecDatabase.QueryRow(
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
	).Scan(outJSON)
}

// SelectCandidateByID retrieves a single candidate's details by their ID and scans the result as a JSON object into outJSON.
func SelectCandidateByID(outJSON *json.RawMessage, id int) error {
	return HirevecDatabase.QueryRow(
		`
		SELECT row_to_json(t) 
		FROM general.candidates t
		WHERE t.id = $1
		`,
		id,
	).Scan(outJSON)
}

// SelectCandidates retrieves a paginated list of candidates, ordered by ID.
func SelectCandidates(outJSON *json.RawMessage, p models.Paginator) error {
	return HirevecDatabase.QueryRow(
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
	).Scan(outJSON)
}

// InsertCandidateReaction records a candidate's interest or reaction to a specific job position.
func InsertCandidateReaction(r models.CandidateReaction) error {
	_, err := HirevecDatabase.Exec(
		`
		INSERT INTO general.candidates_reactions (
			candidate_id,
			position_id,
			reaction_type
		)
		VALUES ($1, $2, $3);
		`,
		r.CandidateID,
		r.PositionID,
		r.ReactionType,
	)
	return err
}

// InsertRecruiterReaction records a recruiter's reaction to a specific candidate for a position.
func InsertRecruiterReaction(r models.RecruiterReaction) error {
	_, err := HirevecDatabase.Exec(
		`
		INSERT INTO general.recruiters_reactions (
			recruiter_id,
			position_id,
			candidate_id,
			reaction_type
		)
		VALUES ($1, $2, $3, $4);
		`,
		r.RecruiterID,
		r.PositionID,
		r.CandidateID,
		r.ReactionType,
	)
	return err
}

// InsertMatch creates a new match record between a candidate and a position when mutual interest is established.
func InsertMatch(m models.Match) error {
	_, err := HirevecDatabase.Exec(
		`
		INSERT INTO general.matches (
			candidate_id,
			position_id
		)
		VALUES ($1, $2);
		`,
		m.CandidateID,
		m.PositionID,
	)
	return err
}
