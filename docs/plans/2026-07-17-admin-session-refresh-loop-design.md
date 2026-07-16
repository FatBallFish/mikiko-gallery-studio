# Admin Session Refresh Loop Design

## Context

The running admin frontend contains silent session refresh support, while the API
currently deployed from this branch does not register the matching admin refresh
route. Requests to the missing route fall through to the root handler and return
HTTP 200 with a `bootstrap-ready` payload. The frontend treats that payload as a
session, stores an empty access token, and triggers another dashboard/config load.
Those requests return 401 and restart the cycle.

## Decision

Restore the complete admin refresh protocol without merging unrelated UI work:

1. Admin login issues an access token plus a rotating, opaque refresh token in a
   dedicated HttpOnly cookie.
2. `POST /api/ops/admin/v1/auth/session/refresh` validates and rotates that
   refresh token, then returns a new access token and refresh cookie.
3. Missing, expired, replayed, or otherwise invalid refresh tokens return 401 and
   clear the refresh cookie.
4. The admin frontend uses the shared client's existing singleflight behavior,
   validates that refresh responses contain a non-empty access token, persists a
   valid replacement session, and retries each original request at most once.
5. Refresh failure clears the stored admin session once and routes to login.
6. A delayed 401 produced with an older access token reuses the access token that
   another request already refreshed instead of rotating the refresh token again.
7. Logout, refresh replay, password reset, administrator role/status changes, and
   administrator deletion revoke the complete access/refresh session family.
8. Transient administrator-store failures return 5xx without clearing or revoking
   the refresh cookie; only terminal authentication failures clear it.
9. A new tab with no sessionStorage access token performs one cookie-only bootstrap
   refresh, while a session generation guard prevents a late refresh from restoring
   state after logout.
10. Login, refresh, and logout serialize their cookie-mutating requests in one
    frontend queue, with a same-origin Web Lock when available. Each queued network
    or lock operation has a 10-second abort budget. Logout keeps the login form
    unavailable until the queued request settles, and only clears the local session
    after a confirmed response; a failed request returns to the existing session
    with an explicit error instead of claiming success.
11. A transient failure while issuing the replacement session restores the current
    refresh token to active. Logout also revokes a locally verified bearer family
    when the administrator store is temporarily unavailable, while preserving the
    original 5xx response.

Admin and end-user cookies remain separate. Refresh state is process-local, as in
the already deployed admin implementation; an API restart invalidates the admin
refresh cookie and safely returns the operator to login. Until refresh state moves
to Redis or the database, production must run a single API replica. Each admin is
limited to 16 live session families and each family to 64 retained generations;
expired and revoked entries are removed opportunistically.

## Verification

- Service tests prove rotation, new access-token validity, and replay rejection.
- Router tests prove login cookie issuance, refresh success, and missing-cookie
  rejection rather than root-handler fallback.
- Frontend contracts reject malformed HTTP 200 refresh payloads and accept valid
  replacement sessions, and prove cookie-mutating auth requests run in order.
- Full repository verification, review gate, API smoke, and runtime browser/API
  checks cover both valid and invalid refresh-cookie paths.
