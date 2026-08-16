# Video Native Pricing Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the current generic video cost/strategy/rule stack with provider-native Seedance and MiniMax H3 sales rate cards, mixed-route fixed quoting, and a simplified admin workflow.

**Architecture:** Each real video model owns a versioned, schema-discriminated CNY sales rate card. A route quote filters eligible candidates, calculates every candidate through a provider-specific calculator, locks the maximum CNY quote, converts it through the global `billing_pricing.cny_per_point`, and stores a v2 pricing snapshot. Legacy task snapshots remain settleable while legacy configuration tables and APIs are retired; provider cost rules stay narrowly readable for pre-upgrade tasks that still need to create an attempt.

**Tech Stack:** Go 1.24, Ent/Atlas, PostgreSQL, `shopspring/decimal`, React 19, TypeScript, Vite, repository workflow scripts.

---

## Required References And Workflow

- Requirement and accepted design: `docs/plans/2026-08-17-video-native-pricing-refactor-design.md`
- Existing multimedia design: `docs/tech/2026-08-12-multimedia-creation-phase1-tech-design.md`
- Repository workflow: `AGENTS.md`
- Before coding invoke `@dev-start-coding` and select the accepted design plus the user-approved requirement conversation as sources.
- Before editing Go invoke `@dev-go-patterns`.
- Before editing React/TypeScript invoke `@dev-react-patterns`.
- Use `@superpowers:test-driven-development` for each behavior change.
- Before push/PR invoke `@dev-ship`.

Do not revive any of these removed concepts under a new label:

- video strategy payment fee
- point-product net-income floor
- target margin
- provider cost buffer
- platform fixed/audio/reference costs
- reserve markup
- per-combination final point rules

### Task 1: Establish Coding Context And Baseline

**Files:**
- Create through workflow: `.coding-context.json`
- Verify: `docs/plans/2026-08-17-video-native-pricing-refactor-design.md`

**Step 1: Run the required pre-coding workflow**

Invoke `@dev-start-coding`. Select the accepted design as the technical source and record the approved user requirements in `.coding-context.json`.

**Step 2: Install repository hooks if missing**

Run:

```bash
./scripts/workflow/install-hooks.sh
```

Expected: `core.hooksPath` is `.githooks`.

**Step 3: Record baseline verification**

Run:

```bash
./scripts/workflow/verify.sh
```

Expected: all existing Go tests/vet and both React builds pass. If the branch is already failing, record the exact unrelated failure before changing code.

**Step 4: Commit coding context if the workflow requires it**

```bash
git add .coding-context.json
git commit -m "chore: bind native video pricing coding context"
```

### Task 2: Add Typed Provider Rate Cards And Pure Calculators

**Files:**
- Create: `internal/domain/video/native_pricing.go`
- Create: `internal/domain/video/native_pricing_test.go`
- Modify: `internal/domain/video/types.go`
- Modify: `pkg/errs/codes.go` or the repository's existing stable error-code file

**Step 1: Write failing tests for Seedance schema validation**

Cover:

- `seedance_token_v1` accepts supported resolution rows.
- input-video price is required only when capability exposes video input.
- zero or negative active output rates fail.
- an unknown resolution or pricing schema fails.

Use typed configuration shaped like:

```go
type SeedanceTokenRateCard struct {
	Resolutions map[Resolution]SeedanceResolutionRate `json:"resolutions"`
}

type SeedanceResolutionRate struct {
	WithoutInputVideoMillionTokensCNY string `json:"without_input_video_million_tokens_cny"`
	WithInputVideoMillionTokensCNY    string `json:"with_input_video_million_tokens_cny,omitempty"`
}
```

**Step 2: Write failing Seedance quote tests**

Assert official examples:

```go
// 720p, 16:9, 5 seconds, 24fps => 108,000 estimated tokens.
// 108,000 / 1,000,000 * 46 = 4.968 CNY.
```

Also test input-video minimum-token lookup and rule-version rejection for unknown model combinations.

**Step 3: Write failing MiniMax H3 quote tests**

Use typed configuration:

