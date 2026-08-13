# v0.0.16 Upgrade Reliability Hotfix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make legacy model-account schema upgrades safe and make mgsctl self-update resilient and observable, then release and validate v0.0.16 in production.

**Architecture:** Extend the existing pre-Ent PostgreSQL compatibility phase to create and backfill `model_accounts.public_id` before Ent applies `UNIQUE NOT NULL`. Keep self-update checksum verification and atomic replacement, but use a transport with bounded connection/header/idle waits, retry only pre-commit download failures, and report byte progress through an injected callback wired to terminal-aware CLI output.

**Tech Stack:** Go, Ent, PostgreSQL, `net/http`, GitHub Releases, Docker Compose, repository workflow scripts.

---

### Task 1: Establish coding context

**Files:**
- Requirement: `docs/prd/2026-08-13-v016-upgrade-reliability-hotfix-requirements.md`
- Design/plan: `docs/plans/2026-08-13-v016-upgrade-reliability-hotfix.md`

1. Run `./scripts/workflow/start-coding.sh --task "fix v0.0.16 legacy public_id migration and mgsctl self-update reliability" --track lightweight`.
2. Confirm `.coding-context.json` names both documents and records the current branch.
3. Commit the requirement, plan, and coding context.

### Task 2: Reproduce and fix legacy public ID migration

**Files:**
- Modify: `internal/repository/db/legacy_migrations.go`
- Modify: `internal/repository/db/legacy_migrations_test.go`

1. Add a PostgreSQL integration test that creates the v0.0.12 `model_accounts` shape with three rows and proves the current full migration fails or the compatibility phase leaves no usable public IDs.
2. Add another fixture with nullable `public_id`, one existing UUID and remaining null rows.
3. Run the focused tests with `PIC_GALLERY_TEST_POSTGRES_URL` and confirm they fail before implementation.
4. Extend `prepareLegacyDataSQL` under its existing advisory transaction lock:
   - add `public_id uuid` only when the table exists and the column is absent;
   - fill only null rows with independently generated UUID values;
   - avoid PostgreSQL extension dependencies by generating UUID text from `md5(random() || clock_timestamp() || id)` and casting to UUID;
   - never alter an existing non-null value;
   - leave `UNIQUE NOT NULL` enforcement to Ent schema creation.
5. Run compatibility preparation twice and full migration once in tests. Assert non-null uniqueness, preservation, idempotency, and schema constraints.
6. Run `go test ./internal/repository/db -run 'PublicID|Legacy' -count=1`.
7. Commit the migration fix.

### Task 3: Reproduce self-update timeout and progress gaps

**Files:**
- Modify: `internal/mgsctl/self_update_test.go`
- Modify: `internal/mgsctl/cli_test.go`

1. Add a slow streaming HTTP server test whose total transfer exceeds a short client timeout while bytes continue to arrive.
2. Add a server that fails an initial checksum/binary request before succeeding, and assert bounded retry behavior.
3. Add callback assertions for known and unknown content lengths and a CLI test proving progress appears only for terminal output.
4. Assert errors identify `checksum` versus `binary`, include attempts, preserve `context.Canceled`, and do not leak signed URL queries.
5. Run focused tests and confirm failure before implementation.

### Task 4: Implement resilient self-update downloads

**Files:**
- Modify: `internal/mgsctl/self_update.go`
- Modify: `internal/mgsctl/cli.go`
- Modify as needed: `internal/mgsctl/terminal.go`

1. Add injected download progress and retry-notice callbacks to `SelfUpdateDependencies` without changing CLI flags.
2. Replace `http.Client.Timeout` with a transport that bounds dial, TLS handshake and response headers while allowing a progressing body transfer to exceed two minutes.
3. Retry checksum and binary downloads independently up to three attempts for transient transport errors and retryable HTTP statuses. Truncate and rewind the staged file before a binary retry.
4. Stream through a counting reader/writer that emits throttled byte progress; include total bytes when `Content-Length` is valid.
5. Preserve SHA-256 size limits, checksum validation, cancellation, atomic replacement, and sensitive-query redaction.
6. Wire TTY progress as an updating stderr line with percentage, human-readable bytes and rate; finish with a newline. Keep non-TTY output free of progress churn.
7. Improve unavailable errors with stage, attempt count and sanitized root cause.
8. Run `go test ./internal/mgsctl -run 'SelfUpdate|self.update' -count=1`.
9. Commit the self-update fix.

### Task 5: Document operator behavior

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/runbooks/backend-deployment.md`

1. Document retry/progress behavior and explain that a progressing download may exceed two minutes.
2. Document migration failure recovery: old services remain active before rollout; operators must not hand-edit `public_id` for v0.0.16+.
3. Run documentation contracts.
4. Commit documentation.

### Task 6: Verify and review

1. Run focused PostgreSQL integration tests against an isolated PostgreSQL 16 container.
2. Run `./scripts/workflow/verify.sh`.
3. Run `./scripts/workflow/api-smoke.sh` because backend, migration and deployment behavior changed.
4. Review the complete committed diff for data loss, retry safety, descriptor leaks, URL leakage and cancellation semantics.
5. Commit any review fixes separately.
6. Run `./scripts/workflow/review-local.sh --scope committed` and `./scripts/workflow/check-review-gate.sh`.
7. Run `./scripts/workflow/ship-guard.sh` or the repository `dev-ship` equivalent.

### Task 7: Merge and release v0.0.16

1. Push only `codex/v016-migration-self-update`.
2. Create a PR to `main`, wait for checks, and merge after GitHub reports it mergeable.
3. Fetch `origin/main`, confirm both fixes are ancestors, and create annotated tag `v0.0.16` on the merge commit.
4. Push the tag and wait for the tagged Release workflow to complete successfully.
5. Verify the GitHub Release, manifest/checksum assets, all image jobs, and `promote-latest` success.

### Task 8: Back up and upgrade production

1. Reconfirm v0.0.12 services are healthy and capture the current deployment manifest.
2. Create a timestamped `pg_dump -Fc` backup outside the container in a dedicated production backup directory; record SHA-256 and validate with `pg_restore --list`.
3. Update mgsctl to v0.0.16 and confirm its build metadata.
4. Run the normal explicit `mgsctl upgrade` path with migration; do not execute ad hoc schema SQL.
5. Verify all expected containers use v0.0.16 image digests and become healthy.
6. Query `model_accounts` to confirm three non-null distinct public IDs and query `installations` for v0.0.16/current schema version.
7. Run `mgsctl status`, `mgsctl doctor`, and HTTP health probes.
8. If migration fails before rollout, confirm v0.0.12 remains healthy and retain the database backup and failure logs. Do not force a partial rollout.
