# Admin Runtime Monitoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the readiness-first admin Monitoring page with real five-second runtime telemetry supporting 5m, 15m, 30m, and 60m windows, and repair the Dashboard readiness table.

**Architecture:** Extend the existing in-process observability collector and HTTP middleware with bounded five-second ring buckets, route histograms, and Go runtime samples. Expose one authenticated snapshot endpoint and render it through typed view models, accessible SVG trends, stale-data-preserving polling, and existing Soft Grid Ops primitives.

**Tech Stack:** Go 1.26 standard library, net/http, runtime/metrics, React 19, TypeScript, Tailwind CSS, Lucide, repository contract scripts.

---

## Sources And Guardrails

- Approved design: `docs/plans/2026-07-12-admin-runtime-monitoring-design.md`
- Admin design system: `docs/plans/2026-07-11-admin-soft-grid-ops-unification-design.md`
- Follow `dev-go-patterns`, `dev-react-patterns`, TDD, and repository review gates.
- Never persist raw request paths, query strings, credentials, prompts, or user identifiers.
- Keep route cardinality bounded and preserve optional `http.ResponseWriter` interfaces.

## Task 1: Add Bounded Runtime Telemetry

**Files:**

- Create: `internal/app/observability/runtime_metrics.go`
- Create: `internal/app/observability/runtime_metrics_test.go`
- Modify: `internal/app/observability/metrics.go`

**Step 1: Write failing tests**

Add tests named:

```go
func TestRuntimeMetricsTracksInflightStatusAndLatency(t *testing.T)
func TestRuntimeMetricsReturnsRequestedWindow(t *testing.T)
func TestRuntimeMetricsRollsOverAfterSixtyMinutes(t *testing.T)
func TestRuntimeMetricsAggregatesNormalizedRoutes(t *testing.T)
func TestRuntimeMetricsCollectingState(t *testing.T)
func TestRuntimeMetricsClassifiesPressureWithReasons(t *testing.T)
```

Use an injected clock and sampler. Assert five-second buckets, a 720-bucket maximum, current and peak concurrency, 2xx/4xx/5xx totals, fixed-histogram P50/P95/P99, 5m/15m/30m/60m cropping, and deterministic hot-route ordering.

**Step 2: Verify red**

```bash
go test ./internal/app/observability -run RuntimeMetrics -count=1
```

Expected: compile failure because the runtime collector does not exist.

**Step 3: Implement minimal collector**

Create `Window5m`, `Window15m`, `Window30m`, and `Window60m`; `RuntimeSnapshot`, `RuntimeCurrent`, `RuntimePoint`, `RuntimeStatuses`, and `RuntimeRoute`; plus:

```go
func (m *RuntimeMetrics) BeginRequest(method, pattern string) func(status int, duration time.Duration)
func (m *RuntimeMetrics) Snapshot(window Window) (RuntimeSnapshot, error)
```

Use a mutex-protected ring keyed by five-second Unix buckets. Store histogram counts, not request durations. Sample CPU-class deltas through `runtime/metrics`; sample heap, runtime system memory, Goroutines, GC pause, and uptime through runtime APIs. Keep first-sample CPU unavailable.

**Step 4: Verify green**

```bash
go test ./internal/app/observability -count=1
```

**Step 5: Commit**

```bash
git add internal/app/observability
git commit -m "feat(observability): collect rolling runtime telemetry"
```

## Task 2: Instrument HTTP Completion

**Files:**

- Create: `internal/http/middleware/metrics_test.go`
- Modify: `internal/http/middleware/metrics.go`

**Step 1: Write failing middleware tests**

Cover implicit 200, explicit 4xx/5xx, duration, concurrency cleanup, post-routing `r.Pattern`, exclusion of `/metrics`, `/readyz`, and the snapshot endpoint, and preservation of supported `Flusher`, `Hijacker`, `Pusher`, and `ReaderFrom` interfaces.

**Step 2: Verify red**

```bash
go test ./internal/http/middleware -run Metrics -count=1
```

**Step 3: Implement**

Wrap the response writer, begin before the downstream handler, and finish exactly once in a defer. Read `r.Pattern` after `next.ServeHTTP`. Preserve panic behavior and all supported writer capabilities.

