# Prompt Optimization and Image Workflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add secure multi-account text-model configuration and prompt optimization, unify image-detail/configuration reuse, and completely remove the unused reference-generation feature.

**Architecture:** Add normalized Ent entities and a dedicated textmodel service/provider boundary, expose admin configuration plus user estimate/execute APIs, and keep prompt optimization state in the workspace. Consolidate image details around one component and pass typed one-time creation drafts through route state/session storage. Perform a destructive pre-schema cleanup for all reference-generation data and delete every legacy enum/compatibility branch.

**Tech Stack:** Go 1.26, Ent, PostgreSQL, React 19, TypeScript, Vite, OpenAPI 3.1, Docker Compose, Node E2E scripts.

---

### Task 1: Establish repository coding context and guardrails

**Files:**
- Existing: `docs/plans/2026-07-20-prompt-optimization-and-image-workflow-design.md`
- Existing: `.coding-context.json` (ignored runtime artifact)

**Step 1: Start heavyweight workflow context**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "Prompt optimization, text model accounts, reference generation removal, unified image details, and reusable creation drafts"
```

Expected: exit 0 and `.coding-context.json` names both requirement and technical-design sources.

**Step 2: Load implementation guardrails**

Read `.agents/skills/dev-go-patterns/SKILL.md`, `.agents/skills/dev-react-patterns/SKILL.md`, and the TDD skill before editing production code.

**Step 3: Record local secret safety**

Run `git check-ignore .env.local` and `git status --short`.

Expected: `.env.local` is ignored and no credential file appears in Git status.

### Task 2: Remove reference generation from backend contracts and data

**Files:**
- Modify: `internal/provider/contracts.go`
- Modify: `internal/config/load.go`
- Modify: `internal/domain/modelhub/resolver.go`
- Modify: `internal/domain/billing/calculator.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/modeladmin/service.go`
- Modify: `internal/repository/db/legacy_migrations.go`
- Modify: `internal/repository/db/legacy_migrations_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `api/openapi/components/parameters/common.yaml`
- Modify: `api/openapi/components/schemas/agent.yaml`
- Modify: affected Go tests under `internal/**`

**Step 1: Write failing removal tests**

Add tests proving:

- `reference_generate` is rejected as an invalid task type;
- capabilities and visible route models never publish it;
- the destructive migration deletes legacy tasks plus dependent results, attempts, reservations, ledger records, and route/model configuration;
- no billing fallback accepts a legacy multiplier.

