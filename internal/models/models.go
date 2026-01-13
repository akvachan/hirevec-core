// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

// Package models implements basic data structures and structs used throughout the backend
package models

type Paginator struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type Match struct {
	CandidateID int
	PositionID  int
}

type ReactionType string

const (
	Like    ReactionType = "like"
	Dislike ReactionType = "dislike"
)

type CandidateReaction struct {
	CandidateID  int
	PositionID   int
	ReactionType string
}

type PostCandidatesReactionRequest struct {
	PositionID   string       `json:"position_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

type PostMatchRequest struct {
	PositionID  string `json:"position_id"`
	CandidateID string `json:"candidate_id"`
}

type RecruiterReaction struct {
	RecruiterID  int
	CandidateID  int
	PositionID   int
	ReactionType string
}

type PostRecruitersReactionRequest struct {
	PositionID   string       `json:"position_id"`
	CandidateID  string       `json:"candidate_id"`
	ReactionType ReactionType `json:"reaction_type"`
}
