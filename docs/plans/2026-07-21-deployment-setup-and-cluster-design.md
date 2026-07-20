# Deployment, Setup, and Cluster Bootstrap Design

## Status

Approved through incremental review on 2026-07-21.

## Summary

This design replaces the current manual first-deploy workflow with one cross-platform deployment CLI, one portable runtime env file, an API-hosted setup experience, and role-scoped cluster enrollment.

The target workflow is:

1. Run one deployment command.
2. Open the setup URL returned by the API.
3. Validate PostgreSQL, Redis, and object storage.
4. Create the first administrator and initialize the database.
5. Restart into normal mode.
6. Finish provider, routing, pricing, product, registration, and payment configuration in the admin console.
7. Use the complete user workflow.

Docker and native deployments share the same application setup state machine. Their only difference is which resources the deployment tool owns before setup begins.

## Goals

- Minimize configuration required before the first process starts.
- Support a one-command interactive deployment flow.
- Support non-interactive deployment for automation.
- Support Docker full, Docker core, and native core deployment profiles.
- Support Linux systemd and Windows service deployments.
- Support single-node operation and distributed API/Worker nodes.
- Keep all application runtime bootstrap configuration in one portable env file.
- Provide a browser-based first-run setup experience without depending on user-web or admin-web routing.
- Prevent business routes from being exposed before initialization completes.
- Keep operational/business configuration in the admin console after setup.
- Make setup recoverable when the operator loses the setup token or leaves initialization unfinished.

## Non-Goals

- Managing domain names, TLS certificates, or external load balancers.
- Building a PostgreSQL, Redis, or MinIO multi-node control plane.
- Installing native middleware packages on Linux or Windows.
- Providing native full-stack deployment.
- Managing SSH or Windows administrator credentials for remote nodes.
- Replacing the admin console for provider, routing, pricing, plan, registration, SMTP, or payment configuration.
- Providing Kubernetes, Docker Swarm, or another scheduler in the first release.

## Current State

The current project has three partially separate deployment stories:

- Production Docker Compose runs PostgreSQL, Redis, API, Worker, three frontend services, Nginx, shared local storage, and optional Prometheus. It does not run MinIO.
- Native/local tooling builds API, Worker, user-web, and admin-web, installs API/Worker through systemd, launchd, or Windows Task Scheduler, and expects an external static web server for production frontend hosting.
- Packaged devops artifacts use backend and frontend env files with different startup scripts.

Current first-start limitations:

- Production API startup requires `DATABASE_URL` and Redis before it can register any routes.
- Worker startup has the same database and Redis dependency.
- API and Worker both run database preparation and schema creation on startup.
- Production requires multiple manually generated application secrets.
- The initial administrator is seeded through env variables.
- Docker Compose injects many runtime values directly as container environment variables, which take precedence over file values.
- No setup-only router, setup state, or browser initialization flow exists.
- Native Windows deployment cannot serve the frontend without separately installed web-server software.
- Current production Compose is a single-host topology and does not enroll remote API or Worker nodes.

The Worker already has useful cluster primitives: PostgreSQL task leases, owner heartbeats, lease expiration and recovery, and Redis-backed concurrency coordination. API services are largely stateless when they share PostgreSQL, Redis, signing keys, and object storage.

## Key Decisions

1. Use an API dual-mode process: setup-only before initialization and normal after initialization.
2. Use `./config/runtime.env` as the default runtime configuration file in every deployment mode.
3. Remove the retired product-specific env-file selector without a compatibility alias.
4. Keep optional generic `APP_ENV_FILE` path override support.
5. Generate runtime env files from one typed configuration schema with detailed Chinese and English comments.
6. Use a cross-platform Go `deployctl` as the deployment control plane.
7. Keep shell and PowerShell files as thin download/launch wrappers only.
8. Support full service only in Docker mode.
9. Support core service in Docker and native modes.
10. Build distributed API/Worker deployments from the core profile only.
11. Use role-scoped, short-lived, single-use join tokens instead of remote SSH installation.
12. Serve setup UI directly from the API binary and style it with the admin design system.
13. Allow HTTP, IP-address, and port-only deployments. TLS remains the deployer's responsibility.
14. Keep setup logic identical for managed and external middleware; resource ownership is metadata, not a separate setup path.
15. Move schema migration out of ordinary API/Worker startup.

