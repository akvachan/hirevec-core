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

-- Recommendations: test candidate recommended for existing positions
insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p
join v1.candidates c on c.user_id = (select id from v1.users where email = 'test@example.com')
where p.title in ('Backend Engineer', 'Frontend Engineer', 'Test Engineer')
on conflict do nothing;

-- Recommendations: existing candidates recommended for test recruiter's position
insert into v1.recommendations (position_id, candidate_id)
select p.id, c.id
from v1.positions p
join v1.candidates c on c.user_id != (select id from v1.users where email = 'test@example.com')
where p.recruiter_id = (
    select id from v1.recruiters
    where user_id = (select id from v1.users where email = 'test@example.com')
)
on conflict do nothing;

-- Sample reaction: test candidate reacts positively to Backend Engineer
insert into v1.reactions (recommendation_id, reactor_type, reactor_id, reaction_type)
select r.id, 'candidate', c.id, 'positive'
from v1.recommendations r
join v1.candidates c on c.id = r.candidate_id
join v1.positions p on p.id = r.position_id
where c.user_id = (select id from v1.users where email = 'test@example.com')
  and p.title = 'Backend Engineer'
on conflict do nothing;

-- Match: test candidate + Backend Engineer
insert into v1.matches (candidate_id, position_id)
select c.id, p.id
from v1.candidates c
join v1.positions p on p.title = 'Backend Engineer'
where c.user_id = (select id from v1.users where email = 'test@example.com')
on conflict do nothing;

-- Match: alice + Test Engineer 
insert into v1.matches (candidate_id, position_id)
select c.id, p.id
from v1.candidates c
join v1.users u on u.id = c.user_id
join v1.positions p on p.title = 'Test Engineer'
where u.email = 'alice@example.com'
on conflict do nothing;
