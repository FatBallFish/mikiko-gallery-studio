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

Admin and end-user cookies remain separate. Refresh state is process-local, as in
the already deployed admin implementation; an API restart invalidates the admin
refresh cookie and safely returns the operator to login.

## Verification

- Service tests prove rotation, new access-token validity, and replay rejection.
- Router tests prove login cookie issuance, refresh success, and missing-cookie
  rejection rather than root-handler fallback.
- Frontend contracts reject malformed HTTP 200 refresh payloads and accept valid
  replacement sessions.
- Full repository verification, review gate, API smoke, and runtime browser/API
  checks cover both valid and invalid refresh-cookie paths.

