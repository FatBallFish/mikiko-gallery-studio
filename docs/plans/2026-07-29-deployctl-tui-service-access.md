# Deployctl TUI and Service Access Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a complete deployctl TUI and post-install service summary while making every published Docker frontend endpoint usable before and after Setup.

**Architecture:** Keep `ParseCommand` and `Run` as the only execution authority. A shared command catalog feeds help and a Bubble Tea argument-builder TUI, while installation summaries derive from the finalized plan. Frontend images use shared runtime API resolution and relocatable assets so direct ports and Gateway prefixes share one artifact.

**Tech Stack:** Go 1.26, Bubble Tea/Bubbles, TypeScript, React/Vite, Nginx, Docker Compose, Bash contracts, Playwright Docker E2E.

---

### Task 1: Establish Help and Command Catalog

**Files:**
- Create: `internal/deployctl/catalog.go`
- Create: `internal/deployctl/catalog_test.go`
- Modify: `internal/deployctl/command.go`
- Modify: `internal/deployctl/command_test.go`
- Modify: `internal/deployctl/cli.go`
- Modify: `internal/deployctl/cli_test.go`

**Step 1: Write failing tests**

Add `TestCommandCatalogCoversApprovedParserTree` with the 14 paths from the requirement document. Add `TestRunHelpAndNonTTYNoArgsExitSuccessfully` for no arguments, `-h`, and `--help`:

```go
stdout := new(bytes.Buffer)
code := Run(context.Background(), []string{"--help"}, CLIDependencies{
    Terminal: &fakeTerminal{interactive: false}, Stdout: stdout, Stderr: new(bytes.Buffer),
})
if code != 0 || !strings.Contains(stdout.String(), "deployctl install") { t.Fatal(stdout.String()) }
```

Keep unknown commands on the existing usage-error path.

**Step 2: Verify RED**

Run: `go test ./internal/deployctl -run 'Test(CommandCatalog|RunHelp)' -count=1`

Expected: FAIL because catalog/help behavior is absent.

**Step 3: Implement minimal behavior**

Define immutable entries containing path, group, summary, and usage. Add `HelpText()`. Before `ParseCommand`, route top-level help flags and non-TTY no-argument calls to help with exit code `0`. Do not change any other command flags or exit codes.

**Step 4: Verify GREEN**

Run `gofmt` on changed Go files, then `go test ./internal/deployctl`.

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/deployctl/catalog.go internal/deployctl/catalog_test.go internal/deployctl/command.go internal/deployctl/command_test.go internal/deployctl/cli.go internal/deployctl/cli_test.go
git commit -m "feat(deployctl): add shared command help catalog"
```

### Task 2: Add the TUI Navigation State Machine

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/deployctl/tui.go`
- Create: `internal/deployctl/tui_test.go`
- Modify: `internal/deployctl/cli.go`
- Modify: `internal/deployctl/cli_test.go`
- Modify: `cmd/deployctl/main.go`

**Step 1: Write failing tests**

Pure-model tests must cover number and arrow selection, Enter, Escape, root Exit, Ctrl+C, and cancellation. Add a dispatch test:

```go
called := 0
code := Run(context.Background(), nil, CLIDependencies{
    Terminal: &fakeTerminal{interactive: true},
    ExecuteTUI: func(context.Context) ([]string, error) { called++; return nil, nil },
    Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer),
})
if code != 0 || called != 1 { t.Fatalf("code=%d called=%d", code, called) }
```

**Step 2: Verify RED**

Run: `go test ./internal/deployctl -run 'Test(TUIRoot|RunNoArgsUsesTUI)' -count=1`

Expected: FAIL because TUI types and injection do not exist.

**Step 3: Implement minimal navigation**

Add compatible Bubble Tea and Bubbles modules with `go get`. Implement a pure root/submenu model plus production alternate-screen adapter. TUI returns `nil` for exit or an in-memory argument slice; `Run` executes returned arguments only after terminal teardown. Never spawn deployctl recursively.

**Step 4: Verify GREEN**

Run `gofmt`, `go test ./internal/deployctl ./cmd/deployctl`, and confirm unit tests emit no terminal escape sequences.

