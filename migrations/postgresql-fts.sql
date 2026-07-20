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
