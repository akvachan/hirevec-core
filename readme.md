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
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJleHAiOiIyMDI2LTA2LTA5VDIxOjEyOjA5WiIsImlhdCI6IjIwMjYtMDYtMDlUMjA6NDI6MDlaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wOVQyMDo0MjowOVoiLCJwcm92aWRlciI6ImVtYWlsIiwic2NvcGUiOiIiLCJzdWIiOiJ1c3JfMDFrdHExeTFhYzhkZ3BrZmV6YmM4ZGdwa2YifdwpBTF7b_UVToVtMpdK4LFElFunfzSfbV6nPHwjtmCUnrZ_WlTlhxddT20K73Or1PIPmAxpkGVCMQr3mM-76A4",
  "token_type": "Bearer",
  "expires_in": 1800,
  "scope": "",
  "user_id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf"
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

```json
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdHExenNnMnBycXE5aG45eHZwcnFxOWgiLCJleHAiOiIyMDI2LTA2LTA5VDIxOjEzOjA2WiIsImlhdCI6IjIwMjYtMDYtMDlUMjA6NDM6MDZaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wOVQyMDo0MzowNloiLCJwcm92aWRlciI6ImVtYWlsIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSAiLCJzdWIiOiJ1c3JfMDFrdHExeTFhYzhkZ3BrZmV6YmM4ZGdwa2YifQ8fxsoGASeBW6FVJVwKk43Zpgu8pLc9ayfGwrr3UHjHVUF-Csiv_VEOohQafJmXPHVpbtd3HemaczCNdgdLlQk",
  "token_type": "Bearer",
  "expires_in": 1800,
  "refresh_token": "v4.local.cUzKtoot3YKjK_kD86gQNUPWKKY9c4WqQAJZqYliXElv1gQtKv3LrX8oBJ0EDY-U17Z1yUXEqwoF4HkbSXZF7FZ63j2hO6ta0TpO8nnNGWCYsyVEuSaAPRMrAes7jiTBh81LaV-dyQDNx0xEnZ3_oQ8EO94ZGmQYlQ5gtQjCxjmDLXAm5HAvU9oErAoxonKaaYz-bE11cgBrADMHLx2BWuO41Ir45sq-Lsey9lOiyc0Jn36gHAvn8mac6uK-3y0cWdHhwrxudSTf4urUJ1-aZw96N81qZbv2odKFiZoQ7BPVQUtGiTCxk4zVlqt5EcNr25QGEtwF_TuvL1vFV2F8-bBIbs-R79GWa5FXawNENNVWPm-dgKai2Ou7iiuyv7XPR6hQkGsfLcIJ",
  "scope": "role:candidate ",
  "user_id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf"
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
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdHExenNnMnBycXE5aG45eHZwcnFxOWgiLCJleHAiOiIyMDI2LTA2LTA5VDIxOjE0OjA4WiIsImlhdCI6IjIwMjYtMDYtMDlUMjA6NDQ6MDhaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wOVQyMDo0NDowOFoiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmNyXzAxa3RxMjFwMmMwY3h6MzFkZGdmMGN4ejMxIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdHExeTFhYzhkZ3BrZmV6YmM4ZGdwa2YifVnpgqlPWNzHk1Szq-91n8rCyTKCWmZkuyyE-xIchjYasGqri3O4AOmEADl8z4U0ERFZlTzVZrdeepyvM4p4sgA",
  "token_type": "Bearer",
  "expires_in": 1800,
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf"
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
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdHExenNnMnBycXE5aG45eHZwcnFxOWgiLCJleHAiOiIyMDI2LTA2LTA5VDIxOjE0OjM3WiIsImlhdCI6IjIwMjYtMDYtMDlUMjA6NDQ6MzdaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wOVQyMDo0NDozN1oiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmNyXzAxa3RxMjFwMmMwY3h6MzFkZGdmMGN4ejMxIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdHExeTFhYzhkZ3BrZmV6YmM4ZGdwa2YifVeo_z9FSnu9PGJ9SkYMsjB7421pnyFHp6VG1WSfOyBrWyrm7AtglKQvR3NC72G_zE33HMHBDKC0OTPVZ-gFGQ0",
  "token_type": "Bearer",
  "expires_in": 1800,
  "refresh_token": "v4.local.JI2GmUdQisrKsYfR-UTmm3nsYujfg2WMTtqDhlIPv4uBl67d8X23WbOcN1jFvI6U-UUq69Pe-gH3ow7XaQCJUY6dGHtRu74KaN5L5MHkdeRBdyWJXah1ufSvW22MnN2T4jfw-wpp1Bg8gO7feq-24XYAkapD-17IERTBVO33Fq_QQUsUTuTQvrvpZlKeVmb92vjjl7KgSEoqhHWZwTDgdfYUHBsMYSTuMvhsx7cb8DE_BMwIOSBi0w1bgnURLEtMJVKdB5dzkRc5tPWhU9kC3k_j7nhUNm3qMuPJ-JMqmlhVK9yuo7B5QSiTUm3UjhtG-sBMWkUgMsPEo_OXmM2W1lgfhE0VNvJ_t8amllt2jrNwHZ4rbu46pdTQtEgGOZX0VWJSp64GXFAW",
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf"
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

```json
{
  "access_token": "v4.public.eyJhdWQiOiJhcGkuaGlyZXZlYy5jb20iLCJjYW5kaWRhdGVfaWQiOiJjYW5fMDFrdHExenNnMnBycXE5aG45eHZwcnFxOWgiLCJleHAiOiIyMDI2LTA2LTA5VDIxOjQxOjQ4WiIsImlhdCI6IjIwMjYtMDYtMDlUMjE6MTE6NDhaIiwiaXNzIjoiYXBpLmhpcmV2ZWMuY29tIiwibmJmIjoiMjAyNi0wNi0wOVQyMToxMTo0OFoiLCJwcm92aWRlciI6ImVtYWlsIiwicmVjcnVpdGVyX2lkIjoicmNyXzAxa3RxMjFwMmMwY3h6MzFkZGdmMGN4ejMxIiwic2NvcGUiOiJyb2xlOmNhbmRpZGF0ZSByb2xlOnJlY3J1aXRlciAiLCJzdWIiOiJ1c3JfMDFrdHExeTFhYzhkZ3BrZmV6YmM4ZGdwa2YifZWsm6Z27qEKVRKX36fwwfXOB0H-9S-XtCBRkg-4gIcUPcIaSivXQs8jnKWT63dYqleTEWoqbPC7Wn5u7T8tPgo",
  "token_type": "Bearer",
  "expires_in": 1800,
  "scope": "role:candidate role:recruiter ",
  "user_id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf"
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
      "id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf",
      "provider": "email",
      "email": "test@test.com",
      "full_name": "Test Tester",
      "user_name": "brave_foxb6ea3a43",
      "updated_at": "2026-06-09T20:42:09Z"
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
      "id": "usr_01ktq1y1ac8dgpkfezbc8dgpkf",
      "provider": "email",
      "email": "test@test.com",
      "full_name": "Different Name",
      "user_name": "cool_tester",
      "updated_at": "2026-06-09T21:14:42Z"
    }
  }
}
```

### Delete my user data

- Request:

```sh
curl -X DELETE http://localhost:8888/users/me \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{
    "password": "test"
  }'
```

- Response:

```json
{
  "status": "success"
}
```

### Create a position

If you have recruiter role, you can create positions.

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

## Features

All currently available settings are demonstrated in [./.example.env](./.example.env).
You can copy-paste them into your `.env` file or just use `export` command.

### Checklist

- [ ] Users can enable Postgres
- [ ] Users can enable TEI (embeddings and reranker worker)
- [ ] Users can enable SMTP relay for security-critical operations
- [ ] Users can enable SSO with Google and Apple

## Misc

### Hot-reload server

```sh
air \
  --build.cmd "go build -o bin/api cmd/run.go" \
  --build.bin "./bin/api" \
  --build.include_ext "go,sql,env" \
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
