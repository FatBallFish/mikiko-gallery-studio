# Pic Gallery Frontend Product Rebuild Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 重构 `web/user` 与 `web/admin`，把设计稿目录 `docs/template/PicGallery` 转换为可运行、可交互、按技术方案 API 形态对接且内置 Mock 服务的准出产品前端。

**Architecture:** 保留 React + Vite + TypeScript 双应用形态，不引入额外运行时依赖；以 `web/shared` 承载设计 token、基础样式与 Mock/API 类型。用户端采用 Luminous Vault：左导航 + 全局 TopBar + 创作工作台/历史/密钥/账户/文档/落地页/登录独立路由。管理端采用 Soft Grid Ops：左导航 + TopBar + 状态条 + 主表/审核/配置/路由/计费连续工作区。所有真实后端接口通过 `apiClient` 统一封装，开发期默认走 `mockTransport`，请求/响应字段对齐 `docs/tech/pic-gallery-tech-design.md` 与 `api/openapi/openapi.yaml`。

**Tech Stack:** React 19, Vite 7, TypeScript, CSS Modules-by-convention via global CSS, browser `localStorage/sessionStorage`, native `fetch`-compatible mock transport, no external UI library.

---

## 固定输入与验收基线

- 设计源：`docs/template/PicGallery/DESIGN-MANIFEST.json`、`DESIGN-HANDOFF.md`、`brand-spec.md`、`landing.html`、`login.html`、`home.html`、`app.html`、`gallery.html`、`api-keys.html`、`profile.html`、`admin.html`。
- 规范源：`docs/design/frontend-design-spec.md`、`docs/design/frontend-visual-directions.md`。
- 接口源：`docs/tech/pic-gallery-tech-design.md`、`api/openapi/openapi.yaml`。
- 用户端必须覆盖：Landing、Login、Home、参考生图、文生图、历史图库、API Key、账户/积分、开发文档。
- 后台端必须覆盖：登录/准入、运营总览、配置中心、模型路由、价格策略、审核队列、用户管理、任务/审计日志、系统健康。
- Mock 必须覆盖：邮箱验证码、登录/刷新/退出、用户资料、余额/流水/估价/兑换、capabilities、参考图上传、任务创建/轮询/历史、API Key CRUD、后台 metrics/config/provider/routing/pricing/review/users/audit。
- 所有页面必须有 loading/empty/error/success 或 disabled 状态；关键操作有 toast/inline feedback；表单有校验。
- 验证命令：`npm run typecheck` 和 `npm run build` 分别在 `web/user`、`web/admin` 通过；浏览器抽查桌面和移动无横向滚动。

## Task 1: Shared Design Tokens + API/Mock Core

**Files:**
- Modify: `web/shared/base.css`
- Modify: `web/shared/tokens.css`
- Modify: `web/shared/user-theme.css`
- Modify: `web/shared/admin-theme.css`
- Create: `web/shared/api-types.ts`
- Create: `web/shared/mock-data.ts`
- Create: `web/shared/mock-api.ts`

**Steps:**
1. Extract token names from `brand-spec.md`: user Luminous Vault colors, admin Soft Grid Ops colors, spacing, radii, shadows, z-index, motion.
2. Add base reset for buttons, inputs, focus-visible, tables, dialogs, responsive media, reduced motion.
3. Define TypeScript API DTOs matching tech design endpoint groups:
   - Agent auth/profile/billing/image/key/doc.
   - Ops admin metrics/config/model/review/audit/users.
   - Common envelope `{ code, message, data, request_id }` and paginated list.
4. Implement `MockPicGalleryApi` with delayed promises and deterministic in-memory state.
5. Include task state progression: create returns `queued/running`; polling advances to `succeeded`; history receives generated results.
6. Include failure toggles: invalid login, low balance estimate, disabled API key, config conflict.
7. Export singleton API plus reset helpers for tests/manual verification.

**Acceptance:**
- Shared CSS imports without path errors from both apps.
- `mock-api.ts` exposes all methods used by user/admin apps.
- DTO endpoint names and path constants align with `/api/agent/*`, `/api/open/*`, `/v1/*`, `/api/ops/*`.

## Task 2: User App Route Shell + Auth/Landing/Home

**Files:**
- Rewrite: `web/user/src/App.tsx`
- Rewrite: `web/user/src/main.tsx`
- Rewrite: `web/user/src/styles.css`
- Create: `web/user/src/types.ts`
- Create: `web/user/src/useMockResource.ts`
- Create: `web/user/src/components.tsx`
- Create: `web/user/src/pages/LandingPage.tsx`
- Create: `web/user/src/pages/LoginPage.tsx`
- Create: `web/user/src/pages/HomePage.tsx`

**Steps:**
1. Replace old hash-only minimal routing with hash routes: `#/landing`, `#/login`, `#/home`, `#/workspace`, `#/text`, `#/gallery`, `#/api-keys`, `#/profile`, `#/docs`.
2. Build a single user shell matching design: fixed 108px sidebar on desktop, mobile bottom/horizontal nav, fixed global TopBar with balance/messages/activity/avatar.
3. Implement Landing as separate public surface with hero, value props, proof strip, CTA; no product shell chrome.
4. Implement Login with password/code tabs, send-code countdown, validation, mock login, redirect return-to.
5. Implement Home as operational entry: hero carousel-ish showcase, quick actions to workspace/text/gallery, inspiration strip, recent tasks.
6. Ensure route guards: protected pages redirect unauthenticated users to `#/login` with return target.
7. Add toast system and inline banners.

