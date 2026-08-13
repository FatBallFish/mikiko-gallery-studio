# Generated Image Thumbnail Repair Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure newly generated images create thumbnails and gradually repair existing generated-image assets that skipped media processing.

**Architecture:** Image-result dual writes create a `ready_original` asset and a unique pending probe job in one transaction. The cleanup reconciliation loop repairs one older generated image per iteration when required derivatives are missing, while thumbnail access retains authenticated original-image fallback until processing completes.

**Tech Stack:** Go, Ent, PostgreSQL/SQLite tests, existing media Worker and FFmpeg pipeline.

---

### Task 1: Make generated-image dual write enqueue media processing

**Files:**
- Modify: `internal/repository/entstore/imagetask_store.go`
- Test: `internal/repository/entstore/imagetask_store_test.go`

1. Extend the existing dual-write test to require `ready_original` and exactly one pending probe job.
2. Run the focused test and confirm it fails because the asset is `ready` and no job exists.
3. Add an idempotent transaction helper that creates the probe job only when absent.
4. Keep replayed assets at their current status so a completed asset is not reprocessed.
5. When a remote/placeholder result is replayed with a persisted object, atomically transition and enqueue it without downgrading processed assets.
6. Run focused tests and confirm pass.

### Task 2: Repair already-created generated images incrementally

**Files:**
- Modify: `internal/repository/entstore/media_worker_store.go`
- Test: `internal/repository/entstore/media_reconciler_test.go`

1. Add failing tests for a `ready/generated/image` asset with no derivatives and no job, idempotent second execution, complete derivatives, and non-generated assets.
2. Run focused tests and confirm the missing repair behavior fails.
3. Extend reconciliation to select at most one eligible asset with row locking, create a missing job, or reset an incomplete terminal job.
4. Resolve the same runtime media policy used by the Worker and build completeness predicates from that policy.
5. Preserve existing handling for `processing` and `ready_original` assets.
6. Run focused tests and confirm pass.

### Task 3: Verify access fallback and end-to-end repository behavior

**Files:**
- Modify: `internal/service/mediaasset/access_test.go` only if naming/coverage needs clarification.
- Test: `internal/service/mediaasset/access_test.go`

1. Confirm derivative priority and authenticated original fallback tests cover realtime and historical generated images.
2. Run mediaasset and entstore focused packages.
3. Run `./scripts/workflow/verify.sh`.
4. Run independent committed-scope review and `./scripts/workflow/ship-guard.sh` after commit.
5. Push the existing `codex/v019-admin-video-null-snapshot` branch only; do not create a PR, merge, tag, or deploy.
