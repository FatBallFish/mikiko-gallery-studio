# MGSCTL Install, Upgrade, and Release Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver the renamed `mgsctl` tool with artifact-derived application versions, reliable runtime discovery and migrations, a bilingual dynamic TUI, renamed application artifacts, and a complete tag release pipeline.

**Architecture:** A signed-by-checksum Release Manifest is the source of truth that maps a concrete application version to immutable Docker digests and GitHub Release assets. Install and upgrade resolve selectors before persisting runtime state; runtime-aware commands share user configuration and deterministic directory discovery, while Docker and Native upgrades execute the target release's migration binary in the correct environment.

**Tech Stack:** Go 1.26, Bubble Tea, Docker Compose/buildx, Bash, PowerShell, GitHub Actions, Docker Hub, React/Vite build artifacts.

---

### Task 1: Rename the deployment tool surface to MGSCTL

**Files:**
- Move: `cmd/deployctl` -> `cmd/mgsctl`
- Move: `internal/deployctl` -> `internal/mgsctl`
- Modify: `Makefile`
- Modify: `scripts/install.sh`
- Modify: `scripts/install.ps1`
- Move: `scripts/devops/package-deployctl.sh` -> `scripts/devops/package-mgsctl.sh`
- Modify: `scripts/test/install-wrapper-contract.sh`
- Modify: `scripts/workflow/verify.sh`
- Modify: `internal/http/setupui/assets.go`
- Modify: `internal/http/setupui/page_test.go`
- Modify: `internal/setup/auth.go`
- Modify: `internal/setup/service_test.go`

**Step 1: Write failing brand-contract tests**

Update wrapper, command, self-update, catalog, setup UI, and workflow contract tests to require `mgsctl`, `MGSCTL_*`, `cmd/mgsctl`, `internal/mgsctl`, and `mgsctl-<os>-<arch>`. Add a repository-current-surface assertion that excludes historical requirements/designs and fails on executable `deployctl` references.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/deployctl ./internal/http/setupui ./internal/setup -count=1
./scripts/test/install-wrapper-contract.sh
```

Expected: FAIL because the binary, package, artifact names, output prefixes, and installer variables still use `deployctl`.

**Step 3: Rename implementation paths and public strings**

Move the command and package directories, update imports and package declarations, make `mgsctl` the only Make target, and change installer lookup/download/install/self-update variables and paths to `MGSCTL_*` and `mgsctl`. Rename runtime lock/stage names and Compose control variables because compatibility was explicitly rejected.

Keep historical docs under `docs/plans/2026-07-2*.md` and `docs/prd/2026-07-2*.md` unchanged.

**Step 4: Run formatting and focused tests to verify GREEN**

Run:

```bash
gofmt -w cmd/mgsctl internal/mgsctl internal/http/setupui internal/setup
go test ./internal/mgsctl ./internal/http/setupui ./internal/setup -count=1
./scripts/test/install-wrapper-contract.sh
```

Expected: PASS with no `cmd/deployctl` or `internal/deployctl` package remaining.

**Step 5: Commit**

```bash
git add Makefile cmd/mgsctl internal/mgsctl internal/http/setupui internal/setup scripts
git commit -m "refactor: rename deployment tool to mgsctl"
```

### Task 2: Add the Release Manifest contract and resolver

**Files:**
- Create: `internal/mgsctl/release_manifest.go`
- Create: `internal/mgsctl/release_manifest_test.go`
- Modify: `internal/mgsctl/native_release.go`
- Modify: `internal/mgsctl/self_update.go`
- Create: `scripts/devops/render-release-manifest.sh`
- Modify: `scripts/devops/release-contract-test.sh`

**Step 1: Write failing resolver tests**

Cover:

```go
func TestResolveLatestReleasePinsConcreteVersionAndDigests(t *testing.T)
func TestResolveReleaseRejectsChecksumVersionAndComponentDrift(t *testing.T)
func TestResolveReleaseRejectsUnknownSchemaAndMissingMigrationImage(t *testing.T)
```

Use an injected HTTP client and controlled local server. Assert that `latest` resolves to `v1.2.3`, each selected component has a `sha256:` digest, and asset checksums are exactly 64 hexadecimal characters.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/mgsctl -run 'TestResolve(Latest)?Release' -count=1
```

Expected: FAIL because no Manifest model or resolver exists.

