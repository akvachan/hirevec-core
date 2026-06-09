// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

package hirevec

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var (
	ErrPositionAlreadyExists       = errors.New("position already exists")
	ErrMissingDatabaseURL          = errors.New("database URL is not set")
	ErrRecommendationAlreadyExists = errors.New("recommendation already exists")
	ErrUserAlreadyExists           = errors.New("user already exists")
	ErrCandidateAlreadyExists      = errors.New("candidate already exists")
	ErrRecruiterAlreadyExists      = errors.New("recruiter already exists")
	ErrUserNoRole                  = errors.New("user has no role")
	ErrUserNotFound                = errors.New("user not found")
	ErrCandidateNotFound           = errors.New("candidate not found")
	ErrRecommendationExists        = errors.New("recommendation already exists")
	ErrReactionAlreadyExists       = errors.New("reaction already exists")
	ErrEmbeddingsCountConflict     = errors.New("mismatch between count of embedding IDs and embeddings")
)

const Enc = "0123456789abcdefghjkmnpqrstvwxyz"

type ULID string

func NewULID(prefix string) (ULID, error) {
	var id [16]byte
	out := make([]byte, 26)

	ts := uint64(time.Now().UnixMilli())

	if _, err := rand.Read(id[6:]); err != nil {
		return "", err
	}

	for i := 9; i >= 0; i-- {
		out[i] = Enc[ts%32]
		ts /= 32
	}

	for i := 0; i < 16; i++ {
		out[10+i] = Enc[id[6+i%10]%32]
	}

	return ULID(prefix + string(out)), nil
}

func NewCandidateULID() (ULID, error) {
	return NewULID("can_")
}

func NewRecruiterULID() (ULID, error) {
	return NewULID("rcr_")
}

func NewUserULID() (ULID, error) {
	return NewULID("usr_")
}

func NewRecommendationULID() (ULID, error) {
	return NewULID("rcm_")
}

func NewJTIULID() (ULID, error) {
	return NewULID("jti_")
}

func NewPositionULID() (ULID, error) {
	return NewULID("pos_")
}

type StoreConfig struct {
	UsePostgres         bool
	PostgresDatabaseURL string
}

