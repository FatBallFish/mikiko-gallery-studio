# Plan, Public Asset, and Media URL Lifecycle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add safe plan lifecycle controls, reversible user publication, searchable admin public-image management, localized statuses, and direct short-lived object-storage media URLs.

**Architecture:** Reuse existing plan and visibility states, adding explicit transition APIs and server-side list filters. Centralize media URL projection in domain services using the existing storage router; signing-capable backends return five-minute URLs while legacy/local storage retains authenticated download endpoints.

**Tech Stack:** Go, Ent/PostgreSQL, React/TypeScript, existing storage router/S3 signer, repository contract tests.

---

### Task 1: Establish the new coding context

**Files:**
- Read: `docs/prd/2026-08-05-plan-public-media-lifecycle-requirements.md`
- Read: `docs/plans/2026-08-05-plan-public-media-lifecycle-design.md`
- Create: `.coding-context.json` through the workflow script

**Step 1: Start the heavyweight workflow**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "plan lifecycle, reversible public assets, admin public-image management, and direct signed media URLs"
```

Expected: `.coding-context.json` references the approved requirement and design documents.

**Step 2: Verify the context**

Run: `sed -n '1,240p' .coding-context.json`

Expected: both 2026-08-05 sources are present.

### Task 2: Add explicit plan state transitions and filtered persistence

**Files:**
- Modify: `internal/domain/billing/types.go`
- Modify: `internal/service/billing/store.go`
- Modify: `internal/service/billing/service.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Test: `internal/service/billing/service_test.go`
- Test: `internal/repository/entstore/billing_store_test.go`

**Step 1: Write failing tests**

Cover active/disabled/archived filtering, archive disabling purchase, restore targeting disabled, repeated transitions, and an order snapshot retaining its original points after the source plan is archived.

**Step 2: Run the red tests**

Run:

```bash
go test ./internal/service/billing ./internal/repository/entstore -run 'Plan(State|List|Archive|Restore|HistoricalOrder)' -count=1
```

Expected: FAIL because filtered list and transition operations are absent.

**Step 3: Implement the domain contract**

Add a list request and explicit target-state request, for example:

```go
type SubscriptionPlanListRequest struct {
    Status string
}

type TransitionSubscriptionPlanRequest struct {
    PlanID int64
    Action string
}
```

Validate only `enable`, `disable`, `archive`, and `restore`. Persist transitions atomically; restore sets `status=disabled` and `purchase_enabled=false`.

**Step 4: Run tests green**

Run the command from Step 2.

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/billing/types.go internal/service/billing internal/repository/entstore/billing_store.go internal/repository/entstore/billing_store_test.go
git commit -m "feat: add safe plan lifecycle transitions"
```

### Task 3: Expose plan lifecycle APIs and admin controls

**Files:**
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/admin_cashier_api_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/admin/src/pages/CashierPage.tsx`
- Modify: `web/admin/src/pages/cashierPlanPurchase.ts`
- Test: `web/admin/src/pages/cashierPlanPurchase.contract.ts`

**Step 1: Write failing router and frontend contracts**

Require `POST /api/ops/admin/v1/cashier/plans/{id}/enable|disable|archive|restore`, status-filtered list responses, permission enforcement, audit events, archived default hiding, and context-sensitive icon actions.

**Step 2: Run red tests**

```bash
go test ./internal/http/router -run 'AdminCashierPlan.*(Enable|Disable|Archive|Restore|Filter)' -count=1
npx tsx web/admin/src/pages/cashierPlanPurchase.contract.ts
```

Expected: FAIL.

**Step 3: Implement API and UI**

Use dedicated transition requests rather than full plan edits. Add active/disabled/deleted filters, confirmation dialogs, restore-only archived rows, and tooltips for icon buttons.

**Step 4: Run focused tests and builds**

```bash
go test ./internal/http/router -run 'AdminCashierPlan' -count=1
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/http/handlers/api.go internal/http/router/admin_cashier_api_test.go web/shared web/admin/src/pages
git commit -m "feat: manage plan availability from admin"
```

### Task 4: Add user publication cancellation