**Step 3: Implement the minimal Manifest API**

Define a versioned JSON schema containing application version, commit, image records, and asset records. Resolve `latest` through the release endpoint, verify the adjacent Manifest checksum, decode with unknown-field rejection, validate SemVer/version consistency, and expose immutable image and asset selections.

Do not accept `latest`, `dev`, an empty digest, or inconsistent component versions as a resolved logical version.

**Step 4: Add and test the renderer contract**

Make `render-release-manifest.sh` consume already-built asset checksum files and image digest metadata, produce deterministic JSON, and fail when an expected image or asset is missing.

Run:

```bash
go test ./internal/mgsctl -run 'TestResolve(Latest)?Release' -count=1
./scripts/devops/release-contract-test.sh
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/mgsctl scripts/devops
git commit -m "feat(mgsctl): resolve immutable release manifests"
```

### Task 3: Make installation defaults and versions artifact-driven

**Files:**
- Modify: `internal/mgsctl/command.go`
- Modify: `internal/mgsctl/command_test.go`
- Modify: `internal/mgsctl/components.go`
- Modify: `internal/mgsctl/components_test.go`
- Modify: `internal/mgsctl/cli.go`
- Modify: `internal/mgsctl/cli_test.go`
- Modify: `internal/mgsctl/install.go`
- Modify: `internal/mgsctl/install_test.go`
- Modify: `internal/mgsctl/runtime.go`
- Modify: `internal/mgsctl/runtime_test.go`

**Step 1: Write failing default and derivation tests**

Add tests proving:

```go
command, _ := ParseCommand([]string{"install", "--yes"})
// docker + full + single, selector latest, no user application version

resolved, _ := ResolveInstallInput(ctx, *command.Install, resolver)
// ApplicationVersion == "v1.2.3" and ImageTag/Digests are concrete
```

Also assert `--application-version` is rejected for install/upgrade and explicit `--mode`, `--profile`, `--topology`, `--image-tag`, and `--release-version` still override selectors.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/mgsctl -run 'Test(ParseCommand.*Install|Install.*Default|.*Artifact.*Version)' -count=1
```

Expected: FAIL because `--yes` requires explicit flags and application version still defaults to `dev`.

**Step 3: Implement selector-first plan resolution**

Set parser defaults to Docker/full/single/latest, remove interactive Application version prompts, and insert a resolver stage before `BuildInstallPlan`. Persist the concrete application version and immutable Docker references returned by the Manifest. Keep advanced public API and registry CLI flags, but do not let them synthesize a logical version.

Generate stable source fallback versions from commit plus dirty content hash and inject that version into locally built images.

**Step 4: Verify GREEN**

Run:

```bash
go test ./internal/mgsctl -run 'Test(ParseCommand.*Install|Install|BuildRuntimeArtifacts|Artifact.*Version)' -count=1
```

Expected: PASS; generated runtime state contains neither bare `latest` nor bare `dev`.

**Step 5: Commit**

```bash
git add internal/mgsctl
git commit -m "feat(mgsctl): derive install versions from release artifacts"
```

### Task 4: Persist preferences and resolve runtime directories

**Files:**
- Create: `internal/mgsctl/user_config.go`
- Create: `internal/mgsctl/user_config_test.go`
- Create: `internal/mgsctl/runtime_resolver.go`
- Create: `internal/mgsctl/runtime_resolver_test.go`
- Modify: `internal/mgsctl/command.go`
- Modify: `internal/mgsctl/cli.go`
- Modify: `internal/mgsctl/operations.go`
- Modify: `cmd/mgsctl/main.go`

**Step 1: Write failing configuration and resolution tests**

Cover default Chinese locale, atomic preference round trips, corrupt JSON fallback, saved runtime preservation after failed install, explicit path precedence, current directory, `./runtime`, saved runtime, ambiguity, and missing candidates.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/mgsctl -run 'Test(UserConfig|ResolveRuntime)' -count=1
```

Expected: FAIL because no user configuration or shared runtime resolver exists.

**Step 3: Implement versioned user configuration**

Store `schema_version`, `language`, and `runtime_dir` under `os.UserConfigDir()/mgsctl/config.json`. Use a same-directory temporary file, restrictive permissions, fsync where supported, and atomic rename. Treat missing/corrupt language as `zh-CN`; report saved-runtime corruption in runtime-required commands.

