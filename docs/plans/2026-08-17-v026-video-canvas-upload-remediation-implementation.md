# v0.0.26 Video, Canvas, And Upload Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove duplicate video capability configuration, enforce route visibility consistently, provide resilient S3/R2 uploads, and complete the canvas image workflow.

**Architecture:** Keep real model capabilities and route visibility relations as the authoritative backend sources. Preserve direct multipart uploads but add a streaming API fallback. Extend canvas schema v1 behavior through validated empty image output slots, pure resize commands, automatic image estimates, and idempotent server-side result placement.

**Tech Stack:** Go, Ent/PostgreSQL, React 19, TypeScript, Zustand, Vite, S3 SigV4, repository contract tests.

---

### Task 1: Remove Video Visible Combinations And Restore Route Group Echo

**Files:**
- Modify: `internal/domain/adminvideo/types.go`
- Modify: `internal/service/adminvideo/config.go`
- Modify: `internal/repository/entstore/admin_video_config_store.go`
- Modify: `internal/repository/entstore/model_admin_store.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/admin/src/pages/VideoRoutingConfigPage.tsx`
- Test: `internal/repository/entstore/admin_video_store_test.go`
- Test: `internal/repository/entstore/model_admin_store_test.go`
- Test: `web/admin/src/pages/videoPricing.contract.ts`
- Test: `web/admin/src/pages/routingPage.contract.ts`

**Step 1: Write failing backend tests**

Add tests proving route list/detail return saved group IDs for image and video routes, and video route configuration no longer requires or persists visible combinations.

**Step 2: Run focused tests and confirm failure**

Run: `go test ./internal/repository/entstore -run 'Test.*(RouteModel.*Group|Video.*Visible)' -count=1`

Expected: FAIL because list/detail currently call `mapRouteModel(entity, nil)` and video config still exposes combinations.

**Step 3: Implement group batching and remove the duplicate field**

Batch visibility rows for route IDs, map sorted unique group IDs, remove the admin video combination field from domain/API/frontend contracts, and ignore historical JSON values.

**Step 4: Run focused backend and frontend contracts**

Run: `go test ./internal/repository/entstore ./internal/service/adminvideo -count=1`

