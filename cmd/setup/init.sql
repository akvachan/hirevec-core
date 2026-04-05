-- enable required extensions
create extension if not exists pgcrypto;

-- create schema
create schema if not exists v1;

-- ulid generation functions
create or replace function v1.generate_ulid() returns text language plpgsql volatile as $$
declare
    encoding constant text := '0123456789abcdefghjkmnpqrstvwxyz';
    ts bytea := repeat(e'\\000', 6);
    rand bytea := substring(uuid_send(gen_random_uuid()) from 1 for 10);
    ulid bytea;
    t bigint;
    b int[];
    out text[];
begin
    t := (extract(epoch from clock_timestamp()) * 1000)::bigint;
    ts := set_byte(ts, 0, ((t >> 40) & 255)::int);
    ts := set_byte(ts, 1, ((t >> 32) & 255)::int);
    ts := set_byte(ts, 2, ((t >> 24) & 255)::int);
    ts := set_byte(ts, 3, ((t >> 16) & 255)::int);
    ts := set_byte(ts, 4, ((t >> 8) & 255)::int);
    ts := set_byte(ts, 5, (t & 255)::int);

    ulid := ts || rand;

    b := array[
      get_byte(ulid,0), get_byte(ulid,1), get_byte(ulid,2), get_byte(ulid,3),
      get_byte(ulid,4), get_byte(ulid,5), get_byte(ulid,6), get_byte(ulid,7),
      get_byte(ulid,8), get_byte(ulid,9), get_byte(ulid,10), get_byte(ulid,11),
      get_byte(ulid,12), get_byte(ulid,13), get_byte(ulid,14), get_byte(ulid,15)
    ];

    out := array[
      substr(encoding, ((b[1] & 224) >> 5) + 1, 1),
      substr(encoding, (b[1] & 31) + 1, 1),
      substr(encoding, ((b[2] & 248) >> 3) + 1, 1),
      substr(encoding, (((b[2] & 7) << 2) | ((b[3] & 192) >> 6)) + 1, 1),
      substr(encoding, ((b[3] & 62) >> 1) + 1, 1),
      substr(encoding, (((b[3] & 1) << 4) | ((b[4] & 240) >> 4)) + 1, 1),
      substr(encoding, (((b[4] & 15) << 1) | ((b[5] & 128) >> 7)) + 1, 1),
      substr(encoding, ((b[5] & 124) >> 2) + 1, 1),
      substr(encoding, (((b[5] & 3) << 3) | ((b[6] & 224) >> 5)) + 1, 1),
      substr(encoding, (b[6] & 31) + 1, 1),

      substr(encoding, ((b[7] & 248) >> 3) + 1, 1),
      substr(encoding, (((b[7] & 7) << 2) | ((b[8] & 192) >> 6)) + 1, 1),
      substr(encoding, ((b[8] & 62) >> 1) + 1, 1),
      substr(encoding, (((b[8] & 1) << 4) | ((b[9] & 240) >> 4)) + 1, 1),
      substr(encoding, (((b[9] & 15) << 1) | ((b[10] & 128) >> 7)) + 1, 1),
      substr(encoding, ((b[10] & 124) >> 2) + 1, 1),
      substr(encoding, (((b[10] & 3) << 3) | ((b[11] & 224) >> 5)) + 1, 1),
      substr(encoding, (b[11] & 31) + 1, 1),

      substr(encoding, ((b[12] & 248) >> 3) + 1, 1),
      substr(encoding, (((b[12] & 7) << 2) | ((b[13] & 192) >> 6)) + 1, 1),
      substr(encoding, ((b[13] & 62) >> 1) + 1, 1),
      substr(encoding, (((b[13] & 1) << 4) | ((b[14] & 240) >> 4)) + 1, 1),
      substr(encoding, (((b[14] & 15) << 1) | ((b[15] & 128) >> 7)) + 1, 1),
      substr(encoding, ((b[15] & 124) >> 2) + 1, 1),
      substr(encoding, (((b[15] & 3) << 3) | ((b[16] & 224) >> 5)) + 1, 1),
      substr(encoding, (b[16] & 31) + 1, 1)
    ];

    return array_to_string(out, '');
end;
$$;

-- prefix ulid functions
create or replace function v1.generate_ulid(prefix text) returns text language plpgsql volatile as $$
begin
  return prefix || '_' || v1.generate_ulid();
end;
$$;