## Deployment Modes

### Module Catalog

The deployment tool models these modules:

- `api`
- `worker`
- `user-web`
- `admin-web`
- `docs-web`
- `gateway`
- `postgres`
- `redis`
- `minio`
- `monitoring`

### Preset Profiles

| Mode | Profile | Modules | Multi-node |
| --- | --- | --- | --- |
| Docker | full | API, Worker, three Web services, Gateway, PostgreSQL, Redis, MinIO | No |
| Docker | core | API, Worker, three Web services, Gateway | Yes |
| Native Linux/Windows | core | API, Worker, three Web assets, portable Gateway | Yes |
| Docker/native | custom | Validated operator selection | Only valid core role combinations |

Docker full is a single-node, self-contained profile. It does not attempt to operate PostgreSQL, Redis, or MinIO clusters.

Native full deployment is intentionally unsupported. Native deployment never installs or mutates operating-system middleware packages. Operators without Docker must provide external PostgreSQL, Redis, and object storage.

### Topology Roles

- `single`: all selected application modules on one node.
- `control`: setup authority and first API node; may also run Worker and Web modules.
- `api`: additional API replica.
- `worker`: additional Worker node.
- `web`: frontend/Gateway-only node.

External load balancers are responsible for distributing traffic across API or Gateway nodes. The project exposes health and readiness information but does not configure the load balancer.

### Custom Module Validation

`deployctl` rejects unsafe combinations before writing configuration:

- Native mode cannot select PostgreSQL, Redis, or MinIO.
- Multi-node topology cannot select local object storage.
- Worker nodes must join an initialized installation.
- Web nodes require a reachable API public base URL.
- Gateway may be omitted only after explicit confirmation that another static server/reverse proxy exists.
- Non-control nodes cannot execute setup or migrations.
- Incompatible application versions cannot join the same installation.

## Portable Runtime Directory

Every deployment uses a movable runtime directory:

```text
runtime/
  config/
    runtime.env
    install-state.json
  data/
  logs/
  compose.yml or native release files
```

The application default is relative to its configured working directory:

```text
./config/runtime.env
```

No path contains the product name, `/opt`, `/etc`, or another operating-system-specific root.

Docker bind-mounts the configuration directory, not an individual file. This permits same-directory temporary writes and atomic rename:

```yaml
services:
  api:
    working_dir: /app
    volumes:
      - ./config:/app/config
  worker:
    working_dir: /app
    volumes:
      - ./config:/app/config:ro
```

The host and container have different absolute filesystem namespaces, but both resolve `./config/runtime.env` relative to their working directory.

Native service definitions set the working directory explicitly. Linux uses directory mode `0700` and file mode `0600`. Windows applies an ACL limited to the service identity and Administrators.

## Configuration Schema and Env Rendering

One typed configuration schema is authoritative for:

- env field names and order;
- Chinese and English descriptions;
- examples and defaults;
- secret classification;
- owning component;
- required conditions by mode/profile/role;
- validation and probe rules;
- restart requirements;
- setup form metadata;
- generated reference documentation.

Every rendered field includes detailed bilingual comments:

```dotenv
# [中文] PostgreSQL 连接地址。账号需要具备当前数据库的建表、索引和读写权限。
# [English] PostgreSQL connection URL. The account must be able to create schema objects and read/write data.
# Required when: all normal deployments
# Example: postgres://app:password@127.0.0.1:5432/app?sslmode=disable
DATABASE_URL=
```

The env document includes a schema version. Upgrade tooling adds missing fields and updated comments without overwriting values. Unknown extension keys are retained in a dedicated extension section.

The parser and writer implement dotenv quoting themselves; they do not call shell `source`. URLs, spaces, `#`, quotes, and other supported values must round-trip safely. Connection passwords embedded in URLs are percent-encoded.

