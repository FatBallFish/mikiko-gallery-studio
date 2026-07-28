# Deployctl Installation and Versioning Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate production deployment on deployctl, make release-unavailable installation fall back to a trusted local build, and add visible deployctl versions plus explicit self-update support.

**Architecture:** Keep deployment policy in the Go deployctl CLI and keep shell/PowerShell installers as thin bootstrap layers. Build metadata is injected through one Make target, while self-update uses dependency-injected download and replacement logic so tests never contact GitHub. Tagged GitHub Releases publish the exact artifacts consumed by both installers and native deployment.

**Tech Stack:** Go 1.24, POSIX shell, PowerShell, GNU Make, GitHub Actions, Node/TypeScript contract tests.

---

### Task 1: Establish the repository coding context

**Files:**
- Create: `.coding-context.json`
- Reference: `docs/plans/2026-07-28-deployctl-install-version-design.md`
- Reference: `docs/plans/2026-07-28-deployctl-install-version.md`

**Step 1: Run the required pre-coding workflow**

Run the `dev-start-coding` skill and point it at the approved design and this implementation plan.

Expected: `.coding-context.json` records both documents, repository state, approval, and heavyweight verification requirements.

**Step 2: Validate the context**

Run:

```bash
./scripts/workflow/check-coding-context.sh
```

Expected: PASS with the design and implementation sources accepted.

**Step 3: Commit the context if repository policy tracks it**

Run `git status --short`. If `.coding-context.json` is intentionally ignored, leave it uncommitted; otherwise commit it with the next test-first change.

### Task 2: Add build identity and the `version` command

**Files:**
- Create: `internal/deployctl/buildinfo.go`
- Create: `internal/deployctl/buildinfo_test.go`
- Modify: `internal/deployctl/command.go`
- Modify: `internal/deployctl/command_test.go`
- Modify: `internal/deployctl/cli.go`
- Modify: `internal/deployctl/cli_test.go`
- Modify: `cmd/deployctl/main.go`
- Modify: `Makefile`

**Step 1: Write failing build information tests**

Add tests for a value shaped like:

```go
BuildInfo{
    Version: "v1.2.3",
    Commit: "0123456789abcdef",
    BuildTime: "2026-07-28T00:00:00Z",
    Dirty: false,
}
```

Assert deterministic text output and JSON fields `version`, `commit`, `build_time`, `dirty`, `go_version`, `go_os`, and `go_arch`. Assert missing linker values normalize to a clearly labeled development build rather than an empty or official-looking version.

**Step 2: Write failing command and CLI tests**

Add parse cases for `deployctl version` and `deployctl version --json`. Reject positional arguments and duplicate flags. Inject build info through `CLIDependencies` and assert `Run` returns zero without invoking deployment dependencies.

**Step 3: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/deployctl -run 'Test(BuildInfo|ParseVersion|RunVersion)' -count=1
```

Expected: FAIL because build info and the version command do not exist.

**Step 4: Implement the minimal version path**

Add `CommandVersion`, `VersionOptions{JSON bool}`, build-info normalization/formatting, and a CLI case that writes text or JSON. Keep tool version separate from `DefaultApplicationVersion`.

Define linker-populated variables in `cmd/deployctl/main.go` and pass normalized `BuildInfo` into CLI dependencies.

**Step 5: Add the canonical Make target**

Add variables for output, version, commit, build time, and dirty state. The target must use `-trimpath` and `-ldflags` to populate the four tool fields:

```make
deployctl:
	$(GO) build -trimpath -ldflags "..." -o "$(DEPLOYCTL_OUTPUT)" ./cmd/deployctl
