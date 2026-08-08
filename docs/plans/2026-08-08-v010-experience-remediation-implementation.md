# v0.0.10 Experience Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement every accepted v0.0.9 experience-remediation requirement, including optional fixed-package expiry, and deliver a reviewed v0.0.10 release from `main`.

**Architecture:** Add backward-compatible Ent schema fields and services first, then move behavior behind server-authoritative domain validators and ownership checks. User/admin React clients consume shared contracts, while durable workers handle storage cleanup and large exports. Existing image fan-out and edits behavior remain intact.

**Tech Stack:** Go, Ent, PostgreSQL, Redis, React, TypeScript, Vite, S3-compatible object storage, GitHub Actions.

---

## Execution Rules

- Follow `dev-go-patterns` before Go changes and `dev-react-patterns` before frontend changes.
- Use red-green-refactor for every behavior change. Generated Ent code is regenerated after schema tests/edits and is exempt from hand-written test-first ordering.
- Keep commits scoped to the task named below.
- Do not implement stream or partial images.
- Do not cap platform task output count at 10 or replace the existing fan-out planner.
- Do not restore `/docs` or add a redirect.
- Do not tag a feature-branch commit. Merge the reviewed PR first, update local `main`, then create the next SemVer tag on that exact main commit.

### Task 1: Schema And Contract Foundations

**Files:**
- Modify: `internal/repository/ent/schema/subscriptionplan.go`
- Modify: `internal/repository/ent/schema/paymentorder.go`
- Modify: `internal/repository/ent/schema/modelaccountmodel.go`
- Modify: `internal/repository/ent/schema/imagetask.go`
- Modify: `internal/repository/ent/schema/imageresult.go`
- Modify: `internal/repository/ent/schema/referenceasset.go`
- Create: `internal/repository/ent/schema/project.go`
- Create: `internal/repository/ent/schema/objectdeletionjob.go`
- Modify: `internal/repository/ent/schema/schema_test.go`
- Modify: `internal/domain/billing/types.go`
- Modify: `internal/domain/modeladmin/types.go`
- Modify: `internal/domain/assets/types.go`
- Modify: `web/shared/api-types.ts`
- Regenerate: `internal/repository/ent/**`

**Step 1: Write failing schema and JSON contract tests**

Assert the following wished-for fields and defaults:

- plan `credit_expiry_enabled=true`;
- order `credit_expiry_enabled`, nullable `credit_valid_days`, `credited_at`, and `credit_expires_at`;
- account-model size/background/pixel-limit capability fields and `max_image_count` range contract;
- projects, task/result `project_id`, reference alias ownership/source, and cleanup jobs.

**Step 2: Run tests and verify RED**

Run: `go test ./internal/repository/ent/schema ./internal/domain/billing ./internal/domain/modelhub`

Expected: FAIL because the new fields/entities/contracts do not exist.

**Step 3: Add Ent schemas and domain/shared API types**

Use nullable-first fields for backfill-sensitive project/order relationships. Preserve current database table annotations such as `task_images`.

**Step 4: Regenerate Ent and verify GREEN**

Run the repository's existing Ent generation command discovered from `go:generate`/workflow files, then:

`go test ./internal/repository/ent/schema ./internal/domain/billing ./internal/domain/modelhub`

**Step 5: Commit**

Commit message: `feat: add remediation data contracts`

### Task 2: Fixed-Package Expiry, Orders, Wallet, And Ledger

