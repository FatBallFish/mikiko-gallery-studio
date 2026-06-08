# Worker, gpt-image-2, Model Test, and Cashier Defect Closure Design

Date: 2026-06-08

## Scope

This design closes the current defects reported from the running admin and user experience:

1. Worker image task throughput is too low and must be configurable from admin system settings.
2. `gpt-image-2` parameter compatibility must keep user-facing quality and ratio controls while translating them into provider-safe request parameters.
3. Model account management needs an image generation test action.
4. Cashier visible payment methods need a create/delete management entry.
5. Cashier provider instances need visual field configuration per provider type, not raw JSON as the main path.
6. Cashier visible payment method save must not fail with method-not-allowed in deployed admin flows.

## Existing State

- `internal/worker/runner.go` currently processes one claimed task per `ProcessOnce`. It starts a goroutine only to run the single leased task while the same worker loop waits for completion and heartbeat. A single worker process therefore has an effective task concurrency of `1`.
- Admin model accounts already expose account-level `concurrency_limit` in `web/admin/src/pages/ProviderModelsPage.tsx` and `internal/domain/modeladmin/types.go`. That is a provider account routing limit, not worker process throughput.
- `internal/provider/openai/client.go` sends `size` and `quality` directly to `/v1/images/generations` and `/v1/images/edits`.
- The reference project `/Users/fatballfish/Documents/Projects/VueProjects/gpt_image_playground` normalizes parameters differently for Codex CLI mode:
  - Codex mode forces `quality` back to `auto`.
  - It records actual returned `size` / `quality` when the upstream response includes them.
  - Its `src/lib/size.ts` calculates legal dimensions with constraints: 16px multiples, max edge 3840px, aspect ratio <= 3:1, total pixels between 655360 and 8294400.
- `web/admin/src/pages/CashierPage.tsx` has a visible methods table but no add/delete entry.
- Cashier provider instance configuration has partial structured fields for JeePay only; other providers still rely on JSON-heavy input.
- Backend route `PUT /api/ops/admin/v1/cashier/visible-methods` exists and is tested. The method-not-allowed issue should be verified at request path/proxy/frontend runtime rather than solved by changing the backend contract blindly.

## Target Contracts

### 1. Worker Runtime Concurrency

Add worker runtime config:

```go
type WorkerConfig struct {
    MaxConcurrentTasks int `yaml:"max_concurrent_tasks"`
}
```

Config sources:

- Static YAML/env default: `worker.max_concurrent_tasks`, env override `WORKER_MAX_CONCURRENT_TASKS`.
- Admin system setting: new config tab `runtime`, item `worker_max_concurrent_tasks`.
- Runtime behavior: worker reads the admin setting periodically and applies it without restart.

Default:

- Local/dev default: `4`.
- Minimum: `1`.
- Maximum guardrail: `64`.

Backend behavior:

- `internal/worker.Run` owns a task execution pool.
- Each slot independently claims one queued task, maintains heartbeat for that task, executes it, then loops.
- Compensation work remains serialized per poll cycle or uses a small single-slot guard to avoid duplicate refund compensation pressure.
- Existing lease ownership and stale worker conflict protections remain the authority for cross-process safety.

Admin UI:

- Add setting under system config page: `runtime.worker_max_concurrent_tasks`.
- Label: `Worker 并发任务数`.
- Hint: `单个 worker 进程同时处理的生图任务数；模型账号并发限制仍会单独生效。`

### 2. gpt-image-2 Parameter Compatibility

The user-facing controls remain unified:

- `quality`: `auto | 1K | 2K | 4K`
- `aspect_ratio`: `1:1 | 3:2 | 2:3 | 16:9 | 9:16 | 4:3 | 3:4 | 21:9 | custom ratio`

Add model/account extra flag:

```json
{
  "source_mode": "images",
  "gpt_image_2_codex_source": false
}
```

Supported source modes:

- `images`: default OpenAI-compatible Images API behavior.
- `codex_responses`: Codex-origin compatible behavior for `gpt-image-2`.

Compatibility rule:

- For `gpt-image-2` with `source_mode=codex_responses` or `gpt_image_2_codex_source=true`:
  - actual request `quality` is always `auto`.
  - actual request `size` is calculated from the frontend requested quality bucket plus aspect ratio.
  - the calculated `size` must be sent to the real image generation API.
- For non-Codex source:
  - keep current compatible behavior unless model-specific mapping requires the same size calculation.

Dimension calculation contract:

Input:

```go
qualityBucket: "1K" | "2K" | "4K" | "auto"
aspectRatio: "W:H"
```

When `qualityBucket=auto`, resolve it through the existing billing/model resolver to a concrete bucket before size calculation. If no concrete bucket is available, fall back to `1K`.

Output:

```text
<width>x<height>
```

Constraints:

- width and height are multiples of 16.
- max(width, height) <= 3840.
- max(width / height, height / width) <= 3.
- width * height is between `655360` and `8294400`.

Preset dimensions:

| Quality | 1:1 | 3:2 | 2:3 | 16:9 | 9:16 | 4:3 | 3:4 | 21:9 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1K | 1024x1024 | 1536x1024 | 1024x1536 | 1280x720 | 720x1280 | 1024x768 | 768x1024 | 1280x544 |
| 2K | 2048x2048 | 2160x1440 | 1440x2160 | 2560x1440 | 1440x2560 | 2048x1536 | 1536x2048 | 2560x1088 |
| 4K | 2880x2880 | 3456x2304 | 2304x3456 | 3840x2160 | 2160x3840 | 3200x2400 | 2400x3200 | 3840x1600 |