```go
type MiniMaxH3SecondRateCard struct {
	Resolutions        map[Resolution]MiniMaxResolutionRate `json:"resolutions"`
	FreeImageCount     int                                  `json:"free_image_count"`
	ExtraImageCNY      string                               `json:"extra_image_cny"`
	InputAudioFree     bool                                 `json:"input_audio_free"`
}

type MiniMaxResolutionRate struct {
	OutputSecondCNY     string `json:"output_second_cny"`
	InputVideoSecondCNY string `json:"input_video_second_cny"`
}
```

Assert 5-second 768P output, 5-image boundary, 7-image surcharge, input-video seconds, and free input audio.

**Step 4: Run tests and verify failure**

```bash
go test ./internal/domain/video -run 'Test.*(Seedance|MiniMax|NativePricing)' -count=1
```

Expected: FAIL because native pricing types/calculators do not exist.

**Step 5: Implement the calculator registry**

Implement an explicit discriminator:

```go
type PricingSchema string

const (
	PricingSchemaSeedanceTokenV1   PricingSchema = "seedance_token_v1"
	PricingSchemaMiniMaxH3SecondV1 PricingSchema = "minimax_h3_second_v1"
)

type CandidateQuote struct {
	CNY         string         `json:"cny"`
	Calculation map[string]any `json:"calculation"`
}

type ProviderPricingCalculator interface {
	Validate(RateCard, Capability) error
	Quote(Request, RateCard) (CandidateQuote, error)
}
```

Use `decimal.Decimal` for every monetary or Token calculation. Never convert through `float64`.

**Step 6: Implement versioned Seedance technical presets**

Keep width/height/FPS/minimum-token lookup outside the admin rate card. Bind presets by recognized model family and rule version. Return `VIDEO_PRICING_SCHEMA_UNSUPPORTED` when a requested combination has no audited preset.

**Step 7: Run focused tests**

```bash
go test ./internal/domain/video -run 'Test.*(Seedance|MiniMax|NativePricing)' -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/domain/video pkg/errs
git commit -m "feat: add native video pricing calculators"
```

### Task 3: Add Versioned Rate-Card Persistence And Route Quote Fields

**Files:**
- Create: `internal/repository/ent/schema/videomodelratecard.go`
- Modify: `internal/repository/ent/schema/videorouteconfig.go`
- Modify: `internal/repository/ent/schema/multimedia_schema_test.go`
- Generate: `internal/repository/ent/**`
- Modify: `internal/repository/entstore/admin_video_config_store.go`
- Modify: `internal/repository/entstore/admin_video_store.go`
- Modify: `internal/repository/entstore/admin_video_store_test.go`

**Step 1: Write failing schema-contract tests**

Require rate-card fields:

```text
account_model_id
provider_code
pricing_schema
rate_version
currency
rate_config
source_reference
effective_at
enabled
deleted_at
```

Require route fields:

```text
candidate_parameter_mappings
minimum_task_points
rounding_step_points
```

Assert `pricing_strategy_id` is no longer part of the route schema.

**Step 2: Run schema tests and verify failure**

```bash
go test ./internal/repository/ent/schema -run TestMultimedia -count=1
```

Expected: FAIL for missing rate-card and route fields.

**Step 3: Implement Ent schemas**

Use `numeric(20,5)` strings for point fields. Store `rate_config` as JSON, but only repository/service boundaries may handle the raw map; domain logic receives typed structs.

**Step 4: Generate Ent code**

```bash
go generate ./internal/repository/ent
```

Expected: generated client, mutation, query, migration schema, and entity files include `VideoModelRateCard`.

**Step 5: Write failing repository versioning tests**

Test:

- create v1 with `expected_rate_version=0`
- create v2 with `expected_rate_version=1`
- stale writes fail
- only current enabled/effective version is loaded
- input maps are cloned and cannot be mutated after save

**Step 6: Implement repository methods**

Add store methods equivalent to:

