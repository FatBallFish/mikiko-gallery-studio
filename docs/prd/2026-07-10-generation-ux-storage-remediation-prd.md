# Generation UX And Storage Remediation Requirements

## Status

- Date: 2026-07-10
- Source: user-reported defects and requested behavior in the active development task
- Approval: approved in conversation
- Priority: P0 defect remediation

## 1. User Creative Workspace

### R1. Truthful generation progress

The output console must not remain on "model routing" for the entire provider generation and then immediately complete every remaining stage. It must reflect the real coarse execution boundary available from the backend. When a provider does not expose a percentage, the UI must use an indeterminate state rather than fabricate progress.

Acceptance criteria:

- A queued task displays queueing.
- A provider request in progress displays image generation, not model routing.
- Storage and billing stages reflect backend state when observed.
- Completion and failure remain accurate after reconnect or refresh.
- The UI does not show a synthetic numeric percentage.

### R2. Light-theme option readability

Model, ratio, pixel-size, and related option labels must use readable foreground colors in the light theme, including selected and hover states.

Acceptance criteria:

- No workspace option label is forced to an unsuitable white color by a component-local rule.
- Dark-theme contrast remains intact.
- Focus, hover, active, and disabled states remain distinguishable.

### R3. Output parameter controls

The creative workspace must expose model-supported quality, output format, compression quality, and moderation parameters.

Acceptance criteria:

- Options come from the selected route-model capability response.
- Output format and moderation use controlled choices.
- Compression quality is configurable from 1 to 100 only when the selected JPEG/WebP model supports it.
- Changing any parameter invalidates/recalculates the estimate.
- Estimate and task creation use the same normalized parameter set.

## 2. Private Asset Gallery

### R4. Filter-to-grid spacing

Increase the visual separation between the filter toolbar and the first asset row without changing unrelated shared-toolbar consumers.

Acceptance criterion: the private gallery has approximately 32px of space after its filter toolbar.

### R5. Asset selection control

The visible checkbox must be smaller and normally transparent, appearing on hover and when selected.

Acceptance criteria:

- The visual checkbox is approximately 20-22px.
- The pointer/touch target remains at least 40px.
- Hover, keyboard focus, selected, and touch-device states remain discoverable.
- Toggle semantics remain accessible.

## 3. Admin Storage Configuration

### R6. Default-storage activation

A newly added custom object-storage configuration must be able to become default after a successful real probe. The UI must not present a successful draft probe as if the saved configuration were activation-ready.

Acceptance criteria:

- The backend continues to reject default activation without a successful persisted probe.
- The admin action probes the saved configuration when needed.
- Set-default uses the version returned by that probe.
- Probe failure displays a specific error and leaves the current default unchanged.
- Unsaved editor changes cannot silently activate an older saved configuration.

## 4. Real Model Capability

### R7. Compression support flag

Replace the real-model parameter label "compression quality" with "supports compression quality". When enabled, the user workspace may configure compression quality for compatible formats.

Acceptance criteria:

- Real-model capability uses `supports_output_compression: boolean`.
- Task requests retain numeric `output_compression`.
- Route capabilities aggregate the boolean and resolver matching enforces it.
- Unsupported candidates do not receive an upstream compression parameter.
- Historical model values default to unsupported unless explicitly enabled.

## 5. Verification

- Changed Go behavior has focused tests.
- Shared API/OpenAPI contracts remain aligned.
- User and admin apps typecheck and build.
- Repository verification and isolated API smoke pass.
- The rebuilt Docker stack is browser-checked at desktop and mobile widths in relevant light/dark states.