Setup updates allowlisted keys only. Image versions, selected modules, host ports, and deployment ownership metadata remain deployctl-owned.

### Configuration Ownership

Deployment-generated fields include:

- deployment mode, profile, topology, role, and installation identity;
- managed-resource flags;
- setup state and setup token;
- auth signing secret;
- API-key secret encryption key;
- secure-config encryption key;
- cashier-config encryption key;
- prompt-optimization quote signing key;
- managed PostgreSQL, Redis, and MinIO credentials;
- host ports and image/release versions.

Setup-managed fields include:

- PostgreSQL connection URL;
- Redis connection URL and key prefix;
- local or S3-compatible object-storage bootstrap configuration;
- public API URL and CORS origins when separate origins are used;
- non-secret application defaults required for normal startup.

The first administrator password is never stored in `runtime.env`. Setup hashes it in the initialization transaction and discards the plaintext.

### Resource Ownership Metadata

Full and core profiles use the same setup request and state machine. They differ only in how the draft connection values were supplied:

```dotenv
DEPLOYMENT_PROFILE=full
POSTGRES_MANAGED=true
REDIS_MANAGED=true
OBJECT_STORAGE_MANAGED=true
```

Full mode pre-populates managed connection fields and renders them read-only in the setup UI. Core mode marks them unmanaged and permits editing. Both modes execute the same probes, migration, administrator creation, and commit logic.

## Deployctl

`deployctl` is a cross-platform Go CLI. Shell and PowerShell bootstrap files only acquire and invoke it.

Primary commands:

```text
deployctl install
deployctl status
deployctl doctor
deployctl restart
deployctl upgrade
deployctl uninstall
deployctl setup status
deployctl setup token show
deployctl setup token reset
deployctl cluster token create --role <role>
deployctl cluster join --server <url> --token <token>
```

Interactive install prompts for mode, profile, topology, components, ports, image/release version, and runtime directory. It generates the runtime skeleton, secrets, setup token, service definitions, and deployment manifest before starting services.

Non-interactive flags support automation:

```bash
deployctl install --mode docker --profile full --topology single --yes
```

Full Docker deployment requires only Docker Engine, Compose v2, a writable runtime directory, and image access. Native core deployment consumes prebuilt release artifacts; target hosts do not need Go or Node.js.

Uninstall never deletes persistent data by default. Destructive removal requires an explicit confirmation token.

## API Startup Modes

### Bootstrap Loading

The API first loads a tolerant bootstrap view of `runtime.env`. It does not apply normal production validation until it knows setup is complete.

Startup modes:

- `setup`: installation has never completed; register only bootstrap/setup surfaces.
- `normal`: completion marker and required runtime fields are valid; construct all services and business routes.
- `broken`: installation previously completed but the runtime file, install identity, or required configuration is missing or invalid; fail closed with diagnostic status.

`install-state.json` contains no secrets. It stores installation ID, schema version, deployment role, and the fact that initialization once completed. If `runtime.env` is deleted after completion, this file prevents anonymous setup from reopening.

### Route Surfaces

Always available:

```text
GET /healthz
GET /readyz
GET /api/system/v1/bootstrap-status
```

Setup mode additionally exposes:

```text
GET  /setup
POST /api/setup/v1/session
POST /api/setup/v1/probes/database
POST /api/setup/v1/probes/redis
POST /api/setup/v1/probes/storage
POST /api/setup/v1/apply
GET  /api/setup/v1/progress/{operation_id}
```

Normal mode does not register `/setup`, setup assets, or setup write endpoints. It registers the normal API surface.

Health semantics:

- `/healthz` reports whether the process is alive and returns success in setup mode.
- `/readyz` reports whether normal business traffic is safe and returns unavailable in setup mode.
- Gateway starts against liveness rather than business readiness so operators can reach setup.

## Setup Token and Recovery

`deployctl` generates a high-entropy setup token before the first API start. It stores the token in the protected runtime env file and prints it once to the interactive terminal.

The token remains reusable until setup completes. First use exchanges it for a short-lived HttpOnly setup session; it does not consume the token. SameSite is strict, and the Secure cookie attribute is enabled when the request is HTTPS.

