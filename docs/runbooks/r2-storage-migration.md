# R2 / Multi-S3 Storage Migration Runbook

## Scope

This runbook covers moving Pic Gallery from a single startup storage backend to admin-managed multi-instance S3-compatible storage, including Cloudflare R2.

R2 is configured as `driver=s3` and `provider=r2`. Do not introduce a separate R2 storage driver.

For response URL projection, fallback endpoints, expiration, and lifecycle checks, also follow [Plan, Public Image, and Media Delivery Runbook](./plan-public-media-lifecycle.md).

## Preconditions

- API and worker are both deployed with multi-storage code.
- Database migrations have created `object_storage_configs`, `task_images.storage_config_id`, and `reference_assets.storage_config_id`.
- The admin console is reachable over HTTPS.
- The operator has `manage:dangerous_config`.
- A Cloudflare R2 bucket, access key ID, and secret access key are ready.

Recommended R2 values:

| Field | Value |
|---|---|
| Driver | `s3` |
| Provider | `r2` |
| Endpoint | `https://<ACCOUNT_ID>.r2.cloudflarestorage.com` |
| Region | `auto` |
| Force path-style | `false` |
| Permissions | Object read/write for the target bucket only |

## Phase 1: Confirm Bootstrap Storage

On first startup after the upgrade, the service creates a bootstrap storage config from the existing startup storage settings when the table is empty.

Check that one bootstrap config exists:

```sql
select id, code, driver, provider, status, read_enabled, write_enabled, is_default
from object_storage_configs
order by created_at;
```

Expected:

- `code` is `bootstrap-local` or `bootstrap-s3`.
- `read_enabled=true`.
- Existing deployments should keep this config readable even after R2 becomes default.

## Phase 2: Backfill Historical Object Rows

Identify the bootstrap config ID:

```sql
select id
from object_storage_configs
where code in ('bootstrap-local', 'bootstrap-s3')
order by created_at
limit 1;
```

Backfill generated images and reference assets that predate multi-storage routing:

```sql
update task_images
set storage_config_id = '<bootstrap-config-id>'
where storage_config_id is null
  and storage_driver in ('local', 's3')
  and object_key <> '';

update reference_assets
set storage_config_id = '<bootstrap-config-id>'
where storage_config_id is null
  and storage_driver in ('local', 's3')
  and object_key <> '';
```

Verify there are no remaining local/S3 rows without a config ID:

```sql
select storage_driver, count(*)
from task_images
where storage_config_id is null
  and storage_driver in ('local', 's3')
  and object_key <> ''
group by storage_driver;

select storage_driver, count(*)
from reference_assets
where storage_config_id is null
  and storage_driver in ('local', 's3')
  and object_key <> ''
group by storage_driver;
```

Expected: no rows, or all counts are `0`.

## Phase 3: Create and Probe R2

In the admin console storage page:

1. Create a storage config with `driver=s3`, `provider=r2`, `region=auto`, and the R2 endpoint.
2. Enter `access_key_id` and `secret_access_key`.
3. Start with `read_enabled=true` and `write_enabled=false`.
4. Run Probe.

Probe must complete Put/Get/Delete against a `.pic-gallery-probe/` object. Do not set the config as default unless `last_probe.status=success`.

## Phase 4: Switch Default Writes

After Probe succeeds:

1. Update the R2 config to `write_enabled=true`.
2. Click Set Default.
3. Generate or upload one test image.
4. Confirm the new row points to the R2 config:

```sql
select id, storage_config_id, storage_driver, object_key, created_at
from task_images
order by created_at desc
limit 5;
```

5. Download the test image through the normal API/UI.
6. Confirm an older historical image still downloads from the bootstrap config.

Observe for at least 15 minutes:

- API and worker error logs.
- Storage read/write failures by `storage_config_id`.
- Image generation success rate.
- Image download success rate and latency.

## Rollback

To stop new writes to R2, set the bootstrap storage config as default again.

Do not disable the R2 config after rollback if any images were written to it. Those rows keep `storage_config_id=<r2-config-id>` and still require R2 reads.

Recommended rollback state:

| Config | read_enabled | write_enabled | is_default |
|---|---:|---:|---:|
| bootstrap legacy storage | true | true | true |
| R2 storage with written objects | true | false | false |

## Hard Rules

- Do not route historical reads through the current default storage.
- Do not delete or fully disable a storage config while `task_images` or `reference_assets` still reference it.
- Do not log or export plaintext storage secrets.
- Do not make R2 default until all API and worker instances run the multi-storage version.
