# v0.0.6 Runtime Hotfix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Repair JeePay order creation, authorized BFSS delivery, complete workspace image details, distinct landing assets, and one-screen login layout, then push an isolated hotfix branch for manual testing.

**Architecture:** Keep billing and authorization boundaries intact while adding protocol defaults at JeePay request construction, an optional S3 temporary-URL capability, and a complete frontend detail projector. Generate seven optimized semantic assets and verify responsive behavior with browser geometry and screenshots.

**Tech Stack:** Go 1.26, AWS Signature V4-compatible S3/BFSS HTTP, React 19, TypeScript, Vite, Tailwind CSS, Playwright/browser automation, OpenAI-compatible `gpt-image-2` image API.

---

### Task 1: Establish approved coding context

**Files:**
- Create: `.coding-context.json` through workflow tooling
- Reference: `docs/prd/2026-08-03-v006-runtime-hotfix-requirements.md`
- Reference: `docs/tech/2026-08-03-v006-runtime-hotfix-tech-design.md`

**Step 1: Run source discovery**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "v0.0.6 runtime hotfix payment API BFSS storage image detail landing login"
```

Expected: exit 0 and the new requirement/design files selected.

**Step 2: Record approval**

Inspect `.coding-context.json`, set its heavyweight approval status to `approved`, and retain the generated discovery report.

**Step 3: Re-read authoritative sources**

Read both selected documents before production edits. Expected: every numbered owner requirement is represented.

**Step 4: Commit workflow context**

Stage the three documents and `.coding-context.json`, then commit `docs: define v006 runtime hotfix`.

### Task 2: Repair JeePay default prepay parameters and errors

**Files:**
- Modify: `internal/service/cashier/jeepay_provider_test.go`
- Modify: `internal/service/cashier/jeepay_provider.go`
- Modify: `web/shared/http-client.ts`
- Create or modify: `web/shared/http-client.contract.ts`

**Step 1: Write failing default-channel tests**

Add tests that capture the JSON request for `ALI_PC` and `WX_NATIVE` without configured channel extra. Assert compact `payUrl`/`codeUrl` defaults and verify an explicit channel extra remains unchanged.

**Step 2: Verify red**

Run:

```bash
go test ./internal/service/cashier -run 'TestJeePay.*DefaultChannelExtra' -count=1
```

Expected: FAIL because `channelExtra` is currently absent.

**Step 3: Implement minimal defaults**

Add a small pure helper selected by provider/way code and insert its result before signature construction only when no explicit value exists.

**Step 4: Verify green**

Run the focused JeePay tests. Expected: PASS.

**Step 5: Write failing sanitized-diagnostic tests**

Cover non-2xx and JeePay non-success responses. Assert stage/status/code/bounded message are retained while merchant key, signature, redirect query, and full payload are absent.

**Step 6: Implement diagnostic wrapping**

Add typed/internal error context or structured logging at the provider boundary without changing the public `PAYMENT_PROVIDER_UNAVAILABLE` envelope.

**Step 7: Write and pass the frontend localization contract**

Assert `PAYMENT_PROVIDER_UNAVAILABLE` maps to a payment-channel message in Chinese and English, then add the explicit map entry.

**Step 8: Run slice tests and commit**

Run all cashier JeePay tests and the shared HTTP contract; commit `fix: repair Jeepay prepay defaults`.

### Task 3: Redirect authenticated S3/BFSS image delivery

**Files:**
- Modify: `internal/storage/backend.go`
- Modify: `internal/storage/backend_test.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/imagetask/service_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: relevant handler/router tests under `internal/http/router/`

**Step 1: Write failing S3 presign tests**

Define the desired optional signer API in tests with a fixed clock. Assert SigV4 query fields, normalized path, expiry clamp, response disposition support, and no network call.

**Step 2: Verify red**

Run `go test ./internal/storage -run TemporaryGetURL -count=1`. Expected: compile/fail because the capability is missing.

**Step 3: Implement the presigner**

Reuse existing endpoint/path/signing helpers, canonicalize sorted query values, and keep `Backend` unchanged.

**Step 4: Verify storage green**

Run all storage tests. Expected: PASS.

**Step 5: Write failing delivery service tests**

Use a signing backend that fails if `Get` is called, and a local backend that returns bytes. Assert ownership, backend routing, redirect delivery, byte fallback, and error normalization.

**Step 6: Implement delivery projection**

Add the service delivery value/method and preserve current download methods for unaffected routes.

**Step 7: Write failing handler tests**

Assert S3 owned images return `307`, `Location`, and no-store headers; local images return the current `200` body/headers.

**Step 8: Implement handler dispatch and verify**

Update `HandleImageDownload`, run storage/imagetask/router focused tests, and commit `fix: redirect signed image delivery`.

### Task 4: Preserve complete workspace image details

**Files:**
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Create: `web/user/src/pages/workspaceImageDetail.ts`
- Create: `web/user/src/pages/workspaceImageDetail.contract.ts`
- Modify: relevant existing image-detail contracts

**Step 1: Write a failing projector contract**

Build an image/task/profile fixture and assert the projected detail contains author, actual dimensions, model, task type, size mode, requested/base size, ratio, quality, format, compression, moderation, count, and prompt with image-specific precedence.

**Step 2: Verify red**