**Files:**
- Modify: `internal/service/billing/service_test.go`
- Modify: `internal/service/billing/store_paid_test.go`
- Modify: `internal/repository/entstore/billing_store_test.go`
- Modify: `internal/repository/entstore/cashier_store_test.go`
- Modify: `internal/http/router/admin_cashier_api_test.go`
- Modify: `internal/service/billing/service.go`
- Modify: `internal/service/billing/store.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Modify: `internal/repository/entstore/cashier_store.go`
- Modify: billing handlers in `internal/http/handlers/api.go`
- Modify: `web/admin/src/pages/cashierPlanDraft.contract.ts`
- Modify: `web/admin/src/pages/cashierPlanPurchase.contract.ts`
- Modify: `web/admin/src/pages/CashierPlanEditorDialog.tsx`
- Modify: `web/admin/src/pages/cashierPlanDraft.ts`
- Modify: `web/admin/src/pages/cashierPlanPurchase.ts`
- Modify: `web/user/src/pages/checkoutRecentOrders.contract.ts`
- Modify: `web/user/src/pages/profileBalanceBuckets.contract.ts`
- Modify: `web/user/src/pages/profileLedgerRows.contract.ts`
- Modify: `web/user/src/pages/ProfilePage.tsx`
- Modify: `web/user/src/pages/profileBalanceModel.ts`

**Step 1: Write failing backend tests**

Cover expiring and permanent fixed packages, base/gift grant separation, order snapshot immutability after plan edits, custom recharge permanence, idempotent completion, partial-success unit-price projection, and next-expiry aggregation.

**Step 2: Verify RED**

Run: `go test ./internal/service/billing ./internal/repository/entstore ./internal/http/router -run 'Plan|Order|Grant|Ledger|Balance|Cashier'`

**Step 3: Implement the billing transaction and API projections**

Validate days only when enabled. Snapshot the flag/days/amounts at order creation. Completion must use snapshots and create unique purchased/gift grants with identical nullable expiry. Remove editable currency writes and keep persisted CNY snapshots.

**Step 4: Verify backend GREEN**

Repeat the focused Go tests.

**Step 5: Write failing frontend contracts and implement UI**

Add an accessible expiry toggle, conditionally render days, default old/new drafts to enabled, remove currency input, and show base/gift/expiry/permanent order and wallet/ledger details.

**Step 6: Verify frontend GREEN**

Run the touched `tsx` contract files, then admin/user typecheck.

**Step 7: Commit**

Commit message: `feat: make package credit expiry configurable`

### Task 3: Documentation, Compliance, Prompt Layout, And History Overview

**Files:**
- Modify: user router entry files under `web/user/src`
- Modify: `web/user/src/docsUrl.contract.ts`
- Delete: obsolete `/docs` page/model/contract files after route references are removed
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/components.contract.ts`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/workspacePage.contract.ts`
- Modify: `web/user/src/pages/workspaceTaskHistory.contract.ts`
- Modify: admin system-settings types/pages/contracts
- Modify: readiness logic/tests in `internal/http/handlers/api.go` and `internal/http/router/admin_readiness_api_test.go`

**Step 1: Write failing contracts**

Assert no ICP string, no `/docs` route, direct resolved docs navigation, no obsolete documentation title/base-path controls, full-width prompt text with bottom-only action clearance, and overview-before-detail for multi-result tasks.

**Step 2: Verify RED**

Run the focused user/admin contract tests and `go test ./internal/http/router -run Readiness`.

**Step 3: Implement minimal UI/readiness changes**

Reuse `docsUrl.ts`, remove the compatibility route, rely on effective OpenAPI/docs readiness, and reuse the shared image-detail component from a task overview.

**Step 4: Verify GREEN and commit**

Commit message: `fix: align docs and workspace experience`

### Task 4: Image Size, Background, And Provider Contract

**Files:**
- Modify: `internal/domain/modelhub/capability.go`
- Modify: `internal/domain/modelhub/resolver.go`
- Create or modify: shared image request validator under `internal/domain/imagetask`
- Modify: `internal/service/capabilities`
- Modify: `internal/service/imagetask/service.go`
- Modify: estimate/create handlers in `internal/http/handlers/api.go`
- Modify: `internal/provider/openai/client.go`
- Modify: `internal/provider/openai/client_test.go`
- Modify: `internal/service/imagetask/service_test.go`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/providerModelCapabilities.contract.ts`
- Modify: `web/admin/src/pages/configRows.ts`
- Modify: `web/user/src/pages/workspaceCreationDraft.ts`
- Modify: workspace parameter/estimate/readiness contracts and `WorkspacePage.tsx`
- Modify: `web/shared/image-size.ts` and its contract

**Step 1: Write failing table-driven tests**

Cover auto omission, ratio resolution/16-grid behavior, custom-ratio bounds, exact pixel rejection, configured min/max bounds, unsupported option filtering, transparent PNG/WebP validation, absent GPT Image `response_format`, and per-model max `n` of 1-10.

**Step 2: Verify RED**

Run focused domain/service/provider tests.

**Step 3: Implement one server-authoritative normalizer**

Estimate, create, worker, and provider construction must agree. Explicit pixels are never rounded. Ratio-derived dimensions may be rounded to legal grid values. Persist requested versus actual dimensions and mismatch diagnostics.

**Step 4: Verify backend GREEN**

Run focused tests, including a regression where platform output count exceeds candidate max `n` and still fans out using existing logic.

