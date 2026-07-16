# User Experience Compatibility Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restore the latest Luminous Vault user application from `0ea714a` while keeping the current backend generation contract and all current operational safeguards.

**Architecture:** Treat `0ea714a:web/user` as the final UI source snapshot, but use a three-way compatibility layer for shared API code and types. Keep unsupported generation capabilities out of the rendered controls, preserve the embedded docs route, and verify both user behavior and admin/auth regressions before delivery.

**Tech Stack:** React 19, TypeScript, Vite, Tailwind CSS 4, GSAP, Lucide React, Go HTTP APIs, Docker Compose, Nginx.

---

### Task 1: Establish A Failing Contract Baseline

**Files:**
- Create from `0ea714a`: `web/user/src/**/*.contract.ts` files absent from the current branch
- Create from `0ea714a`: `scripts/workflow/contracts/luminous-vault-css.mjs`
- Preserve: `scripts/workflow/verify-contracts.sh`

**Step 1: Record the current contract inventory**

Run: `find web/user/src web/shared web/admin/src -name '*.contract.ts' | sort`

Expected: the current branch has fewer user contracts than `0ea714a`.

**Step 2: Import only missing or changed user contract files**

Use `git diff --name-only HEAD..0ea714a -- 'web/user/src/**/*.contract.ts'` to identify the exact set, then restore each path from `0ea714a` without importing implementation files.

**Step 3: Run contracts to verify the expected red state**

Run: `./scripts/workflow/verify-contracts.sh`

Expected: FAIL on missing new models, styles, route behavior, or generation types. Record the first failures before implementation.

### Task 2: Restore Luminous Vault Foundations

**Files:**
- Modify: `web/user/package.json`
- Modify: `web/user/package-lock.json`
- Modify: `web/shared/tokens.css`
- Modify: `web/shared/user-theme.css`
- Modify: `web/user/src/styles.css`
- Create: `web/user/src/ui/icons.ts`
- Create: `web/user/src/ui/luminousVault.ts`
- Create: `web/user/src/ui/motion.ts`
- Create: `web/user/src/ui/useReveal.ts`
- Modify: `web/user/src/ui/classes.ts`
- Modify: `web/user/src/ui/redesign-classes.ts`

**Step 1: Add the required dependencies**

Restore the final dependency declarations and lockfile entries for `@gsap/react`, `gsap`, and `lucide-react` from `0ea714a`.

**Step 2: Restore theme and motion tokens**

Bring the final user token/theme CSS and UI utility modules from `0ea714a`. Preserve unrelated shared CSS behavior and ensure `--pg-ease-in-out`, `--pg-ease-spring`, `--pg-duration-instant`, and `--lv-accent-contrast` exist.

**Step 3: Run foundation contracts**

Run: `node scripts/workflow/contracts/luminous-vault-css.mjs && npm --prefix web/user run typecheck`

Expected: CSS contract passes; typecheck may still fail only on not-yet-restored page modules.

**Step 4: Commit the foundation**

Run: `git commit -m "feat(user): restore luminous vault foundations"`

### Task 3: Restore Landing, Login, Routing, And Shell

