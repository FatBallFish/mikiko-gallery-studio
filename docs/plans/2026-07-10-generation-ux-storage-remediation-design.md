# Generation UX And Storage Remediation Design

## Status

- Date: 2026-07-10
- Scope: user creative workspace, user asset gallery, admin real-model capabilities, admin object-storage activation, shared API contracts, image-task execution
- Decision: approved in conversation

## 1. Objective

Resolve seven related defects without weakening capability validation or storage safety:

1. Replace the creative workspace's inferred progress with real backend stage boundaries.
2. Restore readable option text in the light theme.
3. Expose quality, output format, compression quality, and moderation in the creative workspace.
4. Increase the space between the private asset filter toolbar and the first asset row.
5. Reduce the visual asset checkbox while preserving an accessible hit target.
6. Make a successful saved-config probe part of the admin default-storage workflow.
7. Model compression support as a boolean real-model capability instead of a model-level numeric quality value.

## 2. Root Causes

### 2.1 Task progress

The provider call is blocking and can run for more than a minute. During that call the worker persists only `status=running`; `progress_stage` and `progress_message` are API-time decorations, not stored execution state. The API therefore reports `routing` for the entire provider call. Storage and billing complete shortly after the provider returns, so the two-second SSE polling loop usually jumps directly to the terminal state.

The provider contract exposes no reliable percentage. A determinate percentage would remain synthetic even after fixing stage updates.

### 2.2 Workspace parameters and light theme

The backend task contract already accepts `quality`, `output_format`, `output_compression`, and `moderation`, but the workspace omits them and sends fixed defaults. Capabilities already aggregate most enum fields, while shared frontend types and normalization drop them. Compression support is currently represented by a model-level integer that defaults to `100`, so it cannot distinguish unsupported from supported.

Selected and hovered workspace options also force `text-white` on their descendants. That bypasses light-theme semantic tokens and makes labels unreadable on light accent surfaces.

### 2.3 Default storage activation

The observed admin flow ran a successful draft probe, created the configuration, and immediately attempted to set it as default. Draft probes intentionally do not persist. The saved row therefore retained `last_probe_status=never`, and the service correctly rejected activation. The UI did not distinguish a draft probe from a persisted probe or guide the user through the required sequence.

### 2.4 Asset gallery spacing and selection

The private gallery places the shared filter toolbar directly before the masonry grid with no page-level bottom margin. Its selection button also applies a 40px visual background over a class intended to render a 20px checkbox, making the check mark appear undersized.

## 3. Approved Design

### 3.1 Honest task stages

Persist `progress_stage` and `progress_message` on image tasks. The execution service updates them at real boundaries:

1. `queued`: accepted and waiting for a worker.
2. `provider`: candidate selected and provider generation in progress.
3. `persisting`: provider results returned and storage/persistence is in progress.
4. `settling`: persisted result count is known and billing settlement is in progress.
5. `completed` or `failed`: terminal outcome.

Parameter validation and route resolution happen before an asynchronous task is queued, so the UI must not move backward from queueing to routing. Existing tasks without stored stages use a corrected status-based fallback.

The frontend treats stage as authoritative. It does not invent a numeric percentage. During `provider`, it displays an indeterminate animation and elapsed time. Short stages may be skipped by polling when they complete between snapshots; the current visible stage and terminal outcome must still be accurate.

Progress updates must be lease-owner guarded so a stale worker cannot overwrite a task owned by another worker. Updating stage metadata must not replace results, billing data, or lease fields.

### 3.2 Real-model capability contract

Add `supports_output_compression: boolean` to real-model capability storage and DTOs. Keep task-level `output_compression` as a request integer.

The route-model capability response exposes:

- `quality: string[]`
- `output_format: string[]`
- `supports_output_compression: boolean`
- `moderation: string[]`

Enum fields remain candidate unions for display. Compression support aggregates with boolean OR. Resolver matching remains combination-aware:

