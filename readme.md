# Hirevec

## About Hirevec

Hirevec is a modern job recommendation engine that finds jobs and suitable candidates.

- Homepage: hirevec.com
- API: api.hirevec.com

### Recommendation Engine

The recommendation engine currently only employs a content-based approach.
There are plans to extend it to utilize collaborative filtering with matrix factorization.

#### Model Features

1. [x] Semantic/lexical similarity between candidate's CV and job posting (high weight)
2. [ ] Hard constraints (high weight):
   - [ ] Candidate is based in the same location
   - [ ] Candidate searches the same working mode (Remote / Hybrid / Office)
   - [ ] Candidate speaks required working language
3. [ ] Skills overlap (medium weight)
4. [ ] Semantic/lexical similarity between previous candidate's titles and title in the job posting (medium weight)
5. [ ] Weak constraints (low weight):
   - [ ] Candidate has required YOE
   - [ ] Candidate already worked for the company before

> [!NOTE]
> All implemented features are marked as [x].

## Quick Start

```sh
go run cmd/run.go
```

## API

> [!WARN]
> **If you are an external API consumer or a legacy system, use versioned routes!**
> You can do so, by appending `/v1` after the base url. E.g.: `http://localhost:8888/v1/users`.
>
> If no version is specified, the latest version is used,
> which will break your integrations when a new version of the route is released.
>
> Some routes do not require versions:
>
> - `/health`
> - `/auth/token`
> - `/auth/authorize`
>
> These routes are stable and will not change in a way that may break previous integrations.

### Create a user

- Request:

```sh
curl -X POST http://localhost:8888/users \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@test.com",
    "full_name": "Test Tester",
    "password": "test"
  }'
```

- Response:

```json
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJleHAiOiIyMDI2LTA2LTA2VDE1OjI2OjAyWiIsImlhdCI6IjIwMjYtMDYtMDZUMTQ6NTY6MDJaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wNlQxNDo1NjowMloiLCJwcm92aWRlciI6ImVtYWlsIiwic2NvcGUiOiIiLCJzdWIiOiJ1c3JfMDFrdGVweTRmM2ZiOHMzMjMxM2dmYjhzMzIifSWnab1ikJkMTA8L_bqmNUt4eWVYOnDzVSd5tFwE7M3d0tv9Poqne_oFUmrjtKzjMcfb3rMIRj-DenlAmOC-hQc",
  "token_type": "Bearer",
  "expires_in": 1800,
  "scope": "",
  "user_id": "usr_01ktepy4f3fb8s32313gfb8s32"
}
```

Store `access_token` somewhere safe. **Do not share it with anyone**.
If e-mail confirmation is enabled, confirm the email first before proceeding.

### Create a candidate profile

If you are a candidate, create a candidate profile, notice that we are using
`ACCESS_TOKEN` that we got from the registration.

- Request:

```sh
curl -X POST http://localhost:8888/candidates \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{
    "about": "Test Description"
  }'
```

- Response:

```
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdGVxcTllcm00c2E5dzd6MGVtNHNhOXciLCJleHAiOiIyMDI2LTA2LTA2VDE1OjM5OjQ2WiIsImlhdCI6IjIwMjYtMDYtMDZUMTU6MDk6NDZaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wNlQxNTowOTo0NloiLCJwcm92aWRlciI6ImVtYWlsIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSAiLCJzdWIiOiJ1c3JfMDFrdGVweTRmM2ZiOHMzMjMxM2dmYjhzMzIifbu4KXceqMxG6wSq8y4UY3klBnVlZflv2Om7KcmZigbHNR_cBr7HT_aGcRvALvLwOeVe_-cOk1caA1mCXLZPtgI",
  "token_type": "Bearer",
  "expires_in": 1800,
  "refresh_token": "v4.local.yaBaCfLzq1RY-Jm60Y_Gh67LOX_S9eC2PYmlhNESSuaZR9ve8E-J1P_50ot74B1QlZghCGTOod6TlJItjIfMQBv8OEsLJsdS5SsS4hMvY3tUlcxKlLh1CXFa8YkVn4iz_HDu_G_ajNw0BN7B4nXuEHrdYmyo-O5PoiFobTH769neqFC4iIJ8oQhiyCh5Jbm4AIGKOyZ3QenVHohg8qJM31z1WrftOky4fpYRHiJmzXeXYZ_OErOM3tBhvt2PlXJ8txm55pz12EnFrnMWNZEW-HXI8lmo8pxUcAb7Y5F_yOdZWyOnuXTiTxkG1qMNVGpipLeEUEI5e7b8x54xpFI7xCLZGDzRdWIVo3Q0LI7K4Q_dQdLLkW5aBNT6fHC0W-rFLJ6Rpy-oIZ1d",
  "scope": "role:candidate ",
  "user_id": "usr_01ktepy4f3fb8s32313gfb8s32"
}
```

