create table if not exists users (
    id text primary key not null,
    provider text not null check (provider in ('google','apple')),
    provider_user_id varchar(255) not null,
    email varchar(255),
    full_name varchar(255),
    user_name varchar(100) unique,
    updated_at timestamp default current_timestamp,
    unique(provider, provider_user_id)
);

create table if not exists refresh_tokens (
    jti text primary key not null,
    user_id text not null references users(id) on delete cascade,
    expires_at timestamp not null,
    revoked boolean default false,
    unique(jti)
);

create index if not exists idx_refresh_tokens_user_id
on refresh_tokens(user_id);

create table if not exists candidates (
    id text primary key not null,
    user_id text not null references users(id) on delete cascade,
    about text not null,
    last_recommended_at timestamp not null default current_timestamp
    unique(user_id)
);

create table if not exists recruiters (
    id text primary key not null,
    user_id text not null references users(id) on delete cascade
);

create table if not exists positions (
    id text primary key not null,
    recruiter_id text not null references recruiters(id) on delete cascade,
    title text not null,
    description text not null,
    company text,
    is_active integer not null default 1 check (is_active in (0, 1))
    unique(title, description, company)
);

create table if not exists recommendations (
    id text primary key not null,
    position_id text not null references positions(id) on delete cascade,
    candidate_id text not null references candidates(id) on delete cascade,
    unique(position_id, candidate_id)
);

create index if not exists idx_recommendations_position
on recommendations(position_id);

create index if not exists idx_recommendations_candidate
on recommendations(candidate_id);

create index if not exists idx_recommendations_candidate_id
on recommendations(candidate_id, id);

create table if not exists reactions (
    recommendation_id text not null references recommendations(id) on delete cascade,
    reactor_type text not null check (reactor_type in ('candidate','recruiter')),
    reactor_id text not null,
    reaction_type text not null check (reaction_type in ('positive','negative')),
    created_at timestamp not null default current_timestamp,
    primary key (recommendation_id, reactor_type, reactor_id)
);

create index if not exists idx_reactions_recommendation
on reactions(recommendation_id);

create table if not exists matches (
    candidate_id text not null references candidates(id) on delete cascade,
    position_id text not null references positions(id) on delete cascade,
    created_at timestamp not null default current_timestamp,
    primary key (candidate_id, position_id)
);

create table if not exists embedding_jobs (
    id text primary key not null,
    entity_type text not null check (entity_type in ('candidate', 'position')),
    entity_id text not null,
    status text not null check (status in ('pending', 'done', 'failed')),
);

create table if not exists candidate_embeddings
using vec(dim=768, metric="cosine");

create table if not exists position_embeddings
using vec(dim=768, metric="cosine");