```go
ListVideoModelRateCards(context.Context, int64) ([]RateCardSummary, error)
SaveVideoModelRateCard(context.Context, RateCardWrite) (RateCardSummary, error)
DeleteVideoModelRateCard(context.Context, int64, int) error
GetEffectiveVideoModelRateCard(context.Context, int64, time.Time) (RateCard, error)
```

Use a serializable transaction or existing optimistic-version pattern.

**Step 7: Run repository tests**

```bash
go test ./internal/repository/ent/schema ./internal/repository/entstore -run 'Test.*(VideoModelRateCard|AdminVideo|RouteConfig)' -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/repository/ent internal/repository/entstore
git commit -m "feat: persist native video rate cards"
```

### Task 4: Replace Admin Cost/Strategy APIs With Rate Cards And Route Simulation

**Files:**
- Modify: `internal/service/adminvideo/service.go`
- Modify: `internal/service/adminvideo/config.go`
- Modify: `internal/service/adminvideo/store.go`
- Modify: `internal/service/adminvideo/config_test.go`
- Modify: `internal/service/adminvideo/service_test.go`
- Modify: `internal/http/handlers/admin_video.go`
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/router/admin_video_config_contract_test.go`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`

**Step 1: Write failing service tests for rate-card validation**

Test provider/model ownership, schema/provider agreement, optimistic versioning, capability completeness, and enable blocking.

**Step 2: Write failing route-simulation tests**

The simulation request must identify a route and a canonical video request. Assert response rows include:

```json
{
  "route_candidate_id": 11,
  "account_model_id": 101,
  "eligible": true,
  "estimated_cny": "4.96800",
  "exclusion_code": "",
  "calculation": {}
}
```

Also assert highest candidate, `cny_per_point`, minimum points, rounding step, and final unit points.

**Step 3: Run service tests and verify failure**

```bash
go test ./internal/service/adminvideo -run 'Test.*(RateCard|Simulation|Route)' -count=1
```

**Step 4: Replace service DTOs**

Remove `CostRuleWrite`, `StrategyWrite`, and `PriceRuleWrite` from active APIs. Add `RateCardWrite`, `RateCardSummary`, `RouteQuoteSettingsWrite`, `QuoteSimulationRequest`, and `QuoteSimulationResult`.

Do not delete legacy task-settlement DTOs in this task.

**Step 5: Replace HTTP routes**

Implement:

```text
GET    /api/ops/admin/v1/video-models/{account_model_id}/rate-cards
POST   /api/ops/admin/v1/video-models/{account_model_id}/rate-cards
DELETE /api/ops/admin/v1/video-models/{account_model_id}/rate-cards/{id}
POST   /api/ops/admin/v1/video-routes/{route_model_id}/quote-simulation
```

Keep permissions and audit logging. Remove active registrations for video pricing strategies, recalculation, and video price rules.

**Step 6: Update shared TypeScript contracts**

Use discriminated unions for rate configs. Do not type them as unrestricted `Record<string, unknown>`.

**Step 7: Run API contract tests**