An operator who accesses setup without a token sees exact guidance:

```text
deployctl setup token show
deployctl setup token reset
```

`show` reads the protected local runtime file. `reset` is permitted only before completion, rotates the token, invalidates active setup sessions, atomically updates the env file, and restarts/reloads the setup API. Completed installations reject both reset and any attempt to reopen setup.

Setup authentication and probes use local in-process rate limiting because Redis may not be configured yet. Comparisons are constant-time, and failures are audited without logging token material.

HTTP and IP-address deployments are supported. Documentation warns that HTTP cannot provide TLS-equivalent protection for a browser setup session. The project does not obtain or manage certificates.

## API-Hosted Setup UI

The setup page is served directly by the API at the backend's own origin. It is not hosted by user-web or admin-web and does not require the API, user frontend, and admin frontend to share a hostname or path structure.

User-web and admin-web query bootstrap status through their configured API base URL. When setup is required, they navigate to the authoritative `setup_url` returned by the backend.

The setup UI source is compiled into the API binary or image. API responses provide the page and any embedded setup assets; deployment does not include a separate setup HTML file, frontend service, or CDN dependency.

Visual requirements:

- reuse the admin design tokens and operational visual language;
- use admin form, button, status, validation, and progress patterns;
- do not render user navigation, marketing, balance, or creation UI;
- provide responsive desktop and mobile layouts;
- expose no external font, script, or image dependency during setup.

Admin UI and setup UI consume one shared design-token source so their styles cannot drift independently.

Security headers include `Cache-Control: no-store`, a strict Content Security Policy, content-type protections, and frame denial.

The originating frontend does not need to know the admin frontend URL. The browser history retains the original page; after restart reaches ready, the setup page uses history navigation to return. Direct `/setup` visits without useful history show a completion message instructing the operator to return to the original entry point.

## Setup State Machine

```text
pending
  -> validating
  -> initializing_database
  -> creating_admin
  -> committing_config
  -> restart_pending
  -> complete
```

The setup form provides independent probes for PostgreSQL, Redis, and object storage. Managed full-profile connections are pre-populated and read-only; core-profile connections are editable.

Final apply executes:

1. Acquire a process and file-level setup lock.
2. Validate schema-required fields for the selected mode/profile/role.
3. Probe PostgreSQL, Redis, and object storage again.
4. Check PostgreSQL version and schema privileges.
5. Run the explicit database migration.
6. Create the installation record and first administrator transactionally.
7. Persist an install-state commit journal containing the operation and installation IDs.
8. Render and atomically replace `runtime.env`.
9. Finalize the install-state marker as completed.
10. Return an operation ID and restart countdown.
11. Exit with the documented restart code after the response is flushed.

`SETUP_COMPLETED=true` is written only after all required persistent fields have values and all initialization work succeeds. Optional and mode-inapplicable fields may remain empty according to the configuration schema.

The operation is idempotent. Retries after a network failure or process interruption reuse the installation operation ID and do not create duplicate administrators or apply migrations twice.

The two persistent files use a recoverable commit protocol rather than pretending that two filesystem renames are one transaction. A crash before the runtime env rename resumes setup. A crash after the env rename but before the final state rename reconciles the commit journal against the database installation record, then finalizes completion. Once `EverCompleted` is true, missing or inconsistent files always fail closed.

## Service Lifecycle

Before completion:

- API serves liveness, bootstrap status, and setup only.
- Worker watches the runtime configuration and does not connect to middleware or claim tasks.
- Web services may start and redirect to the API-hosted setup page.
- Gateway routes setup even though business readiness is unavailable.

After successful apply:

- API reports `restart_pending`, flushes the response, and exits.
- Docker, systemd, or Windows service management restarts it.
- Worker observes completion or is restarted by deployctl.
- API and Worker load the same runtime config and verify the expected schema version.
- API reports ready only after database, Redis, storage, and schema checks pass.
- The setup page polls for readiness, shows approximately a ten-second countdown, and returns through browser history.

