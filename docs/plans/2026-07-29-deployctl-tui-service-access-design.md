# Deployctl TUI and Service Access Design

## Context

Deployctl already owns production installation and runtime operations, but its discoverability and first-install handoff are incomplete. The Docker deployment also publishes frontend container ports whose artifacts and API runtime configuration only work through Gateway prefixes.

Investigation against a live pending full/single deployment confirmed:

- `GET :8080/api/system/v1/bootstrap-status` returns `setup_required`, and CORS permits the user and admin direct origins;
- user `env.js` contains an empty API base, so direct access requests the frontend origin;
- admin `/admin/assets/*` and docs `/developer-docs/assets/*` return `index.html` from their direct containers;
- Gateway prefix stripping makes the same images work through `/admin/` and `/developer-docs/`.

The fix therefore belongs at the CLI presentation, frontend runtime-addressing, build-base, and static-server boundaries rather than in Setup routing or API CORS.

## Goals

- Make every existing deployctl capability discoverable through a keyboard-driven TUI.
- Preserve the current CLI parser and execution paths as the only deployment authority.
- Produce a secure, component-aware post-install access summary.
- Make each published frontend endpoint genuinely usable while preserving Gateway and external-proxy support.
- Add executable regression coverage for the real first-install workflow.

## Non-goals

- Adding deployment operations that deployctl does not currently implement.
- Replacing CLI flags or changing automation semantics.
- Managing DNS, TLS certificates, load balancers, or reverse proxies.
- Exposing PostgreSQL, Redis, or MinIO host ports by default.
- Redirecting the documentation application to Setup.

## CLI Architecture

### Command Catalog

A shared command catalog describes the supported command tree, top-level help text, TUI grouping, short descriptions, and argument-building entrypoints. Top-level `-h`, `--help`, non-TTY no-argument behavior, and the TUI read from this catalog.

The existing `ParseCommand` remains authoritative for validation. Catalog tests compare its approved command tree with help and TUI entries so adding a command requires an explicit presentation decision.

### Dispatch

The executable follows this sequence:

```text
arguments contain -h/--help -> render help -> exit 0
arguments are present       -> existing ParseCommand and Run
no arguments, non-TTY       -> render help -> exit 0
no arguments, TTY           -> run TUI
TUI chooses an operation    -> leave alternate screen -> existing Run(args)
TUI exits                   -> restore terminal -> exit 0
```

The TUI returns an in-memory argument slice to the parent process. It does not spawn deployctl recursively, so tokens and credentials are not exposed through a child process command line. Long-running Docker/native output begins only after the alternate screen has been restored.

## TUI Interaction

Bubble Tea and Bubbles provide the state machine, list, text input, viewport, and terminal lifecycle primitives.

The root menu is:

```text
1. Install and deployment
2. Runtime operations
3. Setup initialization
4. Upgrade and configuration migration
5. Cluster management
6. Deployctl tool
0. Exit
```

It maps to all approved current commands. Number keys and arrows move or select, Enter confirms, Space toggles multi-select values, Escape returns one level, and Ctrl+C quits from every state. Forms also support Tab and Shift+Tab. Sensitive fields use masked inputs and redacted review values.

The final review page shows the equivalent safe command with secret values replaced. Generic confirmation can authorize ordinary operations. Persistent data deletion continues to require the exact installation-derived confirmation phrase.

## Installation Summary

The summary is generated from the finalized `InstallPlan`, not from fixed documentation text. It contains only selected components.

Public endpoints may include:

- API-hosted Setup URL
- Gateway root
- user Web direct URL
- admin Web direct URL
- documentation direct URL
- API base URL
- monitoring URL

Internal Docker endpoints may include PostgreSQL, Redis, MinIO API, and MinIO console. They are labeled as Docker-network-only unless the selected plan explicitly publishes them.

Loopback URLs are printed as immediately usable local checks. A following note explains that remote operators use the deployment-node IP or their configured load balancer/reverse proxy. An explicit public API URL is used when it is available and valid.

Numbered next steps direct the operator to open Setup, use the token, wait for restart, sign in to admin, and use status/doctor plus mode-specific log commands for recovery.

Interactive terminal installs display the one-time token. Non-interactive, redirected, or piped output prints `deployctl setup token show --runtime-dir <dir>` and never renders the token.

## Frontend Runtime Addressing

User and admin images share a runtime environment renderer. It emits:

- an explicit public API base when configured;
- the configured API port;
- the frontend's direct published port;
- enough browser-side logic to distinguish direct access from Gateway/proxy access.

Resolution precedence is:

1. valid explicit public API URL;
2. direct frontend port: current browser scheme and hostname with the configured API port;
3. Gateway or external proxy: same origin.

The bootstrap status client resolves relative Setup paths against the resulting API origin. This keeps the API-hosted Setup UI independent of user/admin hostnames.

Native launchers receive the same values as Docker containers. Joined Web nodes continue to require an explicit reachable API/public routing configuration where the API is not on the same deployment node.

## Static Resource Layout

Admin and documentation production builds use relocatable relative asset bases. The same artifact then resolves correctly from:

- admin `/` and `/admin/`;
- docs `/` and `/developer-docs/`.

Gateway continues stripping its public prefix before proxying to the frontend container. Relative URLs cause browsers to retain the public prefix while direct-root browsers request root assets.

Nginx has explicit exact/static locations for runtime configuration, assets, and OpenAPI resources. Missing static files return `404`. SPA fallback applies only to application navigation routes and cannot answer a JavaScript or CSS request with HTML.

The documentation app has no bootstrap dependency and stays available during Setup.

## Error Handling

The user and admin bootstrap guards distinguish connection failure, invalid JSON, invalid API/Setup URL, restart-pending state, and broken runtime configuration. Recovery output includes the resolved non-secret API endpoint and a runtime-dir-aware doctor command where available.

TUI validation errors remain on the current form and focus the invalid field. Execution errors use the existing CLI diagnostics after the TUI exits. Cancellation returns success and restores terminal modes. Context cancellation returns the existing interrupt status.

Endpoint summaries never include DSNs, passwords, join credentials, storage secrets, or token values outside the approved interactive Setup-token case.

## Testing

### Unit and Contract Tests

- help behavior for both flags and non-TTY no-argument invocation;
- TTY selection of TUI without invoking it in non-interactive tests;
- catalog/help/TUI coverage parity with the approved command tree;
- pure TUI update tests for navigation, selection, forms, masking, cancellation, and destructive confirmation;
- generated TUI arguments round-trip through `ParseCommand`;
- summary matrices for Docker/native, profiles, topologies, roles, components, and token-output policy;
- runtime API resolution for direct ports, Gateway, explicit public API, and remote hostname cases;
- static server contracts for relative assets, MIME types, and missing-asset behavior.

### Docker E2E

A temporary full/single runtime with independent ports and volumes is built from the task branch. It does not remove or reuse the developer's current database.

Before Setup:

- user direct and Gateway access redirect to API-hosted Setup;
- admin direct and Gateway access redirect to API-hosted Setup;
- docs direct and Gateway access render successfully;
- bootstrap, JS, CSS, env, and OpenAPI responses have correct status and content type.

After Setup:

- API restart and readiness complete;
- user and admin applications leave the Setup guard;
- docs remain available;
- status and doctor report a healthy runtime.

Final delivery runs repository verification, committed-scope review, isolated API smoke, Docker E2E, and ship guard.

## Delivery

Implementation starts from the latest `origin/main` on a `codex/` feature branch. The approved requirement and design documents are committed before production code. Implementation uses test-first changes and is delivered as a ready pull request to `main` only after all gates pass.
