# Payment Provider Contract Remediation Technical Design

## Status and Sources

- Status: approved
- Requirement source: `docs/prd/2026-08-04-payment-provider-contract-remediation-requirements.md`
- Owner approval: complete JeePay repair plus contract audit of EasyPay, Stripe, native Alipay, and native WeChat Pay
- Reference repository: `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api`, branch `custom_main`

## Architecture

The repair separates durable order creation from provider initialization. The API first persists one local order under the request idempotency key, then invokes the selected provider, and finally applies either initialized payment data or a failed initialization state. Provider adapters continue to own protocol-specific request construction and response parsing; no generic signer or payload abstraction is shared across incompatible providers.

## 1. Order Initialization State

The existing `pending` order remains the durable pre-provider state. Billing storage gains an update operation that applies provider initialization data, and a failure operation that moves an uninitialized order to `failed`. The create endpoint follows:

1. resolve plan, visible method, and provider instance;
2. resolve or create the local order by `(user_id, idempotency_key)`;
3. if the order already has usable provider initialization data, return it without another provider call;
4. invoke the provider with the persisted merchant order number;
5. update the order on success, or mark it failed for a definite failure;
6. return the provider error while retaining the order for audit and recent-order display.

Provider callbacks can now find the local order as soon as the external order exists. Existing ledger crediting and callback idempotency remain unchanged.

## 2. JeePay Contract

Use a typed request struct with numeric `amount` and `reqTime`. Build a string signing projection from the exact outbound fields. The canonical signer excludes only empty values and `sign`, sorts names with Go byte-string ordering, joins `key=value` pairs with `&`, appends `&key=<key>`, hashes with MD5, and uppercases the hex output.

The response uses typed envelopes. Only code zero succeeds. `payDataType` maps as follows:

| Type | Local display |
| --- | --- |
| `payUrl` | redirect URL |
| `form` | form HTML |
| `codeUrl` | QR payload |
| `codeImgUrl` | QR image URL |
| `none` | no actionable display; reject for checkout creation |

Callback verification uses the same canonical algorithm over form values and acknowledges with exact lowercase `success`. Query and refund adapters must use the corrected signer as well.

## 3. Provider Audit Method

Each provider receives a matrix containing create, callback, query, and refund boundaries. Tests use protocol-aware fake servers or deterministic cryptographic fixtures that validate the request independently from production helpers.

### EasyPay

Verify form POST/GET requirements, MD5 parameter exclusion rules, amount precision, success code parsing, callback acknowledgement, query, and refund behavior. Do not apply JeePay signing rules.

### Stripe

Verify PaymentIntent idempotency key propagation, integer minor units, raw-body webhook signature verification, event-to-order correlation, query status mapping, and refund idempotency. Stripe SDK behavior remains authoritative.

### Native Alipay

Verify RSA2 canonical content, charset/sign-type fields, app/private/public key roles, notify verification, exact `success` acknowledgement, amount matching, query, and refund request construction.

### Native WeChat Pay

Verify API v3 Authorization canonical message, certificate/platform signature verification, AES-GCM callback decryption, exact success response, JSAPI/native response mapping, query, close, and refund behavior.

## 4. Error and Security Boundaries

Provider logs contain stage, HTTP status, provider code, and a bounded sanitized message. They never contain credentials, signatures, complete redirect queries, cookies, tokens, private keys, webhook secrets, or raw callback bodies. API clients retain stable payment-domain error codes. Checkout applies a payment-specific fallback for non-JSON 502 responses.

## 5. Testing Strategy

TDD is mandatory. The first red tests are the official JeePay signing vector, numeric JSON types, rejection of code one, form response classification, and durable failed-order persistence. Cross-provider tests are added only after an audit difference is confirmed.

Focused verification runs before the repository-wide workflow. The isolated API smoke uses local fake providers and never contacts live payment services.
