// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrFailedConnectDB       = errors.New("failed to connect to database")
	ErrFailedPingDB          = errors.New("failed to ping database")
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
	ULID string

	// User represents a system user
	User struct {
		ID             ULID     `json:"id,omitempty"`
		Provider       Provider `json:"provider,omitempty"`
		ProviderUserID string   `json:"provider_user_id,omitempty"`
		Email          string   `json:"email,omitempty"`
		FullName       string   `json:"full_name,omitempty"`
		UserName       string   `json:"user_name"`
	}

	// Candidate represents a candidate profile
	Candidate struct {
		ID     ULID   `json:"id"`
		UserID ULID   `json:"user_id,omitempty"`
		About  string `json:"about"`
	}

	// Recruiter represents a recruiter profile
	Recruiter struct {
		ID     ULID `json:"id"`
		UserID ULID `json:"user_id"`
	}

	Recommendation struct {
		ID          ULID `json:"id"`
		PositionID  ULID `json:"position_id"`
		CandidateID ULID `json:"candidate_id"`
	}

	// Position represents a job position
	Position struct {
		ID          ULID   `json:"id"`
		RecruiterID ULID   `json:"recruiter_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Company     string `json:"company"`
	}

	Match struct {
		PositionID  ULID      `json:"position_id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Company     string    `json:"company"`
		MatchedAt   time.Time `json:"matched_at"`
	}

	// Reaction represents either a candidate or recruiter reaction to a recommendation
	Reaction struct {
		RecommendationID ULID         `json:"recommendation_id"`
		ReactorType      ReactorType  `json:"reactor_type"`
		ReactorID        ULID         `json:"reactor_id"`
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
		RecommendationID ULID   `json:"recommendation_id"`
		PositionID       ULID   `json:"position_id"`
		Title            string `json:"title"`
		Company          string `json:"company"`
		Description      string `json:"description"`
	}

	CandidateRecommendation struct {
		RecommendationID ULID   `json:"recommendation_id"`
		CandidateID      ULID   `json:"candidate_id"`
		UserName         string `json:"username"`
		FullName         string `json:"full_name,omitempty"`
		About            string `json:"about"`
	}
)

type StoreInterface interface {
	CreateCandidate(Candidate) error
	CreateReaction(Reaction) error
	CreateRecommendation(positionID, candidateID ULID) (ULID, error)
	CreateRecruiter(Recruiter) error
	CreateRefreshToken(userID ULID, expiresAt time.Time) (jti ULID, err error)
	CreateUser(User) (userID ULID, err error)
	GetReactionsByCandidateID(ULID, Page) (reactions []Reaction, nextCursor ULID, err error)
	GetMatchesByCandidateID(ULID, Page) (matches []Match, nextCursos ULID, err error)
	GetPosition(ULID) (*Position, error)
	GetUserByProvider(Provider, string) (ULID, map[Role]ULID, error)
	GetRecommendation(ULID) (*Recommendation, error)
	GetUserRoles(ULID, Provider) (map[Role]ULID, error)
	GetPositionRecommendations(candidateID ULID, page Page, excludeReacted bool) (positionRecommendations []PositionRecommendation, nextCursor ULID, err error)
	GetCandidateRecommendations(candidateID ULID, page Page, excludeReacted bool) (candidateRecommenations []CandidateRecommendation, nextCursor ULID, err error)
	IsActiveSession(jti ULID) (bool, error)
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

	db, err := sql.Open("pgx", dbConnString)
	if err != nil {
		return nil, ErrFailedConnectDB
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, ErrFailedPingDB
	}
	return &StoreImpl{Postgres: db}, nil
}

