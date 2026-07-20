# Unified Local Docker Environment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the separate dev and E2E Compose projects with one `pic-gallery-local` environment, migrate existing dev PostgreSQL and object data, and make E2E test the shared dev API/database while restoring persistent state after every run.

**Architecture:** Keep the current dev image-building topology as the single local stack and route all browser/API checks through nginx on port 8088. Migrate PostgreSQL with `pg_dump`/`pg_restore`, migrate MinIO and shared storage with validated archives, and wrap E2E in a snapshot/restore trap so it directly exercises the dev database without leaving test data behind.

**Tech Stack:** Docker Compose v2, PostgreSQL 16, Redis 7, MinIO, Bash, Node.js E2E runner, Playwright, TypeScript source contracts.

---

### Task 1: Establish the new coding context

**Files:**
- Existing: `docs/plans/2026-07-20-unified-local-compose-design.md`
- Existing: `docs/plans/2026-07-20-unified-local-compose.md`
- Ignored: `.coding-context.json`

**Step 1: Run the repository workflow**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "Unify dev and E2E Docker environments while preserving dev data"
```

Expected: `.coding-context.json` references the approved design and this implementation plan.

**Step 2: Confirm secret and backup paths are ignored**

Run:

```bash
git check-ignore .env.local tmp/docker-migration/probe
```

Expected: both paths are ignored.

### Task 2: Add failing unified-environment contracts

**Files:**
- Create: `web/shared/unified-local-compose.contract.ts`
- Modify: `web/shared/prompt-optimization-deployment.contract.ts`

**Step 1: Write the failing source contract**

Assert that:

- `docker-compose.local.yml` exists and declares `name: pic-gallery-local`;
- the old dev/e2e Compose files do not exist;
- dev and E2E scripts reference the local Compose file;
- the E2E script defaults to nginx port 8088 and never contains `down -v`;
- E2E invokes snapshot and restore helpers through a cleanup trap;
- the local Compose API and worker both receive the prompt quote signing key;
- the migration script requires validated backups before old-volume deletion.

**Step 2: Run the contract and verify RED**

Run:

```bash
npm exec --prefix web/user -- tsx web/shared/unified-local-compose.contract.ts
```

Expected: FAIL because the local Compose file and migration/snapshot scripts do not exist.

**Step 3: Commit the failing contract**

```bash
git add web/shared
git commit -m "test(docker): define unified local environment contract"
```

### Task 3: Create the single local Compose topology

**Files:**
- Create: `deployments/docker-compose/docker-compose.local.yml`
- Delete: `deployments/docker-compose/docker-compose.dev.yml`
- Delete: `deployments/docker-compose/docker-compose.e2e.yml`
- Modify: `scripts/dev/up.sh`
- Modify: `scripts/dev/down.sh`
- Modify: `Makefile`
- Modify: `scripts/workflow/docs-web-contract-test.sh`
- Modify: `web/shared/prompt-optimization-deployment.contract.ts`

**Step 1: Base local Compose on the dev topology**

Use `name: pic-gallery-local`, retain the built API/worker/web images, keep nginx on
`${LOCAL_NGINX_PORT:-8088}`, and use named volumes for PostgreSQL, Redis, MinIO,
and shared storage. Add fixed local email-code settings required by E2E, but do not
add separate API, worker, or database services.

**Step 2: Point dev lifecycle commands at local Compose**

`scripts/dev/up.sh` must run `up -d --build --remove-orphans` on the local file.
`scripts/dev/down.sh` must preserve volumes by default. A volume purge must require
the exact arguments `--volumes --confirm-destroy-local-data`.

**Step 3: Remove old Compose definitions and update contracts**

Update deployment/docs contracts to expect only the local and production files.

**Step 4: Run focused checks**

```bash
docker compose --env-file deployments/docker-compose/.env.example -f deployments/docker-compose/docker-compose.local.yml config -q
npm exec --prefix web/user -- tsx web/shared/unified-local-compose.contract.ts
npm exec --prefix web/user -- tsx web/shared/prompt-optimization-deployment.contract.ts
```

Expected: PASS for Compose parsing and deployment contracts except snapshot/migration assertions still awaiting later tasks.

**Step 5: Commit**

```bash
git add deployments/docker-compose scripts/dev scripts/workflow Makefile web/shared
git commit -m "refactor(docker): unify local compose topology"
```

### Task 4: Implement persistent-state snapshot and restore

**Files:**
- Create: `scripts/e2e/local-state.sh`
- Create: `scripts/e2e/local-state.contract.sh`

**Step 1: Write a failing shell contract**

Use disposable test volumes and a temporary PostgreSQL container to prove:

- `snapshot` refuses an unreadable database;
- successful snapshots contain a readable custom dump and listable archives;
- `restore` recreates the original database rows and object file checksums;
- a failed restore retains its snapshot directory;
- no helper removes the local named volumes.

**Step 2: Run the contract and verify RED**

```bash
./scripts/e2e/local-state.contract.sh
```

Expected: FAIL because `local-state.sh` is missing.

**Step 3: Implement `local-state.sh`**

Provide explicit `snapshot <directory>` and `restore <directory>` commands. Use
the running `pic-gallery-local-postgres-1` container for `pg_dump`/`pg_restore`,
archive `pic-gallery-local_minio-data` and
`pic-gallery-local_shared-storage`, and flush Redis after restore. Validate all
artifacts before returning success.

**Step 4: Run the shell contract and verify GREEN**

```bash
./scripts/e2e/local-state.contract.sh
```

Expected: PASS and no disposable test volume remains.

**Step 5: Commit**

```bash
git add scripts/e2e/local-state.sh scripts/e2e/local-state.contract.sh
git commit -m "feat(e2e): snapshot and restore local state"
```

### Task 5: Make E2E use and restore the dev environment

**Files:**
- Modify: `scripts/e2e/run-docker-e2e.sh`
- Modify: `scripts/e2e/docker-e2e.mjs`
- Modify: `scripts/e2e/prompt-workflow-browser.py`
- Modify: `web/shared/prompt-optimization-e2e.contract.ts`
- Modify: `web/shared/unified-local-compose.contract.ts`

**Step 1: Extend failing contracts**

Assert local defaults:

```text
BASE_URL=http://127.0.0.1:8088
USER_WEB_URL=http://127.0.0.1:8088
ADMIN_WEB_URL=http://127.0.0.1:8088/admin
NGINX_URL=http://127.0.0.1:8088
```

Assert the runner snapshots before Node E2E, restores from an EXIT/INT/TERM trap,
and rejects the old `--clean` option.

**Step 2: Run contracts and verify RED**

Run both unified and prompt E2E contracts. Expected: FAIL on old URLs and missing
state protection.

**Step 3: Implement the shared runner**

`--start` starts or rebuilds `pic-gallery-local`; no option may delete volumes.
Before testing, stop API/worker/MinIO, snapshot PostgreSQL and object volumes,
restart services, and wait for `/readyz`. Cleanup stops writers, restores the
snapshot, restarts services, verifies readiness, and removes only the successful
temporary snapshot. On restoration failure it retains the snapshot and exits
non-zero.

Update browser URL composition for the `/admin/` base path.

**Step 4: Run focused contracts**

```bash
npm exec --prefix web/user -- tsx web/shared/unified-local-compose.contract.ts
npm exec --prefix web/user -- tsx web/shared/prompt-optimization-e2e.contract.ts
node --check scripts/e2e/docker-e2e.mjs
```

Expected: PASS.

**Step 5: Commit**

```bash
git add scripts/e2e web/shared
git commit -m "test(e2e): run against restored local environment"
```

### Task 6: Implement the one-time dev-data migration

**Files:**
- Create: `scripts/dev/migrate-unified-local.sh`
- Modify: `web/shared/unified-local-compose.contract.ts`

**Step 1: Add migration safety assertions**

Assert the script backs up PostgreSQL, MinIO, and shared storage, validates dumps
and archives, compares pre/post manifests, and does not remove any old volume until
the new environment has passed API and E2E checks.

**Step 2: Run contract and verify RED**

Expected: FAIL because the migration script is missing.

**Step 3: Implement the migration script**

Support `--execute` only. Create timestamped backups under
`tmp/docker-migration/`, use exact old/new project and volume names, restore into
new volumes, compare database table row counts and object checksums, start the
unified stack, and print the exact final-cleanup command. Do not delete old volumes
inside the migration script.

**Step 4: Run static and shell syntax checks**

```bash
bash -n scripts/dev/migrate-unified-local.sh scripts/e2e/local-state.sh scripts/e2e/run-docker-e2e.sh
npm exec --prefix web/user -- tsx web/shared/unified-local-compose.contract.ts
```

Expected: PASS.

**Step 5: Commit**

```bash
git add scripts/dev/migrate-unified-local.sh web/shared/unified-local-compose.contract.ts
git commit -m "feat(docker): migrate dev data into local environment"
```

### Task 7: Execute migration and validate preserved data

**Files:**
- Runtime ignored: `tmp/docker-migration/**`
- Runtime Docker resources only

**Step 1: Record old-resource inventory**

Record exact old containers, networks, volumes, database row-count manifest, and
object checksums without printing secrets.

**Step 2: Run migration**

```bash
./scripts/dev/migrate-unified-local.sh --execute
```

Expected: backup validation, restore, manifest comparison, and local service health
all pass. Old volumes remain intact at this checkpoint.

**Step 3: Run API smoke and shared-environment E2E**

```bash
./scripts/workflow/api-smoke.sh
./scripts/e2e/run-docker-e2e.sh
```

Expected: PASS. The pre/post dev-state manifest generated by E2E is identical.

**Step 4: Remove old projects and volumes**

Delete only resources whose names start with exact prefixes `pic-gallery-dev_` and
`pic-gallery-e2e_`, after confirming the new project remains healthy. Keep the
timestamped backup.

**Step 5: Verify one environment remains**

```bash
docker compose ls --all
docker volume ls
```

Expected: only `pic-gallery-local` remains for Pic Gallery; no dev/e2e containers,
networks, or volumes remain.

### Task 8: Full verification, review, and merge

**Files:**
- Ignored: `.review/gate.json`

**Step 1: Run repository verification**

```bash
./scripts/workflow/verify.sh
```

Expected: PASS.

**Step 2: Run final local API and E2E checks**

```bash
./scripts/workflow/api-smoke.sh
./scripts/e2e/run-docker-e2e.sh
```

Expected: PASS and local persistent-state manifest unchanged.

**Step 3: Run review gate**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: PASS marker for current tree.

**Step 4: Merge into main**

Confirm the main worktree is clean, merge `codex/unified-local-compose`, regenerate
the review marker on main, and confirm the unified project remains healthy.

