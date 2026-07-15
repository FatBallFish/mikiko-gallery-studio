# Artifact Recovery and Storage Unification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Recover paid upstream image results with one initial persistence attempt plus three automatic retries, while making database-backed storage configuration authoritative across every API and worker instance.

**Architecture:** Extend image tasks with an encrypted recovery envelope and retry schedule that uses the existing task lease. Adapt the multi-storage registry pattern to the current main branch, persist storage configuration identity on every resource, and invalidate per-process router caches with Redis plus a bounded database-refresh TTL.

**Tech Stack:** Go, Ent, PostgreSQL, Redis Pub/Sub, existing worker leases, existing billing idempotency, Docker Compose, Go `httptest`.

---

### Task 1: Add durable recovery and storage identity fields

**Files:**
- Modify: `internal/domain/imagetask/types.go`
- Modify: `internal/provider/contracts.go`
- Modify: `internal/domain/assets/types.go`
- Modify: `internal/repository/ent/schema/imagetask.go`
- Modify: `internal/repository/ent/schema/imageresult.go`
- Modify: `internal/repository/ent/schema/referenceasset.go`
- Create: `internal/repository/ent/schema/objectstorageconfig.go`
- Modify: `internal/repository/ent/schema/schema_test.go`
- Modify: `internal/repository/ent/migrations/000001_init.sql`
- Regenerate: `internal/repository/ent/**`

**Step 1: Write failing schema tests**

Add assertions that `image_tasks` contains provider request/recovery status, encrypted payload, attempt count, next retry time, last diagnostic, and upstream success time; assert `task_images` and `reference_assets` contain nullable `storage_config_id`; assert `object_storage_configs` exists with unique code and default-writer indexes.

**Step 2: Run the schema test and verify RED**

Run: `go test ./internal/repository/ent/schema -run 'TestSchema.*(ArtifactRecovery|StorageConfig)' -count=1`

Expected: FAIL because the new fields/schema are absent.

**Step 3: Add domain fields and Ent schemas**

Represent recovery with explicit domain types:

```go
type ArtifactRecovery struct {
    Status           string
    EncryptedPayload string
    AttemptCount     int
    NextRetryAt      *time.Time
    LastDiagnostic  ArtifactDiagnostic
    StorageConfigID string
    StorageVersion  int64
}

type ArtifactDiagnostic struct {
    Code, Stage, URLHost, URLPath, ContentType, Cause string
    Attempt, HTTPStatus int
    ContentLength, BytesRead, DurationMS int64
    Retryable bool
    StartedAt, FinishedAt time.Time
}
```

Add `StorageConfigID` to reference assets and image results. Keep new columns additive and nullable/defaulted for legacy rows.

**Step 4: Regenerate Ent and verify GREEN**

Run: `go generate ./internal/repository/ent`

Run: `go test ./internal/repository/ent/schema -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain internal/provider internal/repository/ent
git commit -m "feat: add durable artifact recovery schema"
```

### Task 2: Implement the database storage configuration service

**Files:**
- Create: `internal/domain/storageconfig/types.go`
- Create: `internal/service/storageconfig/store.go`
- Create: `internal/service/storageconfig/service.go`
- Create: `internal/service/storageconfig/service_test.go`
- Create: `internal/repository/entstore/storage_config_store.go`
- Create: `internal/repository/entstore/storage_config_store_test.go`

**Step 1: Write failing service tests**

Cover empty-table bootstrap, no environment overwrite when records exist, encrypted secret preservation, probe-required default switching, default uniqueness, historical readable resolution, and optimistic version conflict.

**Step 2: Verify RED**

Run: `go test ./internal/service/storageconfig ./internal/repository/entstore -run 'Test(StorageConfig|StorageBootstrap)' -count=1`

Expected: FAIL because the service/store do not exist.

**Step 3: Implement the current-main service**

