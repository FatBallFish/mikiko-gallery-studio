# Admin Runtime Monitoring Design

**Status:** Approved on 2026-07-12

## 1. Purpose

Rebuild the admin `#/monitoring` route as a real runtime-health workspace instead of a readiness-first page. The page must answer whether the application is available, under pressure, or failing by using real request and process telemetry. It must preserve the existing Soft Grid Ops visual language and must not fabricate historical data.

This change also fixes the broken `Dashboard` readiness-risk table, whose outer grid currently treats each nested row as a single grid cell.

## 2. Product Decisions

- Use an authenticated built-in monitoring API rather than Prometheus, SSE, WebSocket, or frontend-generated data.
- Sample and aggregate in process every five seconds.
- Retain the most recent 60 minutes in memory. Process restarts intentionally reset the series.
- Support `5m`, `15m`, `30m`, and `60m` windows. Default to `15m`.
- Poll the snapshot endpoint every five seconds while the page is visible. Reduce work while the tab is hidden and refresh immediately when it becomes visible again.
- Keep runtime health as the primary Monitoring content. Move the full configuration readiness workflow back to Dashboard; Monitoring retains only a compact dependency and diagnostic region.

## 3. Data Architecture

### 3.1 Request telemetry

Extend the existing HTTP metrics middleware so it records request start and completion around the application handler. The collector tracks:

- current and peak in-flight requests;
- total requests and requests per second;
- 2xx, 4xx, and 5xx counts;
- latency histogram buckets sufficient to derive P50, P95, and P99;
- normalized route-level request count, QPS, P95, and error rate.

Use `http.Request.Pattern` after routing to aggregate stable route patterns. Never use raw paths containing user IDs, task IDs, or other unbounded values. Unknown routes fall back to a bounded `METHOD unknown` key.

Exclude `/metrics`, `/readyz`, and the monitoring snapshot endpoint from business traffic metrics so polling does not inflate its own charts.

### 3.2 Rolling history

The collector stores 720 five-second buckets in a concurrency-safe ring. Each bucket contains aggregate counters and fixed latency-histogram counts rather than individual request samples. Route summaries are bounded to known patterns and the response returns only the busiest routes.

The snapshot API crops the ring to the requested `5m`, `15m`, `30m`, or `60m` window. A process with less than one complete sampling interval reports a collecting state and returns only real completed buckets.

### 3.3 Process telemetry

Sample application-process pressure every five seconds:

- CPU use as a percentage of CPU capacity available to the API process;
- Go heap allocation;
- memory requested by the Go runtime from the operating system;
- Goroutine count;
- garbage-collection pause information;
- process uptime.

Labels must identify these as API-process or Go-runtime values. They must not imply that process-scoped values represent the entire Docker host.

### 3.4 Endpoint

Add `GET /api/ops/admin/v1/monitoring/snapshot?window=15m` behind the existing read-only admin permission. The response contains:

- generation time, window, sample interval, warm-up state, and uptime;
- overall state and explicit state reasons;
- current request and process metrics;
- ordered time-series points;
- status-code totals;
- bounded hot-route summaries;
- current Provider dependency health.

Invalid windows return a structured validation error. The endpoint does not expose raw credentials, request bodies, query strings, user identifiers, or prompts.

## 4. Status Semantics

The page uses explicit thresholds rather than an unexplained synthetic score:

- **Healthy:** 5xx rate below 1%, P95 below 1 second, and CPU below 75%.
- **Pressured:** 5xx rate from 1% through 5%, P95 from 1 through 2 seconds, or CPU from 75% through 90%.
- **Critical:** 5xx rate above 5%, P95 above 2 seconds, CPU above 90%, or an unavailable enabled Provider.
- **Collecting:** there is not yet one complete five-second sample.

The API returns every active reason so the UI can explain the state. Empty traffic remains healthy once sampling is ready; it does not invent requests or latency.

## 5. Monitoring Information Architecture

### 5.1 Header and controls

The header contains the system state, uptime, last successful refresh, a `5m / 15m / 30m / 60m` segmented window control, an auto-refresh toggle, and an icon-based manual refresh command. Refreshing keeps the prior snapshot visible. A failed refresh marks the snapshot stale and exposes a local retry without clearing the page.

### 5.2 First viewport

A compact metric strip prioritizes:

- API availability;
- current QPS;
- current and peak concurrency;
- P95 latency;
- 5xx error rate;
- API-process CPU;
- Go memory;
- Goroutine count.

Metrics use stable geometry, tabular numerals, semantic colors, and concise supporting context.

### 5.3 Trend workspace

- **Request load:** QPS and in-flight concurrency.
- **Response quality:** P50, P95, P99, and 5xx rate.
- **Resource pressure:** CPU, Go memory, and Goroutines.

Charts render only returned samples. They provide accessible text summaries and pointer/focus inspection, support both themes, and avoid animation when reduced motion is requested.

### 5.4 Diagnosis

- **Hot routes:** normalized route, request count, QPS, P95, error rate, and state.
- **Status distribution:** 2xx, 4xx, and 5xx totals for the selected window.
- **Dependencies and diagnostics:** real Provider state plus a compact summary of exceptional readiness checks and direct repair routes.

The former full readiness tables and ambiguous upstream-probe framing are removed from the primary workflow.

## 6. Dashboard Risk Table Fix

Replace the hand-nested grid in `ReadinessRiskPanel` with the shared `DataTable` contract. The table must align columns on desktop, use one row model per check, and scroll horizontally at narrow widths. It retains status badges, diagnostic details, and repair links.

## 7. Interaction and Responsive Behavior

- At desktop widths, trend panels use a dense asymmetric grid without nested cards.
- At tablet widths, charts collapse to a single readable column before labels or legends overlap.
- At 390 and 320 pixels, controls wrap predictably, segmented windows remain usable, metric cells retain stable height, and tables use bounded horizontal scrolling.
- Window changes refresh immediately and preserve the previous window until the new response succeeds.
- Auto-refresh pauses network polling while disabled. Hidden tabs do not continue five-second polling.
- Every command has visible focus, disabled, busy, and error states.

## 8. Error Handling

- Initial failure uses the standard retryable page error.
- Background refresh failure preserves the last successful snapshot and records its age.
- A partial or empty series uses a local collecting or no-traffic state while keeping current values visible.
- Unknown process metrics use an unavailable value with a reason; they never become zero silently.
- Invalid endpoint windows fail at the API boundary and are covered by tests.

## 9. Verification

Backend tests cover concurrent request accounting, ring rollover, window cropping, route normalization, status classes, latency percentiles, process sampling fallbacks, permissions, and validation.

Frontend contracts cover window selection, polling visibility behavior, stale-data preservation, time-series conversion, empty and collecting states, responsive chart contracts, and the Dashboard table regression.

Completion requires:

- targeted red/green contract and Go tests;
- `go test ./...` and `go vet ./...`;
- admin and user typecheck/build;
- repository verification and review gate;
- Docker rebuild and real API smoke;
- authenticated browser acceptance at 1440, 1024, 768, 390, and 320 pixels in light and dark themes, including reduced motion and zero horizontal page overflow.