Run the contract. Expected: FAIL because no complete projector exists and preview payload loses fields.

**Step 3: Implement the pure projector**

Create the minimal typed merge helper and extend `ImagePreviewPayload` with a complete detail snapshot.

**Step 4: Wire current/history previews**

Pass the parent task and active profile through generated image, history card, and history dialog paths. Keep reference previews minimal.

**Step 5: Write failing metadata-rendering assertions**

Assert a dedicated actual-resolution label and the additional generation metadata labels/classes exist in the shared detail component.

**Step 6: Implement compact metadata rendering**

Render responsive rows with accessible values and `-` for missing optional data.

**Step 7: Verify and commit**

Run image-detail/workspace contracts, user typecheck, and commit `fix: preserve workspace image metadata`.

### Task 5: Generate and integrate seven semantic landing images

**Files:**
- Local ignored: `.secrets/imagegen.env`
- Local ignored: `tmp/imagegen/v006-hotfix/`
- Create: optimized assets under `web/user/public/landing/`
- Modify: `web/user/src/pages/landingContent.ts`
- Modify: `web/user/src/pages/LandingPage.tsx`
- Modify: `web/user/src/pages/landingPage.contract.ts`
- Modify: `web/user/src/pages/landingContent.contract.ts`

**Step 1: Establish local-only credential boundary**

Create a mode-600 ignored credential file from the owner-supplied endpoint/key/model. Prove Git and Docker ignore it without printing its value.

**Step 2: Define seven prompt specs**

Use the semantic map in the technical design, shared Mikiko art direction, explicit no-text/no-logo/no-watermark constraints, and target crop/orientation for each placement.

**Step 3: Generate masters through the approved API**

Use `gpt-image-2` and high quality. Keep masters ignored. Inspect every image and regenerate only failed semantics/text/crops.

**Step 4: Produce optimized derivatives**

Use ffmpeg or the available structured encoder to create WebP and AVIF outputs. Verify dimensions, decoding, nonblank pixels, and practical byte sizes.

**Step 5: Write failing uniqueness contracts**

Assert every affected capability/mode points at a distinct semantic path and both previously backgroundless capabilities have images.

**Step 6: Integrate assets and stable dimensions**

Update landing content/types and page rendering without filename-specific dimension logic.

**Step 7: Verify contracts/build and commit**

Run landing contracts and user build; commit `feat: add semantic landing visuals`.

### Task 6: Make every login flow fit the desktop viewport

**Files:**
- Modify: `web/user/src/pages/LoginPage.tsx`
- Modify: `web/user/src/pages/loginPage.contract.ts`
- Add or modify browser verification scripts/evidence under existing repository conventions

**Step 1: Capture the failing `1512x982` baseline**

Run the user app, open login/register/reset/password-setup states, record `innerHeight`/`scrollHeight`, and take screenshots. Expected: at least one state overflows.

**Step 2: Write failing static layout assertions**

Require bounded `dvh` desktop layout, contained overflow, and constrained-height spacing rules.

**Step 3: Implement height-aware layout**

Use stable grid tracks, `min-h-0`, smaller desktop vertical padding, and short-height compact spacing without changing authentication behavior.

**Step 4: Verify all desktop states**

At `1512x982`, assert no page scroll and all interactive controls remain within the viewport.

**Step 5: Verify mobile fallback**

Capture mobile states and confirm controls remain accessible without overlap; permit document scrolling only where touch-safe layout requires it.

**Step 6: Run contracts/typecheck/build and commit**

Commit `fix: fit login flows within viewport`.

### Task 7: Cross-slice verification and review

**Files:**
- Modify tests only if a verified defect is found
- Create committed review marker through repository tooling

**Step 1: Run focused suites**

Run JeePay, storage, imagetask, router, shared HTTP, image-detail, landing, login, and checkout contracts.

**Step 2: Run full verification**

Run `./scripts/workflow/verify.sh`. Expected: PASS. Re-run the two known timing-sensitive baseline tests without competing CPU load and require PASS.

**Step 3: Run isolated API smoke**

Run `./scripts/workflow/api-smoke.sh`. Expected: PASS and cleanup completes.

**Step 4: Perform browser acceptance**

Inspect screenshots and DOM geometry for landing desktop/mobile, login states, image detail/zoom, and checkout popup behavior. Confirm no blank assets, overlap, unwanted text, or broken crops.

**Step 5: Review working changes**

Review the complete diff for authorization bypass, secret leakage, open redirects, signed URL caching, payment regression, accessibility, and responsive layout.

**Step 6: Commit fixes and final scope**

Commit any review corrections, then ensure the worktree contains only intended files and ignored local generation intermediates.

### Task 8: Prepare manual-test branch

**Files:**
- Generate: `.review/gate.json`

**Step 1: Run committed-scope review**

Run `./scripts/workflow/review-local.sh --scope committed` and require `PASS` at the current tree SHA.

**Step 2: Check review gate**

Run `./scripts/workflow/check-review-gate.sh`. Expected: PASS.

**Step 3: Push the hotfix branch**

Push `codex/hotfix-v006-runtime-remediation` to origin.

**Step 4: Stop at manual testing**

Report branch, commit, verification evidence, browser evidence, and manual test focus. Do not create/merge a PR, do not update `main`, and do not create a tag or GitHub Release.
