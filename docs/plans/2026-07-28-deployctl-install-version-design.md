# Deployctl Installation and Versioning Design

## Context

The repository currently documents three overlapping production deployment paths:

- `scripts/local/pgctl.sh` source builds and service installation
- `scripts/service/manage.sh` and `manage.ps1` operating-system services
- the production `deployctl` workflow

The first two paths predate deployctl and now duplicate service lifecycle behavior already implemented by deployctl. The production installer also assumes GitHub Release artifacts exist, but the repository currently has no releases. A missing artifact therefore makes first-time installation fail with HTTP 404. Finally, deployctl does not expose its own build version or provide a deliberate tool-upgrade flow.

## Goals

- Make deployctl the only supported production deployment entrypoint.
- Remove obsolete source-deployment and service-manager implementations when deployctl already owns the behavior.
- Preserve a clearly labeled local development workflow for contributors.
- Fall back to a local Make build when a release artifact is unavailable and the installer is running in a complete source checkout.
- Expose deployctl build identity and provide an explicit, administrator-initiated tool update.
- Publish the artifacts required by the installer and native deployment from tagged releases.
- Keep English and Chinese deployment documentation structurally aligned.

## Non-goals

- Automatically checking for or installing updates during normal deployctl commands.
- Downloading a source archive as a second fallback.
- Committing prebuilt deployctl binaries to Git.
- Changing the existing application deployment upgrade or database migration semantics.
- Adding a new Docker registry or credentials model.

## Deployment Entry Point and Removal Boundary

Production deployment has one supported flow:

```text
scripts/install.sh or scripts/install.ps1
                    |
                deployctl
                    |
       Docker full/core/custom or native core/custom
```

Remove the legacy entrypoints and code that exists only to support them:

- `scripts/local/pgctl.sh` and its contract test
- `scripts/service/manage.sh` and `manage.ps1`
- the service `install.*` and `uninstall.*` wrappers
- contract tests dedicated to those scripts
- the Make `local-build`, `local-up`, and `service-*` targets
- verification and documentation references to those targets and scripts

Retain deployctl's independent systemd, Windows SCM, Docker Compose, health-check, restart, and uninstall implementations. Retain runtime environment loading and tests that remain part of deployctl or application startup; migrate useful contracts away from deleted script paths. Inspect `deployments/devops` by reference and retain native release launchers, templates, and proxy configuration that are still packaged by deployctl.

Developer commands such as `make dev`, `make worker`, frontend development servers, and repository verification remain available under a section explicitly labeled for local development, not production deployment.

## Installer Behavior

The shell and PowerShell installers use the same resolution order:

1. Execute `DEPLOYCTL_BIN` when explicitly provided.
2. Execute an existing deployctl found on `PATH`.
3. Download the requested platform artifact and checksum from GitHub Releases.
4. If a verifiable release pair is unavailable, build deployctl from the current source checkout.

A downloaded or locally built deployctl is persisted instead of being executed only from a temporary directory:

- Unix default: `$HOME/.local/bin/deployctl`
- Windows default: `%LOCALAPPDATA%\Programs\deployctl\deployctl.exe`
- Override: `DEPLOYCTL_INSTALL_DIR`

The installer prints the final binary path and a PATH instruction when necessary. It executes the requested command using the absolute installed path so that the current shell does not need to reload PATH.

### Download and Verification

Downloads occur in a temporary directory. The installer obtains the platform binary and either the published `.sha256` file or the explicitly supplied `DEPLOYCTL_SHA256`. The verified file is copied into the destination directory through a same-directory temporary file and atomic rename.

A checksum mismatch is a hard security failure. The installer must not fall back to source and must not modify an existing installed binary. An explicitly supplied checksum mismatch follows the same rule.

Release 404 responses, network unavailability, or an incomplete release pair may use the local build fallback. Diagnostics must state why the release could not be used and that the installer is switching to a local source build.

### Local Build Fallback

The local fallback is allowed only when the installer can locate a repository root containing at least `go.mod`, `Makefile`, and `cmd/deployctl`. Both Go and Make must be available. A new canonical `make deployctl` target accepts an output path and injects build metadata.

