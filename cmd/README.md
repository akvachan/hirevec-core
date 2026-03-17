## API

API script can be used to interact with the server. It automatically injects your `ACCESS_TOKEN` into request headers and can process path, method and body.

## Cleanup

Cleanup script can be used to drops the `POSTGRES_DB` database and `POSTGRES_USER` user via Postgres superuser.

## Common

Common is just a set of helper functions that are used across all scripts.

## Server

Server script can be used to start local server. 

## Setup

Setup sciprt can be used to setup `POSTGRES_DB` database with `POSTGRES_USER`, create tables and ingest some test data as implemented in `init.sql`.

## Token

Token script generates an an access token using `SYMMETRIC_KEY` and `ASYMMETRIC_KEY`.
