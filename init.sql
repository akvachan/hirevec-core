create table if not exists users (
  id text primary key not null,
  provider text not null,
  provider_user_id text not null,
  email text,
  full_name text,
  user_name text unique,
  updated_at timestamp default current_timestamp,
  unique(provider, provider_user_id),
  check (provider in ('google', 'apple'))
);

create table if not exists refresh_tokens (
  jti text primary key not null,
  user_id text not null,
  expires_at timestamp not null,
  revoked integer not null default 0,
  foreign key (user_id) references users(id) on delete cascade,
  check (revoked in (0, 1))
);

create index if not exists idx_refresh_tokens_user_id
on refresh_tokens(user_id);

create table if not exists candidates (
  id text primary key not null,
  user_id text not null,
  about text not null,
  last_recommended_at timestamp not null default current_timestamp,
  unique(user_id),
  foreign key (user_id) references users(id) on delete cascade
);

create table if not exists recruiters (
  id text primary key not null,
  user_id text not null,
  foreign key (user_id) references users(id) on delete cascade
);

create table if not exists positions (
  id text primary key not null,
  recruiter_id text not null,
  title text not null,
  description text not null,
  company text,
  is_active integer not null default 1,
  unique(title, description, company),
  foreign key (recruiter_id) references recruiters(id) on delete cascade,
  check (is_active in (0, 1))
);

create index if not exists idx_positions_active
on positions(is_active);

create table if not exists recommendations (
  id text primary key not null,
  position_id text not null,
  candidate_id text not null,
  unique(position_id, candidate_id),
  foreign key (position_id) references positions(id) on delete cascade,
  foreign key (candidate_id) references candidates(id) on delete cascade
);

create index if not exists idx_recommendations_candidate_position
on recommendations(candidate_id, position_id);

create index if not exists idx_recommendations_position
on recommendations(position_id);

create index if not exists idx_recommendations_candidate
on recommendations(candidate_id);

create index if not exists idx_recommendations_candidate_id
on recommendations(candidate_id, id);

create table if not exists reactions (
  recommendation_id text not null,
  reactor_type text not null,
  reactor_id text not null,
  reaction_type text not null,
  created_at timestamp not null default current_timestamp,
  primary key (recommendation_id, reactor_type, reactor_id),
  foreign key (recommendation_id) references recommendations(id) on delete cascade,
  check (reactor_type in ('candidate', 'recruiter')),
  check (reaction_type in ('positive', 'negative'))
);

create index if not exists idx_reactions_recommendation
on reactions(recommendation_id);

create table if not exists matches (
  candidate_id text not null,
  position_id text not null,
  created_at timestamp not null default current_timestamp,
  primary key (candidate_id, position_id),
  foreign key (candidate_id) references candidates(id) on delete cascade,
  foreign key (position_id) references positions(id) on delete cascade
);
