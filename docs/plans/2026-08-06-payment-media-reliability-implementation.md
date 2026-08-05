# Payment And Media Reliability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make payment completion lossless and idempotent, move media actions to stable resource IDs with refreshable cached object URLs, enforce a dynamic 20 MB image policy, and eliminate gallery-wide refreshes.

**Architecture:** A shared cashier reconciliation path owns provider query, close, completion, and cancellation transitions. Media APIs authorize stable resource IDs and project short-lived time-bucketed URLs, while storage imports use backend copy capabilities. A typed attachment-policy resolver feeds backend validation, capabilities, and the admin/user interfaces; React surfaces consume shared status and media helpers and patch entities locally.

**Tech Stack:** Go 1.24, net/http, Ent/PostgreSQL, S3-compatible SigV4 storage, React 19, TypeScript, Vite, repository contract tests.

---

### Task 1: Normalize JeePay Notifications

**Files:**
- Create: `internal/service/cashier/jeepay_notification.go`
- Create: `internal/service/cashier/jeepay_notification_test.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/cashier_api_test.go`

**Step 1: Write failing parser tests**

Cover JSON and form bodies producing the same canonical values, incorrect content type fallback, empty/non-object JSON, oversize body, and signing-field preservation.

```go
func TestParseJeePayNotificationJSON(t *testing.T) {
    got, err := ParseJeePayNotification([]byte(`{"mchNo":"M1","state":2,"amount":990,"sign":"ABC"}`), "application/json")
    require.NoError(t, err)
    require.Equal(t, "2", got.Get("state"))
    require.Equal(t, "990", got.Get("amount"))
}
```

**Step 2: Run the focused tests and confirm failure**

Run: `go test ./internal/service/cashier -run JeePayNotification -count=1`

Expected: FAIL because the parser does not exist.

**Step 3: Implement canonical parsing**

Decode a bounded JSON object with `json.Decoder.UseNumber`, stringify scalar values without float conversion, or parse URL-encoded form values. Reject arrays, nested values, duplicate ambiguous fields, and empty bodies.

**Step 4: Route the webhook through the parser**

Replace direct `url.ParseQuery` use in `handleJeePayWebhook`; preserve merchant lookup, canonical signature verification, amount conversion, and `success` acknowledgement.

**Step 5: Add route-level JSON callback tests**

Verify a signed JSON notification completes a recharge order and duplicate JSON/form notifications return success without duplicate credits.

**Step 6: Run focused tests**

Run: `go test ./internal/service/cashier ./internal/http/router -run 'JeePay.*Webhook|JeePayNotification' -count=1`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/cashier/jeepay_notification.go internal/service/cashier/jeepay_notification_test.go internal/http/handlers/api.go internal/http/router/cashier_api_test.go
git commit -m "fix: accept official Jeepay payment notifications"
```

### Task 2: Make Paid Reconciliation Recoverable And Idempotent

**Files:**
- Modify: `internal/service/billing/store.go`
- Modify: `internal/service/billing/service_test.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Modify: `internal/repository/entstore/billing_store_test.go`
- Modify: `internal/domain/billing/types.go`

**Step 1: Write failing store-contract tests**

Exercise `pending`, `canceled`, `expired`, and `failed` cashier recharge orders receiving a valid paid result. Assert one completed order, one recharge grant, one ledger entry, and preserved previous-state reconciliation metadata. Assert refunded/chargeback states reject recovery.

**Step 2: Run focused tests and confirm the canceled recovery fails**

Run: `go test ./internal/service/billing ./internal/repository/entstore -run 'MarkOrderPaid|CompleteRecharge|PaidRecovery' -count=1`

**Step 3: Define one transition policy**

Add helpers equivalent to:

```go
func cashierOrderCanRecoverPaid(status string) bool {
    switch normalize(status) {
    case "pending", "canceled", "expired", "failed":
        return true
    default:
        return false
    }
}
```

Use the same policy in MemoryStore and BillingStore. Completed orders with matching provider binding and trade information are idempotent success.

**Step 4: Make completion transactional**

Within the existing serializable transaction, conditionally complete the order, insert the recharge grant/ledger exactly once, and record previous status plus reconciliation source. Do not permit refunded or chargeback recovery.

**Step 5: Add concurrency tests**

Race cancel and paid reconciliation in the Ent store and assert the final state is completed with one credit.

**Step 6: Run focused tests**

