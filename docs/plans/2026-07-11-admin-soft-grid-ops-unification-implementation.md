# Admin Soft Grid Ops 2.0 Unification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rebuild the administration frontend around one compact Soft Grid Ops 2.0 design system while preserving all existing API behavior and management capabilities.

**Architecture:** Establish typography, tokens, motion, shell, and data primitives first. Migrate pages by archetype instead of styling individual pages independently: overview, list, configuration, and workbench. Use existing API clients and behavior models unchanged; add source contracts before each visual or interaction change and finish with full repository verification plus Docker browser acceptance.

**Tech Stack:** React 19, TypeScript, Vite, Tailwind CSS utilities, Lucide React, Geist variable fonts, GSAP for bounded sequenced transitions, existing source-contract scripts, Docker Compose, agent-browser.

---

## Preconditions

- Baseline commit: `0ea714a`
- Approved design commit: `ff7dec2`
- Requirement evidence: `docs/audits/admin-ui-ux-2026-07-07/admin-ui-ux-audit.md`
- Approved design: `docs/plans/2026-07-11-admin-soft-grid-ops-unification-design.md`
- Production scope: `web/admin/**`
- API semantics and `web/shared/**` contracts remain unchanged unless a compile-time alignment issue is discovered.

Before production edits, run:

```bash
./scripts/workflow/start-coding.sh --task "Unify the complete admin frontend with Soft Grid Ops 2.0 typography, density, components, interactions, and page archetypes" --track heavyweight
```

Read the requirement and design files written to `.coding-context.json`, then follow `dev-react-patterns`.

## Task 1: Lock The Visual-System Contract

**Files:**

- Modify: `web/admin/src/ui/adminDesignSystem.contract.ts`
- Modify: `web/admin/src/adminLayout.contract.ts`
- Modify: `web/admin/src/pageScaffold.contract.ts`
- Modify: `web/admin/package.json`
- Modify: `web/admin/package-lock.json`
- Modify: `web/admin/src/main.tsx`
- Modify: `web/admin/src/styles.css`
- Modify: `web/admin/src/ui/classes.ts`

**Step 1: Write the failing contract**

Extend the existing contracts to require:

- Geist and Geist Mono font imports;
- `--admin-font-ui`, `--admin-font-mono`, 11/12/14/16/24px type roles;
- 6/8/12px radius roles;
- 120/180/240ms motion tiers;
- light default and explicit dark semantic tokens;
- no Inter, `rounded-2xl`, `rounded-3xl`, or decorative negative letter spacing in the shared admin primitives;
- stable 216px sidebar and 64px top bar tokens.

**Step 2: Verify the contract fails**

Run:

```bash
npm exec --prefix web/admin -- tsx web/admin/src/ui/adminDesignSystem.contract.ts
npm exec --prefix web/admin -- tsx web/admin/src/adminLayout.contract.ts
```

Expected: FAIL because the runtime still uses Inter and the old size/radius vocabulary.

**Step 3: Install and wire the approved dependencies**

Add local variable-font packages for Geist and Geist Mono. Add `gsap` and `@gsap/react` only if the bounded motion implementation in Task 4 cannot use an existing dependency. Import font CSS from `main.tsx`; do not depend on a remote font CDN.

**Step 4: Implement semantic tokens and primitives**

Define typography, spacing, density, radius, border, focus, overlay, status, and motion tokens in `styles.css`. Refactor `ui/classes.ts` to consume those roles. Preserve existing exported names while introducing focused roles such as `adminType`, `adminMetric`, `adminFeedback`, and compact control sizes.

**Step 5: Verify green**

Run the three changed contracts plus:

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

**Step 6: Commit**

```bash
git add web/admin
git commit -m "feat(admin): establish soft grid ops design tokens"
```

## Task 2: Rebuild The Responsive Admin Shell

**Files:**

- Modify: `web/admin/src/adminLayout.contract.ts`
- Modify: `web/admin/src/adminNavigation.contract.ts`
- Modify: `web/admin/src/layout/AdminLayout.tsx`
- Modify: `web/admin/src/layout/admin-navigation.ts`
- Modify: `web/admin/src/layout/useAdminTheme.ts`
- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/classes.ts`
- Modify: `web/admin/src/styles.css`

**Step 1: Write failing shell contracts**

Require a 216px desktop sidebar, 64px top bar, five approved navigation groups, no duplicate page title in the top bar, accessible account menu, mobile navigation drawer, current-route focus management, and no page-level horizontal overflow.

**Step 2: Verify red**

Run:

```bash
npm exec --prefix web/admin -- tsx web/admin/src/adminLayout.contract.ts
npm exec --prefix web/admin -- tsx web/admin/src/adminNavigation.contract.ts
```

Expected: FAIL on old shell dimensions or missing interaction contracts.

**Step 3: Implement the desktop shell**

Refactor sidebar groups and top-bar content. Keep provider, review, and draft counts compact. Convert theme, notification, and account commands to named icon buttons with tooltips. Remove redundant slogans and page-title duplication.

**Step 4: Implement the mobile shell**

Use the existing overlay/focus patterns to create a menu drawer with focus trap, Escape close, backdrop close, and trigger focus restoration. Keep the current page title in the mobile top bar and make content padding responsive.

**Step 5: Verify green and build**

Run both contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin
git commit -m "feat(admin): unify responsive console shell"
```

