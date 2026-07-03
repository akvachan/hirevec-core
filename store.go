// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var (
	ErrPositionAlreadyExists            = errors.New("position already exists")
	ErrPositionNotFound                 = errors.New("position not found")
	ErrMissingDatabaseURL               = errors.New("database URL is not set")
	ErrRecommendationAlreadyExists      = errors.New("recommendation already exists")
	ErrRecommendationNotFound           = errors.New("recommendation not found")
	ErrUserAlreadyExists                = errors.New("user already exists")
	ErrCandidateAlreadyExists           = errors.New("candidate already exists")
	ErrRecruiterAlreadyExists           = errors.New("recruiter already exists")
	ErrUserNoRole                       = errors.New("user has no role")
	ErrUserNotFound                     = errors.New("user not found")
	ErrCandidateNotFound                = errors.New("candidate not found")
	ErrRecruiterNotFound                = errors.New("recruiter not found")
	ErrRecommendationExists             = errors.New("recommendation already exists")
	ErrReactionAlreadyExists            = errors.New("reaction already exists")
	ErrEmbeddingsCountConflict          = errors.New("mismatch between count of embedding IDs and embeddings")
	ErrUnsupportedDatabaseProvider      = errors.New("unsupported database provider")
	ErrEmptyCandidateProfile            = errors.New("candidate profile is empty")
	ErrFailedGenerateUserULID           = errors.New("failed to generate ULID for user")
	ErrFailedGenerateJTIULID            = errors.New("failed to generate ULID for refresh token (JTI)")
	ErrFailedGenerateRecommendationULID = errors.New("failed to generate ULID for recommendation")
	ErrFailedGenerateRecruiterULID      = errors.New("failed to generate ULID for recruiter")
	ErrFailedGeneratePositionULID       = errors.New("failed to generate ULID for position")
	ErrFailedGenerateCandidateULID      = errors.New("failed to generate ULID for candidate")
)

const Enc = "0123456789abcdefghjkmnpqrstvwxyz"

type ULID string

func NewULID(prefix string) (ULID, error) {
	var id [16]byte
	out := make([]byte, 26)

	ts := uint64(time.Now().UnixMilli())

	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}

	for i := 9; i >= 0; i-- {
		out[i] = Enc[ts%32]
		ts /= 32
	}

	for i := 0; i < 16; i++ {
		out[10+i] = Enc[id[i]%32]
	}

	return ULID(prefix + string(out)), nil
}

const ULIDPrefixCandidate = "can_"

func NewCandidateULID() (ULID, error) {
	return NewULID(ULIDPrefixCandidate)
}

const ULIDPrefixRecruiter = "rcr_"

func NewRecruiterULID() (ULID, error) {
	return NewULID(ULIDPrefixRecruiter)
}

const ULIDPrefixUser = "usr_"

func NewUserULID() (ULID, error) {
	return NewULID(ULIDPrefixUser)
}

const ULIDPrefixRecommendation = "rcm_"

func NewRecommendationULID() (ULID, error) {
	return NewULID(ULIDPrefixRecommendation)
}

const ULIDPrefixJTI = "jti_"

func NewJTIULID() (ULID, error) {
	return NewULID(ULIDPrefixJTI)
}

const ULIDPrefixPosition = "pos_"

func NewPositionULID() (ULID, error) {
	return NewULID(ULIDPrefixPosition)
}

type DatabaseProvider string

const (
	DatabaseProviderPostgreSQL DatabaseProvider = "PostgreSQL"
	DatabaseProviderSQLite     DatabaseProvider = "SQLite"
)

type StoreConfig struct {
	DatabaseProvider      DatabaseProvider
	PostgreSQLDatabaseURL string
}

func ExecMigration(db *sql.DB, path string) error {
	sql, err := EmbeddedStatic.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := db.Exec(string(sql)); err != nil {
		return err
	}
	return nil
}

func ConnectPostgreSQL(url string) (*sql.DB, error) {
	slog.Debug("connecting to database", "database", "PostgreSQL")
	if url == "" {
		return nil, ErrMissingDatabaseURL
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, err
	}

	return db, nil
}

var DefaultSQLiteConn = "file:db/.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