Run: `go test -race ./internal/service/billing ./internal/repository/entstore -run 'Paid|Cancel' -count=1`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/domain/billing/types.go internal/service/billing/store.go internal/service/billing/service_test.go internal/repository/entstore/billing_store.go internal/repository/entstore/billing_store_test.go
git commit -m "fix: make payment success win reconciliation races"
```

### Task 3: Add Provider Close Adapters And Safe Cancellation

**Files:**
- Create: `internal/service/cashier/close_adapter.go`
- Create: `internal/service/cashier/close_adapter_test.go`
- Create: `internal/service/cashier/close_provider.go`
- Create: `internal/service/cashier/close_provider_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/cashier_api_test.go`

**Step 1: Write failing adapter contract tests**

Cover JeePay `/api/pay/close`, Alipay `alipay.trade.close`, WeChat `/v3/pay/transactions/out-trade-no/{order}/close`, Stripe PaymentIntent cancellation, EasyPay configured close, and EasyPay unsupported close.

**Step 2: Run tests and confirm the registry is missing**

Run: `go test ./internal/service/cashier -run 'ClosePayment|CloseOrder' -count=1`

**Step 3: Implement a close registry**

Return a typed result with `Closed`, `AlreadyPaid`, `Unsupported`, `OutcomeUncertain`, provider status, and sanitized raw diagnostics. Reuse existing signing and bounded HTTP helpers.

**Step 4: Replace local-only cancel orchestration**

The user cancel handler loads the order and exact provider instance, queries first, reconciles paid results, accepts already-closed results, closes pending orders, and only then invokes the store's conditional local cancel. Uncertain/unsupported results return conflict and keep the order pending.

**Step 5: Add cancellation integration tests**

Cover paid-before-cancel, paid-during-cancel, close success, close uncertainty, unsupported EasyPay, uninitialized local cancel, and idempotent repeated cancel.

**Step 6: Run focused tests**

Run: `go test ./internal/service/cashier ./internal/http/router -run 'Cashier.*Cancel|ClosePayment' -count=1`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/cashier/close_* internal/http/handlers/api.go internal/http/router/cashier_api_test.go
git commit -m "fix: confirm provider state before canceling orders"
```

### Task 4: Add User Order Sync

**Files:**
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/cashier_api_test.go`
- Modify: `api/openapi/openapi.yaml`
- Modify: `web/shared/user-api.ts`
- Modify: `web/shared/api-types.ts`

**Step 1: Add failing route tests**

Assert owner authorization, cross-user denial, paid provider reconciliation, pending response, amount mismatch rejection, missing provider instance, and per-order throttling.

**Step 2: Run route tests and confirm 404/method failure**

Run: `go test ./internal/http/router -run CashierOrderSync -count=1`

**Step 3: Implement `POST /orders/{id}/sync`**

Reuse the shared query registry and paid reconciliation path. Return `{order, sync}` and keep raw provider payload out of the user response. Add bounded per-order synchronization using a small in-process singleflight/throttle helper.

**Step 4: Add API types and client method**

Expose `syncCashierOrder(id)` with a narrow typed response. Document the endpoint and errors in OpenAPI.

**Step 5: Run focused tests and typecheck**

Run: `go test ./internal/http/router -run CashierOrderSync -count=1 && npm --prefix web/user run typecheck`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/http/router/router.go internal/http/handlers/api.go internal/http/router/cashier_api_test.go api/openapi/openapi.yaml web/shared/user-api.ts web/shared/api-types.ts
git commit -m "feat: reconcile cashier orders from the user session"
```

### Task 5: Split Payment Monitoring From Order Details

**Files:**
- Create: `web/user/src/pages/PaymentMonitorModal.tsx`
- Create: `web/user/src/pages/PaymentOrderDetailModal.tsx`
- Create: `web/user/src/pages/paymentMonitor.contract.ts`
- Modify: `web/user/src/pages/CheckoutPage.tsx`
- Modify: `web/user/src/pages/checkoutOrderState.ts`
- Modify: `web/user/src/pages/checkoutOrderState.contract.ts`

**Step 1: Write failing frontend contracts**

Assert distinct modal components, success-only auto-close, no terminal timer for canceled/failed/expired, focus-triggered sync, compact monitor content, and non-polling historical detail.

**Step 2: Run contracts and confirm failure**

Run: `npm exec --prefix web/user -- tsx web/user/src/pages/paymentMonitor.contract.ts`

**Step 3: Implement explicit modal state**