**Step 4: Wire the shared runtime resolver**

Preserve whether `--runtime-dir` was explicitly supplied. Resolve explicit, cwd, cwd/runtime, then saved runtime without recursive scanning. Use the resolver for status, doctor, restart, upgrade, uninstall, setup operations, and cluster control operations. Save the absolute runtime path only after successful installation.

**Step 5: Verify GREEN and commit**

Run:

```bash
go test ./internal/mgsctl -run 'Test(UserConfig|ResolveRuntime|Run.*Runtime)' -count=1
```

Expected: PASS.

```bash
git add cmd/mgsctl internal/mgsctl
git commit -m "feat(mgsctl): remember and discover runtime directories"
```

### Task 5: Build the dynamic bilingual TUI

**Files:**
- Create: `internal/mgsctl/i18n.go`
- Create: `internal/mgsctl/i18n_test.go`
- Modify: `internal/mgsctl/catalog.go`
- Modify: `internal/mgsctl/catalog_test.go`
- Modify: `internal/mgsctl/tui.go`
- Modify: `internal/mgsctl/tui_test.go`
- Modify: `internal/mgsctl/tui_fields.go`
- Modify: `internal/mgsctl/tui_fields_test.go`

**Step 1: Write failing translation and field-state tests**

Add pure state tests for:

- default `zh-CN` root/catalog/form/review/error text;
- switching to `en-US` immediately and persisting it;
- full selecting its 9 components, excluding Monitoring, forcing S3;
- core/full component selection being read-only and custom being editable;
- conditional fields by mode/component;
- visible port defaults `8080/80/5173/5174/5175/9090`;
- install forms omitting Application version, Public API URL, Image registry, and Release version.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/mgsctl -run 'Test(TUI|I18N|Catalog)' -count=1
```

Expected: FAIL on English-only static fields and missing cross-field updates.

**Step 3: Implement message catalogs and dynamic form state**

Represent UI copy by stable message keys with complete `zh-CN` and `en-US` maps. Inject locale/config persistence into the TUI model. Recompute field visibility and preset selections on mode/profile/component changes while preserving valid custom input. Keep final arguments authoritative through `ParseCommand` and `BuildInstallPlan`.

**Step 4: Verify GREEN and commit**

Run:

```bash
go test ./internal/mgsctl -run 'Test(TUI|I18N|Catalog)' -count=1
```

Expected: PASS with all generated argument sets round-tripping through the parser.

```bash
git add internal/mgsctl
git commit -m "feat(mgsctl): add bilingual dynamic install TUI"
```

### Task 6: Run target-version migrations in the correct environment

**Files:**
- Modify: `internal/mgsctl/upgrade.go`
- Modify: `internal/mgsctl/upgrade_test.go`
- Modify: `internal/mgsctl/docker.go`
- Modify: `internal/mgsctl/docker_test.go`
- Modify: `internal/mgsctl/native.go`
- Modify: `internal/mgsctl/native_test.go`
- Modify: `internal/mgsctl/native_release.go`
- Modify: `cmd/mgsctl/main.go`
- Modify: `Dockerfile.api`
- Modify: `scripts/devops/package.sh`
- Modify: `scripts/workflow/native-package-contract.sh`

**Step 1: Write failing migration-order tests**

Cover target resolution, prepare/pull before migrate, Docker Compose one-shot migration using the target API digest, custom plans without an API component, Native target-bundle migration, rollback before migration, and forward-only recovery after a successful migration.

Assert the Docker process includes the current compose project, runtime env file, no host database port, and entrypoint `mikiko-gallery-studio-db-migrate`.

**Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/mgsctl -run 'Test(Upgrade|Docker.*Migration|Native.*Migration)' -count=1
```

Expected: FAIL because production migration still runs `app.RunDatabaseMigration` in the host process before target-image preparation.

**Step 3: Implement prepare, migrate, and roll phases**

Change upgrade dependencies to receive the resolved plan/release. Docker prepares immutable target images, runs the migration binary in the Compose network, then rolls services. Native stages the target bundle containing `mikiko-gallery-studio-db-migrate`, runs it with the target runtime, then updates services.

Retain current rollback rules: restore old state before migration succeeds; retain target state and print same-target resume guidance after migration succeeds.

