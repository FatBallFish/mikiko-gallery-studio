# Pic Gallery

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![OpenAPI](https://img.shields.io/badge/OpenAPI-3.x-6BA539?logo=openapiinitiative&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![License](https://img.shields.io/badge/license-not%20specified-lightgrey)

English | [简体中文](./README.zh-CN.md)

Pic Gallery is a self-hostable AI image generation platform. It sits between an API proxy and a full product application: it connects to upstream image model providers such as OpenAI and OpenRouter, then adds user accounts, wallet billing, model routing, reference assets, public gallery discovery, a cashier, admin operations, and OpenAPI documentation.

## Overview

Pic Gallery is designed for teams that want to operate an image generation product rather than only expose raw model APIs.

It includes:

- A user-facing web app for prompt-based image generation, reference-image workflows, private history, public gallery discovery, recharge, and developer API keys.
- An admin console for user operations, model routing, pricing, public gallery review, call records, cashier operations, configuration, audit logs, and system readiness.
- A Go API service and worker that manage authentication, task queues, billing ledgers, provider calls, payment orders, webhook processing, and observability.
- Native Open API and OpenAI-compatible image endpoints for external developers.

## Features

### Image Generation

- Text-to-image, image edit, and reference-image generation flows.
- Async image task queue with API and worker entrypoints.
- Reference asset upload, download, and storage abstraction.
- Model capability discovery, quality selection, aspect-ratio selection, and image-count limits.
- Provider adapters for OpenAI and OpenRouter.
- Route models, provider models, model accounts, route candidates, visibility rules, and pricing.

### User Product

- Email-code login, password login, session refresh, logout, password change, and password reset.
- Signup trial credits with configurable amount, validity days, and expiry reminders.
- Wallet buckets for trial, subscription, recharge, gift, and frozen balances.
- Balance, ledger, estimate, recharge, and profile pages.
- Private gallery with generated image history and publish requests.
- Public gallery that supports guest browsing, logged-in detail view, full prompt access, likes, favorites, and same-prompt generation.
- Developer API key lifecycle and API documentation page.

### Cashier and Billing

- Fixed point packages and custom recharge amounts.
- Recharge credits enter the recharge balance bucket and do not expire by default.
- Payment order creation, cancellation, query, mock payment, webhook ingestion, fulfillment, refund, chargeback, manual completion, and channel sync.
- Payment provider instances with encrypted merchant configuration.
- Built-in adapter contracts for Alipay, WeChat Pay, EasyPay, JeePay, and Mock payment.
- Mock payment is intended for local and test environments.

### Admin Console

- Admin login and permission facade with `super_admin` and `admin` roles.
- Readiness dashboard for launch-critical configuration checks.
- User list, user detail, status management, password reset, group assignment, and point adjustment.
- User group management and billing multipliers.
- Redeem code creation, batch creation, export, status management, and redemption records.
- Gallery review queue with approve, reject, and unpublish actions.
- Call records, audit logs, health/config pages, model routing, provider models, pricing, and cashier operations.

### APIs and Docs

- Agent APIs under `/api/agent/*`.
- Public Open APIs under `/api/open/image/v1/*`.
- OpenAI-compatible endpoints:
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
  - `GET /v1/models`
- Runtime API documentation endpoints:
  - `GET /docs/openapi.json`
  - `GET /docs/examples`
  - `GET /docs/errors`
- OpenAPI source file: [`api/openapi/openapi.yaml`](./api/openapi/openapi.yaml).

## Tech Stack

- Backend: Go `net/http`, Ent, PostgreSQL, Redis, JWT, decimal arithmetic.
- Frontend: React 19, TypeScript, Vite.
- Storage: local filesystem by default, with an abstraction for S3-compatible storage.
- Operations: Docker Compose, Nginx, Prometheus config, health checks, smoke scripts, and Docker E2E scripts.

## Production Quick Start

`deployctl` is the only supported production installation and lifecycle entrypoint. For a new single-host instance, use Docker `full/single`: it includes API, Worker, all Web applications, Gateway, PostgreSQL, Redis, and MinIO.

Install Docker Engine with Compose v2, clone the repository, and run:

```bash
git clone https://github.com/fatballfish/pic-gallery.git
cd pic-gallery
./scripts/install.sh install \
  --mode docker \
  --profile full \
  --topology single \
  --runtime-dir ./runtime \
  --application-version v1.2.3 \
  --image-tag v1.2.3 \
  --yes
```

The installer downloads and verifies the matching deployctl Release artifact. If that artifact is unavailable, a complete source checkout automatically falls back to `make deployctl`; this fallback requires Go and Make. A checksum mismatch is a hard failure and never falls back to another binary source.

After services start, open `http://<api-host>:8080/setup`, obtain the one-time token with `deployctl setup token show --runtime-dir ./runtime`, and complete middleware connectivity plus first-administrator initialization. See [Production Deployment](#production-deployment) for other modes, parameters, clustering, upgrades, and recovery.

## Developer Local Workflow

The following commands are for local development and contribution only. They are not production installation alternatives.

### Prerequisites

- Go 1.26 as declared in [`go.mod`](./go.mod).
- Node.js 20+; Node.js 22 LTS is recommended for the Vite apps.
- npm.
- Docker and Docker Compose.

### 1. Clone the repository

```bash
git clone https://github.com/fatballfish/pic-gallery.git
cd pic-gallery
```

### 2. Prepare the portable runtime file

```bash
mkdir -p config
cp config/runtime.env.example config/runtime.env
```

API and worker read `./config/runtime.env` relative to their working directory. Every field in the template has Chinese and English operational guidance. Compose interpolation defaults remain in `deployments/docker-compose/.env.example`.

### 3. Start the development full stack

```bash
make compose-up
```

The default development Compose file starts the full application stack: PostgreSQL, Redis, MinIO, Mailpit, API, worker, user web, admin web, and Nginx. Only Nginx is exposed to the host.

Default entrypoint:

- User web: `http://127.0.0.1:8088/`
- Admin web: `http://127.0.0.1:8088/admin/`
- API and docs: proxied through `/api/*`, `/docs/*`, `/v1/*`, `/healthz`, and `/readyz`

Initial administrator setup:

1. Query `GET /api/system/v1/bootstrap-status` and open the returned `setup_url`.
2. Use the setup token printed by the deployment tool, or recover it with `deployctl setup token show` before initialization completes.
3. Choose the initial administrator email and password in setup. That account is created as `super_admin`; no default administrator credentials are preset.

Complete setup-managed runtime values in `config/runtime.env`, and override Compose-only values in `deployments/docker-compose/.env.example`, before exposing the service outside a local development machine.

Stop it with:

```bash
make compose-down
```

If you previously started the development database with older settings and see a PostgreSQL error such as `no pg_hba.conf entry` or `role "pic_gallery" does not exist`, reset the local development volumes and start again:

```bash
make compose-clean
make compose-up
```

`make compose-clean` deletes only this project's local Docker Compose development volumes.

If you only want middleware for host-run source development, use the middleware-only Compose file:

```bash
make compose-middleware-up
```

The middleware-only stack exposes these ports to the host:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO API: `localhost:9000`
- MinIO Console: `localhost:9001`
- Mailpit UI: `localhost:8025`

### 4. Start the API and worker

Skip this step when using the default full-stack Compose mode. If you started only middleware with `make compose-middleware-up`, open two terminals:

```bash
make dev
```

```bash
make worker
```

The API listens on `http://127.0.0.1:8080` by default.

Useful health endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### 5. Start the web apps

Install dependencies:

```bash
make user-web-install
make admin-web-install
```

Run the user web app:

```bash
make user-web-dev
```

Run the admin web app:

```bash
make admin-web-dev
```

Default local URLs:

- User web: `http://127.0.0.1:5173`
- Admin web: `http://127.0.0.1:5174`

The Vite development servers proxy `/api` and `/docs` to `http://127.0.0.1:8080` by default. You can override the backend target:

```bash
VITE_API_PROXY_TARGET=http://127.0.0.1:8080 make user-web-dev
VITE_API_PROXY_TARGET=http://127.0.0.1:8080 make admin-web-dev
```

## Configuration

Runtime configuration now uses env files for deployment bootstrap and database-backed admin settings for business configuration.

Main templates:

- [`config/runtime.env.example`](./config/runtime.env.example): canonical bilingual API/worker runtime template.
- [`.env.example`](./.env.example): migration notice for the retired root `.env` layout.
- [`deployments/docker-compose/.env.example`](./deployments/docker-compose/.env.example): local Compose environment template.
- [`deployments/docker-compose/.env.prod.example`](./deployments/docker-compose/.env.prod.example): production Compose environment template.

Backend runtime configuration:

- API and worker load `./config/runtime.env` by default. `APP_ENV_FILE` is the only optional path override.
- Process variables with the same names do not override setup-managed file values. `LoadYAML` remains an explicit migration/test API and does not participate in normal startup.
- Keep only bootstrap settings in the runtime file: deployment metadata, database, Redis, storage bootstrap, auth/encryption secrets, and basic service ports. The first administrator password is never persisted there.
- Configure SMTP, payment channels, billing/pricing, provider accounts, model routing, and other operational settings in the admin console after startup.

Before public launch, configure at least one enabled model provider/account/route/price and one usable payment channel if recharge is exposed.

Admin-managed sensitive settings are write-only by contract:

- SMTP delivery is configured in the admin console at `/admin/#/security-config`. The password is encrypted on the server; read APIs return only secret status metadata.
- Payment provider instances are configured in the admin cashier page. Merchant secrets should be submitted through the secret fields and are preserved on update unless rotated or explicitly cleared.
- Run the admin console behind HTTPS/TLS before entering production merchant or SMTP credentials.

## Production Deployment

`deployctl` is the only supported production deployment entrypoint. It creates a portable runtime directory, generates application secrets, renders Docker or native service assets, and maintains one bilingual `config/runtime.env`. The project accepts HTTP and IP-plus-port access. DNS, TLS, reverse proxies, and external load balancers remain the operator's responsibility.

### Choose a Deployment Mode

| Mode | Profile and topology | Components | Middleware | Typical use |
| --- | --- | --- | --- | --- |
| Docker | `full` / `single` only | API, Worker, user/admin/docs Web, Gateway | Managed PostgreSQL, Redis, MinIO | New single-host installation with the fewest prerequisites |
| Docker | `core` / `single` | API, Worker, user/admin/docs Web, Gateway | External PostgreSQL, Redis, object storage | Existing infrastructure or independently managed middleware |
| Docker | `core` / `cluster` | Control node first; API/Worker/Web nodes join later | Shared external PostgreSQL, Redis, S3-compatible storage | Horizontal API and Worker scaling |
| Docker | `custom` / `single` or `cluster` | Explicit component list | Selected middleware on single-node Docker only | Split Web/API/Worker or monitoring layouts |
| Native Linux/Windows | `core` or `custom` / `single` or `cluster` | Prebuilt API, Worker, portable Gateway and Web assets | External middleware only | Hosts where containers are unavailable or undesired |

Important constraints:

- `full` supports only Docker `single` with role `single`; native `full` and clustered `full` are rejected.
- Cluster control, API, and Worker nodes require shared S3-compatible storage. Node-local storage is not a valid cluster backend.
- Cluster deployments never create node-local PostgreSQL, Redis, or MinIO. Prepare those services before running Setup.
- Multiple API nodes require an existing load balancer or reverse proxy. Use `/healthz` for liveness and `/readyz` for traffic readiness.
- Native targets download checksum-verified release bundles and do not need Go or Node.js installed.

### Prerequisites

Docker installation requires Docker Engine, Compose v2, registry access, free host ports, and a writable runtime directory. `full` needs no separately prepared middleware. `core` and clustered installations need reachable PostgreSQL, Redis, and object storage.

Native installation supports Linux and Windows release bundles on `amd64` or `arm64`. It requires service-manager privileges and external PostgreSQL/Redis. Use local storage only for a single node; use S3-compatible storage whenever API or Worker runs on multiple nodes.

Back up any existing database and object storage before importing configuration or upgrading. Do not put unrelated files in the deployctl runtime directory because destructive uninstall deliberately rejects unmanaged paths.

### Installer Wrapper

On Linux and macOS, use `scripts/install.sh`; on Windows, use `scripts/install.ps1`. The wrapper uses `DEPLOYCTL_BIN` or a `deployctl` already on `PATH`. Otherwise it downloads the matching Release artifact, verifies SHA-256, installs deployctl persistently, and executes the requested command through that absolute path.

The default installed paths are `$HOME/.local/bin/deployctl` on Linux/macOS and `%LOCALAPPDATA%\Programs\deployctl\deployctl.exe` on Windows. The installer prints the actual location and PATH guidance.

If the Release artifact or checksum file is unavailable, the wrapper falls back to `make deployctl` only when it is running from a complete checkout containing `go.mod`, `Makefile`, and `cmd/deployctl`, with Go and Make available. It never downloads a second source archive. A checksum mismatch, including a mismatch against `DEPLOYCTL_SHA256`, is a hard failure: the previous deployctl remains intact and local fallback is forbidden.

| Wrapper variable | Purpose |
| --- | --- |
| `DEPLOYCTL_BIN` | Use a specific local deployctl binary; useful for offline or source builds |
| `DEPLOYCTL_INSTALL_DIR` | Persistent deployctl directory; defaults to the user-local paths above |
| `DEPLOYCTL_VERSION` | Select the deployctl release to download; defaults to `latest` |
| `DEPLOYCTL_RELEASE_BASE_URL` | Override the deployctl and native bundle release repository base URL |
| `DEPLOYCTL_DOWNLOAD_URL` | Override the complete deployctl artifact URL |
| `DEPLOYCTL_SHA256` | Pin the expected checksum instead of downloading the `.sha256` file |

`DEPLOYCTL_VERSION` selects the deployment tool itself. `--application-version`, `--image-tag`, and `--release-version` select the application being installed.

### First Installation

Omit `--yes` to use the interactive selector. Non-interactive installation requires `--mode`, `--profile`, and `--topology`.

Docker full, with versions and the runtime location pinned explicitly:

```bash
./scripts/install.sh install \
  --mode docker \
  --profile full \
  --topology single \
  --runtime-dir ./runtime \
  --application-version v1.2.3 \
  --image-registry docker.io/fatballfish \
  --image-tag v1.2.3 \
  --yes
```

Docker core using existing middleware:

```bash
./scripts/install.sh install \
  --mode docker \
  --profile core \
  --topology single \
  --storage-driver s3 \
  --runtime-dir ./runtime \
  --application-version v1.2.3 \
  --image-tag v1.2.3 \
  --yes
```

Native core on Linux or Windows:

```bash
./scripts/install.sh install \
  --mode native \
  --profile core \
  --topology single \
  --storage-driver local \
  --runtime-dir ./runtime \
  --application-version v1.2.3 \
  --release-version v1.2.3 \
  --yes
```

```powershell
.\scripts\install.ps1 install `
  --mode native `
  --profile core `
  --topology single `
  --runtime-dir .\runtime `
  --application-version v1.2.3 `
  --release-version v1.2.3 `
  --yes
```

Installation writes only under `--runtime-dir`, including:

```text
runtime/
├── config/runtime.env
├── config/install-state.json
├── deployment.json
├── compose.yml                 # Docker
├── assets/                     # generated Docker/Gateway assets
├── bin/, web/, api/            # native release contents
├── data/
└── logs/
```

The exact contents depend on mode and selected components. Preserve `config/runtime.env`, `config/install-state.json`, and `deployment.json` together; they carry the installation identity and recovery state.

### Install Parameters

| Parameter | Values/default | Notes |
| --- | --- | --- |
| `--mode` | `docker`, `native` | Required with `--yes` |
| `--profile` | `full`, `core`, `custom` | Required with `--yes`; component overrides require `custom` |
| `--topology` | `single`, `cluster` | Required with `--yes` |
| `--role` | `single`, `control` | Defaults to `single` for single topology and `control` for cluster; joined roles use `cluster join` |
| `--components` | Comma-separated list | Required for `custom`; valid values are `api`, `worker`, `user-web`, `admin-web`, `docs-web`, `gateway`, `postgres`, `redis`, `minio`, `monitoring` |
| `--runtime-dir` | `.` | Portable directory containing configuration, state, assets, data, and logs |
| `--storage-driver` | `local`, `s3` | Defaults to `s3` for full, cluster, or MinIO custom installs; otherwise `local` |
| `--public-api-url` | Absolute HTTP(S) URL | Records the browser-visible API base URL; a joined Web plan requires it and receives it from Control during `cluster join` |
| `--application-version` | `dev` | Installation compatibility version; pin a release version in production |
| `--image-registry` | Compose default when empty | Docker image prefix; current Compose default is `docker.io/fatballfish` |
| `--image-tag` | Application version | Docker image tag; prefer an immutable release or digest-derived tag |
| `--release-version` | Application version | Native GitHub release containing the platform bundle and checksum |
| `--api-port` | `8080` | Host API port |
| `--gateway-port` | `80` | Used when Gateway is selected |
| `--user-web-port` | `5173` | Used when user Web is selected |
| `--admin-web-port` | `5174` | Used when admin Web is selected |
| `--docs-web-port` | `5175` | Used when docs Web is selected |
| `--monitoring-port` | `9090` | Used when monitoring is selected |
| `--external-gateway` | `false` | Required confirmation when Web components are selected without the managed Gateway |
| `--migrate` | `false` on install | Requests a control/single migration; normal first initialization migrates through Setup |
| `--yes` | `false` | Non-interactive confirmation; never authorizes persistent data deletion |

All ports must be in `1-65535`, duplicate flags are rejected, and explicit component lists are canonicalized. Additional custom-profile rules include:

- Gateway requires local API plus all three Web apps, except a joined Web node where it requires the three Web apps.
- Web components without Gateway require `--external-gateway` and an operator-managed host/proxy.
- Monitoring requires Docker and a local API component.
- Native mode cannot manage middleware or monitoring.
- Cluster custom profiles cannot contain `postgres`, `redis`, or `minio`.
- A single/control authority must include API. Joined API, Worker, and Web roles must be created with `cluster join`, not `install`.

Example Docker custom installation with monitoring:

```bash
./scripts/install.sh install \
  --mode docker \
  --profile custom \
  --topology single \
  --components api,worker,user-web,admin-web,docs-web,gateway,monitoring \
  --monitoring-port 9090 \
  --runtime-dir ./runtime \
  --application-version v1.2.3 \
  --image-tag v1.2.3 \
  --yes
```

### Browser Setup and First Administrator

Before initialization, the API exposes health checks, bootstrap status, and the API-hosted Setup UI only. Open `http://<api-host>:<api-port>/setup`; user and admin Web apps also redirect there while preserving their original return URL.

The one-time Setup token is deliberately not printed during non-interactive install. Read or rotate it on the deployment host:

```bash
deployctl setup status --runtime-dir ./runtime
deployctl setup token show --runtime-dir ./runtime
deployctl setup token reset --runtime-dir ./runtime
```

Use `token reset` when initialization is still pending and the token was exposed, used, or lost. Reset invalidates old tokens and Setup sessions and restarts API and Gateway. Token display and reset are permanently disabled after successful initialization.

In Setup:

1. Confirm the public API URL and allowed browser origins.
2. Configure and test PostgreSQL, Redis, and object storage. Docker `full` connection fields are managed and read-only; `core` fields are editable.
3. Enter the first administrator email and password.
4. Review the configuration, click Apply, and wait for migration and service restart.
5. After the countdown, the browser returns to the original user/admin route.

Do not refresh merely because containers restart during Apply. If recovery times out, run `status`, `doctor`, and `restart`. If `setup status` is still pending, reopen `/setup`; an authenticated pending session resumes the persisted operation instead of starting a second migration. If Setup is already complete, `/setup` remains closed and readiness diagnostics identify the service that must recover.

Verify completion:

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/setup  # expect 404
deployctl doctor --runtime-dir ./runtime
```

After Setup, sign in to the admin console and configure provider accounts, text/image models, routes, prices, plans, registration policy, payments/recharge, and SMTP. These business settings live in the database, not `runtime.env`.

### Cluster Deployment

Start one control node against external shared PostgreSQL, Redis, and S3 storage. The control node owns Setup, migrations, cluster tokens, and configuration revision:

```bash
./scripts/install.sh install \
  --mode docker \
  --profile core \
  --topology cluster \
  --role control \
  --storage-driver s3 \
  --runtime-dir ./control \
  --public-api-url http://10.0.0.10:8080 \
  --application-version v1.2.3 \
  --image-tag v1.2.3 \
  --yes
```

Complete Setup on the control node before joining any other node. Then create a role-scoped credential:

```bash
deployctl cluster token create --role api --ttl 10m --runtime-dir ./control
deployctl cluster token create --role worker --ttl 10m --runtime-dir ./control
deployctl cluster token create --role web --ttl 10m --runtime-dir ./control
```

TTL must be greater than zero and no more than 24 hours. Tokens are encrypted in transit, expire, are role-bound, and can be used only once.

Join each target host with a token for its role:

```bash
deployctl cluster join \
  --server http://10.0.0.10:8080 \
  --token '<single-use-token>' \
  --mode docker \
  --runtime-dir ./node \
  --application-version v1.2.3 \
  --image-tag v1.2.3 \
  --api-port 8080
```

`cluster join` also accepts `--image-registry`, `--release-version`, and Gateway/user/admin/docs port overrides. The joining application version must match the control installation. The control API returns installation identity, secrets, shared middleware configuration, schema version, and configuration revision in an authenticated encrypted envelope; the plaintext join token is not stored.

Put joined API nodes behind your load balancer. Worker nodes consume the shared task queue and database leases. A Web-role node can host the three Web apps and Gateway separately from API. Joined nodes refuse startup when installation identity, application/schema version, configuration revision, or node identity does not match.

### Runtime Configuration

API and Worker normally read `./config/runtime.env` relative to the runtime working directory. `APP_ENV_FILE` is the only supported path override for service managers that cannot set that working directory. `PIC_GALLERY_ENV_FILE` is not supported.

The generated file contains detailed Chinese and English comments. Setup writes all required values before declaring the installation complete. Do not manually change `SETUP_COMPLETED`, installation or cluster IDs, runtime schema version, configuration revision, or generated security keys. Use Setup, `upgrade`, `cluster join`, or the admin console according to field ownership.

### Status, Restart, and Diagnostics

Run operational commands on the deployment host and point them at the same runtime directory:

```bash
deployctl status --runtime-dir ./runtime
deployctl doctor --runtime-dir ./runtime
deployctl restart --runtime-dir ./runtime
```

`doctor` verifies required fields, private file permissions, runtime/manifest/state identity, middleware connectivity, readiness, and schema compatibility while redacting DSNs and secrets. For Docker-specific logs, copy a container name from `deployctl status` and inspect it directly:

```bash
docker logs --tail=200 <api-container-name>
docker logs --tail=200 <worker-container-name>
```

### Tool Version and Manual Update

Deployctl never checks for updates during normal commands. Inspect the installed tool locally:

```bash
deployctl version
deployctl version --json
```

Update the deployctl binary only when an administrator explicitly requests it:

```bash
deployctl self-update
deployctl self-update --version v1.3.0
deployctl self-update --version v1.3.0 --yes
```

Without `--yes`, self-update requires an interactive confirmation. It downloads the current platform artifact and checksum, stages the verified file beside the current executable, and replaces only the deployment tool. It does not restart or upgrade an installed Pic Gallery runtime. If the selected Release does not exist, self-update stops; from a complete source checkout, rerun the installer so its documented local Make fallback can be used.

| Command | Updated object | Network behavior |
| --- | --- | --- |
| `deployctl self-update` | The deployctl executable | Connects to the selected deployctl Release only when explicitly run |
| `deployctl upgrade` | Deployed API, Worker, Web/native assets and optional database migration | Resolves the application image or native release requested by its flags |

### Application Upgrade and Recovery

Use immutable versions and back up PostgreSQL plus object storage before every production application update.

Docker single/control node:

```bash
deployctl upgrade \
  --runtime-dir ./runtime \
  --application-version v1.3.0 \
  --image-registry docker.io/fatballfish \
  --image-tag v1.3.0
```

Native single/control node:

```bash
deployctl upgrade \
  --runtime-dir ./runtime \
  --application-version v1.3.0 \
  --release-version v1.3.0
```

Upgrade the control node first. It acquires the distributed migration lock, updates the runtime and manifest atomically, migrates once, then rolls services in dependency order. Upgrade joined API/Worker/Web nodes afterward without migration:

```bash
deployctl upgrade \
  --runtime-dir ./node \
  --application-version v1.3.0 \
  --image-tag v1.3.0 \
  --migrate=false
```

For a native joined node, replace `--image-tag` with `--release-version v1.3.0` and keep `--migrate=false`.

Upgrade migrations are forward-only. If migration succeeds but service rollout fails, rerun the exact same upgrade command to resume the idempotent rollout. If rollout fails before migration, deployctl restores and reapplies the previous runtime plan. Do not try to downgrade the database by changing image tags; restore from a tested backup only when a release-specific recovery procedure requires it.

### Stop, Uninstall, and Permanent Deletion

Ordinary uninstall stops and unregisters services but preserves runtime configuration and persistent data:

```bash
deployctl uninstall --runtime-dir ./runtime --yes
```

Ordinary uninstall is intended for service removal while retaining files for backup or migration. To permanently delete the managed runtime and, for Docker, its named PostgreSQL/Redis/MinIO volumes, first obtain the installation ID and then type the exact case-sensitive phrase:

```bash
deployctl setup status --runtime-dir ./runtime
deployctl uninstall \
  --runtime-dir ./runtime \
  --delete-data \
  --confirm 'DELETE <installation-id> PERSISTENT DATA'
```

`--yes` can never authorize data deletion. Destructive uninstall validates that the runtime tree contains only deployctl-managed configuration, release assets, application data, and logs before stopping any service or deleting any volume. Back up the database and object storage first.

### Import an Older Configuration

Legacy root `.env`, `.env.prod`, or packaged `backend.env` files are not loaded automatically. Import one explicitly into a new runtime directory:

```bash
deployctl import-config \
  --source .env.prod \
  --mode docker \
  --profile full \
  --topology single \
  --storage-driver s3 \
  --runtime-dir ./runtime
```

Import never modifies the source and refuses to overwrite an existing target. Keep the old file until `doctor`, readiness, administrator login, and a business smoke test pass.

### Build and Publish Docker Images

Operators publishing their own images must publish all five application images under the same registry prefix and tag:

```bash
./scripts/docker/images.sh build --tag v1.3.0 --registry registry.example.com/pic-gallery
./scripts/docker/images.sh push --tag v1.3.0 --registry registry.example.com/pic-gallery
```

Create a release and optional `latest` tag in one step:

```bash
./scripts/docker/images.sh release \
  --version v1.3.0 \
  --latest \
  --registry registry.example.com/pic-gallery
```

See [`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md) for failure recovery, native service behavior, backup boundaries, and deployment acceptance tests.

## Contribution Workflow

For secondary development or contribution work, this repository includes an optional local AI development workflow. It installs Git hooks and shared workflow scripts for requirement/design context checks, verification, local review gates, and pre-push checks.

Install the hooks once after cloning:

```bash
./scripts/workflow/install-hooks.sh
```

Related workflow documentation:

- [`AGENTS.md`](./AGENTS.md)
- [`docs/org/workflow/DEVELOPMENT_WORKFLOW.md`](./docs/org/workflow/DEVELOPMENT_WORKFLOW.md)
- [`docs/org/workflow/REVIEW_GATE.md`](./docs/org/workflow/REVIEW_GATE.md)

## Verification

Run the centralized repository verification:

```bash
./scripts/workflow/verify.sh
```

It runs:

- `go test ./...`
- `go vet ./...`
- shared frontend contract checks
- user web typecheck/build
- admin web typecheck/build

Run the isolated API contract smoke:

```bash
BASE_URL=http://127.0.0.1:8080 ./scripts/workflow/api-smoke.sh
```

Prerequisites are Bash, `curl`, Python 3, Go, a running Docker daemon, and local access to or registry access for the `postgres:16-alpine` and `redis:7-alpine` images. The script starts its own API, Worker, fake provider, PostgreSQL, and Redis processes; `BASE_URL` only accepts `http://127.0.0.1:<port>` or `http://localhost:<port>` with an explicit free port and no path, query, fragment, or user info, and does not target a pre-existing API. Exit cleanup stops the child processes, removes the temporary containers, and deletes the temporary runtime env, storage, logs, and test data.

Run the Docker E2E suite:

```bash
./scripts/e2e/run-docker-e2e.sh --start
```

Stop the shared local stack while preserving its data:

```bash
./scripts/dev/down.sh
```

## Project Structure

```text
api/openapi/              OpenAPI contract and API documentation source
cmd/api/                  API service entrypoint
cmd/worker/               async worker entrypoint
deployments/              Docker Compose, Nginx, and monitoring assets
docs/                     PRD, technical design, plans, reviews, and runbooks
internal/app/             application bootstrap and runtime wiring
internal/config/          config model and loader
internal/domain/          domain models and core rules
internal/http/            handlers, middleware, router, and route tests
internal/provider/        upstream provider adapters
internal/repository/      database, cache, and storage repositories
internal/service/         application service orchestration
internal/worker/          async task execution and compensation jobs
pkg/                      shared utility packages
scripts/                  development, workflow, smoke, and E2E scripts
web/shared/               shared frontend API/client helpers
web/user/                 user web app
web/admin/                admin web app
```

## Documentation

- Product requirements: [`docs/prd/pic-gallery-prd.md`](./docs/prd/pic-gallery-prd.md)
- Technical design: [`docs/tech/pic-gallery-tech-design.md`](./docs/tech/pic-gallery-tech-design.md)
- Product defect closure design: [`docs/plans/2026-06-05-product-defect-closure-technical-design.md`](./docs/plans/2026-06-05-product-defect-closure-technical-design.md)
- Acceptance audit: [`docs/plans/2026-06-07-product-defect-closure-acceptance-audit.md`](./docs/plans/2026-06-07-product-defect-closure-acceptance-audit.md)
- Backend deployment runbook: [`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md)

## Status and Roadmap

Pic Gallery is an active product implementation. The current codebase already contains the main user, admin, billing, cashier, gallery, model-routing, API, worker, and deployment foundations.

Potential open-source hardening work:

- Add an explicit open-source license.
- Add public demo screenshots and a hosted documentation site.
- Add migration/versioning documentation for database changes.
- Add seed data or a guided first-run wizard for model and cashier setup.
- Add more provider adapters and production payment-channel examples.
- Add CI examples for verification, API smoke, and Docker E2E.

## Disclaimer

This project can call third-party AI model providers and payment channels. You are responsible for configuring provider credentials, complying with provider terms, protecting secrets, reviewing generated content, and meeting local payment, tax, privacy, and platform compliance requirements.

## License

No license file is currently included in this repository. Add a license before publishing it as an open-source project.
