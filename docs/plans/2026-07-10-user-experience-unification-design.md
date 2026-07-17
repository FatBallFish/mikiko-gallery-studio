# Pic Gallery User Experience Unification Design

## Status

- Date: 2026-07-10
- Scope: `web/user`, new `web/docs`, shared frontend design foundations used by the user app
- Excluded: visual or structural changes to `web/admin`
- Decision: approved in conversation

## 1. Objective

Unify the public landing experience and authenticated user application into one coherent visual and interaction system, while rebuilding the API documentation as a separately deployed site. The redesign may substantially change page structure and information hierarchy, but it must preserve existing business behavior, routes, API contracts, theme preferences, and accessibility.

## 2. Product Boundaries

The repository has three distinct frontend products:

1. The landing experience introduces the product and routes visitors into creation or API integration.
2. The authenticated user application supports image creation, asset management, billing, API keys, profile, and settings.
3. The administration application supports operations and configuration.

The landing experience and user application share the `Luminous Vault` design system. The administration application remains on its independent `Soft Grid Ops` system and is not redesigned in this work.

The existing API documentation page is removed from the authenticated application. A new independently built and deployed documentation application is added at `web/docs`. User-facing documentation links open the configured external documentation URL.

## 3. Visual Direction

### 3.1 Luminous Vault

The shared user-facing identity is an image-led creative instrument rather than a generic dark dashboard.

- Default theme: ink black and graphite surfaces, warm off-white text, restrained amber brand accents.
- Supporting colors: violet for model or capability emphasis, emerald for success, coral for errors and destructive actions, blue for informational states.
- Imagery supplies most of the page color. Interface chrome remains quiet and legible.
- Glass and metallic highlights are local material effects, not a treatment applied to every container.
- The landing page acts as the gallery foyer; the authenticated app acts as the darkroom and control surface.

### 3.2 Light Theme

Light mode remains a complete supported theme, not a compatibility afterthought. It uses cool white and mist-gray surfaces with graphite text and the same amber signature accent. Dark and light modes share component dimensions, hierarchy, interaction states, and motion behavior.

### 3.3 Shared Design Grammar

- Radius scale: 8px, 12px, 16px, and 24px. Tool surfaces avoid excessive 32px rounding.
- Iconography: Lucide icons with consistent size and stroke width.
- Typography: editorial display treatment for brand moments; highly legible sans-serif typography for Chinese body text and operational UI.
- Borders and elevation: fine low-contrast borders, restrained shadows, and explicit semantic layers.
- Controls: shared buttons, icon buttons, segmented controls, fields, menus, toolbars, status indicators, dialogs, drawers, image actions, and empty states.
- Navigation: the public top navigation and authenticated side navigation use different structures but share brand treatment, active-state behavior, and motion language.

## 4. Information Architecture

### 4.1 Stable Application Shell

The authenticated user application uses one stable shell:

- A narrow primary side navigation on desktop.
- A global top utility bar for theme, balance, and account status.
- A persistent bottom navigation on mobile.
- Route changes animate only the content region; shell geometry remains stable.
- Page headers, filter bars, and action placement follow shared patterns instead of being reinvented per route.

### 4.2 Landing Page

The existing landing content and layout are not a source of truth. The page is rebuilt from current product capabilities and uses real generated imagery.

The narrative order is:

1. A cinematic hero that states the literal offer: one image generation platform for creators and developers.
2. Two primary paths: enter the creative workspace or open the API documentation.
3. A dense capability composition covering text-to-image, image editing, reference generation, abstract model selection, and transparent estimates.
4. A pinned creation flow showing configuration, estimate, task progress, generated output, and asset persistence.
5. A developer integration chapter covering native APIs and OpenAI-compatible endpoints.
6. Real public gallery output as product proof.
7. Live or contract-backed pricing and billing guidance rather than invented static metrics.
8. A high-contrast final action leading to creation or documentation.

### 4.3 Authentication

Authentication is visually continuous with the landing experience. The form is an integrated control surface inside an image-led dark scene, not an isolated warm-white card from a different product. Password, email-code, reset, validation, loading, and third-party placeholder states remain accessible and explicit.

### 4.4 Home

Home prioritizes continuation and discovery:

- Continue the most recent task or start a new creation.
- Show recent task state and actionable failures.
- Surface a restrained set of curated public work.
- Keep documentation, billing, and secondary navigation out of the primary visual path.

### 4.5 Creative Workspace

The creative workspace is the authenticated master page:

- Stable parameter column.
- Large unframed or minimally framed result canvas.
- Parameters derived from live capability data.
- Estimate feedback adjacent to relevant inputs.
- A continuous task-state rail for validation, queueing, generation, storage, completion, partial success, and failure.
- Result actions close to each image.
- A compact recent-history strip that does not compete with the active task.
- On mobile, parameters become an accessible draggable bottom sheet and important image actions do not depend on hover.

### 4.6 Assets And Public Gallery

Private assets and the public gallery share filtering, image geometry, hover/focus behavior, and the image detail viewer. Private assets emphasize selection, grouping, deletion, publication state, and reuse. The public gallery emphasizes inspection and importing generation parameters. Community features not present in the product contract are not invented.

### 4.7 Billing

Billing becomes a continuous checkout flow rather than a collection of detached cards:

- Plan or custom amount selection.
- Payment method selection.
- Sticky order summary.
- Payment progress and result.
- Recent orders and redeem-code entry in secondary bands.

### 4.8 API Keys, Profile, And Settings