For custom ratios:

- Iterate legal 16px-multiple dimensions within the chosen quality pixel budget.
- Choose the largest total pixel count whose ratio error is <= 1%.
- If no candidate exists, return a capability mismatch error before charging or submitting.

Code placement:

- Add pure calculation in `internal/domain/modelhub` or `internal/service/imagetask` as a tested helper.
- Apply it before building `provider.ImageRequest`.
- Persist requested fields as user input, and add actual request params into task attempt detail for admin troubleshooting.

### 3. Model Account Test Image

Backend endpoint:

```http
POST /api/ops/admin/v1/model-accounts/{account_id}/test-image
Content-Type: application/json

{
  "model_code": "gpt-image-2",
  "prompt": "A small product photo of a ceramic coffee cup on a clean desk",
  "source_mode": "images|codex_responses"
}
```

Response:

```json
{
  "status": "succeeded",
  "image_url": "...",
  "width": 1024,
  "height": 1024,
  "provider_request_id": "...",
  "actual_params": {
    "model": "gpt-image-2",
    "size": "1024x1024",
    "quality": "auto"
  },
  "elapsed_ms": 1234
}
```

Execution rules:

- Admin permission: `manage_models`.
- Use the selected account directly, bypassing normal route selection.
- Use one output only.
- Use low-cost defaults: prompt above, quality `auto`, aspect ratio `1:1`, size `1024x1024` after compatibility calculation.
- Store the test output through the existing storage backend so the admin can preview it.
- Do not create user billing ledger entries.
- On failure, return upstream code, message, request ID, and sanitized request params.

Admin UI:

- In `web/admin/src/pages/ProviderModelsPage.tsx`, add `测试` action on model account rows.
- Modal fields:
  - prompt textarea with default prompt.
  - model select populated by that account's enabled account models.
  - optional source mode select when model is `gpt-image-2`.
- Show loading state, resulting image, actual params, and error detail.

### 4. Cashier Visible Methods CRUD UX

Keep backend contract:

```http
GET /api/ops/admin/v1/cashier/visible-methods
PUT /api/ops/admin/v1/cashier/visible-methods
```

Frontend changes:

- Add `新增支付方式`.
- Add delete action per draft row.
- Validate before save:
  - method code is required and unique.
  - method code supports `mock`, `alipay`, `wxpay`, and future custom codes.
  - label is required.
  - provider type must be compatible with method.
  - scheduler strategy defaults to `round_robin`.
- Preserve batch save to keep the current config-store shape.

Method-not-allowed investigation and fix:

- Instrument or reproduce the browser request.
- Confirm final URL, HTTP method, status, and response body.
- If the runtime request includes a trailing slash, add backend or client path normalization.
- If nginx rewrites API requests to admin SPA fallback, fix `deployments/nginx/default.conf` and `deployments/nginx/frontend.conf` so `/api/` and `/v1/` always proxy to API before SPA routes.
- Add a regression test or contract test for `PUT /api/ops/admin/v1/cashier/visible-methods`.

### 5. Cashier Provider Instance Visual Config

Add provider field schemas in `web/admin/src/pages/cashierProviderOptions.ts`.

Field kinds:

- `text`
- `password`
- `textarea`
- `select`
- `json`
- `url`

Each field declares:

```ts
{
  key: string
  label: string
  secret: boolean
  required: boolean
  placeholder?: string
  hint?: string
}
```

Provider fields:

- `mock`
  - config: `mock_success`, `mock_trade_no_prefix`
- `alipay_direct`
  - config: `gateway_url`, `notify_url`, `return_url`, `payment_mode`
  - secrets: `app_id`, `app_private_key`, `alipay_public_key`
- `wxpay_direct`
  - config: `gateway_url`, `notify_url`, `return_url`, `payment_mode`, `client_ip`, `openid`
  - secrets: `app_id`, `mch_id`, `api_v3_key`, `merchant_private_key`, `merchant_certificate_serial`, `wechat_pay_public_key`, `wechat_pay_public_key_id`
- `easypay_alipay` / `easypay_wxpay`
  - config: `gateway_url`, `notify_url`, `return_url`, `payment_mode`, `client_ip`
  - secrets: `pid`, `key`
- `jeepay_alipay` / `jeepay_wxpay`
  - config: `gateway_url`, `payment_mode`, `way_code`, `client_ip`, `channel_extra`
  - secrets: `mch_no`, `app_id`, `key`

Save behavior:

- Visual config builds `config`, `secrets`, and `clear_secrets`.
- Existing stored secrets are never displayed as plaintext.
- A secret field left empty means keep existing secret.
- A checked clear action sends `clear_secrets`.
- Advanced JSON remains available for additional non-secret config only.

### 6. Tests and Verification

Backend:

- Worker runner concurrency tests:
  - max concurrency `1` preserves serial behavior.
  - max concurrency `3` can process three blocked tasks concurrently.
  - runtime config clamps invalid values.
- gpt-image-2 size calculation tests:
  - all preset ratios match the table.
  - custom ratios obey constraints.
  - invalid ratios fail before provider call.
  - Codex source sends `quality=auto` and calculated `size`.
- Model account test endpoint tests:
  - direct account is used.
  - response includes actual params.
  - billing is not touched.
- Cashier visible method route regression:
  - `PUT` still accepted.
  - trailing slash or deployed path behavior is covered if that is the reproduced root cause.

Frontend:

- Provider model test modal contract.
- Cashier visible method add/delete draft contract.
- Provider instance visual field mapping contract.

Final verification:

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

