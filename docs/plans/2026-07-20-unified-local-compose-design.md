# Unified Local Docker Environment Design

## Status

Approved by the user on 2026-07-20.

## Requirement

Remove the separate `pic-gallery-dev` and `pic-gallery-e2e` Docker Compose
environments. Create one new environment used by both development verification
and E2E tests. E2E must call the same development API and database. Preserve the
existing development database when creating the new environment.

## Goals

- Expose exactly one Pic Gallery Compose project, named `pic-gallery-local`.
- Preserve the existing development PostgreSQL data.
- Preserve MinIO and shared-storage data so database asset references remain valid.
- Make dev startup and Docker E2E use the same API, worker, web, database, Redis,
  MinIO, Mailpit, docs, and nginx services.
- Prevent E2E runs from permanently polluting the development database.
- Remove the old dev/e2e containers, networks, volumes, and obsolete Compose file
  only after the new environment has passed restoration and runtime checks.

## Non-goals

- Production Compose topology is unchanged.
- Redis cache contents are not migrated; Redis is rebuilt empty.
- E2E does not receive a second API service or a second test database.
- This change does not alter application-level schemas or API behavior.

## Chosen Architecture

Use `deployments/docker-compose/docker-compose.local.yml` as the only local full
stack definition, with Compose project name `pic-gallery-local`. The definition is
based on the current dev image-building topology and exposes a single nginx entry
point on port `8088`. Both `scripts/dev/up.sh` and
`scripts/e2e/run-docker-e2e.sh` target this file.

The E2E runner calls the same URLs used for development:

- API and user web through nginx: `http://127.0.0.1:8088`
- user web direct checks where required: `http://127.0.0.1:8088`
- admin web: `http://127.0.0.1:8088/admin/`
- docs web: `http://127.0.0.1:8088/developer-docs/`

The browser workflow must support the routed base paths rather than relying on the
old Vite development ports.

## Data Migration

Migration is ordered so no destructive action occurs before recoverability is
proven:

1. Start only the old dev PostgreSQL service and wait for readiness.
2. Create a custom-format `pg_dump` plus a plain schema/data inventory under the
   ignored `tmp/docker-migration/` directory.
3. Archive the old MinIO and shared-storage volumes.
4. Validate that the database dump is readable with `pg_restore --list` and that
   each archive can be listed.
5. Stop old projects without deleting volumes.
6. Create the new local volumes and restore PostgreSQL, MinIO, and shared storage.
7. Start `pic-gallery-local` and verify health plus selected row counts and asset
   inventory against the pre-migration manifest.
8. Run API smoke and Docker E2E against the new environment.
9. Only after all checks pass, delete the old dev and e2e containers, networks,
   and volumes.

The ignored migration backup remains on disk after delivery as a rollback artifact.

## E2E Database Protection

E2E uses the same development API and PostgreSQL database, as required. Before
each run it creates a custom-format database snapshot. A shell `trap` always runs
database restoration on success, failure, or interruption:

1. Snapshot the database through the running local PostgreSQL container.
2. Execute the existing API and Playwright workflows against the local URLs.
3. Stop API and worker to close database connections.
4. Terminate remaining database sessions, recreate the database, and restore the
   snapshot.
5. Restart API and worker, wait for readiness, and report restoration status.

If snapshot creation or validation fails, E2E does not start. If restoration fails,
the runner exits non-zero, retains the snapshot, and prints the recovery command.
The runner never invokes `docker compose down -v`.

E2E-created object files are also isolated by taking archives of MinIO and shared
storage before the run and restoring them in the same cleanup trap. This keeps
database and object state consistent.

## Command Changes

- `scripts/dev/up.sh`: start or rebuild `pic-gallery-local`.
- `scripts/dev/down.sh`: stop the unified project while preserving volumes by
  default. Volume deletion requires an explicit destructive confirmation token.
- `scripts/e2e/run-docker-e2e.sh`: start the same local project when requested,
  snapshot persistent state, run E2E, and restore state.
- The old `docker-compose.e2e.yml` is removed.
- Existing callers of `docker-compose.dev.yml` move to
  `docker-compose.local.yml`; the old dev file is removed after migration.

## Safety and Error Handling

- Migration and test backups are stored only under ignored `tmp/` paths.
- Scripts use `set -euo pipefail`, validate required commands, and quote paths.
- Restore operations validate archives before stopping application services.
- Old volumes are deleted by exact name only after successful validation.
- Secrets from local env files are never copied into logs or committed files.
- A failed migration leaves old volumes untouched and the new project disposable.

## Verification

- Contract tests assert there is one local Compose project and no dev/e2e Compose
  project names or volume-destructive E2E path.
- A migration dry run checks dump/archive creation and restoration into new volumes.
- Pre/post row-count and object-count manifests must match.
- `./scripts/workflow/verify.sh` passes.
- `./scripts/workflow/api-smoke.sh` passes.
- Docker E2E passes against `pic-gallery-local` and proves the dev database/object
  manifests are unchanged after restoration.
- `./scripts/workflow/review-local.sh --scope committed` and the review gate pass.
- `docker compose ls` shows only `pic-gallery-local` for this repository.

