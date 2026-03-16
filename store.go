// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrFailedConnectDB       = errors.New("failed to connect to database")
	ErrUserNoRole            = errors.New("user has no role")
	ErrUserNotFound          = errors.New("user not found")
	ErrRecommendationExists  = errors.New("recommendation already exists")
	ErrCandidateNotFound     = errors.New("candidate not found")
	ErrRecruiterNotFound     = errors.New("recruiter not found")
	ErrReactionAlreadyExists = errors.New("reaction already exists")
)

type ReactionType string

const (
	ReactionTypePositive ReactionType = "positive"
	ReactionTypeNegative ReactionType = "negative"
	ReactionTypeNeutral  ReactionType = "neutral"
)

func (r ReactionType) IsValid() bool {
	return r == ReactionTypePositive || r == ReactionTypeNegative || r == ReactionTypeNeutral
}

func (r ReactorType) IsValid() bool {
	return r == ReactorTypeCandidate || r == ReactorTypeRecruiter
}

type ReactorType string

const (
	ReactorTypeCandidate ReactorType = "candidate"
	ReactorTypeRecruiter ReactorType = "recruiter"
)

type (
	// User represents a system user
	User struct {
		ID             string   `json:"id,omitempty"`
		Provider       Provider `json:"provider,omitempty"`
		ProviderUserID string   `json:"provider_user_id,omitempty"`
		Email          string   `json:"email,omitempty"`
		FullName       string   `json:"full_name,omitempty"`
		UserName       string   `json:"user_name"`
	}

	// Candidate represents a candidate profile
	Candidate struct {
		ID     string `json:"id"`
		UserID string `json:"user_id,omitempty"`
		About  string `json:"about"`
	}

	// Recruiter represents a recruiter profile
	Recruiter struct {
		ID     string `json:"id"`
		UserID string `json:"user_id"`
	}

	Recommendation struct {
		ID          string `json:"id"`
		PositionID  string `json:"position_id"`
		CandidateID string `json:"candidate_id"`
	}

	// Position represents a job position
	Position struct {
		ID          string `json:"id"`
		RecruiterID string `json:"recruiter_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Company     string `json:"company"`
	}

	Match struct {
		PositionID  string    `json:"position_id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Company     string    `json:"company"`
		MatchedAt   time.Time `json:"matched_at"`
	}

	// Reaction represents either a candidate or recruiter reaction to a recommendation
	Reaction struct {
		RecommendationID string       `json:"recommendation_id"`
		ReactorType      ReactorType  `json:"reactor_type"`
		ReactorID        string       `json:"reactor_id"`
		ReactionType     ReactionType `json:"reaction_type"`
		ReactedAt        time.Time    `json:"reacted_at"`
	}

	Page struct {
		Cursor  string `json:"cursor,omitempty"`
		Limit   int    `json:"limit"`
		Count   int    `json:"count"`
		HasNext bool   `json:"has_next"`
	}

	PositionRecommendation struct {
		RecommendationID string `json:"recommendation_id"`
		PositionID       string `json:"position_id"`
		Title            string `json:"title"`
		Company          string `json:"company"`
		Description      string `json:"description"`
	}

	CandidateRecommendation struct {
		RecommendationID string `json:"recommendation_id"`
		CandidateID      string `json:"candidate_id"`
		UserName         string `json:"username"`
		FullName         string `json:"full_name,omitempty"`
		About            string `json:"about"`
	}
)

type StoreInterface interface {
	CreateCandidate(Candidate) error
	CreateReaction(Reaction) error
	CreateRecommendation(positionID, candidateID string) (recID string, err error)
	CreateRecruiter(Recruiter) error
	CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
	CreateUser(User) (userID string, err error)
	GetCandidate(id string) (*Candidate, error)
	GetCandidateByUserID(id string) (*Candidate, error)
	GetReactionsByCandidateID(candidateID string, page Page) ([]Reaction, string, error)
	GetMatchesByCandidateID(candidateID string, page Page) ([]Match, string, error)
	GetRecruiterByUserID(id string) (*Recruiter, error)
	GetPosition(id string) (*Position, error)
	GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []Role, err error)
	GetRecommendation(id string) (*Recommendation, error)
	GetUserRoles(userID string, provider Provider) (roles []Role, err error)
	GetPositionRecommendations(candidateID string, page Page, params RecommendationsQueryParams) ([]PositionRecommendation, string, error)
	ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
}

type StoreConfig struct {
	PostgresHost     string
	PostgresPort     uint16
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
}

type StoreImpl struct {
	Postgres *sql.DB
}

func NewStore(c StoreConfig) (*StoreImpl, error) {
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
	return &StoreImpl{Postgres: database}, nil
}

func (s StoreImpl) GetCandidate(id string) (*Candidate, error) {
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

func (s StoreImpl) GetRecommendation(id string) (*Recommendation, error) {
	var r Recommendation

	err := s.Postgres.QueryRow(
		`
		SELECT id, candidate_id, position_id
		FROM v1.recommendations
		WHERE id = $1
		`,
		id,
	).Scan(
		&r.ID,
		&r.CandidateID,
		&r.PositionID,
	)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

func (s StoreImpl) GetPosition(id string) (*Position, error) {
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
func (s StoreImpl) GetCandidateByUserID(userID string) (*Candidate, error) {
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
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}

	return &c, nil
}

// GetUserByProvider retrieves an existing user and his role based on their provider details.
func (s StoreImpl) GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []Role, err error) {
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
		roles = append(roles, RoleCandidate)
	}
	if isRecruiter {
		roles = append(roles, RoleRecruiter)
	}

	if len(roles) == 0 {
		return userID, nil, ErrUserNoRole
	}

	return userID, roles, nil
}