func ConnectSQLite() (*sql.DB, error) {
	slog.Debug("connecting to database", "database", "SQLite")
	db, err := sql.Open("sqlite", DefaultSQLiteConn)
	if err != nil {
		return nil, err
	}

	var enabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		return nil, err
	}
	slog.Debug("setting foreign keys", "enabled", enabled, "database", "SQLite")

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, err
	}

	return db, nil
}

var (
	PathInitMigration          = path.Join("db/migrations/init.sql")
	PathEmbeddingsMigration    = path.Join("db/migrations/embeddings.sql")
	PathPostgreSQLFTSMigration = path.Join("db/migrations/postgresql-fts.sql")
	PathSQLiteFTSMigration     = path.Join("db/migrations/sqlite-fts.sql")
	PathQuickStartMigration    = path.Join("db/migrations/quick-start.sql")
)

func InitPostgreSQL(url string) (*sql.DB, error) {
	db, err := ConnectPostgreSQL(url)
	if err != nil {
		slog.Error("failed to connect to database", "database", "PostgreSQL", "err", err)
		return nil, err
	}

	slog.Debug("creating database schema", "database", "PostgreSQL")
	if err := ExecMigration(db, PathInitMigration); err != nil {
		return nil, err
	}

	slog.Debug("creating embeddings tables", "database", "PostgreSQL")
	if err := ExecMigration(db, PathEmbeddingsMigration); err != nil {
		return nil, err
	}

	slog.Debug("initializing FTS", "database", "PostgreSQL")
	if err := ExecMigration(db, PathPostgreSQLFTSMigration); err != nil {
		return nil, err
	}

	return db, nil
}

func InitSQLite() (*sql.DB, error) {
	db, err := ConnectSQLite()
	if err != nil {
		slog.Error("failed to connect to database", "database", "SQLite", "err", err)
		return nil, err
	}

	slog.Debug("creating database schema", "database", "SQLite")
	if err := ExecMigration(db, PathInitMigration); err != nil {
		return nil, err
	}

	slog.Debug("initializing FTS", "database", "SQLite")
	if err := ExecMigration(db, PathSQLiteFTSMigration); err != nil {
		return nil, err
	}

	return db, nil
}

type Store struct {
	DatabaseProvider DatabaseProvider
	DB               *sql.DB
}

func NewStore(c StoreConfig) (Store, error) {
	s := Store{}

	var err error
	if c.DatabaseProvider == DatabaseProviderPostgreSQL {
		slog.Debug("initializing store", "database", "PostgreSQL")
		if s.DB, err = InitPostgreSQL(c.PostgreSQLDatabaseURL); err != nil {
			slog.Error("failed to initialize database", "database", "PostgreSQL", "err", err)
			return s, err
		}
	} else if c.DatabaseProvider == DatabaseProviderSQLite {
		slog.Debug("initializing store", "database", "SQLite")
		if s.DB, err = InitSQLite(); err != nil {
			slog.Error("failed to initialize database", "database", "SQLite", "err", err)
			return s, err
		}
	} else {
		return s, ErrUnsupportedDatabaseProvider
	}
	s.DatabaseProvider = c.DatabaseProvider

	return s, nil
}

func (s Store) GetRecommendation(recommendationID ULID) (Recommendation, error) {
	var candidateID, positionID ULID
	err := s.DB.QueryRow(`
		select candidate_id, position_id
		from recommendations
		where id = $1
	`, recommendationID).Scan(
		&candidateID,
		&positionID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recommendation{}, ErrRecommendationNotFound
		}
		return Recommendation{}, err
	}
	return Recommendation{
		recommendationID,
		positionID,
		candidateID,
	}, nil
}

