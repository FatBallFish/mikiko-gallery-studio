# Plan, Public Image, and Media Delivery Runbook

This runbook covers subscription-plan lifecycle operations, public-image moderation, and temporary media delivery from S3-compatible storage, including Cloudflare R2.

## Lifecycle Semantics

### Subscription plans

| State | Visible to buyers | Purchasable | Default admin list | Allowed next actions |
| --- | --- | --- | --- | --- |
| `active` | yes | yes | yes | disable, archive |
| `disabled` | no | no | yes | enable, archive |
| `archived` | no | no | no | restore |

Archive is a soft delete. Restoring an archived plan always produces a disabled, non-purchasable plan. Plan transitions never rewrite historical order snapshots, granted points, wallet ledger entries, or subscriptions.

### Public images

| State | User meaning | Public gallery | Admin action |
| --- | --- | --- | --- |
| `private` | private asset | hidden | none |
| `pending_review` | publication requested | hidden | approve or reject |
| `approved` | public | visible | unpublish with a reason |
| `rejected` | publication rejected | hidden | retain moderation record |
| `unpublished` | removed by an administrator | hidden | retain removal reason |

The owner may cancel `pending_review` or `approved`, returning the image to `private`. A later publication request is allowed. Cancellation or unpublishing removes the item from new list/detail responses immediately, but an already issued temporary object URL can remain usable until its five-minute expiry.

## Media Delivery Contract

The database stores `storage_config_id`, `storage_driver`, and `object_key`; it does not store projected temporary URLs. After authorization, the image-task and reference-asset services resolve the recorded storage instance and project response-only URLs.

- A backend implementing `storage.TemporaryURLSigner` returns separate five-minute preview and download URLs. Download signing may set a response filename.
- Local storage, legacy rows, and backends without signing support use an authenticated application fallback route.
- A signing error on a signing-capable backend returns `STORAGE_CONFIG_UNAVAILABLE`; it does not silently proxy bytes.
- Absolute HTTP(S) URLs are consumed unchanged by the frontend. Application access tokens are added only to relative fallback routes.
- Signed query strings must not be persisted, placed in audit payloads or analytics, or written to logs. Log only the storage config ID, object key, image/asset ID, and query-free host/path.

Direct projection is preferred over a 307 redirect because it removes an application request and avoids spending API bandwidth on normal S3/R2 reads. The fallback handlers remain required for local storage, non-signing backends, legacy clients, and rollback compatibility.

## Endpoint Audit Matrix

The audit command is:

```bash
rg -n 'download_url|preview_url|image_url|/api/agent/image/v1/images|reference-assets/.*/download' internal web api
```

The 2026-08-05 audit classified 171 matches as follows. Field definitions and fixtures are listed by group because they do not execute a media read.

| Surface or source | Projection / classification | Intentional fallback |
| --- | --- | --- |
| Agent task detail, list, SSE, and history | `imagetask.Service.GetByID`, `ListByUser`, and `projectTaskMedia` project every result | `/api/agent/image/v1/images/{id}` |
| Private gallery and user publish/group transitions | `ListGalleryByUser`, `RequestPublish`, `CancelPublish`, and `SetImageGroup` use `projectGalleryImageMedia` | `/api/agent/image/v1/images/{id}` |
| Home and public gallery list/detail/interactions | `ListPublicGallery`, `GetPublicImage`, and `SetPublicImageInteraction` preserve signed `url` and `download_url` | `/api/open/image/v1/gallery/images/{id}/image` |
| Admin review list and mutation responses | `ListGallery` and `ProjectGalleryImageForAdmin` project the image and reference assets | `/api/ops/admin/v1/image-reviews/{id}/image` |
| Reference upload, gallery import, detail, and Open API upload/detail | `assets.Service.ProjectURLs` projects `preview_url` and `download_url`; payload code fills a route only when projection is absent | agent or Open API `reference-assets/{id}/download` |
| Admin model-account test image | `TestModelAccount` projects the persisted test result before returning `image_url` | admin image-review endpoint |
| Provider `image_url` and input image URL occurrences | Provider wire format or upstream result source; generated results are mirrored before normal response projection | not a delivery endpoint |
| Domain types, shared API types, and OpenAPI schemas | Contract definitions for `url`, `download_url`, `preview_url`, and `image_url` | not executable |
| Router and handler occurrences | Compatibility/fallback route registration and byte/redirect delivery after authentication | intentionally retained |
| Tests, contracts, mocks, generated docs, and `web/redesign-demo` | Fixtures or non-production design copies that verify direct and fallback behavior | not production consumers |

Frontend consumers are centralized through `web/shared/media-url.ts`. Home, workspace/history, private gallery, public gallery, and admin review request one bounded resource refresh after an absolute signed URL fails. A successful load resets that component's retry gate. Relative fallback failures do not trigger URL refresh loops.

## Verification

For an S3/R2-backed image, inspect API JSON without printing the signed query:

```bash
curl -sS -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "${BASE_URL}/api/agent/image/v1/tasks/${TASK_ID}" \
  | jq -r '.data.results[] | .url | split("?")[0]'
```

Expected: an object-storage host, not an application image route.

Check the public list similarly:

```bash
curl -sS "${BASE_URL}/api/open/image/v1/gallery/images?page=1&page_size=10" \
  | jq -r '.data.items[] | .url | split("?")[0]'
```

For local storage, the same fields should contain an application fallback route. Confirm that a request without the required user/admin/Open API credential is rejected.

Run repository coverage after a media or lifecycle change:

```bash
go test ./internal/service/billing ./internal/service/imagetask ./internal/service/assets ./internal/repository/entstore ./internal/http/router -count=1
./scripts/workflow/verify-contracts.sh
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
```

## Operational Risks

- Temporary URLs are bearer capabilities. Keep the five-minute expiry and do not paste complete URLs into tickets, chat, dashboards, or logs.
- Cancellation and unpublishing cannot revoke an already signed URL immediately. For emergency revocation, remove or quarantine the object and accept that current image loads will fail.
- Browser refresh after expiry depends on the list/detail API remaining healthy. Repeated media failures should be investigated as storage signing, CORS, DNS, or object availability issues rather than retried indefinitely.
- Do not disable or delete a storage configuration while database rows still reference it. Historical objects must continue to resolve through their recorded storage instance.
- Public and admin projections must not expose reference assets that the caller is not authorized to inspect.

## Rollback

Rolling back application code restores fallback-route delivery but does not revert plan or publication states. Keep the existing download handlers and all referenced storage configurations readable during rollback. Do not rewrite historical order or media metadata.