**Step 4: Verify green and commit**

```bash
go test ./internal/http/middleware ./internal/app/observability -count=1
git add internal/http/middleware internal/app/observability
git commit -m "feat(observability): measure completed HTTP requests"
```

## Task 3: Expose The Authenticated Snapshot API

**Files:**

- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `internal/http/router/gallery_api_test.go`
- Modify: `web/shared/api-types.ts`

**Step 1: Write failing API tests**

Assert read-only admins can request all four windows, `10m` returns a structured 400, missing auth returns 401, and the payload includes generation metadata, current values, series, statuses, routes, and Providers without raw paths or secrets.

**Step 2: Verify red**

```bash
go test ./internal/http/router -run MonitoringSnapshot -count=1
```

**Step 3: Implement endpoint**

Register `GET /api/ops/admin/v1/monitoring/snapshot`. Require `PermissionReadOnly`, validate the exact window set, obtain a collector snapshot, load existing Provider health, escalate overall state for an unavailable enabled Provider, and return stable JSON. Add `API_PATHS.ops.monitoringSnapshot`.

**Step 4: Verify green and commit**

```bash
go test ./internal/app/observability ./internal/http/middleware ./internal/http/router -count=1
go vet ./internal/app/observability ./internal/http/middleware ./internal/http/router
git add internal/http internal/app/observability web/shared/api-types.ts
git commit -m "feat(api): expose admin runtime monitoring snapshot"
```

## Task 4: Add Frontend Types And View Models

**Files:**

- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Create: `web/admin/src/pages/monitoringRows.ts`
- Create: `web/admin/src/pages/monitoringRows.contract.ts`

**Step 1: Write failing contract**

Fixture all states and assert stable windows, unavailable formatting, bytes/duration/percentage/QPS/uptime formatting, chronological real-only series, status totals, deterministic hot routes, and direct Provider/readiness diagnostic actions.

**Step 2: Verify red**

```bash
npx --prefix web/admin tsx src/pages/monitoringRows.contract.ts
```

**Step 3: Implement typed client and pure helpers**

Add exact shared snapshot types and `adminApi.getMonitoringSnapshot(window)`. Keep number formatting, state tones, chart mapping, route sorting, and diagnostics outside React. Do not use `any`.

**Step 4: Verify green and commit**

```bash
npx --prefix web/admin tsx src/pages/monitoringRows.contract.ts
npm --prefix web/admin run typecheck
git add web/shared web/admin/src/pages/monitoringRows.ts web/admin/src/pages/monitoringRows.contract.ts
git commit -m "feat(admin): model runtime monitoring data"
```

## Task 5: Build Accessible Telemetry Charts

**Files:**

- Create: `web/admin/src/ui/timeSeriesChart.tsx`
- Create: `web/admin/src/ui/timeSeriesChart.contract.ts`
- Modify: `web/admin/src/styles.css`

**Step 1: Write failing chart contract**

Require stable SVG geometry, zero-span scale protection, no fabricated path for fewer than two points, real timestamps in the accessible summary, ArrowLeft/ArrowRight/Home/End inspection, pointer inspection without layout shift, theme compatibility, and reduced motion.

**Step 2: Verify red**

```bash
npx --prefix web/admin tsx src/ui/timeSeriesChart.contract.ts
```

**Step 3: Implement**

Build a compact one-to-three-series SVG chart. Export pure scale/path helpers for contract testing. Use one focusable plot region and one stable tooltip layer. Provide latest/min/max text for assistive technology. Do not hand-draw interface icons.

**Step 4: Verify green and commit**

```bash
npx --prefix web/admin tsx src/ui/timeSeriesChart.contract.ts
npm --prefix web/admin run typecheck
git add web/admin/src/ui/timeSeriesChart.tsx web/admin/src/ui/timeSeriesChart.contract.ts web/admin/src/styles.css
git commit -m "feat(admin): add accessible telemetry charts"
```

## Task 6: Rebuild System Health

**Files:**