func (s Store) GetUserAndRolesByEmail(email string, provider Provider) (User, map[Role]ULID, error) {
	var userID ULID
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash string
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
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
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, nil, ErrUserNotFound
		}
		return User{}, nil, err
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

	roles := make(map[Role]ULID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ULID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ULID(recruiterID.String)
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUserIDAndRolesByProvider(provider Provider, providerUserID string) (ULID, map[Role]ULID, error) {
	var userID ULID
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
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
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, ErrUserNotFound
		}
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

func (s Store) GetUserRoles(userID ULID, provider Provider) (map[Role]ULID, error) {
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

type User struct {
	ID             ULID      `json:"id"`
	Provider       Provider  `json:"provider"`
	ProviderUserID string    `json:"provider_user_id"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	UserName       string    `json:"user_name"`
	PasswordHash   string    `json:"password_hash"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func CurrentTimestamp(offset ...time.Duration) string {
	t := time.Now()
	if len(offset) > 0 {
		t = t.Add(offset[0])
	}
	return t.UTC().Format(time.RFC3339)
}

func (s Store) CreateUser(
	provider Provider,
	providerUserID,
	email string,
	fullName string,
	userName string,
	passwordHash string,
) (ULID, error) {
	id, err := NewUserULID()
	if err != nil {
		return "", ErrFailedGenerateUserULID
	}

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
		return "", err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
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
	RecommendationID ULID         `json:"recommendation_id"`
	ReactorType      ReactorType  `json:"reactor_type"`
	ReactorID        ULID         `json:"reactor_id"`
	ReactionType     ReactionType `json:"reaction_type"`
	ReactedAt        time.Time    `json:"reacted_at"`
}

func (s Store) CreateReaction(
	recommendationID ULID,
	reactorType ReactorType,
	reactorID ULID,
	reactionType ReactionType,
) error {
	result, err := s.DB.Exec(
		`
		insert into reactions (
			recommendation_id, 
			reactor_type,
			reactor_id,
			reaction_type,
			reacted_at,
		)
		values ($1, $2, $3, $4, $5)
		on conflict (recommendation_id, reactor_type, reactor_id) do nothing
	`,
		recommendationID,
		reactorType,
		reactorID,
		reactionType,
		CurrentTimestamp(),
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

func (s Store) IsRevokedRefreshToken(jti ULID) (bool, error) {
	var isRevoked bool
	return isRevoked, s.DB.QueryRow(`
		select revoked 
		from refresh_tokens 
		where jti = $1 
		and expires_at > $2 
	`, jti, time.Now().UTC()).Scan(&isRevoked)
}

const DefaultMaxRefreshTokensCount = 5

func (s Store) CreateRefreshToken(userID ULID) (jti ULID, err error) {
	jti, err = NewJTIULID()
	if err != nil {
		return "", ErrFailedGenerateJTIULID
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
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
		return "", err
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
			return "", err
		}
	}

	if _, err = tx.Exec(`
		insert into refresh_tokens (jti, user_id, created_at, expires_at)
		values ($1, $2, $3, $4)
	`, jti, userID, currentTimestamp, CurrentTimestamp(DefaultRefreshTokenExpiration)); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return jti, nil
}

type Recommendation struct {
	ID          ULID `json:"id"`
	PositionID  ULID `json:"position_id"`
	CandidateID ULID `json:"candidate_id"`
}

func (s Store) CreateRecommendation(
	positionID ULID,
	candidateID ULID,
) (ULID, error) {
	id, err := NewRecommendationULID()
	if err != nil {
		return "", ErrFailedGenerateRecommendationULID
	}

	result, err := s.DB.Exec(`
		insert into recommendations (recommendation_id, position_id, candidate_id)
		values ($1, $2, $3)
	`, id, positionID, candidateID)
	if err != nil {
		return "", err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
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

type PositionRecommendation struct {
	RecommendationID ULID   `json:"recommendation_id"`
	PositionID       ULID   `json:"position_id"`
	Title            string `json:"title"`
	Company          string `json:"company"`
	Description      string `json:"description"`
}

func (s Store) GetPositionRecommendations(candidateID ULID, page Page, excludeReacted bool) (recommendations []PositionRecommendation, nextCursor ULID, err error) {
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
		return nil, "", err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetPositionRecommendations",
				"err", err,
			)
		}
	}()

	recommendations = make([]PositionRecommendation, 0, page.Limit)
	for rows.Next() {
		var pr PositionRecommendation
		if err := rows.Scan(
			&pr.RecommendationID,
			&pr.PositionID,
			&pr.Title,
			&pr.Company,
			&pr.Description,
		); err != nil {
			return nil, "", err
		}
		recommendations = append(recommendations, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if len(recommendations) > page.Limit {
		recommendations = recommendations[:page.Limit]
		nextCursor = ULID(recommendations[page.Limit-1].RecommendationID)
	}

	return recommendations, nextCursor, nil
}

type CandidateRecommendation struct {
	RecommendationID ULID   `json:"recommendation_id"`
	CandidateID      ULID   `json:"candidate_id"`
	FullName         string `json:"full_name"`
	About            string `json:"about"`
}

func (s Store) GetCandidateRecommendations(recruiterID ULID, page Page, excludeReacted bool) (recommendations []CandidateRecommendation, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
		select r.id, c.id, u.full_name, c.about
		from recommendations r
		join positions p on p.id = r.position_id
		join candidates c on c.id = r.candidate_id
		join users u on u.id = c.user_id
		left join reactions rx on rx.recommendation_id = r.id
				and rx.reactor_type = 'recruiter'
				and rx.reactor_id = $1
		where p.recruiter_id = $1
				and ($2 = '' or r.id > $2)
				and (not $4 or rx.recommendation_id is null)
		order by r.id desc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1, excludeReacted)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetCandidateRecommendations",
				"err", err,
			)
		}
	}()

	recommendations = make([]CandidateRecommendation, 0, page.Limit)
	for rows.Next() {
		var cr CandidateRecommendation
		if err := rows.Scan(
			&cr.RecommendationID,
			&cr.CandidateID,
			&cr.FullName,
			&cr.About,
		); err != nil {
			return nil, "", err
		}
		recommendations = append(recommendations, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if len(recommendations) > page.Limit {
		recommendations = recommendations[:page.Limit]
		nextCursor = ULID(recommendations[page.Limit-1].RecommendationID)
	}

	return recommendations, nextCursor, nil
}

func (s Store) GetReactionsByCandidateID(candidateID ULID, page Page) (reactions []Reaction, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
		select recommendation_id, reactor_type, reactor_id, reaction_type, created_at
		from reactions
		where reactor_id = $1
		  and reactor_type = 'candidate'
		  and ($2 = '' or recommendation_id > $2)
		order by recommendation_id desc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetReactionsByCandidateID",
				"err", err,
			)
		}
	}()

	results := make([]Reaction, 0, page.Limit)
	for rows.Next() {
		var rx Reaction
		if err := rows.Scan(
			&rx.RecommendationID,
			&rx.ReactorType,
			&rx.ReactorID,
			&rx.ReactionType,
			&rx.ReactedAt,
		); err != nil {
			return nil, "", err
		}
		results = append(results, rx)
	}

	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].RecommendationID)
	}

	return results, nextCursor, nil
}

