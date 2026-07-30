# Release Packaging

This directory contains templates and launchers used to assemble native application releases. It is maintainer documentation, not an alternative production installation guide. Operators should follow the top-level `README.md` or `README.zh-CN.md` and use mgsctl.

## Tagged Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`. The workflow first tests mgsctl and both bootstrap installers, then publishes:

```text
mgsctl-linux-amd64
mgsctl-linux-arm64
mgsctl-darwin-amd64
mgsctl-darwin-arm64
mgsctl-windows-amd64.exe
mgsctl-windows-arm64.exe
pic-gallery-native-linux-amd64.tar.gz
pic-gallery-native-linux-arm64.tar.gz
pic-gallery-native-windows-amd64.tar.gz
pic-gallery-native-windows-arm64.tar.gz
```

Every artifact has an adjacent `.sha256` file. The workflow creates a missing Release and uploads only missing asset names. It never overwrites an existing asset, so correcting a published binary requires a new version tag.

`workflow_dispatch` is available for retrying a tag workflow, but the selected ref must still be a `v*` tag. Ordinary branch pushes never create a Release.

## Package MGSCTL Locally

The mgsctl packager uses the same Make target and linker metadata as the tagged workflow:

```bash
RELEASE_TARGET_ROOT=./target/release \
RELEASE_GOOS=linux \
RELEASE_GOARCH=amd64 \
RELEASE_VERSION=v1.2.3 \
RELEASE_COMMIT="$(git rev-parse HEAD)" \
RELEASE_BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
./scripts/devops/package-mgsctl.sh
```

Supported mgsctl targets are Linux, macOS, and Windows on `amd64` or `arm64`. Windows output includes the `.exe` suffix. Inspect a local result with:

```bash
./target/release/mgsctl-linux-amd64 version --json
sha256sum -c ./target/release/mgsctl-linux-amd64.sha256
```

Use `shasum -a 256 -c` on systems without `sha256sum`.

## Package Native Applications Locally

Native bundles contain API, Worker, Gateway, all three Web applications, and the OpenAPI document. Windows bundles also contain the SCM-aware service host.

```bash
DEVOPS_TARGET_ROOT=./target/release \
DEVOPS_GOOS=linux \
DEVOPS_GOARCH=amd64 \
DEVOPS_CGO_ENABLED=0 \
./scripts/devops/package.sh native
```

The output names must remain aligned with `internal/mgsctl/native_release.go`:

```text
pic-gallery-native-<os>-<arch>.tar.gz
pic-gallery-native-<os>-<arch>.tar.gz.sha256
```

The portable archive contains only these top-level paths:

```text
bin/
web/
api/
```

Run the native package contract after changing package contents, frontend build paths, service binaries, or checksums:

```bash
./scripts/workflow/native-package-contract.sh
```

## Component Packages

`scripts/devops/package.sh` also supports `user-web`, `admin-web`, `docs-web`, `api-server`, `worker`, `gateway`, and `all`. These targets are inputs for release engineering and diagnostics. They are not supported production installation entrypoints; mgsctl owns installation, runtime configuration, service registration, health checks, upgrades, and uninstall.

The backend package launchers read `./config/runtime.env` by default and accept `APP_ENV_FILE` only as an explicit override. Frontend launchers render `dist/env.js` from their packaged environment templates.

## Docker Images

Docker image publication remains separate from the GitHub Release workflow because it requires registry-specific credentials:

```bash
./scripts/docker/images.sh build --tag v1.2.3 --registry docker.io/your-org
./scripts/docker/images.sh push --tag v1.2.3 --registry docker.io/your-org
```

Do not add registry credentials to the tagged artifact workflow. Production mgsctl commands should reference an immutable image tag or digest that already exists in the selected registry.
