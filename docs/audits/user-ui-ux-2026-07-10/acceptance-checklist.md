# User Experience Unification Acceptance

Date: 2026-07-10

## Product Boundaries

- [x] Landing and authenticated user routes share the Luminous Vault design system.
- [x] Light mode remains supported by the user application.
- [x] Admin visual design remains independent and was not migrated.
- [x] API documentation is an independent `web/docs` build and deployment target.
- [x] Backend `/docs/` remains available; the frontend docs site uses `/developer-docs/`.

## User Application Evidence

- [x] Authentication desktop dark/light and mobile states: `authentication-acceptance.md`.
- [x] Workspace desktop/mobile capability, progress, and sheet states: `workspace-task6-acceptance.md`.
- [x] Home, private assets, public gallery, detail, lightbox, pagination, empty/error states: `task7-gallery-acceptance.md`.
- [x] Landing, shell, settings workspace, billing, API keys, profile, and theme contracts are wired into `scripts/workflow/verify-contracts.sh` or the user typecheck/build.

## Documentation Site Evidence

- [x] Quick start renders at 1440x900 and 390x844 without page-level horizontal overflow.
- [x] `/` opens search; Chinese/English queries, Arrow navigation, Enter selection, Escape, focus restoration, and background inert state were exercised.
- [x] Mobile navigation is hidden from keyboard flow while closed and uses an inert content layer while open.
- [x] Code copy provides explicit success or retry feedback when clipboard access is unavailable.
- [x] Scalar reference is lazy-loaded with its official stylesheet and renders without overflow at desktop and mobile widths.
- [x] Generated developer OpenAPI contains only `/api/open/*` and `/v1/*`; agent and admin routes/tags are excluded.
- [x] Quick-start examples use the current HMAC signing headers and current task/estimate parameter names.

Final screenshots are under `screenshots/docs-*.png`. The Scalar bundle remains a large lazy chunk; this does not affect the guide-first initial chunk but should be watched during dependency upgrades.
