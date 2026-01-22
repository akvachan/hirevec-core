// Copyright (c) 2026 Arsenii Kvachan. MIT License.

// Package models implements basic data structures and structs used throughout the backend.
package models

// Paginator defines parameters for paginating database queries and API responses.
type Paginator struct {
	// Limit is the maximum number of records to return.
	Limit int `json:"limit"`

	// Offset is the number of records to skip before starting to return results.
	Offset int `json:"offset"`
}

// Match represents a successful connection between a candidate and a specific job position.
type Match struct {
	CandidateID int
	PositionID  int
}

// ReactionType defines a restricted set of strings representing user sentiment.
type ReactionType string

const (
	// Positive indicates interest or approval.
	Positive ReactionType = "positive"

	// Negative indicates a lack of interest or rejection.
	Negative ReactionType = "negative"
)

// CandidateReaction represents the internal model for a candidate's response to a job posting.
type CandidateReaction struct {
	CandidateID  int
	PositionID   int
	ReactionType string
}

// RecruiterReaction represents the internal model for a recruiter's response to a specific candidate.
type RecruiterReaction struct {
	RecruiterID  int
	CandidateID  int
	PositionID   int
	ReactionType string
}

// PostMatchRequest defines the payload for creating a manual match via the API.
type PostMatchRequest struct {
	PositionID  string `json:"position_id"`
	CandidateID string `json:"candidate_id"`
}

// PostCandidateReactionRequest defines the API payload when a candidate swipes or reacts to a position.
type PostCandidateReactionRequest struct {
	PositionID   string       `json:"position_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

// PostRecruiterReactionRequest defines the API payload when a recruiter swipes or reacts to a candidate.
type PostRecruiterReactionRequest struct {
	PositionID   string       `json:"position_id"`
	CandidateID  string       `json:"candidate_id"`
	ReactionType ReactionType `json:"reaction_type"`
}
