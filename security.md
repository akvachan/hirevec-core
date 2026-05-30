# Security 

## Constraints
- Applications's primary client is a native application.
- Each client is responsible for one user.
- User identifiers are [ULIDs](https://github.com/ulid/spec).
- Client authentication is fully passwordless (yes, even with e-mail).
- A provider's SSO is used.
- Tokens' JTIs are stored in the DB for easy revocation.
- The JTI is issued by the server as ULID.
- Refresh tokens can be invalidated.
- Access tokens = 15 minutes
- Refresh tokens = 30 days.
- No security bypass for development or testing.

## Flow
The server implements OAuth2.0 Authorization Code Flow via OIDC. 
Developers should familiarize themselves with [RFC6749](https://www.rfc-editor.org/rfc/rfc6749).

## Tokens Schema
PASETO (Platform-Agnostic Security Tokens). 
Developers should familiarize themselves with [PASETO](https://paseto.io/).

## Authentication
Utilize `/oauth/authorize` to obtain an access and refresh tokens. 

After successful authentication:
- If user has profile, the client is issued a token pair.
- If user does not have a profile yet, the client is issued a onboarding access token (24 hours).

## Authorization
Server uses role-based authorization:

- `role:recruiter`: Recruiter role
- `role:candidate`: Candidate role
- `role:onboarding`: Onboarding role; can send `POST` to `/v1/me/profile` once.

Basic claims extraction and scope checking is handled by middleware.
Handlers **still** need to make decisions whether to authorize user actions or not.
