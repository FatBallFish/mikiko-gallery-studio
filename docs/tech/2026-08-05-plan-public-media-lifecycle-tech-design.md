# Plan, Public Asset, and Media URL Lifecycle Technical Design

## Status and Sources

- Status: approved by the repository owner on 2026-08-05
- Requirement: `docs/prd/2026-08-05-plan-public-media-lifecycle-requirements.md`
- Full approved design: `docs/plans/2026-08-05-plan-public-media-lifecycle-design.md`
- Implementation plan: `docs/plans/2026-08-05-plan-public-media-lifecycle.md`

## Architecture

Plan management keeps the existing `active`, `disabled`, and `archived` states and adds dedicated, idempotent transition operations. Archived plans are hidden by default and restore to disabled. Payment completion continues to credit the immutable price and points snapshot already stored on each order.

Publication management adds an owner-scoped cancellation transition from pending or approved back to private. Administrative rejection and unpublishing remain separate governance states. The existing content-review surface gains database-backed status, user, prompt, model, generation-parameter, and time filters.

Media URL projection moves into image and reference-asset services after ownership or visibility authorization. Storage backends implementing `TemporaryURLSigner` return five-minute preview/download URLs; local, legacy, or non-signing backends retain authenticated application download routes. Task, workspace, gallery, public, admin, and reference-asset DTOs preserve the projected URL instead of rebuilding backend image routes.

Absolute object-storage URLs are used unchanged by frontend clients and never receive application access-token parameters. Existing download endpoints remain for compatibility and fallback. Signed URL query strings must not be persisted or logged, and an expired media load may trigger at most one resource refetch.

## Verification

Implementation follows TDD and must cover state transitions, historical-order credit preservation, publication cancellation/reapplication, admin filtering/unpublishing, localized status copy, signed/fallback URL projection, and every consuming API/frontend surface. Final acceptance requires repository verification, isolated API smoke, and the committed-scope review gate.
