# v0.0.5 Production Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver the approved payment, image-detail, size-capability, password/session, and visual-asset remediation found after `v0.0.5`, then merge and publish a verified release.

**Architecture:** Implement five vertical contracts with backend authority and shared frontend projections. Preserve existing payment polling, model routing, users, and storage while adding a purpose-bound password setup grant and persistent custom-size capability. Use TDD for behavioral code and browser verification for interaction and visual assets.

**Tech Stack:** Go 1.24, Ent, PostgreSQL, Redis, React 19, TypeScript, Vite, Tailwind CSS, Playwright, JeePay HTTP API, OpenAI-compatible `gpt-image-2` CLI workflow.

---

### Task 1: Establish coding context

**Files:**
- Create: `.coding-context.json` through the workflow script
- Reference: `docs/prd/2026-08-02-v005-production-remediation-requirements.md`
- Reference: `docs/tech/2026-08-02-v005-production-remediation-tech-design.md`

**Step 1: Run source discovery**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "v0.0.5 payment image detail custom dimensions password session and visual asset remediation"
```

Expected: exit 0 and both requirement/design sources listed in `.coding-context.json`.

**Step 2: Inspect the generated context**

Run: `sed -n '1,220p' .coding-context.json`

Expected: the two documents above are selected and track is `heavyweight`.

**Step 3: Commit documentation and context**

```bash
git add docs/prd/2026-08-02-v005-production-remediation-requirements.md \
  docs/tech/2026-08-02-v005-production-remediation-tech-design.md \
  docs/plans/2026-08-02-v005-production-remediation-implementation.md \
  .coding-context.json
git commit -m "docs: define v005 production remediation"
```

### Task 2: Make JeePay unified order server-side only

**Files:**
- Modify: `internal/service/cashier/jeepay_provider.go`
- Test: `internal/service/cashier/jeepay_provider_test.go`
- Modify: `web/admin/src/pages/cashierProviderOptions.ts`
- Test: `web/admin/src/pages/cashierProviderOptions.contract.ts`

**Step 1: Replace the legacy GET regression test with a failing POST JSON test**

Add an `httptest.Server` assertion that a JeePay instance with no `payment_mode` sends `POST`, `Content-Type: application/json`, and a JSON body containing `reqTime`, `version=1.0`, `signType=MD5`, merchant fields, fen amount, callbacks, and a non-empty signature.

**Step 2: Verify red**

Run: `go test ./internal/service/cashier -run 'TestJeePayPaymentDisplayBuilderDefaultsToAPIPost' -count=1`

Expected: FAIL because the current implementation returns a browser GET URL.

**Step 3: Add response-classification tests**

Cover explicit `payUrl`, explicit `codeUrl`, HTTP(S) `payData`, QR/native `payData`, non-2xx, invalid JSON, and sanitized provider errors.

**Step 4: Implement JSON API prepay**

Make `BuildJeePayPaymentDisplay` always call `BuildJeePayAPIPayment`. Encode the signed map as JSON, add request time/version before signing, set JSON headers, and project the response into redirect or QR display.

**Step 5: Verify green**

Run: `go test ./internal/service/cashier -run JeePay -count=1`

Expected: PASS.

**Step 6: Remove the JeePay payment-mode UI contract**

Add a failing frontend contract asserting JeePay structured fields contain no `payment_mode` and do not mention popup mode, then remove the field for JeePay while retaining modes for providers that genuinely support them.

**Step 7: Run frontend contract**

Run: `npx --yes tsx web/admin/src/pages/cashierProviderOptions.contract.ts`

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/service/cashier/jeepay_provider.go internal/service/cashier/jeepay_provider_test.go \
  web/admin/src/pages/cashierProviderOptions.ts web/admin/src/pages/cashierProviderOptions.contract.ts
git commit -m "fix: use Jeepay unified order API"
```

### Task 3: Automatically open redirect and form payments

**Files:**
- Create: `web/user/src/pages/checkoutPaymentWindow.ts`
- Test: `web/user/src/pages/checkoutPaymentWindow.contract.ts`
- Modify: `web/user/src/pages/CheckoutPage.tsx`
- Modify: `web/user/src/pages/checkoutPaymentDisplay.contract.ts`

**Step 1: Write a failing reservation lifecycle contract**