**Files:**
- Modify: `internal/service/imagetask/store.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Modify: `internal/domain/imagetask/types.go`
- Test: `internal/service/imagetask/service_test.go`
- Test: `internal/service/imagetask/store_internal_test.go`

**Step 1: Write failing transition tests**

Cover owner-only cancellation from pending and approved, idempotent private cancellation, removal from public/review queries, clearing publication metadata, and successful reapplication.

**Step 2: Verify failure**

```bash
go test ./internal/service/imagetask -run 'CancelPublish|ReapplyPublish' -count=1
```

Expected: FAIL because `CancelPublish` does not exist.

**Step 3: Implement atomically**

Add `CancelPublish(ctx, userID, imageID)` to service/store. Ent update predicates must bind image ownership and accepted source states before setting `private`, clearing review reason, and clearing `published_at`.

**Step 4: Run tests green and commit**

```bash
go test ./internal/service/imagetask -run 'Publish' -count=1
git add internal/domain/imagetask internal/service/imagetask internal/repository/entstore/imagetask_store.go
git commit -m "feat: allow users to cancel image publication"
```

### Task 5: Add user cancellation API and localized actions

**Files:**
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/image_api_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Modify: `web/user/src/pages/galleryRows.ts`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Test: `web/user/src/pages/galleryRows.contract.ts`

**Step 1: Write failing tests**

Require `DELETE` or an explicit cancel action on the existing publication resource, ownership enforcement, status refresh, Chinese labels for every alias, and action labels `申请公开/取消申请/取消公开/重新申请`.

**Step 2: Run red tests**

```bash
go test ./internal/http/router -run 'ImagePublish.*Cancel' -count=1
npx tsx web/user/src/pages/galleryRows.contract.ts
```

Expected: FAIL.

**Step 3: Implement API and UI**

Use one status view model for filter options, cards, batch eligibility, and detail actions. Confirm pending/public cancellation before mutation and refresh selected/list state after success.

**Step 4: Verify and commit**

```bash
go test ./internal/http/router -run 'ImagePublish' -count=1
npm --prefix web/user run typecheck
npm --prefix web/user run build
git add internal/http/handlers/api.go internal/http/router web/shared web/user/src/pages
git commit -m "feat: make image publication reversible"
```

### Task 6: Extend admin public-image query and moderation

**Files:**
- Modify: `internal/domain/imagetask/types.go`
- Modify: `internal/service/imagetask/store.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/repository/entstore/imagetask_store_test.go`
- Test: `internal/http/router/admin_image_api_test.go`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/admin/src/pages/ReviewPage.tsx`
- Test: `web/admin/src/pages/reviewRows.contract.ts`

**Step 1: Write repository and router red tests**

Test status tabs plus combined user ID/email/name, prompt, model, task type, size, aspect ratio, and time filters with correct filtered totals. Test unpublish reason, permission, audit, and disappearance from public gallery.

**Step 2: Run red tests**

```bash
go test ./internal/repository/entstore ./internal/http/router -run 'Admin.*(PublicImage|ReviewFilter|Unpublish)' -count=1
```

Expected: FAIL.

**Step 3: Implement database-backed filtering and UI tabs**

Extend `GalleryListRequest`; build Ent predicates and user joins before `Count` and pagination. Reuse the content-review page for all four status tabs and require an unpublish reason.

**Step 4: Verify and commit**

```bash
go test ./internal/repository/entstore ./internal/http/router -run 'Gallery|Review|PublicImage' -count=1
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
git add internal web/shared web/admin/src/pages
git commit -m "feat: manage public images from content review"
```

### Task 7: Centralize temporary media URL projection