Store new `access_token` and `refresh_token` somewhere safe.
**Do not share them with anyone**.

### Create a recruiter profile

If you are a recruiter, create a recruiter profile.

- Request:

```sh
curl -X POST http://localhost:8888/recruiters \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

- Response:

```json
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdGVxcTllcm00c2E5dzd6MGVtNHNhOXciLCJleHAiOiIyMDI2LTA2LTA2VDE1OjQ4OjI5WiIsImlhdCI6IjIwMjYtMDYtMDZUMTU6MTg6MjlaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wNlQxNToxODoyOVoiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmVjXzAxa3RlcXI5a2owZjZyZjI1Z2c4MGY2cmYyIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdGVweTRmM2ZiOHMzMjMxM2dmYjhzMzIifaUWJFndw0SrNJ-O5yWJYhaFq32GLB1137iaPAmroNtQnyn4IUqd9JQJXIRLMKD-QGKeBIMDxMU8aMw6XkzyfgA",
  "token_type": "Bearer",
  "expires_in": 1800,
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktepy4f3fb8s32313gfb8s32",
  "candidate_id": "can_01kteqq9erm4sa9w7z0em4sa9w",
  "recruiter_id": "rcr_01kteqr9kj0f6rf25gg80f6rf2"
}
```

Since we created a candidate in a previous step, we only get a new access token,
you can reuse your refresh token until it expires or gets revoked.

### Login (expires in 30 days)

You can always "login" to get a new refresh token.
The old one is preserved, but keep in mind that only up to 5 refresh tokens are allowed per user.
Old refresh tokens are revoked automatically.

- Request:

```sh
curl -X POST http://localhost:8888/auth/authorize \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@test.com",
    "password": "test"
  }'
```

- Response:

```json
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdGVxcTllcm00c2E5dzd6MGVtNHNhOXciLCJleHAiOiIyMDI2LTA2LTA2VDE1OjQxOjQ0WiIsImlhdCI6IjIwMjYtMDYtMDZUMTU6MTE6NDRaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wNlQxNToxMTo0NFoiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmVjXzAxa3RlcXI5a2owZjZyZjI1Z2c4MGY2cmYyIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdGVweTRmM2ZiOHMzMjMxM2dmYjhzMzIifeuc2nIZD8IjxZ_YP02PkPgeBactQaXjK9dZR1W3fM3NI2Y3GTLx2AzSzr60oTy-NYuM8uGRLkWI4PHoVs7UyQk",
  "token_type": "Bearer",
  "expires_in": 1800,
  "refresh_token": "v4.local.3W5C9xV-nBOp8cShxN2xsGE0uzRFU-tFd2HDOzbLXWZA9IhMmxgdY1FF-pt05sEouMCcItRtMNHwhRtTCeOqPGMBbefX-Xv_N91pPyR-GC2DmyntFnMYbzL18T_9e_xOB_BBh8Ji3RLaMrvCmZNQo-h8dvLc_zuLo9d4Mr7SXu8-Yb68uaowKWAccLz9fWOp47QlHOtyEHgrCWwI-dIEUNhgYluSKjJRerXsrJ1EoCfbbdRqVCprBtCl5HhPXbZ0LKJjp_UUjsTBPl4OrpvLHJFhCeoTlM9YGphQ58TpEPXQMC0RApO_ULLNuIloFdr0T79MDWAZGF35nsE256aQSjz_-aB_8XEOQVFqDwiHlpZNt42-t4_PFp_Xe2pjaYhk910a_WrCZ1Bq",
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktepy4f3fb8s32313gfb8s32"
}
```

### Refresh access token (expires in 30 minutes)

If access token expires, you need to get a new one using your refresh token:

- Request:

```sh
curl -X POST http://localhost:8888/auth/token \
  -H "Content-Type: application/json" \
  -d "{
    \"grant_type\": \"refresh_token\",
    \"refresh_token\": \"$REFRESH_TOKEN\"
  }"
```

- Response:

```
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdGVxcTllcm00c2E5dzd6MGVtNHNhOXciLCJleHAiOiIyMDI2LTA2LTA2VDE1OjQyOjMyWiIsImlhdCI6IjIwMjYtMDYtMDZUMTU6MTI6MzJaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wNlQxNToxMjozMloiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmVjXzAxa3RlcXI5a2owZjZyZjI1Z2c4MGY2cmYyIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdGVweTRmM2ZiOHMzMjMxM2dmYjhzMzIifTc2qfvpx-QnnRqGKo1NMpyf298IcwxEZCOp-Put9PKWHAeTZDBXXUEnhV3yV2cmd3r4rjXC1_PZxyJd4HsEWgs",
  "token_type": "Bearer",
  "expires_in": 408702976,
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktepy4f3fb8s32313gfb8s32",
  "candidate_id": "can_01kteqq9erm4sa9w7z0em4sa9w",
  "recruiter_id": "rcr_01kteqr9kj0f6rf25gg80f6rf2"
}
```

### Get my user data

- Request:

```sh
curl http://localhost:8888/users/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

