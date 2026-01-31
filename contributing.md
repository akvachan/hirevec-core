## Run Server
- Setup development PGSQL database, refer to [this](#setup-pgsql) section.
- Inside the root of the repository:
```
go run cmd/server/main.go
```

## Unit Tests
```
go test ./internal -v
```

## Integration Tests
- Setup test PGSQL database, refer to [this](#setup-pgsql) section.
- Run tests with:
```
cd test
go test -v
```

## Setup PGSQL
Use this set of SQL queries to create development and test DBs:
```sql
CREATE DATABASE <db_name>;
CREATE SCHEMA IF NOT EXISTS general;
CREATE TABLE IF NOT EXISTS general.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);
CREATE TABLE IF NOT EXISTS general.refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES generla.users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    revoked BOOLEAN DEFAULT FALSE,
);
CREATE TABLE IF NOT EXISTS general.candidates (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES general.users(id) ON DELETE CASCADE,
    about TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS general.recruiters (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES general.users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS general.positions (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    company TEXT
);
CREATE TYPE general.reaction_type AS ENUM ('positive', 'negative', 'neutral');
CREATE TABLE IF NOT EXISTS general.candidates_reactions (
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    reaction_type reaction_type NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (candidate_id, position_id)
);
CREATE TABLE IF NOT EXISTS general.recruiters_reactions (
    recruiter_id INT NOT NULL REFERENCES general.recruiters(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    reaction_type reaction_type NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (recruiter_id, position_id, candidate_id)
);
CREATE TABLE IF NOT EXISTS general.matches (
    candidate_id INT NOT NULL REFERENCES general.candidates(id) ON DELETE CASCADE,
    position_id INT NOT NULL REFERENCES general.positions(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (candidate_id, position_id)
);
CREATE TABLE IF NOT EXISTS general.refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    revoked BOOLEAN DEFAULT FALSE,
    INDEX idx_user_id (user_id),
    INDEX idx_token_hash (token_hash)
);
```