These routes share a settings-workspace layout with group navigation and one primary content region. Key creation and secret reveal remain explicit security events. Profile and theme preferences use the same field, save-state, validation, and destructive-action patterns.

### 4.9 Documentation Exit

Authenticated and public documentation links open `VITE_DOCS_URL` in a new tab. The old user-app documentation route no longer renders documentation content and instead performs a clear external navigation or fallback action.

## 5. Independent Documentation Site

### 5.1 Engineering Boundary

`web/docs` is an independent React and Vite application with its own package manifest, build, styles, routing, and deployment configuration. It does not import the authenticated user shell or the `Luminous Vault` theme.

### 5.2 Visual Direction

The documentation site uses a `Technical Editorial` system:

- High-readability light content surfaces by default.
- Graphite navigation and dark code panels.
- Precise grid and restrained accent colors.
- Compact, predictable motion for navigation, search, anchors, and copy feedback.
- Responsive reading widths and persistent desktop table of contents without nested decorative cards.

### 5.3 Content Architecture

- Quick start and first request.
- Authentication and AK/SK security.
- Native image generation API.
- OpenAI-compatible image endpoints.
- Text-to-image, image editing, and reference-image recipes.
- Asynchronous task state and polling.
- Model capabilities, parameters, and estimate behavior.
- Errors, rate limits, idempotency, and troubleshooting.
- Complete endpoint and schema reference.

Guides are curated content modules. Endpoint and schema reference uses `api/openapi/openapi.yaml` as its source of truth and a proven OpenAPI renderer inside the custom documentation shell. Search combines guide metadata and OpenAPI operations at build time. If the reference cannot load, guides remain readable and the failure is explained locally.

## 6. Motion System

### 6.1 Brand Motion

The landing page uses GSAP for the few interactions that require scroll coordination:

- Controlled image scale and luminance changes.
- One pinned product-flow narrative.
- Scrubbed text emphasis where it materially improves comprehension.
- A final chapter transition into the closing action.

The mandatory `gpt-taste` deterministic selection is recorded in the pre-coding design plan and determines the exact hero architecture, component set, type stack, and two GSAP paradigms.

### 6.2 Product Motion

The authenticated app uses short, consistent interactions:

- 180-260ms route content transition.
- Continuous navigation active indicator.
- Button hover lift, active compression, loading, success, and disabled states.
- Shared focus, validation, save, and error feedback for fields.
- Image hover scale and action reveal with equivalent keyboard focus behavior.
- Shared modal, drawer, menu, and bottom-sheet transitions.
- Layout transitions for filters and theme changes without disruptive jumps.

### 6.3 Task Feedback

Task feedback follows the business state model. Estimate values transition without flashing. Queue, generation, storage, partial success, completion, and failure form one continuous state rail. Copy, download, publish, and import actions provide local confirmation in addition to global toasts.

### 6.4 Accessibility

All motion respects `prefers-reduced-motion`. Core layout and behavior remain correct without GSAP. Touch interfaces do not depend on pointer position or hover. Dialog focus, keyboard navigation, Escape behavior, and visible focus states are acceptance requirements.

## 7. Technical Architecture

- Rebuild user semantic tokens before page-level styling.
- Consolidate user primitives and shell components before route migration.
- Keep `web/shared/api-types.ts`, API clients, route identifiers, and data contracts stable.
- Use GSAP only for landing scroll narratives and genuinely complex layout transitions; use CSS for routine micro-interactions.
- Use real product assets and generated results rather than generic remote stock placeholders.
- Add `VITE_DOCS_URL` to user runtime configuration and deployment examples.
- Keep the documentation application isolated from user and admin themes.

## 8. Failure And Degradation

- Images expose local loading, error, and retry states.
- API errors remain close to the action that caused them.
- Advanced motion failure never hides content or controls.
- Documentation reference failure does not block guides.
- Theme initialization avoids an incorrect-theme flash.
- Long Chinese and English text, identifiers, code, and error strings cannot overlap controls.

## 9. Verification And Acceptance

### 9.1 Functional

- Existing user routes and API behavior continue to work.
- Login, creation configuration, estimate, task progress, asset actions, billing, key management, profile, and settings remain usable.
- Documentation links open the configured external site.
- OpenAPI reference reflects the repository contract.

### 9.2 Visual

- Landing and authenticated routes read as one product in dark and light themes.
- No page-level style forks, nested card stacks, arbitrary radius values, or one-off accent systems remain.
- Desktop, tablet, and mobile layouts have no horizontal overflow, clipped text, or incoherent overlap.
- Landing scroll sections render nonblank with correctly framed assets.

### 9.3 Automated And Manual

- User and documentation type checks and production builds pass.
- Repository `./scripts/workflow/verify.sh` passes.
- Browser screenshots cover every user route at desktop and mobile widths in both themes where meaningful.
- Core interactions are exercised in a real browser.
- Review gate checks pass before delivery.

## 10. Implementation Order

1. Establish task context and deterministic `gpt-taste` design plan.
2. Add regression contracts for tokens, navigation, documentation URL behavior, and content models.
3. Rebuild user tokens, primitives, motion utilities, and shell.
4. Rebuild landing and authentication.
5. Rebuild the creative workspace.
6. Migrate home, assets, public gallery, billing, API keys, profile, and settings.
7. Build the independent documentation application and wire external navigation.
8. Verify behavior, accessibility, themes, responsive layout, animation, and production builds.
9. Run repository review and smoke workflows required by the final change set.
