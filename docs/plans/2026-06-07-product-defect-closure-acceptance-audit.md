# Pic Gallery Product Defect Closure Acceptance Audit

> Date: 2026-06-07
> Scope: `docs/plans/2026-06-05-product-defect-closure-technical-design.md`
> Status: code verification passed; push/PR readiness blocked by heavyweight approval and Docker daemon availability.

## Summary

The main product closure modules are implemented in the current worktree: signup trial credits, wallet buckets, checkout/cashier, payment provider adapters, public gallery, OpenAPI docs, admin readiness, admin user detail, call record preflight failures, and permission facade.

Repository-level verification has passed:

- `./scripts/workflow/verify.sh`
- `./scripts/workflow/api-smoke.sh`
- `bash -n scripts/test/api_contract_smoke.sh scripts/e2e/run-docker-e2e.sh`
- `node --check scripts/e2e/docker-e2e.mjs`
- `docker compose -f deployments/docker-compose/docker-compose.e2e.yml config`
- `git diff --check`

Known non-code blockers:

- `./scripts/workflow/review-local.sh --scope working` is `BLOCK` because `.coding-context.json` has `approval.status=pending` for a heavyweight task.
- Actual Docker E2E cannot run until Docker daemon is available on this machine.

## P0 Acceptance

| Requirement | Current evidence | Status |
|---|---|---|
| New email-code user receives trial credits; profile shows amount, expiry, and warning. | `TestEmailCodeLoginGrantsSignupTrialOnlyForNewUser`, `TestEmailCodeLoginUsesAdminSignupTrialConfig`, `TestEmailCodeLoginUsesAdminSignupTrialExpiryReminderDays`, `TestBillingStoreSignupTrialExpiryWarningUsesGrantReminderDays`; API smoke validates `signup_grant`, `trial` bucket, and `trial_grant` ledger. | Passed |
| Admin can configure signup trial amount, validity, enablement, and expiry reminder threshold. | Admin config tab integration in `cashierTrialConfig`; readiness check `signup_trial`; router tests for admin-config-driven signup trial. | Passed |
| New user can generate first image when route model and price exist; no model disables generation with clear message. | Worker/API smoke covers configured fake provider generation; `workspaceGenerateReadiness.contract.ts` and browser evidence cover no-model disabled state. | Passed |
| User can enter checkout from profile, buy fixed package or custom amount, payment credits recharge bucket, ledger is visible. | `TestCashierMockPaymentCreditsRechargeBucket`, `TestCashierCustomAmountUsesAdminConfig`, `api_contract_smoke.sh` checkout/custom amount/order ledger assertions; profile bucket browser evidence. | Passed |
| Test env can use Mock payment; online config supports Alipay/WeChat sandbox verification. | Mock flow in API smoke; Alipay/WxPay payment display, webhook, query, refund tests; provider instance config UI and redaction tests. | Passed |
| Public gallery: guest list, login detail/full prompt/like/favorite/generate same. | `TestGalleryPublishReviewAndPublicListFlow`, public gallery contracts, browser guest evidence; `query` search now server-side and keeps prompt redaction. | Passed |
| Docs page renders OpenAPI endpoints, errors, and examples correctly. | `TestDocsEndpointsReturnStructuredContract`, `open-api-docs.contract.ts`, `TestOpenAPISpecCoversP0Paths`, API smoke docs checks. | Passed |
| Admin home/readiness shows model account, route model, price, payment, public gallery, and docs checks. | `TestAdminReadinessEndpointReturnsOperationalChecks`, readiness contracts, browser readiness evidence. | Passed |

## P1 Acceptance

| Requirement | Current evidence | Status |
|---|---|---|
| Admin cashier page manages provider instances, visible methods, fixed packages, custom amount, orders, webhook events. | `CashierPage.tsx` six-tab layout; `TestAdminCashierReadEndpoints`, `TestAdminCashierPlanCreateAndUpdate`, `TestAdminCashierProviderInstanceCreateAndUpdate`, webhook retry/list smoke. | Passed |
| Admin user detail shows balance buckets, ledger, orders, tasks, API keys, and admin point adjustments. | `api_contract_smoke.sh` admin user detail and point adjustment assertions; browser admin user detail evidence. | Passed |
| Call records persist preflight failures: no model, no price, insufficient points, provider unavailable. | `TestCreateTaskPersistsFailedCallRecordWhenRoutePriceMissing`, `TestCreateTaskPersistsFailedCallRecordWhenRouteHasNoCandidate`, `TestCreateTaskPersistsFailedCallRecordWhenReserveRejectsInsufficientBalance`, admin call record smoke. | Passed |
| Review queue supports approve, reject, unpublish, and audit. | `TestGalleryPublishReviewAndPublicListFlow`, `TestGalleryPublishRejectedByModeration`, review row contracts, audit service integration. | Passed |

## P2 Acceptance

