# Deployment and Operations Runbook

## Deployment Boundary

`deployctl` is the supported deployment entrypoint. It writes all generated files under the selected runtime directory and uses one canonical configuration file:

```text
./config/runtime.env
```

The file is generated with detailed Chinese and English comments. API and Worker read it from their working directory. `APP_ENV_FILE` is reserved for service managers that cannot set a working directory; the removed `PIC_GALLERY_ENV_FILE` variable is not supported.

The project accepts HTTP and IP-plus-port access. Domain binding, TLS certificates, reverse proxies, and external load balancers are operator responsibilities.

## Deployment Matrix

| Mode | Profile | Included services | Middleware |
| --- | --- | --- | --- |
| Docker | `full` | API, Worker, three Web apps, gateway | Managed PostgreSQL, Redis, and MinIO |
| Docker | `core` | API, Worker, three Web apps, gateway | External PostgreSQL, Redis, and object storage |
| Docker | `custom` | Operator-selected valid component set | Managed only when selected |
| Native Linux/Windows | `core` or `custom` | Prebuilt API, Worker, gateway/services | External middleware only |

Native `full` is intentionally unsupported. Docker and native core nodes can join the same installation when they share PostgreSQL, Redis, S3 storage, application secrets, schema version, and configuration revision.

## Prerequisites

Docker deployment requires Docker Engine, Compose v2, image registry access, and a writable runtime directory.

Native deployment requires a supported prebuilt release bundle, service-manager privileges, external PostgreSQL and Redis, and local shared storage for single-node deployments or S3-compatible storage for clusters. Target hosts do not need Go or Node.js.

Multi-API deployments require an external load balancer. `deployctl` installs nodes and reports health; it does not configure public ingress.

## First Installation

Run from the directory that should own the deployment files:

```bash
./scripts/install.sh install --mode docker --profile full --topology single --yes
```

Windows:

```powershell
./scripts/install.ps1 install --mode docker --profile full --topology single --yes
```

For Docker core:

```bash
./scripts/install.sh install --mode docker --profile core --topology single --yes
```

For native core:

```bash
./scripts/install.sh install --mode native --profile core --topology single --yes
```

Omit `--yes` for the interactive selector. Use `--runtime-dir <path>` when the current directory should not contain the runtime files.

Installation generates secrets, deployment assets, `deployment.json`, `config/install-state.json`, and `config/runtime.env`, then starts the selected services. Non-interactive output does not print the setup token.

## Setup

Before setup is complete, the API exposes health checks and the API-hosted setup UI only. Open:

```text
http://<api-host>:<api-port>/setup/
```

The setup UI uses the admin-console visual system. It configures PostgreSQL, Redis, object storage, and the first administrator, probes connectivity, runs the explicit migration under a distributed lock, commits the installation, and restarts the API.

Retrieve or rotate the one-time setup credential locally:

```bash
deployctl setup status
deployctl setup token show
deployctl setup token reset
```

Reset increments `SETUP_TOKEN_VERSION`, invalidates the old token and sessions, writes the file atomically, and restarts the deployment. Show and reset are permanently refused after setup completes.

After restart, configure provider accounts, text and image models, routes, prices, plans, registration policy, recharge/payment providers, SMTP, and other business settings in the admin console.

## Cluster Nodes

Create credentials only on an initialized control node:

```bash
deployctl cluster token create --role api --ttl 10m
deployctl cluster token create --role worker --ttl 10m
deployctl cluster token create --role web --ttl 10m
```

Join on the target host:

```bash
deployctl cluster join \
  --server http://10.0.0.10:8080 \
  --token '<single-use-token>' \
  --mode docker \
  --runtime-dir .
```

Enrollment uses an authenticated encrypted envelope and stores no plaintext join credential. Tokens expire, are single-use, and are role-bound. Joined API and Worker nodes refuse startup on installation, application, schema, configuration-revision, or node-identity mismatch.

API replicas can serve read-only node health. Token creation and revocation remain control-node operations. External load balancers should use `/healthz` for liveness and `/readyz` for traffic readiness.

## Legacy Configuration Import

The application does not automatically read root `.env`, `.env.prod`, or packaged `backend.env` files. Import one explicitly:

```bash
deployctl import-config --source .env --mode docker --profile core --topology single
deployctl import-config --source .env.prod --mode docker --profile full --topology single --storage-driver s3
deployctl import-config --source /path/to/backend.env --mode native --profile core --topology single
```

Import maps supported fields, generates missing secrets, rebuilds managed connection URLs, renders bilingual comments, and never modifies the source. Existing target files are not overwritten. Partial writes are rolled back. A legacy installation is marked completed only when middleware, installation identity/setup binding, schema, and administrator checks all succeed; otherwise it remains in setup mode.

Keep the source until `doctor`, readiness, administrator login, and a business smoke test all pass.

## Operations

```bash
deployctl status
deployctl doctor
deployctl restart
```

`doctor` checks runtime fields, private file permissions, manifest/state identity, middleware connectivity, and database schema compatibility. Diagnostics redact DSNs, tokens, passwords, and encryption keys.

Upgrade a Docker single/control installation:

```bash
deployctl upgrade --application-version v1.2.3 --image-tag sha-immutable-tag
```

Upgrade a native single/control installation:

```bash
deployctl upgrade --application-version v1.2.3 --release-version v1.2.3
```

The control path atomically upgrades the runtime schema and deployment manifest, acquires the database migration lock, migrates once, then rolls services in dependency order. Joined nodes must not migrate:

```bash
deployctl upgrade --application-version v1.2.3 --image-tag sha-immutable-tag --migrate=false
```

Back up external databases and object storage with provider-native tooling before upgrades. For Docker full, back up the named PostgreSQL and MinIO volumes before destructive maintenance.

## Uninstall Safety

Ordinary uninstall stops and unregisters services but preserves configuration and all persistent data:

```bash
deployctl uninstall --yes
```

Permanent deletion is intentionally not authorized by `--yes`. First read the installation ID:

```bash
deployctl setup status
```

Then provide the exact, case-sensitive phrase:

```bash
deployctl uninstall --delete-data \
  --confirm 'DELETE <installation-id> PERSISTENT DATA'
```

For Docker, this additionally removes Compose named volumes. For native deployments, it removes the selected runtime directory after services stop. The command refuses filesystem roots, the current working directory, and directories containing the current working directory.

## Failure Recovery

- A pending setup can reuse its current token or rotate it with `setup token reset`.
- A failed probe writes no final setup configuration.
- A migration failure keeps setup pending.
- A completed installation with missing or corrupt runtime files fails closed and never reopens anonymous setup.
- Use `deployctl doctor`, service logs, `deployctl status`, and `deployctl restart` in that order after an interrupted operation.
- Never hand-edit `SETUP_COMPLETED`, installation identity, cluster identity, schema version, or configuration revision to bypass recovery checks.