```bash
go test ./internal/service/adminvideo ./internal/http/router -run 'Test.*(AdminVideo|RateCard|QuoteSimulation)' -count=1
npm --prefix web/admin run typecheck
```

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/service/adminvideo internal/http web/shared
git commit -m "feat: expose native video pricing administration"
```

### Task 5: Implement Mixed-Candidate Fixed Quote Generation

**Files:**
- Modify: `internal/service/videorouting/service.go`
- Modify: `internal/service/videorouting/store.go`
- Modify: `internal/repository/entstore/video_config_store.go`
- Modify: `internal/service/videopricing/service.go`
- Modify: `internal/service/videopricing/store.go`
- Modify: `internal/service/videotask/quote.go`
- Modify: `internal/service/videotask/quote_test.go`
- Modify: `internal/domain/video/pricing.go`

**Step 1: Write failing mixed-route quote tests**

Build a route with Seedance and MiniMax candidates. Assert:

- unsupported candidates are excluded
- canonical `720p` maps to MiniMax `768p`
- candidate CNY results are retained
- maximum CNY wins
- `cny_per_point=0.01` converts correctly
- minimum task points applies after conversion
- step 1/5/10 uses ceiling, never nearest rounding
- output count multiplies the locked unit points

**Step 2: Run tests and verify failure**

```bash
go test ./internal/service/videopricing ./internal/service/videotask -run 'Test.*(Mixed|Quote|Rounding|Minimum)' -count=1
```

**Step 3: Replace strategy-based pricing store contract**

The pricing service should receive:

- route quote settings
- eligible candidates
- candidate capability and parameter mapping
- effective rate cards
- global `cny_per_point`

It must not query `VideoPricingStrategy` or `VideoPriceRule`.

**Step 4: Implement one canonical mapping pass**

Return a mapped provider request from routing and reuse it for:

1. capability matching
2. quote calculation
3. Provider request serialization

Do not independently translate resolution in the MiniMax client after this boundary.

**Step 5: Implement fixed quote rounding**

Use decimal arithmetic equivalent to:

```go
stepPoints := rawPoints.Div(step).Ceil().Mul(step)
unitPoints := decimal.Max(minimum, stepPoints)
```

Normalize to the configured point scale before snapshots and ledger operations.

**Step 6: Update quote token payload**

Include route config version, capability digest, all applicable rate versions, global conversion version/value, candidate quote breakdown, winner, unit points, and total reserved points.

**Step 7: Run focused tests**

```bash
go test ./internal/service/videorouting ./internal/service/videopricing ./internal/service/videotask -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/domain/video internal/service/videorouting internal/service/videopricing internal/service/videotask internal/repository/entstore/video_config_store.go
git commit -m "feat: lock mixed-route video quotes"
```

### Task 6: Add V2 Pricing Snapshots And Preserve Legacy Settlement

**Files:**
- Modify: `internal/service/videotask/service.go`
- Modify: `internal/service/videotask/store.go`
- Modify: `internal/repository/entstore/video_task_store.go`
- Modify: `internal/repository/entstore/video_worker_store.go`
- Modify: `internal/repository/entstore/video_worker_store_test.go`
- Modify: `internal/service/videotask/service_test.go`

**Step 1: Write failing v2 snapshot tests**

Assert a created task stores:

- `schema_version=2`
- `quote_mode=route_candidate_max_fixed`
- `cny_per_point`
- route minimum and rounding step
- all candidate quote details
- highest quote model
- fixed unit points

**Step 2: Write failing settlement compatibility tests**

Cover:

- old snapshot with `sales_rule` still settles
- v2 snapshot charges fixed unit points per successful item
- actual Provider seconds/Tokens do not change v2 points
- partial success releases failed item reservation
- all failed releases all points

**Step 3: Run tests and verify failure**

```bash
go test ./internal/service/videotask ./internal/repository/entstore -run 'Test.*(PricingSnapshotV2|LegacySettlement|FixedSettlement|Partial)' -count=1
```

**Step 4: Implement explicit snapshot-version branching**

Do not infer v2 from missing fields. Use `schema_version`:

```go
switch snapshotSchemaVersion(task.PricingSnapshot) {
case 0, 1:
	return legacyItemCharge(task, item, usage)
case 2:
	return fixedItemCharge(task, item)
default:
	return error
}
```

**Step 5: Remove reserve markup for new quotes**

For v2, `reserved_points == estimated_points`. Keep legacy reserve values unchanged.

**Step 6: Run focused tests**

```bash
go test ./internal/service/videotask ./internal/repository/entstore -run 'Test.*(Video|Settlement|PricingSnapshot)' -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/videotask internal/repository/entstore/video_task_store.go internal/repository/entstore/video_worker_store.go internal/repository/entstore/video_worker_store_test.go
git commit -m "feat: settle fixed video pricing snapshots"
```

### Task 7: Correct Usage Auditing And Add Trusted Input-Media Metadata

**Files:**
- Modify: `internal/domain/video/types.go`
- Modify: `internal/service/videotask/service.go`
- Modify: `internal/service/videotask/service_test.go`
- Modify: `internal/repository/ent/schema/videotaskinput.go` only if normalized metadata cannot remain in `asset_snapshot`
- Modify: `internal/provider/video/contracts.go`
- Modify: `internal/provider/video/seedance/client.go`
- Modify: `internal/provider/video/seedance/client_test.go`
- Modify: `internal/provider/video/minimax/client.go`
- Modify: `internal/provider/video/minimax/client_test.go`
- Modify: `internal/repository/entstore/video_worker_store.go`

**Step 1: Write failing Seedance usage test**

Given both fields, assert normalized Provider Tokens use `completion_tokens`, not `total_tokens`.

**Step 2: Write failing input metadata tests**

Assert media type and duration come from owned, ready asset records, not client input. Reject assets missing required probe metadata for billable video inputs.

**Step 3: Write failing Provider serialization tests**

Assert images, videos, and audio become their provider-specific payload types. Keep types unavailable in user capability until real-account tests prove the payload.

**Step 4: Run tests and verify failure**

```bash
go test ./internal/provider/video/... ./internal/service/videotask -run 'Test.*(Usage|Input|Media|Payload)' -count=1
```

**Step 5: Extend domain input metadata**

Add trusted duration in milliseconds or a decimal seconds representation. Use media asset `duration_ms`; never accept billing duration directly from the browser.

**Step 6: Correct usage and cost semantics**

Keep `usage_raw` and `usage_normalized`. Do not calculate new `provider_cost` from sales rate cards. Preserve historical values; use nullable/unknown semantics for new tasks without a procurement-cost source.

**Step 7: Run tests**

```bash
go test ./internal/provider/video/... ./internal/service/videotask ./internal/repository/entstore -run 'Test.*(Video|Usage|Input)' -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/domain/video internal/service/videotask internal/provider/video internal/repository/ent
git commit -m "fix: audit video usage and input metadata"
```

### Task 8: Implement Destructive Configuration Migration And Route Disablement

**Files:**
- Modify: `internal/repository/ent/schema/videoprovidercostrule.go` or remove after migration staging
- Modify: `internal/repository/ent/schema/videopricingstrategy.go` or remove after migration staging
- Modify: `internal/repository/ent/schema/videopricerule.go` or remove after migration staging
- Modify: `internal/repository/ent/migrate/schema.go` through generation
- Modify: repository startup migration/backfill module discovered by `@dev-start-coding`
- Create or modify migration tests in `internal/repository/entstore/`
- Modify: `docs/ops/multimedia-operations.md`

**Step 1: Write a migration test from the latest production schema**

Seed:

- legacy strategies/rules/cost combinations
- enabled video routes
- an in-flight legacy task with pricing snapshot
- completed ledger history

Assert migration:

- creates the new rate-card structure
- clears or retires legacy config while retaining provider cost rules for pre-upgrade in-flight task attempts
- disables existing video routes
- preserves account/model/candidate/capability rows
- preserves task and ledger rows byte-for-byte where applicable
- allows the in-flight legacy task to settle

**Step 2: Run migration test and verify failure**

Run the repository's isolated migration test command identified by `@dev-start-coding`; at minimum:

```bash
go test ./internal/repository/entstore -run 'Test.*Video.*Migration' -count=1
```

**Step 3: Implement idempotent migration behavior**

The migration may run repeatedly. Use explicit migration state or state-derived idempotency. Never infer Token/second rates from old `cost_cny` combinations.

**Step 4: Choose physical table removal safely**

If Atlas cannot safely drop the old tables in the same release, stop all runtime/API reads and soft-retire data now, then schedule physical drop later. The admin and quote paths must not retain legacy dependencies.

**Step 5: Add operator documentation**

Document that after upgrade admins must:

1. configure real-model sales rates
2. confirm route parameter mappings
3. set minimum/rounding values
4. run quote simulation
5. re-enable video routes

**Step 6: Run migration and legacy settlement tests**

```bash
go test ./internal/repository/entstore ./internal/service/videotask -run 'Test.*(Migration|LegacySettlement)' -count=1
```

Expected: PASS and repeated migration is a no-op.

**Step 7: Commit**

```bash
git add internal/repository docs/ops/multimedia-operations.md
git commit -m "feat: migrate video pricing configuration"
```

### Task 9: Replace Real-Model Cost UI With Provider-Native Sales Rates

**Files:**
- Modify: `web/admin/src/pages/VideoProviderAccountsPanel.tsx`
- Modify: `web/admin/src/pages/videoModelManagement.contract.ts`
- Modify: `web/admin/src/pages/videoAdmin.contract.ts`
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Delete after references are gone: `web/admin/src/pages/VideoConfigurationWorkspace.tsx`

**Step 1: Write failing frontend contract tests**

Assert:

- no `cost_cny`, combination JSON, reserve markup, validation status, source type, or manual effective date input
- Seedance renders resolution rows and two CNY/million-Token columns
- MiniMax renders output/input-video CNY/second rows, free image count, extra image CNY, and read-only free audio
- model list renders rate status
- unsupported provider schema cannot be enabled

**Step 2: Run tests and verify failure**

```bash
./scripts/workflow/verify-contracts.sh
```

Expected: FAIL because the existing video admin contracts still require combination costs, strategies, and price rules.

**Step 3: Implement discriminated rate-card editor state**

Use separate Seedance and MiniMax draft types. Do not branch on arbitrary field existence.

**Step 4: Make save boundaries explicit**

Save model/capability and rate card through separate API operations with independent errors, or add a transactional aggregate endpoint. Do not reproduce the current partial-save sequence that can leave an enabled model without a valid rate.

**Step 5: Remove obsolete workspace and contracts**

Delete `VideoConfigurationWorkspace.tsx` once no imports/contracts depend on it. Update contract tests to reject old strategy terms.

**Step 6: Run frontend verification**

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 7: Commit**

```bash
git add web/admin/src/pages web/shared
git commit -m "feat: simplify video model sales rates"
```

### Task 10: Add Candidate Mapping And Video Quote Overview UI

**Files:**
- Modify: `web/admin/src/pages/VideoRouteConfigPanel.tsx`
- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Rewrite: `web/admin/src/pages/VideoPricingPanel.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `web/admin/src/pages/videoModelManagement.contract.ts`
- Modify: `web/admin/src/pages/videoAdmin.contract.ts`