func (s StoreImpl) GetCandidate(id ULID) (*Candidate, error) {
	var c Candidate

	err := s.Postgres.QueryRow(
		`
			select id, about
			from v1.candidates
			where id = $1
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

func (s StoreImpl) GetRecommendation(id ULID) (*Recommendation, error) {
	var r Recommendation

	err := s.Postgres.QueryRow(
		`
			select id, candidate_id, position_id
			from v1.recommendations
			where id = $1
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

func (s StoreImpl) GetPosition(id ULID) (*Position, error) {
	var p Position

	err := s.Postgres.QueryRow(
		`
			select id, recruiter_id, title, description, company
			from v1.positions
			where id = $1
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
func (s StoreImpl) GetCandidateByUserID(userID ULID) (*Candidate, error) {
	var c Candidate
	err := s.Postgres.QueryRow(`
		select id, user_id, about
		from v1.candidates
		where user_id = $1
		limit 1
	`, userID).Scan(&c.ID, &c.UserID, &c.About)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}

	return &c, nil
}

// GetUserByProvider retrieves an existing user and his role based on their provider details.
func (s StoreImpl) GetUserByProvider(provider Provider, providerUserID string) (ULID, map[Role]ULID, error) {
	var userID ULID
	var candidateID, recruiterID sql.NullString
	err := s.Postgres.QueryRow(
		`
			select
					u.id,
					c.id as candidate_id,
					r.id as recruiter_id
			from v1.users u
			left join v1.candidates c on c.user_id = u.id
			left join v1.recruiters r on r.user_id = u.id
			where u.provider = $1
					and u.provider_user_id = $2
    `,
		provider,
		providerUserID,
	).Scan(&userID, &candidateID, &recruiterID)
	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("%w: userID=%s", ErrUserNotFound, userID)
	}
	if err != nil {
		return "", nil, err
	}

	roles := make(map[Role]ULID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ULID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ULID(recruiterID.String)
	}
	if len(roles) == 0 {
		return userID, nil, ErrUserNoRole
	}
	return userID, roles, nil
}

// GetUserRoles fetches user roles by user's ID and provider.
func (s StoreImpl) GetUserRoles(userID ULID, provider Provider) (map[Role]ULID, error) {
	var candidateID, recruiterID sql.NullString
	err := s.Postgres.QueryRow(
		`
			select
					(select c.id from v1.candidates c where c.user_id = u.id) as candidate_id,
					(select r.id from v1.recruiters r where r.user_id = u.id) as recruiter_id
			from v1.users u
			where u.user_id = $1
				and u.provider = $2
		`,
		userID,
		provider,
	).Scan(&candidateID, &recruiterID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: userID=%s", ErrUserNotFound, userID)
	}
	if err != nil {
		return nil, err
	}

	roles := make(map[Role]ULID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ULID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ULID(recruiterID.String)
	}
	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}
	return roles, nil
}

// CreateUser generates a unique username and inserts a new user record.
func (s StoreImpl) CreateUser(u User) (ULID, error) {
	var userID ULID
	err := s.Postgres.QueryRow(
		`
			insert into v1.users (
				provider,
				provider_user_id, 
				email,
				full_name,
				user_name
			)
			values ($1, $2, $3, $4, $5)
			returning id
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
			insert into v1.reactions (
				recommendation_id,
				reactor_type,
				reactor_id,
				reaction_type
			)
			values ($1, $2, $3, $4)
			on conflict (recommendation_id, reactor_type, reactor_id) do nothing
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
			insert into v1.candidates (
				user_id,
				about
			)
			values ($1, $2)
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
			insert into v1.recruiters (
				user_id
			)
			values ($1)
		`,
		r.UserID,
	)
	return err
}

// IsActiveSession checks if the JTI exists and is not expired.
func (s StoreImpl) IsActiveSession(jti ULID) (bool, error) {
	var isActive bool
	return isActive, s.Postgres.QueryRow(
		`
			select revoked 
			from v1.refresh_tokens 
			where jti = $1 
			and expires_at > now()
		`,
		jti,
	).Scan(&isActive)
}

// CreateRefreshToken creates a new refresh token record.
func (s StoreImpl) CreateRefreshToken(userID ULID, expiresAt time.Time) (jti ULID, err error) {
	err = s.Postgres.QueryRow(
		`
		insert into v1.refresh_tokens (
			user_id,
			expires_at
		)
		values ($1, $2) 
		returning jti
		`,
		userID,
		expiresAt,
	).Scan(&jti)
	return jti, err
}

// CreateRecommendation inserts a new recommendation for a candidate and a position.
func (s StoreImpl) CreateRecommendation(positionID, candidateID ULID) (ULID, error) {
	var recID ULID
	if err := s.Postgres.QueryRow(`
		insert into v1.recommendations (position_id, candidate_id)
		values ($1, $2)
		on conflict (position_id, candidate_id) do nothing
		returning id
	`, positionID, candidateID).Scan(&recID); err != nil {
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
func (s StoreImpl) GetPositionRecommendations(candidateID ULID, page Page, excludeReacted bool) (positionRecommendations []PositionRecommendation, nextCursor ULID, err error) {
	rows, err := s.Postgres.Query(`
		select r.id, p.id, p.title, p.company, p.description
		from v1.recommendations r
		join v1.positions p on p.id = r.position_id
		left join v1.reactions rx on rx.recommendation_id = r.id
				and rx.reactor_type = 'candidate'
				and rx.reactor_id = $1
		where r.candidate_id = $1
				and ($2 = '' or r.id > $2)
				and (not $4 or rx.recommendation_id is null)
		order by r.id asc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1, excludeReacted)
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

	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].RecommendationID)
	}

	return results, nextCursor, nil
}