**Step 5: Commit**

```bash
git add go.mod go.sum internal/deployctl/tui.go internal/deployctl/tui_test.go internal/deployctl/cli.go internal/deployctl/cli_test.go cmd/deployctl/main.go
git commit -m "feat(deployctl): open keyboard TUI without arguments"
```

### Task 3: Build TUI Forms for Every Command

**Files:**
- Create: `internal/deployctl/tui_fields.go`
- Create: `internal/deployctl/tui_fields_test.go`
- Modify: `internal/deployctl/tui.go`
- Modify: `internal/deployctl/tui_test.go`
- Modify: `internal/deployctl/catalog.go`

**Step 1: Write failing round-trip tests**

Create one fixture per catalog command. Drive the model, obtain arguments, and require `ParseCommand(args)` to accept them with the expected kind. Separately assert Space toggles custom components, Tab/Shift+Tab move fields, sensitive fields render bullets, review output uses `<redacted>`, invalid input retains focus, and generic confirmation cannot authorize persistent deletion.

**Step 2: Verify RED**

Run: `go test ./internal/deployctl -run 'TestTUI(CommandBuilders|Sensitive|Destructive|MultiSelect)' -count=1`

Expected: FAIL because fields/builders do not exist.

**Step 3: Implement minimal forms**

Add typed single-select, multi-select, text, masked text, boolean, duration, and exact-confirmation fields. Keep conditional install fields aligned with `BuildInstallPlan`. Build only existing flags. The final page displays a safe equivalent command and returns arguments after Enter.

**Step 4: Verify GREEN**

Run `gofmt` and `go test ./internal/deployctl`.

Expected: all 14 catalog paths have a passing TUI round trip.

**Step 5: Commit**

```bash
git add internal/deployctl/tui.go internal/deployctl/tui_test.go internal/deployctl/tui_fields.go internal/deployctl/tui_fields_test.go internal/deployctl/catalog.go
git commit -m "feat(deployctl): cover all operations in TUI forms"
```

### Task 4: Render Secure Installation Summary

**Files:**
- Create: `internal/deployctl/install_summary.go`
- Create: `internal/deployctl/install_summary_test.go`
- Modify: `internal/deployctl/cli.go`
- Modify: `internal/deployctl/cli_test.go`

**Step 1: Write failing matrix tests**

Build Docker/native and full/core/custom plans. Assert summaries contain only selected components. A full Docker case must contain Setup, Gateway, direct user/admin/docs, API, PostgreSQL, Redis, MinIO, remote-host guidance, and numbered next steps. Non-interactive output must exclude the token and contain the exact quoted token-show command.

**Step 2: Verify RED**

Run: `go test ./internal/deployctl -run TestInstallSummary -count=1`

Expected: FAIL because renderer is absent.

**Step 3: Implement minimal renderer**

Derive endpoints and access scope from `InstallPlan.Components`, mode, role, ports, and public API URL. Preserve the existing terminal-versus-redirect token policy. Replace the current two-line completion output only after successful install.

**Step 4: Verify GREEN**

Run `gofmt` and `go test ./internal/deployctl`.

**Step 5: Commit**

```bash
git add internal/deployctl/install_summary.go internal/deployctl/install_summary_test.go internal/deployctl/cli.go internal/deployctl/cli_test.go
git commit -m "feat(deployctl): print post-install service access summary"
```

### Task 5: Add Shared Browser Runtime API Resolution

**Files:**
- Create: `web/shared/runtime-config.ts`
- Create: `web/shared/runtime-config.contract.ts`
- Modify: `web/shared/http-client.ts`
- Modify: `web/shared/bootstrap-status.ts`
- Modify: `web/shared/bootstrap-status.contract.ts`
- Modify: `web/user/src/bootstrapGuard.ts`
- Modify: `web/admin/src/bootstrapGuard.ts`
- Modify: `web/user/src/App.tsx`
- Modify: `web/admin/src/App.tsx`

**Step 1: Write failing contracts**