Production startup no longer performs implicit schema mutation. First setup and upgrades invoke explicit migration commands under a distributed migration lock.

## Middleware Rules

PostgreSQL and Redis are required for every normal production profile.

Object storage rules:

- Single-node deployments may use a shared local path visible to API and Worker.
- Multi-node deployments require S3-compatible storage such as MinIO, R2, or S3.
- Full Docker mode provides managed MinIO.
- Core modes use setup-supplied external storage or single-node local storage.

Core PostgreSQL must already contain an empty target database and an application user with schema creation, index creation, and application read/write permissions. Setup never accepts PostgreSQL server-superuser credentials and never creates server-level users or databases.

Full Docker `deployctl` creates the managed database, user, Redis password, MinIO credentials, and buckets before API setup begins. Setup then treats them exactly like pre-provisioned core resources.

## Cluster Enrollment

Only an initialized control installation can issue join tokens. Tokens are role-scoped, single-use, and short-lived:

```bash
deployctl cluster token create --role api --ttl 10m
deployctl cluster token create --role worker --ttl 10m
deployctl cluster token create --role web --ttl 10m
```

Joining:

```bash
deployctl cluster join --server http://10.0.0.10:8080 --token <token>
```

Enrollment flow:

1. Fetch installation and version challenge metadata.
2. Prove token possession without sending its secret portion directly.
3. Generate an ephemeral node key pair.
4. Validate role, expiry, use count, installation state, and version compatibility.
5. Produce the smallest role-specific configuration set.
6. Encrypt and authenticate the configuration envelope with the ephemeral key and token-derived material.
7. Write the local runtime env and install-state files atomically.
8. Register node identity, role, version, and health metadata.
9. Mark the token consumed.
10. Install and start selected role services.

Role configuration boundaries:

- API receives database, Redis, storage, auth, and required encryption/signing material.
- Worker receives database, Redis, storage, provider/storage decryption material, and Worker settings.
- Web receives only public API/frontend runtime values.
- No joined node receives setup token or administrator password.

Application-layer encryption prevents plaintext database and application secrets from appearing in HTTP response bodies. Documentation still states that HTTP does not provide the complete server-authentication and transport guarantees of TLS.

Every node records `INSTALLATION_ID`, `CLUSTER_NODE_ID`, role, configuration revision, and application version. A node refuses to start against the wrong installation or an unsupported schema/application version.

## Distributed Runtime

API replicas share PostgreSQL, Redis, object storage, signing keys, and encryption keys. External infrastructure distributes traffic and performs readiness checks.

Worker replicas use the existing PostgreSQL lease, heartbeat, expiration, and recovery model. Redis coordinates cross-node concurrency limits and configuration invalidation. Shared object storage guarantees that any API or Worker can read task inputs and outputs.

The control plane records node registrations and last heartbeat. `deployctl status` reports local state; the admin console reports cluster nodes, roles, versions, health, and last contact.

Schema migration is a control operation:

- setup runs the initial migration once;
- `deployctl upgrade` acquires a migration lock and migrates before rolling restart;
- ordinary API/Worker replicas only check schema compatibility;
- incompatible replicas remain unready.

## Portable Native Gateway

Native deployment adds a small cross-platform Gateway process because Windows cannot rely on a preinstalled Nginx.

The Gateway:

- serves user, admin, and docs build assets;
- generates or serves frontend runtime configuration;
- proxies API paths when deployed in same-origin mode;
- exposes setup and readiness routes from the API;
- runs under systemd or Windows service management;
- can be omitted when an external reverse proxy/static host is already configured.

It does not manage TLS or domain names.

## Operational Configuration Boundary

Setup stores only infrastructure/bootstrap configuration and creates the first administrator.

After normal startup, administrators configure these in the admin console:

- image and text provider accounts;
- models and capabilities;
- route models and candidates;
- prices and billing rules;
- plans and recharge products;
- payment channels;
- registration and trial policy;
- SMTP and security policy;
- additional object-storage instances;
- runtime Worker concurrency and other operational settings.

Production is not considered business-ready until at least one usable provider/model route, price, and required registration/payment configuration exists. This is reported by the existing/admin readiness model, not by setup completion.