type Match struct {
	PositionID  ULID      `json:"position_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Company     string    `json:"company"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s Store) GetMatchesByCandidateID(candidateID ULID, page Page) (matches []Match, nextCursor ULID, err error) {
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
		return nil, "", err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetMatchesByCandidateID",
				"err", err,
			)
		}
	}()

	results := make([]Match, 0, page.Limit)
	for rows.Next() {
		var m Match
		if err := rows.Scan(
			&m.PositionID,
			&m.Title,
			&m.Description,
			&m.Company,
			&m.CreatedAt,
		); err != nil {
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

type EmbeddingStatus string

const (
	EmbeddingStatusPending = "pending"
	EmbeddingStatusDone    = "done"
	EmbeddingStatusFailed  = "failed"
)

func (s Store) FetchPendingEmbeddingsMetadata(limit uint16) ([]ULID, []string, error) {
	rows, err := s.DB.Query(`
		select entity_id, aggregated_info
		from embeddings_metadata
		where embedding_status in ('pending', 'failed')
		order by embedding_updated_at desc
		limit $1
  `, limit)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "FetchPendingEmbeddingsMetadata",
				"err", err,
			)
		}
	}()

	ids := make([]ULID, 0, limit)
	texts := make([]string, 0, limit)
	for rows.Next() {
		var id ULID
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		texts = append(texts, text)
	}

	return ids, texts, rows.Err()
}

func SqlIn(column string, n int) string {
	if n <= 0 {
		return "1=0"
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Repeat("?,", n-1)+"?")
}

