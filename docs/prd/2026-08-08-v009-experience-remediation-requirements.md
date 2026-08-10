# v0.0.9 Experience Remediation Requirements

Date: 2026-08-08
Status: Approved requirement source
Source: repository owner v0.0.9 experience report and follow-up decisions
Target repository baseline: `v0.0.9` (`fa6bdda`)

## 1. Background

Production-like use of v0.0.9 exposed correctness, compliance, usability, lifecycle, and observability gaps across the user application and administration console. Several reported gaps are not isolated UI defects: points validity differs between payment paths, generated-image reuse duplicates storage objects, size parameters cannot be omitted, assets have no project ownership, and deletion behavior is inconsistent across database and object storage.

This document freezes the product requirements for the remediation. It supersedes conflicting behavior in earlier documents. In particular, it supersedes the copy-on-import requirement in `docs/prd/2026-08-06-payment-media-reliability-requirements.md`: a gallery image reused as a reference must no longer create a copied object.

## 2. Goals

1. Make order credits, wallet balances, expiry, and generation charges understandable and internally consistent.
2. Make image-generation parameters match the effective GPT Image generation contract while preserving the platform's existing multi-request fan-out behavior.
3. Remove unnecessary media copies and introduce a reliable asset lifecycle.
4. Introduce projects as the mandatory ownership boundary for generated assets.
5. Complete missing administration actions without breaking historical records.
6. Make readiness, cluster, and model-call dashboards report real runtime state.
7. Remove invalid compliance text and obsolete configuration.

## 3. Definitions

| Term | Definition |
|---|---|
| Fixed points package | A purchasable `points_package` plan with base points, optional bonus points, and an administrator-controlled expiring or non-expiring policy. |
| Custom recharge | A user-entered CNY amount converted to points. It is long-lived and has no expiry. |
| Purchased points | Base points credited by a fixed points package. |
| Gift points | Bonus points from a package or another explicit gift grant. |
| Platform output count | Total images requested by the user for one platform task. It may be greater than an upstream model's single-request `n`. |
| Upstream max n | Maximum images one upstream request to a specific account model may request. |
| Project | A user-owned container to which every generated asset belongs. |
| Reference alias | A reference-asset record that points to an existing generated object instead of owning a copied object. |

## 4. Scope And Priority

| Priority | Scope |
|---|---|
| P0 | Points/order correctness, false compliance text, direct docs navigation, obsolete docs config, readiness correctness, prompt textarea layout. |
| P1 | Image size contract, custom ratio/pixel validation, `background`, actual-size diagnostics, no-copy reference aliases, first-reference parameter reuse, multi-image history overview, asset deletion lifecycle. |
| P2 | Projects, global project selection, project management, complete asset batch operations, admin delete actions, single-node cluster visibility, real model-call distribution. |

## 5. User Application Requirements

### 5.1 Orders, Wallet, And Ledger

1. Recent orders must separately display base points and bonus points.
2. A pending fixed-package order with expiry enabled must display `valid for N days after crediting`; a non-expiring package must display `points do not expire`.
3. A completed fixed-package order must display its actual credit expiry time or its non-expiring status.
4. Payment-order expiry and credit expiry are different concepts and must never share the same field or label.
5. Fixed-package base points and bonus points follow the same expiry policy snapshot captured when the order is created. When expiry is enabled, they expire together after the snapshotted number of days; when disabled, both grants are long-lived.
6. Custom recharge points are long-lived.
7. Existing long-lived grants created by v0.0.9 must not be assigned a retroactive expiry.
8. The profile balance must separately display purchased/package points, custom recharge points, gift points, and trial points, plus frozen points and total available points.
9. The profile must show the amount and time of the next expiring grant whenever any expiring grant exists, not only for trial points.
10. A generation charge ledger row must expose successful output quantity, effective unit points, total charged points, and related task ID.
11. For partial success, unit points and total charged points must be based on successful output count; failed outputs must not be charged.
12. Wallet bucket totals and the displayed available total must remain reconcilable to five decimal places.

### 5.2 Compliance And Developer Documentation

