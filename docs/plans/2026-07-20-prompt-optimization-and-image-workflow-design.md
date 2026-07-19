# Prompt Optimization and Image Workflow Design

## Status

Approved on 2026-07-20.

## Scope

This change delivers six related improvements:

1. Expand the workspace prompt into a large responsive editor that remains synchronized with the compact editor and shows referenced image metadata.
2. Add secure, multi-account text-model administration for OpenAI-compatible Chat Completions and Responses APIs.
3. Add prompt optimization with an always-visible preflight estimate, explicit result comparison, apply, and one-step undo.
4. Remove reference generation completely, including routes, configuration, historical records, API values, UI, and compatibility logic.
5. Use one image-detail experience across assets, public gallery, and workspace history, with full prompt copying for authorized viewers.
6. Replace JSON clipboard configuration copying with a structured creation draft that opens the workspace and restores supported generation inputs.

The project is not deployed and has no production data. Reference-generation removal is therefore intentionally destructive and does not preserve legacy snapshots or compatibility reads.

## Architecture

### Text-model administration

Text providers are a dedicated domain rather than generic configuration JSON.

- `text_model_accounts` stores account name, platform type, API style, base URL, encrypted credential material, credential fingerprint, enabled state, optimistic-lock version, and audit timestamps.
- `text_models` stores the account relation, upstream model code, display name, enabled state, currency, and reserved input/output token prices.
- Exactly one enabled text model may be selected as the default prompt optimizer model.
- The initial platform type is `openai_compatible`.
- The initial API styles are `chat_completions` and `responses`.
- Secret values are write-only. Reads return only configured state, fingerprint, and timestamps.
- Connection tests use a minimal request through the selected API style and return a sanitized result.

Text-model administration requires `manage:dangerous_config`. Base URLs are validated and outbound redirects are restricted to reduce SSRF exposure. Existing repository encryption facilities are reused for credentials.

### Prompt optimization

Prompt optimization is a separate service with estimate and execution endpoints.

1. The client requests an estimate for the current prompt.
2. The API returns the selected default model, configuration version, expiry, and zero estimated points while charging is disabled.
3. The client always presents a confirmation dialog, including for zero points.
4. Execution revalidates the prompt, default model, account status, and estimate version.
5. The configured Chat Completions or Responses adapter requests one optimized prompt.
6. The client shows original and optimized prompts side by side.
7. Only an explicit Apply action changes the workspace prompt; Undo restores the pre-optimization prompt once.

Referenced images are displayed in the expanded editor but are not sent to the text model in this release. Optimization calls persist user, account/model snapshot, API style, state, token usage, estimated/actual points, and sanitized errors. Billing fields are present but both estimate and charge remain zero until a future billing feature enables them.

### Prompt editor

The compact prompt editor gains Expand and Optimize icon buttons. Expand opens a large responsive dialog containing:

- a large prompt textarea bound to the same React state as the compact textarea;
- an Optimize icon using the same estimate/execution state machine;
- referenced image thumbnails and non-sensitive metadata;
- accessible focus trapping, Escape/backdrop dismissal, and a near-full-screen mobile layout.

Edits in either editor are immediately visible in the other.

### Unified image detail

Assets, public gallery, and workspace history use one image-detail component and shared image media/zoom behavior. The component renders full available prompt text without ellipsis, supports prompt copying, configuration reuse, download, and optional page-specific engagement or asset actions.

Public access boundaries do not change:

- authenticated viewers load full public image details and may copy prompt/configuration;
- unauthenticated viewers see excerpts and are sent through login with a return location when requesting protected actions.

### Creation drafts

"Copy configuration" becomes "Reuse configuration" and no longer writes JSON to the clipboard. A typed creation draft carries:

- prompt;
- route model;
- size mode;
- aspect ratio or pixel size;
- base resolution;
- quality;
- output format and compression;
- moderation;
- output count;
- accessible source reference asset IDs for image-edit tasks.

Navigation uses router state with a one-time `sessionStorage` fallback. Long prompts and asset identifiers are never placed in the URL. The workspace validates the draft against current capabilities, restores compatible values, applies deterministic fallbacks, and reports fields or source images that could not be restored.

### Reference-generation removal

The removal deletes all `reference_generate` and `reference_to_image` behavior:

- destructive database cleanup removes matching tasks, results, reference relations, billing reservations/ledger records, attempts, and stored snapshots before related configuration rows;
- model capabilities, routes, route prices, defaults, labels, filters, mocks, contracts, and OpenAPI enums are deleted;
- the user reference-generation tab and its separate state are deleted;
- frontend/backend compatibility conversions are deleted;
- old task-type input is rejected as invalid;
- all image-input generation uses `image_edit` and the provider edit path where required.

No legacy task display or data migration to `image_edit` remains.

## API Shape

Admin APIs provide account and nested model CRUD, secret replacement/clearing, connection testing, and default optimizer selection under `/api/ops/admin/v1/text-model-accounts` and `/api/ops/admin/v1/prompt-optimizer`.

User APIs provide:

- `POST /api/agent/text/v1/prompt-optimizations/estimate`
- `POST /api/agent/text/v1/prompt-optimizations`

Estimate responses include model display information, configuration version, expiry, and estimated points. Execution rejects stale estimates with a conflict response so the client can refresh and reconfirm.

## Errors and Security

- Missing or disabled default models return a stable capability-unavailable error.
- Provider authentication, timeout, rate limit, malformed response, and transport failures map to sanitized platform errors.
- Failed optimization never changes the current prompt and never charges points.
- Empty or over-limit optimization results are rejected.
- Configuration drafts degrade explicitly when models, parameters, or reference assets are unavailable.
- Secrets, full upstream payloads, and upstream credential-bearing errors are excluded from API responses, audit payloads, and logs.

## Verification

- Go unit and integration tests cover adapters, encrypted secret handling, default-model constraints, estimate versioning, optimization records, authorization, validation, error mapping, and destructive cleanup.
- TypeScript contract tests cover synchronized editing, both Optimize entry points, estimate confirmation, comparison/apply/undo, unified image details, login return behavior, and creation-draft restoration.
- OpenAPI contract tests verify the new endpoints and absence of legacy reference-generation task types.
- Repository verification, committed-scope review, review gate, API smoke, and real API/E2E checks must pass.
- Browser verification covers desktop and mobile prompt dialogs, admin text-model configuration, unified image details, and reuse-navigation behavior.
- Docker services are rebuilt from the final `main` tree and smoke checked after merge.

