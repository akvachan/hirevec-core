# Security

## Constraints

- Applications's primary client is a native application.
- Each client is responsible for one user.
- User identifiers are [ULIDs](https://github.com/ulid/spec).
- Google and Apple SSO is supported.
- Refresh token JTIs are stored in the DB for easy revocation.
- The JTI is issued by the server as ULID.
- Access token expiration = 30 minutes.
- Refresh token expiration = 30 days.
- Developers and testers must go through the same security layers as ordinary users:
  - No exceptions
  - No bypasses
  - No impersonation endpoints
  - No development builds

## Flow

The server implements OAuth2.0 Authorization Code Flow via OIDC.
Developers should familiarize themselves with [RFC6749](https://www.rfc-editor.org/rfc/rfc6749).

## Tokens Schema

PASETO (Platform-Agnostic Security Tokens).
Developers should familiarize themselves with [PASETO](https://paseto.io/).

## Authentication

- Utilize `/auth/authorize` to obtain an access and refresh tokens.
- Utilize `/auth/token` to get a new access token after the old one expires.

## Authorization

Server uses coarse-grained role-based authorization (RBAC):

- `role:recruiter`: Recruiter role, can access recruiter-specific endpoints.
- `role:candidate`: Candidate role, can access candidate-specific endpoints.

Basic claims extraction and scope checking is handled by middleware.
Handlers **still** need to make decisions whether to authorize user actions or not.

## Rotating credentials

Overview of required credentials when performing security-critical operations:

| Operation                    | Implemented? | Route                        | E-mail | Password | E-mail Confirmation | Access Token | Refresh Token |
| ---------------------------- | ------------ | ---------------------------- | ------ | -------- | ------------------- | ------------ | ------------- |
| Change e-mail                | No           | `POST /auth/change-email`    | Yes    | Yes      | Yes                 | No           | No            |
| Change password              | No           | `POST /auth/change-password` | No     | Yes      | No                  | Yes          | No            |
| Forgot password              | No           | `POST /auth/forgot-password` | Yes    | No       | Yes                 | No           | No            |
| Obtain refresh token (Login) | Yes          | `POST /auth/authorize`       | Yes    | Yes      | No                  | No           | No            |
| Obtain access token          | Yes          | `POST /auth/token`           | No     | No       | No                  | No           | Yes           |