**Step 4: Verify GREEN and package contracts**

Run:

```bash
go test ./internal/mgsctl -run 'Test(Upgrade|Docker.*Migration|Native.*Migration)' -count=1
./scripts/workflow/native-package-contract.sh
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/mgsctl internal/mgsctl Dockerfile.api scripts/devops/package.sh scripts/workflow/native-package-contract.sh
git commit -m "fix(mgsctl): migrate upgrades with target release tooling"
```

### Task 7: Rename all current application artifacts and images

**Files:**
- Modify: `Dockerfile.api`
- Modify: `Dockerfile.worker`
- Modify: `deployments/docker-compose/docker-compose.local.yml`
- Modify: `deployments/docker-compose/docker-compose.prod.yml`
- Modify: `deployments/devops/run-api-server.sh`
- Modify: `deployments/devops/run-worker.sh`
- Modify: `deployments/monitoring/prometheus.yml`
- Modify: `scripts/docker/images.sh`
- Modify: `scripts/dev/test-local-bootstrap.sh`
- Modify: `scripts/test/api_contract_smoke.sh`
- Modify: `scripts/test/api_contract_smoke_contract_test.sh`
- Modify: `scripts/devops/package.sh`

**Step 1: Write failing naming contracts**

Require the five `docker.io/fatballfish/mikiko-gallery-studio-*` repositories and `mikiko-gallery-studio-*` binary/bundle names. Forbid current-surface `pic-gallery-*` executable/image references while excluding historical documents and stable API/domain identifiers.

**Step 2: Run tests to verify RED**

Run:

```bash
./scripts/test/api_contract_smoke_contract_test.sh
./scripts/workflow/native-package-contract.sh
./scripts/devops/release-contract-test.sh
```

Expected: FAIL on old image and binary names.

**Step 3: Rename build outputs and runtime references**

Update Dockerfiles, Compose, package layouts, service scripts, health/smoke selectors, and image tooling. Make the default registry `docker.io/fatballfish`. Ensure buildx receives both version and optional latest tags in the same pushed build and carries required OCI labels.

**Step 4: Verify GREEN and commit**

Run:

```bash
./scripts/test/api_contract_smoke_contract_test.sh
./scripts/workflow/native-package-contract.sh
./scripts/devops/release-contract-test.sh
```

Expected: PASS.

```bash
git add Dockerfile* deployments scripts
git commit -m "refactor: rename application release artifacts"
```

### Task 8: Complete the tag release workflow

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/devops/package.sh`
- Modify: `scripts/devops/package-mgsctl.sh`
- Modify: `scripts/devops/render-release-manifest.sh`
- Modify: `scripts/devops/release-contract-test.sh`
- Modify: `scripts/docker/images.sh`

**Step 1: Extend failing workflow contracts**

Require SemVer validation, full verification, mgsctl matrix, API/Worker package matrix, three frontend packages, Docker Hub login, five multi-arch images, digest capture, Manifest generation/verification, Release upload dependencies, and latest promotion after successful Release publication.

**Step 2: Run the contract to verify RED**

Run:

```bash
./scripts/devops/release-contract-test.sh
```

Expected: FAIL because the workflow only builds the old control binary and native bundle.

**Step 3: Add independently retryable release jobs**

Package API/Worker by OS/arch and frontend bundles once. Generate adjacent checksums. Authenticate with `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN`, publish version tags, record digests, build the Manifest, upload only missing identical assets, verify the published Release, then promote latest by digest.

Never expose secrets to pull requests or run publishing jobs for non-tag refs.

**Step 4: Verify workflow and packagers locally**

Run:

```bash
./scripts/devops/release-contract-test.sh
bash -n scripts/devops/package-mgsctl.sh scripts/devops/package.sh scripts/devops/render-release-manifest.sh scripts/docker/images.sh
```

Expected: PASS.

**Step 5: Commit**

```bash
git add .github/workflows/release.yml scripts/devops scripts/docker/images.sh
git commit -m "ci: publish complete tagged application releases"
```

### Task 9: Update current documentation and add Docker upgrade E2E

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/runbooks/backend-deployment.md`
- Modify: `docs/deploy/backend-runbook.md`
- Modify: `deployments/devops/README.md`
- Modify: `scripts/workflow/deployment-docs-contract.sh`
- Create: `scripts/e2e/mgsctl-upgrade-docker-e2e.sh`
- Modify: `scripts/e2e/deployment-e2e-lib.sh`

