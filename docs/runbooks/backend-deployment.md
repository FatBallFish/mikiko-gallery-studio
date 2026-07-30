# Deployment and Operations Runbook

## Deployment Boundary

`mgsctl` is the supported deployment entrypoint. It writes all generated files under the selected runtime directory and uses one canonical configuration file:

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

Docker deployment requires Docker Engine, Compose v2, and a writable runtime directory. Registry access is normally required. Official images use repositories such as `docker.io/fatballfish/mikiko-gallery-studio-api`. If a selected application image cannot be pulled, mgsctl may build selected images from a complete checkout supplied by `scripts/install.sh` or `scripts/install.ps1`. Base and middleware images still come from their registries.

Native deployment requires a supported prebuilt release bundle, service-manager privileges, external PostgreSQL and Redis, and local shared storage for single-node deployments or S3-compatible storage for clusters. Target hosts do not need Go or Node.js.

Multi-API deployments require an external load balancer. `mgsctl` installs nodes and reports health; it does not configure public ingress.

## First Installation

Run from the directory that should own the deployment files:

```bash
./scripts/install.sh install --mode docker --profile full --topology single --yes
```

These explicit flags show the default plan. `install --yes` defaults to Docker, `full`, `single`, and selector `latest`. mgsctl verifies `release-manifest.json`, resolves `latest` to a concrete SemVer version plus immutable image digests, and persists only the concrete application identity.

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

If `docker compose pull` fails, mgsctl validates the source checkout, locally builds the selected Pic Gallery images with the requested registry/tag, and continues startup. Without a complete checkout it returns the pull error together with actionable fallback guidance.

The same Setup-pending plan resumes automatically after a failed startup. A different interactive plan prompts before replacing generated configuration. For automation, pass `--overwrite` explicitly. Overwrite preserves `data/`, `logs/`, and Docker volumes and is rejected for completed or unrecognized installations.

## TUI, Help, and Endpoint Handoff

Running `mgsctl` without arguments opens the TUI only when both input and output are terminals. Number keys and Arrow keys select, Enter confirms, Space toggles multi-select fields, Esc returns, and Ctrl+C exits. For scripts, redirected output, or command discovery, use `mgsctl -h` or `mgsctl --help`; a no-argument non-terminal invocation prints the same help and exits without blocking.

The TUI defaults to Chinese (`zh-CN`). The Language field switches to English (`en-US`) immediately and stores the preference in `mgsctl/config.json`. Full and core component sets are read-only presets; custom remains editable.

Successful installation prints a plan-derived endpoint summary, access scope, and numbered next steps. Interactive terminal output may include the one-time Setup token. Non-interactive or redirected output never exposes it and prints the exact host-local token recovery command instead.

Direct ports for the default Docker full plan are user `:5173`, admin `:5174`, docs `:5175`, and API `:8080`. Gateway paths are `/`, `/admin/`, `/developer-docs/`, `/api/`, and `/setup`. Direct ports use the current browser hostname plus the configured API port; Gateway paths and external same-origin proxies use their own origin. An explicit `PUBLIC_API_URL` takes precedence. User and admin redirect to the API-hosted Setup page while initialization is pending. Documentation remains available throughout Setup.

## Setup

Before setup is complete, the API exposes health checks and the API-hosted setup UI only. Open:

```text
http://<api-host>:<api-port>/setup
```

The setup UI uses the admin-console visual system. It configures PostgreSQL, Redis, object storage, and the first administrator, probes connectivity, runs the explicit migration under a distributed lock, commits the installation, and restarts the API.

Retrieve or rotate the one-time setup credential locally:

```bash
mgsctl setup status
mgsctl setup token show
mgsctl setup token reset
```

Reset increments `SETUP_TOKEN_VERSION`, invalidates the old token and sessions, writes the file atomically, and restarts only API and Gateway. Worker, Web services, and managed middleware remain running. Show and reset are permanently refused after setup completes.

After restart, configure provider accounts, text and image models, routes, prices, plans, registration policy, recharge/payment providers, SMTP, and other business settings in the admin console.

## Cluster Nodes

Create credentials only on an initialized control node:

```bash
mgsctl cluster token create --role api --ttl 10m
mgsctl cluster token create --role worker --ttl 10m
mgsctl cluster token create --role web --ttl 10m
```

Join on the target host:

```bash
mgsctl cluster join \
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
mgsctl import-config --source .env --mode docker --profile core --topology single
mgsctl import-config --source .env.prod --mode docker --profile full --topology single --storage-driver s3
mgsctl import-config --source /path/to/backend.env --mode native --profile core --topology single
```