Require direct `http://10.0.0.8:5173/` plus API port `8080` to resolve to `http://10.0.0.8:8080`; Gateway `http://10.0.0.8/` must resolve to same-origin empty base; explicit `https://api.example.test/base` must win. Assert `/setup` resolves against the selected API origin and malformed/protocol-relative values fail safely.

**Step 2: Verify RED**

Run: `npx --yes tsx web/shared/runtime-config.contract.ts`

Expected: FAIL because runtime-config does not exist.

**Step 3: Implement shared resolution**

Extend the runtime window type with `apiPort` and `directFrontendPort`. Centralize resolution for HTTP client and bootstrap guard. Include the resolved non-secret endpoint and doctor guidance in failure view models.

**Step 4: Verify GREEN**

```bash
npx --yes tsx web/shared/runtime-config.contract.ts
npx --yes tsx web/shared/bootstrap-status.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
```

Expected: PASS.

**Step 5: Commit**

```bash
git add web/shared/runtime-config.ts web/shared/runtime-config.contract.ts web/shared/http-client.ts web/shared/bootstrap-status.ts web/shared/bootstrap-status.contract.ts web/user/src/bootstrapGuard.ts web/admin/src/bootstrapGuard.ts web/user/src/App.tsx web/admin/src/App.tsx
git commit -m "fix(web): resolve API correctly on direct frontend ports"
```

### Task 6: Make Frontend Artifacts Relocatable

**Files:**
- Modify: `Dockerfile.admin-web`
- Modify: `Dockerfile.docs-web`
- Modify: `web/admin/vite.config.ts`
- Modify: `web/docs/vite.config.ts`
- Modify: `web/admin/index.html`
- Modify: `web/docs/index.html`
- Modify: `deployments/nginx/frontend.conf`
- Modify: `deployments/devops/nginx-docs-web.conf`
- Modify: `deployments/devops/nginx-admin-web.conf`
- Create: `scripts/workflow/frontend-path-contract.sh`
- Modify: `scripts/workflow/verify.sh`

**Step 1: Write a failing build/path contract**

Build admin/docs and require relative `./assets/` URLs. Reject absolute `/admin/assets/` and `/developer-docs/assets/`. Inspect Nginx configuration to require real `404` behavior for missing assets, env files, OpenAPI files, and source maps.

**Step 2: Verify RED**

Run: `./scripts/workflow/frontend-path-contract.sh`

Expected: FAIL on absolute prefix assets and SPA fallback for static files.

**Step 3: Implement minimal relocation and strict static locations**

Use a relative production Vite base for admin/docs while retaining root dev behavior. Install the shared runtime renderer in the admin Docker image. Restrict SPA fallback to navigation routes.

**Step 4: Verify GREEN**

Run admin/docs builds and `./scripts/workflow/frontend-path-contract.sh`.

Expected: PASS.

**Step 5: Commit**

```bash
git add Dockerfile.admin-web Dockerfile.docs-web web/admin web/docs deployments/nginx/frontend.conf deployments/devops/nginx-docs-web.conf deployments/devops/nginx-admin-web.conf scripts/workflow/frontend-path-contract.sh scripts/workflow/verify.sh
git commit -m "fix(web): support direct and prefixed frontend paths"
```

### Task 7: Wire Runtime Values Through Docker and Native

**Files:**
- Create: `deployments/nginx/40-render-frontend-env.sh`
- Delete: `deployments/nginx/40-render-user-env.sh`
- Modify: `Dockerfile.user-web`
- Modify: `Dockerfile.admin-web`
- Modify: `deployments/docker-compose/docker-compose.prod.yml`
- Modify: `deployments/docker-compose/docker-compose.local.yml`
- Modify: `deployments/devops/start-user-web.sh`
- Modify: `deployments/devops/start-admin-web.sh`
- Modify: `internal/deployctl/runtime.go`
- Modify: `internal/deployctl/install_test.go`
- Modify: `web/shared/runtime-env-loading.contract.ts`

**Step 1: Write failing wiring tests**

Require user/admin services and native launchers to render `PUBLIC_API_URL`, `API_PORT`, and the relevant direct frontend port. Require identical runtime object keys and JavaScript escaping. Keep docs API-independent.

