// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

type Provider string

const (
	ProviderApple  Provider = "apple"
	ProviderGoogle Provider = "google"
)

func (p Provider) Raw() string {
	if p == ProviderApple {
		return "apple"
	}
	if p == ProviderGoogle {
		return "google"
	}
	return ""
}

type Position struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
}

type User struct {
	Provider       Provider `json:"provider,omitempty"`
	ProviderUserID string   `json:"provider_user_id,omitempty"`
	Email          string   `json:"email,omitempty"`
	FirstName      string   `json:"first_name,omitempty"`
	LastName       string   `json:"last_name,omitempty"`
	FullName       string   `json:"full_name,omitempty"`
	UserName       string   `json:"user_name"`
}

type Candidate struct {
	ID     string `json:"id"`
	UserID string `json:"user_id,omitempty"`
	About  string `json:"about"`
}

type Recruiter struct {
	UserID string
}

// Match represents a successful connection between a candidate and a specific job position.
type Match struct {
	CandidateID string
	PositionID  string
}

// ReactionType defines a restricted set of strings representing user sentiment.
type ReactionType string

const (
	// ReactionTypePositive indicates interest or approval.
	ReactionTypePositive ReactionType = "positive"

	// ReactionTypeNegative indicates a lack of interest or rejection.
	ReactionTypeNegative ReactionType = "negative"
)

func (r ReactionType) IsValid() bool {
	return r == ReactionTypePositive || r == ReactionTypeNegative
}

// CandidateReaction represents the internal model for a candidate's response to a job posting.
type CandidateReaction struct {
	CandidateID  string
	PositionID   string
	ReactionType ReactionType
}

// RecruiterReaction represents the internal model for a recruiter's response to a specific candidate.
type RecruiterReaction struct {
	RecruiterID  string
	CandidateID  string
	PositionID   string
	ReactionType ReactionType
}