Use a small fake `Window` surface to assert reservation opens synchronously, redirect uses `location.replace`, form HTML is written/submitted, and unused/error reservations close.

**Step 2: Verify red**

Run: `npx --yes tsx web/user/src/pages/checkoutPaymentWindow.contract.ts`

Expected: FAIL because the module does not exist.

**Step 3: Implement the pure reservation helper**

Expose `reservePaymentWindow`, `dispatchPaymentWindow`, and `closePaymentWindow`. Keep DOM access behind injected/opened window objects so the behavior test does not depend on a browser.

**Step 4: Verify helper green**

Run the contract again; expected PASS.

**Step 5: Add a failing CheckoutPage wiring contract**

Assert reservation occurs before `await userApi.createCashierOrder`, dispatch uses `checkoutPaymentDisplayModel(nextOrder)`, failure closes the window, and the payment modal/polling remains active.

**Step 6: Wire the submit flow**

Reserve after local validation and before the network request. Dispatch after order creation. Preserve manual fallback and status polling.

**Step 7: Verify user checkout contracts**

```bash
npx --yes tsx web/user/src/pages/checkoutPaymentWindow.contract.ts
npx --yes tsx web/user/src/pages/checkoutPaymentDisplay.contract.ts
npx --yes tsx web/user/src/pages/checkoutOrderPolling.contract.ts
```

Expected: PASS.

**Step 8: Commit**

```bash
git add web/user/src/pages/checkoutPaymentWindow.ts web/user/src/pages/checkoutPaymentWindow.contract.ts \
  web/user/src/pages/CheckoutPage.tsx web/user/src/pages/checkoutPaymentDisplay.contract.ts
git commit -m "fix: open redirect payments automatically"
```

### Task 4: Unify production image details

**Files:**
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/ui/redesign-classes.ts`
- Modify: `web/user/src/pages/HomePage.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Test: `web/user/src/imageLightboxLayer.contract.ts`
- Test: `web/user/src/imageLightboxMedia.contract.ts`
- Test: `web/user/src/pages/workspaceImageActions.contract.ts`
- Create: `web/user/src/imageDetailUnification.contract.ts`

**Step 1: Write a failing project-wide nesting contract**

Assert production pages use `ImageDetailModal`, no production page renders `ImageLightbox`, and the detail component opens `ImageZoomViewer` directly from the main image.

**Step 2: Verify red**

Run: `npx --yes tsx web/user/src/imageDetailUnification.contract.ts`

Expected: FAIL on current `ImageLightbox` callers and nested behavior.

**Step 3: Extend the detail view model and action slots**

Add typed optional metadata, reference images, stats, and `ImageDetailAction[]`. Keep icon buttons accessible with labels/tooltips.

**Step 4: Bound the prompt panel**

Use a responsive max height, `overflow-y-auto`, focusability, and word wrapping. Add contract assertions for these classes/attributes.

**Step 5: Migrate each caller**

Map Home, Workspace/history, Gallery, and PublicGallery state into the shared detail model and only pass valid actions. Remove the detail-style `ImageLightbox` implementation after the last caller moves; retain `ImageZoomViewer`.

**Step 6: Verify green**

```bash
npx --yes tsx web/user/src/imageDetailUnification.contract.ts
npx --yes tsx web/user/src/imageLightboxLayer.contract.ts
npx --yes tsx web/user/src/imageLightboxMedia.contract.ts
npx --yes tsx web/user/src/pages/workspaceImageActions.contract.ts
```

Expected: PASS.

**Step 7: Commit**

```bash
git add web/user/src/components.tsx web/user/src/ui/redesign-classes.ts \
  web/user/src/pages/HomePage.tsx web/user/src/pages/WorkspacePage.tsx \
  web/user/src/pages/GalleryPage.tsx web/user/src/pages/PublicGalleryPage.tsx \
  web/user/src/imageDetailUnification.contract.ts web/user/src/imageLightboxLayer.contract.ts \
  web/user/src/imageLightboxMedia.contract.ts web/user/src/pages/workspaceImageActions.contract.ts
git commit -m "refactor: unify image detail experience"
```

### Task 5: Add authoritative custom-size normalization

