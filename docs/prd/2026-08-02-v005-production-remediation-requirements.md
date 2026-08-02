# v0.0.5 Production Remediation Requirements

## Status

- Date: 2026-08-02
- Source: repository owner production verification feedback for `v0.0.5`
- Approved approach: end-to-end contract remediation (approach 2)
- Password decision: first email-code login must create a password before a normal session is issued
- Image generation: use the owner-provided OpenAI-compatible endpoint with `gpt-image-2`, `quality=high`, and a local ignored credential file

## Goal

Fix the payment, image-detail, generation-size, password, and visual-asset defects found after `v0.0.5` while preserving existing orders, users, model configuration, and established runtime behavior.

## In Scope

### Payment

1. Redirect and form-based payments open their payment page automatically after order creation.
2. The checkout page that created the order remains open and continues polling until the order reaches a terminal state.
3. A manual open-payment action remains available when the browser blocks a popup.
4. JeePay always performs unified order creation with a server-side `POST`; the browser never opens the unified-order API endpoint directly.
5. Existing JeePay instances with absent or legacy `payment_mode` values work without manual data migration.

### Image details

6. The product exposes one shared image-detail dialog style across home, workspace/history, personal gallery, and public gallery.
7. Clicking the primary image in the detail dialog opens the zoom-only full-screen viewer directly.
8. Prompt text has a bounded height and remains fully readable through scrolling.
9. Each caller supplies only the actions valid in its current context.

### Image dimensions

10. Enabling pixel-size mode provides useful default pixel presets.
11. Model administrators can independently enable custom pixel sizes.
12. Custom width and height are normalized on both client and server to legal model dimensions.
13. Ratio mode displays the estimated output width and height derived from base resolution and aspect ratio.

### Password and session security

14. An account without a password cannot receive a normal access/refresh session from email-code login.
15. After valid email-code verification, the user must create a password before entering the application.
16. The profile basic-information area exposes a password-change entry.
17. Password change requires a verification code sent to the signed-in account email.
18. Successful password setup, change, or reset invalidates every previously issued session for that user.
19. The existing pre-login forgot-password flow remains available.

### Visual assets

20. Landing and login images are product-relevant, original assets without third-party copyright dependency, legacy branding, or test-user content.
21. The user and admin applications use a coherent Mikiko Gallery Studio icon system.
22. Browser-served raster assets are compressed and responsive; generated 4K sources are not served unoptimized.
23. The supplied generation credential is never committed, logged, included in build output, or published in a Release.

## Acceptance Criteria

### Payment navigation

- A synchronous user submit opens a placeholder window before the asynchronous order request.
- Redirect results navigate that window with `location.replace`; form results render and submit into that window.
- QR, Stripe, mock, JSAPI, unsupported, and failed orders close an unused placeholder.
- Popup-blocked flows keep the order modal usable and show the manual action.
- The originating page polls every two seconds and updates balance/recent orders on success.

### JeePay

- `/api/pay/unifiedOrder` receives `POST application/json` with signed merchant data.
- The request includes `reqTime`, `version`, `signType`, callbacks, amount in fen, and the configured `wayCode`.
- Returned `payUrl`, `codeUrl`, and `payData` are classified into redirect or QR display without exposing the signing key.
- Missing, `popup`, or other legacy `payment_mode` values use API prepay behavior.
- Admin JeePay forms no longer offer popup/unified-order GET behavior.

### Image details

- No production page mounts one detail dialog from inside another detail dialog.
- The unified detail dialog retains the history-detail visual language.
- Prompt content cannot expand the dialog beyond the viewport and can be keyboard-scrolled.
- Zoom controls render above all detail overlays on desktop and mobile.
- Action buttons have accessible labels and only appear when callbacks are supplied.

### Dimensions

- Default presets are `1024x1024`, `1536x1024`, `1024x1536`, `1280x720`, `720x1280`, `1024x768`, and `768x1024`.
- `supports_custom_size` defaults to false and is present through Ent schema, domain types, persistence, admin API, routing snapshot, capability API, and frontend shared types.
- Legal output dimensions are multiples of 16, each edge is at most 3840, aspect ratio is at most 3:1, and total pixels are between 655360 and 8294400.
- Server normalization is deterministic and stores/passes only the normalized size.
- Preset-only models reject non-preset requested sizes; custom-enabled models normalize them.
- Ratio preview reuses the existing shared resolution algorithm and matches backend presets.

### Passwords

- Email-code login for a passwordless account returns a short-lived, purpose-bound setup token and no refresh cookie.
- The setup token cannot authorize any normal user API and becomes invalid after password setup.
- Completing password setup returns a normal session and refresh cookie.
- Profile responses expose `has_password` without exposing hashes or password timestamps.
- Password-change codes use a distinct `password_change` scene and can only modify the authenticated user's account.
- Password mutation increments `token_version`, revokes all stored refresh sessions, clears the current refresh cookie, and causes the frontend to remove its local session immediately.

### Visuals

- Generated source is 3840x2160 at high quality, inspected before use, and converted to responsive WebP/AVIF derivatives.
- Landing/login assets contain no rendered text, watermarks, legacy Pic Gallery marks, or copied user output.
- User/admin favicons remain legible at 16, 32, and 64 CSS pixels.
- Playwright screenshots at desktop and mobile viewports show no image/text overlap or broken crops.

## Non-functional Requirements

- Follow TDD for every behavior change; each regression test must be observed failing before implementation.
- Keep API errors in the existing `pkg/httpx` envelope.
- Keep authentication operations valid across clustered nodes; setup grants must be self-verifying and bound to `token_version`.
- Do not perform unrelated dependency upgrades or redesign unrelated pages.
- Run repository verification, isolated API smoke, browser interaction checks, committed-scope review, and the review gate before push.

## Out of Scope

- Replacing the cashier framework or changing existing order accounting.
- Supporting JeePay browser GET unified-order calls.
- Backfilling passwords for existing passwordless users without email verification.
- Redesigning the full landing-page layout.
- Publishing generated 4K intermediate source files when optimized derivatives are the actual product assets.
