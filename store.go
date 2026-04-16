// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrFailedReadInitSQL           = errors.New("failed to read init.sql")
	ErrFailedReadDevSQL            = errors.New("failed to read dev.sql")
	ErrRecommendationAlreadyExists = errors.New("recommendation already exists")
	ErrUserAlreadyExists           = errors.New("user already exists")
	ErrCandidateAlreadyExists      = errors.New("candidate already exists")
	ErrRecruiterAlreadyExists      = errors.New("recruiter already exists")
	ErrFailedConnectDB             = errors.New("failed to connect to DB")
	ErrFailedPingDB                = errors.New("failed to ping DB")
	ErrUserNoRole                  = errors.New("user has no role")
	ErrUserNotFound                = errors.New("user not found")
	ErrRecommendationExists        = errors.New("recommendation already exists")
	ErrCandidateNotFound           = errors.New("candidate not found")
	ErrRecruiterNotFound           = errors.New("recruiter not found")
	ErrReactionAlreadyExists       = errors.New("reaction already exists")
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

type EmbeddingObjectType string

const (
	EmbeddingObjectTypePosition  EmbeddingObjectType = "position"
	EmbeddingObjectTypeCandidate EmbeddingObjectType = "candidate"
)

type ULID string

const enc = "0123456789abcdefghjkmnpqrstvwxyz"

func newULID(prefix string) (ULID, error) {
	var id [16]byte
	out := make([]byte, 26)

	ts := uint64(time.Now().UnixMilli())

	if _, err := rand.Read(id[6:]); err != nil {
		return "", err
	}

	for i := 9; i >= 0; i-- {
		out[i] = enc[ts%32]
		ts /= 32
	}

	for i := 0; i < 16; i++ {
		out[10+i] = enc[id[6+i%10]%32]
	}

	return ULID(prefix + string(out)), nil
}

func NewCandidateULID() (ULID, error) {
	return newULID("can_")
}

func NewRecruiterULID() (ULID, error) {
	return newULID("rec_")
}

func NewUserULID() (ULID, error) {
	return newULID("usr_")
}

func NewRecommendationULID() (ULID, error) {
	return newULID("rcm_")
}

func NewJTIULID() (ULID, error) {
	return newULID("jti_")
}

func NewPositionULID() (ULID, error) {
	return newULID("pos_")
}

func NewPositionEmbeddingJobULID() (ULID, error) {
	return newULID("job_pos_")
}

func NewCandidateEmbeddingJobULID() (ULID, error) {
	return newULID("job_can_")
}

type (
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
		ID                ULID      `json:"id"`
		UserID            ULID      `json:"user_id,omitempty"`
		About             string    `json:"about"`
		LastRecommendedAt time.Time `json:"last_recommended_at"`
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
		IsActive    bool   `json:"is_active"`
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

type Store struct {
	DB *sql.DB
}

func NewStore() (*Store, error) {
	db, err := ConnectToDB()
	if err != nil {
		return nil, err
	}

	if err := InitDB(db); err != nil {
		return nil, err
	}

	if err := IngestData(db); err != nil {
		return nil, err
	}

	return &Store{db}, nil
}

const EmbeddingSize uint64 = 0

func InitDB(db *sql.DB) error {
	sqlBytes, err := os.ReadFile("init.sql")
	if err != nil {
		return ErrFailedReadInitSQL
	}

	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		return err
	}

	return nil
}

func IngestData(db *sql.DB) error {
	sqlBytes, err := os.ReadFile("dev.sql")
	if err != nil {
		return ErrFailedReadDevSQL
	}

	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		return err
	}

	return nil
}

func ConnectToDB() (*sql.DB, error) {
	slog.Debug("connecting to DB")

	db, err := sql.Open("sqlite", ".db")
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

	return db, nil
}

