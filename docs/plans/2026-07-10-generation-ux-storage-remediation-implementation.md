# Generation UX And Storage Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make creative-task feedback truthful, expose model-driven output controls, repair light-theme and asset-gallery details, and make default-storage activation complete its required persisted probe.

**Architecture:** Persist coarse image-task stages behind lease ownership and treat them as the source of truth for the SSE workspace rail. Extend real-model capability metadata with a boolean compression-support flag, aggregate it through route capabilities, and keep request compression numeric and combination-aware. Keep the storage service safety gate intact while the admin UI orchestrates persisted probe then activation.

**Tech Stack:** Go, Ent, PostgreSQL, React 19, TypeScript, Tailwind utility classes, SSE polling, repository contract scripts, Docker Compose.

**Design source:** `docs/plans/2026-07-10-generation-ux-storage-remediation-design.md`

**Working-tree note:** The current frontend/backend baseline contains extensive approved uncommitted changes that overlap this work. Do not stage or commit production files until those existing changes are separated by their owner. Each task still follows red-green-refactor and records exact verification output.

---

### Task 1: Persist lease-owned task progress

**Files:**
- Modify: `internal/domain/imagetask/types.go`
- Modify: `internal/service/imagetask/store.go`
- Modify: `internal/repository/ent/schema/imagetask.go`
- Modify: `internal/repository/ent/migrations/000001_init.sql`
- Modify/generated: `internal/repository/ent/imagetask*.go`, `internal/repository/ent/mutation.go`, `internal/repository/ent/runtime.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Test: `internal/repository/entstore/imagetask_store_test.go`
- Test: `internal/service/imagetask/store_internal_test.go`

**Step 1: Write failing persistence tests**

Add assertions that a task round-trips `progress_stage` and `progress_message`. Add a store test for:

```go
updated, err := store.UpdateProgressIfOwned(ctx, task.ID, owner, "provider", "正在调用模型生成图片", now)
```

Assert a valid lease owner succeeds without changing results or lease expiry, while a stale owner receives `repoerr.ErrConflict`.

**Step 2: Verify red**

Run:

```bash
go test ./internal/repository/entstore ./internal/service/imagetask
```

Expected: FAIL because progress columns and `UpdateProgressIfOwned` do not exist.

**Step 3: Implement the store contract**

Add stage constants `queued`, `provider`, `persisting`, `settling`, `completed`, and `failed`. Add optional/default-empty Ent fields:

```go
field.String("progress_stage").MaxLen(32).Default(""),
field.String("progress_message").MaxLen(256).Default(""),
```

Expose a lease-guarded store method that conditionally updates only stage, message, and `updated_at`. Generate Ent code with:

```bash
go generate ./internal/repository/ent
```

Update the fresh-database migration and entity/domain mapping.

**Step 4: Verify green**

Run the package tests from Step 2 and confirm PASS.

---

### Task 2: Emit truthful execution stages

**Files:**
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/service/imagetask/service_test.go`
- Test: `internal/http/router/tasks_api_test.go`

**Step 1: Write a blocking-provider test**

Use a fake provider with `started` and `release` channels. While `Generate` is blocked, load the task and assert:

```go
task.Status == domainimagetask.StatusRunning
task.ProgressStage == domainimagetask.ProgressStageProvider
```

Record stage updates and assert ordered real boundaries:

```text
provider -> persisting -> settling -> completed
```

Add failure assertions ending in `failed`. Extend the router test so a running SSE/detail snapshot reports `provider`, never `routing` after queueing.

**Step 2: Verify red**

Run:

```bash
go test ./internal/service/imagetask ./internal/http/router
```

Expected: FAIL because provider/persistence/settlement stages are not saved.

**Step 3: Implement stage boundaries**

Before a provider call, decorate the selected candidate and persist `provider`. After provider results return and before object persistence, persist `persisting`. Before billing finalization, persist `settling`. Persist `completed`/`failed` with terminal task updates. Correct the legacy API fallback so `running` with no stored stage maps to `provider`, not `routing`.

Stage-update failure caused by lease conflict must stop the stale worker. Other store failures follow existing task failure/error handling.

**Step 4: Verify green**

Run the Step 2 tests and confirm PASS.

---

### Task 3: Replace model-level compression value with support capability