The build writes to a temporary path and replaces the installed binary only after a successful build. Missing source files or tools produce an itemized error and explain that `DEPLOYCTL_BIN` can point to a trusted prebuilt binary. The installer never downloads another source archive and never stores a binary in Git.

## Build Identity

Deployctl receives four build values:

- `Version`: release tag or local development version
- `Commit`: source commit SHA
- `BuildTime`: UTC build timestamp
- `Dirty`: whether a local source build included uncommitted changes

The values are injected by the canonical Make target and release workflow. A source build identifies itself as a development build and includes commit and dirty state; it must not present itself as an official release.

Deployctl supports:

```text
deployctl version
deployctl version --json
```

Text output is intended for operators. JSON output is stable machine-readable data for inventory and diagnostics. Neither form performs a network request.

## Manual Tool Update

The existing `deployctl upgrade` command upgrades the deployed application, including images, native releases, and optional database migrations. Its meaning remains unchanged.

The deployctl binary is updated explicitly with:

```text
deployctl self-update
deployctl self-update --version v1.2.3
deployctl self-update --version v1.2.3 --yes
```

Self-update displays the current and target versions, downloads the matching platform artifact and checksum, verifies them, and replaces only the running tool. It does not update an application runtime. Normal deployctl commands never check for updates.

Unix uses a same-directory temporary file followed by atomic rename. Windows stages a `.new` executable and starts a small replacement helper that waits for the original process to exit before replacing it. Failure preserves the old binary and reports the staged path and an administrator-safe manual replacement command.

If the requested Release does not exist, self-update stops with an actionable message. It does not silently switch provenance. Operators who have a source checkout can rerun the installer to use the approved local build fallback.

## Release Publication

A tag-triggered GitHub Actions workflow builds deployctl for Linux, macOS, and Windows on amd64 and arm64. Each artifact has an adjacent `.sha256` file and uses the exact name parsed by the installers. The workflow injects tag, commit, and build time metadata and runs deployctl and installer contract tests before publishing.

The same tagged Release includes the existing native application bundles and their checksums. If a Release already exists, automation may add missing artifacts but must not overwrite an existing artifact with different contents. The workflow does not create tags and does not publish from ordinary branch pushes.

Docker images remain under the repository's existing image publication mechanism. This change does not introduce new registry credentials; it only keeps application-version parameters explicit where relevant.

## Documentation Structure

`README.md` and `README.zh-CN.md` use matching sections:

1. Project overview and features
2. Production deployment quick start, leading with Docker full/single
3. Deployment mode selection
4. Setup initialization
5. Routine operations
6. Version and upgrades
7. Uninstall, migration, and troubleshooting
8. Developer local workflow

The version section contrasts `deployctl self-update` for the tool with `deployctl upgrade` for the deployed application. Installer variables include the install directory, tool version, release base, complete download URL, checksum override, and local fallback prerequisites.

`deployments/devops/README.md` becomes release-maintainer documentation and does not duplicate end-user deployment instructions.

## Error Handling

Errors must distinguish these cases:

- unavailable release or network, followed by an announced source fallback
- incomplete source checkout, with missing files listed
- missing Go or Make, with prerequisite and `DEPLOYCTL_BIN` guidance
- checksum mismatch, with fallback forbidden
- unwritable install directory, without replacing the old tool
- failed Windows delayed replacement, with staged-file recovery instructions
- tool update versus application upgrade, explained in command help

All temporary downloads and builds are cleaned up unless a Windows recovery file must be retained intentionally.

## Verification

Tests cover:

- command parsing and text/JSON build information
- release metadata injection through the Make target
- self-update download, confirmation, checksum verification, replacement, and rollback
- Release 404 or network failure triggering a local Make build
- checksum mismatch preventing fallback
- missing source, Go, and Make diagnostics
- aligned shell and PowerShell installer contracts
- release artifact names matching installer platform resolution
- absence of stale README, Make, workflow, and frontend references to removed scripts
- repository verification, isolated API smoke, and the committed-scope local review gate

No test contacts the live GitHub Release service. Download and update tests use controlled local servers or injected dependencies.

## Delivery

Implementation starts from the latest `origin/main` on a `codex/` feature branch. The design and implementation plan are committed before code changes. The feature is complete only after implementation, full repository verification, deployment API smoke, committed-scope review, and a ready pull request to `main`.