**Step 2: Verify RED**

```bash
go test ./internal/deployctl -run 'Test.*(Runtime|Docker)' -count=1
npx --yes tsx web/shared/runtime-env-loading.contract.ts
```

Expected: FAIL because admin Docker rendering and port metadata are missing.

**Step 3: Implement shared renderer and wiring**

Replace the user-only renderer with a shared script. Pass explicit public API, API port, and direct frontend port through Docker Compose and native frontend environments.

**Step 4: Verify GREEN**

Run `go test ./internal/deployctl`, the runtime-env contract, and frontend-path contract.

**Step 5: Commit**

```bash
git add Dockerfile.user-web Dockerfile.admin-web deployments/nginx deployments/docker-compose deployments/devops/start-user-web.sh deployments/devops/start-admin-web.sh internal/deployctl/runtime.go internal/deployctl/install_test.go web/shared/runtime-env-loading.contract.ts
git commit -m "fix(deploy): wire frontend runtime API endpoints"
```

### Task 8: Extend Clean Docker Setup E2E

**Files:**
- Modify: `scripts/e2e/setup-docker-e2e.sh`
- Modify: `scripts/e2e/setup-browser.py`
- Modify: `scripts/e2e/deployment-e2e-lib.sh`
- Modify: `scripts/e2e/run-docker-e2e.contract.sh`
- Modify: `web/shared/setup-deployment-e2e.contract.ts`

**Step 1: Add failing E2E assertions**

Before Setup, assert direct user/admin redirect to API Setup, direct docs render without redirect, Gateway equivalents work, referenced resources have correct MIME, and missing static resources return `404`. After Setup, assert restart recovery and normal user/admin entry.

**Step 2: Verify RED on current behavior**

Run: `DEPLOYMENT_E2E_PROFILES=full ./scripts/e2e/setup-docker-e2e.sh`

Expected: FAIL on direct user bootstrap and direct admin/docs asset MIME. Confirm cleanup leaves the existing developer stack/database untouched.

**Step 3: Rebuild task images and verify GREEN**

Use a unique task image tag and temporary runtime/ports. Re-run the exact E2E command.

Expected: pending Setup, completion/restart, and post-Setup assertions PASS.

**Step 4: Commit**

```bash
git add scripts/e2e/setup-docker-e2e.sh scripts/e2e/setup-browser.py scripts/e2e/deployment-e2e-lib.sh scripts/e2e/run-docker-e2e.contract.sh web/shared/setup-deployment-e2e.contract.ts
git commit -m "test(deploy): cover direct frontend Setup access"
```

### Task 9: Document and Ship

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/runbooks/backend-deployment.md`
- Modify: `scripts/workflow/deployment-docs-contract.sh`

**Step 1: Add failing docs contract requirements**

Require both READMEs to cover no-argument TUI keys, help flags, installation summary, direct ports versus Gateway paths, Setup redirect ownership, docs availability, and token-output safety.

**Step 2: Verify RED**

Run: `./scripts/workflow/deployment-docs-contract.sh`

Expected: FAIL for missing TUI/direct-port guidance.

**Step 3: Update aligned bilingual documentation and verify GREEN**

Run the docs contract and `./scripts/workflow/verify.sh`.

Expected: PASS.

**Step 4: Commit**

```bash
git add README.md README.zh-CN.md docs/runbooks/backend-deployment.md scripts/workflow/deployment-docs-contract.sh
git commit -m "docs(deploy): explain TUI and service endpoints"
```

**Step 5: Run final gates**

Load and follow `dev-review-gate`, `dev-api-smoke`, and `dev-ship`, then run:

```bash
./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
./scripts/workflow/api-smoke.sh
DEPLOYMENT_E2E_PROFILES=full ./scripts/e2e/setup-docker-e2e.sh
./scripts/workflow/ship-guard.sh
git status --short
```

Expected: all gates pass, E2E cleans task-owned resources, the existing developer database remains intact, and the worktree is clean.

**Step 6: Push and open a ready PR**

Push `codex/deployctl-tui-service-access`, create a ready PR to `main`, and verify remote head SHA, base, draft state, and required checks.