Adapt validation and encrypted-secret handling from commit `290657b`, but use the current secret codec, schema, repository conventions, and error types. Make `Bootstrap` insert only when `List` returns no records. Resolve the default writer on every source lookup and resolve historical records by ID even when no longer default.

**Step 4: Verify GREEN**

Run: `go test ./internal/service/storageconfig ./internal/repository/entstore -run 'Test(StorageConfig|StorageBootstrap)' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/storageconfig internal/service/storageconfig internal/repository/entstore/storage_config_store*
git commit -m "feat: add database storage configuration service"
```

### Task 3: Add a versioned router and cross-process invalidation

**Files:**
- Create: `internal/storage/router.go`
- Create: `internal/storage/router_test.go`
- Create: `internal/storage/invalidation.go`
- Create: `internal/storage/invalidation_test.go`
- Modify: `internal/storage/backend.go`

**Step 1: Write failing router tests**

Prove that the router selects the database default, caches by config ID/version/fingerprint, routes historical resources by ID, falls back by driver only for legacy rows, evicts on notification, and refreshes after TTL when notifications are unavailable.

Use two registry instances against one fake source and one in-memory invalidation bus to demonstrate cross-instance convergence.

**Step 2: Verify RED**

Run: `go test ./internal/storage -run 'Test(Registry|Invalidation|Probe)' -count=1`

Expected: FAIL because router and invalidation APIs are absent.

**Step 3: Implement router and invalidation contracts**

Define `DefaultWriter`, `BackendFor`, `Probe`, and `Invalidate`. Cache immutable backends by `id:version:fingerprint`; cache default selection only for the bounded TTL. Define a small publisher/subscriber interface and Redis implementation that carries only storage config ID/version and a default-changed flag.

The subscriber must run under a passed `context.Context`, reconnect through go-redis behavior, sanitize logging, and close cleanly. Redis failure must not prevent database resolution.

**Step 4: Verify GREEN**

Run: `go test ./internal/storage -run 'Test(Registry|Invalidation|Probe)' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage
git commit -m "feat: route storage through database configuration"
```

### Task 4: Route assets and image results through the shared storage registry

**Files:**
- Modify: `internal/service/assets/service.go`
- Modify: `internal/service/assets/service_test.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/imagetask/service_test.go`
- Modify: `internal/repository/entstore/assets_store.go`
- Modify: `internal/repository/entstore/assets_store_test.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Modify: `internal/repository/entstore/imagetask_store_test.go`

**Step 1: Write failing routing tests**

Test that new reference assets and generated images store `storage_config_id`; reads and deletes use that ID after the default changes; legacy rows without an ID resolve by driver; and a probe-created backend behaves identically to a task writer backend.

**Step 2: Verify RED**

Run: `go test ./internal/service/assets ./internal/service/imagetask ./internal/repository/entstore -run 'Test.*Storage(Config|Routing|Historical)' -count=1`

Expected: FAIL because services still hold a static backend.

**Step 3: Replace static backend ownership with `storage.Router`**

Keep static-router constructors for isolated tests and backwards-compatible call sites. Pin the chosen writer ID/version before artifact persistence. Route downloads and deletes using stored IDs. Use deterministic generated object keys so an uncertain post-write retry overwrites the same key.

**Step 4: Verify GREEN**

Run: `go test ./internal/service/assets ./internal/service/imagetask ./internal/repository/entstore -run 'Test.*Storage(Config|Routing|Historical)' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/service/assets internal/service/imagetask internal/repository/entstore
git commit -m "feat: persist resources with storage configuration identity"
```

### Task 5: Persist provider success and encrypted recovery payloads

**Files:**
- Create: `internal/service/imagetask/artifact_recovery.go`
- Create: `internal/service/imagetask/artifact_recovery_test.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/imagetask/service_test.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Modify: `internal/repository/entstore/imagetask_store_test.go`

**Step 1: Write failing recovery-envelope tests**

Use a provider that counts calls and returns a signed URL. Make the first storage operation fail. Assert that the persisted task already contains one successful provider attempt, provider request ID, upstream success timestamp, encrypted recovery payload, and no raw query string in `provider_trace` or error text.

