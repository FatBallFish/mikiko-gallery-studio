# Admin Soft Grid Ops 2.0 Unification Design

## Status

- Date: 2026-07-11
- Scope: `web/admin/**`
- Decision: approved in conversation
- Baseline commit: `0ea714a`
- Requirement evidence: `docs/audits/admin-ui-ux-2026-07-07/admin-ui-ux-audit.md`

## 1. Objective

Unify the complete administration frontend into a compact, readable operations console without changing backend API semantics or removing business capabilities. The redesign must improve cross-page consistency, scanning efficiency, interaction feedback, responsive behavior, and long-session comfort.

The approved direction is **Soft Grid Ops 2.0**:

- light theme by default, with a complete dark theme;
- compact but not cramped information density;
- Geist for interface text and Geist Mono for machine data;
- restrained blue-violet accent and semantic status colors;
- shared page archetypes and component primitives instead of page-local design systems;
- information architecture and operation placement may change, while business behavior remains stable.

## 2. Audit Findings

The live Docker audit covered dashboard, users, routing, storage settings, and the 390px users view. The main cross-page issues are:

1. The runtime still uses Inter, while typography size and weight vary by page.
2. Navigation, buttons, table content, labels, and helper text often have similar visual weight.
3. Dashboard, list, configuration, and workbench pages use different spacing and surface rules.
4. Page-local class maps repeatedly define cards, filters, status badges, and table interactions.
5. Large metric cards consume excessive vertical space, especially on mobile.
6. Nested bordered panels flatten hierarchy and create visual noise.
7. Row actions are inconsistent and frequently expose too many equal-weight commands.
8. Loading, empty, error, saving, and unsaved states do not share one feedback model.
9. The existing mobile shell works, but dense data layouts collapse into inefficient vertical stacks.

## 3. Design Principles

### 3.1 Operational hierarchy

Every page must make three layers visually distinct:

1. primary data or current work;
2. filters, context, and secondary status;
3. commands and destructive operations.

The primary data area receives the strongest structure. Secondary information uses spacing and subtle surface contrast. Destructive actions never compete visually with the main workflow.

### 3.2 Stable density

- Desktop data rows: 48-52px.
- Control heights: 36px compact, 40px default, 44px primary touch-friendly action.
- Page title block: 64-72px excluding wrapped descriptions.
- Main page gaps: 20-24px desktop, 16-20px narrow screens.
- Radius scale: 6px, 8px, and 12px; full radius only for status chips and avatars.
- Shadows are reserved for overlays and elevated menus.

### 3.3 Typography

- UI family: Geist.
- Machine data family: Geist Mono.
- Page title: 24px, weight 650.
- Section title: 16px, weight 650.
- Body and table content: 14px, weight 400-550.
- Supporting text: 12px, weight 450.
- Table headers and compact labels: 11px, weight 600.
- Decorative uppercase English eyebrows are removed.
- Letter spacing remains zero except existing machine-code conventions that require differentiation.

## 4. Global Shell

### 4.1 Desktop

- Fixed 216px sidebar.
- Navigation groups: Overview, Users and Content, Transactions, Models and Generation, System.
- 64px top bar containing provider summary, pending work, notifications, theme, and account menu.
- Page title is not repeated in the top bar.
- Content uses the available width with stable responsive padding.

### 4.2 Mobile

- Compact top bar with menu, current page title, and theme/account access.
- Navigation opens in an accessible drawer.
- Metric summaries use a compact two-column or horizontal strip instead of full-width tall cards.
- Wide data views use controlled horizontal scrolling or responsive row summaries; the entire page must not overflow horizontally.

## 5. Page Archetypes

### 5.1 Overview

Structure: compact metric strip, primary trend or operational surface, anomaly and pending-work rail, supporting detail.

Metrics must not dominate the first viewport. Empty visualizations use local empty states with a relevant next action instead of nested empty cards.

### 5.2 List

Structure: page header, single-layer filter toolbar, result summary and bulk actions, data table, pagination.

Two primary filters remain visible. Secondary filters expand in place. Row actions expose at most one persistent action; remaining actions use a menu. Numeric columns align right and machine data uses Geist Mono.

### 5.3 Configuration

Structure: object list or category navigation on the left, grouped editor on the right, sticky save rail.

The editor communicates pristine, dirty, validating, saving, saved, and failed states. Draft probes, persisted probes, and activation actions use distinct labels and feedback.

### 5.4 Workbench

Structure: queue or selection rail, primary detail/preview, decision or diagnostic panel.

Review, monitoring, and call-record workflows keep context visible while the active item changes. Item changes update content without clearing the full page.

