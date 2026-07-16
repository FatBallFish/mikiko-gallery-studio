# Admin Session Refresh Loop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refresh expired admin access tokens once when a valid refresh cookie exists and terminate at the login page when refresh is unavailable or invalid.

**Architecture:** Extend the existing admin authentication service with rotating process-local refresh sessions and expose a dedicated cookie-backed refresh endpoint. Wire the admin frontend to that endpoint through the shared HTTP client's existing singleflight retry mechanism, with strict response validation so unrelated HTTP 200 payloads cannot create empty sessions.

**Tech Stack:** Go `net/http`, JWT and opaque refresh tokens, React/TypeScript shared API client, executable TypeScript contracts, Docker Compose.

---

### Task 1: Define failing backend refresh behavior

**Files:**
- Modify: `internal/service/adminauth/service_test.go`
- Create: `internal/http/router/admin_auth_refresh_api_test.go`

1. Add a service test requiring login to issue a refresh token, refresh to rotate both tokens, and replay of the old refresh token to fail.
2. Add router tests requiring login to set the admin refresh cookie, refresh to return a new access token, and a request without the cookie to return 401.
3. Run the focused tests and confirm they fail because refresh tokens and the route do not exist.

### Task 2: Implement backend refresh protocol

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/load.go`
- Modify: `internal/config/load_test.go`
- Modify: `internal/domain/adminauth/types.go`
- Modify: `internal/service/adminauth/service.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/router.go`
- Modify: `api/openapi/openapi.yaml`

1. Add the dedicated admin refresh-cookie configuration with a safe default.
2. Add hashed refresh-session families, rotation, replay rejection, and logout revocation to the admin auth service.
3. Set, rotate, and clear the HttpOnly cookie in login, refresh, and logout handlers.
4. Register and document the refresh endpoint.
5. Run the focused service/router/config tests until green.

### Task 3: Define and implement frontend session validation

**Files:**
- Create: `web/shared/admin-session.contract.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/shared/api-types.ts`
- Modify: `web/admin/src/App.tsx`

1. Add a contract proving a malformed HTTP 200 refresh payload rejects and a valid payload yields a non-empty admin session.
2. Run the contract and confirm it fails because the admin refresh API is absent.
3. Add the API path, strict session normalization, and non-recursive refresh request.
4. Update the admin unauthorized handler to persist valid refreshed sessions and clear/redirect on failure.
5. Run the new contract plus admin typecheck/build.

### Task 4: Verify and deploy the protocol repair

**Files:**
- No additional production files.

1. Run `./scripts/workflow/verify.sh`.
2. Commit the scoped implementation.
3. Run committed-scope review and check the review gate.
4. Run the repository API smoke test.
5. Rebuild only the API service so the already-running newer admin frontend is not downgraded.
6. Verify a valid refresh cookie replaces an invalid access token and allows dashboard/config requests.
7. Verify a missing/invalid refresh cookie returns 401 and the admin UI stops at login without repeated calls.

