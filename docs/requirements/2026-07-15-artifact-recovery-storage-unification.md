# Image Artifact Recovery and Storage Unification Requirements

## Background

Task `ca0f6005-c9f8-4734-a2d2-222509f8b5ac` completed its paid upstream image generation call, then failed while reading the returned image URL. The task was marked `IMAGE_STORAGE_FAILED`, the user's reserved points were refunded, and the platform absorbed the upstream cost. The same investigation also found that the admin storage probe and the running worker could resolve different storage configurations.

## Goals

1. Persist proof of upstream success before downloading or storing generated artifacts.
2. Recover artifact download and storage failures without invoking image generation again.
3. Preserve actionable, sanitized failure diagnostics after every artifact attempt.
4. Make database-backed storage configuration authoritative for every API and worker instance.
5. Keep historical assets readable after changing the default write storage.
6. Settle user points only after every successfully generated output is durably stored and readable.

## Acceptance Criteria

### Artifact recovery

- After an upstream generation succeeds, the task durably records the provider request ID, upstream timing, output recovery payload, and selected storage configuration before artifact persistence starts.
- Artifact persistence makes one initial attempt and, after failure, up to three additional automatic retries.
- Retries may download, validate, and store an existing upstream result. They must never call the generation provider again.
- Retry delays are `1s`, `3s`, and `10s` unless the worker is restarted; a restarted worker may immediately claim an overdue retry.
- Recovery state survives worker restart and may be claimed safely by another worker without concurrent duplicate processing.
- Signed source URLs are encrypted at rest and are never returned through normal task APIs or written verbatim to logs.
- Inline base64 output remains recoverable across worker restart without retaining the payload after successful storage.
- Successful recovery creates the normal image result, clears recovery payloads, settles billing once, and completes the task.
- After four failed persistence attempts, the task becomes failed, reserved user points are refunded once, and the final sanitized cause is retained.

### Diagnostics

- The system distinguishes URL construction, HTTP status, timeout, connection/read failure, empty body, size limit, unsupported format, storage configuration, and storage write failures.
- Diagnostic records include attempt number, stage, timestamps, duration, URL host, HTTP status, response content type, declared content length, bytes read, storage config ID/version, and a sanitized wrapped cause where available.
- Provider success attempts are recorded even when artifact persistence later fails.
- Provider request IDs and configured provider costs remain available for loss accounting.

### Storage configuration

- `object_storage_configs` is the authoritative runtime source for API and worker processes.
- Environment storage settings create the bootstrap record only when no storage configuration exists.
- Admin probes and production reads/writes use the same configuration resolver and backend factory.
- New writes use the current default writable configuration and persist its `storage_config_id` on the asset or image result.
- Reads and deletes use the resource's recorded `storage_config_id`; changing the default does not move or orphan historical resources.
- Storage configuration changes invalidate caches across all API and worker replicas through Redis notification, with a bounded database-refresh TTL when Redis is unavailable.
- A configuration must pass a real `put/get/delete` probe before it can become the default writer.

### Billing and loss visibility

- User points stay reserved while artifact recovery is pending.
- Billing finalization is idempotent and occurs only after the stored result is readable.
- Exhausted artifact recovery refunds reserved points exactly once.
- Upstream-success/platform-failure tasks can be counted separately from upstream failures.
- Provider cost configuration is preserved in the task trace so platform loss is measurable.

## Non-goals

- A dedicated artifact worker service or new message broker.
- Migrating existing objects between storage backends.
- Charging users for results that were not made available to them.
- Retrying or repeating the upstream image generation request during artifact recovery.
