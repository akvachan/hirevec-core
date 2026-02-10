// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package models provides an commond database models
package models

type Provider string

const (
	Apple  Provider = "apple"
	Google Provider = "google"
)

func (p Provider) Raw() string {
	if p == Apple {
		return "apple"
	}
	if p == Google {
		return "google"
	}
	return ""
}

type User struct {
	Provider       Provider
	ProviderUserID string
	Email          string
	FirstName      string
	LastName       string
	FullName       string
}

type Candidate struct {
	UserID string
	About  string
}

type Recruiter struct {
	UserID string
}

// Paginator defines parameters for paginating database queries and API responses.
type Paginator struct {
	// Limit is the maximum number of records to return.
	Limit uint8 `json:"limit"`

	// Offset is the number of records to skip before starting to return results.
	Offset uint8 `json:"offset"`
}

// Match represents a successful connection between a candidate and a specific job position.
type Match struct {
	CandidateID uint32
	PositionID  uint32
}

// ReactionType defines a restricted set of strings representing user sentiment.
type ReactionType string

const (
	// Positive indicates interest or approval.
	Positive ReactionType = "positive"

	// Negative indicates a lack of interest or rejection.
	Negative ReactionType = "negative"
)

func (r ReactionType) IsValid() bool {
	return r == Positive || r == Negative
}

// CandidateReaction represents the internal model for a candidate's response to a job posting.
type CandidateReaction struct {
	CandidateID  uint32
	PositionID   uint32
	ReactionType ReactionType
}

// RecruiterReaction represents the internal model for a recruiter's response to a specific candidate.
type RecruiterReaction struct {
	RecruiterID  uint32
	CandidateID  uint32
	PositionID   uint32
	ReactionType ReactionType
}