**Files:**
- Modify: `internal/domain/modelhub/image_size.go`
- Test: `internal/domain/modelhub/image_size_test.go`
- Modify: `internal/domain/modelhub/capability.go`
- Test: `internal/domain/modelhub/capability_compression_test.go`
- Modify: `internal/domain/modelhub/resolver.go`
- Test: `internal/domain/modelhub/route_model_test.go`
- Modify: `web/shared/image-size.ts`
- Test: `web/shared/image-size.contract.ts`

**Step 1: Write table-driven failing Go tests**

Cover already-legal input, non-16 input, too-small square, too-large square, excessive landscape/portrait ratio, max-edge pressure, invalid input, and idempotence.

**Step 2: Verify red**

Run: `go test ./internal/domain/modelhub -run 'TestNormalizeCustomImageSize' -count=1`

Expected: FAIL because the function is absent.

**Step 3: Implement deterministic normalization**

Add `NormalizeCustomImageSize(width, height int) (string, error)` plus invariant helpers. Keep constants shared with `CalculateImageSize`.

**Step 4: Add failing capability/resolver tests**

Assert preset-only candidates reject arbitrary input and custom-enabled candidates accept the backend-normalized result.

**Step 5: Extend capability types and matching**

Add `SupportsCustomSize` to `ImageModelCapability`, `ProviderCandidate`, and route capability aggregation; use it only for pixel mode.

**Step 6: Mirror normalization in TypeScript**

Write the same table in `image-size.contract.ts`, observe failure, then add `normalizeCustomImageSize` returning requested and normalized dimensions plus validity metadata.

**Step 7: Verify green**

```bash
go test ./internal/domain/modelhub -count=1
npx --yes tsx web/shared/image-size.contract.ts
```

Expected: PASS with Go/TypeScript fixtures identical.

**Step 8: Commit**

```bash
git add internal/domain/modelhub/image_size.go internal/domain/modelhub/image_size_test.go \
  internal/domain/modelhub/capability.go internal/domain/modelhub/capability_compression_test.go \
  internal/domain/modelhub/resolver.go internal/domain/modelhub/route_model_test.go \
  web/shared/image-size.ts web/shared/image-size.contract.ts
git commit -m "feat: normalize custom image dimensions"
```

### Task 6: Persist and expose custom-size capability

**Files:**
- Modify: `internal/repository/ent/schema/modelaccountmodel.go`
- Modify: `internal/repository/ent/migrations/000001_init.sql`
- Regenerate: `internal/repository/ent/**`
- Modify: `internal/domain/modeladmin/types.go`
- Modify: `internal/service/modeladmin/service.go`
- Modify: `internal/service/modeladmin/store.go`
- Modify: `internal/repository/entstore/model_admin_store.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/service/modeladmin/service_test.go`
- Test: `internal/http/router/admin_model_api_test.go`
- Test: `internal/http/router/gallery_api_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Test: `web/shared/user-api-generation.contract.ts`

**Step 1: Write failing service/API round-trip tests**

Create/update a model with `supports_custom_size=true`, list it, build a routing snapshot, and fetch capabilities. Assert false remains the default for old payloads.

**Step 2: Verify red**

```bash
go test ./internal/service/modeladmin -run CustomSize -count=1
go test ./internal/http/router -run CustomSize -count=1
```

Expected: compile/test failure because the field is absent.

**Step 3: Add schema and domain field**

Add the Ent boolean with default false, update the baseline SQL schema, then run:

```bash
go generate ./internal/repository/ent
```

Expected: generated builders/entities/mutations contain `supports_custom_size`.

**Step 4: Thread the field through persistence and handlers**

Update write normalization, memory store, Ent store, routing snapshot, handler request/response structs, and visible capabilities.

**Step 5: Update shared frontend projection**

Add optional boolean fields to API types and normalize absent values to false.

**Step 6: Verify green**

Run focused Go tests and `npx --yes tsx web/shared/user-api-generation.contract.ts`; expected PASS.

**Step 7: Commit**

```bash
git add internal/repository/ent internal/domain/modeladmin/types.go internal/service/modeladmin \
  internal/repository/entstore/model_admin_store.go internal/http/handlers/api.go \
  internal/http/router/admin_model_api_test.go internal/http/router/gallery_api_test.go \
  web/shared/api-types.ts web/shared/user-api.ts web/shared/user-api-generation.contract.ts