**Files:**
- Modify: `internal/domain/modelhub/capability.go`
- Modify: `internal/domain/modelhub/resolver.go`
- Modify: `internal/domain/modeladmin/types.go`
- Modify: `internal/repository/ent/schema/modelaccountmodel.go`
- Modify: `internal/repository/ent/migrations/000001_init.sql`
- Modify/generated: relevant `internal/repository/ent/modelaccountmodel*.go`, `mutation.go`, `runtime.go`
- Modify: `internal/repository/entstore/model_admin_store.go`
- Modify: `internal/service/modeladmin/service.go`
- Modify: `api/openapi/components/schemas/admin.yaml`
- Modify: `api/openapi/components/schemas/agent.yaml`
- Test: `internal/domain/modelhub/resolver_test.go`
- Test: `internal/domain/modelhub/route_model_test.go`
- Test: `internal/repository/entstore/model_admin_store_test.go`
- Test: `internal/service/modeladmin/service_test.go`
- Test: `internal/http/router/admin_model_api_test.go`

**Step 1: Write failing capability tests**

Add tests that missing support defaults to false, visible route capability ORs support across candidates, compression `75` filters out unsupported candidates, and compatibility compression `100` remains routable. Add CRUD/API round-trip assertions for `supports_output_compression`.

**Step 2: Verify red**

Run:

```bash
go test ./internal/domain/modelhub ./internal/repository/entstore ./internal/service/modeladmin ./internal/http/router
```

Expected: FAIL because the boolean field and matching behavior do not exist.

**Step 3: Implement and generate**

Add `SupportsOutputCompression bool` to real-model capability, provider candidate, visible route model, admin DTOs, Ent schema, migration, store mapping, and OpenAPI. Keep `image_tasks.output_compression` unchanged. Treat model-level legacy numeric compression as non-authoritative.

Resolver rule:

```go
customCompression := req.OutputCompression > 0 && req.OutputCompression < 100
if customCompression && (!candidate.SupportsOutputCompression || !isCompressibleFormat(req.OutputFormat)) {
    return false
}
```

Regenerate Ent and update all seeded/default model fixtures explicitly.

**Step 4: Verify green**

Run the Step 2 packages and confirm PASS.

---

### Task 4: Omit unsupported upstream compression

**Files:**
- Modify: `internal/provider/contracts.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/provider/openai/client.go`
- Modify: `internal/provider/openrouter/client.go`
- Test: `internal/service/imagetask/service_test.go`
- Test: `internal/provider/openai/client_test.go`
- Test: `internal/provider/openrouter/client_test.go`

**Step 1: Write failing provider tests**

Assert PNG always omits compression, unsupported candidates omit compatibility value `100`, and supported JPEG/WebP candidates pass custom values `1-99`.

**Step 2: Verify red**

Run:

```bash
go test ./internal/provider/... ./internal/service/imagetask
```

Expected: FAIL because default-positive normalization currently sends `100` without support metadata.

**Step 3: Implement minimal request shaping**

Carry candidate support into provider-request compatibility. Set upstream `OutputCompression=0` unless format is JPEG/WebP and the candidate supports compression. Preserve task-level stored request value for audit/history.

**Step 4: Verify green**

Run Step 2 and confirm PASS.

---

### Task 5: Preserve and expose capability fields in the user client

**Files:**
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Modify: mirrored shared files under `web/redesign-demo/shared/` only if repository contract checks require them
- Test/Create: `web/user/src/pages/workspaceParameters.contract.ts`
- Test: existing shared API contract scripts referenced by `scripts/workflow/verify-contracts.sh`

**Step 1: Write failing normalization contract**

Given a capabilities response with all four fields, assert the client retains:

```ts
{
  quality: ['auto', 'high'],
  output_format: ['png', 'webp'],
  supports_output_compression: true,
  moderation: ['auto', 'low'],
}
```

Also assert a missing boolean defaults to false and a running task without backend percentage does not receive a fabricated determinate value.

**Step 2: Verify red**

Run the new contract with the repository's `tsx` pattern. Expected: FAIL because shared types/normalization drop these fields.

**Step 3: Implement shared contract**

Extend `CapabilityItem`, `CapabilityModelGroup`, task types, model admin DTOs, and response normalization. Make numeric task progress optional and stage-authoritative.

**Step 4: Verify green**

Run the contract and:

```bash
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
```

---

### Task 6: Add workspace output controls and truthful status rail

