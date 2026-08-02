# v0.0.5 Production Remediation Technical Design

## Status and Sources

- Status: approved
- Requirement source: `docs/prd/2026-08-02-v005-production-remediation-requirements.md`
- Approved approach: end-to-end contract remediation
- Password flow: mandatory setup before normal session issuance
- Existing related design: `docs/plans/2026-08-02-post-release-remediation-design.md`

## Architecture Summary

The work is divided into five vertical contracts: payment navigation/provider behavior, unified image details, model size capabilities, password/session security, and visual assets. Each contract is enforced at its authoritative backend boundary and then projected through shared frontend types into the user or admin interface. Existing storage identities, orders, users, callback routes, and generation pricing remain unchanged.

## 1. Payment Navigation and JeePay

### 1.1 Popup reservation

Browser popup policies require `window.open` to run synchronously inside the submit event. Checkout therefore creates a small `PaymentWindowReservation` before awaiting order creation. The reservation owns a `Window` reference, can navigate redirect results, can write and submit provider HTML forms, and closes itself when unused or on failure.

The order modal always opens in the original page. Existing polling remains the source of payment completion. If reservation creation returns `null`, the modal exposes the existing manual action and a non-fatal explanation.

### 1.2 Display dispatch

The existing `checkoutPaymentDisplayModel` remains the canonical classification. After order creation:

- `redirect`: navigate the reserved window;
- `form`: write a minimal document, inject the provider HTML, and submit the first form when provider markup did not already do so;
- `qr_code`, `stripe`, `mock`, `unsupported`, `none`: close the reservation;
- request/display failure: close the reservation and preserve the existing error feedback.

The external page receives `window.opener = null` before navigation to prevent reverse-tabnabbing. Manual fallback uses `noopener,noreferrer`.

### 1.3 JeePay unified order

`BuildJeePayPaymentDisplay` always calls the API prepay path. The obsolete URL-building path is removed. Unified-order input is encoded as JSON and sent with `POST` and `application/json`. The signed payload adds `reqTime` in Unix milliseconds and `version=1.0`; signatures continue excluding `sign` and `signType` according to the JeePay contract.

Response projection prefers explicit `payUrl`/`codeUrl`. `payData` is classified by scheme and `wayCode`: HTTP(S) browser destinations become redirect URLs; QR/native payloads become QR data. Provider errors remain sanitized as `PAYMENT_PROVIDER_UNAVAILABLE`.

The admin removes JeePay `payment_mode` selection. Backend behavior ignores missing and legacy values, so no database migration is required.

## 2. Unified Image Detail

`ImageDetailModal` becomes the single production detail component and accepts a typed image view model plus an action list. Its current history-detail layout and public-detail metadata structure are combined without nesting cards or dialogs. Prompt content uses a fixed responsive maximum height, `overflow-y:auto`, and focusable semantics for keyboard scrolling.

The zoom-only `ImageZoomViewer` remains a separate overlay because it serves a distinct full-screen interaction. Clicking the main image opens it directly. The second detail-style `ImageLightbox` is removed after all callers migrate.

Callers map their capabilities into actions:

- workspace/history: reuse, edit, download, delete where authorized;
- personal gallery: copy prompt, publish/unpublish, group, download, delete;
- public gallery/home: like, favorite, copy prompt, download;
- anonymous public views omit authenticated actions.

Overlay focus trapping and layer constants remain centralized. Closing zoom returns to the same unified detail dialog; closing detail returns to the originating page.

## 3. Model Size Capabilities

### 3.1 Persistent capability

Add `supports_custom_size bool default false` to `model_account_models`. The field flows through Ent-generated persistence, domain requests/responses, the model-admin store, routing snapshots, `ProviderCandidate`, visible route capabilities, handler JSON, and shared TypeScript API types.

Default pixel presets become:

```text
1024x1024, 1536x1024, 1024x1536, 1280x720,
720x1280, 1024x768, 768x1024
```

### 3.2 Deterministic normalization

Create a shared Go domain function that accepts requested width and height and returns a legal pair. It:

1. rejects non-positive or unparsable inputs;
2. clamps excessive aspect ratio by expanding the shorter edge toward 3:1;
3. scales down to the 3840 maximum edge;
4. scales area into the allowed pixel interval while preserving the adjusted ratio;
5. rounds both edges to the nearest multiple of 16;
6. performs a final bounded correction so all invariants hold.

The TypeScript helper mirrors the algorithm for immediate feedback. Backend normalization is authoritative. Resolver capability matching accepts a requested pixel size when it is an explicit preset, or when the selected provider candidate has `SupportsCustomSize`; otherwise it returns the existing capability-mismatch error.