Keep separate `monitorOrder` and `detailOrder` state. The monitor polls/syncs and owns the reserved payment window. The detail dialog is opened from recent orders and never starts timers.

**Step 4: Correct auto-close behavior**

Start a three-second timer only on a transition into `success`; clear it on close/unmount/order change. Refresh account and recent orders after success.

**Step 5: Implement focus sync and cancellation feedback**

Sync immediately on `window.focus`, use the server response as authoritative, and keep the monitor open when cancellation is rejected.

**Step 6: Run contracts, typecheck, and build**

Run: `npm exec --prefix web/user -- tsx web/user/src/pages/paymentMonitor.contract.ts && npm --prefix web/user run typecheck && npm --prefix web/user run build`

Expected: PASS.

**Step 7: Commit**

```bash
git add web/user/src/pages/PaymentMonitorModal.tsx web/user/src/pages/PaymentOrderDetailModal.tsx web/user/src/pages/paymentMonitor.contract.ts web/user/src/pages/CheckoutPage.tsx web/user/src/pages/checkoutOrderState.ts web/user/src/pages/checkoutOrderState.contract.ts
git commit -m "fix: separate payment monitoring from order details"
```

### Task 6: Introduce Dynamic Attachment Policy

**Files:**
- Create: `internal/service/assets/policy.go`
- Create: `internal/service/assets/policy_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/load.go`
- Modify: `internal/config/load_test.go`
- Modify: `internal/service/adminconfig/service.go`
- Modify: `internal/service/adminconfig/service_test.go`
- Modify: `internal/service/capabilities/service.go`
- Modify: `internal/service/capabilities/service_test.go`

**Step 1: Write failing policy tests**

Cover the 20 MB default, format alias normalization, SVG rejection, invalid sizes, persisted override resolution, and invalidation after config update.

**Step 2: Run tests and confirm current 10 MB/static behavior**

Run: `go test ./internal/config ./internal/service/assets ./internal/service/adminconfig ./internal/service/capabilities -run 'Attachment|ReferenceImageMax' -count=1`

**Step 3: Add typed defaults and resolver**

Define all eight attachment fields. Resolve persisted `attachment_policy` items over config defaults, normalize allowed image MIME types, and expose immutable snapshots to request handlers.

**Step 4: Wire admin config and capabilities**

Add the config tab/items and return `reference_image_allowed_formats` plus current max bytes/MB to the user capabilities payload.

**Step 5: Run focused tests**

Run: `go test ./internal/config ./internal/service/assets ./internal/service/adminconfig ./internal/service/capabilities -run 'Attachment|ReferenceImageMax' -count=1`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/config internal/service/assets/policy* internal/service/adminconfig internal/service/capabilities
git commit -m "feat: configure attachment policy at runtime"
```

### Task 7: Enforce Bounded Image Uploads

**Files:**
- Modify: `internal/service/assets/service.go`
- Modify: `internal/service/assets/service_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/assets_api_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Write failing validation tests**

Cover max plus one byte, declared/actual MIME mismatch, allowed PNG/JPEG/WebP/GIF, rejected SVG, disallowed configured format, and dynamic policy changes without restart.

**Step 2: Run focused tests and confirm static/full-read behavior**

Run: `go test ./internal/service/assets ./internal/http/router -run 'ReferenceAsset|UploadPolicy' -count=1`

**Step 3: Implement bounded reads and content detection**

Read `maxBytes + 1`, reject overflow before allocation grows further, use `http.DetectContentType`, decode dimensions, register WebP decoding, and check the current policy.

**Step 4: Run focused tests**

Run: `go test ./internal/service/assets ./internal/http/router -run 'ReferenceAsset|UploadPolicy' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add go.mod go.sum internal/service/assets/service.go internal/service/assets/service_test.go internal/http/handlers/api.go internal/http/router/assets_api_test.go
git commit -m "fix: enforce current image attachment limits"
```

### Task 8: Add Stable Media Access And Deterministic Preview URLs

**Files:**
- Modify: `internal/storage/backend.go`
- Modify: `internal/storage/backend_test.go`
- Modify: `internal/service/assets/service.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/router/tasks_api_test.go`
- Modify: `internal/http/router/assets_api_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`

**Step 1: Write failing signing and route tests**

Assert same preview URL within a bucket, changed URL in the next bucket, sufficient remaining validity, separate download disposition, owner/public authorization, expiry metadata, and no application byte proxying.

**Step 2: Run tests and confirm per-request signatures differ**