1. Remove the hard-coded `京ICP备20261024号-1` text from all user-facing and demo surfaces.
2. Every developer-documentation entry must navigate directly to the resolved current documentation URL.
3. The internal `/docs` route must be removed. This is an intentional breaking change; no redirect or compatibility route is retained.
4. Remove the administration fields for documentation title and base path because they do not control the deployed documentation site.
5. The deployment-level `PIC_GALLERY_DOCS_URL`/`VITE_DOCS_URL` remains the authoritative documentation URL.
6. Documentation readiness must probe the effective deployed documentation/OpenAPI endpoint rather than checking obsolete saved title/base-path values.

### 5.3 Workspace Experience

1. The outer prompt textarea must use its full content width.
2. Prompt action buttons remain overlaid at the bottom right and may reserve bottom space, but must not reserve the right side of every text line.
3. Reusing a gallery image as a reference must not copy, upload, or create a second object-storage object.
4. The first imported reference image, when the current reference list was empty, must apply the source task's reusable generation parameters.
5. Additional imported references must not overwrite the current configuration.
6. Reused parameters must be revalidated against the current route-model capability. Unsupported values must fall back to an explicit user-visible valid state rather than being submitted silently.
7. A history task with multiple results must first open a task overview containing all result thumbnails and task-level information.
8. Selecting a thumbnail in the overview must open the shared image-detail component. Closing image detail returns to the overview.

### 5.4 Image Size And Generation Parameters

1. The size dimension consists of `size_mode`, base resolution, aspect ratio, and explicit pixel size.
2. `size_mode` supports `auto`, `ratio`, and `pixel`.
3. In `auto` mode, base resolution, aspect ratio, and explicit pixel controls are hidden, and the downstream `size` field is omitted.
4. Base resolution must not contain `auto` in the administration console, capabilities response, or user selector.
5. In `ratio` mode, the user selects base resolution and aspect ratio.
6. Ratio mode may expose a custom-ratio input only when the selected effective model capability enables it.
7. In `pixel` mode, the user selects a configured preset or enters a custom size only when custom pixels are enabled.
8. Every account model that supports pixel mode must be configurable with minimum width, maximum width, minimum height, and maximum height.
9. Preset and custom pixel sizes must satisfy both platform/provider hard limits and the configured account-model interval.
10. Invalid explicit pixel input must be rejected. The platform must not silently round or replace an explicit user value.
11. Legacy invalid presets must be omitted from user capabilities, and new invalid presets must be rejected when saved by an administrator.
12. Ratio-derived dimensions may be rounded to the nearest legal 16-pixel grid because they are computed values, but the final resolved dimensions must be returned to the user before submission.
13. For GPT Image generation-compatible sizes, width and height must be divisible by 16 and the aspect ratio must remain between `1:3` and `3:1`; official maximum/experimental boundaries must also be enforced.
14. The platform must preserve the distinction between requested dimensions and actual returned dimensions.
15. A requested/actual mismatch must record the outbound size, account/model, upstream request ID, source mode, returned dimensions, and diagnostic classification.
16. The observed `1280x720` request producing `1672x941` must not be "fixed" by changing the local calculator until diagnostics identify a local rewrite. If no local rewrite is found, retain behavior and report the upstream mismatch.

### 5.5 GPT Image Field Contract

1. Generation requests must treat `size` as optional.
2. The OpenAI adapter must not send `response_format` for GPT Image models.
3. Existing quality, moderation, output format, and conditional output compression behavior remains capability-driven.
4. Add model capability configuration for supported background values. Allowed values are `auto`, `opaque`, and `transparent`.
5. The user application displays the background selector only when the effective route-model capability exposes at least one supported value.
6. When `background=transparent`, the output format must be PNG or WebP. Selecting transparent with JPEG is invalid; changing to JPEG must move background to a supported non-transparent value with a visible state update.
7. Background validation must be enforced again by estimate, task creation, and worker/provider request construction.
8. Streaming and partial images are explicitly out of scope.
9. The platform output count behavior remains unchanged. A task may request more images than one upstream call supports.
10. Account-model `max_image_count` represents upstream max `n` and must be limited to `1-10` in administration and backend validation.
11. The existing task fan-out splits total platform output count into multiple upstream calls using each selected candidate's `max_image_count`. This logic, concurrency control, partial-success behavior, billing, and fallback semantics must not be replaced.
12. The OpenAI edits documentation is not normative for this change. Existing edit-adapter behavior is retained and aligned with the platform's existing code plus generation-compatible parameter validation.

### 5.6 Projects And Global Selection