**Step 1: Write failing route UI contracts**

Assert candidate mapping supports route/native resolution pairs, automatic same-name mapping, and MiniMax 720P/768P suggestion.

**Step 2: Write failing quote-overview contracts**

Reject old fields and require:

- route
- candidate count
- rate completeness
- minimum task points
- rounding step
- current `cny_per_point` read-only value
- quote simulation controls
- candidate breakdown and highest-price source
- repair link to the real model

**Step 3: Run contract tests and verify failure**

```bash
npm --prefix web/admin run typecheck
```

Expected: contract/type failures until components are rewritten.

**Step 4: Implement route mapping editor**

Keep mapping controls compact and only display exceptions. Route enable should surface backend blocking reasons without duplicating business validation in React.

**Step 5: Rewrite video pricing panel as quote overview**

Remove strategy/rule CRUD. Persist only minimum points and rounding step through route quote settings. Reuse admin table, modal, refresh button, badges, and error patterns.

**Step 6: Add a complete simulator**

Support task type, canonical resolution, ratio, output duration, image count, input video seconds, and audio presence. Render excluded candidates with reason codes rather than silently hiding them.

**Step 7: Verify responsive UI**

Run the admin app and inspect desktop and tablet landscape. Ensure tables remain scannable and no field labels overflow.

