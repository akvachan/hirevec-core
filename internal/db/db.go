// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

// Package db implement queries for the database
package db

import (
	"database/sql"
	"encoding/json"

	"github.com/akvachan/hirevec-backend/internal/models"
)

var HirevecDatabase *sql.DB

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
