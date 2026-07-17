# Creative Workspace Task 6 Acceptance

Verified on 2026-07-10 against the local user application and a temporary Vite-only component harness.

## Component State Evidence

The following images are browser-rendered component previews, not design mockups. The temporary harness imported the production `createWorkspaceViewModel` and `WorkspaceStatusRail` and rendered the labeled ready, running, success, partial, failure, and insufficient-balance states. The harness used a separate HTML entry that was not referenced by the main application and was removed after capture.

- `screenshots/workspace-component-states-desktop-1440.png`
- `screenshots/workspace-component-states-mobile-390.png`

Only the real backend connection was unavailable during this preview. Consequently, the screenshots prove component rendering and responsive composition, but do not claim a live SSE or provider response. The status-rail SSR contract covers its polite, atomic live-region and busy-state markup.

## Post-portal Mobile Sheet Evidence

The production `WorkspacePage` was verified after moving the compact bottom sheet into the existing `OverlayPortal`. A local stored session was used only to enter the protected route; the API and SSE backend remained unavailable. These checks prove responsive layout, pointer gestures, action visibility, inert behavior, and overflow handling, but do not claim live estimate or generation behavior.

- `screenshots/workspace-quality-mobile-390-collapsed-final.png`
- `screenshots/workspace-quality-mobile-390-expanded-final.png`
- `screenshots/workspace-quality-mobile-320-collapsed-final.png`
- `screenshots/workspace-quality-mobile-320-expanded-final.png`

| Viewport | Gesture/state | Result |
| --- | --- | --- |
| 390 x 844 | Expanded sheet dragged down 80 px | Snapped collapsed; exactly one compact generate action visible; full region `inert=true` and `aria-hidden=true`; no horizontal overflow |
| 390 x 844 | Expanded | Exactly one full generate action visible; compact action absent; full region not inert; no horizontal overflow |
| 320 x 700 | Collapsed | Handle and compact action visible above mobile navigation (`handleBottom=628.5`, `navTop=636`); exactly one action; no horizontal overflow |
| 320 x 700 | Collapsed sheet dragged up 80 px | Snapped expanded; exactly one full generate action visible; compact action absent; no horizontal overflow |

## Collapsed Parameter Accessibility

The production `WorkspacePage` was loaded at `#/genpic` with the local API unavailable, which leaves the page usable in its capability-unavailable state.

| Viewport | Compact query | Toggle target | Collapsed region | Keyboard result |
| --- | --- | --- | --- | --- |
| 390 x 844 | matched | `workspace-parameter-controls` | `inert=true`, `aria-hidden=true` | 8 consecutive Tab presses never entered the region |
| 320 x 700 | matched | `workspace-parameter-controls` | `inert=true`, `aria-hidden=true` | 8 consecutive Tab presses never entered the region |

The pure responsive contract also verifies that a collapsed parameter region is hidden only when the compact media query matches. A desktop viewport therefore keeps the parameter controls exposed even though the mobile sheet state defaults to collapsed.

## Automated Checks

```text
npm exec --prefix web/user -- tsx web/user/src/pages/workspaceResponsive.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/workspacePage.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/WorkspaceStatusRail.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/workspaceMotion.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/workspaceEstimate.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/workspaceReferenceLimit.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/workspaceSheetGesture.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/user run build
```
