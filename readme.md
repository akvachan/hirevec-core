# Hirevec Backend

## About Hirevec
Hirevec is a (placeholder) application that uses a recommendation engine to match candidates with positions and recruiters with candidates.
This repository contains the backend server implementation.

## Philosophy
- The server strives to be simple and lightweight, we intentionally avoid heavy fullstack frameworks.
- The server follows HATEOAS philosophy, meaning that we provide next available actions (`_links`) in the response body where it is appropriate:
```json
{
    "_links": {
      "reaction": {
        "href": "/v1/me/recommendations/rcm_01kmahehzgtq01pq9vy17579ve/reaction"
      },
      "self": {
        "href": "/v1/me/recommendations/rcm_01kmahehzgtq01pq9vy17579ve"
      }
    },
    "about": "Test candidate with full-stack experience",
    "candidate_id": "can_01kmahehzfmh1s64qg7d4szfrk",
    "full_name": "Test User",
    "recommendation_id": "rcm_01kmahehzgtq01pq9vy17579ve"
}
```
- The server does not use any external build systems, package managers or shell scripts, thus trying to be as cross-platform as possible.
- The system is designed to operate without additional infrastructure such as Redis or vector databases.
- The server follows best practices and implements RFCs wherever it can. We do not make up our own concepts or conventions.

## Quick Start

### Requirements
- go >= 1.25.5 
- postgres >= 17.9

1. Set up required environment variables in `.env` as shown in [.example.env](.example.env).
2. Set up server (creates a new database, with a new database user) dependencies:
```bash
go run cmd/setup/main.go --dev
```
3. Run the Go server:
```
go run cmd/server/main.go
```
4. Open [http://localhost:8080/health](http://localhost:8080/health).

## Cleanup
In case, for whatever reason, you want to completely remove the database and everything created by the setup script, run cleanup script:
```bash
go run cmd/cleanup/main.go
```

## CLI API Client
1. Generate access token (token gives access to a test user with some data bound to it already):
```
go run cmd/token/main.go
```
2. Set `ACCESS_TOKEN` either in shell environment variables or `.env`.
3. Call the script with a path:
```
go run cmd/api/main.go "/v1/me/recommendations"
```
or 
```
go run cmd/api/main.go "/v1/me/recommendations/{id}/reaction" POST '{"reaction_type":"positive"}'
```
