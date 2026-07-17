# Admin Soft Grid Ops 2.0 Acceptance

- Date: 2026-07-11
- Runtime: Docker development stack at `http://127.0.0.1:8088/admin/`
- Account: `admin@example.com` (`super_admin`)
- Browser: Chromium via `agent-browser 0.31.1`
- Theme coverage: application light and dark themes

## Automated Verification

- `./scripts/workflow/verify.sh`: PASS
- All frontend/shared contracts: PASS
- Go test and vet: PASS
- User, admin, and docs typecheck/build: PASS
- Admin visual-drift and acceptance contracts: PASS
- `git diff --check`: PASS
- Docker Compose build and health checks: PASS
- `./scripts/workflow/api-smoke.sh`: PASS

The production builds retain the existing bundle-size warning for chunks above 500 kB. It does not fail the build or acceptance gate.

## Responsive Matrix

The following routes were opened at 1440, 1280, 1024, 768, 390, and 320 px widths:

- Operational overview
- Users
- Review queue
- Routing
- Call records
- Cashier configuration
- System settings: General
- System settings: Security
- System settings: Storage

For every route and viewport, `document.documentElement.scrollWidth - document.documentElement.clientWidth` returned `0`. No page-level horizontal overflow, incoherent overlap, clipped command label, or hidden primary action was observed. Wide tables retain local overflow behavior.

## Interaction Results

- Mobile navigation moves focus into the drawer; Escape closes it and restores focus to `打开导航`.
- System settings tabs support ArrowLeft/ArrowRight and update the route without losing focus.
- ActionMenu closes and restores focus to `更多` before opening a Modal; closing the Modal returns focus to the same trigger.
- System-account search submitted an exact server-side query and rendered the zero-result state.
- General configuration exposed dirty state, enabled local actions, and returned to pristine after `恢复本类` without persisting a test change.
- Storage persisted-probe completed successfully and rendered local success feedback while keeping mutation controls mutually exclusive.
- Light and dark theme screenshots preserve readable foreground, borders, disabled states, semantic status colors, and focus affordances.
- With `prefers-reduced-motion: reduce`, the media query matched, no motion item remained hidden, CSS animation duration resolved to `0.01ms`, and page overflow remained zero.
- Browser console and page error collections were empty after the acceptance flow.

## Finding Closed During Acceptance

The dashboard model-distribution empty state was nested inside an already bordered chart surface. A shared `inline` EmptyBlock variant was added and all nested overview empty states now use the unframed variant. Source contracts were added before the fix and pass after the change.

## Screenshot Evidence

- `screenshots/1440-dashboard-light.png`
- `screenshots/1440-dashboard-dark.png`
- `screenshots/1440-review-dark.png`
- `screenshots/1440-system-storage-light.png`
- `screenshots/1440-system-storage-dark.png`
- `screenshots/1280-users-light.png`
- `screenshots/1024-review-light.png`
- `screenshots/768-routing-light.png`
- `screenshots/390-cashier-light.png`
- `screenshots/390-cashier-dark.png`
- `screenshots/390-system-storage-dark.png`
- `screenshots/320-system-general-light.png`