Run: `go test ./internal/storage ./internal/http/router -run 'TemporaryMedia|MediaAccess' -count=1`

**Step 3: Implement bucketed preview signing**

Use a deterministic bucket start for preview URLs, sign beyond the bucket boundary by a refresh margin, and include a private cache-control response override. Keep downloads freshly signed and separately dispositioned.

**Step 4: Add access endpoints**

Add authenticated image/reference access projections by stable ID and purpose. Return `url` and `expires_at`; authorize private ownership and approved-public access.

**Step 5: Add typed frontend clients**

Expose `refreshImageAccess` and `refreshReferenceAssetAccess` without leaking bearer tokens into object URLs.

**Step 6: Run focused tests and typecheck**

Run: `go test ./internal/storage ./internal/http/router -run 'TemporaryMedia|MediaAccess' -count=1 && npm --prefix web/user run typecheck`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/storage internal/service/assets/service.go internal/service/imagetask/service.go internal/http web/shared
git commit -m "feat: refresh stable media access URLs"
```

### Task 9: Import Gallery Images With Backend Copy

**Files:**
- Modify: `internal/storage/backend.go`
- Modify: `internal/storage/backend_test.go`
- Modify: `internal/service/assets/service.go`
- Modify: `internal/service/assets/service_test.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/assets_api_test.go`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/workspaceCreationDraft.contract.ts`

**Step 1: Write failing copy/import tests**

Assert local same-backend copy, S3 CopyObject signing, distinct destination ownership, bounded cross-backend fallback, and source deletion independence.

**Step 2: Run focused tests and confirm import reads bytes through the API**

Run: `go test ./internal/storage ./internal/service/assets ./internal/http/router -run 'Copy|ImportFromGallery' -count=1`

**Step 3: Add optional backend copy capability**

Implement copy for Local and S3 backends. Add an asset import service that chooses server-side copy only when the resolved backend configuration matches, otherwise performs a policy-bounded transfer.

**Step 4: Change workspace edit actions to image IDs**

Change `onUseReference` from `(url: string)` to a stable image identity and call `importReferenceAssetsFromGallery([image.id])`. Remove browser `fetch` and temporary `File` construction.

**Step 5: Run backend tests and frontend contracts**

Run: `go test ./internal/storage ./internal/service/assets ./internal/http/router -run 'Copy|ImportFromGallery' -count=1 && npm --prefix web/user run typecheck`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage internal/service/assets internal/http/handlers/api.go internal/http/router/assets_api_test.go web/user/src/pages/WorkspacePage.tsx web/user/src/pages/workspaceCreationDraft.contract.ts
git commit -m "fix: import generated images by stable identity"
```

### Task 10: Add Admin And User Attachment Controls

**Files:**
- Create: `web/admin/src/pages/AttachmentPolicyPage.tsx`
- Create: `web/admin/src/pages/attachmentPolicy.contract.ts`
- Modify: `web/admin/src/pages/SystemSettingsPage.tsx`
- Modify: `web/admin/src/pages/systemSettingsTabs.ts`
- Modify: `web/admin/src/pages/systemSettings.contract.ts`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/referenceImageUpload.ts`
- Modify: `web/user/src/pages/referenceImageUpload.contract.ts`

**Step 1: Write failing admin and user contracts**

Assert eight visible settings, image defaults, reserved-field copy, dirty/busy behavior, capabilities-driven size/format validation, and localized validation messages.

**Step 2: Run contracts and confirm missing UI**

Run: `npm exec --prefix web/admin -- tsx web/admin/src/pages/attachmentPolicy.contract.ts && npm exec --prefix web/user -- tsx web/user/src/pages/referenceImageUpload.contract.ts`

**Step 3: Implement the attachment-policy tab**

Use existing system-setting primitives, numeric inputs for MB, and tag/comma format controls. Save through versioned config-tab APIs and preserve unsaved-change guards.

**Step 4: Apply capabilities to upload selection**

Validate size and normalized extension/MIME before `uploadReferenceAsset`; keep backend errors authoritative and show configured values.

**Step 5: Run contracts, typechecks, and builds**

Run: `npm --prefix web/admin run typecheck && npm --prefix web/user run typecheck && npm --prefix web/admin run build && npm --prefix web/user run build`

Expected: PASS.

**Step 6: Commit**

```bash
git add web/admin/src/pages web/user/src/pages
git commit -m "feat: manage image attachment policy"
```

