# Authentication Session TTL Defaults Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent deployments from issuing immediately expired authentication sessions when token TTL environment variables are omitted.

**Architecture:** Apply product-defined token lifetimes as backend configuration defaults and also declare them explicitly in the development Docker deployment. Test the backend fallback first, then validate the rebuilt runtime through the real login and profile APIs.

**Tech Stack:** Go configuration loader and tests, Docker Compose, PostgreSQL-backed authentication, shell API smoke checks.

---

### Task 1: Add the backend TTL regression test

**Files:**
- Modify: `internal/config/load_test.go`

1. Add a test that loads local environment configuration without either TTL variable.
2. Assert `AccessTokenTTL == 10*time.Minute` and `RefreshTokenTTL == 2*time.Hour`.
3. Run `go test ./internal/config -run TestLoadEnvDefaultsAuthenticationTokenTTLs -count=1` and confirm it fails because both values are zero.

### Task 2: Apply backend defaults

**Files:**
- Modify: `internal/config/load.go`

1. In `applyDefaults`, set the access TTL to 10 minutes when zero.
2. Set the refresh TTL to 2 hours when zero.
3. Re-run the focused config test and the complete `go test ./internal/config` suite.

### Task 3: Declare development deployment values

**Files:**
- Modify: `deployments/docker-compose/docker-compose.dev.yml`

1. Add `AUTH_ACCESS_TOKEN_TTL: 10m` and `AUTH_REFRESH_TOKEN_TTL: 2h` to API.
2. Add the same values to worker so both backend processes receive a consistent authentication configuration contract.
3. Render the compose configuration and verify both services contain the expected values.

### Task 4: Verify and exercise the runtime

**Files:**
- No production file changes.

1. Run `./scripts/workflow/verify.sh`.
2. Commit the scoped implementation.
3. Run `./scripts/workflow/review-local.sh --scope committed` and `./scripts/workflow/check-review-gate.sh`.
4. Rebuild and recreate API and worker from the development compose file.
5. Run the repository API smoke test.
6. Log in through the real password endpoint, assert the access lifetime is positive, and use the returned access token to fetch `/api/agent/user/v1/profile` successfully.
