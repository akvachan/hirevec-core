// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strconv"
	"strings"
	"time"
	"uuid"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type ID string

func NewID() ID {
	return ID(uuid.NewV7().String())
}

type DatabaseProvider string

const (
	DatabaseProviderPostgreSQL DatabaseProvider = "PostgreSQL"
	DatabaseProviderSQLite     DatabaseProvider = "SQLite"
)

type StoreConfig struct {
	DatabaseProvider      DatabaseProvider
	PostgreSQLDatabaseURL string
	DevMode               bool
}

func ExecMigration(db *sql.DB, path string) error {
	sql, err := EmbeddedStatic.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read embedded file %s: %w", path, err)
	}

	if _, err := db.Exec(string(sql)); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

var ErrMissingDatabaseURL = errors.New("database URL is not set")

func ConnectPostgreSQL(url string) (*sql.DB, error) {
	slog.Debug("connecting to database", "database", "PostgreSQL")
	if url == "" {
		return nil, ErrMissingDatabaseURL
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

var DefaultSQLiteConn = "file:bin/.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

func ConnectSQLite() (*sql.DB, error) {
	slog.Debug("connecting to database", "database", "SQLite")
	db, err := sql.Open("sqlite", DefaultSQLiteConn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	var enabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to scan: %w", err)
	}
	slog.Debug("foreign keys", "enabled", enabled, "database", "SQLite")

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

var (
	PathInitMigration          = path.Join("migrations/init.sql")
	PathEmbeddingsMigration    = path.Join("migrations/embeddings.sql")
	PathPostgreSQLFTSMigration = path.Join("migrations/postgresql-fts.sql")
	PathSQLiteFTSMigration     = path.Join("migrations/sqlite-fts.sql")
	PathDevIngestMigration     = path.Join("migrations/dev-ingest.sql")
)

func InitPostgreSQL(url string) (*sql.DB, error) {
	db, err := ConnectPostgreSQL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Debug("creating database schema", "database", "PostgreSQL")
	if err := ExecMigration(db, PathInitMigration); err != nil {
		return nil, fmt.Errorf("failed to execute migration %s: %w", PathInitMigration, err)
	}

	slog.Debug("creating embeddings tables", "database", "PostgreSQL")
	if err := ExecMigration(db, PathEmbeddingsMigration); err != nil {
		return nil, fmt.Errorf("failed to execute migration %s: %w", PathEmbeddingsMigration, err)
	}

	slog.Debug("initializing FTS", "database", "PostgreSQL")
	if err := ExecMigration(db, PathPostgreSQLFTSMigration); err != nil {
		return nil, fmt.Errorf("failed to execute migration %s: %w", PathPostgreSQLFTSMigration, err)
	}

	return db, nil
}

func InitSQLite() (*sql.DB, error) {
	db, err := ConnectSQLite()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Debug("creating database schema", "database", "SQLite")
	if err := ExecMigration(db, PathInitMigration); err != nil {
		return nil, fmt.Errorf("failed to execute migration %s: %w", PathInitMigration, err)
	}

	slog.Debug("initializing FTS", "database", "SQLite")
	if err := ExecMigration(db, PathSQLiteFTSMigration); err != nil {
		return nil, fmt.Errorf("failed to execute migration %s: %w", PathPostgreSQLFTSMigration, err)
	}

	return db, nil
}

type Store struct {
	DatabaseProvider DatabaseProvider
	DB               *sql.DB
}

var ErrUnsupportedDatabaseProvider = errors.New("unsupported database provider")

func NewStore(c StoreConfig) (Store, error) {
	s := Store{}

	var err error
	if c.DatabaseProvider == DatabaseProviderPostgreSQL {
		slog.Debug("initializing store", "database", "PostgreSQL")
		if s.DB, err = InitPostgreSQL(c.PostgreSQLDatabaseURL); err != nil {
			return s, fmt.Errorf("failed to initialize database: %w", err)
		}

	} else if c.DatabaseProvider == DatabaseProviderSQLite {
		slog.Debug("initializing store", "database", "SQLite")
		if s.DB, err = InitSQLite(); err != nil {
			return s, fmt.Errorf("failed to initialize database: %w", err)
		}

	} else {
		return s, ErrUnsupportedDatabaseProvider
	}
	s.DatabaseProvider = c.DatabaseProvider

	if c.DevMode {
		slog.Debug("dev mode is enabled, ingesting demo data")
		if err := ExecMigration(s.DB, PathDevIngestMigration); err != nil {
			return s, fmt.Errorf("failed to execute migration %s: %w", PathDevIngestMigration, err)
		}
	}

	return s, nil
}

var ErrRecommendationNotFound = errors.New("recommendation not found")

func (s Store) GetRecommendation(recommendationID ID) (Recommendation, error) {
	var candidateID, positionID ID

	if err := s.DB.QueryRow(`
		select candidate_id, position_id
		from recommendations
		where id = $1
	`, recommendationID).Scan(
		&candidateID,
		&positionID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recommendation{}, ErrRecommendationNotFound
		}
		return Recommendation{}, fmt.Errorf("failed to scan: %w", err)
	}

	return Recommendation{
		ID:          recommendationID,
		PositionID:  positionID,
		CandidateID: candidateID,
	}, nil
}

var (
	ErrUserNoRole   = errors.New("user has no role")
	ErrUserNotFound = errors.New("user not found")
)

func (s Store) GetUserAndRolesByEmail(email string, provider Provider) (User, map[Role]ID, error) {
	var userID ID
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash string
	var candidateID, recruiterID sql.NullString

	if err := s.DB.QueryRow(`
		select
			u.id,
			u.provider_user_id,
			u.full_name,
			u.user_name,
			u.password_hash,
			u.updated_at,
			c.id as candidate_id,
			r.id as recruiter_id
		from users u
		left join candidates c on c.user_id = u.id
		left join recruiters r on r.user_id = u.id
		where u.email = $1 and u.provider = $2
	`, email, provider).Scan(
		&userID,
		&optionalProviderUserID,
		&fullName,
		&userName,
		&passwordHash,
		&updatedAt,
		&candidateID,
		&recruiterID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, nil, ErrUserNotFound
		}
		return User{}, nil, fmt.Errorf("failed to scan: %w", err)
	}

	if optionalProviderUserID.Valid {
		providerUserID = optionalProviderUserID.String
	}

	user := User{
		userID,
		provider,
		providerUserID,
		email,
		fullName,
		userName,
		passwordHash,
		updatedAt,
	}

	roles := make(map[Role]ID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ID(recruiterID.String)
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUserIDAndRolesByProvider(provider Provider, providerUserID string) (ID, map[Role]ID, error) {
	var userID ID
	var candidateID, recruiterID sql.NullString

	if err := s.DB.QueryRow(`
		select
			u.id, 
			c.id as candidate_id,
			r.id as recruiter_id
		from users u
		left join candidates c on c.user_id = u.id
		left join recruiters r on r.user_id = u.id
		where u.provider = $1 and u.provider_user_id = $2
   `, provider, providerUserID).Scan(
		&userID,
		&candidateID,
		&recruiterID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, ErrUserNotFound
		}
		return "", nil, fmt.Errorf("failed to scan: %w", err)
	}

	roles := make(map[Role]ID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ID(recruiterID.String)
	}
	if len(roles) == 0 {
		return userID, nil, ErrUserNoRole
	}

	return userID, roles, nil
}

func (s Store) GetUserRoles(userID ID, provider Provider) (map[Role]ID, error) {
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
		select
				(select c.id from candidates c where c.user_id = u.id) as candidate_id,
				(select r.id from recruiters r where r.user_id = u.id) as recruiter_id
		from users u
		where u.id = $1 and u.provider = $2
	`, userID, provider).Scan(
		&candidateID,
		&recruiterID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to scan: %w", err)
	}

	roles := make(map[Role]ID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ID(recruiterID.String)
	}
	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}

	return roles, nil
}

type User struct {
	ID             ID        `json:"id"`
	Provider       Provider  `json:"-"`
	ProviderUserID string    `json:"-"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	UserName       string    `json:"user_name"`
	PasswordHash   string    `json:"-"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func CurrentTimestamp(offset ...time.Duration) string {
	t := time.Now()
	if len(offset) > 0 {
		t = t.Add(offset[0])
	}
	return t.UTC().Format(time.RFC3339)
}

var ErrUserAlreadyExists = errors.New("user already exists")

func (s Store) CreateUser(provider Provider, providerUserID string, email string, fullName string, userName string, passwordHash string) (ID, error) {
	id := NewID()

	result, err := s.DB.Exec(
		`
		insert into users (
			id,
			provider,
			provider_user_id,
		  email,
		  full_name,
		  user_name,
			password_hash,
			updated_at
		)
		values ($1, $2, nullif($3, ''), $4, $5, $6, $7, $8)
		on conflict (provider, provider_user_id) do nothing
	`, id, provider, providerUserID, email, fullName, userName, passwordHash, CurrentTimestamp())
	if err != nil {
		return "", fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return "", ErrUserAlreadyExists
	}

	return id, nil
}

type ReactionType string

const (
	ReactionTypePositive ReactionType = "positive"
	ReactionTypeNegative ReactionType = "negative"
	ReactionTypeNeutral  ReactionType = "neutral"
)

func (r ReactionType) IsValid() bool {
	return r == ReactionTypePositive ||
		r == ReactionTypeNegative ||
		r == ReactionTypeNeutral
}

type ReactorType string

const (
	ReactorTypeCandidate ReactorType = "candidate"
	ReactorTypeRecruiter ReactorType = "recruiter"
)

func (r ReactorType) IsValid() bool {
	return r == ReactorTypeCandidate ||
		r == ReactorTypeRecruiter
}

type Reaction struct {
	RecommendationID ID           `json:"recommendation_id"`
	ReactionType     ReactionType `json:"reaction_type"`
	ReactedAt        time.Time    `json:"reacted_at"`
}

var (
	ErrReactionAlreadyExists = errors.New("reaction already exists")
	ErrUnauthorizedReactor   = errors.New("reactor with the reactorID is unauthorized")
)

func (s Store) CreateReaction(recommendationID ID, reactorType ReactorType, reactorID ID, reactionType ReactionType) error {
	result, err := s.DB.Exec(`
		insert into reactions (
			recommendation_id, 
			reactor_type,
			reactor_id,
			reaction_type,
			created_at
		)
		select $1, $2, $3, $4, $5
		from recommendations rec
		join positions pos on rec.position_id = pos.id
		where rec.id = $1
		  and (
		      ($2 = 'candidate' and rec.candidate_id = $3)
		      or 
		      ($2 = 'recruiter' and pos.recruiter_id = $3)
		  )
		on conflict (recommendation_id, reactor_type, reactor_id) do nothing
	`, recommendationID, reactorType, reactorID, reactionType, CurrentTimestamp())
	if err != nil {
		return fmt.Errorf("failed to execute insert query: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 1 {
		return nil
	}

	var recExists, isAuthorized, reactionExists bool
	if err = s.DB.QueryRow(`
		select 
			exists (select 1 from recommendations where id = $1),
			exists (
				select 1 from recommendations rec
				join positions pos on rec.position_id = pos.id
				where rec.id = $1
				  and (
				      ($2 = 'candidate' and rec.candidate_id = $3)
				      or 
				      ($2 = 'recruiter' and pos.recruiter_id = $3)
				  )
			),
			exists (select 1 from reactions where recommendation_id = $1 and reactor_type = $2 and reactor_id = $3)
	`, recommendationID, reactorType, reactorID).Scan(&recExists, &isAuthorized, &reactionExists); err != nil {
		return fmt.Errorf("failed to run diagnostic query: %w", err)
	}

	if reactionExists {
		return ErrReactionAlreadyExists
	}
	if !recExists {
		return ErrRecommendationNotFound
	}
	if !isAuthorized {
		return ErrUnauthorizedReactor
	}

	return fmt.Errorf("failed to create reaction for an unknown reason")
}

func (s Store) IsRevokedRefreshToken(jti ID) (bool, error) {
	var isRevoked bool

	if err := s.DB.QueryRow(`
		select revoked 
		from refresh_tokens 
		where jti = $1 
		and expires_at > $2 
	`, jti, time.Now().UTC()).Scan(&isRevoked); err != nil {
		return true, fmt.Errorf("failed to scan: %w", err)
	}

	return isRevoked, nil
}

const DefaultMaxRefreshTokensCount = 5

func (s Store) CreateRefreshToken(userID ID) (jti ID, err error) {
	jti = NewID()

	tx, err := s.DB.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err = tx.Rollback(); err != nil {
				slog.Debug("failed to rollback transaction", "err", err)
			}
		}
	}()

	currentTimestamp := CurrentTimestamp()

	var count int
	if err = tx.QueryRow(`
		select count(*)
		from refresh_tokens
		where user_id = $1
		  and revoked = false
		  and expires_at > $2
	`, userID, currentTimestamp).Scan(&count); err != nil {
		return "", fmt.Errorf("failed to scan: %w", err)
	}
	if count >= DefaultMaxRefreshTokensCount {
		if _, err = tx.Exec(`
			update refresh_tokens
			set revoked = true
			where jti in (
				select jti
				from refresh_tokens
				where user_id = $1
				  and revoked = false
				  and expires_at > $2
				order by created_at desc
				limit 1
			)
		`, userID, currentTimestamp); err != nil {
			return "", fmt.Errorf("failed to execute SQL: %w", err)
		}
	}

	if _, err = tx.Exec(`
		insert into refresh_tokens (jti, user_id, created_at, expires_at)
		values ($1, $2, $3, $4)
	`, jti, userID, currentTimestamp, CurrentTimestamp(DefaultRefreshTokenExpiration)); err != nil {
		return "", fmt.Errorf("failed to execute SQL: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return jti, nil
}

type Recommendation struct {
	ID          ID `json:"id"`
	PositionID  ID `json:"position_id"`
	CandidateID ID `json:"candidate_id"`
}

var ErrRecommendationAlreadyExists = errors.New("recommendation already exists")

func (s Store) CreateRecommendation(positionID ID, candidateID ID) (ID, error) {
	id := NewID()

	result, err := s.DB.Exec(`
		insert into recommendations (recommendation_id, position_id, candidate_id)
		values ($1, $2, $3)
	`, id, positionID, candidateID)
	if err != nil {
		return "", fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return "", ErrRecommendationAlreadyExists
	}

	return id, nil
}

type Page struct {
	Cursor  string `json:"cursor,omitempty"`
	Limit   int    `json:"limit"`
	Count   int    `json:"count"`
	HasNext bool   `json:"has_next"`
}

type RecommendationForCandidate struct {
	RecommendationID    ID     `json:"recommendation_id"`
	PositionID          ID     `json:"position_id"`
	PositionTitle       string `json:"position_title"`
	PositionCompany     string `json:"position_company"`
	PositionDescription string `json:"position_description"`
}

func (s Store) GetRecommendationsForCandidate(candidateID ID, page Page, excludeReacted bool) ([]RecommendationForCandidate, Page, error) {
	rows, err := s.DB.Query(`
		select r.id, p.id, p.title, p.company, p.description
		from recommendations r
		join positions p on p.id = r.position_id
		left join reactions rx on rx.recommendation_id = r.id
				and rx.reactor_type = 'candidate'
				and rx.reactor_id = $1
		where r.candidate_id = $1
				and ($2 = '' or r.id > $2)
				and (not $4 or rx.recommendation_id is null)
		order by r.id desc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1, excludeReacted)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	recommendations := make([]RecommendationForCandidate, 0, page.Limit)
	for rows.Next() {
		var recommendation RecommendationForCandidate
		if err := rows.Scan(
			&recommendation.RecommendationID,
			&recommendation.PositionID,
			&recommendation.PositionTitle,
			&recommendation.PositionCompany,
			&recommendation.PositionDescription,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		recommendations = append(recommendations, recommendation)
	}

	hasNext := len(recommendations) > page.Limit
	var nextCursor ID
	if hasNext {
		nextCursor = recommendations[page.Limit-1].RecommendationID
		recommendations = recommendations[:page.Limit]
	}

	return recommendations, Page{
		Cursor:  string(nextCursor),
		Limit:   page.Limit,
		Count:   len(recommendations),
		HasNext: hasNext,
	}, nil
}

type RecommendationForRecruiter struct {
	RecommendationID  ID     `json:"recommendation_id"`
	PositionID        ID     `json:"position_id"`
	PositionTitle     string `json:"position_title"`
	CandidateID       ID     `json:"candidate_id"`
	CandidateFullName string `json:"candidate_full_name"`
	CandidateAbout    string `json:"candidate_about"`
}

func (s Store) GetRecommendationsForRecruiter(recruiterID ID, page Page, includeReacted bool) ([]RecommendationForRecruiter, Page, error) {
	rows, err := s.DB.Query(`
		select r.id,  p.id, p.title, c.id, u.full_name, c.about,
		from recommendations r
		join positions p on p.id = r.position_id
		join candidates c on c.id = r.candidate_id
		join users u on u.id = c.user_id
		left join reactions rx on rx.recommendation_id = r.id
				and rx.reactor_type = 'recruiter'
				and rx.reactor_id = $1
		where p.recruiter_id = $1
				and ($2 = '' or r.id > $2)
				and ($4 or rx.recommendation_id is null)
		order by r.id desc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1, includeReacted)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	recommendations := make([]RecommendationForRecruiter, 0, page.Limit)
	for rows.Next() {
		var recommendation RecommendationForRecruiter
		if err := rows.Scan(
			&recommendation.RecommendationID,
			&recommendation.PositionID,
			&recommendation.PositionTitle,
			&recommendation.CandidateID,
			&recommendation.CandidateFullName,
			&recommendation.CandidateAbout,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		recommendations = append(recommendations, recommendation)
	}

	hasNext := len(recommendations) > page.Limit
	var nextCursor ID
	if hasNext {
		nextCursor = recommendations[page.Limit-1].RecommendationID
		recommendations = recommendations[:page.Limit]
	}

	return recommendations, Page{
		Cursor:  string(nextCursor),
		Limit:   page.Limit,
		Count:   len(recommendations),
		HasNext: hasNext,
	}, nil
}

func (s Store) GetReactionsForRecruiter(recruiterID ID, page Page) ([]Reaction, Page, error) {
	rows, err := s.DB.Query(`
		select recommendation_id, reaction_type, created_at
		from recruiter_reactions
		where reactor_id = $1
		  and ($2 = '' or recommendation_id > $2)
		order by recommendation_id desc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]Reaction, 0, page.Limit)
	for rows.Next() {
		var rx Reaction
		if err := rows.Scan(
			&rx.RecommendationID,
			&rx.ReactionType,
			&rx.ReactedAt,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, rx)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = string(results[page.Limit-1].RecommendationID)
	}

	return results, nextPage, nil
}

func (s Store) GetReactionsForCandidate(candidateID ID, page Page) ([]Reaction, Page, error) {
	rows, err := s.DB.Query(`
		select recommendation_id, reaction_type, created_at
		from candidate_reactions
		where candidate_id = $1
		  and ($2 = '' or recommendation_id > $2)
		order by recommendation_id desc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]Reaction, 0, page.Limit)
	for rows.Next() {
		var rx Reaction
		if err := rows.Scan(
			&rx.RecommendationID,
			&rx.ReactionType,
			&rx.ReactedAt,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, rx)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = string(results[page.Limit-1].RecommendationID)
	}

	return results, nextPage, nil
}

type MatchForCandidate struct {
	PositionID  ID        `json:"position_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Company     string    `json:"company"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s Store) GetMatchesForCandidate(candidateID ID, page Page) ([]MatchForCandidate, Page, error) {
	rows, err := s.DB.Query(`
		select m.position_id, p.title, p.description, coalesce(p.company, ''), m.created_at
		from matches m
		join positions p on p.id = m.position_id
		where m.candidate_id = $1
		  and ($2 = '' or m.position_id > $2)
		order by m.position_id desc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]MatchForCandidate, 0, page.Limit)
	for rows.Next() {
		var m MatchForCandidate
		if err := rows.Scan(
			&m.PositionID,
			&m.Title,
			&m.Description,
			&m.Company,
			&m.CreatedAt,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, m)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = string(results[page.Limit-1].PositionID)
	}

	return results, nextPage, nil
}

type MatchForRecruiter struct {
	PositionID  ID        `json:"position_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Company     string    `json:"company"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s Store) GetMatchesForRecruiter(recruiterID ID, page Page) ([]MatchForRecruiter, Page, error) {
	rows, err := s.DB.Query(`
		select m.position_id, p.title, p.description, coalesce(p.company, ''), m.created_at
		from matches m
		join positions p on p.id = m.position_id
		where m.candidate_id = $1
		  and ($2 = '' or m.position_id > $2)
		order by m.position_id desc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]MatchForRecruiter, 0, page.Limit)
	for rows.Next() {
		var m MatchForRecruiter
		if err := rows.Scan(
			&m.PositionID,
			&m.Title,
			&m.Description,
			&m.Company,
			&m.CreatedAt,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, m)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = string(results[page.Limit-1].PositionID)
	}

	return results, nextPage, nil
}

type EmbeddingStatus string

const (
	EmbeddingStatusPending = "pending"
	EmbeddingStatusDone    = "done"
	EmbeddingStatusFailed  = "failed"
)

func (s Store) FetchPendingEmbeddingsMetadata(limit uint16) ([]ID, []string, error) {
	rows, err := s.DB.Query(`
		select entity_id, aggregated_info
		from embeddings_metadata
		where embedding_status in ('pending', 'failed')
		order by embedding_updated_at desc
		limit $1
  `, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	ids := make([]ID, 0, limit)
	texts := make([]string, 0, limit)
	for rows.Next() {
		var id ID
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, nil, fmt.Errorf("failed to scan: %w", err)
		}
		ids = append(ids, id)
		texts = append(texts, text)
	}

	return ids, texts, nil
}

func SqlIn(column string, n int) string {
	if n <= 0 {
		return "1=0"
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Repeat("?,", n-1)+"?")
}

func (s Store) MarkEmbeddingsStatus(entityIDs []ID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, status)
	for _, id := range entityIDs {
		args = append(args, id)
	}

	if _, err := s.DB.Exec(fmt.Sprintf(`
		update embeddings_metadata
		set embedding_status = ?
		where %s
	`, SqlIn("id", len(entityIDs))), args...); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) MarkEmbeddingsStatusTx(tx *sql.Tx, entityIDs []ID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, status)
	for _, id := range entityIDs {
		args = append(args, id)
	}

	if _, err := tx.Exec(fmt.Sprintf(`
		update embeddings_metadata
		set embedding_status = ?
		where %s
	`, SqlIn("id", len(entityIDs))), args...); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) GetPositionsForCandidateViaEmbeddings(candidateID ID, topPositions uint16) ([]ID, error) {
	rows, err := s.DB.Query(`
		with candidate as (
				select e.embedding
				from embeddings_metadata m
				join embeddings e
						on e.rowid = m.rowid
				where m.entity_id = $1
						and m.entity_type = 'candidate'
						and m.embedding_status = 'done'
				limit 1
		)
		select p.id as position_id
		from positions p
		join embeddings_metadata pe
				on pe.entity_id = p.id
			 and pe.entity_type = 'position'
		join embeddings e
				on e.rowid = pe.rowid
		cross join candidate c
		where p.is_active = 1
				and pe.embedding_status = 'done'
				and not exists (
						select 1
						from recommendations r
						where r.position_id = p.id
							and r.candidate_id = $1
				)
		order by e.embedding <=> c.embedding
		limit $2
	`, candidateID, topPositions)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]ID, 0, topPositions)
	for rows.Next() {
		var positionID ID
		if err := rows.Scan(&positionID); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, positionID)
	}

	return results, nil
}

var ErrEmbeddingsCountConflict = errors.New("mismatch between count of embedding IDs and embeddings")

func (s Store) UpsertEmbeddingsTx(tx *sql.Tx, embeddingIDs []ID, embeddings []EmbeddingEntity) error {
	if len(embeddingIDs) == 0 {
		return nil
	}
	if len(embeddingIDs) != len(embeddings) {
		return ErrEmbeddingsCountConflict
	}

	insertTx, err := tx.Prepare(`
		insert or replace into embeddings (id, embedding)
		values ($1, $2)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare transaction: %w", err)
	}
	defer func() {
		if err := insertTx.Close(); err != nil {
			slog.Error("failed to close insert transaction", "err", err)
		}
	}()

	for i := range embeddingIDs {
		if _, err := insertTx.Exec(
			embeddingIDs[i],
			embeddings[i].Embedding,
		); err != nil {
			return fmt.Errorf("failed to execute transaction: %w", err)
		}
	}

	return nil
}

func (s Store) GetCandidates(limit uint16, recommendationSpan time.Duration) ([]ID, error) {
	cutoff := CurrentTimestamp(-recommendationSpan)
	rows, err := s.DB.Query(`
		select id
		from candidates
		where last_recommended_at <= $1
		   or last_recommended_at is null
		order by last_recommended_at asc
		limit $2
	`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	ids := make([]ID, 0, limit)
	for rows.Next() {
		var id ID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

type Recruiter struct {
	ID     ID `json:"id"`
	UserID ID `json:"user_id"`
}

var ErrRecruiterAlreadyExists = errors.New("recruiter already exists")

func (s Store) CreateRecruiter(userID ID) (ID, error) {
	id := NewID()

	result, err := s.DB.Exec(`
		insert into recruiters (id, user_id)
		values ($1, $2)
		on conflict (user_id) do nothing
	`, id, userID)
	if err != nil {
		return "", fmt.Errorf("failed to exec SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return "", ErrRecruiterAlreadyExists
	}

	return id, nil
}

type Location struct {
	ID         int     `json:"id"`
	Street1    string  `json:"street_1"`
	Street2    *string `json:"street_2"`
	Country    string  `json:"country"`
	City       string  `json:"city"`
	State      *string `json:"state"`
	PostalCode string  `json:"postal_code"`
}

type Position struct {
	ID          ID     `json:"id"`
	RecruiterID ID     `json:"recruiter_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
	IsActive    bool   `json:"is_active"`
}

var ErrPositionAlreadyExists = errors.New("position already exists")

func (s Store) CreatePosition(recruiterID ID, title string, description string, company string, isActive bool) (ID, error) {
	id := NewID()

	result, err := s.DB.Exec(`
		insert into positions (id, recruiter_id, title, description, company, is_active)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (title, description, company) do nothing
	`, id, recruiterID, title, description, company, isActive)
	if err != nil {
		return "", fmt.Errorf("failed to exec SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return "", ErrPositionAlreadyExists
	}

	return id, nil
}

type Candidate struct {
	ID                ID        `json:"id"`
	UserID            ID        `json:"user_id"`
	About             string    `json:"about"`
	PrefRemote        *bool     `json:"pref_remote"`
	PrefTitle1        *string   `json:"pref_title_1"`
	PrefTitle2        *string   `json:"pref_title_2"`
	PrefTitle3        *string   `json:"pref_title_3"`
	PrefLocation1ID   *int      `json:"pref_location_1_id"`
	PrefLocation2ID   *int      `json:"pref_location_2_id"`
	PrefLocation3ID   *int      `json:"pref_location_3_id"`
	LastRecommendedAt time.Time `json:"last_recommended_at"`
}

type ExperienceType string

const (
	ExperienceTypeWork             ExperienceType = "work"
	ExperienceTypeEducation        ExperienceType = "education"
	ExperienceTypeCertification    ExperienceType = "certification"
	ExperienceTypeInternship       ExperienceType = "internship"
	ExperienceTypePersonalProject  ExperienceType = "personal project"
	ExperienceTypeEnterpreneurship ExperienceType = "enterpreneurship"
	ExperienceTypeApprenticeship   ExperienceType = "apprenticeship"
	ExperienceTypeVolunteering     ExperienceType = "volunteering"
	ExperienceTypeOther            ExperienceType = "other"
)

func (t ExperienceType) IsValid() bool {
	switch t {
	case ExperienceTypeWork,
		ExperienceTypeEducation,
		ExperienceTypeCertification,
		ExperienceTypeInternship,
		ExperienceTypePersonalProject,
		ExperienceTypeEnterpreneurship,
		ExperienceTypeApprenticeship,
		ExperienceTypeVolunteering,
		ExperienceTypeOther:
		return true
	}
	return false
}

type CandidateExperience struct {
	ID             ID             `json:"id"`
	CandidateID    ID             `json:"candidate_id"`
	Title          string         `json:"title"`
	StartedAt      time.Time      `json:"started_at"`
	EndedAt        time.Time      `json:"ended_at"`
	Description    *string        `json:"description"`
	Company        *string        `json:"company"`
	ExperienceType ExperienceType `json:"experience_type"`
	Skill1         *string        `json:"skill_1"`
	Skill2         *string        `json:"skill_2"`
	Skill3         *string        `json:"skill_3"`
	Skill4         *string        `json:"skill_4"`
	Skill5         *string        `json:"skill_5"`
}

func CreateCandidateExperiences(tx *sql.Tx, candidateID ID, experiences []CandidateExperience) error {
	for i := range experiences {
		experiences[i].CandidateID = candidateID
		if experiences[i].ID == "" {
			experiences[i].ID = NewID()
		}

		_, err := tx.Exec(`
			insert into candidate_experiences (
				id,
				candidate_id,
				title,
				started_at,
				ended_at,
				description,
				company,
				experience_type,
				skill_1,
				skill_2,
				skill_3,
				skill_4,
				skill_5
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`,
			experiences[i].ID,
			experiences[i].CandidateID,
			experiences[i].Title,
			experiences[i].StartedAt.UTC().Format(time.RFC3339),
			experiences[i].EndedAt.UTC().Format(time.RFC3339),
			experiences[i].Description,
			experiences[i].Company,
			experiences[i].ExperienceType,
			experiences[i].Skill1,
			experiences[i].Skill2,
			experiences[i].Skill3,
			experiences[i].Skill4,
			experiences[i].Skill5,
		)
		if err != nil {
			return fmt.Errorf("failed to execute SQL: %w", err)
		}
	}

	return nil
}

var ErrCandidateAlreadyExists = errors.New("candidate already exists")

func (s Store) CreateCandidate(candidate Candidate, experiences []CandidateExperience) (ID, error) {
	candidate.ID = NewID()

	tx, err := s.DB.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err = tx.Rollback(); err != nil {
				slog.Debug("failed to rollback transaction", "err", err)
			}
		}
	}()

	result, err := tx.Exec(`
		insert into candidates (
			id,
			user_id,
			about,
			pref_remote,
			pref_title_1,
			pref_title_2,
			pref_title_3,
			pref_location_1_id,
			pref_location_2_id,
			pref_location_3_id,
			last_recommended_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		on conflict (user_id) do nothing
	`,
		candidate.ID,
		candidate.UserID,
		candidate.About,
		candidate.PrefRemote,
		candidate.PrefTitle1,
		candidate.PrefTitle2,
		candidate.PrefTitle3,
		candidate.PrefLocation1ID,
		candidate.PrefLocation2ID,
		candidate.PrefLocation3ID,
		CurrentTimestamp(-7*24*time.Hour),
	)
	if err != nil {
		return "", fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return "", ErrCandidateAlreadyExists
	}

	if err = CreateCandidateExperiences(tx, candidate.ID, experiences); err != nil {
		return "", fmt.Errorf("failed to insert candidate experiences: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return candidate.ID, nil
}

func (s Store) ClearAll(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `
		truncate table 
			reactions,
			recommendations,
			matches,
			positions,
			recruiters,
			candidates,
			refresh_tokens,
			users,
			term_frequencies,
			doc_lengths,
			documents,
			corpus_stats
		restart identity cascade
	`); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) UserExistsByEmail(email string, provider Provider) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(`
		select exists(
			select 1
			from users
			where email = $1 and provider = $2
		)
	`, email, provider).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to scan: %w", err)
	}

	return exists, nil
}

func (s Store) GetUserAndRoles(userID ID) (User, map[Role]ID, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash, email string
	var provider Provider
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
		select
			u.provider,
			u.provider_user_id,
			u.email,
			u.full_name,
			u.user_name,
			u.password_hash,
			u.updated_at,
			c.id as candidate_id,
			r.id as recruiter_id
		from users u
		left join candidates c on c.user_id = u.id
		left join recruiters r on r.user_id = u.id
		where u.id = $1
	`, userID).Scan(
		&provider,
		&optionalProviderUserID,
		&email,
		&fullName,
		&userName,
		&passwordHash,
		&updatedAt,
		&candidateID,
		&recruiterID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, nil, ErrUserNotFound
		}
		return User{}, nil, fmt.Errorf("failed to scan: %w", err)
	}

	if optionalProviderUserID.Valid {
		providerUserID = optionalProviderUserID.String
	}

	user := User{
		userID,
		provider,
		providerUserID,
		email,
		fullName,
		userName,
		passwordHash,
		updatedAt,
	}

	roles := make(map[Role]ID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ID(recruiterID.String)
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUser(userID ID) (User, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash, email string
	var provider Provider
	err := s.DB.QueryRow(`
		select provider, provider_user_id, email, full_name, user_name, password_hash, updated_at
		from users
		where id = $1
	`, userID).Scan(
		&provider,
		&optionalProviderUserID,
		&email,
		&fullName,
		&userName,
		&passwordHash,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("failed to scan: %w", err)
	}

	if optionalProviderUserID.Valid {
		providerUserID = optionalProviderUserID.String
	}

	return User{
		userID,
		provider,
		providerUserID,
		email,
		fullName,
		userName,
		passwordHash,
		updatedAt,
	}, nil
}

func (s Store) UpdateUser(
	userID ID,
	newFullName string,
	newUserName string,
) error {
	result, err := s.DB.Exec(`
		update users
		set
			full_name = $1,
			user_name = $2,
			updated_at = $3
		where id = $4
	`, newFullName, newUserName, CurrentTimestamp(), userID)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s Store) UpdateUserAndReturn(
	userID ID,
	newFullName string,
	newUserName string,
) (User, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, passwordHash, email string
	var provider Provider
	err := s.DB.QueryRow(`
		update users
		set
			full_name = $1,
			user_name = $2,
			updated_at = $3
		where id = $4
		returning 
			provider,
			provider_user_id,
			email,
			full_name,
			user_name,
			password_hash,
			updated_at
	`, newFullName, newUserName, CurrentTimestamp(), userID).Scan(
		&provider,
		&optionalProviderUserID,
		&email,
		&newFullName,
		&newUserName,
		&passwordHash,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("failed to scan: %w", err)
	}

	if optionalProviderUserID.Valid {
		providerUserID = optionalProviderUserID.String
	}

	return User{
		userID,
		provider,
		providerUserID,
		email,
		newFullName,
		newUserName,
		passwordHash,
		updatedAt,
	}, nil
}

func (s Store) DeleteUser(userID ID) error {
	res, err := s.DB.Exec(`
		delete from users
		where id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

var ErrCandidateNotFound = errors.New("candidate not found")

func (s Store) GetCandidate(candidateID ID) (Candidate, error) {
	var candidate Candidate
	err := s.DB.QueryRow(`
		select
			user_id,
			about,
			pref_remote,
			pref_title_1,
			pref_title_2,
			pref_title_3,
			pref_location_1_id,
			pref_location_2_id,
			pref_location_3_id,
			last_recommended_at
		from candidates
		where id = $1
	`, candidateID).Scan(
		&candidate.UserID,
		&candidate.About,
		&candidate.PrefRemote,
		&candidate.PrefTitle1,
		&candidate.PrefTitle2,
		&candidate.PrefTitle3,
		&candidate.PrefLocation1ID,
		&candidate.PrefLocation2ID,
		&candidate.PrefLocation3ID,
		&candidate.LastRecommendedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, fmt.Errorf("failed to scan: %w", err)
	}

	candidate.ID = candidateID
	return candidate, nil
}

// UpdateCandidate updates the candidate fields and, if experiences is not nil,
// replaces all existing candidate experiences with the provided ones in the same transaction.
func (s Store) UpdateCandidate(candidate Candidate, experiences []CandidateExperience) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err = tx.Rollback(); err != nil {
				slog.Debug("failed to rollback transaction", "err", err)
			}
		}
	}()

	result, err := tx.Exec(`
		update candidates
		set
			about = $1,
			pref_remote = $2,
			pref_title_1 = $3,
			pref_title_2 = $4,
			pref_title_3 = $5,
			pref_location_1_id = $6,
			pref_location_2_id = $7,
			pref_location_3_id = $8
		where id = $9
	`,
		candidate.About,
		candidate.PrefRemote,
		candidate.PrefTitle1,
		candidate.PrefTitle2,
		candidate.PrefTitle3,
		candidate.PrefLocation1ID,
		candidate.PrefLocation2ID,
		candidate.PrefLocation3ID,
		candidate.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrCandidateNotFound
	}

	if experiences != nil {
		if _, err = tx.Exec(`
			delete from candidate_experiences
			where candidate_id = $1
		`, candidate.ID); err != nil {
			return fmt.Errorf("failed to execute SQL: %w", err)
		}

		if err = CreateCandidateExperiences(tx, candidate.ID, experiences); err != nil {
			return fmt.Errorf("failed to insert candidate experiences: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (s Store) UpdateCandidateAndReturn(
	candidateID ID,
	newAbout string,
) (Candidate, error) {
	candidate := Candidate{ID: candidateID}
	err := s.DB.QueryRow(`
		update candidates
		set about = $1
		where id = $2
		returning user_id, about, last_recommended_at
	`, newAbout, candidateID).Scan(
		&candidate.UserID,
		&candidate.About,
		&candidate.LastRecommendedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, fmt.Errorf("failed to scan: %w", err)
	}

	return candidate, nil
}

func (s Store) GetCandidateExperiences(candidateID ID) ([]CandidateExperience, error) {
	rows, err := s.DB.Query(`
		select 
			id,
			candidate_id,
			title,
			started_at,
			ended_at,
			description,
			company,
			experience_type,
			skill_1,
			skill_2,
			skill_3,
			skill_4,
			skill_5
		from candidate_experiences
		where candidate_id = $1
		order by started_at desc
	`, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	experiences := []CandidateExperience{}
	for rows.Next() {
		var experience CandidateExperience
		err := rows.Scan(&experience.ID, &experience.CandidateID, &experience.Title, &experience.StartedAt, &experience.EndedAt, &experience.Description, &experience.Company, &experience.ExperienceType, &experience.Skill1, &experience.Skill2, &experience.Skill3, &experience.Skill4, &experience.Skill5)
		if err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		experiences = append(experiences, experience)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return experiences, nil
}

func (s Store) DeleteCandidate(candidateID ID) error {
	res, err := s.DB.Exec(`
		delete from candidates
		where id = $1
	`, candidateID)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrCandidateNotFound
	}

	return nil
}

var ErrRecruiterNotFound = errors.New("recruiter not found")

func (s Store) GetRecruiter(recruiterID ID) (Recruiter, error) {
	var userID ID
	err := s.DB.QueryRow(`
		select user_id,
		from recruiters
		where id = $1
	`, recruiterID).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recruiter{}, ErrRecruiterNotFound
		}
		return Recruiter{}, fmt.Errorf("failed to scan: %w", err)
	}

	return Recruiter{
		recruiterID,
		userID,
	}, nil
}

func (s Store) DeleteRecruiter(recruiterID ID) error {
	res, err := s.DB.Exec(`
		delete from recruiters
		where id = $1
	`, recruiterID)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrRecruiterNotFound
	}

	return nil
}

func (s Store) RecruiterExists(recruiterID ID) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(`
		select exists(
			select 1
			from recruiters
			where id = $1
		)
	`, recruiterID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to scan: %w", err)
	}

	return exists, nil
}

var ErrPositionNotFound = errors.New("position not found")

func (s Store) GetPosition(positionID ID) (Position, error) {
	var recruiterID ID
	var title, description, company string
	var isActive bool
	err := s.DB.QueryRow(`
		select recruiter_id, title, description, company, is_active
		from positions
		where id = $1
	`, positionID).Scan(
		&recruiterID,
		&title,
		&description,
		&company,
		&isActive,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Position{}, ErrPositionNotFound
		}
		return Position{}, fmt.Errorf("failed to query: %w", err)
	}
	return Position{
		positionID,
		recruiterID,
		title,
		description,
		company,
		isActive,
	}, nil
}

func (s Store) GetPositions(recruiterID ID, page Page) ([]Position, Page, error) {
	rows, err := s.DB.Query(`
		select id, title, description, company, is_active 
		from positions 
		where recruiter_id = $1 and ($2 = '' or id > $2)
		order by id desc 
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	positions := make([]Position, 0, page.Limit)
	for rows.Next() {
		var position Position
		if err := rows.Scan(
			&position.ID,
			&position.Title,
			&position.Description,
			&position.Company,
			&position.IsActive,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		positions = append(positions, position)
	}

	var nextPage Page
	if len(positions) > page.Limit {
		positions = positions[:page.Limit]
		page.Cursor = string(positions[page.Limit-1].ID)
	}
	return positions, nextPage, nil
}

var ErrInvalidLocationCursor = errors.New("invalid location cursor; must be a positive integer")

func (s Store) SearchLocations(query string, page Page) ([]Location, Page, error) {
	cursorID := 0
	if page.Cursor != "" {
		parsed, err := strconv.ParseInt(page.Cursor, 10, 64)
		if err != nil || parsed < 0 {
			return nil, Page{}, ErrInvalidLocationCursor
		}
		cursorID = int(parsed)
	}

	search := strings.TrimSpace(query)

	rows, err := s.DB.Query(`
		select id, street_1, street_2, country, city, state, postal_code
		from locations
		where ($1 = 0 or id > $1)
				and ($2 = ''
						or lower(country) like '%' || lower($2) || '%'
						or lower(city) like '%' || lower($2) || '%')
		order by id
		limit $3
	`, cursorID, search, page.Limit+1)
	if err != nil {
		return nil, Page{}, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	locations := make([]Location, 0, page.Limit)
	for rows.Next() {
		var location Location
		if err := rows.Scan(
			&location.ID,
			&location.Street1,
			&location.Street2,
			&location.Country,
			&location.City,
			&location.State,
			&location.PostalCode,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	hasNext := len(locations) > page.Limit
	var nextCursor int
	if hasNext {
		locations = locations[:page.Limit]
		nextCursor = locations[page.Limit-1].ID
	}

	return locations, Page{
		Cursor:  strconv.Itoa(nextCursor),
		Limit:   page.Limit,
		Count:   len(locations),
		HasNext: hasNext,
	}, nil
}

func (s Store) UpdatePosition(
	positionID ID,
	newTitle string,
	newDescription string,
	newCompany string,
	isActive bool,
) error {
	result, err := s.DB.Exec(
		`
		update positions
		set 
			title = $2
			description = $3
			company = $4
			is_active = $5
		where id = $1
	`,
		positionID,
		newTitle,
		newDescription,
		newCompany,
		isActive,
	)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrPositionNotFound
	}

	return nil
}

func (s Store) DeletePosition(positionID ID) error {
	res, err := s.DB.Exec(`
		delete from positions
		where id = $1
	`, positionID)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return ErrPositionNotFound
	}

	return nil
}

// EscapeSQLiteFTS prevents SQLite SQLITE_ERROR panics if a user types FTS operators
// (like double quotes, OR, AND, NOT, or asterisks) into their profile bio.
func EscapeSQLiteFTS(query string) string {
	replacer := strings.NewReplacer(
		`"`, ` `,
		`'`, ` `,
		`*`, ` `,
		`^`, ` `,
		`(`, ` `,
		`)`, ` `,
		`-`, ` `,
	)
	query = replacer.Replace(query)
	return `"` + strings.TrimSpace(query) + `"`
}

var ErrEmptyCandidateProfile = errors.New("candidate profile is empty")

func (s Store) GetPositionsForCandidateViaFTS(candidateID ID, topPositions uint16) ([]ID, error) {
	var candidateAbout string
	err := s.DB.QueryRow(`
		select about
		from candidates
		where id = $1
	`, candidateID).Scan(&candidateAbout)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []ID{}, ErrCandidateNotFound
		}
		return nil, fmt.Errorf("failed to scan: %w", err)
	}

	if strings.TrimSpace(candidateAbout) == "" {
		return []ID{}, ErrEmptyCandidateProfile
	}

	var query string
	var args []any
	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		query = `
			select id as position_id
			from positions
			where is_active = 1
					and search_vector @@ websearch_to_tsquery('english', $1)
					and not exists (
							select 1
							from recommendations r
							where r.position_id = positions.id
								and r.candidate_id = $2
					)
			order by ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) desc
			limit $3
		`
		args = []any{candidateAbout, candidateID, topPositions}

	} else if s.DatabaseProvider == DatabaseProviderSQLite {
		escapedAbout := EscapeSQLiteFTS(candidateAbout)
		query = `
			select p.id as position_id
			from positions_fts
			join positions p on p.id = positions_fts.id
			where positions_fts match $1
					and p.is_active = 1
					and not exists (
							select 1
							from recommendations r
							where r.position_id = p.id
								and r.candidate_id = $2
					)
			order by bm25(positions_fts) asc
			limit $3
		`
		args = []any{escapedAbout, candidateID, topPositions}

	} else {
		return nil, ErrUnsupportedDatabaseProvider
	}

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]ID, 0, topPositions)
	for rows.Next() {
		var positionID ID
		if err := rows.Scan(&positionID); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, positionID)
	}

	return results, nil
}