Add a base64 case that reconstructs the service/store and can still recover the payload.

**Step 2: Verify RED**

Run: `go test ./internal/service/imagetask ./internal/repository/entstore -run 'Test.*(ProviderSuccessBeforeArtifact|EncryptedRecovery)' -count=1`

Expected: FAIL because provider success is currently saved only after persistence.

**Step 3: Implement recovery codec and durable checkpoint**

Use the configured secure encryption key and existing secret codec conventions. Persist the checkpoint with the owned lease before the first artifact attempt. Ensure task/API mapping omits encrypted payload and inline bytes.

**Step 4: Verify GREEN**

Run: `go test ./internal/service/imagetask ./internal/repository/entstore -run 'Test.*(ProviderSuccessBeforeArtifact|EncryptedRecovery)' -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/service/imagetask internal/repository/entstore
git commit -m "feat: checkpoint paid provider results before storage"
```

### Task 6: Implement automatic artifact recovery and detailed errors

**Files:**
- Modify: `pkg/errs/codes.go`
- Modify: `internal/service/imagetask/artifact_recovery.go`
- Modify: `internal/service/imagetask/artifact_recovery_test.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/imagetask/service_test.go`
- Modify: `internal/service/imagetask/store.go`
- Modify: `internal/repository/entstore/imagetask_store.go`
- Modify: `internal/repository/entstore/imagetask_store_test.go`
- Modify: `internal/worker/runner_test.go`

**Step 1: Write one failing test per recovery behavior**

Cover:

- initial attempt plus retries after `1s`, `3s`, and `10s`;
- success on the fourth total attempt;
- exhaustion after four total attempts with one refund;
- no second provider call under any artifact failure;
- worker/service reconstruction claims an overdue recovery;
- HTTP timeout, connection reset, non-200, empty body, over-limit, invalid format, storage resolution/write/verify errors map to distinct diagnostics;
- object write followed by uncertain save retries the deterministic key;
- billing remains reserved while pending and consumes only after read verification.

Inject a clock/scheduler rather than sleeping in unit tests.

**Step 2: Verify RED**

Run: `go test ./internal/service/imagetask ./internal/repository/entstore ./internal/worker -run 'Test.*Artifact' -count=1`

Expected: FAIL for missing recovery scheduling and diagnostics.

**Step 3: Implement the recovery state machine**

Make overdue recovery tasks eligible in `AcquireNextQueuedTask`. `ExecuteLeasedTask` checks the encrypted recovery envelope before resolving/calling a provider. A transient failure saves `artifact_recovery_pending`, next retry time, released lease, and reserved billing. The fourth total failure clears sensitive payload, settles a refund idempotently, and records the terminal diagnostic.

Preserve wrapped causes internally with `%w`, but sanitize signed URLs and credentials before persistence/logging.

**Step 4: Verify GREEN**

Run: `go test ./internal/service/imagetask ./internal/repository/entstore ./internal/worker -run 'Test.*Artifact' -count=1`

Expected: PASS without real sleeps or duplicate provider calls.

**Step 5: Commit**

```bash
git add pkg/errs internal/service/imagetask internal/repository/entstore internal/worker
git commit -m "feat: automatically recover generated image artifacts"
```

### Task 7: Wire database storage into API, worker, and admin operations

