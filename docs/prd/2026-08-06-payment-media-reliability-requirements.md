# Payment And Media Reliability Requirements

Date: 2026-08-06
Status: Approved

## Context

Production testing exposed correctness and usability defects across cashier payment reconciliation, generated-image reuse, expiring media URLs, attachment policy, and gallery mutations. The most severe defect allows a locally pending order to be canceled after the provider has collected payment but before the callback has completed local crediting.

The approved accounting rule is **payment success wins**. A valid provider callback or a trusted provider query that proves payment succeeded must complete and credit the order exactly once, even when the local order was previously canceled, expired, or failed.

## Payment Requirements

1. JeePay callbacks must accept the official JSON notification body as well as form-encoded notifications used by compatible deployments.
2. Callback verification must bind the merchant instance, verify the signature, verify the successful provider state, and verify the paid amount before crediting.
3. Payment completion must be idempotent and must never issue duplicate points or duplicate ledger entries.
4. A valid paid result must recover a locally `canceled`, `expired`, or `failed` cashier order and complete it exactly once.
5. Canceling an initialized order must query the provider before changing local state.
6. When the provider reports paid, cancellation must reconcile and complete the order instead.
7. When the provider reports pending, cancellation may complete only after a definitive provider close succeeds.
8. When provider query or close is unsupported or uncertain, the local order must remain pending and cancellation must be rejected.
9. The implementation must cover JeePay, EasyPay, Stripe, native Alipay, and native WeChat Pay. Providers without a reliable close API must fail closed.
10. The user application must have an authenticated order-sync endpoint so missed callbacks can be recovered by active polling without administrator intervention.
11. Payment monitoring and historical order details must use separate dialogs.
12. The payment monitoring dialog may close automatically three seconds after confirmed success only. Failure, cancellation, and expiry must remain visible until the user closes them.

## Media Identity And Delivery Requirements

1. Business actions must use stable image or asset identifiers rather than expiring signed URLs.
2. Using a generated image as an edit reference must call the gallery import business API with the image ID.
3. Same-storage imports must use server-side object copy so image bytes do not traverse the application server.
4. Cross-storage imports may use bounded streaming as a fallback and must enforce the current image policy.
5. Preview and download URLs must be refreshable by stable resource ID.
6. Download actions must request a fresh download URL at click time.
7. An expired preview must refresh only the affected image, not reload its entire list.
8. Media access URLs must remain short-lived and permission checked.
9. Signed preview URLs should remain stable within a short time bucket so normal browser caching can reuse an image across page reloads.
10. Media bytes must continue to be served directly by BFSS/S3/R2 instead of being proxied through the API service.

## Attachment Policy Requirements

1. The default maximum image attachment size is 20 MB.
2. System settings must expose maximum attachment size fields for image, video, audio, and document files.
3. System settings must expose supported-format fields for image, video, audio, and document files.
4. Video, audio, and document settings are reserved in this release and do not create new upload workflows.
5. The image size and format settings must take effect dynamically without restarting the service.
6. The default image formats are PNG, JPEG, WebP, and GIF. SVG is not supported.
7. The user application must validate configured image size and format before upload.
8. The API must enforce the same current policy using bounded request reads and content-based validation. Client validation is advisory only.

## Gallery Experience Requirements

1. `申请公开`, `取消申请`, and `取消公开` must have distinct labels, icons, and visual tones.
2. The cancel-publish confirmation dialog must use a dedicated responsive layout and must not collapse text into single-character columns.
3. Publish, cancel publish, group, and delete mutations must update only affected list entries.
4. Mutating one asset must not reload other images or replace their signed URLs.
5. Batch operations must patch successful items, preserve failed items, and report partial failures.
6. Public status values must be consistently localized in Chinese.
7. Home, workspace, history, public gallery, and detail views must share the same refreshable media behavior.

## Acceptance Criteria

1. A valid JeePay JSON callback credits a pending order and returns the provider-required success response.
2. Replaying the same callback or sync request does not credit twice.
3. A paid callback racing with cancellation results in one completed, credited order.
4. A user cannot locally cancel an initialized provider order unless the provider has confirmed it is closed or a close request succeeds definitively.
5. Missed callbacks are recovered from the payment monitoring dialog through active provider sync.
6. Only a success result starts the three-second monitoring-dialog close timer.
7. Editing a generated image does not fetch its signed object URL in the browser.
8. Downloads work after the original five-minute URL has expired.
9. A 20 MB valid configured image format is accepted; over-limit or disallowed content is rejected consistently by frontend and backend.
10. Gallery publish and grouping operations do not trigger network reloads for unrelated images.
11. Reloading a page within the signing bucket reuses the same preview URL and can hit browser cache.
