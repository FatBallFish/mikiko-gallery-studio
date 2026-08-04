# v0.0.6 Runtime Hotfix Technical Design

## Status and Sources

- Status: approved
- Requirement source: `docs/prd/2026-08-03-v006-runtime-hotfix-requirements.md`
- Approved approach: protocol-first complete hotfix (approach 2)
- Delivery: independent branch for manual verification; no merge and no tag

## Architecture Summary

The hotfix is divided into four vertical contracts: payment prepay, authorized image delivery, workspace image-detail projection, and responsive visual assets. Backend authorization remains authoritative; storage signing is an optional capability, and frontend detail data is projected from complete image/task snapshots instead of lossy preview objects.

## 1. JeePay Prepay and Checkout Errors

### 1.1 Channel-extra defaults

`BuildJeePayPaymentParams` first reads the configured `channel_extra` aliases. A non-empty configured value is serialized/preserved exactly and remains authoritative. If it is absent, the provider type and `wayCode` select a minimal default:

- browser redirect payment (`ALI_PC`, PayPal PC, and other browser URL modes): `{"payDataType":"payUrl"}`;
- native QR payment (`WX_NATIVE` and QR-oriented codes): `{"payDataType":"codeUrl"}`.

The default is inserted before `jeepaySign`, so it is covered by the existing signature. The already approved signature contract remains unchanged: empty values, `sign`, and `signType` are excluded and keys use the current canonical ordering.

### 1.2 Response and diagnostics

The response mapper continues preferring explicit `payUrl` and `codeUrl`, then classifies `payData` using `payDataType`, scheme, and `wayCode`. Provider-facing errors remain `PAYMENT_PROVIDER_UNAVAILABLE` to clients.

Internally, errors retain a sanitized diagnostic with stage, HTTP status, JeePay code, and a bounded message. Diagnostics must never include the merchant key, request signature, authorization/cookie values, complete request payload, complete response payload, or redirect URL query secrets. Tests assert both useful context and secret absence.

### 1.3 Order boundary

The API keeps the current sequence: schedule instance, create provider prepay, then persist the pending billing order. This avoids local pending rows that cannot be paid and avoids a new order-initialization state machine in a hotfix. The existing synchronous popup reservation remains unchanged and begins navigating once JeePay returns a usable URL.

`web/shared/http-client.ts` gains an explicit localized entry for `PAYMENT_PROVIDER_UNAVAILABLE`, so status 502 no longer falls through to the image-provider message.

## 2. Authorized Temporary Image Delivery

### 2.1 Optional capability

The storage package adds an optional interface rather than expanding the mandatory `Backend` contract:

```go
type TemporaryURLSigner interface {
    TemporaryGetURL(ctx context.Context, objectKey string, options TemporaryGetURLOptions) (string, error)
}
```

`TemporaryGetURLOptions` carries an expiry and optional response filename/content type. `S3Backend` implements AWS Signature V4 query signing. `LocalBackend` deliberately does not implement it.

### 2.2 Delivery projection

The image-task service adds an owned delivery method that:

1. resolves the image result under the requesting user ID;
2. rejects remote/no-object-key results;
3. resolves the historical storage backend;
4. asks a signer for a five-minute GET URL when supported;
5. otherwise reads bytes through the existing backend.

The handler receives one delivery value. A signed destination produces `307 Temporary Redirect`, `Location`, and private/no-store cache headers. A byte delivery preserves the current `200`, MIME type, content-disposition, and body. The same lower-level pattern can later serve admin/public endpoints, but this hotfix changes the authenticated agent route required by production.

Permanent `301` is explicitly forbidden because browsers and intermediaries may cache it after its signed destination expires.

### 2.3 SigV4 query signing

The presigner reuses S3 endpoint, path-style, key normalization, credential scope, and signing-key helpers. It signs `host` and canonical query fields:

- `X-Amz-Algorithm`
- `X-Amz-Credential`
- `X-Amz-Date`
- `X-Amz-Expires`
- `X-Amz-SignedHeaders`
- optional response-content fields

Expiry is clamped to a safe positive maximum. Tests use a fixed clock and assert deterministic query fields/signature without contacting S3.

## 3. Complete Workspace Image Details

### 3.1 View model

`ImagePreviewPayload` gains an optional complete `detailImage` snapshot. A pure projector combines `ImageResult`, parent `ImageTask`, and current profile display name into the shared `ImageResult` detail shape. Image-specific values win; task values fill absent generation metadata.

Current output, history cards, and history dialog all use this projector. Reference-asset previews remain intentionally minimal because they are inputs, not generated results.

### 3.2 Metadata rendering

`PublicImageDetail` renders dedicated rows for:

- creator;
- model;
- actual resolution (`width x height`);
- base resolution;
- aspect ratio;
- size mode/requested size;
- task type/quality;
- output format/compression;
- moderation/output count.

Rows are compact and responsive, available values are never discarded, and absent optional values render `-`. The existing bounded prompt, actions, and zoom-only viewer remain unchanged.

## 4. Landing Assets and Login Viewport

### 4.1 Asset map

Seven generated semantic masters map to distinct paths:

- `capability-edit`: source scene transforming into a refined variation;
- `capability-reference`: multiple visual references converging on one subject;
- `capability-estimate`: precise material/light quantities represented without text or UI;
- `workflow-strip`: a panoramic progression from intent to finished contact sheet;
- `mode-text`: language-like abstract rhythm becoming a photographic scene, with no legible glyphs;
- `mode-edit`: a vertical before/after continuation scene;
- `mode-reference`: a reference collection influencing a coherent final artwork.

The private `gpt-image-2` endpoint generates high-quality masters. Prompts share the existing dark creative-studio art direction, amber/coral/cyan accents, natural material detail, no text, no watermark, no logo, and no application UI. Masters stay in ignored local storage; optimized WebP/AVIF derivatives are committed.

### 4.2 Integration

`landingContent.ts` uses a general landing asset-path type and assigns a distinct asset to every capability/mode. `LandingPage.tsx` removes filename-based dimension branching and consumes explicit asset metadata or stable shared dimensions. Capability backgrounds remain subdued enough for text contrast, while mode crops preserve their focal subject during flex expansion.

### 4.3 Login height

The login root and desktop scene/panel use `h-dvh`/`min-h-0` with page overflow contained. The default desktop panel uses smaller vertical padding and a height-aware media query/utility set that compacts card padding, headings, tabs, fields, and provider/footer spacing when viewport height is constrained. Mobile retains document scrolling only when controls genuinely cannot fit a safe touch layout.

Browser verification measures both pixels and DOM geometry. At `1512x982`, login, register, reset, and password-setup states must satisfy `document.documentElement.scrollHeight <= window.innerHeight` and keep every control inside the viewport.

## 5. Security and Compatibility

- Application authorization occurs before signing; signed URLs do not authorize arbitrary keys.
- S3/BFSS credentials and application access tokens never enter logs or generated URLs together.
- Existing storage rows and backend selection remain unchanged.
- Explicit JeePay channel configuration remains authoritative.
- Existing local storage, public gallery behavior, payment polling, callbacks, and billing settlement remain compatible.
- Generated-image credentials are local only and all generated content is inspected for unwanted text/marks.

## 6. Verification

Focused red-green tests cover JeePay defaults/diagnostics, frontend error localization, S3 presigning and handler redirect/fallback, detail projection/rendering, landing asset uniqueness, and login height contracts. Final evidence includes:

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Browser evidence includes landing screenshots at desktop/mobile and login state screenshots plus scroll geometry at `1512x982` and mobile. The branch is pushed only after all gates pass; no PR merge, tag, or Release operation is performed.
