# v0.0.17 Setup Binding Runtime Defaults Hotfix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Safely reconcile v0.0.12 Setup bindings after v0.0.16 added defaulted Worker/media runtime fields, then release v0.0.17 and complete the production upgrade.

**Architecture:** Add one fixed, fail-closed historical digest profile for the 14 Worker/media fields introduced after v0.0.12. Reuse the existing identity checks, constant-time candidate matching, database CAS, local completed-state update, rollback, and idempotent database migration path.

**Tech Stack:** Go, HMAC-SHA256 Setup binding, PostgreSQL/Ent, Docker Compose, GitHub Actions, repository workflow scripts.

---

### Task 1: Establish the v0.0.17 coding context

**Files:**
- Create: `docs/prd/2026-08-13-v017-setup-binding-runtime-defaults-hotfix-requirements.md`
- Create: `docs/tech/2026-08-13-v017-setup-binding-runtime-defaults-hotfix-tech-design.md`
- Create: `docs/plans/2026-08-13-v017-setup-binding-runtime-defaults-hotfix.md`

1. Record the production failure, exact new-field set/defaults, fail-closed constraints, release and recovery steps.
2. Run `./scripts/workflow/start-coding.sh --task "fix v0.0.17 setup binding compatibility for v0.0.12 runtime defaults" --track lightweight`.
3. Confirm `.coding-context.json` points to the new requirement and technical design.
4. Commit the documents.

### Task 2: Add the failing historical-profile tests

**Files:**
- Modify: `internal/setup/legacy_binding_reconcile_test.go`

1. Build a current runtime fixture and remove the 14 audited fields to derive a v0.0.12 canonical digest.
2. Assert reconciliation migrates both database binding and local completed state to current canonical.
3. Add the equivalent release-field legacy case using the previous v0.0.12 release identity.
4. Run `go test ./internal/setup -run 'V012RuntimeDefaults' -count=1` and confirm RED with `ErrSetupBindingMismatch`.
5. Add table-driven fail-closed tests for each non-default value and one additional omitted non-profile field.

### Task 3: Implement the fixed compatibility profile

**Files:**
- Modify: `internal/setup/legacy_binding_reconcile.go`

1. Define the fixed map of 14 field names to audited v0.0.16 defaults.
2. Add a helper that enables the profile only when every key exists and exactly matches its fixed value.
3. Append canonical and release-field legacy candidates computed with only that fixed field set omitted.
4. Do not change identity validation, divergent-history rejection, CAS, rollback or idempotency logic.
5. Run the RED test and all `TestReconcileLegacyCompletedBinding` tests; confirm GREEN.
6. Commit the implementation and tests.

### Task 4: Verify and review

1. Run `go test ./internal/setup ./internal/app ./internal/mgsctl -count=1`.
2. Run `./scripts/workflow/verify.sh`.
3. Run `./scripts/workflow/api-smoke.sh`.
4. Review the committed diff for overbroad field omission, mutable defaults, digest divergence, CAS rollback and production retry safety.
5. Commit any review fix separately.
6. Run `./scripts/workflow/review-local.sh --scope committed` and `./scripts/workflow/check-review-gate.sh`.
7. Run `./scripts/workflow/ship-guard.sh`.

### Task 5: Merge and publish v0.0.17

1. Push only `codex/v017-setup-binding-runtime-defaults`.
2. Create a ready PR to main and wait for required checks.
3. Merge only when GitHub reports the PR mergeable and checks pass.
4. Fetch main, confirm the compatibility commit is an ancestor, and tag the merge commit `v0.0.17`.
5. Wait for Tagged Release, release assets/checksums, manifest, all Docker images and `promote-latest` to succeed.

### Task 6: Retry and validate production upgrade

1. Recheck v0.0.12 containers and verify the existing backup SHA256 plus `pg_restore --list`.
2. Update mgsctl to v0.0.17 and confirm build metadata.
3. Run explicit v0.0.17 application upgrade without manual SQL.
4. Verify every application image and health status, `mgsctl status`, `mgsctl doctor`, `/healthz` and `/readyz`.
5. Verify `installations.app_version=v0.0.17`, database schema 5, and all three `model_accounts.public_id` values are non-null and distinct with final constraints.
6. Mark the persistent goal complete only after all production checks pass.