**Step 8: Run frontend checks**

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

Expected: PASS.

**Step 9: Commit**

```bash
git add web/admin/src/pages
git commit -m "feat: add video route quote overview"
```

### Task 11: Complete API, Quote, And Migration Integration Tests

**Files:**
- Modify: `internal/http/router/admin_video_config_contract_test.go`
- Modify: `internal/http/router/video_tasks_api_test.go`
- Modify: `internal/http/router/video_capability_api_test.go`
- Modify: `scripts/test/api_contract_smoke.sh` or the video-specific smoke fixture discovered during implementation
- Modify: `docs/ops/multimedia-operations.md`

**Step 1: Add admin API integration scenario**

Create accounts/models, save capabilities, save Seedance and MiniMax rate cards, bind both to a route, save mapping/minimum/rounding, run simulation, and enable the route.

**Step 2: Add user quote and create-task scenario**

Assert the returned quote uses maximum candidate CNY and global `cny_per_point`, then create a task using the signed token.

**Step 3: Add stale-quote scenarios**

Change each independently and assert old quote rejection:

- route config version
- capability version
- applicable rate-card version
- global `cny_per_point`

**Step 4: Add settlement scenario**

Complete one result, fail one result, send Provider usage different from estimate, and assert fixed per-success result points.

**Step 5: Add migration smoke scenario**

