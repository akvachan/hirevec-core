## Setup PGSQL

Use this set of SQL queries to create development and test DBs:
```sql
CREATE DATABASE <db_name>;
CREATE SCHEMA IF NOT EXISTS general;
CREATE TABLE IF NOT EXISTS general.users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(512) NOT NULL UNIQUE,
    user_name VARCHAR(64) NOT NULL UNIQUE,
    full_name VARCHAR(128) NOT NULL
);
CREATE TABLE IF NOT EXISTS general.candidates (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES general.users(id) ON DELETE CASCADE,
    about TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS general.recruiters (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES general.users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS general.positions (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    company TEXT
);
CREATE TYPE reaction_type AS ENUM ('like', 'dislike');
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
```

## Run server
- Inside the root of the repository:
```
make 
```
- Development setup with a hot-reload (Optional):
```
make watch
```

## Unit Tests
- Run unit tests with:
```
make test
```