**Acceptance:**
- Login form actually sends mock code, validates email/code/password, creates session, and redirects.
- Landing CTA navigates to login/workspace correctly.
- Home shows real mock balance/profile/history data, not static unreachable filler.

## Task 3: User Creation Workspace + History Gallery

**Files:**
- Create: `web/user/src/pages/WorkspacePage.tsx`
- Create: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/styles.css`

**Steps:**
1. Implement reference image workspace from `app.html`: tabs for text/reference, prompt textarea, reference upload list, model/quality/ratio/count controls, estimate panel, create button.
2. Wire estimate to `GET /api/agent/billing/v1/estimate` mock and task creation to `POST /api/agent/image/v1/tasks` mock.
3. Add task progress panel: queued/running/succeeded/failed with retry/cancel-like UI where applicable.
4. Render generated results with preview, download, copy prompt, send to gallery, apply as reference.
5. Implement text-only page as same workspace with upload disabled and `task_type=text_to_image`.
6. Implement Gallery with filters type/status/date/search, preview drawer/modal, publish request, delete-hide, download action, empty state.

**Acceptance:**
- User can complete prompt -> estimate -> generate -> progress -> result -> gallery flow entirely in Mock.
- Gallery reflects newly generated tasks and supports filtering and publish request state updates.
- Low balance/invalid prompt paths show actionable errors and keep form state intact.

## Task 4: User Developer/API Key/Profile/Docs

**Files:**
- Create: `web/user/src/pages/ApiKeysPage.tsx`
- Create: `web/user/src/pages/ProfilePage.tsx`
- Create: `web/user/src/pages/DocsPage.tsx`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/styles.css`

**Steps:**
1. Implement API Key table from `api-keys.html` with create modal, permission scopes, RPM, expiry, one-time SK reveal, copy, disable/enable/delete.
2. Add quick-start code samples for native `/api/open/image/v1/tasks`, OpenAI `/v1/images/generations`, `/v1/images/edits` with selected key placeholder.
3. Implement Profile: balance summary, redeem code, ledger list, profile edit, preferences default model/ratio/quality.
4. Implement Docs: searchable endpoint catalog from tech design/API constants, categorized by Agent/Open API/OpenAI Compat/Ops, code tabs and error code examples.
5. Ensure copy buttons use Clipboard API with fallback success message.

**Acceptance:**
- API Key create/disable/delete mutates mock state and updates UI.
- Redeem code changes balance/ledger with success and invalid-code error.
- Docs search filters real endpoint definitions and displays request/response shape.

## Task 5: Admin App Product Console

**Files:**
- Rewrite: `web/admin/src/App.tsx`
- Rewrite: `web/admin/src/main.tsx`
- Rewrite: `web/admin/src/styles.css`
- Create: `web/admin/src/types.ts`
- Create: `web/admin/src/components.tsx`
- Create: `web/admin/src/pages/LoginPage.tsx`
- Create: `web/admin/src/pages/OverviewPage.tsx`
- Create: `web/admin/src/pages/ConfigPage.tsx`
- Create: `web/admin/src/pages/RoutingPage.tsx`
- Create: `web/admin/src/pages/PricingPage.tsx`
- Create: `web/admin/src/pages/ReviewPage.tsx`
- Create: `web/admin/src/pages/UsersPage.tsx`
- Create: `web/admin/src/pages/AuditPage.tsx`

**Steps:**
1. Build admin route shell: `#/login`, `#/overview`, `#/config`, `#/routing`, `#/pricing`, `#/reviews`, `#/users`, `#/audit`, `#/health`.
2. Keep Soft Grid Ops visual: 216px sidebar, topbar alert chips, status strip, dense tables, low-noise feedback rail.
3. Implement admin login with mock credentials and route guard.
4. Overview: metrics dashboard, provider health, queue, recent audit.
5. Config: tabbed config table, inline edit, dirty-state, conflict detection, publish/revert.
6. Routing/Pricing: provider routes, error policies, price matrix edit and publish summary.
7. Reviews: approve/reject/unpublish with reason drawer, status mutation, empty state.
8. Users/Audit/Health: search/filter users, adjust status/points/group, audit timeline, system probes.

**Acceptance:**
- Admin can login, edit config/pricing/routes, publish/revert, approve/reject review items, search users.
- All admin write actions produce nearby status feedback and append audit rows.
- Dense layout remains usable at 1366x768 and collapses gracefully on tablet/mobile.

## Task 6: Verification, Visual QA, Cleanup

**Files:**
- Modify as needed: all touched files.
- Optional docs update: `README.md` if dev commands changed.

**Steps:**
1. Run `npm run typecheck` in `web/user`; fix all TS errors.
2. Run `npm run build` in `web/user`; fix build errors.
3. Run `npm run typecheck` in `web/admin`; fix all TS errors.
4. Run `npm run build` in `web/admin`; fix build errors.
5. Start both Vite dev servers or previews and inspect via browser at desktop and mobile widths.
6. Exercise critical flows:
   - User: landing -> login -> workspace -> generate -> gallery -> publish -> API key create -> docs search -> redeem code.
   - Admin: login -> config edit/publish -> pricing edit -> review approve/reject -> user search/status.
7. Check no horizontal overflow at 390, 820, 1366 widths; focus states visible; buttons disabled during async actions.
8. Remove dead old files only if no imports reference them; do not delete unrelated user changes.

**Acceptance:**
- Both apps typecheck and build.
- Browser smoke tests pass for critical flows.
- Final implementation is not a static demo: state changes, validation, async loading, errors, and generated data are observable.