func (s Store) GetCandidate(id ULID) (*Candidate, error) {
	var c Candidate

	err := s.DB.QueryRow(`
		select id, about
		from v1.candidates
		where id = $1
	`, id).Scan(&c.ID, &c.About)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s Store) GetRecommendation(id ULID) (*Recommendation, error) {
	var r Recommendation

	err := s.DB.QueryRow(`
		select id, candidate_id, position_id
		from v1.recommendations
		where id = $1
	`, id).Scan(&r.ID, &r.CandidateID, &r.PositionID)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

func (s Store) GetPosition(id ULID) (*Position, error) {
	var p Position

	err := s.DB.QueryRow(`
		select id, recruiter_id, title, description, company
		from v1.positions
		where id = $1
	`, id).Scan(&p.ID, &p.RecruiterID, &p.Title, &p.Description, &p.Company)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// GetCandidateByUserID fetches a candidate by their associated user ID.
func (s Store) GetCandidateByUserID(userID ULID) (*Candidate, error) {
	var c Candidate
	err := s.DB.QueryRow(`
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
func (s Store) GetUserByProvider(provider Provider, providerUserID string) (ULID, map[Role]ULID, error) {
	var userID ULID
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
		select
				u.id,
				c.id as candidate_id,
				r.id as recruiter_id
		from v1.users u
		left join v1.candidates c on c.user_id = u.id
		left join v1.recruiters r on r.user_id = u.id
		where u.provider = $1
				and u.provider_user_id = $2
   `, provider, providerUserID).Scan(&userID, &candidateID, &recruiterID)
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
func (s Store) GetUserRoles(userID ULID, provider Provider) (map[Role]ULID, error) {
	var candidateID, recruiterID sql.NullString
	err := s.DB.QueryRow(`
		select
				(select c.id from v1.candidates c where c.user_id = u.id) as candidate_id,
				(select r.id from v1.recruiters r where r.user_id = u.id) as recruiter_id
		from v1.users u
		where u.user_id = $1
			and u.provider = $2
	`, userID, provider).Scan(&candidateID, &recruiterID)
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

// CreateUser inserts a new user record.
func (s Store) CreateUser(u User) error {
	result, err := s.DB.Exec(`
		insert into v1.users (id, provider, provider_user_id, email, full_name, user_name)
		values ($1, $2, $3, $4, $5, $6) 
		on conflict (provider, provider_user_id) do nothing
	`, u.ID, u.Provider, u.ProviderUserID, u.Email, u.FullName, u.UserName)
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

// CreateReaction records a reaction (from a candidate or recruiter) to a recommendation.
func (s Store) CreateReaction(r Reaction) error {
	result, err := s.DB.Exec(`
		insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
		values ($1, $2, $3, $4)
		on conflict (recommendation_id, reactor_type, reactor_id) do nothing
	`, r.RecommendationID, r.ReactorType, r.ReactorID, r.ReactionType)
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
func (s Store) CreateCandidate(c Candidate) error {
	result, err := s.DB.Exec(`
		insert into v1.candidates (id, user_id, about)
		values ($1, $2, $3)
		on conflict (id) do nothing
	`, c.ID, c.UserID, c.About)
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

// CreateRecruiter creates a recruiter
func (s Store) CreateRecruiter(r Recruiter) error {
	result, err := s.DB.Exec(`
		insert into v1.recruiters (id, user_id)
		values ($1, $2)
		on conflict (id) do nothing
	`, r.ID, r.UserID)
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

// IsActiveSession checks if the JTI exists and is not expired.
func (s Store) IsActiveSession(jti ULID) (bool, error) {
	var isActive bool
	return isActive, s.DB.QueryRow(`
		select revoked 
		from v1.refresh_tokens 
		where jti = $1 
		and expires_at > now()
	`, jti).Scan(&isActive)
}

// CreateRefreshToken creates a new refresh token record.
func (s Store) CreateRefreshToken(jti ULID, userID ULID, expiresAt time.Time) error {
	_, err := s.DB.Exec(
		`
		insert into v1.refresh_tokens (jti, user_id, expires_at)
		values ($1, $2, $3) 
		on conflict (jti, user_id, expires_at) do nothing
		`,
		jti,
		userID,
		expiresAt,
	)
	return err
}

// CreateRecommendation inserts a new recommendation for a candidate and a position.
func (s Store) CreateRecommendation(recommendation Recommendation) error {
	result, err := s.DB.Exec(`
		insert into v1.recommendations (recommendation_id, position_id, candidate_id)
		values ($1, $2, $3)
		on conflict (recommendation_id, position_id, candidate_id) do nothing
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

// GetPositionRecommendations returns paginated position recommendations for a candidate.
func (s Store) GetPositionRecommendations(candidateID ULID, page Page, excludeReacted bool) (positionRecommendations []PositionRecommendation, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
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

func (s Store) GetCandidateRecommendations(recruiterID ULID, page Page, excludeReacted bool) ([]CandidateRecommendation, ULID, error) {
	rows, err := s.DB.Query(`
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
func (s Store) GetReactionsByCandidateID(candidateID ULID, page Page) (reactions []Reaction, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
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

	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextCursor = ULID(results[page.Limit-1].RecommendationID)
	}

	return results, nextCursor, nil
}

// GetMatchesByCandidateID returns paginated matches for a candidate.
func (s Store) GetMatchesByCandidateID(candidateID ULID, page Page) (matches []Match, nextCursor ULID, err error) {
	rows, err := s.DB.Query(`
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
func (s Store) GetRecruiterByUserID(userID ULID) (*Recruiter, error) {
	var rec Recruiter
	err := s.DB.QueryRow(`
		select id, user_id 
		from v1.recruiters 
		where user_id = $1 limit 1
	`, userID).Scan(&rec.ID, &rec.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecruiterNotFound
		}
		return nil, err
	}
	return &rec, nil
}

func (s Store) UpsertEmbeddings(GoogleEmbeddingsResponseBody) error {
	return nil
}

type EntityType string

const (
	EntityTypePosition  = "position"
	EntityTypeCandidate = "candidate"
)

type EmbeddingJobStatus string

const (
	EmbeddingJobStatusPending = "pending"
	EmbeddingJobStatusDone    = "done"
	EmbeddingJobStatusFailed  = "failed"
)

type EmbeddingJob struct {
	ID         ULID
	EntityType EntityType
	EntityID   ULID
	Status     EmbeddingJobStatus
}

func (s Store) FetchPendingEmbeddingJobs(limit uint16) ([]EmbeddingJob, error) {
	rows, err := s.DB.Query(`
        select entity_type, entity_id
        from embedding_jobs
        where status in ('pending', 'failed')
        order by updated_at asc
        limit ?
    `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []EmbeddingJob

	for rows.Next() {
		var j EmbeddingJob
		if err := rows.Scan(&j.EntityType, &j.EntityID); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}

	return jobs, rows.Err()
}

func (s Store) MarkJobs(jobs []EmbeddingJob, status EmbeddingJobStatus) error {
	if len(jobs) == 0 {
		return nil
	}

	ids := make([]any, len(jobs))
	placeholders := make([]string, len(jobs))

	for i, job := range jobs {
		ids[i] = job.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		 update embedding_jobs 
		 set status = ? 
		 where id in (%s)
		`,
		strings.Join(placeholders, ","),
	)

	args := append([]any{status}, ids...)

	_, err := s.DB.Exec(query, args...)
	return err
}

func (s Store) MarkJob(jobID ULID, status EmbeddingJobStatus) error {
	_, err := s.DB.Exec(`
		 update embedding_jobs 
		 set status = ? 
		 where id = ?
		`,
		status,
		jobID,
	)
	return err
}

func (s Store) UpsertEmbedding(entityType EntityType, entityID ULID, embedding Embedding) error {
	var table string

	switch entityType {
	case EntityTypeCandidate:
		table = "candidate_embeddings"
	case EntityTypePosition:
		table = "position_embeddings"
	default:
		return fmt.Errorf("unknown entity type: %s", entityType)
	}

	_, err := s.DB.Exec(
		fmt.Sprintf(`
			insert into %s (rowid, embedding)
			values (?, ?)
			on conflict(rowid) do update set embedding = excluded.embedding
		`, table),
		entityID,
		embedding,
	)

	return err
}

func placeholders(n int) string {
	ps := make([]string, n)
	for i := range ps {
		ps[i] = "?"
	}
	return strings.Join(ps, ",")
}

func (s Store) GetPositionsByIDs(ids []ULID) ([]Position, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := `
		select id, title, description, is_active
		from positions
		where id in (` + placeholders(len(ids)) + `)
	`

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []Position

	for rows.Next() {
		var p Position
		if err := rows.Scan(
			&p.ID,
			&p.Title,
			&p.Description,
			&p.IsActive,
		); err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}

	return positions, nil
}
