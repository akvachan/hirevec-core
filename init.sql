create schema if not exists v1;

-- ULID base generator
create or replace function generate_ulid() returns text as $$
declare
  encoding   bytea = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
  timestamp  bytea = e'\\000\\000\\000\\000\\000\\000';
  output     text  = '';
  unix_time  bigint;
  ulid       bytea;
begin
  unix_time = (extract(epoch from clock_timestamp()) * 1000)::bigint;

  timestamp = set_byte(timestamp, 0, ((unix_time >> 40) & 255)::int);
  timestamp = set_byte(timestamp, 1, ((unix_time >> 32) & 255)::int);
  timestamp = set_byte(timestamp, 2, ((unix_time >> 24) & 255)::int);
  timestamp = set_byte(timestamp, 3, ((unix_time >> 16) & 255)::int);
  timestamp = set_byte(timestamp, 4, ((unix_time >> 8)  & 255)::int);
  timestamp = set_byte(timestamp, 5, (unix_time         & 255)::int);

  ulid = timestamp || substring(uuid_send(gen_random_uuid()) from 1 for 10);

  -- ULID encoding logic
  output = output
      || chr(get_byte(encoding, (get_byte(ulid, 0) & 224) >> 5))
      || chr(get_byte(encoding,  get_byte(ulid, 0) & 31))
      || chr(get_byte(encoding, (get_byte(ulid, 1) & 248) >> 3))
      || chr(get_byte(encoding, ((get_byte(ulid, 1) & 7) << 2) | ((get_byte(ulid, 2) & 192) >> 6)))
      || chr(get_byte(encoding, (get_byte(ulid, 2) & 62) >> 1))
      || chr(get_byte(encoding, ((get_byte(ulid, 2) & 1) << 4) | ((get_byte(ulid, 3) & 240) >> 4)))
      || chr(get_byte(encoding, ((get_byte(ulid, 3) & 15) << 1) | ((get_byte(ulid, 4) & 128) >> 7)))
      || chr(get_byte(encoding, (get_byte(ulid, 4) & 124) >> 2))
      || chr(get_byte(encoding, ((get_byte(ulid, 4) & 3) << 3) | ((get_byte(ulid, 5) & 224) >> 5)))
      || chr(get_byte(encoding,  get_byte(ulid, 5) & 31))
      || chr(get_byte(encoding, (get_byte(ulid, 6) & 248) >> 3))
      || chr(get_byte(encoding, ((get_byte(ulid, 6) & 7) << 2) | ((get_byte(ulid, 7) & 192) >> 6)))
      || chr(get_byte(encoding, (get_byte(ulid, 7) & 62) >> 1))
      || chr(get_byte(encoding, ((get_byte(ulid, 7) & 1) << 4) | ((get_byte(ulid, 8) & 240) >> 4)))
      || chr(get_byte(encoding, ((get_byte(ulid, 8) & 15) << 1) | ((get_byte(ulid, 9) & 128) >> 7)))
      || chr(get_byte(encoding, (get_byte(ulid, 9) & 124) >> 2))
      || chr(get_byte(encoding, ((get_byte(ulid, 9) & 3) << 3) | ((get_byte(ulid, 10) & 224) >> 5)))
      || chr(get_byte(encoding,  get_byte(ulid, 10) & 31))
      || chr(get_byte(encoding, (get_byte(ulid, 11) & 248) >> 3))
      || chr(get_byte(encoding, ((get_byte(ulid, 11) & 7) << 2) | ((get_byte(ulid, 12) & 192) >> 6)))
      || chr(get_byte(encoding, (get_byte(ulid, 12) & 62) >> 1))
      || chr(get_byte(encoding, ((get_byte(ulid, 12) & 1) << 4) | ((get_byte(ulid, 13) & 240) >> 4)))
      || chr(get_byte(encoding, ((get_byte(ulid, 13) & 15) << 1) | ((get_byte(ulid, 14) & 128) >> 7)))
      || chr(get_byte(encoding, (get_byte(ulid, 14) & 124) >> 2))
      || chr(get_byte(encoding, ((get_byte(ulid, 14) & 3) << 3) | ((get_byte(ulid, 15) & 224) >> 5)))
      || chr(get_byte(encoding,  get_byte(ulid, 15) & 31));

  return output;