| Requirement | Current evidence | Status |
|---|---|---|
| Admin authorization goes through permission facade, currently `super_admin` / `admin`, no scattered naked admin checks. | `TestAdminHandlersUsePermissionFacadeContract`, `TestRolePermissionResolver`, `TestAdminLoginReturnsPermissions`, admin permission API smoke. | Passed |
| Payment adapters share a unified cashier contract; Alipay, WeChat, EasyPay, JeePay, and Mock support order, callback/query/refund or test fulfillment. | `internal/service/cashier` payment/query/refund registries and provider tests; `TestCashier*Webhook*`, `TestAdminCashierOrderSync*`, `TestAdminCashierOrderRefundCalls*Provider`; API smoke cashier flows. | Passed |
| E2E covers admin config -> signup -> trial credits -> generation -> public review -> gallery -> checkout -> API docs. | API smoke and browser evidence cover the chain in local runtime; Docker E2E script is updated and syntactically valid, compose config parses. | Partially blocked by Docker daemon |

## Current Blockers

1. Heavyweight approval is pending.
   - Evidence: `.review/gate.json` decision is `BLOCK`.
   - Required action: set `.coding-context.json` approval status to approved through the intended workflow, then rerun review gate.

2. Docker daemon is not running.
   - Evidence: `docker info` fails with `Cannot connect to the Docker daemon`.
   - Required action: start Docker Desktop/daemon, then run `scripts/e2e/run-docker-e2e.sh --start`.

## Recommended Next Steps

1. Get heavyweight approval.
2. Start Docker and run Docker E2E.
3. Rerun:
   - `./scripts/workflow/verify.sh`
   - `./scripts/workflow/api-smoke.sh`
   - `./scripts/workflow/review-local.sh --scope working`
4. After approval and Docker E2E pass, prepare the commit and push.

## Update: 2026-06-07 Cashier CRUD Closure

The remaining admin cashier CRUD wording gap has been closed in the current worktree:

- `DELETE /api/ops/admin/v1/cashier/plans/{plan_id}` archives a cashier plan, sets `purchase_enabled=false`, keeps historical order references intact, writes audit `cashier.plan.delete`, and returns the archived plan snapshot.
- `DELETE /api/ops/admin/v1/cashier/provider-instances/{instance_id}` deletes a payment provider instance, writes audit `cashier.provider.delete`, and returns the deleted instance snapshot.
- Admin `/#/cashier` now exposes delete actions for recharge plans and provider instances with confirmation prompts.
- OpenAPI documents both delete operations.

Verification rerun after this closure:

- `go test ./internal/service/cashier ./internal/repository/entstore ./internal/http/router -run 'TestConfigFacadeReadsAndWritesCashierRuntimeConfig|TestCashierStorePersistsProviderInstancesInDedicatedTable|TestAdminCashierPlanCreateAndUpdate|TestAdminCashierProviderInstanceCreateAndUpdate' -count=1`
- `go test ./api/openapi -run TestOpenAPISpec -count=1`
- `npm --prefix web/admin run typecheck`
- `npm --prefix web/admin run build`
- `git diff --check`
- `./scripts/workflow/api-smoke.sh`
- `./scripts/workflow/verify.sh`

Remaining blockers are unchanged: heavyweight approval is still pending, and actual Docker E2E still requires a running Docker daemon.

## Update: 2026-06-07 Docker E2E Closure

Docker E2E is now executable and passed on the local Docker Desktop stack.

Changes made for repeatable E2E setup:

- `scripts/e2e/docker-e2e.mjs` now starts a local fake OpenRouter-compatible provider and seeds the runtime model route chain before user image task creation:
  - model account
  - account model
  - public `basic` route model
  - route model candidate
  - `text_to_image` / `1k` route price
- The E2E flow explicitly restores the visible Mock cashier payment method before checkout validation, so reruns are not affected by prior sweep/config mutations.
- The OpenAPI route sweep now sends a valid custom amount config body and preserves Mock visible methods instead of pushing empty config payloads.

Verification evidence:

- `node --check scripts/e2e/docker-e2e.mjs`
- `scripts/e2e/run-docker-e2e.sh --start`
  - Report: `tmp/e2e/latest-report.md`
  - Status: `PASS`
  - Passed through signup, trial credits, checkout, API keys, seeded generation route, agent image task creation, native Open API task creation, OpenAI-compatible generation, admin management, OpenAPI route sweep, and admin logout coverage.
  - Only warnings were expected missing-resource preconditions for synthetic sweep paths.
- `./scripts/workflow/verify.sh`
- `./scripts/workflow/api-smoke.sh`

Remaining blocker:

- None from the local review gate. The current `.coding-context.json` track is lightweight and approval is not required.

## Update: 2026-06-07 Current Worktree Verification

The current worktree was reverified after the latest public gallery direct-detail and admin cashier order filter changes.

Verification evidence:

- `./scripts/workflow/verify.sh`
  - Go tests passed.
  - `go vet` passed.
  - Frontend/shared contract verification passed.
  - `web/user` typecheck and production build passed.
  - `web/admin` typecheck and production build passed.
- `./scripts/workflow/api-smoke.sh`
  - Passed against the temporary local API runtime at `http://127.0.0.1:65246`.
- `./scripts/workflow/review-local.sh --scope working`
  - `PASS`.
- `./scripts/workflow/review-local.sh --scope committed && ./scripts/workflow/check-review-gate.sh`
  - `PASS` and `review gate: OK`.

Current blocker:

- None known from verify, API smoke, or review gate.
