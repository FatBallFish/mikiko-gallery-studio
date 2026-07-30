# Release Packaging

This directory contains templates and launchers used to assemble native application releases. It is maintainer documentation, not an alternative production installation guide. Operators should follow the top-level `README.md` or `README.zh-CN.md` and use mgsctl.

## Tagged Releases

Pushing a SemVer `v*` tag runs `.github/workflows/release.yml`. The workflow runs full repository verification, builds all release assets, publishes five multi-architecture Docker images, renders `release-manifest.json`, verifies the GitHub Release, and only then promotes the published image digests to `latest`.

```text
mgsctl-linux-amd64
mgsctl-linux-arm64
mgsctl-darwin-amd64
mgsctl-darwin-arm64
mgsctl-windows-amd64.exe
mgsctl-windows-arm64.exe
mikiko-gallery-studio-native-linux-amd64.tar.gz
mikiko-gallery-studio-native-linux-arm64.tar.gz
mikiko-gallery-studio-native-windows-amd64.tar.gz
mikiko-gallery-studio-native-windows-arm64.tar.gz
mikiko-gallery-studio-api-linux-amd64.tar.gz
mikiko-gallery-studio-api-linux-arm64.tar.gz
mikiko-gallery-studio-api-windows-amd64.tar.gz
mikiko-gallery-studio-api-windows-arm64.tar.gz
mikiko-gallery-studio-worker-linux-amd64.tar.gz
mikiko-gallery-studio-worker-linux-arm64.tar.gz
mikiko-gallery-studio-worker-windows-amd64.tar.gz
mikiko-gallery-studio-worker-windows-arm64.tar.gz
mikiko-gallery-studio-user-web.tar.gz
mikiko-gallery-studio-admin-web.tar.gz
mikiko-gallery-studio-docs-web.tar.gz
release-manifest.json
```

Every artifact has an adjacent `.sha256` file. The workflow creates a missing Release and uploads only missing asset names; an existing asset must be byte-identical or publication fails. The Manifest binds the concrete application version to asset checksums and immutable image digests.

Configure the repository secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` for Docker Hub publication under `docker.io/fatballfish`. The five repositories are `mikiko-gallery-studio-api`, `mikiko-gallery-studio-worker`, `mikiko-gallery-studio-user-web`, `mikiko-gallery-studio-admin-web`, and `mikiko-gallery-studio-docs-web`. The API image also contains the `mikiko-gallery-studio-db-migrate` executable; no separate migration image is published.

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
mikiko-gallery-studio-native-<os>-<arch>.tar.gz
mikiko-gallery-studio-native-<os>-<arch>.tar.gz.sha256
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

`scripts/devops/package.sh` supports directory targets plus `api-release`, `worker-release`, `user-web-release`, `admin-web-release`, and `docs-web-release`. Release targets create the named archives above and adjacent checksums. They are release-engineering inputs, not production installation entrypoints; mgsctl owns installation, runtime configuration, service registration, health checks, upgrades, and uninstall.

The backend package launchers read `./config/runtime.env` by default and accept `APP_ENV_FILE` only as an explicit override. Frontend launchers render `dist/env.js` from their packaged environment templates.

## Docker Images

The tag workflow authenticates with the Docker Hub secrets, publishes the SemVer tag for `linux/amd64` and `linux/arm64`, records the resulting digest in `release-manifest.json`, and promotes that digest to `latest` only after Release verification. Local maintainers can exercise the same image names with:

```bash
./scripts/docker/images.sh build --tag v1.2.3 --registry docker.io/fatballfish
./scripts/docker/images.sh push --tag v1.2.3 --registry docker.io/fatballfish
```