Run: `npm --prefix web/admin run test:contracts`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/adminvideo internal/service/adminvideo internal/repository/entstore web/shared web/admin/src/pages
git commit -m "fix: derive video options and restore route groups"
```

### Task 2: Enforce Video Route Visibility For User Groups

**Files:**
- Modify: `internal/service/videorouting/store.go`
- Modify: `internal/service/videorouting/service.go`
- Modify: `internal/repository/entstore/video_config_store.go`
- Modify: `internal/http/handlers/video.go`
- Modify: `internal/service/videotask/service.go`
- Modify: `internal/service/canvas/generator.go`
- Test: `internal/service/videorouting/service_test.go`
- Test: `internal/http/router/video_capability_api_test.go`
- Test: `internal/http/router/video_tasks_api_test.go`
- Test: `internal/service/canvas/generator_test.go`

**Step 1: Write failing visibility tests**

Cover public, matching group, non-matching group, disabled group and hidden route behavior for capability list, single capability lookup, estimate, create and canvas submission.

**Step 2: Run focused tests and confirm failure**

Run: `go test ./internal/service/videorouting ./internal/http/router ./internal/service/canvas -run 'Test.*Video.*(Visibility|Group)' -count=1`

Expected: FAIL because the current store only queries `visibility=public` and has no user group input.

**Step 3: Add visibility context**

Change video routing APIs to accept user group codes, resolve enabled groups and visibility relations in the Ent store, and pass authenticated group codes through capability, quote, create and canvas call paths.

**Step 4: Preserve capability mismatch behavior**

Keep candidate capability union for display and full-request candidate matching for estimate/create. Verify direct route codes cannot bypass visibility.

**Step 5: Run tests and commit**

Run: `go test ./internal/service/videorouting ./internal/service/videotask ./internal/service/canvas ./internal/http/router -count=1`

```bash
git add internal/service/videorouting internal/repository/entstore/video_config_store.go internal/http/handlers/video.go internal/service/videotask internal/service/canvas
git commit -m "fix: enforce video route group visibility"
```

### Task 3: Add Streaming S3 Multipart Proxy Fallback

**Files:**
- Modify: `internal/storage/multipart.go`
- Modify: `internal/service/mediaasset/service.go`
- Modify: `internal/http/handlers/media_uploads.go`
- Modify: `web/user/src/features/media/uploadManager.ts`
- Modify: `web/user/src/features/media/UploadTray.tsx`
- Modify: `web/shared/user-api.ts`
- Modify: `web/admin/src/pages/StorageConfigPage.tsx`
- Test: `internal/storage/multipart_test.go`
- Test: `internal/service/mediaasset/service_test.go`
- Test: `internal/http/router/media_upload_api_test.go`
- Test: `web/user/src/features/media/uploadManager.contract.ts`
- Test: `web/admin/src/pages/storageConfig.contract.ts`

**Step 1: Write failing storage tests**

Use an HTTP test server to assert `S3Backend.PutMultipartPart` streams the body, signs the checksum payload, forwards cancellation, validates size and returns an unquoted ETag.

**Step 2: Run focused tests and confirm failure**

Run: `go test ./internal/storage ./internal/service/mediaasset ./internal/http/router -run 'Test.*Multipart.*(Proxy|Part)' -count=1`

Expected: FAIL with `ErrDirectUploadRequired` for S3.

**Step 3: Implement streaming upload**

Create a signed request that uses the declared SHA-256 and incoming `io.Reader`, set exact content length, execute it through the configured client, and parse the response ETag without buffering the full part.

**Step 4: Generalize the same-origin part handler**

Allow the existing authenticated part endpoint to call `PutMultipartPart` for S3/R2 and local backends. Preserve session ownership, part limits and completed part reconciliation.

**Step 5: Add client transport state and failure localization**

Persist `transport` in upload snapshots. On direct-fetch network failure, atomically switch the session to proxy and retry that part through the API. Map errors to Chinese messages containing the part number and retryability.

**Step 6: Add non-blocking CORS guidance**

Show provider-specific required origin/method/header/exposed-header guidance in storage configuration without attempting bucket mutation.

**Step 7: Run tests and commit**

Run: `go test ./internal/storage ./internal/service/mediaasset ./internal/http/router -count=1`

Run: `npm --prefix web/user run test:contracts && npm --prefix web/admin run test:contracts`

```bash
git add internal/storage internal/service/mediaasset internal/http/handlers/media_uploads.go web/user/src/features/media web/shared/user-api.ts web/admin/src/pages/StorageConfigPage.tsx
git commit -m "fix: fall back to streaming multipart uploads"
```

### Task 4: Add Canvas Resize Commands And Correct Pointer Boundaries

**Files:**
- Modify: `internal/domain/canvas/graph.go`
- Modify: `web/user/src/features/canvas/core/canvasState.ts`
- Modify: `web/user/src/features/canvas/store/canvasStore.ts`
- Modify: `web/user/src/features/canvas/CanvasEditorPage.tsx`
- Modify: `web/user/src/features/canvas/canvas.css`
- Test: `internal/domain/canvas/graph_test.go`
- Test: `web/user/src/features/canvas/core/canvasState.contract.ts`
- Test: `web/user/src/features/canvas/CanvasEditor.contract.ts`

**Step 1: Write failing pure-state tests**

Assert resize clamps to node-type minimums, creates one undo entry, supports undo/redo and rejects non-finite backend dimensions.

**Step 2: Run focused tests and confirm failure**

Run: `go test ./internal/domain/canvas -count=1`

Run: `npm --prefix web/user run test:contracts -- CanvasEditor canvasState`

Expected: FAIL because no resize command or handle exists.

**Step 3: Implement resize state and view**

Add a transient resize draft divided by viewport zoom, commit once on pointer up, render a selected-node handle, and persist size through the existing document save path.

**Step 4: Restrict drag initiation**

Start dragging only from the node header or explicit drag surface. Mark forms, actions, ports, media controls and resize handles interactive. Do not pointer-capture internal controls.

**Step 5: Run tests and commit**

Run: `go test ./internal/domain/canvas -count=1 && npm --prefix web/user run test:contracts`

```bash
git add internal/domain/canvas web/user/src/features/canvas
git commit -m "feat: resize canvas nodes and preserve controls"
```

### Task 5: Support Empty Image Output Slots And Idempotent Result Placement

**Files:**
- Modify: `internal/domain/canvas/graph.go`
- Modify: `internal/service/canvas/store.go`
- Modify: `internal/service/canvas/service.go`
- Modify: `internal/repository/entstore/canvas_store.go`
- Modify: `web/user/src/features/canvas/core/canvasState.ts`
- Test: `internal/domain/canvas/graph_test.go`
- Test: `internal/service/canvas/service_test.go`
- Test: `internal/repository/entstore/canvas_store_test.go`
- Test: `web/user/src/features/canvas/core/canvasState.contract.ts`

**Step 1: Write failing graph tests**

Assert an empty image node is valid only with a legal incoming image-generation result edge, and remains excluded from asset reference extraction.

**Step 2: Write failing placement tests**

Cover first-run empty-slot filling, overflow node creation, second-run full append, stable IDs, repeated attach idempotency and revision conflict recovery.

**Step 3: Run focused tests and confirm failure**

Run: `go test ./internal/domain/canvas ./internal/service/canvas ./internal/repository/entstore -run 'Test.*(EmptyImage|ResultPlacement|Attach)' -count=1`

**Step 4: Implement validation and transactional attachment**

Validate empty media nodes after edge indexing. Extend attachment records with existing-node updates, apply updates/new nodes/new edges in one CAS revision, and determine first successful run from attached run history.

**Step 5: Implement batch layout**

Use generation-node right side for result batches, keep stable result IDs and connect every generated node with an ordinal result edge.

**Step 6: Run tests and commit**

Run: `go test ./internal/domain/canvas ./internal/service/canvas ./internal/repository/entstore -count=1`

```bash
git add internal/domain/canvas internal/service/canvas internal/repository/entstore/canvas_store.go web/user/src/features/canvas/core
git commit -m "feat: add canvas image output slots"
```

### Task 6: Complete Canvas Image Frames, Upload Targets, And Prompt Resources

**Files:**
- Modify: `web/user/src/features/canvas/CanvasEditorPage.tsx`
- Modify: `web/user/src/features/canvas/CanvasAssetDrawer.tsx`
- Modify: `web/user/src/features/canvas/core/canvasState.ts`
- Modify: `web/user/src/features/canvas/canvas.css`
- Modify: `web/user/src/features/media/uploadManager.ts`
- Modify: `web/user/src/features/media/UploadTray.tsx`
- Test: `web/user/src/features/canvas/CanvasEditor.contract.ts`
- Test: `web/user/src/features/canvas/core/canvasState.contract.ts`
- Test: `web/user/src/features/media/uploadManager.contract.ts`

**Step 1: Write failing frontend contracts**

Cover empty image creation, image-only asset selection, targeted upload completion, related reference candidate discovery, cursor insertion and duplicate-name errors.

**Step 2: Run contracts and confirm failure**

Run: `npm --prefix web/user run test:contracts`

Expected: FAIL because empty images cannot be created or targeted by upload events.

**Step 3: Implement image frame UI**

Add toolbar/menu creation, empty state actions, image-only asset drawer mode and node filling. Keep signed access URLs transient.

**Step 4: Correlate upload completion**

Carry optional canvas/node target metadata in upload snapshots and completion events. Fill only a still-existing node in the same open canvas.

**Step 5: Implement related resource picker**

Traverse prompt-to-generation and image-to-generation edges, show image thumbnail/name candidates, and insert the chosen resource token at the textarea selection.

**Step 6: Run contracts and commit**

Run: `npm --prefix web/user run test:contracts && npm --prefix web/user run typecheck`

```bash
git add web/user/src/features/canvas web/user/src/features/media
git commit -m "feat: complete canvas image frame inputs"
```

### Task 7: Add Canvas Image Auto-Estimate And Platform Output Counts

**Files:**
- Modify: `web/user/src/features/canvas/CanvasEditorPage.tsx`
- Modify: `web/user/src/features/canvas/canvas.css`
- Modify: `web/user/src/pages/workspaceViewModel.ts`
- Test: `web/user/src/features/canvas/CanvasEditor.contract.ts`
- Test: `web/user/src/pages/workspaceViewModel.contract.ts`
- Test: `internal/service/canvas/generator_test.go`

**Step 1: Write failing estimate state tests**

Cover debounce eligibility, missing-input suppression, stale signature invalidation, out-of-order response rejection and direct confirm-generation readiness.

**Step 2: Write failing output-count test**

Assert canvas image drafts accept the same normalized platform count as the creation page and forward it unchanged to the existing image task service.

**Step 3: Run focused tests and confirm failure**

Run: `npm --prefix web/user run test:contracts && go test ./internal/service/canvas -run Test.*Image.*Count -count=1`

**Step 4: Implement auto-estimate**

Debounce only complete image nodes, flush the document before estimate, ignore stale responses, show loading/error/points inline and expose confirm generation only for the current signature.

**Step 5: Replace native-count dropdown**

Use the shared platform safety limit and normalized numeric stepper. Do not alter video output count behavior or image service fan-out.

**Step 6: Run tests and commit**

Run: `npm --prefix web/user run test:contracts && npm --prefix web/user run typecheck && go test ./internal/service/canvas -count=1`

```bash
git add web/user/src/features/canvas web/user/src/pages/workspaceViewModel.ts internal/service/canvas
git commit -m "feat: auto-estimate canvas image generations"
```

### Task 8: End-To-End Verification And Review

**Files:**
- Modify if required: `docs/ops/multimedia-operations.md`
- Create: `.review/gate.json` through the workflow script

**Step 1: Run focused integration suites**

Run: `go test ./internal/repository/entstore ./internal/service/videorouting ./internal/service/videotask ./internal/storage ./internal/service/mediaasset ./internal/domain/canvas ./internal/service/canvas ./internal/http/router -count=1`

Run: `npm --prefix web/user run test:contracts && npm --prefix web/admin run test:contracts`

**Step 2: Run repository verification**

Run: `./scripts/workflow/verify.sh`

Expected: all Go tests/vet, frontend contracts, typechecks and builds PASS.

**Step 3: Run API smoke**

Run: `./scripts/workflow/api-smoke.sh`

Expected: isolated API, worker, PostgreSQL, Redis and fake provider smoke PASS.

**Step 4: Perform browser acceptance**

Verify desktop and tablet landscape node resize, prompt actions, automatic estimate, image upload fallback, reference selection, first-run slots, multi-result overflow and second-run append. Capture screenshots and console/network evidence.

**Step 5: Commit final test/docs adjustments**

```bash
git add docs web internal
git commit -m "test: verify v026 video canvas upload remediation"
```

**Step 6: Run committed review gate**

Run: `./scripts/workflow/review-local.sh --scope committed`

Run: `./scripts/workflow/check-review-gate.sh`

Expected: committed-scope review marker is PASS and matches HEAD tree.