- Response:

```json
{
  "status": "success",
  "data": {
    "user": {
      "id": "usr_01ktepy4f3fb8s32313gfb8s32",
      "provider": "email",
      "email": "test@test.com",
      "full_name": "Test Tester",
      "user_name": "silent_koala5c13",
      "update_at": "2026-06-09T19:38:58Z"
    }
  }
}
```

### Update my user data

- Request:

```sh
curl -X PATCH http://localhost:8888/users/me \
  -H "Content-Type: application/json" \          
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{
    "full_name": "Different Name",             
    "user_name": "cool_tester"
  }'
```

- Response:

```json
{
  "status": "success",
  "data": {
    "user": {
      "id": "usr_01ktepy4f3fb8s32313gfb8s32",
      "provider": "email",
      "email": "test@test.com",
      "full_name": "Different Name",
      "user_name": "cool_tester",
      "update_at": "2026-06-09T19:38:58Z"
    }
  }
}
```

### Create a position

If you have a recruiter bound to your user, you can create positions.

- Request

```sh
curl
```

### Get recommendations

Recommendations are served for the entire user account:

```sh
curl
```

You can include reacted recommendations:

```sh
curl
```

You can also include only recommendations for candidate:

```sh
curl
```

### React to recommendations

Since you now have some new recommendations, you can react to them positively:

```sh
curl
```

Or negatively:

```sh
curl
```

### See your matches

After some time, the other side will also react to your positions
or candidate profile and you get a match:

```sh
curl
```

> [!TIP]
> If you are developing or testing, you can also run:
>
> ```sh
> go run cmd/create_test_data/main.go
> ```
>
> This prepares the entire development environment for you (bunch of test
> users, candidates and positions) and saves refresh tokens for each user in
> `.env`.

## Q&A

1. Can I create recommendations manually?

- No, this is not supported via API. You can of course create DB records,
  but overall it's the responsibility of the recommendation engine.

2. Can I enforce E-mail confirmation / SSO?

- Yes, see instructions below.

3. Can I edit user reactions and/or remove matches?

- No, this is not possible via API. Reactions and matches must be immutable in
  order for the recommendation engine to be stable.

## Settings

All available settings are demonstrated in [./.example.env](./.example.env).
You can copy-paste them into your `.env` file or just use `export` command.

### Enabling Postgres with pgvector

Recommendation engine uses SQLite by default.
To enable Postgres, set `POSTGRES_DATABASE_URL`:

```sh
export POSTGRES_DATABASE_URL=postgres://postgres@localhost:5432/postgres?sslmode=disable
```

To enable `pgvector`, just have it installed on your machine.
Here is [how to install](https://github.com/pgvector/pgvector).

If Postgres is enabled, but `pgvector` cannot be configured,
the server will halt with an error.

### Enabling Embeddings and Reranker

Recommendation engine uses Okapi BM25 by default.
To enable embeddings and reranking, enable Postgres with pgvector first,
then set `TEI_BASE_URL` and `TEI_API_KEY`:

```sh
export TEI_BASE_URL=yourdomain.com
export TEI_API_KEY=your-api-key
```

Your TEI instance **must** be protected by an API key.
Here is [how to setup TEI](https://github.com/huggingface/text-embeddings-inference).

Okapi BM25 will be used in following scenarios:

- The embeddings are not yet created (cold start).
- SQLite database is used instead of Postgres.

### Enabling E-mail confirmation

E-mail confirmation is not enforced by default. Enable with:

```sh
export SMTP_URL=smtp://username:password@mail.yourdomain.com:465?tls=true.
```

### Enabling SSO

SSO registration/login is not enabled by default. Enable with:

```sh
export GOOGLE_CLIENT_ID=your-google-client-id
export GOOGLE_CLIENT_SECRET=your-google-client-secret
export APPLE_CLIENT_ID=your-apple-client-id
export APPLE_CLIENT_SECRET=your-apple-client-secret
```

Afterwards, users can login and register via:

```sh
curl -L localhost:8888/auth/authorize?provider=google
```

or for Apple:

```sh
curl -L localhost:8888/auth/authorize?provider=apple
```

## Misc

### Hot-reload server

```sh
air \
  --build.cmd "go build -o bin/api cmd/run.go" \
  --build.bin "./bin/api" \
  --build.include_ext "go,db,sql,env" \
  --misc.clean_on_exit true \
  --log.main_only true \
  --log.silent true \
  --screen.clear_on_rebuild true \
  --color never \
  --screen.keep_scroll false
```

### Remove database, cache, environment files and binaries

```
rm -rf bin/ tmp/ .db .env data/bm25_cache.json
```