create or replace function v1.generate_ulid_usr() returns text as $$ begin return v1.generate_ulid('usr'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_rtk() returns text as $$ begin return v1.generate_ulid('rtk'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_can() returns text as $$ begin return v1.generate_ulid('can'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_rec() returns text as $$ begin return v1.generate_ulid('rec'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_pos() returns text as $$ begin return v1.generate_ulid('pos'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_rcm() returns text as $$ begin return v1.generate_ulid('rcm'); end $$ language plpgsql volatile;
create or replace function v1.generate_ulid_doc() returns text as $$ begin return v1.generate_ulid('doc'); end $$ language plpgsql volatile;

-- enum types
do $$ begin create type v1.provider_type as enum ('google','apple'); exception when duplicate_object then null; end $$;
do $$ begin create type v1.reaction_type as enum ('positive','negative','neutral'); exception when duplicate_object then null; end $$;
do $$ begin create type v1.reactor_type as enum ('candidate','recruiter'); exception when duplicate_object then null; end $$;
do $$ begin create type v1.embedding_status as enum ('pending','failed','done'); exception when duplicate_object then null; end $$;
do $$ begin create type v1.embedding_object_type as enum ('candidate','position'); exception when duplicate_object then null; end $$;

-- users table
create table if not exists v1.users (
  id text primary key default v1.generate_ulid_usr(),
  provider v1.provider_type not null,
  provider_user_id varchar(255) not null,
  email varchar(255),
  full_name varchar(255),
  user_name varchar(100) unique,
  updated_at timestamp not null default now(),
  unique(provider, provider_user_id)
);

-- refresh tokens
create table if not exists v1.refresh_tokens (
  jti text primary key default v1.generate_ulid_rtk(),
  user_id text not null references v1.users(id) on delete cascade,
  expires_at timestamp not null,
  revoked boolean default false
);
create index if not exists idx_refresh_tokens_user_id on v1.refresh_tokens(user_id);

-- candidates
create table if not exists v1.candidates (
  id text primary key default v1.generate_ulid_can(),
  user_id text not null references v1.users(id) on delete cascade,
  about text not null,
  updated_at timestamp not null default now(),
  unique(user_id)
);

-- recruiters
create table if not exists v1.recruiters (
  id text primary key default v1.generate_ulid_rec(),
  user_id text not null references v1.users(id) on delete cascade
);

-- positions
create table if not exists v1.positions (
  id text primary key default v1.generate_ulid_pos(),
  recruiter_id text not null references v1.recruiters(id) on delete cascade,
  title text not null,
  description text not null,
  company text,
  updated_at timestamp not null default now(),
  unique(title, description, company)
);

-- embeddings
create table if not exists v1.embeddings (
  object_id text not null,
  object_type v1.embedding_object_type not null,
  created_at timestamp not null default now(),
  updated_at timestamp not null default now(),
  embedding vector(1024),
  status v1.embedding_status not null default 'pending',
  primary key (object_id, object_type)
);

-- main embeddings index
create index if not exists idx_embeddings_vector_hnsw_done
on v1.embeddings
using hnsw (embedding vector_ip_ops)
where status = 'done';

-- filter embeddings index
create index idx_embeddings_lookup
on v1.embeddings (object_type, status);

-- trigger for creating embeddings when a new position or candidate is inserted
create or replace function v1.create_embedding_record()
returns trigger
language plpgsql
as $$
begin
  insert into v1.embeddings (object_id, object_type, status)
  values (
    new.id,
    case
      when tg_table_name = 'candidates' then 'candidate'::v1.embedding_object_type
      when tg_table_name = 'positions' then 'position'::v1.embedding_object_type
    end,
    'pending'
  )
  on conflict (object_id, object_type) do nothing;

  return new;
end;
$$;

-- candidates trigger
drop trigger if exists trg_candidates_create_embedding on v1.candidates;
create trigger trg_candidates_create_embedding
after insert on v1.candidates
for each row
execute function v1.create_embedding_record();

-- positions trigger
drop trigger if exists trg_positions_create_embedding on v1.positions;
create trigger trg_positions_create_embedding
after insert on v1.positions
for each row
execute function v1.create_embedding_record();

-- trigger for updating updated_at field
create or replace function v1.set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

-- users
drop trigger if exists trg_users_updated_at on v1.users;
create trigger trg_users_updated_at
before update on v1.users
for each row
execute function v1.set_updated_at();

-- candidates
drop trigger if exists trg_candidates_updated_at on v1.candidates;
create trigger trg_candidates_updated_at
before update on v1.candidates
for each row
execute function v1.set_updated_at();

-- positions
drop trigger if exists trg_positions_updated_at on v1.positions;
create trigger trg_positions_updated_at
before update on v1.positions
for each row
execute function v1.set_updated_at();

-- embeddings
drop trigger if exists trg_embeddings_updated_at on v1.embeddings;
create trigger trg_embeddings_updated_at
before update on v1.embeddings
for each row
execute function v1.set_updated_at();

-- recommendations
create table if not exists v1.recommendations (
  id text primary key default v1.generate_ulid_rcm(),
  position_id text not null references v1.positions(id) on delete cascade,
  candidate_id text not null references v1.candidates(id) on delete cascade,
  score numeric(3,2) not null check (score >= 0 and score <= 1),
  unique(position_id, candidate_id)
);
create index if not exists idx_recommendations_position on v1.recommendations(position_id);
create index if not exists idx_recommendations_candidate on v1.recommendations(candidate_id);

-- reactions
create table if not exists v1.reactions (
  recommendation_id text not null references v1.recommendations(id) on delete cascade,
  reactor_type v1.reactor_type not null,
  reactor_id text not null,
  reaction_type v1.reaction_type not null,
  created_at timestamp not null default now(),
  primary key (recommendation_id, reactor_type, reactor_id)
);
create index if not exists idx_reactions_recommendation on v1.reactions(recommendation_id);

-- matches
create table if not exists v1.matches (
  candidate_id text not null references v1.candidates(id) on delete cascade,
  position_id text not null references v1.positions(id) on delete cascade,
  created_at timestamp not null default now(),
  primary key (candidate_id, position_id)
);
