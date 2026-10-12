# Browser sessions

The dashboard restores the signed-in account after refresh and in new tabs. Its
existing PostgreSQL session expires 24 hours after login; page loads do not extend
that deadline. Logout revokes the current session, and password change/reset
revokes all of that account's sessions through the existing recovery logic.

## REST and browser flow

`POST /auth/session-login` accepts email/password JSON and requires the exact
configured `FRONTEND_ORIGIN` header. It issues a fresh HttpOnly session cookie and
returns only user, expiry and a session-bound CSRF token. A previous browser
session is revoked after successful login. `GET /auth/session` validates the
cookie against PostgreSQL and returns the same public session data; it never
returns the session credential. Missing, expired or revoked sessions return 401.

The frontend uses credentialed fetch, keeps only user/expiry/CSRF data in memory,
and waits for restoration before redirecting private pages. An API outage offers
a retry instead of assuming the user logged out. Failed logout shows an error
and keeps the active UI session until logout can be confirmed. Successful logout
clears the cookie. Expired/revoked cookies are cleared on failed authentication.
Another tab detects revocation on its next API request or refresh.

All unsafe cookie-authenticated REST requests require both the exact frontend
origin and `X-CSRF-Token`. The token is a domain-separated SHA-256 derivation of
the random 256-bit session credential. It is compared in constant time, changes
when the session rotates, and cannot authenticate a request itself. CORS allows
credentials only for the configured origin and permits this custom header.
JSON-only, origin-checked login prevents login CSRF before a token exists.
Console access continues to use short-lived tickets minted by an authorized,
CSRF-protected request; the session cookie grants no direct host console access.

`POST /auth/login` still returns a bearer token for existing REST/operator clients.
An explicit Authorization header uses bearer authentication and never falls back
to a browser cookie. Browser session restoration requires a cookie. Bearer
clients do not require CSRF headers, because credentials are explicitly supplied
rather than attached by the browser.

## Deployment

Production uses `__Host-blockyard_session`: Secure, HttpOnly, SameSite=Lax,
Path=/, no Domain. The API selects Secure cookies in production or when the
configured frontend origin uses HTTPS, including behind TLS termination. It
does not trust forwarded headers to disable security. HTTP local development
uses the distinct `blockyard_dev_session` name with Secure disabled.

Keep the frontend and API on the same site and scheme: for example
`https://dashboard.example.com` and `https://api.example.com`. The host-only cookie
belongs to the API host. HTTPS and correct CORS origins are required. Unrelated
frontend/API domains are not supported by this SameSite policy; do not weaken it
to bypass a deployment mismatch. In local development use the same hostname for
both URLs, such as `127.0.0.1:3000` and `127.0.0.1:8080`, or localhost for both.
Rebuild the frontend when changing `NEXT_PUBLIC_API_URL`.

No schema migration or new dependency is required. HttpOnly limits credential
access from JavaScript but does not prevent malicious scripts from making actions
in an already authenticated browser; retain the application's existing input,
output and server authorization protections.

## Validation

PostgreSQL API tests cover secure cookie attributes, restoration, login origin,
CSRF rejection, rotation, logout, expiry, password-change revocation, bearer
compatibility and omission of credentials from responses/logs. Browser tests
cover refresh on a server page, CSRF headers, logout across refresh, outage retry
and failed logout. Follow [CODE_QUALITY.md](CODE_QUALITY.md) for commands.

The cookie flags and CSRF approach follow [MDN cookie guidance](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)
and [OWASP CSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).