Assert old configurations do not appear in admin APIs and upgraded video routes require reconfiguration.

**Step 6: Run focused integration tests**

```bash
go test ./internal/http/router -run 'Test.*(AdminVideo|VideoTask|VideoQuote|RateCard)' -count=1
```

Expected: PASS.

**Step 7: Run isolated API smoke**

Invoke `@dev-api-smoke`, then run:

```bash
./scripts/workflow/api-smoke.sh
```

Expected: isolated PostgreSQL/Redis/API/Worker/fake-provider scenario passes and cleans itself up.

**Step 8: Commit**

```bash
git add internal/http/router scripts/test docs/ops/multimedia-operations.md
git commit -m "test: cover native video pricing workflow"
```

### Task 12: Remove Dead Legacy Runtime Code And Verify Delivery

**Files:**
- Remove or simplify: `internal/domain/video/pricing.go`
- Remove or simplify: `internal/provider/video/contracts.go` legacy pricing contract section
- Remove: `docs/tech/contracts/video-provider-pricing-v1.json` if no historical tooling consumes it
- Remove obsolete generated entities only after schema migration decision
- Modify impacted tests and docs

**Step 1: Search for remaining legacy concepts**

Run:

```bash
rg -n "VideoPricingStrategy|VideoPriceRule|video-pricing-strategies|provider_cost_buffer_rate|payment_fee_rate|reserve_markup|pricing_bindings|combinations.*cost_cny" internal web docs scripts
```

Expected: only intentional historical design references, legacy snapshot compatibility, and migration tests remain.

**Step 2: Remove dead active code**

Delete obsolete service/store/handler/types and generated references. Keep a narrowly scoped legacy snapshot calculator for existing tasks; label it clearly and prevent new tasks from producing that schema.

**Step 3: Run formatting**

```bash
gofmt -w <all-modified-go-files>
```

Use the repository formatter for frontend files if configured.

**Step 4: Run full verification**

Invoke `@dev-verify`, then run:

```bash
./scripts/workflow/verify.sh
```

Expected: Go tests/vet, user typecheck/build, and admin typecheck/build all pass.

**Step 5: Run committed-scope review gate**

Commit any final cleanup first:

```bash
git add -A
git commit -m "refactor: retire legacy video pricing stack"
```

Then invoke `@dev-review-gate` and run:

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: `.review/gate.json` is `PASS`, scope is `committed`, and tree SHA matches `HEAD`.

**Step 6: Run final ship workflow**

Invoke `@dev-ship`. It must rerun verification, committed review, stale-gate validation, and API smoke because this change touches backend, API, config, database, deployment behavior, and worker settlement.

**Step 7: Manual acceptance checklist**

Verify in the running admin application:

1. Seedance rate editor only shows supported resolution rows.
2. MiniMax rate editor applies five-image free allowance.
3. Mixed route maps 720P to MiniMax 768P.
4. Quote overview shows every candidate and highest-price source.
5. Minimum points and rounding step alter final points correctly.
6. A formal user quote matches admin simulation.
7. Changing a rate invalidates an unsubmitted quote.
8. Completed tasks retain locked price despite different Provider usage.
9. Upgraded legacy routes are disabled with actionable repair guidance.

**Step 8: Prepare PR**

Push only after `dev-ship` passes. The PR description must call out the destructive configuration reset and mandatory post-upgrade video-rate reconfiguration.