1. Every user has exactly one immutable default project named `默认`.
2. Every generated image and future media asset must belong to one project.
3. Existing users and assets are migrated into each user's default project.
4. A new asset with no explicitly supplied project uses the user's default project.
5. The workspace exposes a project switcher; subsequent tasks and generated assets use the selected project.
6. The asset page exposes the same project switcher and lists only assets in the selected project.
7. Project selection is shared across the site and remembered in browser storage.
8. On login or refresh, the remembered project is used only if it still exists and belongs to the current user; otherwise the default project is selected and storage is repaired.
9. Add a `项目` menu supporting list, create, rename, and delete.
10. The default project cannot be renamed or deleted.
11. Deleting a non-default empty project is allowed after confirmation.
12. Deleting a project containing assets requires a target project and atomically transfers all contained tasks/assets before deletion. The default project is preselected unless it is the project being deleted.
13. Project ownership checks must be enforced server-side for all reads, writes, transfers, task creation, and reference imports.

### 5.7 Asset Batch Operations And Lifecycle

1. Asset selection must be discoverable without relying only on hover.
2. The batch action surface supports select all, invert current selection, clear selection, ZIP download, batch publish, batch group, batch delete, and batch project transfer.
3. Select all and invert selection apply to the current filtered result set. The UI must clearly state whether unloaded matching records are included; the initial release may limit actions to loaded/selected IDs.
4. ZIP download must produce one archive, not multiple browser downloads.
5. Batch mutations return per-item success/failure results, patch successes locally, and preserve failed selections for retry.
6. Deleting an asset first makes it unavailable through business queries, then schedules physical object cleanup.
7. Object cleanup is asynchronous, idempotent, retryable, and safe across process restarts.
8. A physical object may be deleted only when no live generated result, reference alias, public record, or pending recovery record uses it.
9. A low-frequency reconciliation job must retry failed cleanup and detect orphaned objects/records.

## 6. Administration Requirements

### 6.1 Plans And Currency

1. Fixed points packages expose an `enable points expiry` switch. When enabled, validity days are required and effective in the current cashier completion path; when disabled, validity days are hidden/ignored and credited points never expire.
2. Existing fixed-package definitions are migrated with expiry enabled so their configured validity and current behavior are preserved. Newly created fixed packages default to expiry enabled.
3. An order snapshots both the expiry-enabled flag and validity days at creation. Later plan edits never change the expiry policy of an existing order or grant.
4. Subscription-plan definitions may retain their existing period semantics; only purchasable `points_package` plans are in this remediation's checkout scope.
5. Remove editable plan currency from the administration UI and write contract. Package price remains explicitly CNY.
6. Keep the database/order currency snapshot as `CNY` for historical compatibility.
7. Do not add exchange rates or multi-currency settlement in this remediation.
8. Plans support enable, disable, soft-delete/archive, deleted filtering, and restore-to-disabled.
9. These lifecycle controls already present in the source must be verified in the deployed bundle and made discoverable with clear labels/tooltips.
10. Plan transitions affect future purchase only and never mutate historical orders or grants.

### 6.2 Model And Pricing Lifecycle

1. Add delete actions for model accounts and their real account models.
2. Add delete actions for route models and route candidates/model capabilities.
3. Add delete actions for route-model resolution price rows.
4. Existing backend dependency conflicts must be surfaced with actionable guidance, such as removing route candidates before deleting an account model.
5. Model accounts, account models, and route models use soft deletion.
6. Route candidates and price rows may be physically deleted because image tasks retain immutable routing and pricing snapshots.
7. Historical call/task records must display their snapshotted route code, upstream model code, provider/account identity fallback, cost, and pricing even after current configuration is deleted.
8. Every lifecycle action requires confirmation and an audit event.

### 6.3 Cluster, Readiness, And Metrics

