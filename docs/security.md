# Security 

## Constraints
Applications's primary client is a native application. Each client is responsible for one user. User identifiers are issued by the database as [ULIDs](https://github.com/ulid/spec).
ULIDs have several nice properties: they are more lightweight than UUIDs, they are sortable and can be generated in a distributed fashion.
Client authentication is fully passwordless, a provider's SSO is used.

## Flow
The server implements OAuth2.0 Authorization Code Flow via OIDC. Developers should familiarize themselves with [RFC6749](https://www.rfc-editor.org/rfc/rfc6749).

## Tokens Schema
PASETO (Platform-Agnostic Security Tokens) are used for access and refresh tokens.
PASETO is a more robust alternative to JWTs. 
Developers should familiarize themselves with [PASETO](https://paseto.io/).

## Authentication
Anyone can access authentication endpoint (currently `/v1/auth/login/{provider}`) to obtain an access and refresh tokens. 

After successful authentication:

- If the user already has a profile, the client is issued a pair of access and refresh tokens with the appropriate scopes based on their profile type (candidate or recruiter).
- If the user does not have a profile, the client is issued a short-lived onboarding access token (24 hours) with the `candidates:write`, `recruiters:write` scope, which allows them to create one candidate and one recruiter profile.
- If the onboarding access token expires before the user creates a profile, they can simply authenticate again to obtain a new registration access token.

Refresh tokens are not stored in the DB, nor are their hashes, instead their JTIs are stored.
The JTI is issued by the database as ULID.
Refresh tokens can be invalidated by setting a flag in the table for a specific token.
Access tokens have a lifespan of 15 minutes, refresh tokens have a lifespan of 30 days.

## Authorization

Server uses scope-based authorization:

- `role:recruiter`: Recruiter role
- `role:candidate`: Candidate role
- `candidates:write`: Can write to the candidates table
- `recruiters:write`: Can write to the recruiters table

Authentication and authorization are handled by middleware, so all protected endpoints will require a valid access token with the appropriate scopes.
Handlers do not need to worry about authentication and authorization, they can assume that if the request reaches them, the user is authenticated and authorized to perform the action.
