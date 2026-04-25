# Hirevec

## About Hirevec

Hirevec is a job recommendation engine, implemented in pure Go with minimal dependencies.
It helps candidates find suitable positions based on their profile and helps recruiters find suitable candidates.

## Quick Start

The server implements **Okapi BM25** as a base recommender algorithm and uses **SQLite** by default. 
To try it out, just run:
```sh
go run cmd/server/main.go
```

## Development and testing

For **development and testing** purposes we may want to quickly ingest some new data and create a test API key.
The output of the script is a refresh token of a user with a username `test_user_1`.
```sh
go run cmd/dev/main.go
```

> [!TIP]
> You can modify `*.csv` files in [./cmd/dev/data](./cmd/dev/data/) to customize or add ingested data.
> By default, data with existing primary key will be updated.
> To clean up the DB, stop the server and run: `psql -d postgres -c "DROP DATABASE hirevec;"`.

We may want to save a refresh token in a file for an easier frontend app consumption:
```sh
go run cmd/dev/main.go > .test_user_1.apikey
```

> [!WARN]
> The `.apikey` files are **not encrypted**, so only use those during **development and testing**.

If you want to create a new custom user, run:
```sh
go run cmd/register_user/main.go --user=./register_user/test_user_2.json > .test_user_2.apikey
```

`user.json` should have a following payload structure:
```json
{

}
```

## Enabling Postgres with pgvector

This project uses SQLite by default. To enable Postgres, set `POSTGRES_DATABASE_URL`.
For example: `export POSTGRES_DATABASE_URL=postgres://postgres@localhost:5432/postgres?sslmode=disable`.

To enable `pgvector`, just have it installed on your machine together with Postgres, the server will activate it automatically.
Here is a guide [how to get `pgvector` extension](https://github.com/pgvector/pgvector).

SQLite will continue to be used in following scenarios:
- Postgres instance cannot be pinged.
- Postgres does not have `pgvector` extension installed.

## Enabling Embeddings and Reranker

This project uses Okapi BM25 by default. 
To enable embeddings and reranking, enable Postgres with pgvector first, then set `TEI_BASE_URL` and `TEI_API_KEY`.
Your TEI instance **must** be protected by an API key or the enablement will fail.
For example: `export TEI_BASE_URL=localhost:8080` and `export TEI_API_KEY=your-api-key`.
The endpoint should serve TEI-compatible embeddings and reranking responses. 
Here is a guide on [how to setup TEI](https://github.com/huggingface/text-embeddings-inference).

Okapi BM25 will continue to be used in following scenarios:
- The embeddings and reranker services are not available.
- The embeddings are not yet created (cold start).
- The `pgvector` extension cannot be created or is not available.
- The default SQLite database is used instead of Postgres.

## Enabling SMTP service (E-mail resgistration)

This project does not allow E-mail registration by default. 
To enable, set `SMTP_URL`.
For example: `export SMTP_URL=smtp://username:password@mail.yourdomain.com:587?tls=true`.

## Enabling SSO provider (Social registration)

This project does not allow SSO registration by default.
To enable, set following environment variables (you can do all or only specific ones):

- Google SSO:
```sh
export GOOGLE_CLIENT_ID=your-google-client-id
export GOOGLE_CLIENT_SECRET=your-google-client-secret
```

- Apple SSO:
```sh
export APPLE_CLIENT_ID=your-apple-client-id
export APPLE_CLIENT_SECRET=your-apple-client-secret
```

## API Examples

### Login / Registration (must be enabled)

### Refreshing a token

### Fetching recommendations

### Reacting to recommendations

### Updating a profile

### Deleting a profile

### Deleting a user

## Running evaluations

Evaluation script runs a benchmark on an `input-*.csv` dataset, available in [./cmd/evals/data/](./cmd/evals/data/). Run:
```sh
go run cmd/eval/main.go
```

Following methods will be evaluated:
- Okapi BM25
- Okapi BM25 with Intelligent Scoring
- `nomic-ai/nomic-embed-text-v1` embeddings 
- `nomic-ai/nomic-embed-text-v1` embeddings with a `BAAI/bge-reranker-base` reranker
- `nomic-ai/nomic-embed-text-v1` embeddings with a `BAAI/bge-reranker-base` reranker and with Intelligent Scoring

The results are available in [./cmd/evals/results.json](./cmd/evals/results.json).

## Misc

### Hot-reload server

```sh
air --build.cmd "go build -o bin/api cmd/server/main.go" --build.entrypoint "./bin/api"
```

### Recommended subdomains

- Since we do not have `/api` in the routes, we recommend having an `api` subdomain, for example: `api.hirevec.com`
- Since we do not provide documentation via API or via SwaggerUI, we recommend having a `docs` subdomain, for example: `docs.hirevec.com`
- The home website resides at `hirevec.com`
- Postgres can reside at `api.hirevec.com`
