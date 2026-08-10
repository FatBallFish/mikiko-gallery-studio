# v0.0.11 Setup Binding Compatibility Hotfix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restore safe, idempotent mgsctl upgrades for installations whose completed Setup binding predates the v0.0.10 documentation runtime fields.

**Architecture:** Extend Setup binding reconciliation with one allowlisted historical schema profile. Reuse the current canonical digest, durable identity checks, database CAS update, local state update, and rollback path; do not add a general field-omission mechanism.

**Tech Stack:** Go, PostgreSQL/Ent Setup store, mgsctl Docker upgrade flow, repository workflow scripts.

---

### Task 1: Establish the failing compatibility contract

**Files:**
- Modify: `internal/setup/legacy_binding_reconcile_test.go`

**Step 1: Write the failing test**

Add a test that computes a stored canonical digest after removing
`PIC_GALLERY_DOCS_URL` and `PIC_GALLERY_DOCS_PROBE_URL`, then supplies current
default values in the upgrade bootstrap. Assert that reconciliation succeeds and
updates both binding stores to the current canonical digest.

**Step 2: Verify RED**

Run:

```bash
go test ./internal/setup -run TestReconcileLegacyCompletedBindingAcceptsPreDocumentationSchema -count=1
```

Expected: fail with `setup binding does not match the requested commit`.

### Task 2: Implement the allowlisted historical profile

**Files:**
- Modify: `internal/setup/service.go`
- Modify: `internal/setup/legacy_binding_reconcile.go`
- Modify: `internal/setup/legacy_binding_reconcile_test.go`

**Step 1: Add digest helper support**

Refactor the internal digest helper so a fixed set of compatibility fields can
be omitted without changing current callers.

**Step 2: Add the historical candidate**

Generate pre-documentation canonical and legacy candidates only when the current
documentation fields equal their renderer defaults.

**Step 3: Generalize reconciliation classification**

Accept current or allowlisted previous candidates, reject divergent non-current
state/database digests, and preserve CAS rollback using the exact previous
database digest.

**Step 4: Verify GREEN**

Run:

```bash
go test ./internal/setup -run 'TestReconcileLegacyCompletedBinding' -count=1
```

Expected: pass.

### Task 3: Cover fail-closed and retry behavior

**Files:**
- Modify: `internal/setup/legacy_binding_reconcile_test.go`
- Modify if required: `internal/app/migrate_test.go`

**Step 1: Add fail-closed tests**

Cover non-default documentation values and divergent historical database/local
digests.

**Step 2: Add partial-migration retry coverage**

Prove a database migration that is already at the target version can rerun and
complete binding reconciliation without additional mutation assumptions.

**Step 3: Run focused suites**

```bash
go test ./internal/setup ./internal/app ./internal/mgsctl -count=1
```

Expected: pass.

### Task 4: Verify, review, and exercise deployment paths

**Files:**
- Update only if a verified defect is found.

**Step 1: Run full verification**

```bash
./scripts/workflow/verify.sh
```

**Step 2: Run Docker upgrade E2E and API smoke**

```bash
./scripts/e2e/mgsctl-upgrade-docker-e2e.sh
./scripts/workflow/api-smoke.sh
```

**Step 3: Run local review**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

### Task 5: Ship the hotfix release

**Files:**
- No additional source changes expected.

**Step 1: Create and merge a PR targeting `main`**

Confirm required checks and review pass before merge.

**Step 2: Create the next annotated SemVer patch tag**

Tag the merge commit without moving or modifying an existing tag.

**Step 3: Verify tagged release output**

Wait for the release workflow, then verify every adjacent checksum, the release
manifest, five immutable multi-architecture image digests, OCI labels, and each
`latest` tag.
