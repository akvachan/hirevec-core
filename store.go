// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"uuid"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgvector/pgvector-go"
	_ "modernc.org/sqlite"
)

type DatabaseProvider string

const (
	DatabaseProviderPostgreSQL DatabaseProvider = "PostgreSQL"
	DatabaseProviderSQLite     DatabaseProvider = "SQLite"
)

type StoreConfig struct {
	DatabaseProvider      DatabaseProvider
	PostgreSQLDatabaseURL *url.URL

	LlamaCppEmbeddingsDim uint32

	DevelopmentMode                 bool
	DevelopmentModeDropTablesOnExit bool
	DevelopmentModeUserFullName     string
	DevelopmentModeUserName         string
	DevelopmentModeUserEmail        *mail.Address
	DevelopmentModeUserPassword     string
}

var ErrMissingDatabaseURL = errors.New("database URL is not set")

func ConnectPostgreSQL(databaseURL *url.URL) (*sql.DB, error) {
	urlStr := databaseURL.String()
	if urlStr == "" {
		return nil, ErrMissingDatabaseURL
	}

	db, err := sql.Open("pgx", urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.Exec("create extension if not exists vector"); err != nil {
		return nil, fmt.Errorf("failed to create vector extension: %w", err)
	}

	return db, nil
}

var DefaultSQLiteConn = "file:bin/.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

func ConnectSQLite() (*sql.DB, error) {
	db, err := sql.Open("sqlite", DefaultSQLiteConn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	var enabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to scan: %w", err)
	}
	slog.Debug("enabling foreign keys")

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

const SQLDB = `
-- Schema for SQLite and PostgreSQL. Syntax must stay compliant to both databases.

begin transaction;

create table if not exists locations (
  id serial primary key not null,
  country text not null,
  city text not null,
  state text
);

create table if not exists users (
  id text primary key not null, -- UUID
  provider text not null,
  provider_user_id text,
  email text,
  full_name text,
  user_name text unique not null,
  password_hash text,
  updated_at timestamp not null, -- UTC, RFC3339
  check (provider in ('google', 'apple', 'email')),
  unique(provider, provider_user_id)
);

create table if not exists refresh_tokens (
  jti text primary key not null, -- UUID
  user_id text not null, -- UUID
  created_at timestamp not null, -- UTC, RFC3339
  revoked integer not null default 0,
  foreign key (user_id) references users(id) on delete cascade,
  check (revoked in (0, 1))
);

create index if not exists idx_refresh_tokens_user_id
on refresh_tokens(user_id);

create table if not exists candidates (
  id text primary key not null, -- UUID
  user_id text not null, -- UUID
  about text not null,
  pref_remote integer, 
  pref_title_1 text,
  pref_title_2 text,
  pref_title_3 text,
  pref_location_1_id int,
  pref_location_2_id int,
  pref_location_3_id int,
  last_recommended_at timestamp not null, -- UTC, RFC3339
  unique(user_id),
  foreign key (user_id) references users(id) on delete cascade,
  foreign key (pref_location_1_id) references locations(id),
  foreign key (pref_location_2_id) references locations(id),
  foreign key (pref_location_3_id) references locations(id),
  check (pref_remote in (0, 1))
);

create table if not exists candidate_experiences (
  id text primary key not null, -- UUID
  candidate_id text not null, -- UUID
  title text not null,
  started_at timestamp not null, -- UTC, RFC3339
  ended_at timestamp not null, -- UTC, RFC3339
  description text,
  company text,
  experience_type text not null,
  skill_1 text,
  skill_2 text,
  skill_3 text,
  skill_4 text,
  skill_5 text,
  foreign key (candidate_id) references candidates(id) on delete cascade,
  check (experience_type in (
      'work',
      'education',
			'certification', 
      'internship', 
			'personal project', 
			'enterpreneurship', 
			'apprenticeship', 
			'volunteering', 
			'other'
    )
  )
);

create index if not exists idx_candidate_experiences_candidate
on candidate_experiences(candidate_id);

create table if not exists recruiters (
  id text primary key not null, -- UUID
  user_id text not null, -- UUID
  unique(user_id),
  foreign key (user_id) references users(id) on delete cascade
);

create table if not exists positions (
  id text primary key not null, -- UUID
  recruiter_id text not null, -- UUID
  title text not null,
  description text not null,
  company text,
  is_remote integer not null default 0,
  location_id int,
  skill_1 text,
  skill_2 text,
  skill_3 text,
  skill_4 text,
  skill_5 text,
  is_active integer not null default 1,
  created_at timestamp not null, -- UTC, RFC3339
  unique(title, description, company),
  foreign key (recruiter_id) references recruiters(id) on delete cascade,
  foreign key (location_id) references locations(id),
  check (is_active in (0, 1)),
  check (is_remote in (0, 1))
);

create index if not exists idx_positions_active
on positions(is_active);

create table if not exists recommendations (
  id text primary key not null, -- UUID
  position_id text not null, -- UUID
  candidate_id text not null, -- UUID
  foreign key (position_id) references positions(id) on delete cascade,
  foreign key (candidate_id) references candidates(id) on delete cascade,
  unique(position_id, candidate_id)
);

create index if not exists idx_recommendations_candidate_position
on recommendations(candidate_id, position_id);

create index if not exists idx_recommendations_position
on recommendations(position_id);

create index if not exists idx_recommendations_candidate
on recommendations(candidate_id);

create index if not exists idx_recommendations_candidate_id
on recommendations(candidate_id, id);

create table if not exists candidate_reactions (
  recommendation_id text not null, -- UUID
  candidate_id text not null, -- UUID
  reaction_type text not null,
  reacted_at timestamp not null, -- UTC, RFC3339
  primary key (recommendation_id, candidate_id),
  foreign key (recommendation_id) references recommendations(id) on delete cascade,
  foreign key (candidate_id) references candidates(id) on delete cascade,
  check (reaction_type in ('positive', 'negative'))
);

create index if not exists idx_reactions_recommendation
on candidate_reactions(recommendation_id);

create table if not exists recruiter_reactions (
  recommendation_id text not null, -- UUID
  recruiter_id text not null, -- UUID
  reaction_type text not null,
  reacted_at timestamp not null, -- UTC, RFC3339
  primary key (recommendation_id, recruiter_id),
  foreign key (recommendation_id) references recommendations(id) on delete cascade,
  foreign key (recruiter_id) references recruiters(id) on delete cascade,
  check (reaction_type in ('positive', 'negative'))
);

create index if not exists idx_reactions_recommendation
on recruiter_reactions(recommendation_id);

create table if not exists matches (
  candidate_id text not null, -- UUID
  position_id text not null, -- UUID
  matched_at timestamp not null, -- UTC, RFC3339
  primary key (candidate_id, position_id),
  foreign key (candidate_id) references candidates(id) on delete cascade,
  foreign key (position_id) references positions(id) on delete cascade
);

commit;
`

type DevelopmentDataConfig struct {
	UserFullName string
	UserName     string
	UserEmail    *mail.Address
	UserPassword string
}

func (s Store) IngestDevelopmentData(c DevelopmentDataConfig) error {
	if err := ValidateUserPassword(c.UserPassword); err != nil {
		return fmt.Errorf("failed to process user password: %w", err)
	}

	userFullName := "Alex Chen"
	if c.UserFullName != "" {
		userFullName = c.UserFullName
	}

	userName := "alexchen"
	if c.UserName != "" {
		userName = c.UserName
	}

	userEmail := "alex.chen@example.com"
	if c.UserEmail != nil {
		userEmail = c.UserEmail.Address
	}

	exists, err := s.UserExistsByEmail(userEmail, ProviderEmail)
	if err != nil {
		return fmt.Errorf("failed to check for existing development data: %w", err)
	}
	if exists {
		return nil
	}

	locationID := 1
	if _, err := s.Database.Exec(`
		insert into locations (id, country, city)
		values (1, $1, $2)
		on conflict do nothing
	`, "United States", "San Francisco"); err != nil {
		return fmt.Errorf("failed to create development location: %w", err)
	}

	passwordHash, err := HashPassword(c.UserPassword)
	if err != nil {
		return fmt.Errorf("failed to hash development password: %w", err)
	}

	candidateUserID, err := s.CreateUser(ProviderEmail, "", userEmail, userFullName, userName, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to create development candidate user: %w", err)
	}

	candidateID, err := s.CreateCandidate(
		Candidate{
			UserID:          candidateUserID,
			About:           "Senior Full-Stack Engineer with 8+ years building scalable SaaS platforms. Expert in TypeScript, React, Node.js, Go and AWS. Led teams of up to 10 engineers, shipped products used by millions of users, and enjoys mentoring, system design and developer experience.",
			PrefLocation1ID: &locationID,
		},
		[]CandidateExperience{
			{
				Title:          "Senior Full-Stack Engineer",
				StartedAt:      time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC),
				EndedAt:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				Description:    new("Led a team of 10 engineers building a multi-tenant SaaS platform serving millions of users."),
				Company:        new("TechNova"),
				ExperienceType: ExperienceTypeWork,
				Skill1:         new("TypeScript"),
				Skill2:         new("React"),
				Skill3:         new("Node.js"),
				Skill4:         new("Go"),
				Skill5:         new("AWS"),
			},
			{
				Title:          "Full-Stack Developer",
				StartedAt:      time.Date(2018, 5, 1, 0, 0, 0, 0, time.UTC),
				EndedAt:        time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
				Description:    new("Built and shipped customer-facing features across the full stack."),
				Company:        new("BrightApps"),
				ExperienceType: ExperienceTypeWork,
				Skill1:         new("TypeScript"),
				Skill2:         new("React"),
				Skill3:         new("Node.js"),
			},
			{
				Title:          "MSc, Computer Science",
				StartedAt:      time.Date(2016, 9, 1, 0, 0, 0, 0, time.UTC),
				EndedAt:        time.Date(2018, 4, 1, 0, 0, 0, 0, time.UTC),
				Company:        new("State University"),
				ExperienceType: ExperienceTypeEducation,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create development candidate: %w", err)
	}

	recruiter1UserID, err := s.CreateUser(ProviderEmail, "", "sarah@nova.io", "Sarah Williams", "sarahrecruits", passwordHash)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter user: %w", err)
	}
	recruiter1ID, err := s.CreateRecruiter(recruiter1UserID)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter: %w", err)
	}

	recruiter2UserID, err := s.CreateUser(ProviderEmail, "", "michael@brightlabs.io", "Michael Rodriguez", "michaeltalent", passwordHash)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter user: %w", err)
	}
	recruiter2ID, err := s.CreateRecruiter(recruiter2UserID)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter: %w", err)
	}

	recruiter3UserID, err := s.CreateUser(ProviderEmail, "", "emily@cloudforge.io", "Emily Johnson", "emilyhires", passwordHash)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter user: %w", err)
	}
	recruiter3ID, err := s.CreateRecruiter(recruiter3UserID)
	if err != nil {
		return fmt.Errorf("failed to create development recruiter: %w", err)
	}

	position1ID, err := s.CreatePosition(Position{
		RecruiterID: recruiter1ID,
		Title:       "Senior Full Stack Engineer",
		Description: "Build customer-facing SaaS applications with React, TypeScript and Node.js.",
		Company:     "Nova AI",
		IsRemote:    true,
		IsActive:    true,
	})
	if err != nil {
		return fmt.Errorf("failed to create development position: %w", err)
	}

	position2ID, err := s.CreatePosition(Position{
		RecruiterID: recruiter2ID,
		Title:       "Staff Backend Engineer",
		Description: "Design distributed systems and APIs powering cloud infrastructure.",
		Company:     "Bright Labs",
		IsActive:    true,
	})
	if err != nil {
		return fmt.Errorf("failed to create development position: %w", err)
	}

	position3ID, err := s.CreatePosition(Position{
		RecruiterID: recruiter3ID,
		Title:       "Engineering Manager",
		Description: "Lead a high-performing product engineering team.",
		Company:     "CloudForge",
		IsActive:    true,
	})
	if err != nil {
		return fmt.Errorf("failed to create development position: %w", err)
	}

	// Positions 4 and 5 are intentionally left without recommendations so the
	// recommendation engine's text search path has something to discover.
	if _, err = s.CreatePosition(Position{
		RecruiterID: recruiter1ID,
		Title:       "Principal Platform Engineer",
		Description: "Drive platform reliability, developer productivity and cloud architecture.",
		Company:     "Nova AI",
		IsRemote:    true,
		IsActive:    true,
	}); err != nil {
		return fmt.Errorf("failed to create development position: %w", err)
	}

	if _, err = s.CreatePosition(Position{
		RecruiterID: recruiter2ID,
		Title:       "Lead Solutions Architect",
		Description: "Partner with enterprise customers on large-scale cloud adoption.",
		Company:     "Bright Labs",
		IsRemote:    true,
		IsActive:    true,
	}); err != nil {
		return fmt.Errorf("failed to create development position: %w", err)
	}

	recommendation1ID, err := s.CreateRecommendation(position1ID, candidateID)
	if err != nil {
		return fmt.Errorf("failed to create development recommendation: %w", err)
	}

	recommendation2ID, err := s.CreateRecommendation(position2ID, candidateID)
	if err != nil {
		return fmt.Errorf("failed to create development recommendation: %w", err)
	}

	recommendation3ID, err := s.CreateRecommendation(position3ID, candidateID)
	if err != nil {
		return fmt.Errorf("failed to create development recommendation: %w", err)
	}

	if err := s.CreateRecruiterReaction(recommendation1ID, recruiter1ID, ReactionTypePositive); err != nil {
		return fmt.Errorf("failed to create development recruiter reaction: %w", err)
	}
	if err := s.CreateRecruiterReaction(recommendation2ID, recruiter2ID, ReactionTypePositive); err != nil {
		return fmt.Errorf("failed to create development recruiter reaction: %w", err)
	}
	if err := s.CreateRecruiterReaction(recommendation3ID, recruiter3ID, ReactionTypePositive); err != nil {
		return fmt.Errorf("failed to create development recruiter reaction: %w", err)
	}

	return nil
}

