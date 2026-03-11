// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type (
	Store interface {
		CreateCandidate(Candidate) error
		CreateMatch(Match) error
		CreateReaction(Reaction) error
		CreateRecommendation(positionID, candidateID string) (recID string, err error)
		CreateRecruiter(Recruiter) error
		CreateRefreshToken(userID string, expiresAt time.Time) (jti string, err error)
		CreateUser(User) (userID string, err error)
		GetCandidate(id string) (*Candidate, error)
		GetCandidateByUserID(userID string) (Candidate, error)
		GetCandidates(Pagination) (PagedResult[Candidate], error)
		GetPosition(id string) (*Position, error)
		GetPositions(Pagination) (PagedResult[Position], error)
		GetRecommendations(userID string, p Pagination, includePositions bool, includeCandidates bool) (PagedResult[Recommendation], error)
		GetUserByProvider(provider Provider, providerUserID string) (userID string, roles []string, err error)
		GetUserRoles(userID string, provider Provider) (roles []string, err error)
		ValidateActiveSession(jti string) (isSessionRevoked bool, err error)
	}

	PostgresStore struct {
		Postgres *sql.DB
	}

	PagedResult[T any] struct {
		Items   []T
		HasPrev bool
		HasNext bool
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
		return nil, ErrFailedConnectDB
	}
	return &PostgresStore{Postgres: database}, nil
}

func (s PostgresStore) GetPositions(p Pagination) (PagedResult[Position], error) {
	baseQuery := `
		SELECT id, title, description, company
		FROM v1.positions
		WHERE 1=1
	`

	result, err := queryWithPagination[Position](s, baseQuery, "id", []any{}, p, func(rows *sql.Rows) (Position, error) {
		var pos Position
		if err := rows.Scan(&pos.ID, &pos.Title, &pos.Description, &pos.Company); err != nil {
			return Position{}, err
		}
		return pos, nil
	})
	if err != nil {
		return PagedResult[Position]{nil, false, false}, err
	}

	return PagedResult[Position]{result.Items, result.HasPrev, result.HasNext}, nil
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

func (s PostgresStore) GetPosition(id string) (*Position, error) {
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
func (s PostgresStore) GetCandidateByUserID(userID string) (Candidate, error) {
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
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, err
	}

	return c, nil
}

func (s PostgresStore) GetCandidates(p Pagination) (PagedResult[Candidate], error) {
	baseQuery := `
		SELECT id, about
		FROM v1.candidates
		WHERE 1=1
	`

	result, err := queryWithPagination(s, baseQuery, "id", []any{}, p, func(rows *sql.Rows) (Candidate, error) {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.About); err != nil {
			return Candidate{}, err
		}
		return c, nil
	})
	if err != nil {
		return PagedResult[Candidate]{nil, false, false}, err
	}

	return PagedResult[Candidate]{result.Items, result.HasPrev, result.HasNext}, nil
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
		return "", nil, fmt.Errorf("%w: providerUserID=%s", ErrUserNotFound, providerUserID)
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
		return userID, nil, ErrUserNoRole
	}

	return userID, roles, nil
}