### 3.3 Admin and workspace

When pixel mode is enabled, the admin shows preset tags and a custom-size toggle. New model drafts receive the default preset list. Workspace shows either preset selection or a “自定义尺寸” option. Custom width/height fields update a visible final normalized size and include the exact owner-provided model-limit description.

Ratio mode calls the existing `calculateImageSizeForBaseResolution` helper and displays the estimated dimensions beside the controls. For `auto`, each visible route model exposes `auto_base_resolution_by_task_type`, derived with the same route-default and configured-price fallback rules used by request resolution. The workspace uses the selected task type's value and hides the estimate when that authoritative value is absent; it never assumes 1K. This is advisory UI; the request continues sending ratio/base resolution and the backend remains authoritative.

## 4. Password Setup and Session Invalidation

### 4.1 Limited setup grant

Email-code login verifies and consumes the code as today, creates the user/trial grant when needed, and checks `PasswordHash`. For a passwordless user it does not create a refresh session. Instead it signs a short-lived JWT with:

- subject/user ID and email;
- current `token_version`;
- `purpose=password_setup`;
- issuer and ten-minute expiry.

This token is only parsed by the password-setup endpoint. Normal access-token parsing rejects it because normal claims do not carry the expected access purpose. The setup endpoint reloads the user and checks ID, email, token version, status, expiry, issuer, and purpose before setting the password.

Setting the password increments `token_version` and revokes all refresh sessions in one database transaction, then returns a newly issued normal session. Refresh-session records persist the token version present at issuance; refresh rejects passwordless users and stale token versions before rotation. This makes the setup grant one-time, prevents pre-upgrade passwordless cookies from bypassing setup, and keeps invalidation correct across clustered nodes without cluster-local memory.

Adding `refresh_sessions.token_version` advances the explicit database schema version from 1 to 2. PostgreSQL migration adds the non-null column with default 0 so existing sessions remain readable; passwordless or stale legacy sessions are then rejected by the authentication service rather than failing at the storage boundary.

### 4.2 Profile password change

Profile payloads add `has_password`. The existing send-code endpoint accepts `password_change`. The authenticated password-change endpoint accepts `code` and `new_password`, verifies the code against the authenticated user's canonical email, updates the password, revokes all sessions, clears the refresh cookie, and records the existing audit event. It does not accept an arbitrary email or rely on an old password.

The profile dialog first sends a code, then collects code/new password/confirmation. On success the application synchronously clears local session state and navigates to login. Forgot-password request/confirm behavior stays unchanged.

### 4.3 Login UI state machine

Email-code login becomes a result union:

- normal session: fetch profile and enter the requested route;
- password setup required: remain on the login page, show new password and confirmation, submit the setup token, then install the returned normal session.

Refresh cookies are never set for the intermediate result. Reloading the page intentionally restarts verification rather than persisting the setup token in local storage.

## 5. Visual Asset System

The owner-requested custom API path is used through the image-generation CLI workflow. Its key lives in a locally ignored, permission-restricted file and is read into a single generation process; it is never passed to git, build arguments, screenshots, or test fixtures.

Generate one 3840x2160 high-quality master visual with no text, watermark, UI chrome, or third-party marks. The art direction is a sophisticated AI image studio contact sheet: distinct original scenes, restrained dark-neutral surround, amber/coral/cyan accents, and clear regions that survive responsive cropping. Inspect the master, then create desktop/mobile WebP and AVIF derivatives with a size budget verified by `file`, image dimensions, and byte counts.

The workspace feature image is a truthful screenshot of the repaired application captured with seeded local test data, not a generated fake interface. User and admin favicon/logo variants are deterministic SVG assets sharing a Mikiko “M + image frame” mark; raster generation is not used for icons because small-scale legibility requires vector control.

## 6. Error and Compatibility Boundaries

- Existing JeePay config remains readable; legacy `payment_mode` is ignored for JeePay only.
- Existing preset-only models remain preset-only because `supports_custom_size` defaults false.
- Existing password users continue receiving normal email-code sessions.
- Password hashes, setup grants, signing inputs, API keys, and provider response bodies never enter logs or audits.
- New authentication responses are additive unions under the existing endpoint; normal login response fields remain stable.
- Existing zoom controls, payment polling, and forgot-password routes are retained.

## 7. Verification

Each vertical slice follows red-green-refactor. Final evidence includes:

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Browser verification covers checkout popup behavior, unified image details and zoom, custom/ratio sizes, mandatory password setup, profile password change/logout, and responsive landing/login visuals. The release is complete only after the PR is merged, a new tag is pushed, GitHub Actions succeeds, and the GitHub Release contains the expected artifacts and templated Chinese notes.