git commit -m "feat: expose custom image size capability"
```

### Task 7: Add admin and workspace size controls

**Files:**
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/providerModelRows.ts`
- Test: `web/admin/src/pages/providerModelCapabilities.contract.ts`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/workspaceCreationDraft.ts`
- Test: `web/user/src/pages/workspaceParameters.contract.ts`
- Test: `web/user/src/pages/workspaceCreationDraft.contract.ts`
- Test: `web/user/src/pages/workspaceEstimate.contract.ts`

**Step 1: Write failing admin contracts**

Assert pixel mode seeds all seven presets and exposes an “允许用户自定义尺寸” checkbox whose value reaches the admin API payload.

**Step 2: Verify red, implement, verify green**

Run `npx --yes tsx web/admin/src/pages/providerModelCapabilities.contract.ts` before and after implementation.

**Step 3: Write failing workspace parameter tests**

Assert custom-enabled models expose custom mode; typed dimensions are normalized; exact explanatory copy is present; preset-only models do not expose custom inputs; ratio mode returns expected size labels for 1k/2k/4k fixtures.

**Step 4: Verify red**

Run the three user contracts; expected FAIL on missing UI/state.

**Step 5: Implement workspace controls**

Keep preset selection intact, add custom width/height state and draft persistence, send the normalized `pixel_size`, and display ratio-mode expected output through `calculateImageSizeForBaseResolution`.

**Step 6: Verify green and frontend builds**

```bash
npx --yes tsx web/user/src/pages/workspaceParameters.contract.ts
npx --yes tsx web/user/src/pages/workspaceCreationDraft.contract.ts
npx --yes tsx web/user/src/pages/workspaceEstimate.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
```

Expected: PASS.

**Step 7: Commit**

```bash
git add web/admin/src/pages/ProviderModelsPage.tsx web/admin/src/pages/providerModelRows.ts \
  web/admin/src/pages/providerModelCapabilities.contract.ts web/user/src/pages/WorkspacePage.tsx \
  web/user/src/pages/workspaceCreationDraft.ts web/user/src/pages/workspaceParameters.contract.ts \
  web/user/src/pages/workspaceCreationDraft.contract.ts web/user/src/pages/workspaceEstimate.contract.ts
git commit -m "feat: configure and preview image dimensions"
```

### Task 8: Enforce mandatory password setup in the backend

**Files:**
- Modify: `internal/service/auth/service.go`
- Modify: `internal/service/auth/store.go`
- Test: `internal/service/auth/service_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/router.go`
- Test: `internal/http/router/auth_api_test.go`
- Modify: `api/openapi/openapi.yaml`

**Step 1: Write failing auth-service tests**

Assert passwordless email-code login returns a setup grant and no session, existing password users receive a session, setup grant purpose/expiry/version are validated, successful setup issues a new session, and replay fails.

**Step 2: Verify red**

Run: `go test ./internal/service/auth -run 'PasswordSetup|EmailCodeLogin' -count=1`

Expected: FAIL because login always issues a normal session.

**Step 3: Implement purpose-bound setup grants**

Extend claims with an explicit purpose, sign normal access claims as `access`, add setup issuance/parsing with a ten-minute expiry, and expose a service operation that sets the password then issues a new session.

**Step 4: Write failing API tests**

Assert the passwordless login response contains `password_setup_required` and `password_setup_token`, does not set a refresh cookie, normal user APIs reject the setup grant, and `/password/setup` returns a session/cookie only after a valid password.

**Step 5: Implement handler and route**

Use the standard success/error envelopes, preserve signup trial idempotency, and never include a setup token in audit metadata.

**Step 6: Verify green**

```bash
go test ./internal/service/auth -count=1
go test ./internal/http/router -run Auth -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/auth internal/http/handlers/api.go internal/http/router/router.go \
  internal/http/router/auth_api_test.go api/openapi/openapi.yaml
git commit -m "feat: require password setup after code login"
```

### Task 9: Add verified password change and frontend flows

**Files:**
- Modify: `internal/service/auth/service.go`
- Test: `internal/service/auth/service_test.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/auth_api_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Modify: `web/user/src/pages/loginPresentation.ts`
- Modify: `web/user/src/pages/LoginPage.tsx`
- Modify: `web/user/src/pages/ProfilePage.tsx`
- Modify: `web/user/src/App.tsx`
- Test: `web/user/src/pages/loginPresentation.contract.ts`
- Test: `web/user/src/pages/loginPage.contract.ts`
- Create: `web/user/src/pages/profilePassword.contract.ts`
- Modify: `web/user/src/sessionLifecycle.contract.ts`

