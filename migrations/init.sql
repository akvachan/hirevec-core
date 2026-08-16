-- Init migration for SQLite and PostgreSQL. Syntax must stay compliant to both databases.

begin transaction;

create table if not exists locations (
  id serial primary key not null,
  street_1 text not null,
  street_2 text,
  country text not null,
  city text not null,
  state text,
  postal_code text not null
);

create table if not exists users (
  id text primary key not null, -- ULID
  provider text not null,
  provider_user_id text,
  email text,
  full_name text,
  user_name text unique not null,
  location_id int not null,
  password_hash text,
  updated_at timestamp not null, -- UTC, RFC3339
  foreign key (location_id) references locations(id) on delete cascade,
  check (provider in ('google', 'apple', 'email')),
  unique(provider, provider_user_id)
);

create table if not exists refresh_tokens (
  jti text primary key not null, -- ULID
  user_id text not null, -- ULID
  created_at timestamp not null, -- UTC, RFC3339
  expires_at timestamp not null, -- UTC, RFC3339
  revoked integer not null default 0,
  foreign key (user_id) references users(id) on delete cascade,
  check (revoked in (0, 1))
);

create index if not exists idx_refresh_tokens_user_id
on refresh_tokens(user_id);

create table if not exists candidates (
  id text primary key not null, -- ULID
  user_id text not null, -- ULID
  about text not null,
  pref_remote integer not null default 0, 
  preferred_title_1 text,
  preferred_title_2 text,
  preferred_title_3 text,
  last_recommended_at timestamp not null, -- UTC, RFC3339
  unique(user_id),
  foreign key (user_id) references users(id) on delete cascade
);

create table if not exists candidate_experiences (
  candidate_id text primary key not null, -- ULID
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
  foreign key (candidate_id) references candidate(id) on delete cascade,
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

create table if not exists recruiters (
  id text primary key not null, -- ULID
  user_id text not null, -- ULID
  unique(user_id),
  foreign key (user_id) references users(id) on delete cascade
);

create table if not exists positions (
  id text primary key not null, -- ULID
  recruiter_id text not null, -- ULID
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
  check (is_active in (0, 1))
);

create index if not exists idx_positions_active
on positions(is_active);

create table if not exists recommendations (
  id text primary key not null, -- ULID
  position_id text not null, -- ULID
  candidate_id text not null, -- ULID
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
  recommendation_id text not null, -- ULID
  candidate_id text not null, -- ULID
  reaction_type text not null,
  created_at timestamp not null, -- UTC, RFC3339
  primary key (recommendation_id, candidate_id),
  foreign key (recommendation_id) references recommendations(id) on delete cascade,
  foreign key (recruiter_id) references candidates(id) on delete cascade,
  check (reaction_type in ('positive', 'negative'))
);

create table if not exists recruiter_reactions (
  recommendation_id text not null, -- ULID
  recruiter_id text not null, -- ULID
  reaction_type text not null,
  created_at timestamp not null, -- UTC, RFC3339
  primary key (recommendation_id, recruiter_id),
  foreign key (recommendation_id) references recommendations(id) on delete cascade,
  foreign key (recruiter_id) references recruiters(id) on delete cascade,
  check (reaction_type in ('positive', 'negative'))
);

create index if not exists idx_reactions_recommendation
on reactions(recommendation_id);

create table if not exists matches (
  candidate_id text not null, -- ULID
  position_id text not null, -- ULID
  created_at timestamp not null, -- UTC, RFC3339
  primary key (candidate_id, position_id),
  foreign key (candidate_id) references candidates(id) on delete cascade,
  foreign key (position_id) references positions(id) on delete cascade
);

commit;