// GetUserRoles fetches user roles by user's ID and provider.
func (s PostgresStore) GetUserRoles(userID string, provider Provider) (roles []string, err error) {
	var isCandidate, isRecruiter, isAdmin bool

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
				EXISTS (
						SELECT 1 FROM v1.admins r WHERE r.user_id = u.id
				) AS is_admin
		FROM v1.users u
		WHERE u.user_id = $1
			AND u.provider = $2
		`,
		userID,
		provider,
	).Scan(&isCandidate, &isRecruiter, &isAdmin)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: userID=%s", ErrUserNotFound, userID)
	}
	if err != nil {
		return nil, err
	}

	if isCandidate {
		roles = append(roles, "candidate")
	}
	if isRecruiter {
		roles = append(roles, "recruiter")
	}
	if isAdmin {
		roles = append(roles, "admin")
	}

	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}

	return roles, nil
}

// CreateUser generates a unique username and inserts a new user record.
func (s PostgresStore) CreateUser(u User) (userID string, err error) {
	if u.FullName == "" {
		return "", ErrFullNameRequired
	}

	if u.UserName == "" {
		return "", ErrUserNameRequired
	}

	suffix := make([]byte, 2)
	_, err = rand.Read(suffix)
	if err != nil {
		return "", ErrFailedGenerateUsernameSuffix
	}

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
func (s PostgresStore) CreateReaction(r Reaction) error {
	_, err := s.Postgres.Exec(
		`
		INSERT INTO v1.reactions (
			recommendation_id,
			reactor_type,
			reactor_id,
			reaction_type
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (recommendation_id, reactor_type, reactor_id) DO UPDATE
		SET reaction_type = EXCLUDED.reaction_type,
		    created_at = NOW()
		`,
		r.RecommendationID,
		r.ReactorType,
		r.ReactorID,
		r.ReactionType,
	)
	return err
}

// CreateCandidate creates a candidate
func (s PostgresStore) CreateCandidate(c Candidate) error {
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
func (s PostgresStore) CreateRecruiter(r Recruiter) error {
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

// CreateRecommendation inserts a new recommendation for a candidate and a position.
func (s PostgresStore) CreateRecommendation(positionID, candidateID string) (recID string, err error) {
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

	// If recommendationID is empty, the insert was skipped due to conflict
	if recID == "" {
		return "", ErrRecommendationExists
	}

	return recID, nil
}

// GetRecommendations gets aggregated recommendations for candidate account and recruiter account of the user with the provided userID.
func (s PostgresStore) GetRecommendations(
	userID string,
	p Pagination,
	includePositions bool,
	includeCandidates bool,
) (PagedResult[Recommendation], error) {
	candidate, err := s.GetCandidateByUserID(userID)
	if err != nil {
		return PagedResult[Recommendation]{nil, false, false}, fmt.Errorf("failed to get candidate: %w", err)
	}

	baseQuery := `
		SELECT r.id, r.candidate_id, r.position_id,
		       p.id, p.recruiter_id, p.title, p.description, p.company
		FROM v1.recommendations r
		JOIN v1.positions p ON p.id = r.position_id
		LEFT JOIN v1.reactions react ON react.recommendation_id = r.id
		WHERE r.candidate_id = $1 AND react.recommendation_id IS NULL
	`

	result, err := queryWithPagination(s, baseQuery, "r.id", []any{candidate.ID}, p, func(rows *sql.Rows) (Recommendation, error) {
		var r Recommendation
		var pos Position
		if err := rows.Scan(&r.ID, &r.CandidateID, &r.PositionID, &pos.ID, &pos.RecruiterID, &pos.Title, &pos.Description, &pos.Company); err != nil {
			return Recommendation{}, err
		}
		r.Position = &pos
		return r, nil
	})
	if err != nil {
		return PagedResult[Recommendation]{nil, false, false}, err
	}

	return PagedResult[Recommendation]{result.Items, result.HasPrev, result.HasNext}, nil
}

// queryWithPagination executes a paginated query.
func queryWithPagination[T any](
	s PostgresStore,
	baseQuery string,
	cursorCol string,
	args []any,
	p Pagination,
	scanFunc func(*sql.Rows) (T, error),
) (PagedResult[T], error) {
	result := PagedResult[T]{}
	argPos := len(args) + 1
	order := "ASC"

	if p.After != nil {
		baseQuery += fmt.Sprintf(" AND %s > $%d", cursorCol, argPos)
		args = append(args, *p.After)
		argPos++
		order = "ASC"
	}
	if p.Before != nil {
		baseQuery += fmt.Sprintf(" AND %s < $%d", cursorCol, argPos)
		args = append(args, *p.Before)
		argPos++
		order = "DESC"
	}

	baseQuery += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d", cursorCol, order, argPos)
	args = append(args, p.Limit+1)

	rows, err := s.Postgres.Query(baseQuery, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	items := make([]T, 0, p.Limit+1)
	for rows.Next() {
		item, err := scanFunc(rows)
		if err != nil {
			return result, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	hasMore := uint64(len(items)) > p.Limit
	if hasMore {
		items = items[:p.Limit]
	}

	if p.Before != nil {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	switch {
	case p.After != nil:
		result.HasPrev = true
		result.HasNext = hasMore
	case p.Before != nil:
		result.HasPrev = hasMore
		result.HasNext = true
	default:
		result.HasPrev = false
		result.HasNext = hasMore
	}

	result.Items = items
	return result, nil
}