// GetUserRoles fetches user roles by user's ID and provider.
func (s StoreImpl) GetUserRoles(userID string, provider Provider) (roles []Role, err error) {
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
		WHERE u.user_id = $1
			AND u.provider = $2
		`,
		userID,
		provider,
	).Scan(&isCandidate, &isRecruiter)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: userID=%s", ErrUserNotFound, userID)
	}
	if err != nil {
		return nil, err
	}

	if isCandidate {
		roles = append(roles, RoleCandidate)
	}
	if isRecruiter {
		roles = append(roles, RoleRecruiter)
	}

	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}

	return roles, nil
}

// CreateUser generates a unique username and inserts a new user record.
func (s StoreImpl) CreateUser(u User) (userID string, err error) {
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
func (s StoreImpl) CreateReaction(r Reaction) error {
	result, err := s.Postgres.Exec(
		`
		INSERT INTO v1.reactions (
			recommendation_id,
			reactor_type,
			reactor_id,
			reaction_type
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (recommendation_id, reactor_type, reactor_id) DO NOTHING
		`,
		r.RecommendationID,
		r.ReactorType,
		r.ReactorID,
		r.ReactionType,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrReactionAlreadyExists
	}
	return nil
}

// CreateCandidate creates a candidate
func (s StoreImpl) CreateCandidate(c Candidate) error {
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
func (s StoreImpl) CreateRecruiter(r Recruiter) error {
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

// ValidateActiveSession checks if the JTI exists and is not expired.
func (s StoreImpl) ValidateActiveSession(jti string) (isSessionRevoked bool, err error) {
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
func (s StoreImpl) CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error) {
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
func (s StoreImpl) CreateRecommendation(positionID, candidateID string) (recID string, err error) {
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

// GetPositionRecommendations returns paginated position recommendations for a candidate.
func (s StoreImpl) GetPositionRecommendations(candidateID string, page Page, params RecommendationsQueryParams) ([]PositionRecommendation, string, error) {
	rows, err := s.Postgres.Query(`
		SELECT r.id, p.id, p.title, p.company, p.description
		FROM v1.recommendations r
		JOIN v1.positions p ON p.id = r.position_id
		WHERE r.candidate_id = $1
		  AND ($2 = '' OR r.id > $2)
		  AND (NOT $4 OR NOT EXISTS (
		      SELECT 1 FROM v1.reactions rx
		      WHERE rx.recommendation_id = r.id
		        AND rx.reactor_type = 'candidate'
		        AND rx.reactor_id = $1
		  ))
		ORDER BY r.id ASC
		LIMIT $3
	`, candidateID, page.Cursor, page.Limit+1, params.HideReacted)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var results []PositionRecommendation
	for rows.Next() {
		var pr PositionRecommendation
		if err := rows.Scan(&pr.RecommendationID, &pr.PositionID, &pr.Title, &pr.Company, &pr.Description); err != nil {
			return nil, "", err
		}
		results = append(results, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = results[page.Limit-1].RecommendationID
	}

	return results, nextCursor, nil
}

// GetReactionsByCandidateID returns paginated reactions made by a candidate.
func (s StoreImpl) GetReactionsByCandidateID(candidateID string, page Page) ([]Reaction, string, error) {
	rows, err := s.Postgres.Query(`
		SELECT recommendation_id, reactor_type, reactor_id, reaction_type, created_at
		FROM v1.reactions
		WHERE reactor_id = $1
		  AND reactor_type = 'candidate'
		  AND ($2 = '' OR recommendation_id > $2)
		ORDER BY recommendation_id ASC
		LIMIT $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var results []Reaction
	for rows.Next() {
		var rx Reaction
		if err := rows.Scan(&rx.RecommendationID, &rx.ReactorType, &rx.ReactorID, &rx.ReactionType, &rx.ReactedAt); err != nil {
			return nil, "", err
		}
		results = append(results, rx)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = results[page.Limit-1].RecommendationID
	}

	return results, nextCursor, nil
}

// GetMatchesByCandidateID returns paginated matches for a candidate.
func (s StoreImpl) GetMatchesByCandidateID(candidateID string, page Page) ([]Match, string, error) {
	rows, err := s.Postgres.Query(`
		SELECT m.position_id, p.title, p.description, COALESCE(p.company, ''), m.created_at
		FROM v1.matches m
		JOIN v1.positions p ON p.id = m.position_id
		WHERE m.candidate_id = $1
		  AND ($2 = '' OR m.position_id > $2)
		ORDER BY m.position_id ASC
		LIMIT $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var results []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.PositionID, &m.Title, &m.Description, &m.Company, &m.MatchedAt); err != nil {
			return nil, "", err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = results[page.Limit-1].PositionID
	}

	return results, nextCursor, nil
}

// GetRecruiterByUserID fetches a recruiter by their associated user ID.
func (s StoreImpl) GetRecruiterByUserID(userID string) (*Recruiter, error) {
	var rec Recruiter
	err := s.Postgres.QueryRow(
		`SELECT id, user_id FROM v1.recruiters WHERE user_id = $1 LIMIT 1`,
		userID,
	).Scan(&rec.ID, &rec.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecruiterNotFound
		}
		return nil, err
	}
	return &rec, nil
}