1. A full single-node deployment must show one logical cluster node.
2. The node identity must remain stable across process restarts and must not create one row per API/Worker process.
3. Distributed deployments continue to use heartbeat-backed node registration.
4. Payment readiness must use the same effective method-to-provider eligibility used by checkout scheduling.
5. Production readiness must exclude mock providers and require configured enabled provider instances for each reported usable method.
6. Documentation readiness must probe the resolved deployed docs/OpenAPI target.
7. Model-call distribution must be calculated from real image task/call records, not provider health weights.
8. The dashboard must provide at least route-model call count and percentage for a defined time window. Account-model and upstream-model breakdowns may be included in the same response.
9. Distribution totals must reconcile with the number of call records in the selected window, including explicit handling of preflight failures with no selected upstream model.
10. Every production image-task persistence path must reject, without truncation, a provider trace whose compact JSON exceeds 8 MiB or whose attempt list exceeds 10,000 entries. This safety boundary must not change platform image-count fan-out, retry, fallback, or partial-success behavior.
11. Distribution reads must reject an oversized historical provider trace using a stable sanitized error before transferring or deserializing that trace. The database transport limit may include explicit headroom for PostgreSQL JSONB text formatting, but decoded compact JSON must still satisfy the 8 MiB semantic limit and 10,000-attempt limit.
12. Distribution aggregation must use repeatable-read keyset pagination and bounded raw-trace sub-batches. It must preserve exact totals, equal timestamps, and soft-deleted task attempts without issuing one trace query per task.

## 7. Non-goals

1. No compatibility route or redirect for `/docs`.
2. No streaming image generation or partial-image events.
3. No rewrite of platform output fan-out, concurrency, retry, fallback, or partial-success settlement.
4. No requirement to follow the current OpenAI edits documentation model list.
5. No multi-currency checkout, exchange-rate service, foreign-currency refund, or settlement reconciliation.
6. No team collaboration, project membership, roles, invitations, or shared projects.
7. No generalized video asset implementation in this release; the project model must allow a future video entity to reference a project.
8. No retroactive expiration of existing long-lived wallet grants.
9. No physical deletion of historical payment, wallet ledger, image task, or audit records required for accounting/audit.

## 8. Acceptance Criteria

1. A newly paid expiring fixed package creates separate purchased and gift grants with the snapshotted expiry; a newly paid non-expiring fixed package creates long-lived purchased and gift grants; a custom recharge creates a long-lived recharge grant.
2. Recent order, order detail, profile balance, next expiry, and generation charge ledger display the required split and calculation fields.
3. No repository user-facing surface contains the invalid ICP number, and `/docs` is not registered by the user application.
4. Every docs entry opens the effective current documentation URL directly; readiness reports the same target's real availability.
5. Auto size produces an outbound request without `size`; ratio and pixel modes send only validated legal dimensions.
6. Invalid explicit pixels are rejected without silent normalization; invalid legacy configured options are hidden.
7. Transparent background cannot be submitted with JPEG from any API path.
8. Account-model max `n` outside `1-10` is rejected, while a platform task requesting more than ten images still succeeds through the existing fan-out behavior.
9. Importing an existing gallery image performs no object-store Copy or Put and later tasks can still load it as a reference.
10. First-reference import reuses compatible source parameters; later imports do not overwrite configuration.
11. Multi-image history opens overview before image detail.
12. Every existing asset belongs to exactly one default project after migration; project selection remains consistent between workspace and asset pages.
13. Deleting a non-empty project transfers its assets atomically to the selected target.
14. Batch download returns one valid ZIP and every batch mutation reports partial results.
15. Deleting a referenced image hides it immediately but retains its object until the last reference is gone; cleanup retries safely after injected storage failures.
16. Admin lifecycle actions are available and historical records remain understandable after configuration deletion.
17. Full deployment reports exactly one logical single node.
18. Model-call distribution contains real counts whose total reconciles with the selected call-record window; malformed or oversized historical traces fail with a sanitized error, and provider-trace writes and reads enforce the documented exact limits without truncation or unbounded batch materialization.
19. The observed size mismatch is diagnosable from one correlated task/request record without logging credentials or signed URLs.
20. Administrators can toggle fixed-package expiry. Existing and newly created packages default to enabled; disabling it hides/ignores validity days, and orders retain the creation-time policy after later plan edits.

## 9. Compatibility And Migration Rules

1. New additive database fields are introduced nullable or with backward-safe defaults, backfilled, then tightened in a later migration step.
2. Existing generated assets are backfilled in bounded batches to a per-user default project.
3. Existing copied gallery reference assets remain valid and continue owning their copied objects; only new imports use aliases.
4. Existing long-lived grants remain long-lived. The corrected expiry behavior applies only to orders completed after rollout.
5. Existing tasks with `requested_size=auto`, `base_resolution=auto`, or missing project data remain readable; new writes use the new contract.
6. Removal of `/docs` and editable documentation settings is intentionally breaking and requires deployment/release-note communication.
7. Existing fixed-package rows are backfilled with expiry enabled. No existing order or wallet grant changes expiry as part of this schema migration.