end
$$ language plpgsql volatile;

-- prefixed ULIDs
create or replace function generate_ulid(prefix text) returns text as $$ begin return prefix || '_' || generate_ulid(); end $$ language plpgsql volatile;
create or replace function generate_ulid_usr() returns text as $$ begin return generate_ulid('usr'); end $$ language plpgsql volatile;
create or replace function generate_ulid_rtk() returns text as $$ begin return generate_ulid('rtk'); end $$ language plpgsql volatile;
create or replace function generate_ulid_can() returns text as $$ begin return generate_ulid('can'); end $$ language plpgsql volatile;
create or replace function generate_ulid_rec() returns text as $$ begin return generate_ulid('rec'); end $$ language plpgsql volatile;
create or replace function generate_ulid_pos() returns text as $$ begin return generate_ulid('pos'); end $$ language plpgsql volatile;
create or replace function generate_ulid_rcm() returns text as $$ begin return generate_ulid('rcm'); end $$ language plpgsql volatile;

-- provider types
do $$ begin create type v1.provider_type as enum ('google', 'apple'); exception when duplicate_object then null; end $$;

-- reaction types
do $$ begin create type v1.reaction_type as enum ('positive','negative','neutral'); exception when duplicate_object then null; end $$;

-- users
create table if not exists v1.users (
    id text primary key default generate_ulid_usr(),
    provider v1.provider_type not null,
    provider_user_id varchar(255) not null,
    email varchar(255),
    full_name varchar(255),
    user_name varchar(100) unique,
    updated_at timestamp default now(),
    unique(provider, provider_user_id)
);

-- refresh tokens
create table if not exists v1.refresh_tokens (
    jti text primary key default generate_ulid_rtk(),
    user_id text not null references v1.users(id) on delete cascade,
    expires_at timestamp not null,
    revoked boolean default false,
    unique(jti)
);
create index if not exists idx_refresh_tokens_user_id on v1.refresh_tokens(user_id);

-- candidates
create table if not exists v1.candidates (
    id text primary key default generate_ulid_can(),
    user_id text not null references v1.users(id) on delete cascade,
    about text not null,
    unique(user_id)
);

-- recruiters
create table if not exists v1.recruiters (
    id text primary key default generate_ulid_rec(),
    user_id text not null references v1.users(id) on delete cascade
);

-- positions
create table if not exists v1.positions (
    id text primary key default generate_ulid_pos(),
    recruiter_id text not null references v1.recruiters(id) on delete cascade,
    title text not null,
    description text not null,
    company text,
    unique(title, description, company)
);

-- unified recommendations
create table if not exists v1.recommendations (
    id text primary key default generate_ulid_rcm(),
    position_id text not null references v1.positions(id) on delete cascade,
    candidate_id text not null references v1.candidates(id) on delete cascade,
    unique(position_id, candidate_id)
);
create index if not exists idx_recommendations_position on v1.recommendations(position_id);
create index if not exists idx_recommendations_candidate on v1.recommendations(candidate_id);

