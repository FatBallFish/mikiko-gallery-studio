# Plan, Public Asset, and Media URL Lifecycle Requirements

## Status

- Date: 2026-08-05
- Source: repository owner request
- Status: approved

## Requirements

1. Administrators can quickly enable, disable, soft-delete, filter, and restore plans.
2. Soft-deleted plans are hidden by default and restored as disabled.
3. Plan state changes affect only future visibility and purchase; historical order prices, credits, grants, and subscriptions remain unchanged.
4. Users can cancel pending publication or an already public asset, returning it to private and allowing a later application.
5. Administrators manage pending, public, unpublished, and rejected images in the existing content-review page with server-side filters for user, prompt, model, generation parameters, status, and time.
6. Administrators can unpublish a public image with a recorded reason.
7. Every public-asset status displayed to users has localized Chinese copy and a context-appropriate action.
8. S3/R2-backed image and file API projections directly return five-minute temporary signed URLs across home, workspace, history, public gallery, admin review, task results, and reference assets.
9. Existing authenticated download endpoints remain as fallback for local storage, legacy clients, and backends without URL signing.
10. Absolute storage URLs never receive application access-token query parameters and signed URL queries do not enter persistent logs, analytics, audits, or client storage.

## Acceptance Criteria

- Archived plans are absent from default admin and checkout lists, visible under the deleted filter, and restorable to disabled.
- An order created before a plan transition credits exactly its snapshotted points after payment.
- Cancelling pending/public assets removes them immediately from review/public queries and permits a new publication request.
- Admin public-image filters are database-backed and pagination totals match filtered results.
- Every supported visibility alias renders Chinese copy in filters, cards, bulk actions, and detail dialogs.
- S3/R2 resources in all named API projections use direct signed URLs rather than application image routes.
- Local and legacy resources remain readable through fallback routes.
- Full repository verification, isolated API smoke, and committed-scope review pass.

## Non-goals

- Physical deletion of plans.
- Rewriting historical payment orders or wallet ledgers.
- Introducing a CDN or permanent public object URLs.
- Removing legacy media download endpoints.