## 10. Source Report Traceability

| Source item | Final requirement decision | Primary section |
|---|---|---|
| User 1 | Recent orders show base points, bonus points, validity, and actual expiry without conflating payment expiry. | 5.1 |
| User 2 | Ledger shows successful count/unit/total; balance splits purchased, recharge, gift, and trial points and shows next expiry. | 5.1 |
| User 3 | Remove the nonexistent ICP number from every user-facing/demo surface. | 5.2 |
| User 4 | Developer-document buttons navigate directly to the effective documentation URL. | 5.2 |
| User 5 | Remove obsolete documentation title/base-path settings and readiness dependence. | 5.2, 6.3 |
| User 6 | Prompt text uses the full textarea width; actions overlay only the bottom-right area. | 5.3 |
| User 7 | Gallery reference import becomes a no-copy alias and performs no storage upload/copy. | 5.3, 5.7 |
| User 8 | The first reference reuses compatible source parameters; later references never overwrite them. | 5.3 |
| User 9 | Preserve requested versus actual dimensions and add correlated diagnostics before changing the local 1K 16:9 calculator. | 5.4 |
| User 10 | Redesign size as auto/ratio/pixel; remove base-resolution auto; filter unsupported values; add custom ratio and pixel bounds. | 5.4 |
| User 11 | Use the GPT Image generations-compatible field contract and one server-authoritative validator. | 5.4, 5.5 |
| User 12 | Multi-result history opens a task overview before shared image detail. | 5.3 |
| User 13 | Add mandatory project ownership and migrate legacy assets into a default project. | 5.6 |
| User 14 | Share and remember project selection across workspace and gallery, with ownership validation/fallback. | 5.6 |
| User 15 | Add project CRUD and atomic transfer-before-delete; default project is immutable. | 5.6 |
| User 16 | Restore discoverable selection and all requested batch actions, including one ZIP and project transfer. | 5.7 |
| User 17 | Soft-delete business records and perform durable, reference-safe asynchronous storage cleanup. | 5.7 |
| Admin 1 | Fixed points packages can enable validity or remain permanent; existing packages default to enabled, custom recharge remains long-lived, and user UI exposes the snapshotted policy. | 5.1, 6.1 |
| Admin 2 | Remove editable plan currency, retain CNY snapshots, and defer unsupported multi-currency settlement. | 6.1, 7 |
| Admin 3 | Verify/expose enable, disable, archive, and restore; plan deletion remains soft and affects future purchase only. | 6.1 |
| Admin 4 | Expose deletion for model accounts and real account models while preserving historical snapshots. | 6.2 |
| Admin 5 | Expose deletion for route models and candidates/capabilities while preserving historical snapshots. | 6.2 |
| Admin 6 | Expose deletion for resolution price rows while preserving historical pricing snapshots. | 6.2 |
| Admin 7 | A full single-node deployment reports one stable logical cluster node. | 6.3 |
| Admin 8 | Payment/docs readiness derives from executable effective configuration, not obsolete fields. | 6.3 |
| Admin 9 | Model-call distribution aggregates actual call records for an explicit window. | 6.3 |
| Follow-up 1 | Remove `/docs` as a breaking change with no redirect. | 5.2, 7, 9 |
| Follow-up 2 | Configure supported background values per model; transparent requires PNG/WebP. | 5.5 |
| Follow-up 3 | Do not implement stream or partial images. | 5.5, 7 |
| Follow-up 4 | Keep platform output fan-out; constrain only per-account-model upstream max `n` to 1-10. | 5.5, 7 |
| Follow-up 5 | Retain edits behavior; do not treat the current edits reference as normative. | 5.5, 7 |

## 11. Ownership And Schedule

| Role | Owner |
|---|---|
| Backend, migrations, worker | To be assigned by Tech Lead |
| User Web | To be assigned by Tech Lead |
| Admin Web | To be assigned by Tech Lead |
| QA | To be assigned by QA Lead |
| Deployment/SRE | To be assigned by SRE owner |

Implementation schedule is intentionally not estimated in this requirement document. Milestones and sequencing are defined in the associated technical design.