## Failure Handling

- Probe failure preserves the form and writes no final configuration.
- Final validation failure does not run migration.
- Migration failure leaves setup pending and returns a sanitized diagnostic.
- Administrator and installation creation are transactional.
- Env write failure does not write completion state.
- Restart timeout shows `deployctl status`, log, doctor, and restart commands.
- Completed installations with unavailable middleware fail normal readiness and never reopen setup.
- Completed installations with missing/corrupt runtime config enter broken/fail-closed state.
- Concurrent apply requests are serialized and idempotent.
- Join expiry, replay, version mismatch, wrong role, or configuration authentication failure aborts before service installation.
- API responses and logs never contain full DSNs, passwords, object-storage keys, signing keys, or encryption keys.

## Migration from Current Deployments

Removing the retired product-specific env-file selector is an intentional breaking change. Application runtime does not retain an alias.

`deployctl` provides an explicit one-time import command for current `.env`, `.env.prod`, and packaged backend env files. Import:

- parses the old file;
- maps supported values into the new schema;
- generates missing secrets;
- renders bilingual comments;
- marks existing initialized databases as completed only after probing the installation and administrator state;
- preserves the source file until the operator confirms the new deployment is healthy.

Current Docker deployments are migrated to the runtime directory layout without deleting volumes. Upgrade and uninstall commands remain fail-closed around persistent data deletion.

## Verification Strategy

### Unit and Contract Tests

- Bootstrap versus full config loading.
- Dotenv encoding/decoding and bilingual comment rendering.
- Schema required-field matrices for every mode/profile/role.
- Unknown-field preservation and schema upgrades.
- Setup state transitions, locking, idempotency, and fail-closed behavior.
- Setup token display/reset/session invalidation and rate limiting.
- Managed versus unmanaged form behavior.
- Route absence in setup and normal modes.
- Cluster token expiry, single use, role boundaries, envelope authentication, and version checks.
- Custom module dependency validation.
- Linux and Windows service definition generation.

### Integration Tests

- PostgreSQL, Redis, and object-storage probes.
- Explicit migration and administrator transaction.
- Atomic env commit and restart status.
- Recovery after interruption at each apply phase.
- Completed install with missing/corrupt runtime config.
- API/Worker schema compatibility checks.
- Multi-Worker lease competition and failover.

### End-to-End Matrix

1. Empty directory to Docker full setup without manual env editing.
2. Missing setup token guidance, token show, and token reset.
3. Full setup, restart, setup-route closure, and admin login.
4. Admin operational configuration followed by user registration, login, recharge, image task, history, and public gallery.
5. Docker core against external middleware.
6. Linux native core package and service lifecycle.
7. Windows native core package and service lifecycle.
8. API replica enrollment and external load-balancer health behavior.
9. Multiple Worker enrollment, exactly-once claim/settlement behavior, and crash recovery.
10. Expired/replayed join tokens and incompatible versions.
11. HTTP enrollment responses contain no plaintext secret values.
12. Custom component profiles and dependency errors.
13. Desktop and mobile API-hosted setup UI matching the admin design system.

Repository verification, API smoke, code review gate, and Docker E2E remain mandatory before delivery.

## Acceptance Criteria

- A fresh Docker full deployment reaches setup with one command and no manual env editing.
- Docker and native core use the same API setup implementation.
- All persistent runtime bootstrap values live in `./config/runtime.env`.
- Every env field has detailed Chinese and English comments generated from one schema.
- Setup-only mode exposes no business APIs.
- Normal mode exposes no setup page or setup write APIs.
- Setup Token can be shown or reset before completion and cannot be used afterward.
- User and admin frontends redirect to the API-provided setup URL without knowing each other's hostnames.
- Setup UI is API-hosted and visually aligned with the admin design system.
- Setup creates an administrator who can log in after restart.
- Full, core, Linux, Windows, API-cluster, and Worker-cluster acceptance paths pass.
- Completed installations fail closed if bootstrap configuration is lost or invalid.
- Normal business configuration and user workflows pass after initialization.