// TODO: Update FTS to include all fields

const SQLFTSPostgreSQL = `
-- Text search setup for PostgreSQL.

begin transaction;

-- Positions FTS
alter table positions 
add column if not exists search_vector tsvector generated always as (to_tsvector('english', title || ' ' || description || ' ' || coalesce(company, ''))) stored;

create index if not exists idx_positions_search on positions using gin(search_vector);

-- Candidates FTS
alter table candidates 
add column if not exists search_vector tsvector generated always as (to_tsvector('english', coalesce(about, ''))) stored;

create index if not exists idx_candidates_search on candidates using gin(search_vector);

commit;
`

// TODO: Update FTS to include all fields

const SQLFTSSQlite = `
-- Text search setup for SQLite.

begin transaction;

-- Positions FTS Setup
create virtual table if not exists positions_fts using fts5(
  id unindexed,
  title,
  description,
  company
);

create trigger if not exists positions_ai after insert on positions
begin
  insert into positions_fts(id, title, description, company)
  values (new.id, new.title, new.description, new.company);
end;

create trigger if not exists positions_ad after delete on positions
begin
  delete from positions_fts where id = old.id;
end;

create trigger if not exists positions_au after update on positions
begin
  delete from positions_fts where id = old.id;
  insert into positions_fts(id, title, description, company)
  values (new.id, new.title, new.description, new.company);
end;

-- Candidates FTS Setup
create virtual table if not exists candidates_fts using fts5(
  id unindexed,
  about
);

create trigger if not exists candidates_ai after insert on candidates
begin
  insert into candidates_fts(id, about)
  values (new.id, new.about);
end;

create trigger if not exists candidates_ad after delete on candidates
begin
  delete from candidates_fts where id = old.id;
end;

create trigger if not exists candidates_au after update on candidates
begin
  delete from candidates_fts where id = old.id;
  insert into candidates_fts(id, about)
  values (new.id, new.about);
end;

commit;
`

