## Security Fundamentals

Token Management
- [ ] Use cryptographically secure random token generation (crypto/rand, not math/rand)
- [ ] Hash refresh tokens before storing (you're doing this ✓)
- [ ] Store access tokens as opaque tokens or use short-lived JWTs
- [ ] Set appropriate token expiration (access: 15min-1hr, refresh: 30-90 days)
- [ ] Implement token rotation on refresh (issue new refresh token, revoke old one)
- [ ] Support refresh token revocation (logout, security events)
- [ ] Implement "revoke all sessions" functionality per user
- [ ] Clean up expired tokens periodically (background job)

State & CSRF Protection
- [ ] Generate cryptographically random state parameter for each OAuth flow
- [ ] Store state in secure, httpOnly session cookie or server-side cache
- [ ] Validate state matches on callback
- [ ] Set short expiration on state (5-10 minutes)
- [ ] Implement PKCE (Proof Key for Code Exchange) for additional security
- [ ] Use SameSite=Lax or Strict on cookies

Authorization Code Flow
- [ ] Never expose client secrets in frontend code
- [ ] Validate redirect_uri matches registered URIs exactly
- [ ] Use authorization code only once (mark as used after exchange)
- [ ] Set short expiration on authorization codes (5-10 minutes)
- [ ] Verify code was issued to the same client making the token request

## Cookie & Session Security

Cookie Configuration
- [ ] Set Secure flag (HTTPS only)
- [ ] Set HttpOnly flag (no JavaScript access)
- [ ] Set SameSite=Lax or Strict
- [ ] Use appropriate Domain and Path restrictions
- [ ] Implement proper cookie expiration matching token lifetime
- [ ] Consider separate cookies for access vs refresh tokens

Session Management
- [ ] Implement sliding session expiration
- [ ] Log all login/logout events
- [ ] Support concurrent sessions or enforce single session per user
- [ ] Clear all session data on logout
- [ ] Implement session fixation protection (regenerate session ID after login)

## Database & Storage

Schema Design
- [ ] Index token_hash column for fast lookups
- [ ] Index user_id for user-level operations
- [ ] Index expires_at for cleanup queries
- [ ] Add created_at timestamp for audit trail
- [ ] Consider storing device/IP info for session management
- [ ] Add last_used_at for detecting stale sessions

Data Integrity
- [ ] Use database transactions for multi-step operations
- [ ] Implement proper foreign key constraints
- [ ] Add unique constraints where appropriate
- [ ] Handle concurrent token operations safely (row-level locking)
- [ ] Implement soft deletes for audit requirements

## Error Handling & Logging

Error Responses
- [ ] Never leak sensitive info in error messages (no "wrong password" vs "user not found")
- [ ] Use generic error messages to users
- [ ] Log detailed errors server-side with request IDs
- [ ] Return proper HTTP status codes (401, 403, 500, etc.)
- [ ] Implement rate limiting on auth endpoints
- [ ] Add retry-after headers when rate limited

Security Logging
- [ ] Log all authentication attempts (success & failure)
- [ ] Log token creation, refresh, and revocation
- [ ] Log suspicious activity (multiple failed attempts, unusual locations)
- [ ] Include: timestamp, user ID, IP, user agent, action, result
- [ ] Never log tokens or passwords
- [ ] Implement log retention policy
- [ ] Set up alerts for suspicious patterns

## API Integration

Provider Communication
- [ ] Validate SSL certificates (don't skip verification)
- [ ] Set reasonable timeouts on HTTP requests
- [ ] Handle provider errors gracefully (rate limits, downtime)
- [ ] Cache provider's public keys for JWT verification (if applicable)
- [ ] Implement retry logic with exponential backoff
- [ ] Validate JWT signatures if provider uses JWTs
- [ ] Verify JWT claims (iss, aud, exp, iat)