- Modify: `web/admin/src/healthRows.contract.ts`
- Create: `web/admin/src/pages/monitoringPage.contract.ts`
- Modify: `web/admin/src/pages/MonitoringPage.tsx`
- Modify: `web/admin/src/styles.css`

**Step 1: Write failing page contracts**

Require 5m/15m/30m/60m controls, five-second visible-tab polling, `visibilitychange` resume, auto-refresh, stale-data preservation, three trend regions, hot routes, status distribution, dependency diagnostics, collecting/no-traffic states, and removal of the primary full-readiness tables and ambiguous probe heading.

**Step 2: Verify red**

```bash
npx --prefix web/admin tsx src/healthRows.contract.ts
npx --prefix web/admin tsx src/pages/monitoringPage.contract.ts
```

**Step 3: Implement lifecycle**

Separate initial loading, background refreshing, and background error. Keep the last successful snapshot visible during refresh and failed window changes. Poll only when auto-refresh is enabled and the document is visible. Clean up timers and listeners.

**Step 4: Implement layout**

Build the compact state/header controls, eight stable metrics, request-load/response-quality/resource-pressure charts, hot-route DataTable, status distribution, and compact Provider/readiness diagnosis. Reuse `PageHeader`, `MetricStrip`, `DataTable`, `Badge`, `InlineFeedback`, and existing button classes.

**Step 5: Verify green and commit**

```bash
npx --prefix web/admin tsx src/healthRows.contract.ts
npx --prefix web/admin tsx src/pages/monitoringPage.contract.ts
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
git add web/admin/src
git commit -m "feat(admin): rebuild system health monitoring"
```

## Task 7: Repair Dashboard Risk Table

**Files:**

- Modify: `web/admin/src/pages/overviewPage.contract.ts`
- Modify: `web/admin/src/pages/OverviewPage.tsx`

**Step 1: Write failing regression contract**

Require `ReadinessRiskPanel` to use `DataTable` and a column factory. Forbid applying `readinessGrid` to both a collection and its nested rows. Retain check/key, status, detail, and repair action.

**Step 2: Verify red**

```bash
npx --prefix web/admin tsx src/pages/overviewPage.contract.ts
```

**Step 3: Implement minimal fix**

Replace the nested grid with `DataTable<OverviewReadinessRow>`, explicit column widths, and bounded horizontal table scrolling. Keep the inline empty state.

**Step 4: Verify green and commit**

```bash
npx --prefix web/admin tsx src/pages/overviewPage.contract.ts
npm --prefix web/admin run typecheck
git add web/admin/src/pages/OverviewPage.tsx web/admin/src/pages/overviewPage.contract.ts
git commit -m "fix(admin): align dashboard readiness risks"
```

## Task 8: Verify, Review, Rebuild, And Accept

**Files:**

- Create: `docs/audits/admin-runtime-monitoring-2026-07-12/acceptance.md`
- Create: `docs/audits/admin-runtime-monitoring-2026-07-12/screenshots/*.png`

**Step 1: Focused verification**

```bash
go test -race ./internal/app/observability ./internal/http/middleware
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

**Step 2: Repository verification**

```bash
./scripts/workflow/verify.sh
git diff --check
./scripts/workflow/review-local.sh --scope all
```

Fix every P1/P2 finding. Review ring races, cardinality, response-writer capabilities, timer leaks, stale-window ambiguity, chart accessibility, and overflow.

**Step 3: Docker and API smoke**

```bash
docker compose -f deployments/docker-compose/docker-compose.dev.yml up -d --build
./scripts/workflow/api-smoke.sh
docker compose -f deployments/docker-compose/docker-compose.dev.yml ps
```

**Step 4: Browser acceptance**

At 1440, 1024, 768, 390, and 320 pixels verify Dashboard alignment, all windows, polling controls, hidden-tab resume, real load changes, chart pointer/keyboard use, light/dark/reduced motion, no console errors, and zero page-level horizontal overflow. Save screenshots and observations.

**Step 5: Commit evidence and generate committed gate**

```bash
git add docs/audits/admin-runtime-monitoring-2026-07-12 web/admin internal web/shared
git commit -m "test(admin): verify runtime monitoring workspace"
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```