## Task 3: Standardize Shared Operational Components

**Files:**

- Modify: `web/admin/src/adminControls.contract.ts`
- Modify: `web/admin/src/adminListPage.contract.ts`
- Modify: `web/admin/src/adminTabs.contract.ts`
- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/dataTable.tsx`
- Modify: `web/admin/src/ui/dataGrid.ts`
- Modify: `web/admin/src/ui/classes.ts`
- Modify: `web/admin/src/ui/icons.tsx`

**Step 1: Write failing primitive contracts**

Require `MetricStrip`, `FilterToolbar`, consistent `DataTable`, semantic `StatusChip`, accessible `ActionMenu`, complex-flow `Drawer`, short-form `Modal`, `InlineFeedback`, geometry-matched `Skeleton`, and icon-based `EmptyState`.

The table contract must require 48-52px rows, sticky headers, numeric alignment, machine-data typography, focusable rows where interactive, stable loading geometry, and a controlled overflow wrapper.

**Step 2: Verify red**

Run the three component contracts. Expected: FAIL for missing primitives and old class vocabulary.

**Step 3: Implement primitives without changing page behavior**

Keep backward-compatible wrappers for currently imported component names. Centralize aria labels, focus-visible behavior, busy state, disabled state, and motion-reduction rules.

**Step 4: Replace ad hoc status and loading primitives in shared components**

Map success, warning, error, and neutral states to one semantic implementation. Do not infer status from arbitrary color class strings.

**Step 5: Verify green**

Run all admin component contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin
git commit -m "feat(admin): standardize operational components"
```

## Task 4: Add Bounded Admin Motion

**Files:**

- Create: `web/admin/src/ui/adminMotion.ts`
- Create: `web/admin/src/ui/adminMotion.contract.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/pages/ReviewPage.tsx`
- Modify: `web/admin/src/styles.css`

**Step 1: Write the failing motion contract**

Require scoped animation contexts, cleanup, reduced-motion bypass, 180-240ms duration bounds, stable transform origins, and no scroll pinning, magnetic movement, or layout-affecting animation.

**Step 2: Verify red**

Run the new contract directly. Expected: FAIL because `adminMotion.ts` does not exist.

**Step 3: Implement bounded motion helpers**

Add page-content reveal, drawer layer transition, and review-preview replacement helpers. Use GSAP context and cleanup only where sequencing improves comprehension. Common hover and focus transitions stay in CSS.

**Step 4: Integrate motion**

Apply page transitions at the route content boundary, overlay transitions in shared Drawer, and content replacement in the review preview. Prevent initial hidden content from remaining hidden when JavaScript motion setup fails.

**Step 5: Verify green**

Run the contract, typecheck, and build. Inspect the production bundle for a single GSAP inclusion.

**Step 6: Commit**

```bash
git add web/admin
git commit -m "feat(admin): add restrained console motion"
```

## Task 5: Refactor The Dashboard Into An Operational Overview

**Files:**

- Modify: `web/admin/src/pages/overviewPage.contract.ts`
- Modify: `web/admin/src/pages/overviewRows.contract.ts`
- Modify: `web/admin/src/pages/OverviewPage.tsx`
- Modify: `web/admin/src/pages/overviewRows.ts`

**Step 1: Write failing overview contracts**

Require a compact metric strip, primary operational surface, actionable anomaly rail, no four-card mobile stack, and no nested bordered empty state.

**Step 2: Verify red**

Run the two overview contracts. Expected: FAIL on old metric-card structure.

**Step 3: Implement the approved hierarchy**

Use current dashboard API data. Preserve all existing metrics and rankings, but place trends and anomalies before supporting detail. Keep metric geometry stable when values change.

**Step 4: Verify green**

Run overview contracts, typecheck, and build.

**Step 5: Commit**

```bash
git add web/admin/src/pages/OverviewPage.tsx web/admin/src/pages/overview*
git commit -m "feat(admin): refocus the operational dashboard"
```

## Task 6: Turn User Management Into A Dense List And Detail Flow

**Files:**

- Modify: `web/admin/src/pages/userRows.contract.ts`
- Modify: `web/admin/src/pages/userDetailLedgerRows.contract.ts`
- Modify: `web/admin/src/pages/userDetailResourceRows.contract.ts`
- Modify: `web/admin/src/pages/UsersPage.tsx`
- Modify: `web/admin/src/pages/userRows.ts`