func (s Store) MarkEmbeddingsStatus(entityIDs []ULID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, status)
	for _, id := range entityIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		update embeddings_metadata
		set embedding_status = ?
		where %s
	`, SqlIn("id", len(entityIDs)))

	_, err := s.DB.Exec(query, args...)
	return err
}

func (s Store) MarkEmbeddingsStatusTx(tx *sql.Tx, entityIDs []ULID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, status)
	for _, id := range entityIDs {
		args = append(args, id)
	}

	_, err := tx.Exec(fmt.Sprintf(`
		update embeddings_metadata
		set embedding_status = ?
		where %s
	`, SqlIn("id", len(entityIDs))), args...)
	return err
}

func (s Store) GetPositionsForCandidateViaEmbeddings(candidateID ULID, topPositions uint16) ([]ULID, error) {
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
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetPositionsForCandidateWithEmbedding",
				"err", err,
			)
		}
	}()

	results := make([]ULID, 0, topPositions)
	for rows.Next() {
		var positionID ULID
		if err := rows.Scan(&positionID); err != nil {
			return nil, err
		}
		results = append(results, positionID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (s Store) UpsertEmbeddingsTx(tx *sql.Tx, embeddingIDs []ULID, embeddings []EmbeddingEntity) error {
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
		return err
	}
	defer func() {
		if err := insertTx.Close(); err != nil {
			slog.Error(
				"failed to close insert transaction",
				"method", "UpsertEmbeddingsTx",
				"err", err,
			)
		}
	}()

	for i := range embeddingIDs {
		if _, err := insertTx.Exec(
			embeddingIDs[i],
			embeddings[i].Embedding,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s Store) GetCandidates(limit uint16, recommendationSpan time.Duration) ([]ULID, error) {
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
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetCandidates",
				"err", err,
			)
		}
	}()

	ids := make([]ULID, 0, limit)
	for rows.Next() {
		var id ULID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

type Recruiter struct {
	ID     ULID `json:"id"`
	UserID ULID `json:"user_id"`
}

func (s Store) CreateRecruiter(userID ULID) (ULID, error) {
	id, err := NewRecruiterULID()
	if err != nil {
		return "", ErrFailedGenerateRecruiterULID
	}

	result, err := s.DB.Exec(`
		insert into recruiters (id, user_id)
		values ($1, $2)
		on conflict (user_id) do nothing
	`, id, userID)
	if err != nil {
		return "", err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrRecruiterAlreadyExists
	}

	return id, nil
}

type Position struct {
	ID          ULID   `json:"id"`
	RecruiterID ULID   `json:"recruiter_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
	IsActive    bool   `json:"is_active"`
}

func (s Store) CreatePosition(
	recruiterID ULID,
	title string,
	description string,
	company string,
	isActive bool,
) (ULID, error) {
	id, err := NewPositionULID()
	if err != nil {
		return "", ErrFailedGeneratePositionULID
	}

	result, err := s.DB.Exec(
		`
		insert into positions (id, recruiter_id, title, description, company, is_active)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (title, description, company) do nothing
	`,
		id,
		recruiterID,
		title,
		description,
		company,
		isActive,
	)
	if err != nil {
		return "", err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrPositionAlreadyExists
	}

	return id, nil
}

type Candidate struct {
	ID                ULID      `json:"id"`
	UserID            ULID      `json:"user_id"`
	About             string    `json:"about"`
	LastRecommendedAt time.Time `json:"last_recommended_at"`
}

func (s Store) CreateCandidate(
	userID ULID,
	about string,
) (ULID, error) {
	id, err := NewCandidateULID()
	if err != nil {
		return "", ErrFailedGenerateCandidateULID
	}

	result, err := s.DB.Exec(
		`
		insert into candidates (id, user_id, about, last_recommended_at)
		values ($1, $2, $3, $4)
		on conflict (user_id) do nothing
	`,
		id,
		userID,
		about,
		CurrentTimestamp(-7*24*time.Hour),
	)
	if err != nil {
		return "", err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrCandidateAlreadyExists
	}

	return id, nil
}

func (s Store) ClearAll(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `
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
	`)
	return err
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
		return false, err
	}

	return exists, nil
}

