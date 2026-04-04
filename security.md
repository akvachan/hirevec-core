# Security 

## Constraints
- Applications's primary client is a native application.
- Each client is responsible for one user.
- User identifiers are issued by the database as [ULIDs](https://github.com/ulid/spec).
- Client authentication is fully passwordless.
- A provider's SSO is used.
- Tokens are not stored in the DB, nor are their hashes, instead their JTIs are stored.
- The JTI is issued by the database as ULID.
- Refresh tokens can be invalidated.
- Access tokens have a lifespan of 15 minutes, refresh tokens have a lifespan of 30 days.

## Flow
The server implements OAuth2.0 Authorization Code Flow via OIDC. Developers should familiarize themselves with [RFC6749](https://www.rfc-editor.org/rfc/rfc6749).

## Tokens Schema
PASETO (Platform-Agnostic Security Tokens) are used for access, refresh and state tokens. Developers should familiarize themselves with [PASETO](https://paseto.io/).

## Authentication
Anyone can access authentication endpoint (currently `/oauth/authorize`) to obtain an access and refresh tokens. 

After successful authentication:
- If the user already has a profile, the client is issued a pair of access and refresh tokens with the appropriate scopes based on their profile type (candidate or recruiter).
- If the user does not have a profile, the client is issued a short-lived onboarding access token (24 hours) with the `role:onboarding` scope, which allows them to create one candidate and one recruiter profile.
- If the onboarding access token expires before the user creates a profile, they can simply authenticate again to obtain a new registration access token.

## Authorization
Server uses role-based authorization:

- `role:recruiter`: Recruiter role
- `role:candidate`: Candidate role
- `role:onboarding`: Special onboarding role; can send `POST` to `/v1/me/profile` once.

Basic claims extraction and scope checking is handled by middleware.
Handlers **still** need to make decisions whether to authorize user actions or not.