**Step 1: Write failing backend verification tests**

Assert `password_change` code is required, bound to the authenticated email, one-time, and that success increments token version and revokes every refresh session.

**Step 2: Verify red, implement, verify green**

Change the authenticated password endpoint body to `{code,new_password}`, consume the canonical user's code, clear the refresh cookie, and run focused auth/router tests.

**Step 3: Expose `has_password` and response unions**

Add failing handler/shared-type tests, then include `has_password` in profile payload and model password-setup login results in `web/shared/api-types.ts`/`user-api.ts`.

**Step 4: Add failing login state-machine tests**

Assert a setup-required result renders the password/confirmation step, does not install a session early, and installs the returned normal session after setup.

**Step 5: Implement mandatory setup UI**

Keep the setup token only in React state. Validate password/confirmation and preserve the original return route after completion.

**Step 6: Add failing profile password tests**

Assert the basic-information section contains a password entry, sends a `password_change` code, submits code/new password, and invokes application session expiration after success.

**Step 7: Implement profile dialog and local logout**

Use accessible dialog/focus behavior, explicit required labels, resend cooldown, and synchronous local session invalidation before navigation.

**Step 8: Verify frontend contracts and typecheck**

```bash
npx --yes tsx web/user/src/pages/loginPresentation.contract.ts
npx --yes tsx web/user/src/pages/loginPage.contract.ts
npx --yes tsx web/user/src/pages/profilePassword.contract.ts
npx --yes tsx web/user/src/sessionLifecycle.contract.ts
npm --prefix web/user run typecheck
```

Expected: PASS.

**Step 9: Commit**

```bash
git add internal/service/auth/service.go internal/service/auth/service_test.go \
  internal/http/handlers/api.go internal/http/router/auth_api_test.go \
  web/shared/api-types.ts web/shared/user-api.ts web/user/src/App.tsx \
  web/user/src/pages/LoginPage.tsx web/user/src/pages/loginPresentation.ts \
  web/user/src/pages/ProfilePage.tsx web/user/src/pages/profilePassword.contract.ts \
  web/user/src/pages/loginPresentation.contract.ts web/user/src/pages/loginPage.contract.ts \
  web/user/src/sessionLifecycle.contract.ts
git commit -m "feat: verify password changes by email"
```

### Task 10: Generate and integrate visual assets

**Files:**
- Local ignored: `.secrets/imagegen.env`
- Modify local exclude: `.git/info/exclude` (not committed)
- Create: `web/user/public/landing/studio-showcase-1280.webp`
- Create: `web/user/public/landing/studio-showcase-1920.webp`
- Create: `web/user/public/landing/studio-showcase-1280.avif`
- Create: `web/user/public/landing/studio-showcase-1920.avif`
- Replace: `web/user/public/landing/workspace.webp`
- Replace: `web/user/public/favicon.svg`
- Replace: `web/admin/public/favicon.svg`
- Create: `web/user/src/assets/mikiko-mark.svg`
- Modify: `web/user/src/brand.tsx`
- Modify: `web/user/src/pages/LandingPage.tsx`
- Modify: `web/user/src/pages/LoginPage.tsx`
- Modify: `web/user/index.html`
- Modify: `web/admin/index.html`
- Test: `web/user/src/pages/landingContent.contract.ts`
- Test: `web/user/src/pages/landingPage.contract.ts`

**Step 1: Configure the local-only credential**

Add `.secrets/` to `.git/info/exclude`, create a mode-600 env file containing the supplied endpoint/key/model, then prove `git status --short --ignored` does not expose a tracked candidate. Never print the file.

**Step 2: Generate a 4K master through the approved CLI/API path**

Use `gpt-image-2`, `3840x2160`, `quality=high`, and the approved no-text studio-showcase prompt. Save the master under ignored `tmp/imagegen/` and inspect it with `view_image`.

**Step 3: Iterate only if inspection fails**

Reject text artifacts, watermarks, legacy branding, poor subject relevance, or crops that cannot support desktop/mobile. Apply one targeted prompt change per retry.

**Step 4: Create optimized derivatives**

Use a structured image tool to resize and encode 1280/1920 WebP and AVIF variants. Verify dimensions, byte sizes, decode success, and meaningful nonblank pixels.