**Step 1: Write failing users contracts**

Require one filter toolbar, one persistent View action, an overflow action menu, a detail drawer with profile/ledger/resources/limits/danger sections, compact mobile metrics, and preserved pagination behavior.

**Step 2: Verify red**

Run the three user contracts. Expected: FAIL because current detail/actions are split across multiple modal flows.

**Step 3: Implement toolbar and table hierarchy**

Keep current query, status, group, sorting, and pagination API parameters. Move secondary filters into advanced expansion and align balance values with Geist Mono.

**Step 4: Implement the detail drawer**

Reuse existing detail and mutation APIs. Keep destructive confirmations and localize mutation feedback in the drawer. Preserve the action-menu portal behavior.

**Step 5: Verify green**

Run user contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin/src/pages/UsersPage.tsx web/admin/src/pages/user*
git commit -m "feat(admin): streamline user management"
```

## Task 7: Build The Routing And Model Master-Detail Workspace

**Files:**

- Modify: `web/admin/src/pages/routingRows.contract.ts`
- Modify: `web/admin/src/pages/providerModelRows.contract.ts`
- Modify: `web/admin/src/pages/providerModelCapabilities.contract.ts`
- Modify: `web/admin/src/pages/pricingRows.contract.ts`
- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`

**Step 1: Write failing workspace contracts**

Require route selection, selected-route detail, candidate/capability/price completeness, direct repair actions, and preserved context after creation.

**Step 2: Verify red**

Run routing, provider-model, capability, and pricing contracts. Expected: FAIL on disconnected page-local summaries.

**Step 3: Implement route master-detail structure**

Use current list and mutation APIs. Expose missing candidate, price, visibility, and capability conditions as semantic status rows with direct actions.

**Step 4: Align provider model and pricing pages**

Migrate them to shared filter, table, status, drawer, and feedback primitives. Preserve compression capability and test-model request behavior.

**Step 5: Verify green**