**Step 5: Implement admin/user capability controls test-first**

Move auto to size mode, remove base-resolution auto, add custom-ratio switch, pixel bounds, background options, and conditional controls. Do not add stream/partial-image fields or rewrite edits.

**Step 6: Verify frontend GREEN and commit**

Commit message: `feat: enforce image generation field contracts`

### Task 5: Projects And Global Selection

**Files:**
- Create: `internal/domain/project/types.go`
- Create: `internal/service/project/service.go`
- Create: `internal/service/project/service_test.go`
- Create: `internal/repository/entstore/project_store.go`
- Create: `internal/repository/entstore/project_store_test.go`
- Modify: `internal/http/router/router.go`
- Modify: project/task/gallery handlers and stores
- Add: bounded migration/backfill command or repository migration step and tests
- Create: `web/user/src/pages/ProjectsPage.tsx`
- Create: shared user project store/hook and contracts
- Modify: user navigation, `WorkspacePage.tsx`, and `GalleryPage.tsx`

**Step 1: Write failing service/repository/API tests**

Cover one default per user, idempotent ensure, ownership isolation, omitted-project fallback, immutable default, rename uniqueness, empty delete, atomic populated transfer, stale version, and bounded backfill.

**Step 2: Verify RED**

Run focused project, router, task, and gallery tests.

**Step 3: Implement backend and migration**

Use `/api/agent/project/v1/projects`, nullable-first backfill, stable lock ordering, and audit events. Task/result writes always resolve a valid same-user project.

**Step 4: Verify backend GREEN**

Repeat focused tests against memory and Ent stores where both exist.

**Step 5: Write failing frontend contracts and implement**

Add project navigation/management, user-scoped browser memory, same-tab/cross-tab synchronization, deleted/foreign fallback, and shared workspace/gallery selection.

**Step 6: Verify frontend GREEN and commit**

Commit message: `feat: add project-scoped asset ownership`

### Task 6: No-Copy References And Durable Object Cleanup

**Files:**
- Modify: `internal/service/assets/gallery_import.go`
- Modify: `internal/service/assets/service_test.go`
- Modify: `internal/repository/entstore/assets_store.go`
- Modify: `internal/service/imagetask/service.go`
- Create: cleanup service/store/worker files and tests
- Modify: worker/app dependency wiring
- Replace: `internal/http/router/gallery_import_copy_api_test.go` with alias behavior tests
- Modify: `web/user/src/pages/workspaceGalleryImport.contract.ts`
- Modify: `web/user/src/pages/imageConfigurationReuse.contract.ts`

**Step 1: Write failing storage-spy and race tests**

Assert a gallery import performs zero Copy/Put, aliases survive source business deletion, import/delete races cannot lose live objects, last-reference deletion enqueues once, not-found is success, failures retry across worker restart, and legacy copies retain ownership.

**Step 2: Verify RED**

Run: `go test ./internal/service/assets ./internal/service/imagetask ./internal/repository/entstore ./internal/worker -run 'Import|Alias|Delete|Cleanup'`

**Step 3: Implement alias and outbox lifecycle**

Soft-delete business records and enqueue canonical object identities transactionally. Cleanup rechecks generated, alias, public, and recovery references immediately before physical deletion.

**Step 4: Implement first-reference parameter reuse test-first**

Return a source task parameter snapshot; apply it only when the reference list was empty and revalidate it against current effective capability.

**Step 5: Verify GREEN and commit**

Commit message: `feat: reuse media without storage copies`

### Task 7: Gallery Batch Operations And ZIP Export

**Files:**
- Modify: gallery routes/handlers/stores and tests
- Create: ZIP/export service and tests
- Modify: cleanup integration for temporary archives
- Modify: `web/user/src/pages/galleryBatchActions.ts`
- Modify: `web/user/src/pages/galleryBatchActions.contract.ts`
- Modify: `web/user/src/pages/GalleryPage.tsx`

**Step 1: Write failing API/service tests**

Cover explicit selected IDs, select/invert semantics, per-item results, batch publish/group/delete/project transfer, one authorized ZIP, duplicate/path-safe filenames, manifest errors, thresholds, and cross-tenant rejection.

**Step 2: Verify RED and implement backend**

Use the `images:batch-*` routes from the technical design and bounded direct ZIP with async promotion for large exports.

**Step 3: Verify backend GREEN**

Run focused gallery/export/router tests.

**Step 4: Write failing UI contracts and implement toolbar**

