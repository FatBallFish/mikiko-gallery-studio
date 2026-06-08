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

## Quick Start

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

### 2. Prepare local environment variables

```bash
cp .env.example .env
```

For local development, the default `.env.example` is enough to start the infrastructure. Add `OPENAI_API_KEY` or `OPENROUTER_API_KEY` when you want to call real upstream providers.

### 3. Start the development full stack

```bash
make compose-up
```

The default development Compose file starts the full application stack: PostgreSQL, Redis, MinIO, Mailpit, API, worker, user web, admin web, and Nginx. Only Nginx is exposed to the host.

Default entrypoint:

- User web: `http://127.0.0.1:8088/`
- Admin web: `http://127.0.0.1:8088/admin/`
- API and docs: proxied through `/api/*`, `/docs/*`, `/v1/*`, `/healthz`, and `/readyz`

Default development admin login:

- Email: `admin@example.com`
- Password: `admin123456`

Override these with `PIC_GALLERY_ADMIN_EMAIL` and `PIC_GALLERY_ADMIN_PASSWORD` in your Compose env file before exposing the service outside a local development machine.

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

## Source Service Installation

When installing from source, the API, worker, user web app, and admin web app can be registered as operating-system services. This is useful for a self-hosted machine where you want the source-run processes to restart automatically.

The service scripts install all four components by default. You can restrict the target components with `--components api,worker,user-web,admin-web`.

### Linux

Linux uses `systemd`.

Install user-level services:

```bash
./scripts/service/install.sh --user
```

Uninstall user-level services:

```bash
./scripts/service/uninstall.sh --user
```

Install system-level services:

```bash
sudo ./scripts/service/install.sh
```

Uninstall system-level services:

```bash
sudo ./scripts/service/uninstall.sh
```

### macOS

macOS uses `launchd`.

Install user-level services:

```bash
./scripts/service/install.sh --user
```

Uninstall user-level services:

```bash
./scripts/service/uninstall.sh --user
```

Install system-level daemons:

```bash
sudo ./scripts/service/install.sh
```

Uninstall system-level daemons:

```bash
sudo ./scripts/service/uninstall.sh
```

### Windows

Windows source service installation uses Scheduled Tasks so the ordinary Go and Vite foreground processes can be managed without an additional service wrapper.

Install services:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/install.ps1
```

Uninstall services:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/uninstall.ps1
```

Install selected components:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/install.ps1 -Components "api,worker"
```

## Configuration

Main configuration templates:

- [`configs/config.example.yaml`](./configs/config.example.yaml): application config template.
- [`.env.example`](./.env.example): local development environment variables.
- [`deployments/docker-compose/.env.example`](./deployments/docker-compose/.env.example): compose environment template.
- [`deployments/docker-compose/.env.prod.example`](./deployments/docker-compose/.env.prod.example): production compose environment template.

Important environment variables:

- `APP_CONFIG_PATH`: path to the YAML config file.
- `DATABASE_URL`: PostgreSQL connection URL.
- `REDIS_URL`: Redis connection URL.
- `AUTH_ACCESS_TOKEN_SECRET`: access-token signing secret for production.
- `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`: encryption key for API-key signing secrets.
- `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`: encryption key for payment provider merchant secrets.
- `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY` or `SECURE_CONFIG_ENCRYPTION_KEY`: encryption key for admin-managed secure settings such as SMTP passwords.
- `PIC_GALLERY_ADMIN_EMAIL`: initial admin account email. Development Compose defaults to `admin@example.com`; production Compose requires an explicit value.
- `PIC_GALLERY_ADMIN_PASSWORD`: initial admin account password. Development Compose defaults to `admin123456`; production Compose requires an explicit value.
- `OPENAI_API_KEY`: OpenAI provider key.
- `OPENROUTER_API_KEY`: OpenRouter provider key.

Before public launch, configure at least one enabled model provider/account/route/price and one usable payment channel if recharge is exposed.

Admin-managed sensitive settings are write-only by contract:

- SMTP delivery is configured in the admin console at `/admin/#/security-config`. The password is encrypted on the server; read APIs return only secret status metadata.
- Payment provider instances are configured in the admin cashier page. Merchant secrets should be submitted through the secret fields and are preserved on update unless rotated or explicitly cleared.
- Run the admin console behind HTTPS/TLS before entering production merchant or SMTP credentials.

## Docker Compose Deployment

The production compose file is located at [`deployments/docker-compose/docker-compose.prod.yml`](./deployments/docker-compose/docker-compose.prod.yml).

```bash
cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
$EDITOR deployments/docker-compose/.env.prod

docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml up -d --build
```

The production stack includes PostgreSQL, Redis, API, worker, user web, admin web, Nginx, shared storage, and optional Prometheus. PostgreSQL, Redis, API, worker, and frontend containers are on the same Compose network and are not published to the host. Nginx is the public entrypoint.

Default public routes:

- User web: `http://localhost:${NGINX_PORT:-80}/`
- Admin web: `http://localhost:${NGINX_PORT:-80}/admin/`
- API and docs: proxied through `/api/*`, `/docs/*`, `/v1/*`, `/healthz`, and `/readyz`

See [`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md) for a deployment runbook.

### Dev vs Prod Compose

`docker-compose.dev.yml` is optimized for local iteration:

- Uses `APP_ENV=local` and `configs/config.dev.yaml`.
- Uses trust-based local PostgreSQL defaults.
- Starts API, worker, user web, admin web, Nginx, and middleware by default.
- Keeps API, worker, frontend containers, PostgreSQL, Redis, MinIO, and Mailpit inside the Compose network.
- Exposes only Nginx on `DEV_NGINX_PORT` (`8088` by default).
- Seeds a convenient local admin by default.
- Uses [`deployments/docker-compose/docker-compose-middileware.yml`](./deployments/docker-compose/docker-compose-middileware.yml) when you only want middleware for host-run source development.

`docker-compose.prod.yml` is optimized for deployment:

- Uses `APP_ENV=prod` and `configs/config.example.yaml` with required secret environment variables.
- Requires explicit database password, JWT secret, API-key encryption key, cashier provider encryption key, secure config encryption key, and admin credentials.
- Does not expose PostgreSQL, Redis, API, worker, or frontend containers to the host.
- Exposes only Nginx on `NGINX_PORT` (`80` by default).
- Adds optional Prometheus through the `monitoring` profile.

## Development Guide

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

Run API smoke checks against a live API:

```bash
BASE_URL=http://127.0.0.1:8080 ./scripts/workflow/api-smoke.sh
```

Run the Docker E2E suite:

```bash
./scripts/e2e/run-docker-e2e.sh --start
```

Stop the E2E stack:

```bash
docker compose -f deployments/docker-compose/docker-compose.e2e.yml down --remove-orphans
```

## Project Structure

```text
api/openapi/              OpenAPI contract and API documentation source
cmd/api/                  API service entrypoint
cmd/worker/               async worker entrypoint
configs/                  local and deployment config templates
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