### Task 11: Use Fresh Media URLs Across User Surfaces

**Files:**
- Create: `web/user/src/mediaAccess.ts`
- Create: `web/user/src/mediaAccess.contract.ts`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/pages/LandingPage.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`

**Step 1: Write failing media-helper contracts**

Assert download always refreshes, preview refresh updates one resource, expiring URL refresh coalescing, stable React identity, and no full-page reload callback.

**Step 2: Run contracts and confirm direct stale-link use**

Run: `npm exec --prefix web/user -- tsx web/user/src/mediaAccess.contract.ts`

**Step 3: Implement shared access helpers**

Provide an async fresh-download helper and resource-scoped preview refresh. Coalesce concurrent refresh calls by stable resource key.

**Step 4: Migrate all user media surfaces**

Replace direct `window.open`, anchor construction with page-load URLs, and `reloadLoadedPages` media callbacks. Preserve loading placeholders and accessible action labels.

**Step 5: Run contracts, typecheck, and build**

Run: `npm exec --prefix web/user -- tsx web/user/src/mediaAccess.contract.ts && npm --prefix web/user run typecheck && npm --prefix web/user run build`

Expected: PASS.

**Step 6: Commit**

```bash
git add web/user/src/mediaAccess* web/user/src/components.tsx web/user/src/pages
git commit -m "fix: refresh media URLs at the point of use"
```

### Task 12: Patch Gallery Mutations Locally And Repair Publication UI

**Files:**
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/galleryRows.ts`
- Modify: `web/user/src/pages/galleryRows.contract.ts`
- Modify: `web/user/src/pages/galleryPagination.ts`
- Modify: `web/user/src/pages/galleryPagination.contract.ts`

**Step 1: Write failing local-mutation contracts**

Assert patch-by-ID, removal-by-ID, detail synchronization, filtered removal, batch partial success, distinct publish/withdraw/unpublish tone and icon, and absence of `reloadLoadedPages()` after mutations.

**Step 2: Run contracts and confirm whole-list reload behavior**

Run: `npm exec --prefix web/user -- tsx web/user/src/pages/galleryRows.contract.ts && npm exec --prefix web/user -- tsx web/user/src/pages/galleryPagination.contract.ts`

**Step 3: Add state mutation helpers**

Implement pure `patchGalleryItems` and `removeGalleryItems` helpers that preserve object identity for untouched rows. Use returned API entities for single and batch updates.

**Step 4: Repair dialog and action semantics**

Use a dedicated one-column confirmation class, distinct action tones/icons, normal-width content, and responsive buttons. Keep all status labels localized.

**Step 5: Run contracts, typecheck, and build**

Run: `npm --prefix web/user run typecheck && npm --prefix web/user run build`

Expected: PASS.

**Step 6: Commit**

```bash
git add web/user/src/pages/GalleryPage.tsx web/user/src/pages/galleryRows* web/user/src/pages/galleryPagination*
git commit -m "fix: update gallery assets without reloading media"
```

### Task 13: Verify, Smoke, And Review

**Files:**
- Modify only files required by findings from verification or review.

**Step 1: Format code**

Run: `gofmt -w <changed-go-files>`

Run the repository's existing frontend formatting command only if one is configured; do not introduce formatting churn.

**Step 2: Run focused race and contract tests**

Run: `go test -race ./internal/service/billing ./internal/service/cashier ./internal/service/assets ./internal/storage ./internal/http/router`

Run all newly created TypeScript contract files with `npm exec --prefix`.

Expected: PASS.

**Step 3: Run full repository verification**

Run: `./scripts/workflow/verify.sh`

Expected: `OK: verification passed`.

**Step 4: Run isolated API smoke**

Run: `./scripts/workflow/api-smoke.sh`

Expected: API, Worker, PostgreSQL, Redis, and fake-provider scenarios pass and clean up.

**Step 5: Perform browser verification**

Start the local stack and verify payment dialogs, 1512x982 desktop, mobile gallery confirmation, slow-network local mutation behavior, expired download refresh, and edit-reference import. Capture screenshots and network evidence under the repository's audit convention if needed.

**Step 6: Commit verification fixes**

Commit only if verification required code changes.

**Step 7: Run committed-scope review gate**

Run: `./scripts/workflow/review-local.sh --scope committed`

Run: `./scripts/workflow/check-review-gate.sh`

Expected: `.review/gate.json` is `PASS`, scope is `committed`, and tree SHA matches HEAD.