type Store struct {
	DatabaseProvider DatabaseProvider
	Database         *sql.DB
}

var ErrUnsupportedDatabaseProvider = errors.New("unsupported database provider")

func NewStore(c StoreConfig) (Store, error) {
	slog.Debug("initializing store")

	s := Store{}
	var err error

	slog.Debug("connecting to database provider", "database", c.DatabaseProvider)
	switch c.DatabaseProvider {
	case DatabaseProviderPostgreSQL:
		if s.Database, err = ConnectPostgreSQL(c.PostgreSQLDatabaseURL); err != nil {
			return Store{}, fmt.Errorf("failed to connect to database: %w", err)
		}

		slog.Debug("creating database schema")
		if _, err := s.Database.Exec(SQLDB); err != nil {
			return Store{}, fmt.Errorf("failed to create database schema: %w", err)
		}

		slog.Debug("creating FTS schema")
		if _, err := s.Database.Exec(SQLFTSPostgreSQL); err != nil {
			return Store{}, fmt.Errorf("failed to create FTS schema: %w", err)
		}

		slog.Debug("creating embeddings schema")
		if _, err := s.Database.Exec(
			fmt.Sprintf(`
				create table if not exists embeddings (
					id bigserial primary key,
					entity_id text not null, -- UUID
					entity_type text not null,
					entity_text text not null,
					embedding vector(%d),
					embedding_status text not null,
					embedding_updated_at timestamp not null, -- UTC, RFC3339
					check (entity_type in ('candidate', 'position')),
					check (embedding_status in ('done', 'pending', 'failed'))
				)
			`, c.LlamaCppEmbeddingsDim),
		); err != nil {
			return Store{}, fmt.Errorf("failed to create embeddings schema: %w", err)
		}
		if _, err := s.Database.Exec(
			`
			create unique index if not exists idx_embeddings_entity_id
			on embeddings(entity_id);
			`,
		); err != nil {
			return Store{}, fmt.Errorf("failed to create index on embeddings table: %w", err)
		}

	case DatabaseProviderSQLite:
		if s.Database, err = ConnectSQLite(); err != nil {
			return Store{}, fmt.Errorf("failed to connect to SQLite: %w", err)
		}

		slog.Debug("creating database schema")
		if _, err := s.Database.Exec(SQLDB); err != nil {
			return Store{}, fmt.Errorf("failed to create database schema: %w", err)
		}

		slog.Debug("creating FTS schema")
		if _, err := s.Database.Exec(SQLFTSSQlite); err != nil {
			return Store{}, fmt.Errorf("failed to create FTS schema: %w", err)
		}

	default:
		return s, ErrUnsupportedDatabaseProvider
	}

	s.DatabaseProvider = c.DatabaseProvider

	if c.DevelopmentMode {
		slog.Debug("ingesting development data", "development_mode", c.DevelopmentMode)
		if err := s.IngestDevelopmentData(DevelopmentDataConfig{
			UserEmail:    c.DevelopmentModeUserEmail,
			UserPassword: c.DevelopmentModeUserPassword,
		}); err != nil {
			if c.DevelopmentModeDropTablesOnExit {
				if err := s.DropTables(); err != nil {
					slog.Error("failed to drop tables", "err", err, "development_mode", c.DevelopmentMode)
				}
				slog.Debug("dropped tables on exit", "development_mode", c.DevelopmentMode)
			}
			return Store{}, fmt.Errorf("failed to ingest development data: %w", err)
		}
	}

	return s, nil
}

var ErrRecommendationNotFound = errors.New("recommendation not found")

