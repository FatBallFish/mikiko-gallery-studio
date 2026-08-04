# Payment Provider Contract Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Repair JeePay end to end and eliminate confirmed protocol defects in all currently supported production payment providers.

**Architecture:** Persist the merchant order before provider initialization, then update or fail that durable order. Keep provider protocols isolated and validate each one with independent fixtures derived from official contracts and `sub2api/custom_main`.

**Tech Stack:** Go, `net/http`, Ent, Stripe Go SDK, React shared HTTP client, repository workflow scripts.

---

### Task 1: Establish protocol audit fixtures

**Files:**
- Modify: `internal/service/cashier/jeepay_provider_test.go`
- Modify: `internal/http/router/cashier_api_test.go`

1. Add the official JeePay signing vector with a hard-coded expected digest.
2. Add a fake unified-order server that independently includes `signType` in signing.
3. Add assertions that JSON `amount` and `reqTime` are numeric.
4. Add failing cases for response code one and `payDataType=form`.
5. Run the focused tests and record the expected failures.

### Task 2: Correct the JeePay protocol adapter

**Files:**
- Modify: `internal/service/cashier/jeepay_provider.go`
- Modify: `internal/service/cashier/jeepay_provider_test.go`

1. Introduce typed request and response structures.
2. Correct canonical signing to exclude only `sign` and empty values.
3. Serialize numeric amount and request time.
4. Accept only response code zero and map every documented pay-data type.
5. Run provider tests to green before refactoring duplicated response code.

### Task 3: Make order creation durable before provider initialization

**Files:**
- Modify: `internal/domain/billing/types.go`
- Modify: `internal/service/billing/store.go`
- Modify: `internal/service/billing/service.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Modify: `internal/http/handlers/api.go`
- Modify: corresponding billing, store, and router tests

1. Add failing tests proving a provider rejection leaves one failed local order.
2. Add failing tests proving immediate callbacks can resolve the local order.
3. Add store operations to apply successful initialization or failed initialization.
4. Reorder plan and custom-amount creation to persist before provider invocation.
5. Preserve idempotency and avoid repeated provider calls for initialized orders.
6. Run billing and cashier router tests to green.

### Task 4: Correct JeePay callback and lifecycle operations

**Files:**
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/service/cashier/query_provider.go`
- Modify: `internal/service/cashier/refund_provider.go`
- Modify: corresponding tests

1. Add official form callback fixtures and independent signature assertions.
2. Reuse the corrected canonical signer for callback, query, and refund paths.
3. Verify exact `success` acknowledgement and amount/order matching.
4. Run focused callback/query/refund tests to green.

### Task 5: Audit and repair the remaining providers

**Files:**
- Inspect/modify: `internal/service/cashier/easypay_provider.go`
- Inspect/modify: `internal/service/cashier/stripe_provider.go`
- Inspect/modify: `internal/service/cashier/alipay_provider.go`
- Inspect/modify: `internal/service/cashier/wxpay_provider.go`
- Inspect/modify: `internal/service/cashier/query_provider.go`
- Inspect/modify: `internal/service/cashier/refund_provider.go`
- Inspect/modify: `internal/http/handlers/api.go`
- Modify: provider and router tests only for confirmed differences

1. Build the request/callback/query/refund comparison matrix against official contracts and `sub2api/custom_main`.
2. For each confirmed defect, write one failing independent contract test.
3. Apply the smallest provider-specific correction and run it to green.
4. Record valid implementation differences in the provider runbook.

### Task 6: Repair payment-specific client fallback

**Files:**
- Modify if required: `web/shared/http-client.ts`
- Modify if required: checkout error handling and contract tests

1. Add a failing checkout test for an HTML 502 response without a local error code.
2. Map the cashier context to a payment-channel message without changing image-provider errors.
3. Run user web typecheck and contract tests.

### Task 7: Verify and review

**Files:**
- Modify: `docs/runbooks/cashier-provider-configuration.md`
- Generated: `.review/gate.json`

1. Run focused payment tests.
2. Run `./scripts/workflow/verify.sh`.
3. Run `./scripts/workflow/api-smoke.sh`.
4. Commit the implementation in reviewable slices.
5. Run `./scripts/workflow/review-local.sh --scope committed`.
6. Run `./scripts/workflow/check-review-gate.sh`.