**Files:**
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/workspaceTaskProgress.ts`
- Modify: `web/user/src/pages/WorkspaceStatusRail.tsx`
- Modify: `web/user/src/pages/workspaceViewModel.ts`
- Modify: `web/user/src/pages/workspaceGenerateReadiness.ts`
- Modify: `web/user/src/ui/redesign-classes.ts`
- Modify: `web/shared/user-theme.css`
- Modify: `web/user/src/styles.css`
- Test: `web/user/src/pages/workspaceTaskProgress.contract.ts`
- Test: `web/user/src/pages/workspaceGenerateReadiness.contract.ts`
- Test: `web/user/src/pages/workspaceViewModel.contract.ts`
- Test: `web/user/src/pages/workspaceParameters.contract.ts`
- Test/Create: `web/user/src/pages/workspaceTheme.contract.ts`

**Step 1: Write failing UI contracts**

Cover stage mapping without percentages, monotonic queue-to-provider behavior, unknown-stage running fallback, model/format option normalization, conditional compression visibility, estimate/create payload equality, and absence of hardcoded workspace descendant `text-white` rules.

**Step 2: Verify red**

Run each contract through `npm exec --prefix web/user -- tsx <file>`. Expected: FAIL for missing controls and old progress/theme classes.

**Step 3: Implement controls and status rail**

Add state for quality, output format, compression, and moderation. Derive options from the selected capability, reset invalid selections, and include normalized values in estimate/create. Render compression as a range input plus numeric value only for supported JPEG/WebP. Use localized labels.

Map backend stages directly. During `provider`, show indeterminate motion and elapsed time; do not display a determinate percent. Replace hardcoded white with a semantic `--accent-contrast` token for both themes.

**Step 4: Verify green**

Run all workspace contracts, user typecheck, and user build.

---

### Task 7: Repair storage activation workflow

**Files:**
- Modify: `web/admin/src/pages/StorageConfigPage.tsx`
- Modify: `web/admin/src/pages/storageConfig.contract.ts`
- Test: `internal/http/router/admin_storage_config_api_test.go`

**Step 1: Write failing workflow contracts**

Extract/test pure readiness helpers for dirty state, persistent-probe requirement, action label, and activation version. Extend the API test for:

```text
create -> saved-config probe -> set-default using probe response version -> 200/default
```

Keep the pre-probe `400` test.

**Step 2: Verify red**

Run:

```bash
npm exec --prefix web/admin -- tsx web/admin/src/pages/storageConfig.contract.ts
go test ./internal/http/router -run StorageConfig
```

Expected: frontend contract FAIL because activation calls set-default directly.

**Step 3: Implement activation state machine**

Track the editable signature to block dirty activation. If `last_probe.status` is not success, call `probeStorageConfig(id)` and use the returned version for `setDefaultStorageConfig`. Surface validating/activating labels and preserve detailed API errors. Do not weaken `storageconfig.Service.SetDefault`.

**Step 4: Verify green**

Run Step 2, admin typecheck, and admin build.

---

### Task 8: Polish private gallery spacing and checkbox

**Files:**
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/ui/redesign-classes.ts`
- Test: `web/user/src/pages/galleryExperience.contract.ts`

**Step 1: Write failing class contract**

Assert the private toolbar has a 32px bottom gap, the outer button keeps a 40px target, the inner visual checkbox is 20-22px, and selected/hover/focus/coarse-pointer states remain discoverable.

**Step 2: Verify red**

Run the contract and confirm FAIL because the visual checkbox currently inherits `size-10`.

**Step 3: Implement scoped markup/classes**

Pass a private-page margin class to `GalleryFilterToolbar`. Render a Lucide check inside a separate visual span. Keep the button transparent, `aria-pressed`, focus visibility, and touch affordance.

**Step 4: Verify green**

Run the contract, user typecheck, and user build.

---

### Task 9: Full verification and Docker acceptance

**Files:**
- Update only if required by changed contracts: `scripts/workflow/verify-contracts.sh`, relevant audit evidence under `docs/audits/`

**Step 1: Format and focused verification**

Run `gofmt` on changed Go files and all focused Go/React contracts from Tasks 1-8.

**Step 2: Repository verification**

Run:

```bash
./scripts/workflow/verify.sh
```

Expected: all Go tests/vet and both frontend typecheck/build commands PASS.

**Step 3: Review gate**

Run the local review gate appropriate for the dirty worktree and inspect every changed production file for unrelated deltas. Do not stage unrelated existing modifications.

**Step 4: Real API smoke**

Run:

```bash
./scripts/workflow/api-smoke.sh
```

Expected: isolated API contract smoke PASS.

**Step 5: Docker rebuild**

Run:

```bash
BUILDKIT_PROGRESS=plain docker compose -f deployments/docker-compose/docker-compose.dev.yml up -d --build --remove-orphans
```

Confirm all health-checked services are healthy and `/readyz`, `/`, `/admin/`, and `/developer-docs/` return `200`.

**Step 6: Browser acceptance**

At desktop and 390px mobile widths verify:

- A real generation advances from queueing to provider generation and terminal completion without a fake percentage.
- Light-theme model, size, ratio, quality, output, and moderation controls are legible.
- Compression appears only for supported JPEG/WebP models and reaches estimate/create payloads.
- Storage activation probes the saved config and switches default using the latest version.
- Asset toolbar spacing and selection hover/focus/selected states match the design.

Capture screenshots and leave the Docker stack running for user inspection.