**Step 5: Capture the repaired workspace**

Run the local application with seeded test data, capture a truthful workspace screenshot, redact no secrets because test data contains none, and encode the serving asset.

**Step 6: Replace vector brand assets**

Implement a coherent Mikiko mark in SVG, derive user/admin favicon color variants, and replace the 1 MB raster brand import.

**Step 7: Add responsive picture markup**

Use `<picture>`/`srcSet` for landing and login. Preserve stable aspect-ratio containers and suitable object positions for desktop/mobile.

**Step 8: Verify contracts and build size**

```bash
npx --yes tsx web/user/src/pages/landingContent.contract.ts
npx --yes tsx web/user/src/pages/landingPage.contract.ts
npm --prefix web/user run build
find web/user/public/landing -type f -maxdepth 1 -exec ls -lh {} \;
```

Expected: contracts/build PASS; no served hero derivative is an uncompressed 4K source.

**Step 9: Commit only production assets and references**

```bash
git add web/user/public/landing web/user/public/favicon.svg web/admin/public/favicon.svg \
  web/user/src/assets/mikiko-mark.svg web/user/src/brand.tsx web/user/src/pages/LandingPage.tsx \
  web/user/src/pages/LoginPage.tsx web/user/index.html web/admin/index.html \
  web/user/src/pages/landingContent.contract.ts web/user/src/pages/landingPage.contract.ts
git commit -m "feat: replace production visual assets"
```

### Task 11: Integration, browser verification, and documentation

**Files:**
- Modify: `api/openapi/openapi.yaml`
- Modify: `web/docs/src/**` only where generated/reference contract requires
- Modify: release note history input if required by repository release workflow
- Create: browser screenshots under ignored verification output

**Step 1: Run focused integration tests**

```bash
go test ./internal/service/auth ./internal/service/cashier ./internal/domain/modelhub ./internal/service/modeladmin -count=1
go test ./internal/http/router -run 'Auth|Cashier|Model|Capabilities' -count=1
```

Expected: PASS.

**Step 2: Run frontend checks**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 3: Start the local stack and run Playwright checks**

Verify desktop/mobile checkout popup dispatch, order polling, unified image detail/zoom, preset/custom/ratio sizes, mandatory password setup, profile password change/session expiry, landing/login crops, and both favicons. Capture screenshots and assert important overlays do not overlap.

**Step 4: Run API smoke**

Run: `./scripts/workflow/api-smoke.sh`

Expected: PASS with isolated PostgreSQL/Redis/API/Worker cleanup.

**Step 5: Run full verification**

Run: `./scripts/workflow/verify.sh`

Expected: `OK: verification passed`.

**Step 6: Commit integration/doc corrections**

Stage only deliberate files, inspect `git diff --cached --check`, and commit with a scoped message.

### Task 12: Review, PR, merge, and release

**Files:**
- Generated: `.review/gate.json`
- Update: release notes/history according to `.github` release workflow

**Step 1: Review the committed scope**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: PASS marker bound to the current HEAD tree.

**Step 2: Inspect and fix review findings**

Perform a requirement-by-requirement audit. For every finding, add a failing regression test, implement the fix, rerun focused/full verification, commit, and regenerate the review marker.

**Step 3: Push and create a ready PR**

```bash
git push -u origin codex/v005-production-remediation
gh pr create --base main --head codex/v005-production-remediation --title "fix: remediate v0.0.5 production issues" --body-file <prepared-pr-body>
```

Expected: ready PR targeting `main` with tests and visual evidence summarized.

**Step 4: Wait for CI and merge**

Use `gh pr checks --watch` and inspect any failure before fixing. Merge only after required checks and review succeed.

**Step 5: Tag the next version**

Determine the next unused semver tag after merge, render Chinese release notes from the repository template, create an annotated tag on the merged `main`, and push it.

**Step 6: Verify GitHub Actions and Release artifacts**

Watch the tag workflow to completion. Confirm the Release includes ctl, API, worker, three frontend packages, manifest/checksums, Docker latest/version images, and templated Chinese notes with features, fixes, optimizations, install, and upgrade sections.

**Step 7: Complete the goal only after evidence audit**

Cross-check all numbered requirements against code, tests, screenshots, PR state, workflow run, tag, and Release. Mark the goal complete only when every item has authoritative evidence.