func ConnectPostgres(c StoreConfig) (*sql.DB, error) {
	slog.Debug("connecting to Postgres")
	if c.PostgresDatabaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	db, err := sql.Open("pgx", c.PostgresDatabaseURL)
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

var DefaultSQLitePath = ".db"

func ConnectSQLite() (*sql.DB, error) {
	slog.Debug("connecting to SQLite")
	db, err := sql.Open("sqlite", DefaultSQLitePath)
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

type Store struct{ db *sql.DB }

var (
	DefaultInitSQLPath       = path.Join("init.sql")
	DefaultEmbeddingsSQLPath = path.Join("embeddings.sql")
)

func NewStore(c StoreConfig) (s *Store, err error) {
	var db *sql.DB

	if c.UsePostgres {
		db, err = ConnectPostgres(c)
		if err != nil {
			slog.Error(
				"failed to connect Postgres",
				"err", err,
			)
			return nil, err
		}
	} else {
		db, err = ConnectSQLite()
		if err != nil {
			slog.Error(
				"failed to connect SQLite",
				"err", err,
			)
			return nil, err
		}
	}

	slog.Debug("initializing database")
	sqlBytes, err := os.ReadFile(DefaultInitSQLPath)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		return nil, err
	}

	if c.UsePostgres {
		slog.Debug("creating embeddings tables")
		sqlBytes, err := os.ReadFile(DefaultEmbeddingsSQLPath)
		if err != nil {
			return nil, err
		}

		_, err = db.Exec(string(sqlBytes))
		if err != nil {
			return nil, err
		}
	}

	return &Store{db}, nil
}

func (s Store) GetRecommendation(id ULID) (*Recommendation, error) {
	var r Recommendation
	err := s.db.QueryRow(`
		select candidate_id, position_id
		from recommendations
		where id = $1
	`, id).Scan(&r.CandidateID, &r.PositionID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s Store) GetUserByEmail(
	email string,
	provider Provider,
) (*User, map[Role]ULID, error) {
	var userID ULID
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash string
	var candidateID, recruiterID sql.NullString
	err := s.db.QueryRow(`
		select
			u.id,
			u.provider_user_id,
			u.full_name,
			u.user_name,
			u.password_hash,
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
		&candidateID,
		&recruiterID,
	)
	if err == sql.ErrNoRows {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	if optionalProviderUserID.Valid {
		providerUserID = optionalProviderUserID.String
	}

	user := User{
		ID:             userID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		Email:          email,
		FullName:       fullName,
		UserName:       userName,
		PasswordHash:   passwordHash,
	}

	roles := make(map[Role]ULID, 2)
	if candidateID.Valid {
		roles[RoleCandidate] = ULID(candidateID.String)
	}
	if recruiterID.Valid {
		roles[RoleRecruiter] = ULID(recruiterID.String)
	}
	if len(roles) == 0 {
		return &user, nil, ErrUserNoRole
	}

	return &user, roles, nil
}

func (s Store) GetUserByProvider(
	provider Provider,
	providerUserID string,
) (ULID, map[Role]ULID, error) {
	var userID ULID
	var candidateID, recruiterID sql.NullString
	err := s.db.QueryRow(`
		select u.id, c.id as candidate_id,
				r.id as recruiter_id
		from users u
		left join candidates c on c.user_id = u.id
		left join recruiters r on r.user_id = u.id
		where u.provider = $1 and u.provider_user_id = $2
   `, provider, providerUserID).Scan(&userID, &candidateID, &recruiterID)
	if err == sql.ErrNoRows {
		return "", nil, ErrUserNotFound
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

func (s Store) GetUserRoles(
	userID ULID,
	provider Provider,
) (map[Role]ULID, error) {
	var candidateID, recruiterID sql.NullString
	err := s.db.QueryRow(`
		select
				(select c.id from candidates c where c.user_id = u.id) as candidate_id,
				(select r.id from recruiters r where r.user_id = u.id) as recruiter_id
		from users u
		where u.id = $1 and u.provider = $2
	`, userID, provider).Scan(&candidateID, &recruiterID)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
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

type User struct {
	ID             ULID      `json:"id,omitempty"`
	Provider       Provider  `json:"provider,omitempty"`
	ProviderUserID string    `json:"provider_user_id,omitempty"`
	Email          string    `json:"email,omitempty"`
	FullName       string    `json:"full_name,omitempty"`
	UserName       string    `json:"user_name,omitempty"`
	PasswordHash   string    `json:"password_hash,omitempty"`
	UpdatedAt      time.Time `json:"update_at,omitempty"`
}

func (s Store) CreateUser(u User) error {
	result, err := s.db.Exec(`
		insert into users (
			id,
			provider,
			provider_user_id,
		  email,
		  full_name,
		  user_name,
			password_hash
		)
		values ($1, $2, nullif($3, ''), $4, $5, $6, $7) 
		on conflict (provider, provider_user_id) do nothing
	`, u.ID, u.Provider, u.ProviderUserID, u.Email, u.FullName, u.UserName, u.PasswordHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserAlreadyExists
	}
	return err
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

func (s Store) CreateReaction(r Reaction) error {
	result, err := s.db.Exec(`
		insert into reactions (
			recommendation_id, 
			reactor_type,
			reactor_id,
			reaction_type,
			reacted_at,
		)
		values ($1, $2, $3, $4, current_timestamp)
		on conflict (recommendation_id, reactor_type, reactor_id) do nothing
	`, r.RecommendationID, r.ReactorType, r.ReactorID, r.ReactionType, r.ReactedAt)
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
	return isRevoked, s.db.QueryRow(`
		select revoked 
		from refresh_tokens 
		where jti = $1 
		and expires_at > $2 
	`, jti, time.Now().UTC()).Scan(&isRevoked)
}

const DefaultMaxRefreshTokensCount = 5

func (s Store) CreateRefreshToken(
	jti ULID,
	userID ULID,
	expiresAt time.Time,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var count int
	if err = tx.QueryRow(`
		select count(*)
		from refresh_tokens
		where user_id = $1
		  and revoked = false
		  and expires_at > current_timestamp
	`, userID).Scan(&count); err != nil {
		return err
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
				  and expires_at > current_timestamp
				order by created_at asc
				limit 1
			)
		`, userID); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`
		insert into refresh_tokens (jti, user_id, expires_at)
		values ($1, $2, $3)
	`, jti, userID, expiresAt); err != nil {
		return err
	}

	return tx.Commit()
}

type Recommendation struct {
	ID          ULID `json:"id"`
	PositionID  ULID `json:"position_id"`
	CandidateID ULID `json:"candidate_id"`
}

func (s Store) CreateRecommendation(recommendation Recommendation) error {
	result, err := s.db.Exec(`
		insert into recommendations (recommendation_id, position_id, candidate_id)
		values ($1, $2, $3)
	`, recommendation.ID, recommendation.PositionID, recommendation.CandidateID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecommendationAlreadyExists
	}
	return nil
}

func (s Store) CreateRecommendations(recommendations []Recommendation) error {
	if len(recommendations) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		insert into recommendations (recommendation_id, position_id, candidate_id)
		values ($1, $2, $3)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, recommendation := range recommendations {
		if _, err = stmt.Exec(
			recommendation.ID,
			recommendation.PositionID,
			recommendation.CandidateID,
		); err != nil {
			return err
		}
		if _, err = s.db.Exec(`
			update candidates
			set last_recommended_at = current_timestamp
			where candidate_id = $1 
		`, recommendation.CandidateID); err != nil {
			return err
		}
	}

	return tx.Commit()
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

func (s Store) GetPositionRecommendations(
	candidateID ULID,
	page Page,
	excludeReacted bool,
) (
	positionRecommendations []PositionRecommendation,
	nextCursor ULID,
	err error,
) {
	rows, err := s.db.Query(`
		select r.id, p.id, p.title, p.company, p.description
		from recommendations r
		join positions p on p.id = r.position_id
		left join reactions rx on rx.recommendation_id = r.id
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

	results := make([]PositionRecommendation, 0, page.Limit)
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

type CandidateRecommendation struct {
	RecommendationID ULID   `json:"recommendation_id"`
	CandidateID      ULID   `json:"candidate_id"`
	FullName         string `json:"full_name,omitempty"`
	About            string `json:"about"`
}

func (s Store) GetCandidateRecommendations(
	recruiterID ULID,
	page Page,
	excludeReacted bool,
) ([]CandidateRecommendation, ULID, error) {
	rows, err := s.db.Query(`
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
		order by r.id asc
		limit $3
	`, recruiterID, page.Cursor, page.Limit+1, excludeReacted)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	results := make([]CandidateRecommendation, 0, page.Limit)
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

func (s Store) GetReactionsByCandidateID(
	candidateID ULID,
	page Page,
) (reactions []Reaction, nextCursor ULID, err error) {
	rows, err := s.db.Query(`
		select recommendation_id, reactor_type, reactor_id, reaction_type, created_at
		from reactions
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

func (s Store) GetMatchesByCandidateID(
	candidateID ULID,
	page Page,
) (matches []Match, nextCursor ULID, err error) {
	rows, err := s.db.Query(`
		select m.position_id, p.title, p.description, coalesce(p.company, ''), m.created_at
		from matches m
		join positions p on p.id = m.position_id
		where m.candidate_id = $1
		  and ($2 = '' or m.position_id > $2)
		order by m.position_id asc
		limit $3
	`, candidateID, page.Cursor, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

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

func (s Store) FetchPendingEmbeddingsMetadata(
	limit uint16,
) ([]ULID, []string, error) {
	rows, err := s.db.Query(`
		select entity_id, aggregated_info
		from embeddings_metadata
		where embedding_status in ('pending', 'failed')
		order by embedding_updated_at asc
		limit $1
  `, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

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

func (s Store) MarkEmbeddingsStatus(
	entityIDs []ULID,
	status EmbeddingStatus,
) error {
	if len(entityIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, status)
	for _, id := range entityIDs {
		args = append(args, id)
	}

	_, err := s.db.Exec(`
		update embeddings_metadata
		set embedding_status = $1
		where id in any($2)
	`, status, entityIDs)
	return err
}

func (s Store) MarkEmbeddingsStatusTx(
	tx *sql.Tx,
	entityIDs []ULID,
	status EmbeddingStatus,
) error {
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

func (s Store) GetPositionsForCandidateWithEmbedding(
	candidateID ULID,
	nearestNeighbors uint16,
) ([]ULID, error) {
	rows, err := s.db.Query(`
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
	`, candidateID, nearestNeighbors)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]ULID, 0, nearestNeighbors)
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

func (s Store) UpsertEmbeddings(
	tx *sql.Tx,
	embeddingIDs []ULID,
	embeddings []EmbeddingEntity,
) error {
	if len(embeddingIDs) == 0 {
		return nil
	}
	if len(embeddingIDs) != len(embeddings) {
		return ErrEmbeddingsCountConflict
	}

	stmt, err := tx.Prepare(`
		insert or replace into embeddings (id, embedding)
		values ($1, $1)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range embeddingIDs {
		if _, err := stmt.Exec(
			embeddingIDs[i],
			embeddings[i].Embedding,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s Store) GetCandidates(
	limit uint16,
	recommendationSpan time.Duration,
) ([]ULID, []string, error) {
	rows, err := s.db.Query(`
		select id, aggregated_info
		from embeddings_metadata
		where last_recommended_at <= datetime('now', '-' || $1 || ' seconds')
		order by last_recommended_at asc
		limit $2
	`, int64(recommendationSpan.Seconds()), limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

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

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return ids, texts, nil
}

type Recruiter struct {
	ID     ULID `json:"id"`
	UserID ULID `json:"user_id"`
}

func (s Store) CreateRecruiter(recruiter Recruiter) error {
	result, err := s.db.Exec(`
		insert into recruiters (id, user_id)
		values ($1, $2)
		on conflict (user_id) do nothing
	`, recruiter.ID, recruiter.UserID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecruiterAlreadyExists
	}
	return nil
}

type Position struct {
	ID          ULID   `json:"id"`
	RecruiterID ULID   `json:"recruiter_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
	IsActive    bool   `json:"is_active"`
}

func (s Store) CreatePosition(position Position) error {
	result, err := s.db.Exec(`
		insert into positions (id, recruiter_id, title, description, company, is_active)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (tite, description, company) do nothing
	`,
		position.ID,
		position.RecruiterID,
		position.Title,
		position.Description,
		position.Company,
		position.IsActive,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrPositionAlreadyExists
	}
	return nil
}

type Candidate struct {
	ID                ULID      `json:"id"`
	UserID            ULID      `json:"user_id,omitempty"`
	About             string    `json:"about"`
	LastRecommendedAt time.Time `json:"last_recommended_at"`
}

func (s Store) CreateCandidate(candidate Candidate) error {
	result, err := s.db.Exec(`
		insert into candidates (id, user_id, about, last_recommended_at)
		values ($1, $2, $3, current_timestamp)
		on conflict (user_id) do nothing
	`,
		candidate.ID,
		candidate.UserID,
		candidate.About,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrCandidateAlreadyExists
	}
	return nil
}

func (s Store) ClearAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
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

func (s Store) UserExistsByEmail(
	email string,
	provider Provider,
) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
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

func (s Store) GetUser(id ULID) (*User, error) {
	user := User{}
	err := s.db.QueryRow(`
		select id, provider, email, full_name, user_name, updated_at
		from users
		where id = $1
	`, id).Scan(
		&user.ID,
		&user.Provider,
		&user.Email,
		&user.FullName,
		&user.UserName,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (s Store) UpdateUser(u User) (*User, error) {
	updatedUser := User{}
	err := s.db.QueryRow(`
		update users
		set
			full_name = $1,
			user_name = $2,
			updated_at = current_timestamp
		where id = $3
		returning id, provider, email, full_name, user_name, updated_at
	`, u.FullName, u.UserName, u.ID).Scan(
		&updatedUser.ID,
		&updatedUser.Provider,
		&updatedUser.Email,
		&updatedUser.FullName,
		&updatedUser.UserName,
		&updatedUser.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return &updatedUser, nil
}
