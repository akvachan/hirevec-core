# Security 

## Constraints
Our primary client is a native application. Each client is responsible for one user. User identifiers are issued by the database as UUIDs.
Client authentication is fully passwordless, a provider's SSO is used.

## Flow
The server implements OAuth2.0 Authorization Code Flow with OIDC and PKCE. Developers should familiarize themselves with [rfc6749](https://www.rfc-editor.org/rfc/rfc6749).

## PASETO
We use PASETO (Platform-Agnostic Security Tokens) for access and refresh tokens, which are more secure and easier to implement than JWTs. Developers should familiarize themselves with [PASETO](https://paseto.io/).

## Authentication
Anyone can access authentication endpoint (currently `/api/v1/auth/login/{provider}`) to obtain an access and refresh tokens. 
After successful authentication:
    - If the user already has a profile, the client is issued a pair of access and refresh tokens with the appropriate scopes based on their profile type (candidate or recruiter).
    - If the user does not have a profile, the client is issued a short-lived onboarding access token (24 hours) with the `candidates:write`, `recruiters:write` scope, which allows them to create one candidte and one recruiter profile. 
    - If the onboarding access token expires before the user creates a profile, they can simply authenticate again to obtain a new registration access token.

After profile creation, the client is issued a pair of access and refresh tokens with the appropriate scopes based on their profile type (candidate or recruiter).
Refresh tokens are not stored in the DB, instead their JTI is stored.
This way we can easily invalidate refresh tokens and guarantee that each refresh_token is unique. 
Access tokens have a lifespan of 15 minutes, refresh tokens have a lifespan of 30 days.

## Authorization
Server uses scope-based authorization:
    - Scopes: 
        - `role:recruiter`: Recruiter role
        - `role:candidate`: Candidate role
    - After profile creation, the client can  a new access token that will contain the following scopes based on the profile type:
        - `role:recruiter` has following permissions:
            - Can access "candidates" with the method GET
            - Can access "positions"  with the method GET
            - Can access "recruiters/reactions" with the method POST if id == claims.UserID
        - For candidates: `role:candidates` 

Authentication and authorization are handled by middleware, so all protected endpoints will require a valid access token with the appropriate scopes. 
Handlers do not need to worry about authentication and authorization, they can assume that if the request reaches them, the user is authenticated and authorized to perform the action.
