-- Data ingestion migration for development and testing purposes.
--
-- Creates:
--   - 1 location
--   - 1 candidate user with a strong profile and experiences
--   - 3 recruiter users
--   - 5 positions (one per recommendation)
--   - 5 recommendations
--   - 3 positive recruiter reactions (one from each recruiter)
--
-- Safe to run multiple times.
-- Syntax must stay compliant to both SQLite and PostgreSQL.

begin transaction;

insert into locations (
    id,
    street_1,
    country,
    city,
    postal_code
) values (
    1,
    '1 Demo Street',
    'United States',
    'San Francisco',
    '94105'
) on conflict do nothing;

insert into users (
    id,
    provider,
    provider_user_id,
    email,
    full_name,
    user_name,
    password_hash,
    updated_at
) values (
    'usr_demo_candidate',
    'email',
    null,
    'alex.chen.demo@example.com',
    'Alex Chen',
    'alexchen',
    '$2a$10$t9LpVDfHKqaB1RtcpPBHmOvgghnlMEs9tKCFu0TKm8TkyQJt51pXu', -- "test"
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into candidates (
    id,
    user_id,
    about,
    pref_location_1_id,
    last_recommended_at
) values (
    'can_demo',
    'usr_demo_candidate',
    'Senior Full-Stack Engineer with 8+ years building scalable SaaS platforms. Expert in TypeScript, React, Node.js, Go and AWS. Led teams of up to 10 engineers, shipped products used by millions of users, and enjoys mentoring, system design and developer experience.',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

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
) values (
    'cex_demo_1',
    'can_demo',
    'Senior Full-Stack Engineer',
    '2021-03-01T00:00:00Z',
    '2026-06-01T00:00:00Z',
    'Led a team of 10 engineers building a multi-tenant SaaS platform serving millions of users.',
    'TechNova',
    'work',
    'TypeScript',
    'React',
    'Node.js',
    'Go',
    'AWS'
) on conflict do nothing;

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
) values (
    'cex_demo_2',
    'can_demo',
    'Full-Stack Developer',
    '2018-05-01T00:00:00Z',
    '2021-02-01T00:00:00Z',
    'Built and shipped customer-facing features across the full stack.',
    'BrightApps',
    'work',
    'TypeScript',
    'React',
    'Node.js',
    null,
    null
) on conflict do nothing;

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
) values (
    'cex_demo_3',
    'can_demo',
    'MSc, Computer Science',
    '2016-09-01T00:00:00Z',
    '2018-04-01T00:00:00Z',
    null,
    'State University',
    'education',
    null,
    null,
    null,
    null,
    null
) on conflict do nothing;

insert into users (
    id,
    provider,
    provider_user_id,
    email,
    full_name,
    user_name,
    password_hash,
    updated_at
) values (
    'usr_rec_1',
    'email',
    null,
    'sarah@nova.io',
    'Sarah Williams',
    'sarahrecruits',
    '$2a$10$t9LpVDfHKqaB1RtcpPBHmOvgghnlMEs9tKCFu0TKm8TkyQJt51pXu',
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into recruiters (
    id,
    user_id
) values (
    'rec_1',
    'usr_rec_1'
) on conflict do nothing;

insert into users (
    id,
    provider,
    provider_user_id,
    email,
    full_name,
    user_name,
    password_hash,
    updated_at
) values (
    'usr_rec_2',
    'email',
    null,
    'michael@brightlabs.io',
    'Michael Rodriguez',
    'michaeltalent',
    '$2a$10$t9LpVDfHKqaB1RtcpPBHmOvgghnlMEs9tKCFu0TKm8TkyQJt51pXu',
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into recruiters (
    id,
    user_id
) values (
    'rec_2',
    'usr_rec_2'
) on conflict do nothing;

insert into users (
    id,
    provider,
    provider_user_id,
    email,
    full_name,
    user_name,
    password_hash,
    updated_at
) values (
    'usr_rec_3',
    'email',
    null,
    'emily@cloudforge.io',
    'Emily Johnson',
    'emilyhires',
    '$2a$10$t9LpVDfHKqaB1RtcpPBHmOvgghnlMEs9tKCFu0TKm8TkyQJt51pXu',
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into recruiters (
    id,
    user_id
) values (
    'rec_3',
    'usr_rec_3'
) on conflict do nothing;

insert into positions (
    id,
    recruiter_id,
    title,
    description,
    company,
    is_active,
    created_at
) values (
    'pos_1',
    'rec_1',
    'Senior Full Stack Engineer',
    'Build customer-facing SaaS applications with React, TypeScript and Node.js.',
    'Nova AI',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into positions (
    id,
    recruiter_id,
    title,
    description,
    company,
    is_active,
    created_at
) values (
    'pos_2',
    'rec_2',
    'Staff Backend Engineer',
    'Design distributed systems and APIs powering cloud infrastructure.',
    'Bright Labs',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into positions (
    id,
    recruiter_id,
    title,
    description,
    company,
    is_active,
    created_at
) values (
    'pos_3',
    'rec_3',
    'Engineering Manager',
    'Lead a high-performing product engineering team.',
    'CloudForge',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into positions (
    id,
    recruiter_id,
    title,
    description,
    company,
    is_active,
    created_at
) values (
    'pos_4',
    'rec_1',
    'Principal Platform Engineer',
    'Drive platform reliability, developer productivity and cloud architecture.',
    'Nova AI',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into positions (
    id,
    recruiter_id,
    title,
    description,
    company,
    is_active,
    created_at
) values (
    'pos_5',
    'rec_2',
    'Lead Solutions Architect',
    'Partner with enterprise customers on large-scale cloud adoption.',
    'Bright Labs',
    1,
    '2026-07-01T10:00:00Z'
) on conflict do nothing;

insert into recommendations (
    id,
    position_id,
    candidate_id
) values (
    'rcm_1',
    'pos_1',
    'can_demo'
) on conflict do nothing;

insert into recommendations (
    id,
    position_id,
    candidate_id
) values (
    'rcm_2',
    'pos_2',
    'can_demo'
) on conflict do nothing;

insert into recommendations (
    id,
    position_id,
    candidate_id
) values (
    'rcm_3',
    'pos_3',
    'can_demo'
) on conflict do nothing;

insert into recommendations (
    id,
    position_id,
    candidate_id
) values (
    'rcm_4',
    'pos_4',
    'can_demo'
) on conflict do nothing;

insert into recommendations (
    id,
    position_id,
    candidate_id
) values (
    'rcm_5',
    'pos_5',
    'can_demo'
) on conflict do nothing;

insert into recruiter_reactions (
    recommendation_id,
    recruiter_id,
    reaction_type,
    created_at
) values (
    'rcm_1',
    'rec_1',
    'positive',
    '2026-07-01T10:10:00Z'
) on conflict do nothing;

insert into recruiter_reactions (
    recommendation_id,
    recruiter_id,
    reaction_type,
    created_at
) values (
    'rcm_2',
    'rec_2',
    'positive',
    '2026-07-01T10:12:00Z'
) on conflict do nothing;

insert into recruiter_reactions (
    recommendation_id,
    recruiter_id,
    reaction_type,
    created_at
) values (
    'rcm_3',
    'rec_3',
    'positive',
    '2026-07-01T10:15:00Z'
) on conflict do nothing;

commit;