func (s StoreImpl) GetCandidateRecommendations(recruiterID ULID, page Page, excludeReacted bool) ([]CandidateRecommendation, ULID, error) {
	rows, err := s.Postgres.Query(`
		select r.id, c.id, u.full_name, c.about
		from v1.recommendations r
		join v1.positions p on p.id = r.position_id
		join v1.candidates c on c.id = r.candidate_id
		join v1.users u on u.id = c.user_id
		left join v1.reactions rx on rx.recommendation_id = r.id
				and rx.reactor_type = 'recruiter'
				and rx.reactor_id = $1
		where p.recruiter_id = $1
				and ($2 = '' or r.id > $2)
				and (not $4 or rx.recommendation_id is null)
		order by r.id asc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1, excludeReacted)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var results []CandidateRecommendation
	for rows.Next() {
		var cr CandidateRecommendation
		if err := rows.Scan(&cr.RecommendationID, &cr.CandidateID, &cr.FullName, &cr.About); err != nil {
			return nil, "", err
		}
		results = append(results, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor ULID
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].RecommendationID)
	}

	return results, nextCursor, nil
}

// GetReactionsByCandidateID returns paginated reactions made by a candidate.
func (s StoreImpl) GetReactionsByCandidateID(candidateID ULID, page Page) (reactions []Reaction, nextCursor ULID, err error) {
	rows, err := s.Postgres.Query(`
		select recommendation_id, reactor_type, reactor_id, reaction_type, created_at
		from v1.reactions
		where reactor_id = $1
		  and reactor_type = 'candidate'
		  and ($2 = '' or recommendation_id > $2)
		order by recommendation_id asc
		limit $3
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

	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].RecommendationID)
	}

	return results, nextCursor, nil
}

// GetMatchesByCandidateID returns paginated matches for a candidate.
func (s StoreImpl) GetMatchesByCandidateID(candidateID ULID, page Page) (matches []Match, nextCursor ULID, err error) {
	rows, err := s.Postgres.Query(`
		select m.position_id, p.title, p.description, coalesce(p.company, ''), m.created_at
		from v1.matches m
		join v1.positions p on p.id = m.position_id
		where m.candidate_id = $1
		  and ($2 = '' or m.position_id > $2)
		order by m.position_id asc
		limit $3
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

	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].PositionID)
	}

	return results, nextCursor, nil
}

// GetRecruiterByUserID fetches a recruiter by their associated user ID.
func (s StoreImpl) GetRecruiterByUserID(userID ULID) (*Recruiter, error) {
	var rec Recruiter
	err := s.Postgres.QueryRow(
		`
			select id, user_id 
			from v1.recruiters 
			where user_id = $1 limit 1
		`,
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
