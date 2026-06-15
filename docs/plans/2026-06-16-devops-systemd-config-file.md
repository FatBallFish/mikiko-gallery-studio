# DevOps Systemd Config File Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make backend DevOps artifacts start through systemd and use packaged `config.yaml` as the runtime configuration source.

**Architecture:** Backend runtime configuration values come from YAML, not environment overrides. DevOps packaging chooses `configs/config.<APP_ENV>.yaml` at build time, copies it into the backend artifact as `config.yaml`, and backend services run with the artifact directory as their working directory. API and worker run scripts install or update systemd units and return after restarting the service.

**Tech Stack:** Go config loader, Bash packaging scripts, POSIX shell systemd deployment scripts, YAML config templates.

---

### Task 1: Config Loader

**Files:**
- Modify: `internal/config/load.go`
- Modify: `internal/config/load_test.go`

**Steps:**
1. Remove business environment variable overrides from `Load`.
2. Keep config path environment variables as locator-only compatibility.
3. Change default config path to `config.yaml`.
4. Update tests to assert YAML values are preserved even when old override env vars are present.
5. Run `go test ./internal/config`.

### Task 2: DevOps Packaging

**Files:**
- Modify: `scripts/devops/package.sh`
- Add: `configs/config.pro.yaml`

**Steps:**
1. Add build-time `APP_ENV` handling with `dev` default and `prod|production` aliases to `pro`.
2. Fail clearly when the selected `configs/config.<env>.yaml` file is missing.
3. Copy the selected backend config into each backend artifact as `config.yaml`.
4. Stop copying backend env templates into backend artifacts.
5. Run `APP_ENV=dev scripts/devops/package.sh api-server` and `APP_ENV=pro scripts/devops/package.sh worker`.

### Task 3: Systemd Scripts

**Files:**
- Modify: `deployments/devops/run-api-server.sh`
- Modify: `deployments/devops/run-worker.sh`

**Steps:**
1. Replace direct `exec` behavior with service unit rendering.
2. Write units to `/etc/systemd/system/pic-gallery-api.service` and `/etc/systemd/system/pic-gallery-worker.service`.
3. Run `systemctl daemon-reload`, `enable`, and `restart`.
4. Support root and non-root execution through `sudo`.
5. Run shell syntax checks.

### Task 4: Documentation

**Files:**
- Modify: `deployments/devops/README.md`
- Remove: `deployments/devops/env/backend.env.example`

**Steps:**
1. Document `APP_ENV=dev|pro` build-time config selection.
2. Document backend artifact layout with `config.yaml`.
3. Document systemd script behavior and service names.
4. Remove stale backend env-file instructions.

### Task 5: Verification

**Commands:**
1. `go test ./internal/config`
2. `sh -n deployments/devops/run-api-server.sh deployments/devops/run-worker.sh`
3. `APP_ENV=dev scripts/devops/package.sh api-server`
4. `APP_ENV=pro scripts/devops/package.sh worker`
5. `./scripts/workflow/verify.sh`
6. `./scripts/workflow/api-smoke.sh` if a local API is available.
