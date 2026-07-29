# Deployctl TUI and Service Access Requirements

## Problem

The production installer starts a multi-service deployment but does not clearly report which services are available, where they can be reached, or what the operator should do next. First-time operators see a Setup token without a complete access summary.

Deployctl also requires an explicit command even when launched from an interactive terminal. Operators need a discoverable terminal UI that exposes every existing deployctl operation without replacing scriptable commands.

In the Docker full/single deployment, the directly published frontend ports are currently unusable during Setup:

- the user Web app on port 5173 resolves the bootstrap API against its own origin and receives HTML instead of API JSON;
- the admin Web app is built for `/admin/`, so direct port 5174 returns HTML for JavaScript asset requests and renders a blank page;
- the documentation app is built for `/developer-docs/`, so direct port 5175 has the same asset-path failure.

The API bootstrap endpoint and cross-origin policy are healthy. The failure is in frontend runtime addressing and static-resource path handling.

## Requirements

1. A successful install must print a component-aware service access summary and clear next steps.
2. Interactive terminal output may display the one-time Setup token. Redirected or non-interactive output must not expose it and must print the secure recovery command instead.
3. The summary must list every deployed public endpoint and every deployed internal middleware endpoint, with access scope clearly identified.
4. Remote-node output must explain how loopback addresses relate to node IPs, load balancers, and reverse proxies.
5. Running deployctl without arguments in a real terminal must open an interactive TUI.
6. Running deployctl without arguments outside a terminal must print help and exit successfully without blocking.
7. `deployctl -h` and `deployctl --help` must print the same command help and exit successfully.
8. The TUI must expose every currently supported deployctl command and must reuse the existing parser and execution paths.
9. TUI navigation must support number keys and arrow keys, Enter to confirm, Space to toggle selections, Escape to return, and Ctrl+C or the root Exit item to quit.
10. Sensitive input must be masked and redacted from review screens, diagnostics, and child process arguments.
11. Destructive uninstall must retain its installation-specific typed confirmation and cannot be authorized by a generic TUI confirmation.
12. User and admin Web apps must work both on their directly published ports and behind the Gateway or an external same-origin proxy.
13. The documentation Web app must work both on its direct port and under the Gateway documentation prefix.
14. User and admin Web apps must redirect to the API-hosted Setup page while initialization is pending. The documentation app must remain available and must not redirect to Setup.
15. Explicit `PUBLIC_API_URL` configuration must take precedence over automatic direct-port or same-origin API resolution.
16. Missing JavaScript, CSS, runtime environment, and OpenAPI assets must return real errors rather than SPA HTML with a successful status.
17. Docker, native, and clustered Web-node deployments must use consistent frontend runtime-addressing behavior.

## Current Command Coverage

The TUI and help catalog cover:

- `install`
- `import-config`
- `status`
- `doctor`
- `restart`
- `version`
- `self-update`
- `upgrade`
- `uninstall`
- `setup status`
- `setup token show`
- `setup token reset`
- `cluster token create`
- `cluster join`

## Acceptance Criteria

- Interactive install output includes the Setup token, all selected service endpoints, access scope, and numbered next steps.
- Non-interactive install output contains no Setup token and includes the exact token-show command.
- Help and TUI command coverage cannot drift without a failing test.
- Every TUI path generates arguments accepted by the existing command parser.
- Direct access to ports 5173 and 5174 redirects to the API-hosted Setup page during pending initialization.
- Direct access to port 5175 renders documentation during pending initialization.
- Gateway user, admin, documentation, API, and Setup routes remain functional.
- Direct and Gateway JavaScript, CSS, and runtime configuration responses have correct content types; missing assets do not return `index.html`.
- A clean Docker full/single E2E completes Setup, observes the service restart, and reaches the normal user and admin applications afterward.
- Full verification, committed-scope review, isolated API smoke, and ship guard pass before delivery.

## Approved Design

See `docs/plans/2026-07-29-deployctl-tui-service-access-design.md`.