Import maps supported fields, generates missing secrets, rebuilds managed connection URLs, renders bilingual comments, and never modifies the source. Existing target files are not overwritten. Partial writes are rolled back. A legacy installation is marked completed only when middleware, installation identity/setup binding, schema, and administrator checks all succeed; otherwise it remains in setup mode.

Keep the source until `doctor`, readiness, administrator login, and a business smoke test all pass.

## Operations

Runtime discovery is shared by status, doctor, restart, upgrade, uninstall, Setup, and cluster control commands. Resolution order is explicit `--runtime-dir`, the current directory, `./runtime` under the current directory, then the saved runtime from `mgsctl/config.json`. Ambiguous or invalid candidates fail closed.

`mgsctl self-update` replaces only the control-tool executable. `mgsctl upgrade` resolves and deploys an application Release, performs the target database migration when authorized, and rolls the selected services. Updating the tool does not implicitly change a running application.

```bash
mgsctl status
mgsctl doctor
mgsctl restart
```

`doctor` checks runtime fields, private file permissions, manifest/state identity, middleware connectivity, and database schema compatibility. Diagnostics redact DSNs, tokens, passwords, and encryption keys.

For Docker nodes that include API, `doctor` checks the loopback-published `/readyz`; normal API readiness is reached only after the container-network database, Redis, storage, schema, and installation binding checks pass. Native deployments and nodes without API use direct middleware/schema probes.

Upgrade a Docker single/control installation:

```bash
mgsctl upgrade --image-tag v1.2.3
```

Upgrade a native single/control installation:

```bash
mgsctl upgrade --release-version v1.2.3
```

If no selector is supplied, upgrade resolves `latest`. The checksummed `release-manifest.json` determines the concrete logical application version; it is not entered manually. Docker first pulls target digests, then runs `mikiko-gallery-studio-db-migrate` from the target API image inside the current Compose network before rolling services. Native mode stages and runs the target Release migration binary. Joined nodes must not migrate:

```bash
mgsctl upgrade --image-tag v1.2.3 --migrate=false
```

Database migrations are forward-compatible and are not automatically reversed. If service rollout fails after a successful migration, the target runtime and manifest remain published; rerun the same `mgsctl upgrade` command to resume the idempotent rollout. If rollout fails without a migration, `mgsctl` restores the previous runtime and manifest and actively reapplies the previous deployment plan.

Back up external databases and object storage with provider-native tooling before upgrades. For Docker full, back up the named PostgreSQL and MinIO volumes before destructive maintenance.

## Uninstall Safety

Ordinary uninstall stops and unregisters services but preserves configuration and all persistent data:

```bash
mgsctl uninstall --yes
```

Permanent deletion is intentionally not authorized by `--yes`. First read the installation ID:

```bash
mgsctl setup status
```

Then provide the exact, case-sensitive phrase:

```bash
mgsctl uninstall --delete-data \
  --confirm 'DELETE <installation-id> PERSISTENT DATA'
```

For Docker, this additionally removes Compose named volumes. Before stopping services or deleting any persistent resource, the command verifies that the runtime tree contains only mgsctl-managed configuration, deployment assets, native release files, application data, and logs. Unknown files or directories fail closed. The command also refuses filesystem roots, the current working directory, and directories containing the current working directory.

## Failure Recovery

- If an initial image pull failed, rerun the exact install command to resume. From a complete source checkout, missing Pic Gallery application images are built locally. Use `--overwrite` only when intentionally replacing a different Setup-pending plan.
- A pending setup can reuse its current token or rotate it with `setup token reset`.
- If a browser closes or the API restarts after setup has crossed a durable boundary, open `/setup` again and authenticate with the current Token. The authenticated session returns the persisted operation ID; re-enter the same editable configuration and secrets, rerun the probes, and apply to resume that operation.
- A failed probe writes no final setup configuration.
- A migration failure keeps setup pending.
- A completed installation with missing or corrupt runtime files fails closed and never reopens anonymous setup.
- Use `mgsctl doctor`, service logs, `mgsctl status`, and `mgsctl restart` in that order after an interrupted operation.
- Never hand-edit `SETUP_COMPLETED`, installation identity, cluster identity, schema version, or configuration revision to bypass recovery checks.

## Acceptance Tests

The deployment suites use unique Compose projects, temporary registries, and fresh volumes. They never connect to or remove the shared `pic-gallery-local` development database.

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/e2e/setup-docker-e2e.sh
./scripts/e2e/cluster-docker-e2e.sh
./scripts/e2e/run-docker-e2e.sh
```

`verify.sh` also cross-builds and inspects Linux and Windows native release archives. Windows service-definition behavior is covered by unit tests; starting an actual Windows service remains a manual platform acceptance step. Failed deployment E2E runs retain redacted evidence under ignored `tmp/e2e/` paths and remove only resources labeled with their own project IDs.
