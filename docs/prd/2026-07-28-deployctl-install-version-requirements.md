# Deployctl Installation and Versioning Requirements

## Problem

The repository exposes several overlapping installation and deployment systems. README users can choose between source deployment with `pgctl.sh`, operating-system service installation with `manage.sh` or `manage.ps1`, and the newer production deployctl workflow. The competing paths increase operator confusion and failure risk.

The production bootstrap scripts currently download deployctl from GitHub Releases, but the repository has no published releases. A new installation therefore fails with an HTTP 404 instead of continuing from the available source checkout.

Deployctl also has no visible tool version and no explicit operator-controlled way to update the deployctl binary.

## Requirements

1. Deployctl must be the only supported production deployment entrypoint.
2. README source-deployment and legacy operating-system service installation content must be removed.
3. Legacy source/service implementations must be deleted when deployctl independently provides the same production behavior.
4. A concise developer-only local workflow must remain for source debugging and tests.
5. `install.sh` and `install.ps1` must first try a verified Release artifact and fall back to `make deployctl` when the release is unavailable.
6. Local fallback is allowed only from a complete source checkout with Go and Make installed.
7. The installer must not download a source archive and must not commit or rely on repository-stored binaries.
8. Checksum mismatch is a hard failure and must never trigger source fallback.
9. A successfully downloaded or locally built deployctl must be installed persistently and its location must be shown to the operator.
10. Deployctl must expose human-readable and machine-readable tool build information.
11. Normal deployctl commands must not check for or install updates automatically.
12. Operators must be able to explicitly update the deployctl tool while preserving the existing application `deployctl upgrade` behavior.
13. Tagged releases must publish the exact deployctl and native application artifacts consumed by installation and upgrade flows.
14. English and Chinese documentation must explain one production workflow and clearly distinguish tool self-update from deployed-application upgrade.

## Confirmed Decisions

- Update policy: no automatic checks; administrators explicitly initiate tool updates.
- Local fallback: current complete source checkout only, requiring Go and Make.
- Documentation: retain local developer commands but never describe them as production installation.
- Removal: delete `pgctl.sh`, legacy service managers, wrappers, dedicated tests, Make targets, and stale references after confirming deployctl ownership.
- Tool update command: use `deployctl self-update` because `deployctl upgrade` already updates the deployed application.
- Release publication: tagged releases publish checksummed deployctl artifacts for Linux, macOS, and Windows on amd64/arm64 plus native application bundles.

## Acceptance Criteria

- A missing or unreachable Release produces an explicit fallback message and a successful local Make build in a valid checkout.
- The same failure outside a source checkout lists missing prerequisites and leaves any installed deployctl untouched.
- A checksum mismatch exits non-zero, does not call Make, and preserves the old binary.
- `deployctl version` and `deployctl version --json` report non-empty version, commit, build time, dirty state, and runtime platform information.
- `deployctl self-update` downloads and verifies the selected platform artifact, requires confirmation unless `--yes` is supplied, and preserves the old tool on failure.
- `deployctl upgrade` continues to update only the installed application runtime.
- GitHub Release assets use installer-compatible names and include adjacent checksum files.
- Repository search finds no executable or user-documentation references to the deleted legacy deployment entrypoints.
- Repository verification, deployment API smoke, and committed-scope review gate pass before pull request creation.

## Approved Design

See `docs/plans/2026-07-28-deployctl-install-version-design.md`.