**Files:**
- Create: `web/user/public/landing/hero-gallery.webp`
- Create: `web/user/public/landing/workspace.webp`
- Modify: `web/user/src/App.tsx`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/routeState.ts`
- Modify: `web/user/src/shellLayout.ts`
- Modify: `web/user/src/types.ts`
- Modify: `web/user/src/pages/LandingPage.tsx`
- Modify: `web/user/src/pages/LoginPage.tsx`
- Modify: `web/user/src/pages/loginCopy.ts`
- Create: `web/user/src/pages/landingContent.ts`
- Create: `web/user/src/pages/loginPresentation.ts`
- Create: `web/user/src/ui/useLandingMotion.ts`
- Create: `web/user/src/ui/focusTrap.ts`
- Create: `web/user/src/ui/overlayPortal.tsx`

**Step 1: Restore the page and model implementations**

Copy the final implementations from `0ea714a`, including lazy-loaded landing and `task_id` route restoration.

**Step 2: Preserve current authentication semantics**

Keep the existing user API configuration and shared refresh behavior. Adapt default profile preferences to the reconciled type instead of weakening HTTP client handling.

**Step 3: Preserve embedded documentation**

Do not import `docsUrl.ts` or external-doc behavior. Keep `DocsPage.tsx` rendered inside the shell and retain the current documentation data flow.

**Step 4: Run focused contracts**

Run: `npm exec --prefix web/user -- tsx web/user/src/pages/loginPresentation.contract.ts`

Run: `npm exec --prefix web/user -- tsx web/user/src/pages/loginPage.contract.ts`

Run: `npm exec --prefix web/user -- tsx web/user/src/routeState.contract.ts`

Run: `npm exec --prefix web/user -- tsx web/user/src/shellLayout.contract.ts`

Expected: PASS.

### Task 4: Restore Home, Gallery, And Account Experiences

**Files:**
- Modify: `web/user/src/pages/HomePage.tsx`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Modify: `web/user/src/pages/CheckoutPage.tsx`
- Modify: `web/user/src/pages/ApiKeysPage.tsx`
- Modify: `web/user/src/pages/ProfilePage.tsx`
- Modify: `web/user/src/pages/SettingsPage.tsx`
- Create/modify: `web/user/src/pages/home*.ts`
- Create/modify: `web/user/src/pages/gallery*.ts`
- Create/modify: `web/user/src/pages/publicGallery*.ts`
- Create: `web/user/src/ui/SettingsWorkspace.tsx`
- Create: `web/user/src/ui/imageMediaModel.ts`
- Create: `web/user/src/ui/zoomPointer.ts`

**Step 1: Restore final page and view-model sources from `0ea714a`**

Keep current API calls where the target branch assumes a newer backend response. Reconcile compile errors in shared types rather than replacing current admin or artifact fields.

**Step 2: Run page-model contracts**

Run: `./scripts/workflow/verify-contracts.sh`

Expected: all non-workspace contracts pass; any remaining failures are isolated to generation compatibility.

**Step 3: Commit restored user surfaces**

Run: `git commit -m "feat(user): restore latest application experiences"`

### Task 5: Reconcile Workspace And Shared API Contracts

**Files:**
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Create/modify: `web/user/src/pages/workspace*.ts`
- Create: `web/user/src/pages/WorkspaceStatusRail.tsx`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Modify: `web/shared/image-size.ts`
- Modify: `web/shared/image-size.contract.ts`
- Modify: `web/shared/mock-api.ts`
- Modify: `web/shared/mock-data.ts`
- Modify if required: `web/shared/open-api.ts`
- Preserve: `web/shared/http-client.ts`
- Preserve: `web/shared/admin-api.ts`
- Preserve: `web/shared/admin-session.contract.ts`

**Step 1: Make generation model contracts fail for the current adapter**

Run the imported `workspaceParameters`, `workspaceEstimate`, `workspaceViewModel`, task history, task progress, task failure, and reference-limit contracts individually.

Expected: FAIL until the compatibility types and mappings exist.

**Step 2: Add a bidirectional compatibility model**

Keep `BackendEstimateRequest` and task creation payloads on `requested_quality` and `requested_size`. Normalize current capability/task responses into the fields consumed by the workspace. Add aliases only where they remove duplicated translation.

**Step 3: Remove unsupported controls**

Do not render or submit output format, compression, moderation, free-pixel, or other controls absent from the current capability response. Estimate and create must derive from one normalized state.

**Step 4: Verify serialized requests**

Add or extend a shared/user contract that captures estimate and create requests and asserts the presence of `requested_quality` and `requested_size` and the absence of unsupported fields.

Run: `npm exec --prefix web/user -- tsx <compatibility-contract-path>`

Expected: PASS.

**Step 5: Run all frontend contracts and builds**

Run: `./scripts/workflow/verify-contracts.sh`

Run: `npm --prefix web/user run typecheck && npm --prefix web/user run build`

Expected: PASS.

**Step 6: Commit workspace compatibility**

Run: `git commit -m "feat(user): align workspace with current generation API"`

### Task 6: Wire Landing Build And User-Web Image

**Files:**
- Create: `scripts/workflow/contracts/landing-build.mjs`
- Modify: `web/user/package.json`
- Modify: `Dockerfile.user-web`
- Preserve: `web/user/public/env.js`
- Preserve: `deployments/nginx/40-render-user-env.sh` unless used by the final UI

**Step 1: Add the landing chunk build contract**

Make the user build run `landing-build.mjs` and copy that script into the Docker build stage.

**Step 2: Build locally and in Docker**

Run: `npm --prefix web/user run build`

Run: `docker build -f Dockerfile.user-web -t pic-gallery-user-web:compat .`

Expected: both builds pass and a lazy `LandingPage-*` chunk contains `ScrollTrigger` while the entry chunk does not.

**Step 3: Commit contract wiring**

Run: `git commit -m "chore(user): verify landing build and deployment"`

### Task 7: Full Verification And Runtime Acceptance

**Files:**
- Modify only for defects found during verification.
- Create screenshots under a temporary directory outside tracked source.

**Step 1: Run repository verification**

Run: `./scripts/workflow/verify.sh`

Expected: Go tests/vet and both frontend typechecks/builds pass.

**Step 2: Rebuild the development stack**

Run the repository's documented full Docker rebuild command and wait for all health checks.

Expected: API, worker, user web, admin web, database, and supporting services are healthy.

**Step 3: Run real API smoke**

Run: `BASE_URL=http://localhost:8080 ./scripts/workflow/api-smoke.sh`

Extend with authenticated capability, estimate, create, and task-detail requests using the current backend fields.

Expected: all requests succeed and no provider request uses unsupported fields.

**Step 4: Run browser acceptance**

Check 1440x900, 390x844, and 320x700 viewports in light and dark themes. Exercise landing, login tabs and validation, protected-route return, workspace controls and task continuation, gallery detail/lightbox, responsive navigation, and admin expired-token behavior.

Expected: no console errors, request loops, horizontal overflow, blank media, inaccessible hidden controls, or incoherent overlap.

### Task 8: Review, Push, And PR

**Files:**
- Fix: `docs/plans/2026-07-17-admin-session-refresh-loop.md` only if `git diff --check` confirms the known trailing blank-line issue.
- Generate: `.review/gate.json`

**Step 1: Inspect branch scope and secrets**

Run: `git diff --check && git status --short && git diff --stat main...HEAD`

Expected: clean tracked worktree and no whitespace errors or generated evidence in source.

**Step 2: Run an independent code review and fix findings**

Review shared authentication, generation serialization, responsive UI, and deployment changes against the design document.

**Step 3: Generate the committed review gate**

Run: `./scripts/workflow/review-local.sh --scope committed`

Run: `./scripts/workflow/check-review-gate.sh`

Expected: PASS with current HEAD tree SHA.

**Step 4: Push and create the PR**

Run: `git push -u origin codex/artifact-recovery-storage-unification`

Create one PR to `main` with a structured description covering operational safeguards, restored user experience, compatibility mapping, and exact verification evidence.
