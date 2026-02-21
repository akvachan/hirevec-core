-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS general;

-- USERS
CREATE TABLE IF NOT EXISTS general.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);

-- REFRESH TOKENS
CREATE TABLE IF NOT EXISTS general.refresh_tokens (
    jti UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES general.users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    revoked BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
    ON general.refresh_tokens(user_id);

-- CANDIDATES
CREATE TABLE IF NOT EXISTS general.candidates (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES general.users(id) ON DELETE CASCADE,
    about TEXT NOT NULL
);

-- RECRUITERS
CREATE TABLE IF NOT EXISTS general.recruiters (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES general.users(id) ON DELETE CASCADE
);

-- POSITIONS
CREATE TABLE IF NOT EXISTS general.positions (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    company TEXT
);

-- REACTIONS
CREATE TYPE general.reaction_type AS ENUM ('positive', 'negative', 'neutral');

CREATE TABLE IF NOT EXISTS general.candidates_reactions (
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    reaction_type general.reaction_type NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (candidate_id, position_id)
);

CREATE TABLE IF NOT EXISTS general.recruiters_reactions (
    recruiter_id INT NOT NULL REFERENCES general.recruiters(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    reaction_type general.reaction_type NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (recruiter_id, position_id, candidate_id)
);

-- MATCHES
CREATE TABLE IF NOT EXISTS general.matches (
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (candidate_id, position_id)
);

-- TEST DATA

-- Users
INSERT INTO general.users (provider, provider_user_id, email)
VALUES
('google', 'google-123', 'candidate@test.com'),
('google', 'google-456', 'recruiter@test.com');

-- Candidates & recruiters
INSERT INTO general.candidates (user_id, about)
SELECT id, 'Backend developer with 5 years of experience'
FROM general.users
WHERE email = 'candidate@test.com';

INSERT INTO general.recruiters (user_id)
SELECT id
FROM general.users
WHERE email = 'recruiter@test.com';

-- Positions
INSERT INTO general.positions (title, description, company)
VALUES
('Backend Engineer', 'Work on APIs and databases', 'Acme Inc'),
('Fullstack Developer', 'React + Node.js role', 'Tech Corp');

-- Candidate reaction
INSERT INTO general.candidates_reactions (candidate_id, position_id, reaction_type)
VALUES (1, 1, 'positive');

-- Recruiter reaction
INSERT INTO general.recruiters_reactions (recruiter_id, position_id, candidate_id, reaction_type)
VALUES (1, 1, 1, 'positive');

-- Match
INSERT INTO general.matches (candidate_id, position_id)
VALUES (1, 1);

-- SAMPLE REFRESH TOKEN
INSERT INTO general.refresh_tokens (
    user_id,
    expires_at
)
SELECT
    id,
    NOW() + INTERVAL '30 days'
FROM general.users
WHERE email = 'candidate@test.com';
