# API Contract Smoke Test

`scripts/test/api_contract_smoke.sh` creates a fully isolated local API contract environment and verifies the P0 API, Worker, billing, admin, and image-generation paths without external provider credentials.

## Prerequisites

- Bash, `curl`, Python 3, and Go.
- A running Docker daemon.
- Local image availability or registry access for `postgres:16-alpine` and `redis:7-alpine`.

The script starts its own API and Worker processes, a local fake image provider, and temporary PostgreSQL and Redis containers. Do not start an API, Worker, database, or Redis instance for this smoke test.

## Run

Use the workflow entrypoint:

```bash
./scripts/workflow/api-smoke.sh
```

The script chooses an unused loopback port by default. `BASE_URL` only selects the temporary API listening address; it does not point the smoke test at a live or pre-existing API. The override must be a free loopback HTTP address:

```bash
BASE_URL=http://127.0.0.1:18081 ./scripts/workflow/api-smoke.sh
```

The smoke covers readiness, authentication, billing and payment operations, API keys and HMAC signing, OpenAI-compatible endpoints, admin operations, model routing, asynchronous Worker execution, and public-gallery workflows.

## Isolation And Cleanup

Each run uses unique container names, random PostgreSQL and Redis host ports, a generated runtime env, and test-only storage and log directories below `$TMPDIR`. It never reads or writes the shared dev database.

The exit cleanup trap stops the API, Worker, and fake provider; force-removes the temporary PostgreSQL and Redis containers; and deletes the temporary runtime env, storage, logs, cookies, and seeded test data. Cleanup also runs after a failed assertion or interrupted startup.
