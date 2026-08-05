# Payment And Media Reliability Technical Design

Date: 2026-08-06
Status: Approved

## Overview

This change introduces a coordinated cashier state transition path, stable-ID media access, a dynamic attachment policy, and local gallery mutations. It deliberately avoids a service-worker cache or a new authenticated CDN gateway. Existing S3/R2 direct delivery remains the media data plane.

## Cashier State Coordination

### Provider notification parsing

JeePay notification parsing will normalize either a JSON object or form values into one canonical string map. The canonical map is used for merchant lookup, signature verification, state validation, amount conversion, and transaction ID extraction. Content type is a hint; parsing may fall back when a compatible gateway sends an incorrect content type.

The handler preserves the raw bounded body for audit diagnostics but never records secrets or signing keys. Provider acknowledgement remains plain `success` for JeePay, EasyPay, and Alipay and the documented JSON acknowledgement for WeChat Pay.

### Reconciliation service

Payment completion becomes one store transaction with these properties:

- lock or serializable-read the order by order number;
- validate provider type, provider instance, amount, and trade number;
- treat an existing completed order with the same payment as idempotent success;
- permit cashier recharge recovery from `pending`, `canceled`, `expired`, or `failed`;
- create the recharge grant and ledger entry once;
- update the order to `completed`;
- record a webhook/reconciliation event including previous local status and source.

Refunded and chargeback terminal states are not recoverable through a payment-success notification.

Memory and Ent stores must implement the same transition contract.

### Query and close adapters

The existing query registry is promoted from an admin-only helper into a shared reconciliation dependency. A matching close registry is introduced with adapters for JeePay, native Alipay, native WeChat Pay, and Stripe. EasyPay supports close only when an explicit compatible close endpoint is configured; otherwise it reports unsupported.

Cancellation orchestration is:

1. Load the user-owned order.
2. Return the current terminal state idempotently where appropriate.
3. If the order has no upstream initialization, cancel locally with a conditional update.
4. Resolve the exact provider instance attached to the order.
5. Query provider state.
6. If paid, validate the amount and reconcile completion.
7. If closed, conditionally mark the local order canceled.
8. If pending, invoke the provider close adapter.
9. Mark canceled only after a definitive close result.
10. Keep pending and return conflict when query/close is unsupported, failed, or uncertain.

Conditional store updates and serializable completion resolve callback/cancel races. A callback that wins first makes cancellation observe completed. A cancellation that writes first can still be recovered by a later valid paid result.

### User sync endpoint

`POST /api/agent/cashier/v1/orders/{id}/sync` performs the same query-and-reconcile operation for the authenticated owner. It returns both the current order and a narrow sync result. Per-order throttling prevents provider polling more frequently than the configured short interval. Throttled calls return the most recent local order without becoming an error loop.

The payment monitor polls local order state and periodically invokes sync, immediately syncing again when the browser window regains focus.

## Cashier User Interface

`PaymentMonitorModal` is responsible for the just-created order and contains only the purchase title, amount, payment method, concise status, and relevant actions. It opens the reserved payment window, watches payment state, and starts a three-second close timer only after a transition to success.

`PaymentOrderDetailModal` is opened from recent-order history. It displays stable order facts and does not poll or auto-close. Safe cancellation is exposed only for pending orders and server rejection is rendered as actionable feedback.

The two dialogs share formatting helpers and status models but not lifecycle state.

## Stable Media Access

### Access projection

Image and reference-asset payloads retain stable IDs and include preview/download URL expiry metadata. New authenticated access endpoints accept a stable ID and purpose, authorize ownership or public visibility, and project a fresh object-storage URL.

Frontend code uses a shared media access helper:

- previews consume the projected URL from list/detail payloads;
- near-expiry or failed previews refresh only their resource;
- downloads always refresh for purpose `download` immediately before navigation;
- edit-reference actions send the image ID and never fetch a signed URL.

### Import by server-side copy

Storage backends gain an optional copy capability. Local storage copies within its root using bounded file operations. S3/R2 performs a signed server-side CopyObject request when source and destination use the same backend configuration. Cross-backend imports use a bounded get and put fallback. The resulting reference asset owns its copied object and remains valid if the original gallery record is deleted.

### Cache behavior

Preview signatures use a deterministic time bucket. A URL remains stable during the bucket and is signed long enough to remain valid through the next refresh margin. The response override asks object storage for a private browser cache lifetime bounded by the remaining signature lifetime. Download signatures are generated separately with content disposition and are not reused for previews.

No application proxy or service-worker persistent cache is introduced.

## Dynamic Attachment Policy

A typed attachment-policy resolver combines config-file defaults with the current persisted admin config version. The initial image limit is 20 MB. The resolver normalizes MIME/extension aliases and caches only until admin-config invalidation.

System settings add an `attachment-policy` tab with eight fields:

- image/video/audio/document maximum MB;
- image/video/audio/document allowed formats.

Only image policy is consumed by current upload paths. Reserved values are validated and stored for future workflows.

The capabilities response exposes the resolved image byte limit and allowed image MIME/extensions. The React upload path checks both before request submission.

The API uses a maximum-body reader sized to current policy plus multipart overhead, reads an individual file to `max + 1`, detects actual content, decodes image dimensions, and requires the decoded format to be allowed. WebP decoding support is registered. SVG remains rejected regardless of extension or declared MIME.

Gallery-to-reference imports pass through the same policy.

## Gallery Local Mutations

Gallery page state exposes patch and remove helpers keyed by image ID. Single publish, cancel, and group actions merge the returned entity into the row and open detail. Batch actions apply successful returned entities and leave failures untouched. Delete removes only successful IDs and updates selection state.

The cancel-publication dialog gets a one-column confirmation layout with an icon block, normal-width copy, and responsive actions. Publish, withdraw, and unpublish actions use different icons and tones from a shared presentation model.

All media surfaces use the shared refreshable image primitive keyed by stable resource ID. Refresh callbacks update the affected resource URL instead of re-fetching a page.

## Error Handling And Observability

- Provider transport uncertainty never becomes a local cancel success.
- Signature, merchant, and amount mismatches remain non-chargeable client-visible payment errors and auditable webhook failures.
- Recovery from a canceled/expired/failed state records previous status and reconciliation source.
- Media URL refresh failure leaves the existing item and presents a retry state.
- Upload errors include normalized configured limit and allowed formats without exposing storage credentials.

## Test Strategy

Backend tests cover JSON/form JeePay callbacks, duplicate delivery, callback/cancel concurrency, terminal recovery, all provider query/close adapters, sync throttling, deterministic signatures, copy behavior, dynamic config invalidation, bounded upload reads, and content/format mismatch.

Frontend contract and component tests cover modal separation, success-only close timing, focus sync, fresh download URLs, edit import by ID, attachment validation, local gallery patching, semantic action tones, and the responsive confirmation layout.

Verification requires the repository verify script, committed-scope review gate, isolated API smoke, and browser checks at desktop and mobile sizes.