**Files:**
- Modify: `internal/app/run.go`
- Modify: `internal/app/worker.go`
- Modify: `internal/app/redis.go`
- Modify: `internal/app/storage_validation.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/router.go`
- Create: `internal/http/router/admin_storage_config_api_test.go`
- Modify: `api/openapi/openapi.yaml`
- Modify: `api/openapi/components/schemas/admin.yaml`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/shared/api-types.ts`

**Step 1: Write failing integration tests**

Start two routers/services against one database source and invalidation bus. Update and activate a probed storage config through the admin API. Assert both API and worker service instances use the new default, while an old image remains readable from its original config.

Assert bootstrap environment values are ignored after database records exist.

**Step 2: Verify RED**

Run: `go test ./internal/app ./internal/http/router -run 'Test.*Storage(Config|Propagation|Bootstrap)' -count=1`

Expected: FAIL because app wiring still creates static backends and admin endpoints are absent.

**Step 3: Implement application lifecycle wiring**

Create one storage config service and router per process. API publishes invalidations after committed admin mutations; API and worker both subscribe using the process context. Pass the same router to asset, image task, download, delete, and probe paths. Update contracts only for admin storage operations and sanitized diagnostic views.

**Step 4: Verify GREEN and frontend contracts**

Run: `go test ./internal/app ./internal/http/router -run 'Test.*Storage(Config|Propagation|Bootstrap)' -count=1`

Run: `npm --prefix web/admin run typecheck`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/app internal/http api/openapi web/shared
git commit -m "feat: synchronize storage configuration across services"
```

### Task 8: Add loss visibility and end-to-end smoke coverage

**Files:**
- Modify: `internal/domain/admincallrecord/types.go`
- Modify: `internal/repository/entstore/admin_call_record_store.go`
- Modify: `internal/repository/entstore/admin_call_record_store_test.go`
- Modify: `scripts/test/api_contract_smoke.sh`
- Modify: `scripts/workflow/api-smoke.sh` if required
- Modify: `docs/runbooks/backend-deployment.md`

**Step 1: Write failing call-record and smoke assertions**

Assert admin records distinguish upstream failure from upstream-success/artifact-failure, expose sanitized artifact attempt diagnostics, preserve provider request ID/cost, and never expose recovery ciphertext or signed URLs.

Extend smoke with a fake provider whose result URL fails twice then succeeds, and assert one provider request, three persistence attempts, one consume ledger, successful historical read after changing defaults, and synchronized API/worker storage selection.

**Step 2: Verify RED**

Run: `go test ./internal/repository/entstore -run 'TestAdminCallRecord.*Artifact' -count=1`

Expected: FAIL for missing diagnostic/loss mapping.

**Step 3: Implement sanitized admin mapping and smoke fixtures**

Expose only stable diagnostic fields. Document database-authoritative storage, bootstrap behavior, Redis invalidation fallback, recovery metrics, and rollback restrictions.

**Step 4: Run focused GREEN checks**

Run: `go test ./internal/repository/entstore -run 'TestAdminCallRecord.*Artifact' -count=1`

Run: `./scripts/workflow/api-smoke.sh`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/admincallrecord internal/repository/entstore scripts docs/runbooks
git commit -m "test: cover artifact recovery and storage propagation"
```

### Task 9: Full verification and review gate

**Files:**
- Update only files required by verification failures caused by this change.

**Step 1: Format and inspect secrets**

Run: `gofmt -w <changed-go-files>`

Run: `git diff --check`

Run: `git diff | rg -i '(access[_-]?key|secret|token|signed.*url)'`

Expected: no credentials or raw signed URLs.

**Step 2: Run centralized verification**

Run: `./scripts/workflow/verify.sh`

Expected: Go tests/vet and both frontend typecheck/build suites PASS.

**Step 3: Rebuild local Docker services and run real smoke**

Run the repository Docker development startup command documented in `docs/runbooks/backend-deployment.md`, then:

```bash
BASE_URL=http://localhost:8088 ./scripts/workflow/api-smoke.sh
```

Expected: readiness, storage propagation, artifact recovery, billing, and historical reads PASS.

**Step 4: Generate committed-scope review marker**

Run: `./scripts/workflow/review-local.sh --scope committed`

Run: `./scripts/workflow/check-review-gate.sh`

Expected: committed-scope `PASS` marker matching the current HEAD tree.

**Step 5: Commit any verification-only fixes and regenerate the gate**

```bash
git add <verification-fix-files>
git commit -m "fix: close artifact recovery verification gaps"
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```
