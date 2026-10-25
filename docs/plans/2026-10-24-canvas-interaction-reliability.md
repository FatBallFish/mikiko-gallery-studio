# Canvas interaction and reliability repair

## Requirement source

Owner request2026-10-24: borderless prompt toolbar; unified select/pan gesture (drag node moves node, drag background pans); connections from either end/port represent references irrespective visual direction; connect tool click node starts pointer-following ghost until other compatible node/Escape; canvas-reference hover only eligible highlights; filled video nodes select+drag instead media playback intercept; diagnose/fix image edit reference notfound, slow estimates, local upload stuck creating session, and single-editor revision conflicts.

## Design

Preserve historical documents, billing and worker recovery. New media node graph retains canonical resource->emptygenerator directed edge internally for validation/dependency execution, but UI gestures resolve either start endpoint and either port to that relation. Visual anchors retain chosen sides separately source_handle/target_handle. No arbitrary incompatible links or cycles; active legacy restrictions retained. Both filled resource nodes can be associated only where meaningful without invalid generation roles; generator references get role from current mode slots, not left/right.

Default pointer mode selects/drags nodes, pans background, one combined tool; modifiers for box select if retained explicit. Filled video preview is inert poster at rest, explicit playback/detail control after selection; click/drag surfaces never toggle video unexpectedly. Reference pick eligible hover via same validator used click; ineligible no hover styling.

Investigate request provenance before changes. Fix media UUID/reference alias adaptation on server at shared canvas generation boundary. Estimate must validate full inputs without downloading full binary on every quote, cache/reuse bounded metadata if existing helpers support; stale responses cancel/signature guard and truthful timing. Upload session timeout/errors+targeting preserve unified asset pipeline. Save scheduler and server result attachment coordinate revision acknowledgements/rebase without overwriting local changes or auto-resubmitting paid tasks; genuine external conflicts still explicit.

## Frontend follow-up acceptance (2026-10-24)

1. Blank canvas uses the same default arrow cursor as nodes; Space/pan retain explicit grab feedback.
2. A primary blank click clears node and edge selection, closes the generation console (selection-derived), connection/reference picking and double-click menu. Background drag preserves selection. Node/edge/control pointerup bubbling, middle/right clicks, pinch, long press and cancelled gestures never count as a blank click.
3. Hide the App-level UploadTray presentation only while editing a canvas. Keep the component mounted, its queue/controllers, file input and upload request/completion events active across navigation. Restore and retain the dock asset library button; canvas list and other routes keep the upload tray.
4. The short handle sits at the horizontal center of the dock, is visible on desktop and tablet, shows grab/grabbing cursors, and supports horizontal pointer capture dragging.
5. Clamp the dock against its actual containing editor and measured width; observe editor/dock resize to reposition after screen/orientation changes. Small screens keep content-sized dock translation and move zoom controls onto a separate row.

Implementation stays in `web/user` and its frontend contracts. Preserve existing backend and unrelated uncommitted work. Escape shares transient cleanup with blank click, respects editable/dialog controls, and has no unreachable legacy branch. Contract checks read cursor rules from CSS, retain applicable existing coverage across extracted components, and cover event ownership and persistent hidden upload processing.

## Verification

Focused gesture/core state, bidirectional port canonical orientation+visualanchors, reference hover, video click/drag, fake timers estimate/saves/remoteattach, real local API upload/ref estimates; ownership/cycle/legacy guards; full verify/review/API smoke as appropriate. No commit/push/paid real generation. Preserve all previous uncommitted API/MCP/canvas/media changes.