Make selection discoverable and stable; preserve failed selections after partial mutation results.

**Step 5: Verify frontend GREEN and commit**

Commit message: `feat: complete gallery batch workflows`

### Task 8: Administration Lifecycle Actions

**Files:**
- Modify: model-account/model/route/candidate/pricing admin pages and contracts
- Modify: `web/admin/src/pages/CashierPage.tsx` where plan lifecycle discoverability needs correction
- Modify: `internal/service/modeladmin` and tests only where existing endpoints lack dependency/audit guarantees
- Modify: admin router tests

**Step 1: Write failing contracts/API tests**

Cover visible delete buttons, confirmation, dependency conflicts, plan quick enable/disable/archive/restore, account/model/route soft deletion, candidate/price deletion safety, and historical snapshot display.

**Step 2: Verify RED, implement, and verify GREEN**

Reuse existing DELETE endpoints. Add backend work only for missing invariants/audit/history fallback.

**Step 3: Commit**

Commit message: `feat: complete admin lifecycle controls`

### Task 9: Cluster, Readiness, And Real Call Distribution

**Files:**
- Modify: `internal/app/cluster_heartbeat.go`
- Modify: `internal/service/cluster` and tests
- Modify: `internal/repository/entstore/cluster_store.go`
- Modify: readiness and dashboard handlers/tests in `internal/http/handlers/api.go` and `internal/http/router`
- Modify: call-record service/store queries and tests
- Modify: `web/admin/src/pages/OverviewPage.tsx`
- Modify: `web/admin/src/pages/clusterRows.contract.ts`
- Modify: readiness/overview row contracts

**Step 1: Write failing tests**

Assert full mode exposes exactly one stable logical node, distributed mode still uses heartbeat, payment readiness matches checkout eligibility, docs readiness ignores removed config, and model distribution counts actual call records for the selected window with reconciling totals.

**Step 2: Verify RED, implement, and verify GREEN**

Do not label provider health weights as call distribution. Keep preflight-without-call counts separate.

**Step 3: Commit**

Commit message: `fix: report real operational state`

### Task 10: End-To-End Verification And Visual QA

**Files:**
- Modify/add: API smoke fixtures and browser scripts only when required to cover the accepted workflows
- Update: requirement/technical docs if implementation reveals a justified contract correction

**Step 1: Run focused integration scenarios**

Exercise package expiry on/off, order snapshots, auto/ratio/pixel/background requests, output fan-out above max `n`, alias lifecycle, project migration/transfer, ZIP, cluster, and call distribution.

**Step 2: Run full repository verification**

Run: `./scripts/workflow/verify.sh`

Expected: PASS.

**Step 3: Run isolated API smoke**

Run: `./scripts/workflow/api-smoke.sh`

Expected: PASS with its temporary PostgreSQL, Redis, API, worker, and fake provider cleaned up.

**Step 4: Run browser QA**

Verify user/admin desktop and mobile layouts, prompt control overlap, project switching, history overview, batch toolbar, permanent-expiry toggle, and size/background controls. Capture screenshots/evidence as required by repository practice.

**Step 5: Commit verification changes**

Commit message: `test: cover v010 remediation workflows`

### Task 11: Review, PR, Merge, And Tagged Release

**Files:**
- Update: release notes/changelog files required by `.github/workflows/release.yml`
- Generated: `.review/gate.json`

**Step 1: Inspect complete diff against `origin/main`**

Perform a requirement-by-requirement completion audit and a security/concurrency/migration review. Fix findings test-first.

**Step 2: Run ship guard**

Run: `./scripts/workflow/ship-guard.sh`

Expected: verify, committed-scope review gate, stale-gate check, and isolated API smoke all PASS.

**Step 3: Push and create PR**

Push `codex/v010-experience-remediation`, create a non-draft PR targeting `main`, and include acceptance evidence and rollout/rollback notes.

**Step 4: Wait for required PR checks and merge**

Do not bypass protection. Merge only after required checks succeed, then verify the merge commit is on `origin/main`.

**Step 5: Create and push release tag**

Resolve the latest SemVer tag and require the next tag to be `v0.0.10` unless repository state has advanced. Annotate the exact merged `main` commit and push the tag.

**Step 6: Verify GitHub Release Action**

Wait for `.github/workflows/release.yml` to complete successfully. Verify the GitHub Release, release manifest, checksums, native packages, and expected container image metadata/artifacts.

