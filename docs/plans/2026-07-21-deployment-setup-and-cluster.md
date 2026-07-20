# Setup-Driven Deployment and Cluster Bootstrap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace manual first-deploy configuration with one portable runtime env, an API-hosted setup flow, cross-platform deployment tooling, and role-scoped API/Worker cluster enrollment.

**Architecture:** The API starts in setup-only, normal, or fail-closed mode based on `./config/runtime.env` and a non-secret install-state file. A cross-platform Go `deployctl` prepares Docker or native core deployments, while the API performs one shared middleware probe, migration, administrator creation, and atomic config commit flow. Distributed nodes join an initialized core deployment with short-lived role tokens and receive authenticated encrypted configuration envelopes.

**Tech Stack:** Go 1.26, `net/http`, Ent/PostgreSQL, Redis 7, S3-compatible storage/MinIO, React 19 frontends, Docker Compose v2, systemd, Windows services, Node-based repository contract tests.

---

### Task 1: Establish Coding Context and Baseline

**Files:**
- Reference: `docs/plans/2026-07-21-deployment-setup-and-cluster-design.md`
- Runtime-only: `.coding-context.json`

**Step 1: Start the heavyweight repository workflow**

Run:

```bash
./scripts/workflow/start-coding.sh --track heavyweight --task "Setup-driven Docker/native deployment and API/Worker cluster bootstrap"
```

Expected: exit 0 and `.coding-context.json` records the approved design and implementation plan as requirement/design sources.

**Step 2: Load implementation guardrails**

Read and follow:

```text
@dev-go-patterns
@dev-react-patterns
@superpowers:test-driven-development
```

Expected: all production edits start from failing tests or contract assertions.

**Step 3: Record the clean baseline**

Run:

```bash
git status --short
./scripts/workflow/verify.sh
```

Expected: clean worktree and existing verification PASS.

**Step 4: Commit only if workflow context files are intentionally tracked**

`.coding-context.json` must remain ignored. No source commit is expected for this task.

---

### Task 2: Add the Typed Runtime Configuration Schema and Bilingual Env Renderer

**Files:**
- Create: `internal/config/runtime_schema.go`
- Create: `internal/config/runtime_env.go`
- Create: `internal/config/runtime_schema_test.go`
- Create: `internal/config/runtime_env_test.go`
- Modify: `internal/config/config.go`

**Step 1: Write failing schema tests**

Define tests that require each field to expose structured metadata:

```go
type RuntimeField struct {
    Key          string
    Group        string
    DescriptionZH string
    DescriptionEN string
    Example      string
    Secret       bool
    Owner        FieldOwner
    RequiredWhen func(DeploymentContext) bool
    Validate     func(string) error
}
```

Test at minimum:

- unique stable keys;
- non-empty Chinese and English descriptions for every field;
- secrets have no real secret example;
- Docker full/core, native core, control/API/Worker/Web role required matrices;
- multi-node storage requires S3;
- setup/admin plaintext password is not a persistent field.

**Step 2: Run focused tests and confirm failure**

Run:

```bash
go test ./internal/config -run 'TestRuntimeSchema|TestRequiredRuntimeFields'
```

Expected: FAIL because the schema does not exist.

**Step 3: Implement schema metadata**

Add deployment enums and a deterministic schema:

```go
type DeploymentMode string
const (
    DeploymentModeDocker DeploymentMode = "docker"
    DeploymentModeNative DeploymentMode = "native"
)

type DeploymentProfile string
const (
    DeploymentProfileFull   DeploymentProfile = "full"
    DeploymentProfileCore   DeploymentProfile = "core"
    DeploymentProfileCustom DeploymentProfile = "custom"
)

type DeploymentRole string
const (
    DeploymentRoleSingle  DeploymentRole = "single"
    DeploymentRoleControl DeploymentRole = "control"
    DeploymentRoleAPI     DeploymentRole = "api"
    DeploymentRoleWorker  DeploymentRole = "worker"
    DeploymentRoleWeb     DeploymentRole = "web"
)
```

Include schema version, deployment metadata, managed flags, setup state/token, middleware connections, application secrets, public URLs, CORS, ports, image/release fields, and role/version identity.

**Step 4: Write failing dotenv round-trip tests**

Cover:

```text
spaces
# characters
= characters
single and double quotes
Unicode comments
percent-encoded DSN credentials
empty optional values
unknown extension fields
existing value preservation during schema upgrades
```

Assert the rendered document contains two comment lines for every field and never renders unredacted secret examples.

**Step 5: Implement a document parser and canonical renderer**

Do not invoke shell `source`. Model the env document as entries so schema upgrades can preserve unknown fields. Render through a temporary file in the target directory, `fsync`, set permissions, and atomically rename.

Expose:

```go
func ParseRuntimeEnv(data []byte) (RuntimeEnvDocument, error)
func RenderRuntimeEnv(schema RuntimeSchema, values map[string]string, extensions []EnvEntry) ([]byte, error)
func WriteRuntimeEnvAtomic(path string, data []byte) error
```

