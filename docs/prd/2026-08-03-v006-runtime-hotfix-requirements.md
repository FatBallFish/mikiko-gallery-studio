# v0.0.6 Runtime Hotfix Requirements

## Status

- Date: 2026-08-03
- Source: repository owner production verification feedback for `v0.0.6`
- Approved approach: protocol-first complete hotfix (approach 2)
- Delivery boundary: push an independent hotfix branch for manual testing
- Release boundary: do not merge to `main` and do not create a tag or Release
- Image generation: use the owner-provided OpenAI-compatible endpoint with `gpt-image-2` and keep its credential local and ignored

## Goal

Repair the production defects in workspace image details, image delivery, landing and login visuals, and JeePay checkout without changing existing order accounting, storage identities, or release state.

## In Scope

### Workspace image details

1. Images opened from current generation and task history show the signed-in creator name instead of `Anonymous user`.
2. The shared detail dialog receives the complete task and image snapshot rather than a reduced preview payload.
3. The detail dialog shows the actual stored width and height as a dedicated actual-resolution value.
4. Relevant generation metadata is preserved: task type, size mode, requested size, base resolution, aspect ratio, quality, output format, compression, moderation, output count, model, and prompt.
5. Existing context-specific actions, prompt scrolling, and zoom behavior remain intact.

### Image delivery

6. Authenticated image access continues to enforce ownership before exposing object data.
7. S3-compatible BFSS storage returns a short-lived signed GET destination after authorization so image bytes do not traverse API bandwidth.
8. The API uses a temporary redirect (`307`), never a permanent `301`, because the destination expires.
9. Local storage and storage backends without URL signing continue using the existing byte-stream fallback.
10. Invalid keys, unreadable storage configurations, unauthorized images, and missing objects retain safe errors.

### Landing and login visuals

11. The three current `/landing/workspace.webp` placements no longer reuse one image for different meanings.
12. The three cards under "Choose an input method" use distinct images; the first and third no longer reuse `studio-showcase-1280.webp`.
13. The "reference image generation" and "point estimate" capability cards gain relevant image backgrounds.
14. Seven new semantic images are generated: image editing, reference generation, point estimation, workflow strip, text generation, source-image editing, and reference-led expression.
15. Generated assets contain no text, watermark, third-party marks, legacy branding, or fake application UI.
16. Browser assets are optimized WebP/AVIF derivatives with stable dimensions and practical byte sizes.
17. At a `1512x982` browser viewport, every login flow fits in one viewport without vertical page scrolling.
18. Login remains usable on shorter desktop and mobile viewports without overlap, clipped controls, or unreadable content.

### Checkout and JeePay

19. JeePay `ALI_PC` and equivalent browser redirect flows automatically request `payDataType=payUrl` when no `channelExtra` is configured.
20. JeePay native QR flows automatically request `payDataType=codeUrl` when no `channelExtra` is configured.
21. Explicit administrator `channelExtra` remains authoritative and is never overwritten by defaults.
22. The signed unified-order request remains a server-side JSON `POST` and excludes `sign` and `signType` from signature input per the existing approved contract.
23. Redirect/form results automatically navigate the synchronously reserved payment window while the checkout page keeps the order modal and polling active.
24. Provider errors produce sanitized server diagnostics without merchant keys, signatures, tokens, or complete upstream payloads.
25. `PAYMENT_PROVIDER_UNAVAILABLE` displays a payment-channel failure message rather than an upstream image-model failure message.
26. The existing order accounting sequence is retained: the local pending order is persisted only after provider prepay succeeds, preventing unpayable placeholder orders.

## Acceptance Criteria

### Image detail

- Current-output and history dialogs render the active profile display name.
- Actual resolution renders as `<width> x <height>` and is distinct from base resolution.
- A contract test proves the preview path preserves every metadata field listed above.
- Missing optional metadata renders `-` without hiding available values or inventing defaults.

### Image redirect

- The optional storage URL signer produces an AWS Signature V4 query URL with bounded expiry.
- The signed URL contains no application access token.
- An owned S3/BFSS image endpoint responds with `307` and `Location` set to the signed URL without calling `Backend.Get`.
- An owned local image still responds `200` with the original bytes and download headers.
- Redirect responses set private/no-store cache policy so an expiring destination is not retained.

### Visuals

- Each of the seven semantic placements resolves to a distinct asset path.
- Responsive containers declare stable dimensions/aspect ratios and crops remain meaningful.
- Generated masters are visually inspected before derivatives are integrated.
- Automated image checks prove expected dimensions, successful decode, nonblank pixels, and size budgets.
- Browser screenshots cover landing desktop/mobile and all login intents at `1512x982` plus mobile.
- Login page `scrollHeight` does not exceed its viewport height at `1512x982` for login, registration, reset, and mandatory password setup states.

### Payment

- A JeePay `ALI_PC` request with no configured channel extra sends compact JSON `{"payDataType":"payUrl"}` and includes it in the signature.
- A JeePay native request with no configured channel extra sends `{"payDataType":"codeUrl"}`.
- Explicit channel extra is sent unchanged.
- A returned `payData` redirect reaches the reserved popup and the checkout page retains the created order/modal.
- Provider HTTP status, JeePay result code, and a bounded sanitized message are available to server logs/tests but secrets are absent.
- The browser error for `PAYMENT_PROVIDER_UNAVAILABLE` tells the user that the payment channel is temporarily unavailable.

## Non-functional Requirements

- Follow red-green-refactor for each behavior change and observe every new regression test fail before implementation.
- Preserve `context.Context` through storage and provider operations.
- Keep API responses inside the existing `pkg/httpx` error envelope.
- Do not perform unrelated dependency upgrades or modify the existing untracked `runtime/` directory.
- Keep the supplied image-generation credential outside Git, Docker contexts, generated assets, logs, screenshots, and test fixtures.
- Run focused tests, repository verification, isolated API smoke, browser verification, committed-scope review, and the review gate before push.

## Out of Scope

- Persisting failed provider-prepay attempts as local orders or adding a payment retry state machine.
- Changing billing ledger, plan pricing, callbacks, refunds, or payment completion semantics.
- Making BFSS objects public or bypassing application authorization.
- Redesigning the landing page, checkout page, or login authentication state machine.
- Merging the hotfix, tagging a release, or publishing GitHub Release artifacts.
