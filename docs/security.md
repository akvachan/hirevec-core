## Authentication

- The server implements OAuth2.0 Authorization Code Flow with PKCE. Developers should familiarize themselves with [rfc6749](https://www.rfc-editor.org/rfc/rfc6749).
- TLS version used: 1.3.
- Token schema used: PASETO.
- Our primary client is a native application.
- Client identifiers are issued by the database as UUIDs.
- Client authentication is fully passwordless, a provider's SSO is used. Following providers are supported:
    - Google
    - Apple
- There are no plans to support password-based client authentication.
- Server uses scope-based authorization with higher-lever roles:
    - Roles: Recruiter (role:recruiter), Candidate (role:candidate)
    - Tiers: Free (tier:free), Premium (tier:premium)
    - There are 10 simple rules: 
        1. All roles can access public OAuth2 endpoints.
        2. Recruiters can only access candidate endpoints and recruiter reaction endpoints.
        3. Candidates can only access position endpoints and candidate reaction endpoints.
        4. Free tier has a limit of 60 candidates or positions and 60 reactions per day.
        5. Premium tier has limit of 120 candidates or positions and 120 reactions per day.
        6. Upon each reaction a match/nomatch is sent back to client, client can fetch up to 60 matches per hour that relate to him and only him.
        7. Candidates and recruiters can access their personal records and their personal profile deletion endpoint.
        8. Recruiters can request their positions.
        9. Free tier recruiters can create up to 10 positions per day.
        10. Premium tier recruiters can create up to 30 positions per day.
    - It is a responsibility of the client to cache the items.
    - Currently **only** free tier is implemented.
    - Premium tier will need additional endpoints for promotion and billing.
    - Upon promotion from the free tier to a premium tier the client is issued entirely new access and refresh token.
