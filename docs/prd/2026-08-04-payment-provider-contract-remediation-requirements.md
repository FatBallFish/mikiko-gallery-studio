# Payment Provider Contract Remediation Requirements

## Status

- Date: 2026-08-04
- Source: repository owner production verification feedback after `v0.0.7`
- Status: approved for implementation
- Approved approach: protocol-first remediation with durable local order lifecycle
- Reference implementation: `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api`, branch `custom_main`
- Protocol sources: official JeePay, EasyPay, Stripe, Alipay Open Platform, and WeChat Pay documentation

## Goal

Make JeePay order creation and fulfillment reliable, then audit and repair equivalent contract defects in EasyPay, Stripe, native Alipay, and native WeChat Pay without changing pricing or crediting semantics.

## Superseded Constraints

For payment behavior only, this document supersedes the following constraints in the `v0.0.6` hotfix sources:

- JeePay signatures must include every non-empty request field except `sign`; `signType` is not excluded.
- A durable local order is created before invoking a payment provider. Provider initialization failure marks the order failed instead of omitting it.
- Payment callback, query, refund, and idempotency behavior may be corrected when protocol audit evidence requires it.

## In Scope

### JeePay

1. Unified order uses server-side `POST /api/pay/unifiedOrder` with a typed JSON payload.
2. `amount` and `reqTime` are JSON numbers matching the official `int` and `long` contracts.
3. MD5 signing sorts non-empty field names by ASCII order, includes `signType`, excludes only `sign`, appends `&key=<merchant-key>`, and returns uppercase hexadecimal.
4. Only `code == 0` is successful.
5. `payDataType` values `payUrl`, `form`, `codeUrl`, `codeImgUrl`, and `none` are handled explicitly.
6. Provider response diagnostics remain sanitized and bounded.
7. Form-encoded payment callbacks are verified against every non-empty callback field except `sign`, then acknowledge with the exact text `success`.
8. Query and refund requests use the same canonical signing contract.

### Durable order lifecycle

9. Order identity and idempotency are persisted before a provider call.
10. Provider success updates the pending order with payment URL, QR code, client token, display metadata, and channel trade number.
11. Definite provider rejection marks the order failed with safe diagnostic metadata.
12. Timeout or transport uncertainty does not create duplicate provider orders when the same idempotency key is retried.
13. A callback arriving immediately after provider order creation can resolve an existing local order.

### Cross-provider audit

14. EasyPay, Stripe, native Alipay, and native WeChat Pay are compared field-by-field against official contracts and the `sub2api/custom_main` reference.
15. Audit covers request method and content type, amount units, signing, response parsing, callback verification and acknowledgement, query, refund, timeout, and idempotency.
16. Confirmed defects are fixed with failing regression tests. Differences that are valid for this product are documented and left unchanged.
17. Provider secrets, private keys, webhook secrets, signatures, authorization values, and raw sensitive payloads never enter client responses or logs.

### User-visible errors

18. Checkout failures identify payment-channel failures even when a reverse proxy returns an HTML 502 body.
19. Failed provider initialization remains visible in recent orders with a stable failed status and no unusable payment action.

## Acceptance Criteria

- The official JeePay signing vector produces `924065BA077FA461A9B06D2E76E9ED3C`.
- A fake JeePay server independently rejects a request whose `signType` is omitted from signing.
- JeePay request JSON decodes `amount` and `reqTime` as numbers.
- A JeePay `ALI_PC` response with `payDataType=payUrl` returns a redirect payment URL.
- A JeePay `form` response remains form HTML and is never classified as a QR payload.
- JeePay business code `1` is rejected.
- A failed provider call leaves exactly one failed local order for an idempotency key.
- Retrying the same idempotency key does not call the provider twice after a known completed initialization.
- Official-form callback fixtures verify successfully and return exactly `success` without a trailing newline.
- Each audited provider has an evidence-backed test matrix and all confirmed defects are covered.
- Existing paid orders, balances, plan prices, and webhook idempotency remain unchanged.

## Verification

- Focused Go provider and cashier router tests
- `./scripts/workflow/verify.sh`
- `./scripts/workflow/api-smoke.sh`
- `./scripts/workflow/review-local.sh --scope committed`
- `./scripts/workflow/check-review-gate.sh`

## Out of Scope

- Adding new payment providers or currencies
- Replacing the cashier domain with a plugin framework
- Migrating historical completed orders
- Publishing a release, merging to `main`, or changing production configuration in this task