## 6. Shared Component Language

The redesign standardizes these primitives:

- `PageHeader`: one primary action and a bounded secondary action menu.
- `MetricStrip`: compact metrics with consistent number, delta, and context positions.
- `FilterToolbar`: search, primary filters, advanced filters, clear action, and result count.
- `DataTable`: stable row height, sticky header, hover/focus states, overflow policy, and loading rows.
- `StatusChip`: success, warning, error, and neutral semantics only.
- `ActionMenu`: named icon trigger, keyboard navigation, and portal positioning.
- `Drawer`: complex edit and detail flows with focus trap and focus restoration.
- `Modal`: short, low-risk forms and confirmations.
- `InlineFeedback`: local save, validation, and retry feedback.
- `EmptyState`: icon, concise explanation, and optional next action.
- `Skeleton`: geometry-matched loading placeholders.

Lucide icons use a shared 1.5 stroke width. Clickable surfaces define hover, focus-visible, pressed, busy, and disabled states. Icon-only actions require an accessible name and tooltip.

## 7. Priority Page Changes

### 7.1 Dashboard

- Replace four tall cards with a compact metric strip.
- Promote operational trends and anomalies into the first viewport.
- Consolidate provider failures, pending reviews, and configuration drafts in one actionable rail.
- Reduce nested empty-state surfaces.

### 7.2 Users

- Merge filters, result count, and batch controls into one toolbar.
- Prioritize identity, balance, status, and last activity in the table.
- Move low-frequency attributes into a detail drawer.
- Keep only View visible; move status, group, points, limits, password, and delete actions into the row menu.
- Group detail content into profile, ledger, resources, limits, and dangerous actions.

### 7.3 Routing and models

- Use a master-detail workspace for route selection and configuration.
- Surface candidate, capability, visibility, and pricing completeness together.
- Provide direct repair actions for missing candidates, missing prices, and unavailable routes.
- After route creation, preserve context and continue into candidate configuration.

### 7.4 Review queue

- Keep the three-column workbench while standardizing density and action hierarchy.
- Support keyboard queue navigation.
- Use reason presets plus optional explanation for rejection.
- Crossfade preview and metadata changes without full-page loading flashes.

### 7.5 System settings

- Retain General, Security, and Storage as page-level tabs.
- Group forms by business meaning and use a sticky save rail.
- Keep draft test, saved-config probe, and default activation visually and behaviorally distinct.
- Show dirty, failed probe, enabled, and default states in the object list.

## 8. Interaction And Motion

Motion durations use 120ms, 180ms, and 240ms tiers.

- CSS handles hover, press, focus, status, menu, and skeleton motion.
- GSAP is limited to page-content transitions, drawer layering, and review preview replacement where sequencing improves comprehension.
- Marketing motion, scroll pinning, magnetic controls, large zooms, and decorative parallax are excluded from the operations console.
- `prefers-reduced-motion` removes nonessential movement without hiding state changes.

Layouts must keep stable dimensions during loading, hover, label changes, and asynchronous updates.

## 9. State And Error Handling

- Initial loads use geometry-matched skeletons.
- Refreshes retain stale data and show local progress.
- Filters synchronize with the route where practical.
- Forms expose pristine, dirty, validating, saving, saved, and error states.
- Field and section errors appear near the affected control.
- Toasts are reserved for authentication and cross-page outcomes.
- Dangerous confirmations name the target, impact, and reversibility.
- Leaving a dirty editor requires confirmation.

## 10. Accessibility And Responsive Acceptance

- Test at 1440, 1280, 1024, 768, 390, and 320px.
- No incoherent overlap, clipped button labels, or page-level horizontal overflow.
- Tabs, menus, modals, drawers, and workbench navigation support the keyboard.
- Overlays trap focus and restore it to the trigger.
- Status is never communicated by color alone.
- Light and dark themes preserve readable foreground, borders, focus, and disabled states.

## 11. Verification

Automated coverage includes source contracts for typography, density, tokens, shell geometry, component primitives, and page archetype adoption. Existing behavior contracts must remain green.

Required commands:

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
./scripts/workflow/verify.sh
```

Docker browser acceptance covers dashboard, users, review, routing, call records, cashier configuration, system settings, theme switching, filters, drawers, saving, error feedback, and keyboard navigation on desktop and mobile.

## 12. Non-Goals

- No backend API behavior changes.
- No removal of existing management capabilities.
- No marketing hero, decorative imagery, or editorial landing-page composition.
- No global rewrite of `web/user` or `web/docs`.
- No speculative custom charting engine.
