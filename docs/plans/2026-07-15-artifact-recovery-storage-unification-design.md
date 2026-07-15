# Image Artifact Recovery and Storage Unification Design

## Context

The current image task path treats provider generation and artifact persistence as one attempt. A provider may return a successful, billable result URL, but a subsequent download or object-store failure immediately fails the task and refunds the user. The original provider result is not durably retained before persistence, so the worker cannot recover without generating again.

The current main branch also initializes API and worker storage backends from process configuration. The admin console stores and probes database-backed object storage records, but those records are not necessarily used by running task and asset services. Commit `290657b` contains a useful multi-storage router design, but it does not implement artifact recovery or active cross-process cache invalidation and cannot be migrated wholesale onto the current main branch.

## Chosen Approach

Use the existing image worker and database task lease mechanism. Add a durable artifact recovery envelope to image tasks, then make artifact persistence a resumable phase of task execution. Adapt the multi-storage router around the current schema and services, using the database as the source of truth and Redis only for cache invalidation.

This avoids a new worker service and queue while ensuring retries survive worker restart and never repeat generation.

## Task State and Data Model

The public task status remains compatible with the current queued/running/succeeded/failed contract. Internal progress and recovery fields represent the finer state:

```text
queued
  -> running / provider_call
  -> running / artifact_persisting
  -> running / artifact_recovery_pending
  -> succeeded

artifact_recovery_pending
  -> artifact_persisting (claim due retry)
  -> failed (four total persistence attempts exhausted)
```

Add durable task fields for:

- provider request ID and upstream success timestamp;
- encrypted recovery payload containing source URLs or inline output content;
- artifact attempt count and next retry timestamp;
- artifact stage and sanitized last-error detail;
- storage configuration ID and version selected for the output;
- provider call attempts recorded independently of artifact attempts.

The recovery payload is encrypted with the existing secure configuration encryption key. URL query parameters and inline image bytes never enter plain JSON traces. The payload is cleared after successful persistence or terminal failure. Inline content may temporarily increase database storage, but it is bounded by the existing generated-image size limit and removed promptly; this is preferable to losing a paid result or relying on worker-local disk.

Repository acquisition treats overdue `artifact_recovery_pending` tasks as claimable work alongside queued tasks. Existing lease ownership and heartbeat rules prevent concurrent processing. A claimed recovery task bypasses provider resolution and calls.

## Execution Flow

For a new task:

1. Claim the task and call the selected provider once.
2. On provider failure, retain current provider retry/fallback behavior.
3. On provider success, immediately append the successful provider attempt and durably save the encrypted recovery envelope plus provider request ID.
4. Resolve and pin the current default writable storage configuration.
5. Attempt artifact persistence.
6. On a retryable persistence failure, increment the attempt count, persist diagnostic detail, set the next retry time using `1s`, `3s`, then `10s`, release the lease, and leave billing reserved.
7. On the fourth failed attempt, clear sensitive recovery payloads, finalize the task as failed, and refund reserved points idempotently.
8. On success, save image results, verify the stored object can be read, clear recovery payloads, finalize billing once, and mark the task succeeded.

An artifact retry never invokes `Generate` or `Edit`. The current user-facing retry endpoint remains for genuine failed task resubmission, but artifact recovery is automatic and does not require a user action.

Non-retryable failures include an invalid source URL, unsupported image format, and a payload exceeding the configured limit. Transient download, response read, storage configuration, and storage write failures consume the automatic retry budget.

## Diagnostics

Introduce stable internal artifact error codes:

- `ARTIFACT_SOURCE_URL_INVALID`
- `ARTIFACT_FETCH_HTTP_STATUS`
- `ARTIFACT_FETCH_TIMEOUT`
- `ARTIFACT_FETCH_CONNECTION_FAILED`
- `ARTIFACT_FETCH_READ_FAILED`
- `ARTIFACT_EMPTY_BODY`
- `ARTIFACT_SIZE_LIMIT_EXCEEDED`
- `ARTIFACT_FORMAT_UNSUPPORTED`
- `ARTIFACT_STORAGE_CONFIG_UNAVAILABLE`
- `ARTIFACT_STORAGE_WRITE_FAILED`
- `ARTIFACT_STORAGE_VERIFY_FAILED`