Run all four contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin/src/pages/RoutingPage.tsx web/admin/src/pages/ProviderModelsPage.tsx web/admin/src/pages/PricingPage.tsx web/admin/src/pages/*Rows* web/admin/src/pages/providerModelCapabilities.contract.ts
git commit -m "feat(admin): unify routing and model operations"
```

## Task 8: Standardize Review, Monitoring, And Call-Record Workbenches

**Files:**

- Modify: `web/admin/src/pages/reviewRows.contract.ts`
- Modify: `web/admin/src/pages/callRecordRows.contract.ts`
- Modify: `web/admin/src/healthRows.contract.ts`
- Modify: `web/admin/src/pages/ReviewPage.tsx`
- Modify: `web/admin/src/pages/MonitoringPage.tsx`
- Modify: `web/admin/src/pages/CallRecordsPage.tsx`

**Step 1: Write failing workbench contracts**

Require stable queue/detail/action regions, keyboard queue navigation, local refresh state, no full-page clearing, compact diagnostics, and semantic failure actions.

**Step 2: Verify red**

Run the three contracts. Expected: FAIL on missing keyboard or stable-replacement behavior.

**Step 3: Implement review interactions**

Keep the existing decision API. Add queue focus, ArrowUp/ArrowDown selection, reason presets, optional explanation, and bounded preview replacement motion.

**Step 4: Align monitoring and call records**

Use the workbench hierarchy and shared status/table primitives. Retain every existing filter, diagnostic field, and repair link.

**Step 5: Verify green**

Run contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin/src/pages/ReviewPage.tsx web/admin/src/pages/MonitoringPage.tsx web/admin/src/pages/CallRecordsPage.tsx web/admin/src/pages/reviewRows.contract.ts web/admin/src/pages/callRecordRows.contract.ts web/admin/src/healthRows.contract.ts
git commit -m "feat(admin): unify operational workbenches"
```

## Task 9: Rebuild Configuration Pages Around Dirty-State Feedback

**Files:**

- Modify: `web/admin/src/pages/systemSettings.contract.ts`
- Modify: `web/admin/src/pages/securityConfig.contract.ts`
- Modify: `web/admin/src/pages/storageConfig.contract.ts`
- Modify: `web/admin/src/pages/SystemSettingsPage.tsx`
- Modify: `web/admin/src/pages/ConfigPage.tsx`
- Modify: `web/admin/src/pages/SecurityConfigPage.tsx`
- Modify: `web/admin/src/pages/StorageConfigPage.tsx`

**Step 1: Write failing configuration contracts**

Require grouped editors, sticky save rail, dirty-state indication, leave confirmation, local validation/save feedback, and distinct storage draft-test/persisted-probe/default actions.

**Step 2: Verify red**

Run all three contracts. Expected: FAIL on missing shared dirty/save-state model.

**Step 3: Implement shared editor state**

Use a small page-local hook or shared admin helper to derive pristine, dirty, validating, saving, saved, and failed states. Do not change request payloads or endpoint sequencing.

**Step 4: Apply the configuration archetype**

Keep General, Security, and Storage tabs. Make object states visible in the left rail and keep all existing storage activation safety behavior.

**Step 5: Verify green**

Run contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin/src/pages/SystemSettingsPage.tsx web/admin/src/pages/ConfigPage.tsx web/admin/src/pages/SecurityConfigPage.tsx web/admin/src/pages/StorageConfigPage.tsx web/admin/src/pages/*Config.contract.ts web/admin/src/pages/systemSettings.contract.ts
git commit -m "feat(admin): clarify configuration workflows"
```

## Task 10: Migrate Remaining Pages And Remove Design Drift

**Files:**

- Modify: `web/admin/src/pages/OrdersPage.tsx`
- Modify: `web/admin/src/pages/PackagesPage.tsx`
- Modify: `web/admin/src/pages/CashierPage.tsx`
- Modify: `web/admin/src/pages/RedeemPage.tsx`
- Modify: `web/admin/src/pages/AuditPage.tsx`
- Modify: `web/admin/src/pages/SystemUsersPage.tsx`
- Modify: `web/admin/src/pages/UserGroupsPage.tsx`
- Modify: relevant existing contracts under `web/admin/src/pages/*.contract.ts`

**Step 1: Extend existing contracts**

Require shared page headers, filter toolbars, status chips, tables, action menus, empty/loading/error states, and bounded primary actions on every remaining page.

**Step 2: Verify red**

Run every admin contract through the repository contract entrypoint. Expected: FAIL on remaining page-local patterns.

**Step 3: Migrate by archetype**

Move each page to Overview, List, Configuration, or Workbench structure. Preserve all current tabs, filters, actions, forms, exports, pagination, and API behavior.

**Step 4: Sweep visual drift**

Search `web/admin/src` for Inter, arbitrary font sizes, negative letter spacing, oversized radius, hardcoded status colors, nested panels, unlabelled icon buttons, and duplicated status/empty/loading implementations. Remove or document every remaining exception.

**Step 5: Verify green**

Run all admin contracts, typecheck, and build.

**Step 6: Commit**

```bash
git add web/admin
git commit -m "feat(admin): complete console page migration"
```

## Task 11: Responsive, Theme, Accessibility, And Browser Acceptance

**Files:**

- Modify: `web/admin/src/styles.css`
- Modify: existing responsive/accessibility contracts under `web/admin/src/*.contract.ts`
- Create: `docs/audits/admin-ui-ux-2026-07-11/acceptance.md`
- Create: `docs/audits/admin-ui-ux-2026-07-11/screenshots/*.png`

**Step 1: Add failing acceptance contracts**

Require no page-level horizontal overflow, stable metric dimensions, readable light/dark tokens, 44px mobile primary targets, menu and drawer focus restoration, reduced motion, and no text overlap patterns.

**Step 2: Verify red, then implement**

Fix responsive and theme issues in shared styles and primitives rather than page-specific patches.

**Step 3: Run complete automated verification**

```bash
./scripts/workflow/verify.sh
git diff --check
```

Expected: all Go tests/vet, contracts, and user/admin/docs typechecks/builds pass.

**Step 4: Rebuild Docker**

```bash
BUILDKIT_PROGRESS=plain docker compose -f deployments/docker-compose/docker-compose.dev.yml up -d --build --remove-orphans
```

**Step 5: Perform browser acceptance**

Audit dashboard, users, review, routing, call records, cashier, and all system-settings tabs at 1440, 1280, 1024, 768, 390, and 320px. Cover light/dark themes, filters, action menus, drawers, save states, errors, keyboard navigation, reduced motion, and refresh behavior. Record findings and screenshots under the new audit directory.

**Step 6: Run review and API smoke**

```bash
./scripts/workflow/review-local.sh --scope all
./scripts/workflow/api-smoke.sh
```

Expected: review gate PASS and API contract smoke passed.

**Step 7: Commit acceptance evidence**

```bash
git add web/admin docs/audits/admin-ui-ux-2026-07-11
git commit -m "test(admin): verify soft grid ops redesign"
```

## Completion Criteria

- Every protected admin page uses one of the four approved archetypes.
- Runtime typography is Geist plus Geist Mono and follows the approved scale.
- Shared controls and data views use the approved density, radius, status, and motion language.
- Light and dark themes are readable and complete.
- Desktop and mobile acceptance show no incoherent overlap or page-level overflow.
- Existing API behavior and permission contracts remain green.
- Full repository verification, local review, API smoke, Docker health, and browser acceptance pass.