**Step 2: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/repository/db ./internal/domain/modelhub ./internal/service/imagetask ./internal/http/router
```

Expected: new assertions fail against current legacy behavior.

**Step 3: Implement destructive cleanup and task-type removal**

Add an idempotent, transaction-protected pre-schema PostgreSQL cleanup guarded by table/column existence. Delete dependent rows before tasks and configuration, then remove legacy constants, defaults, normalization, handlers, and compatibility branches. Keep only `text_to_image` and `image_edit`.

**Step 4: Update OpenAPI and focused tests**

Remove legacy enum values and examples. Run the same focused Go tests plus:

```bash
go test ./api/openapi
```

Expected: PASS and repository search finds no production use of `reference_generate` or `reference_to_image`.

**Step 5: Commit**

```bash
git add internal api/openapi
git commit -m "refactor(image): remove reference generation"
```

### Task 3: Add text-model Ent schema, domain, and repository

**Files:**
- Create: `internal/repository/ent/schema/textmodelaccount.go`
- Create: `internal/repository/ent/schema/textmodel.go`
- Create: `internal/repository/ent/schema/promptoptimizationrun.go`
- Create: `internal/domain/textmodel/types.go`
- Create: `internal/service/textmodel/store.go`
- Create: `internal/repository/entstore/text_model_store.go`
- Create: `internal/repository/entstore/text_model_store_test.go`
- Modify: `internal/repository/ent/schema/schema_test.go`
- Generated: `internal/repository/ent/**`

**Step 1: Write failing schema/store tests**

Cover account CRUD, encrypted secret persistence, secret redaction, nested model CRUD, enabled default uniqueness, optimistic version conflicts, and optimization-run persistence.

**Step 2: Run tests and confirm failure**

```bash
go test ./internal/repository/ent/schema ./internal/repository/entstore
```

Expected: missing schema/store failures.

**Step 3: Implement schemas and domain types**

Use normalized child models, decimal strings for token prices, ISO currency codes, soft deletion where repository conventions require it, and a single default marker enforced transactionally.

**Step 4: Generate Ent code**

```bash
go generate ./internal/repository/ent
```

Expected: generated clients/builders for all three entities.

**Step 5: Implement Ent store and encryption integration**

Reuse `internal/service/secretcodec`. Never return encrypted or plaintext credentials from domain views.

**Step 6: Run focused tests**

```bash
go test ./internal/repository/ent/schema ./internal/repository/entstore
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/domain/textmodel internal/repository internal/service/textmodel/store.go
git commit -m "feat(textmodel): add secure account persistence"
```

### Task 4: Implement OpenAI-compatible text adapters

**Files:**
- Create: `internal/provider/text/contracts.go`
- Create: `internal/provider/text/openai/client.go`
- Create: `internal/provider/text/openai/client_test.go`

**Step 1: Write failing protocol tests**

Use `httptest.Server` to assert:

- Chat Completions sends `POST /v1/chat/completions` and parses `choices[0].message.content` plus usage;
- Responses sends `POST /v1/responses` and parses `output_text` and structured output content plus usage;
- base URLs with or without `/v1` resolve exactly once;
- authorization headers are correct but absent from errors;
- timeout, rate-limit, auth, and malformed response errors are classified and sanitized;
- redirect targets are rejected when they violate the validated destination policy.

**Step 2: Run the test and confirm failure**

```bash
go test ./internal/provider/text/openai
```

Expected: package or symbols missing.

**Step 3: Implement the minimal shared client**

Keep request construction per API style and share transport/error parsing. Require a concise system instruction that returns only the optimized prompt.

**Step 4: Run focused tests**

```bash
go test ./internal/provider/text/openai
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/provider/text
git commit -m "feat(textmodel): support OpenAI-compatible text APIs"
```

### Task 5: Implement text-model admin and prompt-optimization services

**Files:**
- Create: `internal/service/textmodel/service.go`
- Create: `internal/service/textmodel/service_test.go`
- Create: `internal/service/promptoptimizer/service.go`
- Create: `internal/service/promptoptimizer/service_test.go`
- Modify: `internal/http/handlers/admin_permissions.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/router.go`
- Create: `internal/http/router/admin_text_model_api_test.go`
- Create: `internal/http/router/prompt_optimization_api_test.go`
- Modify: `internal/app/app.go` and wiring files discovered there

**Step 1: Write failing service and router tests**

Cover dangerous-config authorization, account/model validation, write-only secrets, default selection, connection test sanitization, unavailable default behavior, zero-point estimates, estimate expiry/version conflicts, successful optimize records, and failed optimize no-charge behavior.

**Step 2: Run focused tests and confirm failure**

```bash
go test ./internal/service/textmodel ./internal/service/promptoptimizer ./internal/http/router
```

Expected: missing service/routes.

**Step 3: Implement services and dependency wiring**

Use a stable estimate payload with model ID, configuration version, prompt digest, expiry, and `0.00000` points. Execution must recompute all authoritative fields and reject stale or mismatched estimates.

**Step 4: Implement handlers and audit events**

Add account/model/default/test endpoints and the two user endpoints. Audit configuration mutations without credentials or full prompts.

**Step 5: Run focused tests**

```bash
go test ./internal/service/textmodel ./internal/service/promptoptimizer ./internal/http/router
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal
git commit -m "feat(prompt): add optimization APIs"
```

### Task 6: Publish OpenAPI and shared web contracts

**Files:**
- Modify: `api/openapi/openapi.yaml`
- Modify: `api/openapi/components/schemas/admin.yaml`
- Create: `api/openapi/components/schemas/text.yaml`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/shared/user-api.ts`
- Modify: `web/shared/open-api.ts`
- Modify: relevant `web/shared/*.contract.ts`

**Step 1: Write failing contract assertions**

Assert typed admin CRUD/default/test requests, prompt estimate/execute envelopes, creation-draft generation inputs, and the complete absence of legacy task types.

**Step 2: Run contracts and confirm failure**

Use the repository's TypeScript contract invocation discovered from existing files, plus:

```bash
go test ./api/openapi
```

**Step 3: Implement schemas and clients**

Keep secret fields write-only and prompt estimate/execute errors stable. Delete all frontend legacy conversion functions.

**Step 4: Run typechecks and OpenAPI tests**

```bash
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
go test ./api/openapi
```

Expected: PASS.

**Step 5: Commit**

```bash
git add api/openapi web/shared
git commit -m "feat(api): publish prompt optimization contracts"
```

### Task 7: Build admin text-model configuration

**Files:**
- Create: `web/admin/src/pages/TextModelsPage.tsx`
- Create: `web/admin/src/pages/textModelRows.ts`
- Create: `web/admin/src/pages/textModelRows.contract.ts`
- Modify: `web/admin/src/pages/SystemSettingsPage.tsx`
- Modify: `web/admin/src/pages/systemSettingsTabs.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/styles.css` only where existing class utilities cannot express the layout

**Step 1: Write failing view-model contracts**

Cover account/model draft normalization, URL/API-style validation, secret unchanged/replace/clear semantics, token price validation, default-model selection, dirty-state protection, and sanitized connection-test feedback.

**Step 2: Run the contract and confirm failure**

Run it with the same TypeScript runner used by existing `*.contract.ts` files.

**Step 3: Implement the page**

Add a work-focused account list and editor under System Settings. Use icons/tooltips, compact tables, explicit enabled toggles, API-style segmented control, password input, model rows, default selector, and connection-test action.

**Step 4: Run admin checks**

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 5: Commit**

```bash
git add web/admin
git commit -m "feat(admin): manage text model accounts"
```

### Task 8: Build synchronized prompt expansion and optimization UI

**Files:**
- Create: `web/user/src/pages/workspacePromptOptimization.ts`
- Create: `web/user/src/pages/workspacePromptOptimization.contract.ts`
- Create: `web/user/src/pages/PromptEditorDialog.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/ui/icons.ts`
- Modify: `web/user/src/styles.css` only as needed

**Step 1: Write failing state-machine contracts**

Cover estimate-required flow, zero-point confirmation, duplicate-submit prevention, stale-estimate refresh, comparison, apply, cancel, one-step undo, failure preservation, and shared compact/dialog prompt state.

**Step 2: Run the contract and confirm failure**

Run with the existing contract runner.

**Step 3: Implement the shared state machine and dialog**

Place Expand and Optimize icons in the compact editor and another Optimize icon inside the expanded editor. Show current edit-source thumbnails/metadata without including them in the optimization request.

**Step 4: Verify user app**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

Expected: PASS.

**Step 5: Commit**

```bash
git add web/user
git commit -m "feat(workspace): expand and optimize prompts"
```

### Task 9: Unify image details and replace configuration copying

**Files:**
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/routeState.ts`
- Create: `web/user/src/pages/workspaceCreationDraft.ts`
- Create: `web/user/src/pages/workspaceCreationDraft.contract.ts`
- Modify: existing gallery/workspace/public-gallery contract files

**Step 1: Write failing component-model and draft tests**

Cover full prompt presentation, copy permission, login return state, shared action configuration, serialization without URL leakage, one-time session fallback, capability-aware parameter restoration, accessible reference restoration, and explicit fallback notices.

**Step 2: Run contracts and confirm failure**

Run the relevant user contract files.

**Step 3: Consolidate image-detail composition**

Use one detail layout and image media/zoom path while injecting page-specific actions. Delete JSON clipboard configuration copying.

**Step 4: Implement typed creation drafts**

Route to the workspace, consume the draft exactly once, load accessible references, validate every option against current capability, and report deterministic fallbacks.

**Step 5: Run user checks**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

Expected: PASS.

**Step 6: Commit**

```bash
git add web/user
git commit -m "feat(gallery): reuse image configurations"
```

### Task 10: Finish reference-generation deletion across UI, docs, and fixtures

**Files:**
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/galleryRows.ts`
- Modify: `web/user/src/pages/publicGalleryModel.ts`
- Modify: `web/admin/src/pages/adminTaskTypes.ts`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `web/shared/mock-data.ts`
- Modify: all affected `*.contract.ts`, docs, examples, and seed fixtures

**Step 1: Add absence contracts**

Assert user/admin task options contain only text-to-image and image-edit, and no mock/API serialization revives the old type.

**Step 2: Delete remaining UI/configuration paths**

Remove the tab, separate reference arrays, labels, filters, model capability options, pricing rows, demo data, and obsolete documentation claims.

**Step 3: Search for leftovers**

```bash
rg -n "reference_generate|reference_to_image|参考生图" --glob '!docs/plans/2026-07-20-*'
```

Expected: no production/config/API/test fixture occurrence; only intentional historical design/review documents may remain if repository policy keeps them immutable.

**Step 4: Run both web builds**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 5: Commit**

```bash
git add web docs scripts
git commit -m "refactor(web): remove reference generation UI"
```

### Task 11: Configure and test the supplied text models locally

**Files:**
- Ignored: `.env.local`
- Modify: local database through admin APIs only

**Step 1: Verify model availability without exposing credentials**

Use the supplied local environment values to call the provider model listing or minimal text endpoint. Do not print request headers, environment contents, or credential-bearing errors.

**Step 2: Resolve official price metadata**

Use official OpenAI pricing documentation for exact matching public model IDs. If a supplied alias has no official price, store `0.000000` reserved prices and report the unresolved alias rather than inventing a price.

**Step 3: Create local account and models**

Configure both supported API styles as applicable, add the supplied model list, and select the requested default model through the real admin API.

**Step 4: Execute a real prompt estimate and optimization**

Assert estimate is zero, response is non-empty, run metadata is stored, and no secret appears in API responses or logs.

### Task 12: Extend Docker E2E and browser verification

**Files:**
- Modify: `scripts/e2e/docker-e2e.mjs`
- Modify: `scripts/e2e/run-docker-e2e.sh` if environment plumbing is needed
- Modify: `deployments/docker-compose/docker-compose.e2e.yml` only for non-secret test wiring
- Create/modify: focused Playwright/browser verification script under `scripts/visual/`

**Step 1: Extend the fake provider**

Support both `/v1/chat/completions` and `/v1/responses`, record sanitized request shape, and return deterministic optimized prompts and usage.

**Step 2: Add backend E2E flow**

Create an admin text account/model/default, test connection, estimate/execute optimization, assert zero charge, and verify legacy task types are rejected.

**Step 3: Add browser workflow checks**

Verify desktop and mobile prompt expansion, Optimize icons in both locations, zero-point confirmation, compare/apply/undo, unified details, prompt copy, login return, and reuse configuration restoration. Save screenshots under ignored E2E output.

**Step 4: Run Docker E2E**

```bash
./scripts/e2e/run-docker-e2e.sh
```

Expected: all steps PASS and browser screenshots show no overlap or clipped controls.

**Step 5: Commit**

```bash
git add scripts/e2e scripts/visual deployments/docker-compose
git commit -m "test(e2e): cover prompt optimization workflow"
```

### Task 13: Full verification, review, merge, and Docker rebuild

**Files:**
- Generated ignored review artifacts: `.review/`

**Step 1: Run repository verification**

```bash
./scripts/workflow/verify.sh
```

Expected: Go test/vet and both web typecheck/build pipelines PASS.

**Step 2: Run local API smoke**

```bash
./scripts/workflow/api-smoke.sh
```

Expected: PASS.

**Step 3: Review committed scope**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: PASS marker for current HEAD tree. Fix all BLOCK findings, recommit, rerun verification, and regenerate the marker as needed.

**Step 4: Run ship guard**

```bash
./scripts/workflow/ship-guard.sh
```

Expected: PASS.

**Step 5: Merge into main**

Confirm the primary `main` worktree is clean, merge `codex/prompt-optimization-text-models` into `main` without discarding external changes, and verify the resulting tree.

**Step 6: Rebuild Docker services from final main**

Use the repository Docker Compose/run scripts to stop the task-owned stack, rebuild images without stale application layers, recreate services, wait for health, and rerun API plus E2E smoke against the final `main` tree.

Expected: all services healthy and final smoke/E2E PASS.

