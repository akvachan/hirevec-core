-- Ingest some development data

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

-- combined user
insert into v1.users (provider, provider_user_id, email, full_name, user_name)
values ('google', 'google-004', 'dave@example.com', 'Dave Miller', 'dave_miller')
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

-- Add test user
insert into v1.users (provider, provider_user_id, email, full_name, user_name)
values ('google', 'google-test-001', 'test@example.com', 'Test User', 'test_user')
on conflict (provider, provider_user_id) do nothing;

-- Test candidate account
insert into v1.candidates (user_id, about)
select id, 'Test candidate with full-stack experience'
from v1.users where email = 'test@example.com'
on conflict do nothing;

-- Test recruiter account
insert into v1.recruiters (user_id)
select id from v1.users where email = 'test@example.com'
on conflict do nothing;

-- Positions posted by test recruiter
insert into v1.positions (recruiter_id, title, description, company)
select r.id, 'Test Engineer', 'QA and testing focused role', 'TestCorp'
from v1.recruiters r
where r.user_id = (select id from v1.users where email = 'test@example.com')
on conflict do nothing;