func (s Store) GetUserAndRoles(userID ULID) (User, map[Role]ULID, error) {
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
		return User{}, nil, err
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

	roles := make(map[Role]ULID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ULID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ULID(recruiterID.String)
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUser(userID ULID) (User, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash, email string
	var provider Provider
	err := s.DB.QueryRow(`
		select
			provider,
			provider_user_id,
			email,
			full_name,
			user_name,
			password_hash,
			updated_at
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
		return User{}, err
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
	userID ULID,
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
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s Store) UpdateUserAndReturn(
	userID ULID,
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
		return User{}, err
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

func (s Store) DeleteUser(userID ULID) error {
	res, err := s.DB.Exec(`
		delete from users
		where id = $1
	`, userID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s Store) GetCandidate(candidateID ULID) (Candidate, error) {
	var userID ULID
	var about string
	var lastRecommendedAt time.Time
	err := s.DB.QueryRow(`
		select
			user_id,
			about,
			last_recommended_at
		from candidates
		where id = $1
	`, candidateID).Scan(
		&userID,
		&about,
		&lastRecommendedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, err
	}

	return Candidate{
		candidateID,
		userID,
		about,
		lastRecommendedAt,
	}, nil
}

func (s Store) UpdateCandidate(
	candidateID ULID,
	newAbout string,
) error {
	result, err := s.DB.Exec(`
		update candidates
		set about = $1
		where id = $2
	`, newAbout, candidateID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrCandidateNotFound
	}

	return nil
}

func (s Store) UpdateCandidateAndReturn(
	candidateID ULID,
	newAbout string,
) (Candidate, error) {
	var userID ULID
	var lastRecommendedAt time.Time
	err := s.DB.QueryRow(`
		update candidates
		set
			about = $1,
		where id = $2
		returning 
			user_id,
			about,
			last_recommended_at
	`, newAbout, candidateID).Scan(
		&userID,
		&newAbout,
		&lastRecommendedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Candidate{}, ErrCandidateNotFound
		}
		return Candidate{}, err
	}

	return Candidate{
		candidateID,
		userID,
		newAbout,
		lastRecommendedAt,
	}, nil
}

func (s Store) DeleteCandidate(candidateID ULID) error {
	res, err := s.DB.Exec(`
		delete from candidates
		where id = $1
	`, candidateID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrCandidateNotFound
	}

	return nil
}

func (s Store) GetRecruiter(recruiterID ULID) (Recruiter, error) {
	var userID ULID
	err := s.DB.QueryRow(`
		select user_id,
		from recruiters
		where id = $1
	`, recruiterID).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recruiter{}, ErrRecruiterNotFound
		}
		return Recruiter{}, err
	}

	return Recruiter{
		recruiterID,
		userID,
	}, nil
}

func (s Store) DeleteRecruiter(recruiterID ULID) error {
	res, err := s.DB.Exec(`
		delete from recruiters
		where id = $1
	`, recruiterID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecruiterNotFound
	}

	return nil
}

func (s Store) RecruiterExists(recruiterID ULID) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(`
		select exists(
			select 1
			from recruiters
			where id = $1
		)
	`, recruiterID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (s Store) GetPosition(positionID ULID) (Position, error) {
	var recruiterID ULID
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
		return Position{}, err
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

func (s Store) GetPositions(recruiterID ULID, page Page) (positions []Position, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
		select id, title, description, company, is_active 
		from positions 
		where recruiter_id = $1 and ($2 = '' or id > $2)
		order by id desc 
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetPositions",
				"err", err,
			)
		}
	}()

	positions = make([]Position, 0, page.Limit)
	for rows.Next() {
		var position Position
		if err := rows.Scan(
			&position.ID,
			&position.Title,
			&position.Description,
			&position.Company,
			&position.IsActive,
		); err != nil {
			return nil, "", err
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if len(positions) > page.Limit {
		positions = positions[:page.Limit]
		nextCursor = ULID(positions[page.Limit-1].ID)
	}
	return positions, nextCursor, nil
}

func (s Store) UpdatePosition(
	positionID ULID,
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
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrPositionNotFound
	}

	return nil
}

func (s Store) DeletePosition(positionID ULID) error {
	res, err := s.DB.Exec(`
		delete from positions
		where id = $1
	`, positionID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
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

func (s Store) GetPositionsForCandidateViaFTS(candidateID ULID, topPositions uint16) ([]ULID, error) {
	var candidateAbout string
	err := s.DB.QueryRow(`
		select about
		from candidates
		where id = $1
	`, candidateID).Scan(&candidateAbout)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []ULID{}, ErrCandidateNotFound
		}
		return nil, err
	}

	if strings.TrimSpace(candidateAbout) == "" {
		return []ULID{}, ErrEmptyCandidateProfile
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
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error(
				"failed to close rows",
				"method", "GetPositionsForCandidateViaFTS",
				"err", err,
			)
		}
	}()

	results := make([]ULID, 0, topPositions)
	for rows.Next() {
		var positionID ULID
		if err := rows.Scan(&positionID); err != nil {
			return nil, err
		}
		results = append(results, positionID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