func (s Store) GetRecommendation(recommendationID uuid.UUID) (Recommendation, error) {
	var candidateID, positionID uuid.UUID

	if err := s.Database.QueryRow(`
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

func (s Store) GetUserAndRolesByEmail(email string, provider Provider) (User, map[Role]uuid.UUID, error) {
	var userID uuid.UUID
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash string
	var candidateID, recruiterID sql.NullString

	if err := s.Database.QueryRow(`
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

	roles := make(map[Role]uuid.UUID, 2)
	var err error
	if candidateID.Valid {
		if roles[RoleCandidate], err = uuid.Parse(candidateID.String); err != nil {
			return user, roles, err
		}
	}
	if recruiterID.Valid {
		if roles[RoleRecruiter], err = uuid.Parse(recruiterID.String); err != nil {
			return user, roles, err
		}
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUserIDAndRolesByProvider(provider Provider, providerUserID string) (uuid.UUID, map[Role]uuid.UUID, error) {
	var userID uuid.UUID
	var candidateID, recruiterID sql.NullString

	if err := s.Database.QueryRow(`
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
			return uuid.Nil(), nil, ErrUserNotFound
		}
		return uuid.Nil(), nil, fmt.Errorf("failed to scan: %w", err)
	}

	roles := make(map[Role]uuid.UUID, 2)
	var err error
	if candidateID.Valid {
		if roles[RoleCandidate], err = uuid.Parse(candidateID.String); err != nil {
			return uuid.Nil(), nil, fmt.Errorf("failed to parse candidateID: %w", err)
		}
	}
	if recruiterID.Valid {
		if roles[RoleRecruiter], err = uuid.Parse(recruiterID.String); err != nil {
			return uuid.Nil(), nil, fmt.Errorf("failed to parse recruiterID: %w", err)
		}
	}
	if len(roles) == 0 {
		return userID, nil, ErrUserNoRole
	}

	return userID, roles, nil
}

func (s Store) GetUserRoles(userID uuid.UUID, provider Provider) (map[Role]uuid.UUID, error) {
	var candidateID, recruiterID sql.NullString
	err := s.Database.QueryRow(`
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

	roles := make(map[Role]uuid.UUID, 2)
	if candidateID.Valid {
		if roles[RoleCandidate], err = uuid.Parse(candidateID.String); err != nil {
			return nil, fmt.Errorf("failed to parse candidateID: %w", err)
		}
	}
	if recruiterID.Valid {
		if roles[RoleRecruiter], err = uuid.Parse(recruiterID.String); err != nil {
			return nil, fmt.Errorf("failed to parse recruiterID: %w", err)
		}
	}
	if len(roles) == 0 {
		return nil, ErrUserNoRole
	}

	return roles, nil
}

type User struct {
	ID             uuid.UUID `json:"id"`
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

func (s Store) CreateUser(provider Provider, providerUserID string, email string, fullName string, userName string, passwordHash string) (uuid.UUID, error) {
	id := uuid.NewV7()
	result, err := s.Database.Exec(`
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
		return uuid.Nil(), fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return uuid.Nil(), ErrUserAlreadyExists
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
	RecommendationID uuid.UUID    `json:"recommendation_id"`
	ReactionType     ReactionType `json:"reaction_type"`
	ReactedAt        time.Time    `json:"reacted_at"`
}

var (
	ErrReactionAlreadyExists = errors.New("reaction already exists")
	ErrUnauthorizedCandidat  = errors.New("reactor with the reactorID is unauthorized")
	ErrUnauthorizedReactor   = errors.New("reactor with the reactorID is unauthorized")
)

func (s Store) CreateCandidateReaction(recommendationID uuid.UUID, candidateID uuid.UUID, reactionType ReactionType) error {
	result, err := s.Database.Exec(`
		insert into candidate_reactions (
			recommendation_id,
			candidate_id,
			reaction_type,
			reacted_at
		)
		values ($1, $2, $3, $4)
	`, recommendationID, candidateID, reactionType, CurrentTimestamp())
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

	return fmt.Errorf("failed to create reaction for an unknown reason")
}

func (s Store) CreateRecruiterReaction(recommendationID uuid.UUID, recruiterID uuid.UUID, reactionType ReactionType) error {
	result, err := s.Database.Exec(`
		insert into recruiter_reactions (
			recommendation_id,
			recruiter_id,
			reaction_type,
			reacted_at
		)
		values ($1, $2, $3, $4)
	`, recommendationID, recruiterID, reactionType, CurrentTimestamp())
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

	return fmt.Errorf("failed to create reaction for an unknown reason")
}

func (s Store) IsRevokedRefreshToken(jti uuid.UUID) (bool, error) {
	var isRevoked bool

	if err := s.Database.QueryRow(`
		select revoked 
		from refresh_tokens 
		where jti = $1 
	`, jti).Scan(&isRevoked); err != nil {
		return true, fmt.Errorf("failed to scan: %w", err)
	}

	return isRevoked, nil
}

const DefaultMaxRefreshTokensCount = 5

func (s Store) CreateRefreshToken(userID uuid.UUID) (jti uuid.UUID, err error) {
	jti = uuid.NewV7()

	tx, err := s.Database.Begin()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to start transaction: %w", err)
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
	`, userID).Scan(&count); err != nil {
		return uuid.Nil(), fmt.Errorf("failed to scan: %w", err)
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
				order by created_at desc
				limit 1
			)
		`, userID); err != nil {
			return uuid.Nil(), fmt.Errorf("failed to execute SQL: %w", err)
		}
	}

	if _, err = tx.Exec(`
		insert into refresh_tokens (jti, user_id, created_at)
		values ($1, $2, $3, $4)
	`, jti, userID, currentTimestamp); err != nil {
		return uuid.Nil(), fmt.Errorf("failed to execute SQL: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil(), fmt.Errorf("failed to commit transaction: %w", err)
	}

	return jti, nil
}

type Recommendation struct {
	ID          uuid.UUID `json:"id"`
	PositionID  uuid.UUID `json:"position_id"`
	CandidateID uuid.UUID `json:"candidate_id"`
}

var ErrRecommendationAlreadyExists = errors.New("recommendation already exists")

func (s Store) CreateRecommendation(positionID uuid.UUID, candidateID uuid.UUID) (uuid.UUID, error) {
	id := uuid.NewV7()

	result, err := s.Database.Exec(`
		insert into recommendations (id, position_id, candidate_id)
		values ($1, $2, $3)
		on conflict do nothing
	`, id, positionID, candidateID)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return uuid.Nil(), ErrRecommendationAlreadyExists
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
	RecommendationID    uuid.UUID `json:"recommendation_id"`
	PositionID          uuid.UUID `json:"position_id"`
	PositionTitle       string    `json:"position_title"`
	PositionCompany     string    `json:"position_company"`
	PositionDescription string    `json:"position_description"`
	PositionIsRemote    bool      `json:"position_is_remote"`
	PositionLocation    *Location `json:"position_location"`
	PositionSkills      []string  `json:"position_skills"`
}

func (s Store) GetRecommendationsForCandidate(candidateID uuid.UUID, page Page, excludeReacted bool) ([]RecommendationForCandidate, Page, error) {
	rows, err := s.Database.Query(`
		select
			r.id,
			p.id,
			p.title,
			coalesce(p.company, ''),
			p.description,
			p.is_remote,
			l.id, coalesce(l.country, ''), coalesce(l.city, ''), l.state,
			p.skill_1, p.skill_2, p.skill_3, p.skill_4, p.skill_5
		from recommendations r
		join positions p on p.id = r.position_id
		left join locations l on l.id = p.location_id
		where r.candidate_id = $1
				and p.is_active = 1
				and ($2 = '' or r.id > $2)
				and (not $4 or not exists (
						select 1
						from candidate_reactions cr
						where cr.recommendation_id = r.id
							and cr.candidate_id = $1
				))
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
		var (
			recommendation                         RecommendationForCandidate
			isRemote                               int
			locID                                  *int
			country, city                          string
			state                                  *string
			skill1, skill2, skill3, skill4, skill5 *string
		)
		if err := rows.Scan(
			&recommendation.RecommendationID,
			&recommendation.PositionID,
			&recommendation.PositionTitle,
			&recommendation.PositionCompany,
			&recommendation.PositionDescription,
			&isRemote,
			&locID,
			&country, &city, &state,
			&skill1, &skill2, &skill3, &skill4, &skill5,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}

		recommendation.PositionIsRemote = isRemote == 1

		if locID != nil {
			recommendation.PositionLocation = &Location{
				ID:      *locID,
				Country: country,
				City:    city,
				State:   state,
			}
		}

		recommendation.PositionSkills = make([]string, 0, 5)
		if skill1 != nil && *skill1 != "" {
			recommendation.PositionSkills = append(recommendation.PositionSkills, *skill1)
		}
		if skill2 != nil && *skill2 != "" {
			recommendation.PositionSkills = append(recommendation.PositionSkills, *skill2)
		}
		if skill3 != nil && *skill3 != "" {
			recommendation.PositionSkills = append(recommendation.PositionSkills, *skill3)
		}
		if skill4 != nil && *skill4 != "" {
			recommendation.PositionSkills = append(recommendation.PositionSkills, *skill4)
		}
		if skill5 != nil && *skill5 != "" {
			recommendation.PositionSkills = append(recommendation.PositionSkills, *skill5)
		}

		recommendations = append(recommendations, recommendation)
	}
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	hasNext := len(recommendations) > page.Limit
	var nextCursor uuid.UUID
	if hasNext {
		nextCursor = recommendations[page.Limit-1].RecommendationID
		recommendations = recommendations[:page.Limit]
	}

	return recommendations, Page{
		Cursor:  nextCursor.String(),
		Limit:   page.Limit,
		Count:   len(recommendations),
		HasNext: hasNext,
	}, nil
}

type RecommendationForRecruiter struct {
	RecommendationID       uuid.UUID             `json:"recommendation_id"`
	PositionID             uuid.UUID             `json:"position_id"`
	PositionTitle          string                `json:"position_title"`
	CandidateID            uuid.UUID             `json:"candidate_id"`
	CandidateFullName      string                `json:"candidate_full_name"`
	CandidateAbout         string                `json:"candidate_about"`
	CandidatePrefRemote    *bool                 `json:"candidate_pref_remote"`
	CandidatePrefTitles    []string              `json:"candidate_pref_titles"`
	CandidatePrefLocations []Location            `json:"candidate_pref_locations"`
	CandidateExperiences   []CandidateExperience `json:"candidate_experiences"`
}

func (s Store) GetRecommendationsForRecruiter(recruiterID uuid.UUID, page Page, includeReacted bool) ([]RecommendationForRecruiter, Page, error) {
	var jsonAggExpr string
	switch s.DatabaseProvider {
	case DatabaseProviderPostgreSQL:
		jsonAggExpr = `
			coalesce(json_agg(
				json_build_object(
					'id', ce.id,
					'title', ce.title,
					'started_at', ce.started_at,
					'ended_at', ce.ended_at,
					'description', ce.description,
					'company', ce.company,
					'experience_type', ce.experience_type,
					'skill_1', ce.skill_1,
					'skill_2', ce.skill_2,
					'skill_3', ce.skill_3,
					'skill_4', ce.skill_4,
					'skill_5', ce.skill_5
				)
			), '[]'::json)`
	case DatabaseProviderSQLite:
		jsonAggExpr = `
			coalesce(json_group_array(
				json_object(
					'id', ce.id,
					'title', ce.title,
					'started_at', ce.started_at,
					'ended_at', ce.ended_at,
					'description', ce.description,
					'company', ce.company,
					'experience_type', ce.experience_type,
					'skill_1', ce.skill_1,
					'skill_2', ce.skill_2,
					'skill_3', ce.skill_3,
					'skill_4', ce.skill_4,
					'skill_5', ce.skill_5
				)
			), '[]')`
	default:
		return nil, Page{}, ErrUnsupportedDatabaseProvider
	}

	rows, err := s.Database.Query(fmt.Sprintf(`
		select
			r.id,
			p.id,
			p.title,
			c.id,
			u.full_name,
			c.about,
			c.pref_remote,
			c.pref_title_1, c.pref_title_2, c.pref_title_3,
			l1.id, coalesce(l1.country, ''), coalesce(l1.city, ''), l1.state,
			l2.id, coalesce(l2.country, ''), coalesce(l2.city, ''), l2.state,
			l3.id, coalesce(l3.country, ''), coalesce(l3.city, ''), l3.state,
			(
				select %s
				from candidate_experiences ce
				where ce.candidate_id = c.id
			) as experiences
		from recommendations r
		join positions p on p.id = r.position_id
		join candidates c on c.id = r.candidate_id
		join users u on u.id = c.user_id
		left join locations l1 on l1.id = c.pref_location_1_id
		left join locations l2 on l2.id = c.pref_location_2_id
		left join locations l3 on l3.id = c.pref_location_3_id
		where p.recruiter_id = $1
				and ($2 = '' or r.id > $2)
				and ($4 or not exists (
						select 1
						from recruiter_reactions rr
						where rr.recommendation_id = r.id
							and rr.recruiter_id = $1
				))
		order by r.id desc
		limit $3
	`, jsonAggExpr), recruiterID, page.Cursor, page.Limit+1, includeReacted)
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
		var (
			recommendation                     RecommendationForRecruiter
			prefRemote                         *bool
			prefTitle1, prefTitle2, prefTitle3 *string
			loc1ID                             *int
			loc1Country, loc1City              string
			loc1State                          *string
			loc2ID                             *int
			loc2Country, loc2City              string
			loc2State                          *string
			loc3ID                             *int
			loc3Country, loc3City              string
			loc3State                          *string
			experiencesJSON                    []byte
		)
		if err := rows.Scan(
			&recommendation.RecommendationID,
			&recommendation.PositionID,
			&recommendation.PositionTitle,
			&recommendation.CandidateID,
			&recommendation.CandidateFullName,
			&recommendation.CandidateAbout,
			&prefRemote,
			&prefTitle1, &prefTitle2, &prefTitle3,
			&loc1ID, &loc1Country, &loc1City, &loc1State,
			&loc2ID, &loc2Country, &loc2City, &loc2State,
			&loc3ID, &loc3Country, &loc3City, &loc3State,
			&experiencesJSON,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		if err := rows.Err(); err != nil {
			return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
		}

		recommendation.CandidatePrefRemote = prefRemote

		recommendation.CandidatePrefTitles = make([]string, 0, 3)
		if prefTitle1 != nil && *prefTitle1 != "" {
			recommendation.CandidatePrefTitles = append(recommendation.CandidatePrefTitles, *prefTitle1)
		}
		if prefTitle2 != nil && *prefTitle2 != "" {
			recommendation.CandidatePrefTitles = append(recommendation.CandidatePrefTitles, *prefTitle2)
		}
		if prefTitle3 != nil && *prefTitle3 != "" {
			recommendation.CandidatePrefTitles = append(recommendation.CandidatePrefTitles, *prefTitle3)
		}

		recommendation.CandidatePrefLocations = make([]Location, 0, 3)
		if loc1ID != nil {
			recommendation.CandidatePrefLocations = append(recommendation.CandidatePrefLocations, Location{
				ID:      *loc1ID,
				Country: loc1Country,
				City:    loc1City,
				State:   loc1State,
			})
		}
		if loc2ID != nil {
			recommendation.CandidatePrefLocations = append(recommendation.CandidatePrefLocations, Location{
				ID:      *loc2ID,
				Country: loc2Country,
				City:    loc2City,
				State:   loc2State,
			})
		}
		if loc3ID != nil {
			recommendation.CandidatePrefLocations = append(recommendation.CandidatePrefLocations, Location{
				ID:      *loc3ID,
				Country: loc3Country,
				City:    loc3City,
				State:   loc3State,
			})
		}

		if err := json.Unmarshal(experiencesJSON, &recommendation.CandidateExperiences); err != nil {
			return nil, Page{}, fmt.Errorf("failed to unmarshal experiences: %w", err)
		}

		if recommendation.CandidateExperiences == nil {
			recommendation.CandidateExperiences = []CandidateExperience{}
		}

		recommendations = append(recommendations, recommendation)
	}

	hasNext := len(recommendations) > page.Limit
	var nextCursor uuid.UUID
	if hasNext {
		nextCursor = recommendations[page.Limit-1].RecommendationID
		recommendations = recommendations[:page.Limit]
	}

	return recommendations, Page{
		Cursor:  nextCursor.String(),
		Limit:   page.Limit,
		Count:   len(recommendations),
		HasNext: hasNext,
	}, nil
}

func (s Store) GetReactionsForRecruiter(recruiterID uuid.UUID, page Page) ([]Reaction, Page, error) {
	rows, err := s.Database.Query(`
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
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = results[page.Limit-1].RecommendationID.String()
	}

	return results, nextPage, nil
}

func (s Store) GetReactionsForCandidate(candidateID uuid.UUID, page Page) ([]Reaction, Page, error) {
	rows, err := s.Database.Query(`
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
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = results[page.Limit-1].RecommendationID.String()
	}

	return results, nextPage, nil
}

type MatchForCandidate struct {
	PositionID        uuid.UUID `json:"position_id"`
	PositionTitle     string    `json:"position_title"`
	PositionCompany   string    `json:"position_company"`
	RecruiterID       uuid.UUID `json:"recruiter_id"`
	RecruiterFullName string    `json:"recruiter_full_name"`
	MatchedAt         time.Time `json:"matched_at"`
}

func (s Store) GetMatchesForCandidate(candidateID uuid.UUID, page Page) ([]MatchForCandidate, Page, error) {
	rows, err := s.Database.Query(`
		select
			m.position_id,
			p.title,
			coalesce(p.company, ''),
			p.recruiter_id,
			ru.full_name,
			m.matched_at
		from matches m
		join positions p on p.id = m.position_id
		join recruiters r on r.id = p.recruiter_id
		join users ru on ru.id = r.user_id
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
			&m.PositionTitle,
			&m.PositionCompany,
			&m.RecruiterID,
			&m.RecruiterFullName,
			&m.MatchedAt,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = results[page.Limit-1].PositionID.String()
	}

	return results, nextPage, nil
}

type MatchForRecruiter struct {
	CandidateID       uuid.UUID `json:"candidate_id"`
	CandidateFullName string    `json:"candidate_full_name"`
	PositionID        uuid.UUID `json:"position_id"`
	PositionTitle     string    `json:"position_title"`
}

func (s Store) GetMatchesForRecruiter(recruiterID uuid.UUID, page Page) ([]MatchForRecruiter, Page, error) {
	rows, err := s.Database.Query(`
		select
			c.id,
			u.full_name,
			m.position_id,
			p.title
		from matches m
		join positions p on p.id = m.position_id
		join candidates c on c.id = m.candidate_id
		join users u on u.id = c.user_id
		where p.recruiter_id = $1
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
			&m.CandidateID,
			&m.CandidateFullName,
			&m.PositionID,
			&m.PositionTitle,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	var nextPage Page
	if len(results) > page.Limit {
		results = results[:page.Limit]
		nextPage.Cursor = results[page.Limit-1].PositionID.String()
	}

	return results, nextPage, nil
}

type EmbeddingStatus string

const (
	EmbeddingStatusPending = "pending"
	EmbeddingStatusDone    = "done"
	EmbeddingStatusFailed  = "failed"
)

const (
	EmbeddingEntityTypeCandidate = "candidate"
	EmbeddingEntityTypePosition  = "position"
)

func (s Store) UpsertPendingEmbedding(entityID uuid.UUID, entityType string, text string) error {
	if _, err := s.Database.Exec(`
		insert into embeddings (entity_id, entity_type, entity_text, embedding_status, embedding_updated_at)
		values ($1, $2, $3, $4, $5)
		on conflict (entity_id) do update set
			entity_type = excluded.entity_type,
			entity_text = excluded.entity_text,
			embedding_status = excluded.embedding_status,
			embedding_updated_at = excluded.embedding_updated_at
	`, entityID, entityType, text, EmbeddingStatusPending, CurrentTimestamp(0)); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) FetchPendingEmbeddings(limit uint16) ([]uuid.UUID, []string, error) {
	rows, err := s.Database.Query(`
		select entity_id, entity_text
		from embeddings
		where embedding_status in ('pending', 'failed')
		order by embedding_updated_at asc, id asc
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

	ids := make([]uuid.UUID, 0, limit)
	texts := make([]string, 0, limit)
	for rows.Next() {
		var id string
		var text string

		if err := rows.Scan(&id, &text); err != nil {
			return nil, nil, fmt.Errorf("failed to scan: %w", err)
		}

		uuid, err := uuid.Parse(id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse entity_id: %w", err)
		}

		ids = append(ids, uuid)
		texts = append(texts, text)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return ids, texts, nil
}

func (s Store) MarkEmbeddingsStatus(entityIDs []uuid.UUID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	if _, err := s.Database.Exec(`
		update embeddings
		set embedding_status = $1,
			embedding_updated_at = $2
		where entity_id = any($3)
	`, status, CurrentTimestamp(0), entityIDs); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) MarkEmbeddingsStatusTx(tx *sql.Tx, entityIDs []uuid.UUID, status EmbeddingStatus) error {
	if len(entityIDs) == 0 {
		return nil
	}

	if _, err := tx.Exec(`
		update embeddings
		set embedding_status = $1,
			embedding_updated_at = $2
		where entity_id = any($3)
	`, status, CurrentTimestamp(0), entityIDs); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}

func (s Store) GetPositionsForCandidateViaEmbeddings(candidateID uuid.UUID, topPositions uint16) ([]uuid.UUID, error) {
	var prefRemote *bool
	var prefLocation1ID, prefLocation2ID, prefLocation3ID *int
	err := s.Database.QueryRow(`
		select pref_remote, pref_location_1_id, pref_location_2_id, pref_location_3_id
		from candidates
		where id = $1
	`, candidateID).Scan(&prefRemote, &prefLocation1ID, &prefLocation2ID, &prefLocation3ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []uuid.UUID{}, ErrCandidateNotFound
		}
		return nil, fmt.Errorf("failed to scan: %w", err)
	}

	rows, err := s.Database.Query(`
		with candidate as (
				select embedding
				from embeddings
				where entity_id = $1
						and entity_type = 'candidate'
						and embedding_status = 'done'
				limit 1
		)
		select p.id as position_id
		from positions p
		join embeddings e
				on e.entity_id = p.id
		cross join candidate c
		where p.is_active = 1
				and e.embedding_status = 'done'
				and not exists (
						select 1
						from recommendations r
						where r.position_id = p.id
							and r.candidate_id = $1
				)
				and ($3::boolean is null or not $3::boolean or p.is_remote = 1)
				and (
						$4::int is null and $5::int is null and $6::int is null
						or p.is_remote = 1
						or p.location_id is null
						or p.location_id in ($4, $5, $6)
				)
		order by e.embedding <=> c.embedding
		limit $2
	`, candidateID, topPositions, prefRemote, prefLocation1ID, prefLocation2ID, prefLocation3ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]uuid.UUID, 0, topPositions)
	for rows.Next() {
		var positionID string
		if err := rows.Scan(&positionID); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		uuid, err := uuid.Parse(positionID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse position_id: %w", err)
		}

		results = append(results, uuid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return results, nil
}

func BuildPositionText(title, description, company string, skill1, skill2, skill3, skill4, skill5 *string) string {
	var builder strings.Builder
	builder.WriteString(title)
	builder.WriteString(" ")
	builder.WriteString(description)
	if company != "" {
		builder.WriteString(" ")
		builder.WriteString(company)
	}
	if skill1 != nil && *skill1 != "" {
		builder.WriteString(" ")
		builder.WriteString(*skill1)
	}
	if skill2 != nil && *skill2 != "" {
		builder.WriteString(" ")
		builder.WriteString(*skill2)
	}
	if skill3 != nil && *skill3 != "" {
		builder.WriteString(" ")
		builder.WriteString(*skill3)
	}
	if skill4 != nil && *skill4 != "" {
		builder.WriteString(" ")
		builder.WriteString(*skill4)
	}
	if skill5 != nil && *skill5 != "" {
		builder.WriteString(" ")
		builder.WriteString(*skill5)
	}
	return builder.String()
}

func BuildCandidateText(about string, experiences []CandidateExperience) string {
	var builder strings.Builder
	builder.WriteString(about)
	for _, experience := range experiences {
		builder.WriteString(" ")
		builder.WriteString(experience.Title)
		if experience.Description != nil && *experience.Description != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Description)
		}
		if experience.Company != nil && *experience.Company != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Company)
		}
		if experience.Skill1 != nil && *experience.Skill1 != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Skill1)
		}
		if experience.Skill2 != nil && *experience.Skill2 != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Skill2)
		}
		if experience.Skill3 != nil && *experience.Skill3 != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Skill3)
		}
		if experience.Skill4 != nil && *experience.Skill4 != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Skill4)
		}
		if experience.Skill5 != nil && *experience.Skill5 != "" {
			builder.WriteString(" ")
			builder.WriteString(*experience.Skill5)
		}
	}
	return builder.String()
}

func (s Store) GetPositionsTexts(positionIDs []uuid.UUID) ([]string, error) {
	if len(positionIDs) == 0 {
		return make([]string, 0), nil
	}

	placeholders := make([]string, len(positionIDs))
	args := make([]any, len(positionIDs))
	for i, positionID := range positionIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = positionID
	}

	rows, err := s.Database.Query(fmt.Sprintf(`
		select
			title,
			description,
			coalesce(company, ''),
			skill_1,
			skill_2,
			skill_3,
			skill_4,
			skill_5
		from positions
		where id in (%s)
		order by id
	`, strings.Join(placeholders, ", ")), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	texts := make([]string, 0, len(positionIDs))
	for rows.Next() {
		var (
			title       string
			description string
			company     string
			skill1      *string
			skill2      *string
			skill3      *string
			skill4      *string
			skill5      *string
		)
		if err := rows.Scan(
			&title,
			&description,
			&company,
			&skill1,
			&skill2,
			&skill3,
			&skill4,
			&skill5,
		); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		texts = append(texts, BuildPositionText(title, description, company, skill1, skill2, skill3, skill4, skill5))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return texts, nil
}

var ErrEmbeddingsCountConflict = errors.New("mismatch between count of embedding IDs and embeddings")

func (s Store) UpsertEmbeddingsTx(tx *sql.Tx, embeddingIDs []uuid.UUID, embeddings []EmbeddingEntity) error {
	if len(embeddingIDs) == 0 {
		return nil
	}
	if len(embeddingIDs) != len(embeddings) {
		return ErrEmbeddingsCountConflict
	}

	update, err := tx.Prepare(`
		update embeddings
		set embedding = $2
		where entity_id = $1
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() {
		if err := update.Close(); err != nil {
			slog.Error("failed to close update statement", "err", err)
		}
	}()

	for i := range embeddingIDs {
		if _, err := update.Exec(
			embeddingIDs[i],
			pgvector.NewVector(embeddings[i].Embedding),
		); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	return nil
}

func (s Store) GetCandidates(limit uint16, recommendationSpan time.Duration) ([]uuid.UUID, error) {
	cutoff := CurrentTimestamp(-recommendationSpan)
	rows, err := s.Database.Query(`
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

	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		uuid, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("failed to parse candidate_id: %w", err)
		}

		ids = append(ids, uuid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return ids, nil
}

type Recruiter struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"user_id"`
}

var ErrRecruiterAlreadyExists = errors.New("recruiter already exists")

func (s Store) CreateRecruiter(userID uuid.UUID) (uuid.UUID, error) {
	id := uuid.NewV7()

	result, err := s.Database.Exec(`
		insert into recruiters (id, user_id)
		values ($1, $2)
		on conflict (user_id) do nothing
	`, id, userID)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to exec SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return uuid.Nil(), ErrRecruiterAlreadyExists
	}

	return id, nil
}

type Location struct {
	ID      int     `json:"id"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	State   *string `json:"state"`
}

type Position struct {
	ID          uuid.UUID `json:"id"`
	RecruiterID uuid.UUID `json:"recruiter_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Company     string    `json:"company"`
	IsRemote    bool      `json:"is_remote"`
	LocationID  *int      `json:"location_id"`
	Skill1      *string   `json:"skill_1"`
	Skill2      *string   `json:"skill_2"`
	Skill3      *string   `json:"skill_3"`
	Skill4      *string   `json:"skill_4"`
	Skill5      *string   `json:"skill_5"`
	IsActive    bool      `json:"is_active"`
}

var ErrPositionAlreadyExists = errors.New("position already exists")

func (s Store) CreatePosition(position Position) (uuid.UUID, error) {
	position.ID = uuid.NewV7()

	isRemote := 0
	if position.IsRemote {
		isRemote = 1
	}

	isActive := 0
	if position.IsActive {
		isActive = 1
	}

	result, err := s.Database.Exec(
		`
		insert into positions (
			id,
			recruiter_id,
			title,
			description,
			company,
			is_remote,
			location_id,
			skill_1,
			skill_2,
			skill_3,
			skill_4,
			skill_5,
			is_active,
			created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		on conflict (title, description, company) do nothing
	`,
		position.ID,
		position.RecruiterID,
		position.Title,
		position.Description,
		position.Company,
		isRemote,
		position.LocationID,
		position.Skill1,
		position.Skill2,
		position.Skill3,
		position.Skill4,
		position.Skill5,
		isActive,
		CurrentTimestamp(0),
	)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to exec SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return uuid.Nil(), ErrPositionAlreadyExists
	}

	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		if err = s.UpsertPendingEmbedding(
			position.ID,
			EmbeddingEntityTypePosition,
			BuildPositionText(position.Title, position.Description, position.Company, position.Skill1, position.Skill2, position.Skill3, position.Skill4, position.Skill5),
		); err != nil {
			return uuid.Nil(), fmt.Errorf("failed to create embedding metadata: %w", err)
		}
	}

	return position.ID, nil
}

type Candidate struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
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
	ID             uuid.UUID      `json:"id"`
	CandidateID    uuid.UUID      `json:"candidate_id"`
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

func CreateCandidateExperiences(tx *sql.Tx, candidateID uuid.UUID, experiences []CandidateExperience) error {
	for i := range experiences {
		experiences[i].CandidateID = candidateID
		if experiences[i].ID == uuid.Nil() {
			experiences[i].ID = uuid.NewV7()
		}

		_, err := tx.Exec(
			`
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

func (s Store) CreateCandidate(candidate Candidate, experiences []CandidateExperience) (uuid.UUID, error) {
	candidate.ID = uuid.NewV7()

	tx, err := s.Database.Begin()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if err = tx.Rollback(); err != nil {
				slog.Debug("failed to rollback transaction", "err", err)
			}
		}
	}()

	result, err := tx.Exec(
		`
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
		return uuid.Nil(), fmt.Errorf("failed to execute SQL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return uuid.Nil(), fmt.Errorf("failed to determine number of affected rows: %w", err)
	}
	if rows == 0 {
		return uuid.Nil(), ErrCandidateAlreadyExists
	}

	if err = CreateCandidateExperiences(tx, candidate.ID, experiences); err != nil {
		return uuid.Nil(), fmt.Errorf("failed to insert candidate experiences: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return uuid.Nil(), fmt.Errorf("failed to commit transaction: %w", err)
	}

	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		if err = s.UpsertPendingEmbedding(
			candidate.ID,
			EmbeddingEntityTypeCandidate,
			BuildCandidateText(candidate.About, experiences),
		); err != nil {
			return uuid.Nil(), fmt.Errorf("failed to create embedding metadata: %w", err)
		}
	}

	return candidate.ID, nil
}

func (s Store) DropTables() error {
	// Tables must be dropped child-first: SQLite enforces foreign keys
	// (foreign_keys(1) in the connection string) and fails to drop a
	// parent table while dependent tables still exist.
	for _, table := range []string{
		"refresh_tokens",
		"candidate_experiences",
		"candidate_reactions",
		"recruiter_reactions",
		"matches",
		"recommendations",
		"candidates",
		"positions",
		"recruiters",
		"users",
		"locations",
		"positions_fts",
		"candidates_fts",
		"embeddings",
	} {
		if _, err := s.Database.Exec(fmt.Sprintf("drop table if exists %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	return nil
}

func (s Store) UserExistsByEmail(email string, provider Provider) (bool, error) {
	var exists bool
	err := s.Database.QueryRow(`
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

func (s Store) GetUserAndRoles(userID uuid.UUID) (User, map[Role]uuid.UUID, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash, email string
	var provider Provider
	var candidateID, recruiterID sql.NullString
	err := s.Database.QueryRow(`
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

	roles := make(map[Role]uuid.UUID, 2)
	if candidateID.Valid {
		if roles[RoleCandidate], err = uuid.Parse(candidateID.String); err != nil {
			return user, nil, fmt.Errorf("failed to parse candidateID: %w", err)
		}
	}
	if recruiterID.Valid {
		if roles[RoleRecruiter], err = uuid.Parse(recruiterID.String); err != nil {
			return user, nil, fmt.Errorf("failed to parse recruiterID: %w", err)
		}
	}
	if len(roles) == 0 {
		return user, nil, ErrUserNoRole
	}

	return user, roles, nil
}

func (s Store) GetUser(userID uuid.UUID) (User, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, fullName, userName, passwordHash, email string
	var provider Provider
	err := s.Database.QueryRow(`
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
	userID uuid.UUID,
	newFullName string,
	newUserName string,
) error {
	result, err := s.Database.Exec(`
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
	userID uuid.UUID,
	newFullName string,
	newUserName string,
) (User, error) {
	var updatedAt time.Time
	var optionalProviderUserID sql.NullString
	var providerUserID, passwordHash, email string
	var provider Provider
	err := s.Database.QueryRow(`
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

func (s Store) DeleteUser(userID uuid.UUID) error {
	res, err := s.Database.Exec(`
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

func (s Store) GetCandidate(candidateID uuid.UUID) (Candidate, error) {
	var userIDStr string

	var candidate Candidate
	err := s.Database.QueryRow(`
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
		&userIDStr,
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

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return Candidate{}, fmt.Errorf("failed to parse user_id: %w", err)
	}

	candidate.ID = candidateID
	candidate.UserID = userID

	return candidate, nil
}

// UpdateCandidate updates the candidate fields and, if experiences is not nil,
// replaces all existing candidate experiences with the provided ones in the same transaction.
func (s Store) UpdateCandidate(candidate Candidate, experiences []CandidateExperience) error {
	tx, err := s.Database.Begin()
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

	result, err := tx.Exec(
		`
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

	if experiences == nil {
		if experiences, err = s.GetCandidateExperiences(candidate.ID); err != nil {
			return fmt.Errorf("failed to get candidate experiences: %w", err)
		}
	}

	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		if err = s.UpsertPendingEmbedding(
			candidate.ID,
			EmbeddingEntityTypeCandidate,
			BuildCandidateText(candidate.About, experiences),
		); err != nil {
			return fmt.Errorf("failed to create embedding metadata: %w", err)
		}
	}

	return nil
}

func (s Store) UpdateCandidateAndReturn(
	candidateID uuid.UUID,
	newAbout string,
) (Candidate, error) {
	candidate := Candidate{ID: candidateID}
	err := s.Database.QueryRow(`
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

	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		experiences, err := s.GetCandidateExperiences(candidateID)
		if err != nil {
			return Candidate{}, fmt.Errorf("failed to get candidate experiences: %w", err)
		}
		if err := s.UpsertPendingEmbedding(
			candidateID,
			EmbeddingEntityTypeCandidate,
			BuildCandidateText(newAbout, experiences),
		); err != nil {
			return Candidate{}, fmt.Errorf("failed to create embedding metadata: %w", err)
		}
	}

	return candidate, nil
}

func (s Store) GetCandidateExperiences(candidateID uuid.UUID) ([]CandidateExperience, error) {
	rows, err := s.Database.Query(`
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

func (s Store) DeleteCandidate(candidateID uuid.UUID) error {
	res, err := s.Database.Exec(`
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

func (s Store) GetRecruiter(recruiterID uuid.UUID) (Recruiter, error) {
	var userID uuid.UUID
	err := s.Database.QueryRow(`
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

func (s Store) DeleteRecruiter(recruiterID uuid.UUID) error {
	res, err := s.Database.Exec(`
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

func (s Store) RecruiterExists(recruiterID uuid.UUID) (bool, error) {
	var exists bool
	err := s.Database.QueryRow(`
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

func (s Store) GetPosition(positionID uuid.UUID) (Position, error) {
	var isRemote, isActive int
	position := Position{}
	err := s.Database.QueryRow(`
		select
			recruiter_id,
			title,
			description,
			company,
			is_remote,
			location_id,
			skill_1,
			skill_2,
			skill_3,
			skill_4,
			skill_5,
			is_active
		from positions
		where id = $1
	`, positionID).Scan(
		&position.RecruiterID,
		&position.Title,
		&position.Description,
		&position.Company,
		&isRemote,
		&position.LocationID,
		&position.Skill1,
		&position.Skill2,
		&position.Skill3,
		&position.Skill4,
		&position.Skill5,
		&isActive,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Position{}, ErrPositionNotFound
		}
		return Position{}, fmt.Errorf("failed to query: %w", err)
	}
	position.ID = positionID
	position.IsRemote = isRemote == 1
	position.IsActive = isActive == 1
	return position, nil
}

func (s Store) GetPositions(recruiterID uuid.UUID, page Page) ([]Position, Page, error) {
	rows, err := s.Database.Query(`
		select
			id,
			title,
			description,
			company,
			is_remote,
			location_id,
			skill_1,
			skill_2,
			skill_3,
			skill_4,
			skill_5,
			is_active
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
		var isRemote, isActive int
		var position Position
		if err := rows.Scan(
			&position.ID,
			&position.Title,
			&position.Description,
			&position.Company,
			&isRemote,
			&position.LocationID,
			&position.Skill1,
			&position.Skill2,
			&position.Skill3,
			&position.Skill4,
			&position.Skill5,
			&isActive,
		); err != nil {
			return nil, Page{}, fmt.Errorf("failed to scan: %w", err)
		}
		position.IsRemote = isRemote == 1
		position.IsActive = isActive == 1
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, Page{}, fmt.Errorf("failed to iterate rows: %w", err)
	}

	var nextPage Page
	if len(positions) > page.Limit {
		positions = positions[:page.Limit]
		page.Cursor = positions[page.Limit-1].ID.String()
	}
	return positions, nextPage, nil
}

var ErrInvalidLocationCursor = errors.New("invalid location cursor; must be a positive integer")

func (s Store) SearchLocations(query string, page Page) ([]Location, Page, error) {
	if page.Cursor == "" {
		return nil, Page{}, ErrInvalidLocationCursor
	}

	search := strings.TrimSpace(query)

	rows, err := s.Database.Query(`
		select id, country, city, state
		from locations
		where ($1 = 0 or id > $1)
				and ($2 = ''
						or lower(country) like '%' || lower($2) || '%'
						or lower(city) like '%' || lower($2) || '%')
		order by id
		limit $3
	`, page.Cursor, search, page.Limit+1)
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
			&location.Country,
			&location.City,
			&location.State,
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

var ErrLocationNotFound = errors.New("location not found")

func (s Store) GetLocation(locationID int) (Location, error) {
	location := Location{}
	err := s.Database.QueryRow(`
		select id, country, city, state
		from locations
		where id = $1
	`, locationID).Scan(
		&location.ID,
		&location.Country,
		&location.City,
		&location.State,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Location{}, ErrLocationNotFound
		}
		return Location{}, fmt.Errorf("failed to query: %w", err)
	}
	return location, nil
}

func (s Store) UpdatePosition(position Position) error {
	isRemote := 0
	if position.IsRemote {
		isRemote = 1
	}
	isActive := 0
	if position.IsActive {
		isActive = 1
	}

	result, err := s.Database.Exec(
		`
		update positions
		set
			title = $2,
			description = $3,
			company = $4,
			is_remote = $5,
			location_id = $6,
			skill_1 = $7,
			skill_2 = $8,
			skill_3 = $9,
			skill_4 = $10,
			skill_5 = $11,
			is_active = $12
		where id = $1
	`,
		position.ID,
		position.Title,
		position.Description,
		position.Company,
		isRemote,
		position.LocationID,
		position.Skill1,
		position.Skill2,
		position.Skill3,
		position.Skill4,
		position.Skill5,
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

	if s.DatabaseProvider == DatabaseProviderPostgreSQL {
		if err := s.UpsertPendingEmbedding(
			position.ID,
			EmbeddingEntityTypePosition,
			BuildPositionText(position.Title, position.Description, position.Company, position.Skill1, position.Skill2, position.Skill3, position.Skill4, position.Skill5),
		); err != nil {
			return fmt.Errorf("failed to create embedding metadata: %w", err)
		}
	}

	return nil
}

func (s Store) DeletePosition(positionID uuid.UUID) error {
	res, err := s.Database.Exec(`
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

func BuildTextSearchTerms(text string, quote bool) string {
	tokens := strings.Fields(text)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ReplaceAll(token, `"`, "")
		token = strings.TrimLeft(token, "-")
		if !strings.ContainsFunc(token, func(r rune) bool {
			return unicode.IsLetter(r) || unicode.IsDigit(r)
		}) {
			continue
		}
		if quote {
			token = `"` + token + `"`
		}
		terms = append(terms, token)
	}
	return strings.Join(terms, " OR ")
}

var ErrEmptyCandidateProfile = errors.New("candidate profile is empty")

func (s Store) GetPositionsForCandidateViaFTS(candidateID uuid.UUID, topPositions uint16) ([]uuid.UUID, error) {
	var candidateAbout string
	var prefRemote *bool
	var prefLocation1ID, prefLocation2ID, prefLocation3ID *int
	err := s.Database.QueryRow(`
		select about, pref_remote, pref_location_1_id, pref_location_2_id, pref_location_3_id
		from candidates
		where id = $1
	`, candidateID).Scan(&candidateAbout, &prefRemote, &prefLocation1ID, &prefLocation2ID, &prefLocation3ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []uuid.UUID{}, ErrCandidateNotFound
		}
		return nil, fmt.Errorf("failed to scan: %w", err)
	}

	var query string
	var args []any
	switch s.DatabaseProvider {
	case DatabaseProviderPostgreSQL:
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
					and ($4::boolean is null or not $4::boolean or positions.is_remote = 1)
					and (
							$5::int is null and $6::int is null and $7::int is null
							or positions.is_remote = 1
							or positions.location_id is null
							or positions.location_id in ($5, $6, $7)
					)
			order by ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) desc
			limit $3
		`
		args = []any{BuildTextSearchTerms(candidateAbout, false), candidateID, topPositions, prefRemote, prefLocation1ID, prefLocation2ID, prefLocation3ID}

	case DatabaseProviderSQLite:
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
					and ($4 is null or not $4 or p.is_remote = 1)
					and (
							$5 is null and $6 is null and $7 is null
							or p.is_remote = 1
							or p.location_id is null
							or p.location_id in ($5, $6, $7)
					)
			order by bm25(positions_fts) asc
			limit $3
		`
		args = []any{BuildTextSearchTerms(candidateAbout, true), candidateID, topPositions, prefRemote, prefLocation1ID, prefLocation2ID, prefLocation3ID}

	default:
		return nil, ErrUnsupportedDatabaseProvider
	}

	rows, err := s.Database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}()

	results := make([]uuid.UUID, 0, topPositions)
	for rows.Next() {
		var positionIDStr string
		if err := rows.Scan(&positionIDStr); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		positionID, err := uuid.Parse(positionIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse position_id: %w", err)
		}

		results = append(results, positionID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return results, nil
}