Each artifact attempt records stage, attempt number, start/end time, duration, URL scheme/host/path without query, HTTP status, response content type, declared content length, bytes read, storage config ID/version, retryability, and a sanitized wrapped error. Normal APIs expose only stable public errors. Admin call records and structured logs expose the diagnostic fields but never credentials, signed query strings, or inline data.

The provider success attempt is persisted before artifact work, fixing the current empty `provider_trace.attempts` behavior when storage fails.

## Unified Storage Router

Adapt the useful parts of commit `290657b` instead of cherry-picking it:

- retain `Router.DefaultWriter` and `Router.BackendFor` concepts;
- construct backends from resolved `object_storage_configs` records;
- persist `storage_config_id` on reference assets and generated image results;
- route reads/deletes to the recorded configuration;
- preserve a legacy-driver fallback only for rows created before `storage_config_id` existed.

API and worker processes each create the same database-backed router. The router caches immutable backend instances by configuration ID plus version. Admin create/update/status/default operations publish a Redis invalidation event after the database commit. Every API and worker replica subscribes and evicts the affected ID/default selection. A short TTL remains as convergence protection if notification delivery or Redis is unavailable.

Environment variables are bootstrap input only. On startup, the storage configuration service inserts a default bootstrap record only when the table is empty. Once any database configuration exists, process environment cannot silently override it.

Admin probes call the same resolver and backend factory used by production traffic. Setting a default requires enabled read/write flags and a successful `put/get/delete` probe. Default selection and cache invalidation are ordered so new writes converge on the committed database default.

Historical resources retain their original storage config ID. Default changes affect new writes only; no implicit data migration occurs.

## Billing and Loss Accounting

Reserved points remain frozen during recovery. Successful artifact verification precedes billing finalization. Existing billing idempotency keys ensure only one consume or refund ledger entry is created across retries and worker races.

Tasks with provider success followed by exhausted artifact recovery are identifiable by the provider success timestamp and terminal artifact code. They contribute to a platform-loss count and configured provider cost total, rather than being grouped with upstream failures. Provider cost configuration itself is operational data and must be populated separately; this implementation preserves and reports it but does not invent missing prices.

## Failure and Concurrency Handling

- A worker crash after saving provider success leaves a claimable recovery task.
- A worker crash after object write but before database save retries with the same deterministic object key, making the write idempotent.
- Lease conflicts use the existing terminal recovery path and billing idempotency.
- Configuration deletion is logical; a configuration referenced by historical resources remains resolvable for reads while marked readable.
- Redis failure does not block storage operations because routers revalidate through TTL-based database resolution.
- Storage verification failure consumes the same persistence retry budget and does not charge the user.

## Testing Strategy

Unit and repository tests will prove:

- provider success is persisted before artifact work;
- one initial persistence attempt plus three retries;
- transient failures recover without a second provider call;
- fourth failure produces the final detailed cause and one refund;
- recovery continues after service/worker reconstruction;
- deterministic keys make post-write retries safe;
- signed URLs and inline bytes are encrypted and absent from traces;
- backend selection follows database default changes;
- Redis invalidation updates multiple router instances;
- TTL fallback converges without Redis;
- historical reads use recorded storage config IDs;
- admin probes and task writes use the same backend factory.

Repository verification and local API smoke will exercise task creation, worker processing, storage default switching, artifact recovery, billing settlement, and historical reads.

## Rollout

1. Apply additive schema changes and deploy API/worker versions that understand both legacy and new rows.
2. Bootstrap the current environment-backed storage into the database only if no records exist.
3. Verify the configured default through the admin probe before enabling writes.
4. Deploy API and worker replicas with Redis invalidation enabled.
5. Monitor artifact recovery counts, exhausted recoveries, storage route resolution errors, and upstream-success/platform-failure cost.

Rollback keeps existing stored objects intact. Legacy rows continue resolving by their recorded driver; new recovery fields are additive and may remain unused by an older binary, so rollback should occur only when no tasks are in `artifact_recovery_pending`.