**Step 6: Run tests**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/config
git commit -m "feat(config): add portable runtime env schema"
```

---

### Task 3: Change Runtime Loading to `./config/runtime.env`

**Files:**
- Modify: `internal/config/load.go`
- Modify: `internal/config/load_test.go`
- Modify: `.env.example`
- Modify: repository files containing `PIC_GALLERY_ENV_FILE`
- Test: `web/shared/prompt-optimization-deployment.contract.ts`

**Step 1: Write failing loading-contract tests**

Require:

- default file is `./config/runtime.env` relative to process working directory;
- `APP_ENV_FILE` overrides the default;
- `PIC_GALLERY_ENV_FILE` is ignored/removed;
- process env does not silently override setup-managed runtime fields;
- explicit test helpers can still load a supplied path;
- bootstrap loading tolerates incomplete pending files;
- full loading validates all required values.

**Step 2: Run tests and confirm failure**

```bash
go test ./internal/config -run 'TestLoadEnv|TestBootstrap'
```

Expected: FAIL against current `.env` and `PIC_GALLERY_ENV_FILE` behavior.

**Step 3: Split bootstrap and complete loading**

Introduce explicit APIs:

```go
func LoadBootstrap(path string) (BootstrapConfig, error)
func LoadRuntime(path string) (Config, error)
func DefaultRuntimeEnvPath() string
```

`LoadBootstrap` reads deployment/setup state without requiring database or Redis. `LoadRuntime` requires the mode/profile/role-specific schema matrix and applies application defaults only after setup completion.

**Step 4: Remove branded path selector usage**

Delete `PIC_GALLERY_ENV_FILE` from Go, service scripts, packaged scripts, documentation contracts, and examples. Retain only optional `APP_ENV_FILE`.

**Step 5: Run focused and repository contracts**

```bash
go test ./internal/config
./scripts/workflow/verify-contracts.sh
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/config .env.example scripts deployments web/shared
git commit -m "refactor(config): standardize runtime env loading"
```

---

### Task 4: Persist Fail-Closed Installation State

**Files:**
- Create: `internal/setup/state.go`
- Create: `internal/setup/state_store.go`
- Create: `internal/setup/state_store_test.go`
- Create: `internal/setup/mode.go`
- Create: `internal/setup/mode_test.go`

**Step 1: Write failing state-store tests**

Define a non-secret state file:

```go
type InstallState struct {
    SchemaVersion  int       `json:"schema_version"`
    InstallationID string    `json:"installation_id"`
    DeploymentRole string    `json:"deployment_role"`
    Phase           string    `json:"phase"`
    EverCompleted   bool      `json:"ever_completed"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

Test atomic writes, corrupt JSON, mismatched installation ID, pending, completed, and completed-with-missing-env behavior.
Also test a `committing` journal before and after the runtime env rename so restart can reconcile the two-file commit without reopening setup or forcing a completed installation into an unrecoverable state.

**Step 2: Run and confirm failure**

```bash
go test ./internal/setup
```

Expected: FAIL because package does not exist.

**Step 3: Implement state persistence and mode resolution**

Expose:

```go
type StartupMode string
const (
    StartupModeSetup  StartupMode = "setup"
    StartupModeNormal StartupMode = "normal"
    StartupModeBroken StartupMode = "broken"
)

func ResolveStartupMode(bootstrap config.BootstrapConfig, state InstallState, stateExists bool) (StartupMode, error)
```

Rules:

- never-completed + incomplete env -> setup;
- completed marker + complete env + matching installation ID -> normal;
- ever-completed + missing/corrupt/inconsistent env -> broken;
- joined non-control roles can never enter setup.

**Step 4: Run tests**

```bash
go test ./internal/setup
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/setup
git commit -m "feat(setup): add fail-closed installation state"
```

---

### Task 5: Make Database Migration an Explicit Control Operation

**Files:**
- Create: `internal/repository/ent/schema/installation.go`
- Create: `internal/repository/ent/schema/clusternode.go`
- Create: `internal/repository/ent/schema/clustertoken.go`
- Create: `internal/repository/db/migrate.go`
- Create: `internal/repository/db/migrate_test.go`
- Modify: `internal/repository/ent/schema/schema_test.go`
- Modify: generated files under `internal/repository/ent/`
- Modify: `internal/app/run.go`
- Modify: `internal/app/worker.go`

**Step 1: Write failing migration lifecycle tests**

Assert:

- migration holds a PostgreSQL advisory lock;
- concurrent migration callers serialize;
- an installation row records installation ID, config schema, database schema, and application version;
- normal API/Worker startup checks compatibility without calling Ent schema creation;
- incompatible schema leaves the node unready;
- migration is idempotent.

**Step 2: Run focused tests and confirm failure**

```bash
go test ./internal/repository/db ./internal/repository/ent/schema ./internal/app
```

Expected: new lifecycle assertions fail.

**Step 3: Add Ent schemas and generate code**

Installation has a unique singleton key and installation ID. Cluster node records node ID, role, version, config revision, health, and last heartbeat. Cluster token stores only token ID, hash, role, expiry, consumed timestamp, and audit actor.

Run:

```bash
go generate ./internal/repository/ent
```

Expected: generated clients/builders compile.

**Step 4: Implement explicit migrator**

Expose:

```go
type MigrationRequest struct {
    InstallationID string
    AppVersion      string
    ConfigVersion   int
}

func Migrate(ctx context.Context, databaseURL string, req MigrationRequest) (MigrationResult, error)
func CheckSchemaCompatibility(ctx context.Context, client *ent.Client, expected SchemaVersion) error
```

Move `PrepareLegacyData`, `Schema.Create`, and required backfills behind `Migrate`. Remove these mutations from ordinary `Run` and `RunWorker`.

**Step 5: Run tests**

```bash
go test ./internal/repository/db ./internal/repository/ent/schema ./internal/app
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/repository internal/app
git commit -m "refactor(db): make schema migration explicit"
```

---

### Task 6: Implement Setup Token Authentication and Recovery

**Files:**
- Create: `internal/setup/auth.go`
- Create: `internal/setup/auth_test.go`
- Create: `internal/setup/session.go`
- Create: `internal/setup/session_test.go`
- Modify: `internal/config/runtime_schema.go`

**Step 1: Write failing token/session tests**

Cover:

- 256-bit token generation;
- constant-time verification;
- token remains usable until completion;
- short-lived HttpOnly SameSite Strict session;
- Secure cookie only on HTTPS;
- in-memory IP/token rate limiting;
- reset invalidates all current sessions;
- completion invalidates token and sessions permanently;
- logs/errors contain no token value.

**Step 2: Run and confirm failure**

```bash
go test ./internal/setup -run 'TestToken|TestSession|TestRateLimit'
```

Expected: FAIL.

**Step 3: Implement the auth service**

Use only standard-library cryptography. Store setup token plaintext only in the protected env file because the operator must be able to display it locally; store derived session signing state in memory. Include a token generation/version value so reset invalidates existing sessions.

**Step 4: Run tests**

```bash
go test ./internal/setup
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/setup internal/config/runtime_schema.go
git commit -m "feat(setup): add recoverable setup token sessions"
```

---

### Task 7: Add Middleware Probe Services

**Files:**
- Create: `internal/setup/probe.go`
- Create: `internal/setup/probe_test.go`
- Create: `internal/setup/postgres_probe.go`
- Create: `internal/setup/redis_probe.go`
- Create: `internal/setup/storage_probe.go`
- Test: `internal/setup/probe_integration_test.go`

**Step 1: Write failing unit tests**

Define sanitized results:

```go
type ProbeResult struct {
    Kind      string `json:"kind"`
    Success   bool   `json:"success"`
    Code      string `json:"code"`
    Message   string `json:"message"`
    LatencyMS int64  `json:"latency_ms"`
    Version   string `json:"version,omitempty"`
}
```

Require timeouts, validation before dialing, no credentials in messages, and stable error codes.

**Step 2: Write integration tests**

Use repository test PostgreSQL/Redis and the existing storage abstractions to cover:

- valid PostgreSQL connection/version/schema privileges;
- invalid credentials and missing privileges;
- Redis ping/auth;
- local-directory create/write/read/delete;
- S3 bucket put/get/delete probe;
- cleanup after partial failure.

**Step 3: Run tests and confirm failure**

```bash
go test ./internal/setup -run Probe
```

Expected: FAIL.

**Step 4: Implement probes**

Probes consume submitted draft values and do not mutate final runtime config. PostgreSQL checks the target database, not server-level create-database privileges.

**Step 5: Run tests**

```bash
go test ./internal/setup -run Probe
```

Expected: PASS or documented integration skips when external test endpoints are absent.

**Step 6: Commit**

```bash
git add internal/setup
git commit -m "feat(setup): probe deployment middleware"
```

---

### Task 8: Implement the Idempotent Setup Apply Orchestrator

**Files:**
- Create: `internal/setup/service.go`
- Create: `internal/setup/service_test.go`
- Create: `internal/setup/store.go`
- Create: `internal/repository/entstore/setup_store.go`
- Create: `internal/repository/entstore/setup_store_test.go`
- Modify: `internal/service/adminauth/`

**Step 1: Write failing state-machine tests**

Cover every transition:

```text
pending -> validating -> initializing_database -> creating_admin
        -> committing_config -> restart_pending -> complete
```

Inject failures after each phase. Assert no completion marker is written early, retry is safe, concurrent apply is rejected or joins the existing operation, and plaintext administrator password is never persisted.

**Step 2: Run tests and confirm failure**

```bash
go test ./internal/setup ./internal/repository/entstore -run Setup
```

Expected: FAIL.

**Step 3: Implement setup transaction/store**

The store transaction creates or resumes the singleton installation and first administrator. It accepts an operation ID and returns the existing result for retries. Only a pending installation may set/reset the first administrator.

**Step 4: Implement apply orchestration**

Expose:

```go
type ApplyRequest struct {
    OperationID string
    Runtime     map[string]string
    AdminEmail  string
    AdminPassword string
}

func (s *Service) Apply(ctx context.Context, req ApplyRequest) (OperationView, error)
func (s *Service) Progress(ctx context.Context, id string) (OperationView, error)
```

Order: schema validation, all probes, explicit migration, installation/admin transaction, install-state commit journal, env atomic commit, install-state completion, restart-pending response. Reconcile a surviving commit journal against the database installation record on restart.

**Step 5: Run focused tests**

```bash
go test ./internal/setup ./internal/repository/entstore
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/setup internal/repository/entstore internal/service/adminauth
git commit -m "feat(setup): initialize runtime and administrator"
```

---

### Task 9: Add Setup-Only and Normal Router Factories

**Files:**
- Create: `internal/http/router/setup.go`
- Create: `internal/http/router/setup_test.go`
- Create: `internal/http/handlers/setup.go`
- Create: `internal/http/handlers/setup_test.go`
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/health.go` or current health handler file
- Modify: `internal/app/run.go`
- Modify: `api/openapi/openapi.yaml`
- Create/Modify: `api/openapi/components/schemas/setup.yaml`
- Modify: `api/openapi/openapi_test.go`

**Step 1: Write route absence tests**

Require:

- setup mode has health, readiness, bootstrap status, setup page, session, probes, apply, and progress only;
- every business route returns 404 in setup mode;
- normal mode has no setup page/assets/write endpoints;
- bootstrap status remains available in normal and broken modes;
- setup readiness is unavailable while liveness is healthy.

**Step 2: Run focused tests and confirm failure**

```bash
go test ./internal/http/router ./internal/http/handlers -run 'Setup|Bootstrap|Ready'
```

Expected: FAIL.

**Step 3: Add handler contracts**

Bootstrap status returns no secrets:

```json
{
  "phase": "setup_required",
  "setup_url": "http://127.0.0.1:8080/setup",
  "operation_id": "",
  "retry_after_seconds": 2
}
```

Generate `setup_url` from trusted forwarded-host configuration/request data without assuming user/admin frontend domains.

**Step 4: Refactor app startup**

`app.Run` loads bootstrap state, chooses mode, constructs only setup dependencies in setup mode, and constructs DB/Redis/business services only in normal mode. Broken mode exposes liveness/bootstrap diagnostics but never setup writes or business routes.

**Step 5: Update OpenAPI and tests**

Document public status and pending-only setup endpoints without exposing secret examples. Assert all request secrets are `writeOnly`.

**Step 6: Run tests**

```bash
go test ./internal/http/router ./internal/http/handlers ./internal/app ./api/openapi
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/http internal/app api/openapi
git commit -m "feat(api): serve setup-only startup mode"
```

---

### Task 10: Build the API-Hosted Setup UI with Admin Styling

**Files:**
- Create: `internal/http/setupui/page.go`
- Create: `internal/http/setupui/page_test.go`
- Create: `internal/http/setupui/model.go`
- Create: `web/shared/admin-design-tokens.css`
- Modify: admin global stylesheet importing current admin tokens
- Test: `web/shared/setup-ui.contract.ts`

**Step 1: Extract shared admin design tokens**

Move the canonical admin color, typography, spacing, form, status, and focus variables to `web/shared/admin-design-tokens.css`. Admin continues consuming the same source without visual change.

**Step 2: Write failing setup-page contracts**

Assert the API-generated page:

- uses the shared admin token source/build artifact;
- contains Token guidance commands;
- renders middleware steps, probe buttons, administrator form, progress, and restart state;
- has no user-shell/navigation/marketing references;
- stores no token in URL or localStorage;
- calls relative `/api/setup/v1/*` endpoints;
- uses `history.back()` after ready with a direct-entry fallback;
- includes accessible labels, focus states, live regions, and mobile constraints;
- contains no external script/font/image URL.

**Step 3: Run contracts and confirm failure**

```bash
node --experimental-strip-types web/shared/setup-ui.contract.ts
go test ./internal/http/setupui
```

Expected: FAIL.

**Step 4: Implement the embedded response**

Keep the deployable UI inside the API binary. `page.go` returns a complete response assembled from embedded/generated constants; no separate setup frontend service or deployed HTML file exists. Add `Cache-Control: no-store`, CSP, `X-Content-Type-Options`, and frame denial.

The page reads configuration field metadata from a sanitized setup-schema endpoint or embedded JSON generated from `internal/config` schema so labels, required conditions, and bilingual help stay consistent.

**Step 5: Verify desktop/mobile rendering**

Use Playwright against a setup-mode API at desktop and mobile widths. Capture evidence under ignored `tmp/e2e/` and verify no overlap or clipped labels.

**Step 6: Run tests/builds**

```bash
go test ./internal/http/setupui ./internal/http/router
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
node --experimental-strip-types web/shared/setup-ui.contract.ts
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/http/setupui web/shared web/admin
git commit -m "feat(setup): embed admin-styled setup interface"
```

---

### Task 11: Redirect User and Admin Frontends to the API Setup URL

**Files:**
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/http-client.ts`
- Create: `web/shared/bootstrap-status.ts`
- Create: `web/shared/bootstrap-status.contract.ts`
- Modify: `web/user/src/App.tsx`
- Modify: `web/admin/src/App.tsx`
- Create: `web/user/src/bootstrapGuard.contract.ts`
- Create: `web/admin/src/bootstrapGuard.contract.ts`

**Step 1: Write failing shared model tests**

Model phases `setup_required`, `initializing`, `restart_pending`, `ready`, and `broken`. Require only HTTP(S) setup URLs and preserve an API-provided absolute or relative URL without inferring an admin hostname.

**Step 2: Write failing app contracts**

Require both apps to resolve bootstrap status before auth/session refresh. `setup_required` navigates with `window.location.assign(status.setup_url)`. `ready` continues current routing. `broken` renders an operational error without a login loop.

**Step 3: Run contracts and confirm failure**

```bash
node --experimental-strip-types web/shared/bootstrap-status.contract.ts
node --experimental-strip-types web/user/src/bootstrapGuard.contract.ts
node --experimental-strip-types web/admin/src/bootstrapGuard.contract.ts
```

Expected: FAIL.

**Step 4: Implement one shared guard**

Do not duplicate status interpretation. Avoid auth refresh until ready so setup mode never generates repeated 404/401 traffic.

**Step 5: Run frontend verification**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
./scripts/workflow/verify-contracts.sh
```

Expected: PASS.

**Step 6: Commit**

```bash
git add web/shared web/user web/admin
git commit -m "feat(web): redirect first-run traffic to API setup"
```

---

### Task 12: Make Worker Wait for Completion and Check Schema Compatibility

**Files:**
- Modify: `internal/app/worker.go`
- Create: `internal/app/worker_bootstrap.go`
- Create: `internal/app/worker_bootstrap_test.go`
- Modify: `cmd/worker/main.go`

**Step 1: Write failing Worker lifecycle tests**

Assert Worker:

- does not open database/Redis/storage while setup is pending;
- watches the runtime config or exits with a documented supervisor code;
- starts after completion;
- refuses setup/control behavior on joined Worker role;
- checks installation ID and schema/app compatibility;
- never runs migration;
- logs no secret values.

**Step 2: Run and confirm failure**

```bash
go test ./internal/app -run WorkerBootstrap
```

Expected: FAIL.

**Step 3: Implement wait/start behavior**

Prefer a bounded file watcher/poll loop with cancellation so Docker does not crash-loop during browser setup. After completion, load full runtime config and construct the existing runner.

**Step 4: Run tests**

```bash
go test ./internal/app ./internal/worker
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/app cmd/worker
git commit -m "feat(worker): wait for initialized runtime"
```

---

### Task 13: Create the Cross-Platform Deployctl Foundation

**Files:**
- Create: `cmd/deployctl/main.go`
- Create: `internal/deployctl/command.go`
- Create: `internal/deployctl/command_test.go`
- Create: `internal/deployctl/install.go`
- Create: `internal/deployctl/install_test.go`
- Create: `internal/deployctl/components.go`
- Create: `internal/deployctl/components_test.go`
- Create: `internal/deployctl/runtime.go`
- Create: `scripts/install.sh`
- Create: `scripts/install.ps1`

**Step 1: Write failing CLI parser tests**

Cover interactive and non-interactive forms for:

```text
install
status
doctor
restart
upgrade
uninstall
setup status
setup token show
setup token reset
cluster token create
cluster join
```

Use an injected terminal/filesystem/process runner. Tests must never call real systemd, Docker, or Windows services.

**Step 2: Write failing component dependency tests**

Test every approved preset and rejection rule, including native full, multi-node local storage, uninitialized Worker join, missing Web API URL, and migration attempts by non-control nodes.

**Step 3: Run and confirm failure**

```bash
go test ./internal/deployctl
```

Expected: FAIL.

**Step 4: Implement command and install planning layers**

Separate pure planning from side effects:

```go
type InstallPlan struct {
    Mode       config.DeploymentMode
    Profile    config.DeploymentProfile
    Role       config.DeploymentRole
    Components []Component
    RuntimeDir string
}

func BuildInstallPlan(Input) (InstallPlan, error)
func ExecuteInstall(ctx context.Context, plan InstallPlan, deps Dependencies) error
```

Generate secrets, setup token, bilingual runtime env skeleton, install state, and deployment manifest without requiring manual env editing.

**Step 5: Implement thin bootstrap wrappers**

Shell/PowerShell wrappers only locate/download a release-compatible `deployctl` and pass arguments. They contain no duplicated deployment policy.

**Step 6: Run tests and cross-build**

```bash
go test ./internal/deployctl ./cmd/deployctl
GOOS=linux GOARCH=amd64 go build ./cmd/deployctl
GOOS=windows GOARCH=amd64 go build ./cmd/deployctl
```

Expected: PASS.

**Step 7: Commit**

```bash
git add cmd/deployctl internal/deployctl scripts/install.sh scripts/install.ps1
git commit -m "feat(deploy): add cross-platform deployment CLI"
```

---

### Task 14: Refactor Docker Compose into Full and Core Profiles

**Files:**
- Modify: `deployments/docker-compose/docker-compose.prod.yml`
- Replace/Modify: `deployments/docker-compose/.env.prod.example`
- Modify: `deployments/docker-compose/prepare.sh`
- Create: `deployments/docker-compose/minio-init.sh`
- Modify: `deployments/nginx/default.conf`
- Create: `web/shared/deployment-profiles.contract.ts`
- Modify: `internal/deployctl/install.go`

**Step 1: Write failing Compose contracts**

Assert:

- full includes PostgreSQL, authenticated Redis, MinIO, MinIO bucket init, API, Worker, three Web services, and Gateway;
- core excludes PostgreSQL/Redis/MinIO;
- API mounts `./config` read-write and Worker read-only;
- application runtime values are not duplicated in Compose `environment`;
- services use the fixed working directory/default env path;
- Gateway starts from API liveness, not business readiness;
- persistent deletion requires explicit deployctl confirmation;
- setup URL and bootstrap status route correctly.

**Step 2: Run contract and confirm failure**

```bash
node --experimental-strip-types web/shared/deployment-profiles.contract.ts
docker compose -f deployments/docker-compose/docker-compose.prod.yml config -q
```

Expected: contract FAIL against current stack.

**Step 3: Implement profiles/templates**

Use Compose profiles or deployctl-generated overrides without duplicating service definitions. Full managed credentials are generated into runtime env before Compose starts. MinIO bucket init must be idempotent.

**Step 4: Add deployctl Docker executor tests**

Use a fake process runner to assert exact `docker compose` arguments for full/core/custom, update, restart, status, and non-destructive uninstall.

**Step 5: Run Compose/contracts**

```bash
docker compose -f deployments/docker-compose/docker-compose.prod.yml config -q
go test ./internal/deployctl -run Docker
node --experimental-strip-types web/shared/deployment-profiles.contract.ts
```

Expected: PASS.

**Step 6: Commit**

```bash
git add deployments/docker-compose deployments/nginx internal/deployctl web/shared/deployment-profiles.contract.ts
git commit -m "feat(docker): add full and core deployment profiles"
```

---

### Task 15: Add the Portable Native Gateway and Service Installation

**Files:**
- Create: `cmd/gateway/main.go`
- Create: `internal/gateway/server.go`
- Create: `internal/gateway/server_test.go`
- Create: `internal/deployctl/native.go`
- Create: `internal/deployctl/native_test.go`
- Modify: `scripts/devops/package.sh`
- Modify: `scripts/service/manage.sh`
- Modify: `scripts/service/manage.ps1`
- Modify: `deployments/devops/`

**Step 1: Write failing Gateway tests**

Require static user/admin/docs routing, SPA fallback, runtime frontend config, API reverse proxy, bootstrap/setup forwarding, health checks, traversal prevention, cache headers, and configurable public ports. No TLS/domain management is included.

**Step 2: Write failing Linux/Windows service-plan tests**

Assert systemd and Windows service definitions set the runtime working directory, start API/Worker/Gateway in dependency order, restart on the documented API restart code, and use no `PIC_GALLERY_ENV_FILE`.

**Step 3: Run and confirm failure**

```bash
go test ./internal/gateway ./internal/deployctl -run Native
```

Expected: FAIL.

**Step 4: Implement Gateway**

Serve packaged frontend artifacts and reverse proxy configured API prefixes. Use standard library where practical. Keep frontend runtime config public and separate from backend `runtime.env` secrets.

**Step 5: Implement native install executor**

Linux uses systemd. Windows uses a real long-running service integration rather than the current development Task Scheduler behavior. Keep macOS development scripts working but exclude macOS from production one-command acceptance.

**Step 6: Package and cross-build**

```bash
./scripts/devops/package.sh all
GOOS=linux GOARCH=amd64 go build ./cmd/api ./cmd/worker ./cmd/gateway ./cmd/deployctl
GOOS=windows GOARCH=amd64 go build ./cmd/api ./cmd/worker ./cmd/gateway ./cmd/deployctl
go test ./internal/gateway ./internal/deployctl
```

Expected: PASS.

**Step 7: Commit**

```bash
git add cmd/gateway internal/gateway internal/deployctl scripts deployments/devops
git commit -m "feat(native): add portable gateway and services"
```

---

### Task 16: Implement Cluster Token Persistence and Admin APIs

**Files:**
- Create: `internal/domain/cluster/types.go`
- Create: `internal/service/cluster/service.go`
- Create: `internal/service/cluster/service_test.go`
- Create: `internal/repository/entstore/cluster_store.go`
- Create: `internal/repository/entstore/cluster_store_test.go`
- Create: `internal/http/handlers/cluster.go`
- Create: `internal/http/router/cluster_api_test.go`
- Modify: `internal/http/router/router.go`
- Modify: `api/openapi/openapi.yaml`
- Create/Modify: `api/openapi/components/schemas/cluster.yaml`

**Step 1: Write failing service/store tests**

Cover token hash-only persistence, role, expiry, one-time consumption, admin actor, installation binding, node uniqueness, heartbeat updates, version/config revision, and audit events.

**Step 2: Run and confirm failure**

```bash
go test ./internal/service/cluster ./internal/repository/entstore ./internal/http/router -run Cluster
```

Expected: FAIL.

**Step 3: Implement cluster service**

Expose admin-protected token creation/list/revoke and public challenge/join endpoints. Only initialized control installations issue tokens. Never return stored token hashes.

**Step 4: Add OpenAPI contracts**

Mark join/token secret request fields write-only. Document role-specific responses as encrypted envelopes, not plaintext runtime config.

**Step 5: Run tests**

```bash
go test ./internal/service/cluster ./internal/repository/entstore ./internal/http/router ./api/openapi
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/domain/cluster internal/service/cluster internal/repository/entstore internal/http api/openapi
git commit -m "feat(cluster): issue role-scoped join tokens"
```

---

### Task 17: Encrypt Enrollment and Implement `deployctl cluster join`

**Files:**
- Create: `internal/service/cluster/crypto.go`
- Create: `internal/service/cluster/crypto_test.go`
- Create: `internal/deployctl/cluster.go`
- Create: `internal/deployctl/cluster_test.go`
- Modify: `internal/http/handlers/cluster.go`
- Test: `internal/http/router/cluster_api_test.go`

**Step 1: Write failing protocol tests**

Test challenge freshness, possession proof, ephemeral key exchange, role/installation/version binding, AEAD authentication, replay rejection, wrong-token failure, ciphertext tampering, expiry, and absence of plaintext secrets from HTTP bodies.

Use fixed deterministic test vectors for protocol interoperability.

**Step 2: Run and confirm failure**

```bash
go test ./internal/service/cluster ./internal/deployctl -run 'Crypto|Join|Envelope'
```

Expected: FAIL.

**Step 3: Implement protocol**

Use audited Go standard-library/x-crypto primitives. Token format separates a public token ID from a high-entropy secret. Derive proof and envelope keys with domain-separated HKDF inputs. Bind ciphertext to installation ID, role, node public key, expiry, and challenge ID.

**Step 4: Implement role-minimized configuration**

Create explicit allowlists:

```go
func RuntimeKeysForRole(role DeploymentRole) []string
```

Worker must not receive admin/setup/cashier-only values; Web receives no backend secrets.

**Step 5: Implement deployctl join**

Validate remote status/version, complete challenge, decrypt to memory, validate the received schema, atomically write local runtime/install-state, install the selected role, and consume the token only after server-side acceptance.

**Step 6: Run tests**

```bash
go test ./internal/service/cluster ./internal/deployctl ./internal/http/router -run Cluster
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/cluster internal/deployctl internal/http
git commit -m "feat(cluster): securely enroll application nodes"
```

---

### Task 18: Add Cluster Runtime Health and Admin Visibility

**Files:**
- Create: `internal/service/cluster/heartbeat.go`
- Modify: `internal/app/run.go`
- Modify: `internal/app/worker.go`
- Create: `web/admin/src/pages/ClusterPage.tsx`
- Create: `web/admin/src/pages/clusterRows.ts`
- Create: `web/admin/src/pages/clusterRows.contract.ts`
- Modify: `web/admin/src/layout/admin-navigation.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/shared/api-types.ts`

**Step 1: Write failing heartbeat tests**

Require API/Worker nodes to report node ID, role, version, config revision, schema version, state, and last error without secrets. Wrong installation IDs or incompatible schemas keep nodes unready.

**Step 2: Write failing admin UI contracts**

Require the page to show role, health, last heartbeat, version/config drift, and actionable status. It must not imply that the project configures the external load balancer.

**Step 3: Run and confirm failure**

```bash
go test ./internal/service/cluster ./internal/app -run Heartbeat
node --experimental-strip-types web/admin/src/pages/clusterRows.contract.ts
```

Expected: FAIL.

**Step 4: Implement runtime registration/heartbeat**

Start only after normal config loads. Worker heartbeat can share lifecycle context with the task runner but remains distinct from per-task leases.

**Step 5: Implement admin view**

Use existing admin design system and permissions. Include no remote shell controls in the first release.

**Step 6: Run tests/builds**

```bash
go test ./internal/service/cluster ./internal/app
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
./scripts/workflow/verify-contracts.sh
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/service/cluster internal/app web/admin web/shared
git commit -m "feat(cluster): report application node health"
```

---

### Task 19: Implement Config Import, Upgrade, Doctor, and Safe Uninstall

**Files:**
- Create: `internal/deployctl/import_config.go`
- Create: `internal/deployctl/import_config_test.go`
- Create: `internal/deployctl/doctor.go`
- Create: `internal/deployctl/doctor_test.go`
- Create: `internal/deployctl/upgrade.go`
- Create: `internal/deployctl/upgrade_test.go`
- Modify: `internal/deployctl/command.go`
- Modify: current deployment documentation and scripts

**Step 1: Write failing import tests**

Cover current `.env`, `.env.prod`, and packaged `backend.env` input. Assert values map to the new schema, missing secrets are generated, source files remain untouched, bilingual comments render, and completion is inferred only after middleware/installation/admin checks.

**Step 2: Write failing operational-command tests**

Require:

- `doctor` identifies missing fields, permission errors, installation mismatch, middleware failure, and schema drift without printing secrets;
- `upgrade` migrates once on control and rolls services in safe order;
- non-control upgrade refuses migration;
- uninstall preserves data/config by default;
- destructive delete requires the exact generated confirmation phrase;
- completed setup token show/reset is refused.

**Step 3: Run and confirm failure**

```bash
go test ./internal/deployctl -run 'Import|Doctor|Upgrade|Uninstall'
```

Expected: FAIL.

**Step 4: Implement commands**

Keep deployment actions behind injected process/filesystem interfaces for deterministic tests. Print redacted summaries only.

**Step 5: Run tests**

```bash
go test ./internal/deployctl
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/deployctl scripts deployments docs
git commit -m "feat(deploy): migrate and operate installations safely"
```

---

### Task 20: Add Fresh-Install and Cluster E2E Coverage

**Files:**
- Create: `scripts/e2e/setup-docker-e2e.sh`
- Create: `scripts/e2e/setup-browser.py`
- Create: `scripts/e2e/cluster-docker-e2e.sh`
- Modify: `scripts/e2e/docker-e2e.mjs`
- Modify: `scripts/workflow/verify.sh`
- Create: `web/shared/setup-deployment-e2e.contract.ts`
- Modify: `docs/runbooks/backend-deployment.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Step 1: Write the E2E safety contract first**

Require setup E2E to use isolated project names/volumes, never touch the shared local dev database, clean only its own resources, redact generated secrets, and retain failure evidence.

**Step 2: Implement Docker full fresh-install E2E**

Test:

1. Empty runtime directory and one install command.
2. Setup-only route surface and business-route absence.
3. Missing-token guidance, token show, token reset, and old-token rejection.
4. Managed PostgreSQL/Redis/MinIO probes.
5. Administrator creation and apply.
6. API restart/countdown/readiness.
7. Setup route closure and administrator login.
8. Provider/model/route/price/plan/registration/payment configuration.
9. User registration, login, recharge, image task, history, and public gallery.

**Step 3: Implement Docker core E2E**

Run the same API setup flow with external test PostgreSQL, Redis, and MinIO. Assert editable rather than managed/read-only fields.

**Step 4: Implement cluster E2E**

Start control core services, join a second API and two Workers, then test:

- role-minimized env files;
- token expiry/replay/version mismatch;
- API replica readiness;
- external test proxy distribution;
- exactly-once task claim and settlement;
- Worker termination followed by lease recovery on another node;
- no plaintext secret in captured HTTP enrollment bodies.

**Step 5: Implement API-hosted UI browser checks**

Verify desktop/mobile admin styling, field comments/help, independent probes, error states, progress, restart, browser-history return, and absence of calls to user/admin frontend assets.

**Step 6: Add packaging checks**

Cross-build and inspect Linux/Windows release bundles. Where the CI host cannot run Windows services, keep service-definition unit tests mandatory and document the manual Windows acceptance environment.

**Step 7: Run the full acceptance suite**

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
./scripts/e2e/setup-docker-e2e.sh
./scripts/e2e/cluster-docker-e2e.sh
./scripts/e2e/run-docker-e2e.sh
```

Expected: all PASS; every setup E2E reports successful restart and normal route mode; shared dev E2E reports state restore verified.

**Step 8: Update user/operator documentation**

Document the deployment matrix, prerequisites, one-command examples, HTTP warning, runtime directory, bilingual env fields, Token recovery, external middleware privileges, cluster join, external load balancer boundary, upgrade, backup, and non-destructive uninstall.

**Step 9: Commit**

```bash
git add scripts web/shared docs README.md README.zh-CN.md
git commit -m "test(deploy): cover setup and cluster workflows"
```

---

### Task 21: Final Review and Delivery

**Files:**
- Review: all committed changes relative to the approved design
- Generated marker: `.review/gate.json`

**Step 1: Audit the approved design requirement by requirement**

Verify current files and runtime evidence for every deployment mode, setup state, Token path, UI behavior, role boundary, failure mode, and E2E acceptance item. Missing evidence is incomplete work.

**Step 2: Run repository delivery workflow**

Use:

```text
@dev-verify
@dev-api-smoke
@dev-review-gate
@dev-ship
@superpowers:verification-before-completion
@superpowers:requesting-code-review
```

Run the final ship command required by those skills and ensure the review marker matches the current committed tree.

**Step 3: Perform independent code review**

Review security boundaries first: setup reopening, env permissions, secret redaction, HTTP enrollment cryptography, migration concurrency, Worker exactly-once behavior, open redirects, and destructive deployment commands.

Expected: no P1/P2 or merge blocker.

**Step 4: Rebuild the final Docker full environment from the final branch**

Use a clean runtime directory and fresh volumes. Complete setup and confirm every service is healthy, normal `/readyz` is ready, setup writes are absent, and admin login works.

**Step 5: Commit any review fixes and rerun all affected gates**

No test or review result from before the final fix may be reused as final evidence.

**Step 6: Merge only after every acceptance requirement is proven**

Use the repository branch policy. Do not push unless explicitly authorized.
