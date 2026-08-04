# Plan, Public Asset, and Media URL Lifecycle Design

## Status

- Date: 2026-08-05
- Source: repository owner feature request and interactive design approval
- Status: approved
- Scope: plan lifecycle controls, public asset cancellation and administration, status localization, and direct object-storage media URLs

## Goals

1. Let administrators enable, disable, soft-delete, filter, and restore plans without changing historical order credits.
2. Let users cancel pending or approved publication and apply again later.
3. Extend content review into a searchable public-image management surface with administrative unpublishing.
4. Localize every public-asset status shown in the user application.
5. Return short-lived object-storage URLs directly in API media projections so S3/R2 objects do not normally traverse application download endpoints.

## Plan Lifecycle

The existing `active`, `disabled`, and `archived` states remain authoritative. No physical plan deletion is introduced.

- Enable sets the plan to `active` and enables purchase for purchasable point packages.
- Disable sets the plan to `disabled` and disables new purchase.
- Delete sets the plan to `archived` and disables new purchase.
- Restore moves an archived plan to `disabled`; an administrator must explicitly enable it before sale.
- User checkout lists only active, purchase-enabled plans.
- The default admin list hides archived plans and exposes explicit active, disabled, and archived filters.

Enable, disable, delete, and restore use dedicated state-transition endpoints. They do not reuse the complete edit payload, preventing a stale UI draft from overwriting price or credit fields.

Payment orders already persist the purchased plan name, price, points, and bonus points. Completion and crediting continue to use that immutable order snapshot. Plan state transitions never update historical orders, ledgers, wallet grants, or user subscription records.

## Public Asset Lifecycle

User-owned publication transitions are:

```text
private -> pending_review -> approved
   ^            |             |
   +------------+-------------+
          user cancellation
```

- A user may cancel either a pending review or an approved publication.
- Cancellation immediately returns the asset to `private`, removes it from review/public queries, clears publication timing, and permits a later application.
- Cancellation is ownership-checked and idempotent when the asset is already private.
- Rejected and administratively unpublished assets require another publication request and review before becoming public.

Administrative transitions remain distinct:

- pending review -> approved or rejected;
- approved -> unpublished;
- unpublished/rejected -> approved only through an explicit review action.

Every administrative transition and user cancellation records an audit event with actor, image ID, previous status, next status, and a sanitized reason.

## Admin Experience

### Plan management

Plan cards expose icon actions for edit, enable/disable, and delete. Archived rows expose restore only. Destructive actions use confirmation dialogs and explain that historical purchases and credits remain unchanged.

### Public image management

The existing content review page becomes one management surface with tabs for pending review, approved/public, unpublished, and rejected images. Server-side pagination supports combined filtering by:

- user ID, email, or display name;
- prompt keyword;
- abstract or route model;
- task type;
- size, dimensions, and aspect ratio;
- creation/publication time.

Approved rows support detail inspection and unpublishing with a required reason. Pending rows retain approve/reject actions. Results include only the prompt and generation fields needed for moderation and never expose storage credentials.

## User Experience and Localization

The history gallery uses one public-status view model across filters, cards, bulk actions, and image details:

| API status | Chinese label | Primary action |
| --- | --- | --- |
| `private` | 私有 | 申请公开 |
| `reviewing`, `pending_review` | 待审核 | 取消申请 |
| `public`, `approved` | 已公开 | 取消公开 |
| `rejected` | 已拒绝 | 重新申请 |
| `unpublished` | 已下架 | 重新申请 |

Cancelling a pending or public asset requires confirmation. A successful transition refreshes the row and any open detail modal. Bulk publication operates only on states eligible for application; mixed selections report skipped items explicitly.

## Direct Media URL Projection

Media URL generation moves into the image/reference-asset service projection layer. It uses each resource's recorded `storage_config_id` and the existing storage router.

- A backend implementing `storage.TemporaryURLSigner` returns a five-minute signed preview URL.
- Preview and download URLs may be signed separately so downloads can include a response filename.
- Local storage, legacy remote rows, and backends without signing support retain the authenticated application download URL.
- The current image and reference-asset download handlers remain for old clients and fallback storage.

The same projection is applied to task results, task streams, creation history, private gallery, public gallery, public detail, admin moderation results, and reference assets. API DTOs must not rebuild `/api/agent/image/v1/images/{id}` after the service has projected an absolute URL.

Frontend URL handling follows:

- absolute `http`/`https` URLs are used unchanged and never receive an access-token query parameter;
- relative legacy URLs continue through the existing authenticated URL helper;
- a failed signed image load may refetch its list/detail resource once to obtain a fresh URL;
- signed URLs are not written to local storage, analytics, audit payloads, or durable application state.

Direct signed URLs eliminate the extra application authentication and redirect request. They do not weaken confidentiality relative to the existing redirect because the browser already receives the signed URL. The URL remains a bearer capability until expiry, so logs must sanitize query strings. After cancellation or administrative unpublishing, an already issued URL may remain usable for at most five minutes; public listings and detail APIs stop returning it immediately.

## Failure Handling

- Signing failure for a signing-capable private backend returns a stable storage-unavailable error rather than silently proxying and hiding configuration failure.
- A backend without signing capability deliberately uses the existing authenticated download path.
- A missing or unreadable historical storage configuration uses the established legacy-driver fallback; if resolution still fails, the API returns the existing not-found/storage-unavailable contract.
- State transition conflicts return a conflict error and never partially update publication metadata.
- Repeated plan and publication state commands are idempotent where the requested target state already holds.

## Verification

Tests must prove:

- plan enable, disable, archive, default hiding, filtering, restore-to-disabled, and historical order credit preservation;
- user cancellation from pending and approved states, repeat cancellation, reapplication, and immediate removal from public/review queries;
- admin filtering, pagination, unpublishing, reasons, permissions, and audit records;
- localized labels and context-sensitive actions for every accepted status alias;
- signed URL projection for S3/R2 and authenticated fallback for local/legacy backends;
- task, gallery, home, workspace, admin, public, and reference-asset DTOs do not overwrite projected URLs;
- absolute signed URLs never receive application access tokens and signed queries do not enter logs or persistent client state.

Final acceptance runs focused tests, `./scripts/workflow/verify.sh`, `./scripts/workflow/api-smoke.sh`, and the committed-scope local review gate.

## Rollback and Compatibility

No destructive schema migration is required. Older binaries can still read the existing plan and visibility states. Existing download endpoints remain available, so rolling back only changes whether new API responses prefer direct signed URLs. Archived plans and unpublished assets retain their state and historical records across rollback.