**Step 1: Write failing documentation and E2E contracts**

Require current docs to use `mgsctl`, explain latest resolution versus persisted concrete versions, list new assets/images, document TUI language settings and runtime discovery, and distinguish `self-update` from application `upgrade`.

The E2E must create a temporary runtime with free ports and unique Compose project/volumes; it must explicitly reject the repository's existing `runtime/` path.

**Step 2: Run contracts to verify RED**

Run:

```bash
./scripts/workflow/deployment-docs-contract.sh
./scripts/e2e/mgsctl-upgrade-docker-e2e.sh --contract-only
```

Expected: FAIL because current docs and E2E use old naming and have no real upgrade migration path.

**Step 3: Update docs and implement E2E**

Document the approved production workflow in English and Chinese. Implement full Docker install, Setup completion, invocation from outside runtime, target migration in Compose network, rolling update, and final readiness checks. Cleanup only the generated temporary directory/project/volumes.

**Step 4: Verify GREEN and commit**

Run:

```bash
./scripts/workflow/deployment-docs-contract.sh
./scripts/e2e/mgsctl-upgrade-docker-e2e.sh --contract-only
```

Expected: PASS.

```bash
git add README.md README.zh-CN.md docs deployments/devops/README.md scripts/e2e scripts/workflow/deployment-docs-contract.sh
git commit -m "docs: document mgsctl installation and releases"
```

### Task 10: Run complete verification, smoke, E2E, and review gates

**Files:**
- Modify as needed: only files already in this plan
- Generated: `.review/gate.json`

**Step 1: Run focused Go and contract tests**

```bash
go test ./internal/mgsctl -count=1
./scripts/test/install-wrapper-contract.sh
./scripts/devops/release-contract-test.sh
./scripts/workflow/deployment-docs-contract.sh
```

Expected: PASS.

**Step 2: Run repository verification**

Use `dev-verify` and run:

```bash
./scripts/workflow/verify.sh
```

Expected: `OK: verification passed`.

**Step 3: Run isolated API smoke**

Use `dev-api-smoke` and run:

```bash
./scripts/workflow/api-smoke.sh
```

Expected: PASS with its temporary API, Worker, PostgreSQL, and Redis cleaned up.

**Step 4: Run Docker upgrade E2E**

```bash
./scripts/e2e/mgsctl-upgrade-docker-e2e.sh
```

Expected: install, Setup, Manifest-resolved upgrade, Compose-network migration, and readiness all PASS without touching the repository `runtime/`.

**Step 5: Search current surfaces for stale names**

```bash
rg -n --glob '!docs/plans/2026-07-2*.md' --glob '!docs/prd/2026-07-2*.md' --glob '!runtime/**' 'deployctl|DEPLOYCTL|pic-gallery-(api|worker|gateway|user-web|admin-web|docs-web|db-migrate|native)' Makefile cmd internal deployments scripts .github README.md README.zh-CN.md docs/runbooks docs/deploy
```

Expected: no current-surface matches.

**Step 6: Run review gate**

Use `dev-review-gate`:

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: PASS marker for the current committed tree.

**Step 7: Final implementation commit if verification required fixes**

```bash
git add <only-files-fixed-during-verification>
git commit -m "fix: address mgsctl verification findings"
```

Re-run Steps 1-6 after any commit because the committed-scope marker is tree-specific.

### Task 11: Migrate verifiable legacy setup binding digests during upgrade

**Files:**
- Modify: `cmd/db-migrate/main.go`
- Modify: `internal/app/migrate.go`
- Modify: `internal/mgsctl/upgrade.go`
- Modify: `internal/mgsctl/docker.go`
- Modify: `internal/mgsctl/native.go`
- Modify: `internal/repository/entstore/setup_store.go`
- Modify: `internal/setup/service.go`
- Create: `internal/setup/legacy_binding_reconcile.go`

Use the upgrade's previous release identity to reproduce the legacy digest, require matching completed state and database identities, update the database digest with compare-and-swap, and atomically reconcile `install-state.json`. Invoke this only from the target migration binary's explicit upgrade flag. Keep ordinary startup fail-closed and cover legacy, canonical, forged, rollback, Docker, and Native paths with focused tests.