-- unified reactions
create table if not exists v1.reactions (
    recommendation_id text not null references v1.recommendations(id) on delete cascade,
    reactor_type text not null check (reactor_type in ('candidate','recruiter')),
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

-- users
insert into v1.users (provider, provider_user_id, email, full_name, user_name)
values
    ('google', 'google-001', 'alice@example.com', 'Alice Doe', 'alice_doe'),
    ('google', 'google-002', 'bob@example.com', 'Bob Smith', 'bob_smith'),
    ('google', 'google-003', 'carol@example.com', 'Carol Jones', 'carol_jones')
on conflict (provider, provider_user_id) do nothing;

-- candidates
insert into v1.candidates (user_id, about)
select id, 'Backend developer with 5 years experience'
from v1.users where email = 'alice@example.com'
on conflict do nothing;

insert into v1.candidates (user_id, about)
select id, 'Frontend developer expert in React'
from v1.users where email = 'carol@example.com'
on conflict do nothing;

-- recruiters
insert into v1.recruiters (user_id)
select id from v1.users where email = 'bob@example.com'
on conflict do nothing;

-- positions
insert into v1.positions (recruiter_id, title, description, company)
select r.id, 'Backend Engineer', 'Develop APIs and databases', 'TechCorp'
from v1.recruiters r
where r.user_id = (select id from v1.users where email = 'bob@example.com')
on conflict do nothing;

insert into v1.positions (recruiter_id, title, description, company)
select r.id, 'Frontend Engineer', 'React & UI focused role', 'DesignHub'
from v1.recruiters r
where r.user_id = (select id from v1.users where email = 'bob@example.com')
on conflict do nothing;

insert into v1.positions (recruiter_id, title, description, company)
select r.id, 'Fullstack Developer', 'Backend + Frontend role', 'TechFusion'
from v1.recruiters r
where r.user_id = (select id from v1.users where email = 'dave@example.com')
on conflict do nothing;

insert into v1.positions (recruiter_id, title, description, company)
select r.id, 'UI/UX Designer', 'Design and frontend focus', 'DesignHub'
from v1.recruiters r
where r.user_id = (select id from v1.users where email = 'dave@example.com')
on conflict do nothing;

-- recommendations
insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p, v1.candidates c
where p.title = 'Backend Engineer'
  and c.user_id = (select id from v1.users where email = 'alice@example.com')
on conflict do nothing;

insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p, v1.candidates c
where p.title = 'Frontend Engineer'
  and c.user_id = (select id from v1.users where email = 'carol@example.com')
on conflict do nothing;

-- candidate reacts to position 
insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'candidate', c.id, 'positive'
from v1.recommendations r
join v1.candidates c on c.id = r.candidate_id
where c.user_id = (select id from v1.users where email = 'alice@example.com')
on conflict do nothing;

-- recruiter reacts to candidate
insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'recruiter', rec.id, 'positive'
from v1.recommendations r
join v1.recruiters rec on rec.user_id = (select id from v1.users where email = 'bob@example.com')
join v1.candidates c on c.id = r.candidate_id
where c.user_id = (select id from v1.users where email = 'alice@example.com')
on conflict do nothing;

insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'candidate', c.id, 'neutral'
from v1.recommendations r
join v1.candidates c on c.id = r.candidate_id
where c.user_id = (select id from v1.users where email = 'carol@example.com')
on conflict do nothing;

insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'recruiter', rec.id, 'negative'
from v1.recommendations r
join v1.recruiters rec on rec.user_id = (select id from v1.users where email = 'bob@example.com')
join v1.candidates c on c.id = r.candidate_id
where c.user_id = (select id from v1.users where email = 'carol@example.com')
on conflict do nothing;

-- combined user
insert into v1.users (provider, provider_user_id, email, full_name, user_name)
values
    ('google', 'google-004', 'dave@example.com', 'Dave Miller', 'dave_miller')
on conflict (provider, provider_user_id) do nothing;

-- as candidate
insert into v1.candidates (user_id, about)
select id, 'Fullstack developer and part-time recruiter'
from v1.users
where email = 'dave@example.com'
on conflict do nothing;

-- as recruiter
insert into v1.recruiters (user_id)
select id
from v1.users
where email = 'dave@example.com'
on conflict do nothing;

-- recommendations for combined user as candidate
insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p
join v1.candidates c on c.user_id = (select id from v1.users where email = 'dave@example.com')
where p.title = 'Backend Engineer'
on conflict do nothing;

insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p
join v1.candidates c on c.user_id = (select id from v1.users where email = 'dave@example.com')
where p.title = 'Frontend Engineer'
on conflict do nothing;

-- reactions by combined user as candidate
insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'candidate', c.id, 'positive'
from v1.recommendations r
join v1.candidates c on c.id = r.candidate_id
where c.user_id = (select id from v1.users where email = 'dave@example.com')
on conflict do nothing;

-- reactions by combined user as recruiter
insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'recruiter', rec.id, 'neutral'
from v1.recommendations r
join v1.recruiters rec on rec.user_id = (select id from v1.users where email = 'dave@example.com')
join v1.candidates c on c.id = r.candidate_id
where c.user_id <> rec.user_id  -- avoid reacting to self
on conflict do nothing;
