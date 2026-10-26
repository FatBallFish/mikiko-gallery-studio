# Integrated canvas media nodes

## Requirement and approval

Source: repository owner detailed request and supplied competitor screenshot, 2026-10-24; implementation plan approved in this session. New image/video components combine media and generation. Empty selected nodes show floating controls; filled nodes show only media with fixed aspect ratio and canonical editable name. Toolbar groups selection/pan, connection, image/video/note creation, and separate rightmost asset library. Legacy prompt/image/image_generation/video_generation (and historical video asset display) remain readable and deletable, but cannot be newly created, copied, edited or executed.

## Design

Add image_v2/video_v2 node types to document schema version1 without rewriting historical documents or revisions. Empty new nodes are independently valid. Filled nodes can supply compatible media inputs. Preserve asset ownership, project validation, DAG/role constraints and deployed document limits. Inline draft uses existing image/video request fields plus prompt_template and variables; canonical bindings use stable aliases separate from editable display names. Support ${variable} input as an adapter to existing canonical template syntax.

Reuse existing canvas command history/persistence, server task orchestration, shared prompt editor/variables, capabilities/parameter normalization, uploads/picker, optimizer and quote semantics. Split media view and console from CanvasEditorPage. Console is viewport overlay anchored to selected empty node, normal readable UI scale; fullscreen modal shares same persisted draft. Upload/library selection adds references with matching graph edges. Reference picking validates ready media, role, duplicates and cycles. Preserve upload targeting and recheck target on async completion.

Both media types auto-estimate with ~300ms debounce and complete draft/input signatures; stale quotes cannot enable generation. Save then revalidate revision/quote before submitting. Result first output fills source v2 node; overflow creates deterministic sibling v2 nodes. Transactional idempotent attachment, conflicts/unplaced recovery and paid task/billing invariants remain. Legacy existing runs remain recoverable, but no new legacy runs. Failed runs retain drafts; retries use new idempotency keys.

Display real stages and only genuine reported percentages; indeterminate waiting animation otherwise. Filled resize uses common ratio-preserving helper. Canonical asset rename allows duplicates without changing reference aliases.

## Validation

Focused graph/service/store/API tests, frontend state/console/toolbar contracts, input/quote/retry/result idempotency and legacy compatibility; complete repository verification, isolated API smoke, concurrency checks, independent review, browser desktop/smaller viewport acceptance. No commit/push/deploy authorized. Preserve all pre-existing uncommitted media naming and artifact transfer changes.