**Files:**
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/assets/service.go`
- Modify: `internal/storage/backend.go`
- Test: `internal/service/imagetask/service_test.go`
- Test: `internal/service/assets/service_test.go`

**Step 1: Write failing projection tests**

Test five-minute preview URLs, separate download filename signing, local fallback routes, legacy storage routing, signing failures, and query sanitization.

**Step 2: Run red tests**

```bash
go test ./internal/service/imagetask ./internal/service/assets -run 'Temporary.*URL|Media.*Projection' -count=1
```

Expected: FAIL because signed delivery exists only inside download handling.

**Step 3: Implement shared service helpers**

Project URLs after ownership/visibility checks using the resource's storage configuration. Do not persist projected URLs. Keep fallback route construction centralized and distinguish preview from attachment options.

**Step 4: Run tests and commit**

```bash
go test ./internal/service/imagetask ./internal/service/assets -count=1
git add internal/service/imagetask internal/service/assets internal/storage
git commit -m "feat: project direct temporary media URLs"
```

### Task 8: Apply projection across every API surface

**Files:**
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Test: `internal/http/router/image_api_test.go`
- Test: `internal/http/router/public_gallery_api_test.go`
- Test: `internal/http/router/admin_image_api_test.go`
- Test: `internal/http/router/reference_asset_api_test.go`

**Step 1: Add a table-driven API coverage test**

Cover task detail/list/stream, workspace history, private gallery, home/public gallery, public detail, admin review, and reference assets. Assert S3 fixtures return absolute signed URLs and local fixtures return fallback routes.

**Step 2: Run red tests**

```bash
go test ./internal/http/router -run 'DirectMediaURL|ReferenceAssetURL' -count=1
```

Expected: FAIL on DTOs that overwrite service URLs with `/api/agent/image/v1/images/...`.

**Step 3: Remove DTO URL rebuilding and apply projections**

Keep authorization checks before signing. Do not sign deleted, private-for-other-user, or unpublished public resources.

**Step 4: Verify and commit**

```bash
go test ./internal/http/router -run 'Image|Gallery|ReferenceAsset' -count=1
git add internal/http/handlers/api.go internal/repository/entstore/imagetask_store.go internal/http/router
git commit -m "fix: return direct object storage media URLs"
```

### Task 9: Harden frontend URL handling and refresh behavior

**Files:**
- Modify: `web/shared/user-api.ts`
- Modify: `web/shared/http-client.ts`
- Modify: `web/user/src/pages/HomePage.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Modify: `web/admin/src/pages/ReviewPage.tsx`
- Test: `web/user/src/pages/homePublicAssets.contract.ts`
- Test: `web/user/src/pages/workspaceImageDetailWiring.contract.ts`
- Test: `web/user/src/pages/galleryRows.contract.ts`
- Test: `web/admin/src/pages/reviewRows.contract.ts`

**Step 1: Write failing URL contracts**

Assert absolute URLs are unchanged, relative fallback URLs receive credentials as before, and each media component can refetch once after a signed URL load failure without an infinite loop.

**Step 2: Run red contracts**

```bash
./scripts/workflow/verify-contracts.sh
```

Expected: FAIL on the new URL requirements.

**Step 3: Implement minimal shared URL classification**

Use `new URL`/scheme parsing rather than prefix string guesses. Keep refreshed signed URLs in component state only.

**Step 4: Verify and commit**

```bash
./scripts/workflow/verify-contracts.sh
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
git add web/shared web/user/src web/admin/src
git commit -m "fix: consume signed media URLs directly"
```

### Task 10: Complete audit, verification, and review gate

**Files:**
- Modify: `docs/runbooks/storage-configuration.md` or the matching existing storage runbook
- Create/Modify: media endpoint audit matrix under `docs/runbooks/`

**Step 1: Audit all media fields**

Run:

```bash
rg -n 'download_url|preview_url|image_url|/api/agent/image/v1/images|reference-assets/.*/download' internal web api
```

Classify every occurrence as direct projection, fallback endpoint, upload source, or test fixture. Document intentional fallbacks.

**Step 2: Run focused suites**

```bash
go test ./internal/service/billing ./internal/service/imagetask ./internal/service/assets ./internal/repository/entstore ./internal/http/router -count=1
./scripts/workflow/verify-contracts.sh
```

Expected: PASS.

**Step 3: Run repository verification and isolated smoke**

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
```

Expected: PASS.

**Step 4: Commit documentation and generate review marker**

```bash
git add docs/runbooks
git commit -m "docs: document media delivery and lifecycle controls"
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: review gate `PASS` and `OK`.
