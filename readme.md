# Hirevec

## About Hirevec

Hirevec is a job recommendation engine that enables candidates to find jobs and recruiters to find candidates.

## Quick Start

The server implements **Okapi BM25** and uses **SQLite** by default. 

To start server:
```sh
go run cmd/server/main.go
```

## API

Create a test user:
```sh
# create a user and save a token somewhere
curl 

# create a candidate profile
curl

# create a recruiter profile
curl
```

Create test positions:
```sh
# create a single position
curl

# endpoint accepts batches as well
curl
```

Get recommendations:
```sh
curl
```

React to recommendations:
```sh
curl
```

See your matches:
```sh
curl
```

> [!TIP] If you are developing, you can also run:
> ```sh
> go run cmd/create_test_data/main.go
> ```
>
> This will prepare the entire test environment for you (bunch of test users, 
> candidates and positions) and will save tokens for
> each user in `test_tokens/` directory.

### Q&A

1. Can I create recommendations manually?
- No, this is not supported via API. You can of course create DB records,
    but overall it's the responsibility of the recommendation engine.

2. Can I enforce E-mail confirmation / SSO?
- Yes, see instructions below.

3. Can I edit user reactions and/or remove matches?
- No, this is not possible via API. Reactions and matches must be immutable in 
    order for the recommendation engine to remain simple.

## Enabling Postgres with pgvector

This project uses SQLite by default. 
To enable Postgres, set `POSTGRES_DATABASE_URL`:
```sh
export POSTGRES_DATABASE_URL=postgres://postgres@localhost:5432/postgres?sslmode=disable`.
```

To enable `pgvector`, just have it installed on your machine.
Here is [how to install](https://github.com/pgvector/pgvector).

If Postgres is enabled, but `pgvector` cannot be configured, 
the server will halt with an error.

## Enabling Embeddings and Reranker

This project uses Okapi BM25 by default. 
To enable embeddings and reranking, enable Postgres with pgvector first, 
then set `TEI_BASE_URL` and `TEI_API_KEY`:
```sh
export TEI_BASE_URL=localhost:8080
export TEI_API_KEY=your-api-key
```

Your TEI instance **must** be protected by an API key.
Here is [how to setup TEI](https://github.com/huggingface/text-embeddings-inference).

Okapi BM25 will be used in following scenarios:
- The embeddings are not yet created (cold start).
- SQLite database is used instead of Postgres.

## Enabling E-mail confirmation

E-mail confirmation is not enforced by default. Enable with:
```sh
export SMTP_URL=smtp://username:password@mail.yourdomain.com:465?tls=true.
```

## Enabling SSO

SSO registration/login is not enabled by default. Enabled with:
```sh
export GOOGLE_CLIENT_ID=your-google-client-id
export GOOGLE_CLIENT_SECRET=your-google-client-secret
export APPLE_CLIENT_ID=your-apple-client-id
export APPLE_CLIENT_SECRET=your-apple-client-secret
```

## Misc

### Hot-reload server

```sh
air --build.cmd "go build -o bin/api cmd/server/main.go" --build.entrypoint "./bin/api"
```