- Compression `1-99` requires a candidate with `supports_output_compression=true` and a JPEG/WebP output format.
- Compression `100` is the compatibility/default value. A candidate without compression support may accept it, but the Worker omits the upstream compression parameter.
- PNG never sends upstream compression.
- Missing capability data defaults to compression unsupported. Existing model-level integer defaults must not be migrated to `true` because historical `100` values do not prove support.

The existing model-level numeric compression field may remain readable during transition, but new admin writes use the boolean as the authoritative capability.

### 3.3 Workspace parameter behavior

The workspace adds capability-driven controls for:

- Quality
- Output format
- Compression quality
- Moderation level

Compression uses a slider and synchronized numeric value from `1-100`. It is visible only when the selected output format is JPEG/WebP and the selected route model reports compression support. The default is `100`.

Switching model, task type, or output format normalizes invalid selections to the first supported value. Every parameter change invalidates the prior estimate and triggers the existing debounced estimate flow. Task creation uses exactly the normalized values used for the successful estimate.

Labels use localized display text while request values remain stable API enums. Unsupported or empty capability sets block generation through the existing readiness model.

### 3.4 Theme behavior

Introduce or reuse a semantic accent-contrast token instead of hardcoded white text. Active workspace controls use that token; inactive controls use foreground and muted tokens. Avoid descendant-wide `text-white` rules that override icons, labels, and metadata indiscriminately. Dark and light themes retain the same dimensions and interaction states.

### 3.5 Default storage workflow

Keep the backend rule that a default storage must be enabled for reads and writes and have a successful persisted probe.

The admin action becomes an explicit workflow:

1. Reject activation while the editor contains unsaved changes and ask the administrator to save.
2. If the saved config has no successful probe, call the saved-config probe endpoint.
3. If probe succeeds, call set-default with the version returned by the probe.
4. If probe fails, stop and show its specific message.
5. Refresh the selected row after success or failure.

The action communicates `validating` and `activating` states. A draft probe remains useful before creation, but it is described as a draft check and never treated as activation readiness.

### 3.6 Asset gallery polish

Add a private-gallery-only 32px gap after the filter toolbar so shared toolbar consumers are unaffected.

Keep a 40px selection button target. Render a separate 20-22px visual checkbox inside it:

- Unselected pointer state: visual frame transparent.
- Card hover or button focus-visible: frame and background become visible.
- Selected: accent fill and a properly sized check icon remain visible.
- Coarse pointer: retain a low-contrast visible frame because hover discovery is unavailable.

Preserve button semantics and `aria-pressed`; keyboard focus must remain visible.

## 4. Failure Handling And Compatibility

- Old tasks without stored progress fields retain fallback progress decoration.
- Unknown future progress stages render as a truthful running state rather than falling back to a fake percentage.
- Old clients sending compression `100` remain routable.
- Unsupported custom compression returns the existing capability-mismatch error before task creation.
- Storage probe failures never clear the current default and never bypass the service safety gate.
- A probe-induced version increment is always used by the following set-default request.
- Light-theme changes must not reduce dark-theme contrast.

## 5. Verification

Automated verification includes:

- Go tests for stage persistence, lease ownership, service stage order, API/SSE rendering, capability normalization and matching, model CRUD, provider omission, storage probe/default sequencing, and OpenAPI contracts.
- React contracts for stage mapping, elapsed/indeterminate behavior, capability normalization, parameter reset and payload construction, light-theme class safety, storage readiness, gallery spacing, and checkbox states.
- User and admin typecheck/build plus the repository verification entrypoint.
- Real API smoke after backend and contract changes.
- Docker rebuild followed by desktop and mobile browser checks in dark and light themes.

## 6. Non-Goals

- No synthetic provider percentage.
- No message bus or durable task-event history in this iteration.
- No removal of the server-side storage activation gate.
- No global restyling of the admin application or public gallery.
