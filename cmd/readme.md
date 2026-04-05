## API

API script can be used to interact with the server. It automatically injects your `ACCESS_TOKEN` into request headers and can process path, method and body.

## Cleanup

Cleanup script is used to drop the `POSTGRES_DB` database and `POSTGRES_USER` user via Postgres superuser.

## Common

Common is just a set of helper functions that are used across all scripts.

## Server

Server script can be used to start local server instance.

## Embedding

Embedding script can be used to start a local embedding worker.

## Setup

Setup sciprt can be used to setup `POSTGRES_DB` database with `POSTGRES_USER` user via Postgres superuser. 
It creates tables and ingests some test data, as implemented in `init.sql`.

## Token

Token script generates an an access token using `SYMMETRIC_KEY` and `ASYMMETRIC_KEY`.
Generated token can be copied used in frontend applications for **testing/development purposes only**.