```

Ensure quoted paths work and a clean checkout derives its version from an exact tag or a development identifier.

**Step 6: Run focused tests and inspect a real build**

Run:

```bash
go test ./internal/deployctl -run 'Test(BuildInfo|ParseVersion|RunVersion)' -count=1
make deployctl DEPLOYCTL_OUTPUT="$(mktemp -d)/deployctl"
```

Then run the produced binary with `version` and `version --json`.

Expected: tests PASS and both outputs contain the current commit and a non-empty version.

**Step 7: Commit**

```bash
git add Makefile cmd/deployctl/main.go internal/deployctl/buildinfo.go internal/deployctl/buildinfo_test.go internal/deployctl/command.go internal/deployctl/command_test.go internal/deployctl/cli.go internal/deployctl/cli_test.go
git commit -m "feat(deployctl): expose tool build version"
```

### Task 3: Add explicit deployctl self-update

**Files:**
- Create: `internal/deployctl/self_update.go`
- Create: `internal/deployctl/self_update_test.go`
- Create: `internal/deployctl/self_update_unix.go`
- Create: `internal/deployctl/self_update_windows.go`
- Modify: `internal/deployctl/command.go`
- Modify: `internal/deployctl/command_test.go`
- Modify: `internal/deployctl/cli.go`
- Modify: `internal/deployctl/cli_test.go`
- Modify: `cmd/deployctl/main.go`

**Step 1: Write failing command tests**

Add cases for:

```text
self-update
self-update --version v1.2.3
self-update --version v1.2.3 --yes
```

Assert default version `latest`, configurable release base/download URL/checksum, no positional arguments, and no collision with the existing application `upgrade` command.

**Step 2: Write failing updater tests**

Use `httptest.Server` and temporary executables to assert:

- platform artifact and adjacent checksum URL selection
- checksum verification before any replacement
- mismatch returns a security error and preserves the original file
- HTTP 404 returns an actionable release-unavailable error without a source fallback
- cancellation stops before replacement
- explicit confirmation is required without `--yes`
- Unix replacement uses a same-directory staged file and preserves executable mode
- Windows replacement command waits for the old PID, moves `.new`, and retains recovery details on failure

**Step 3: Run tests and confirm failure**

```bash
go test ./internal/deployctl -run 'Test(ParseSelfUpdate|SelfUpdate|RunSelfUpdate)' -count=1
```

Expected: FAIL because the command and updater are absent.

**Step 4: Implement dependency-injected update logic**

Introduce `SelfUpdateOptions`, `SelfUpdateDependencies`, artifact resolution, bounded HTTP downloads, SHA-256 parsing, same-filesystem staging, and platform replacement interfaces. Never log credentials embedded in override URLs.

Use terminal confirmation to show current and target versions. `--yes` bypasses only the confirmation, never checksum verification.

**Step 5: Implement platform replacement**

Unix: sync and chmod the staged file, then atomic rename over the current executable.

Windows: retain `<executable>.new`, spawn a safely quoted PowerShell process that waits for the current PID to exit, atomically moves the file, and emits a recovery command if scheduling fails. Keep platform-specific files buildable with cross-compilation.

**Step 6: Wire production dependencies**

Pass current build info, executable lookup, HTTP client, and platform replacement from `cmd/deployctl/main.go`. Ensure `deployctl upgrade` still executes only application upgrade logic.

**Step 7: Run tests and cross-build**

```bash
go test ./internal/deployctl -run 'Test(ParseSelfUpdate|SelfUpdate|RunSelfUpdate)' -count=1
GOOS=windows GOARCH=amd64 go test ./internal/deployctl -run '^$'
GOOS=linux GOARCH=arm64 go test ./internal/deployctl -run '^$'
```

Expected: focused tests PASS and both target packages compile.

**Step 8: Commit**

```bash
git add cmd/deployctl/main.go internal/deployctl/
git commit -m "feat(deployctl): add verified self update"
```

### Task 4: Make bootstrap installers persistent and release-tolerant

**Files:**
- Modify: `scripts/install.sh`
- Modify: `scripts/install.ps1`
- Modify: `internal/deployctl/wrappers_test.go`
- Create: `scripts/test/install-wrapper-contract.sh`
- Modify: `scripts/workflow/verify.sh`

**Step 1: Write failing installer contract tests**

Build a temporary fake toolchain (`curl`, `make`, `go`, hash utility) and assert for the POSIX installer:

- successful download verifies and installs into `DEPLOYCTL_INSTALL_DIR`
- artifact 404 invokes `make deployctl` only from a complete source checkout
- network failure produces the same announced fallback
- checksum mismatch never invokes Make and preserves an existing tool
- missing `go.mod`, `Makefile`, or `cmd/deployctl` lists the missing source prerequisite
- missing Go or Make lists the tool prerequisite
- a successful replacement is executable and receives all original arguments

Update the Go source contract to require equivalent PowerShell branches and forbid unsafe evaluation.

**Step 2: Run tests and confirm failure**

```bash
go test ./internal/deployctl -run TestBootstrapWrappers -count=1
./scripts/test/install-wrapper-contract.sh
```

Expected: FAIL because installers neither persist nor fall back locally.

**Step 3: Refactor the POSIX installer**

Implement small functions for platform resolution, download, verification, source-root validation, local build, and atomic installation. Preserve `DEPLOYCTL_BIN` and PATH precedence. Add `DEPLOYCTL_INSTALL_DIR`, announce fallback causes, and print PATH guidance without editing shell profiles.

Treat checksum mismatch as a distinct exit path that can never call local build.

**Step 4: Refactor the PowerShell installer with matching semantics**

Use `Join-Path`, `Get-FileHash`, a same-directory staging file, and `Move-Item` only after successful verification/build. Do not use `Invoke-Expression`. Source fallback requires both `go` and `make`, as approved.

**Step 5: Run contracts**

```bash
go test ./internal/deployctl -run TestBootstrapWrappers -count=1
./scripts/test/install-wrapper-contract.sh
```

If `pwsh` is installed, also parse and execute the PowerShell failure-path fixtures; otherwise the source contract remains mandatory and the skipped runtime check is reported.

Expected: PASS.

**Step 6: Commit**

```bash
git add scripts/install.sh scripts/install.ps1 scripts/test/install-wrapper-contract.sh scripts/workflow/verify.sh internal/deployctl/wrappers_test.go
git commit -m "fix(install): fall back to local deployctl build"
```

### Task 5: Publish installable tagged release artifacts

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `scripts/devops/package-deployctl.sh`
- Create: `scripts/devops/release-contract-test.sh`
- Modify: `scripts/devops/package.sh`
- Modify: `scripts/workflow/native-package-contract.sh`
- Modify: `scripts/workflow/verify.sh`
- Modify: `deployments/devops/README.md`

**Step 1: Write the failing release contract**

Assert that the workflow:

- triggers only on `v*` tags or explicit workflow dispatch for the same ref
- grants only required permissions
- builds deployctl for Linux, Darwin, and Windows on amd64/arm64
- names files exactly `deployctl-<os>-<arch>[.exe]`
- uploads adjacent `.sha256` files
- builds Linux/Windows native packages on amd64/arm64
- skips existing release assets rather than clobbering them
- runs tests before release upload

**Step 2: Run the contract and confirm failure**

```bash
./scripts/devops/release-contract-test.sh
```

Expected: FAIL because the release workflow is absent.

**Step 3: Add a deployctl packaging script**

Make `package-deployctl.sh` call the canonical Make target with explicit GOOS/GOARCH, extension, output directory, version, commit, and build time. Generate a portable checksum file containing only the artifact basename.

**Step 4: Add the tag release workflow**

Use pinned major versions of checkout/setup-go/setup-node. Build/test before packaging. Collect artifacts in a release job, create the Release when absent, query existing asset names, and upload only missing files with `gh release upload` without `--clobber`.

Reuse `scripts/devops/package.sh native` for native bundles and inject the same application version metadata. Do not add Docker credentials or an image push.

**Step 5: Run release and package contracts**

```bash
./scripts/devops/release-contract-test.sh
./scripts/workflow/native-package-contract.sh
```

Expected: PASS. Run `actionlint` when available and report whether it was unavailable.

**Step 6: Commit**

```bash
git add .github/workflows/release.yml scripts/devops/package-deployctl.sh scripts/devops/release-contract-test.sh scripts/devops/package.sh scripts/workflow/native-package-contract.sh scripts/workflow/verify.sh deployments/devops/README.md
git commit -m "ci(release): publish deployctl and native artifacts"
```

### Task 6: Remove the obsolete production deployment implementations

**Files:**
- Delete: `scripts/local/pgctl.sh`
- Delete: `scripts/local/pgctl_contract_test.sh`
- Delete: `scripts/service/install.sh`
- Delete: `scripts/service/uninstall.sh`
- Delete: `scripts/service/manage.sh`
- Delete: `scripts/service/install.ps1`
- Delete: `scripts/service/uninstall.ps1`
- Delete: `scripts/service/manage.ps1`
- Delete: `scripts/service/service_config_contract_test.sh`
- Modify: `Makefile`
- Modify: `scripts/workflow/verify.sh`
- Modify: `web/shared/native-deployment.contract.ts`
- Modify: `web/shared/runtime-env-loading.contract.ts`

**Step 1: Update contracts to describe deployctl ownership**

Replace assertions against legacy service managers with assertions that `internal/deployctl/native.go` owns Linux systemd and Windows SCM behavior, and that installers delegate all production actions to deployctl. Remove the README assertion that mandates the deleted PowerShell service command while retaining the runtime-env and no-default-credential contracts.

**Step 2: Run contracts before deletion**

```bash
./scripts/workflow/verify-contracts.sh
```

Expected: FAIL until the legacy files and references are removed.

**Step 3: Delete legacy scripts and Make targets**

Remove only the files listed above. Retain `make dev`, `make worker`, frontend development, tests, and Docker development compose targets. Remove legacy tests from `verify.sh`.

**Step 4: Prove no stale code references remain**

Run:

```bash
rg -n 'pgctl|scripts/service/manage|service-install|service-uninstall|local-build|local-up' README.md README.zh-CN.md Makefile scripts web deployments docs
```

Expected: no user-facing or executable references, excluding the approved design/history documents when appropriate.

**Step 5: Run focused contracts**

```bash
./scripts/workflow/verify-contracts.sh
go test ./internal/deployctl -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
git add -A scripts/local scripts/service Makefile scripts/workflow/verify.sh web/shared/native-deployment.contract.ts web/shared/runtime-env-loading.contract.ts
git commit -m "refactor(deploy): remove legacy service entrypoints"
```

### Task 7: Rewrite installation and upgrade documentation

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `deployments/devops/README.md`

**Step 1: Add a documentation contract before rewriting**

Extend an existing deployment contract or add `scripts/workflow/deployment-docs-contract.sh` to assert both READMEs:

- lead with Docker full/single as the recommended production quick start
- name deployctl as the only production entrypoint
- document release-to-source fallback and its prerequisites
- document every installer variable including `DEPLOYCTL_INSTALL_DIR`
- contrast `self-update` with application `upgrade`
- contain a clearly non-production developer workflow
- contain no legacy script or Make target

Run it and expect FAIL against current README content.

**Step 2: Rewrite the English README deployment flow**

Remove source deployment and operating-system service installation sections. Reorder the production guide to quick start, mode selection, Setup, operations, version/update, uninstall/migration/troubleshooting. Keep concise local developer commands in a final dedicated section.

**Step 3: Mirror the Chinese README**

Keep headings, examples, parameter tables, warnings, and command distinctions aligned with English. Preserve accurate bilingual runtime-env field guidance already present.

**Step 4: Narrow the devops README**

Document package inputs, artifact names, checksums, tag workflow, and maintainer verification only. Link users back to the top-level README instead of presenting another deployment procedure.

**Step 5: Run documentation contracts**

```bash
./scripts/workflow/deployment-docs-contract.sh
./scripts/workflow/verify-contracts.sh
```

Expected: PASS.

**Step 6: Commit**

```bash
git add README.md README.zh-CN.md deployments/devops/README.md scripts/workflow/deployment-docs-contract.sh scripts/workflow/verify.sh
git commit -m "docs(deploy): consolidate production installation guide"
```

### Task 8: Full verification, review, and pull request

**Files:**
- Modify only when fixing verified defects from this task.
- Generate: `.review/gate.json`

**Step 1: Run formatting and targeted tests**

```bash
gofmt -w cmd/deployctl internal/deployctl
go test ./internal/deployctl -count=1
go vet ./cmd/deployctl ./internal/deployctl
./scripts/test/install-wrapper-contract.sh
./scripts/devops/release-contract-test.sh
./scripts/workflow/deployment-docs-contract.sh
```

Expected: PASS.

**Step 2: Run repository verification**

```bash
./scripts/workflow/verify.sh
```

Expected: all Go tests/vet, frontend typechecks/builds, shared contracts, installer/release contracts, and native package contracts PASS.

**Step 3: Run isolated deployment/API smoke**

```bash
./scripts/workflow/api-smoke.sh
```

Expected: isolated PostgreSQL, Redis, API, Worker, fake provider, Setup, and readiness checks PASS and clean up their temporary resources.

**Step 4: Inspect the committed scope**

Run:

```bash
git diff --check origin/main...HEAD
git status --short
git log --oneline origin/main..HEAD
```

Expected: no whitespace errors, only intended task files, and no secrets or generated binaries.

**Step 5: Run the committed-scope local review gate**

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

Expected: `.review/gate.json` is PASS and matches the current HEAD tree.

If review finds an issue, add a regression test, fix it, recommit, rerun targeted tests, full verify, API smoke, and regenerate the review marker.

**Step 6: Run the ship guard**

Use the `dev-ship` skill or its equivalent repository scripts. Record explicit approval already given in the task context for heavyweight deployment changes.

Expected: all pre-push requirements PASS.

**Step 7: Push and create a ready PR**

Push `codex/deployctl-install-version`, then create a non-draft PR to `main` summarizing behavior, removals, security boundaries, and verification evidence.

Expected: remote branch exists and the PR targets the latest `main` without merge conflicts.
