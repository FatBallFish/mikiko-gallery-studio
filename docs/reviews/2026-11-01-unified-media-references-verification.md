# Unified media references verification

Date: 2026-11-01
Scope: unify image reference resolution with media assets, migrate legacy references on API/Worker startup, align canvas asset queries and upload readiness, and use the same upload IDs throughout MCP image/video creation.

## Independent review

Backend migration and lifecycle review identified mixed live/deleted duplicate selection, gallery-reference deletion affecting its source, late unmapped references blocking cleanup, and historical alias rename consistency. Each finding was fixed and independently re-reviewed as closed. Frontend/MCP review identified early marked-reference replacement and unbounded abort cleanup; both were fixed with behavioral regressions and independently re-reviewed as closed. No remaining P0/P1/P2 findings were reported by these reviews.

## Migration verification

SQLite tests cover repeated backfill, concurrent callers, old unique-index retirement, permanent many-to-one legacy mappings, storage moves followed by restarts, existing tombstones, mixed live/deleted duplicate UUID ordering, owner isolation, different storage configurations, name/project preservation, and late aliases inheriting deletion.

A real isolated PostgreSQL 16 instance exercised `TestReferenceMediaBackfillPostgresConcurrentInstances`, including retirement of the old partial unique index and four concurrent migration callers. The same test is included in the isolated API smoke prerequisite checks. API/Worker startup ordering is tested to run reference migration after storage-identity cutover and before serving or cleanup.

## Upload and reference verification

Real Ent store/API tests cover unified upload IDs, old alias IDs, current canonical state, owner/type isolation, image preflight/create and execution loading. MCP transport integration covers upload, list, get, image/video preflight and creation using the same asset ID, checks zero new reference aliases, and executes image tasks against a fake native provider. Video quote/persistence are deterministic test doubles; no paid external generation is invoked.

Frontend behavior contracts cover readiness polling, direct S3 upload with proxy fallback, checksums, cancellation/timeouts, bounded cleanup, project/context changes, drawer pagination and refresh, marked-reference failure preservation, and ready_original success.

Final full repository verification, committed review gate and isolated worker/API smoke results are reported in the task delivery and must pass before remote publication.
