# Public media and task history remediation

## Requirement source

Owner request 2026-10-24: public homepage/image details hide edit/start-frame/group/rename/delete and other owner mutations, output quantity/project ownership and raw route model codes; published assets show truthful visibility. Add video publication with suitable public detail projection. Remove image/video share buttons. Starting image editing must not report missing historical settings as failures. Failed image/video history tasks support retry as NEW tasks preserving full original parameters and records. History single delete and clear-all-fully-failed must retain assets; partial output tasks excluded from bulk cleanup. Grouping uses existing single group_name with searchable existing groups and new-name creation instead browser prompt.

## Technical design

Explicit public/owned detail context. Public mode never enriches with private owner APIs even if viewer owns asset; no owner actions or project/internal route code in projection. Normalize approved/public visibility without overwriting canonical result state. Video publication follows pending review/approved/rejected/private lifecycle with admin moderation and approved-only public list/detail/poster/playback access; owner can withdraw, unauthorized assets never exposed. Reuse existing media storage/auth and image moderation patterns. Public video details omit owner and internal execution metadata.

History hiding is distinct from execution deletion: use dedicated durable per-user hidden history state, not task.deleted_at or result/media deletion. Hidden tasks remain reachable for owned source metadata and worker recovery; history lists filter hidden. Single deletion terminal only, bulk targets terminal completely failed with zero successes across all matches (project filter if supplied), excluding partial/rejected/cancelled unless status is explicitly failed. No object cleanup or billing changes. Retry reuses faithful request fields, variables, references and video inputs, validates current capability and fresh quote, creates a new task/idempotency key; no loss of original failure. If inputs/model now unavailable return actionable error instead silent configuration change.

Group selection remains single-select group_name, clear option + distinct server group query scoped user/project so all groups independent of loaded page. Share UI removed. Asset-edit intent uses quiet current defaults, distinct from historical restore warnings.

## Validation

Public details logged out/logged in owner, visibility aliases; video publish/review/withdraw/public access; retry new identities/full settings/fresh video quote; terminal history hide without result/ref loss; server bulk partial exclusion and ownership; distinct group query/create/select/clear; focused tests then full verify, isolated API smoke, independent review and real local UI. Preserve all preexisting canvas/media/artifact changes; no commit/push/deploy authorized.
