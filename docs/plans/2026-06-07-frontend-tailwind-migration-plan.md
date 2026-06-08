# Pic Gallery 前端 Tailwind 迁移方案

日期：2026-06-07

## 1. 背景与目标

本方案最初基于“前端尚未接入 Tailwind”的状态制定。当前工作区已经完成 Tailwind v4 接入，并推进了一批迁移；因此本文件同时保留迁移前基线、完整替换策略和当前代码的后续执行矩阵。迁移前，用户端和管理后台分别依赖大体量手写 CSS：

- 用户端入口：`web/user/src/styles.css`，约 761 行，约 250 个 class。
- 管理后台入口：`web/admin/src/styles.css`，约 2095 行，约 193 个 class。
- 共享基础样式：`web/shared/base.css`。
- 共享主题变量：`web/shared/user-theme.css`、`web/shared/admin-theme.css`、`web/shared/tokens.css`。

迁移目标不是把每条 CSS 机械翻译成一长串 `className`，而是用 Tailwind 替代绝大部分布局、间距、边框、字号、状态色、响应式和交互样式，同时保留少量更适合全局维护的样式资产：

- 保留：字体导入、主题 CSS 变量、全局 reset、focus-visible、reduced-motion、复杂背景纹理、少数 data-grid 列模板常量。
- 替换：按钮、表单、卡片、壳布局、顶部栏、侧边栏、状态标签、Toast、Modal、表格行、页面栅格、图库卡片、收银台卡片、管理后台密集数据视图等。

最终验收标准仍按“清空绝大部分手写样式”判断：

- `web/user/src/styles.css` 和 `web/admin/src/styles.css` 不再承载页面级大量手写样式，总行数从迁移前约 2856 行降到 500 行以内。
- 新页面和新组件优先使用 Tailwind utilities 或受控的 class 常量，不再新增大段页面专属 CSS。
- 用户端、管理后台在桌面和移动视口下视觉不退化、不重叠、可滚动区域正确。
- `npm --prefix web/user run typecheck && npm --prefix web/user run build` 通过。
- `npm --prefix web/admin run typecheck && npm --prefix web/admin run build` 通过。

## 2. 当前前端结构结论

### 2.1 项目结构

两个前端是独立 Vite React 应用：

- `web/user`
  - `web/user/package.json`
  - `web/user/vite.config.ts`
  - `web/user/src/main.tsx`
  - `web/user/src/App.tsx`
  - `web/user/src/components.tsx`
  - `web/user/src/pages/*`
- `web/admin`
  - `web/admin/package.json`
  - `web/admin/vite.config.ts`
  - `web/admin/src/main.tsx`
  - `web/admin/src/App.tsx`
  - `web/admin/src/components.tsx`
  - `web/admin/src/pages/*`

两端共同依赖：

- `web/shared/api-types.ts`
- `web/shared/user-api.ts`
- `web/shared/admin-api.ts`
- `web/shared/base.css`
- `web/shared/user-theme.css`
- `web/shared/admin-theme.css`
- `web/shared/tokens.css`

### 2.2 当前样式特点

用户端风格：

- 深色高质感产品界面。
- 大量 `oklch()`、`color-mix()`、玻璃拟态、径向渐变。
- 主要布局：左侧窄 sidebar + sticky topbar + 内容区。
- 高频组件：`btn`、`card`、`modal`、`toast`、`gallery-card`、`public-detail-*`、`checkout-*`、`workspace-*`。

管理后台风格：

- 浅色高密度运营控制台。
- 大量数据网格，靠 CSS class 定义 `grid-template-columns`。
- 高频组件：`btn`、`ghost`、`pg-admin-card`、`page-header`、`ops-status-strip`、`admin-data-grid`、`table-row`、`badge`、`inline-feedback`、`modal-*`、`form-grid`、`field`。

### 2.3 迁移难点

1. 管理后台数据表格列模板很多，例如：
   - `.users-grid`
   - `.call-record-grid`
   - `.cashier-plan-grid`
   - `.cashier-order-grid`
   - `.route-model-grid`
   - `.provider-model-grid`
   这些不应拆成零散手写 CSS，而应沉淀为 `DataGrid` 组件或列模板 class 常量。

2. 现有样式大量依赖语义 class 和后代选择器，例如：
   - `.vault-nav a.active`
   - `.auth-tabs-line button.active::after`
   - `.generated-image:hover figcaption`
   - `.check-option:has(input:checked)`
   Tailwind 可以覆盖大部分，但 `:has()`、复杂 `::after`、深层 hover 联动要通过组件结构或 `group` 迁移。

3. 现有主题变量不能全部删除。它们仍适合作为设计 token 的来源，尤其是：
   - `--pg-user-*`
   - `--pg-admin-*`
   - `--pg-radius-*`
   - `--pg-shadow-*`
   - `--pg-duration-*`
   - `--pg-ease-out`

## 3. Tailwind 接入方案

### 3.1 版本与安装方式

采用 Tailwind CSS v4 的 Vite 插件接入方式。官方 Vite 安装方式为安装 `tailwindcss` 和 `@tailwindcss/vite`，在 Vite plugins 中加入 Tailwind 插件，并在 CSS 入口中 `@import "tailwindcss"`。

参考：

- Tailwind Vite 安装文档：https://tailwindcss.com/docs/installation/using-vite
- Tailwind PostCSS 安装文档：https://tailwindcss.com/docs/installation/using-postcss
- Tailwind v4 兼容性说明：https://tailwindcss.com/docs/compatibility

两个前端分别安装依赖：

```bash
npm --prefix web/user install -D tailwindcss @tailwindcss/vite
npm --prefix web/admin install -D tailwindcss @tailwindcss/vite
```

修改 `web/user/vite.config.ts`：

```ts
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  return {
    plugins: [tailwindcss(), react()],
    // 保留现有 server.proxy
  }
})
```

修改 `web/admin/vite.config.ts` 同理。

### 3.2 CSS 入口结构

用户端 `web/user/src/styles.css` 迁移后结构：

```css
@import "tailwindcss";
@import "../../shared/base.css";
@import "../../shared/user-theme.css";

@theme {
  --font-vault-display: "Cormorant Garamond", serif;
  --font-vault-body: "Manrope", system-ui, sans-serif;
  --font-vault-mono: "JetBrains Mono", monospace;

  --color-vault-bg: oklch(12% 0.015 260);
  --color-vault-surface: oklch(16% 0.02 260);
  --color-vault-fg: oklch(95% 0.01 80);
  --color-vault-muted: oklch(90% 0.01 80 / 0.68);
  --color-vault-border: oklch(100% 0 0 / 0.1);
  --color-vault-accent: oklch(70% 0.12 75);
  --color-vault-coral: oklch(65% 0.14 45);
  --color-vault-purple: oklch(60% 0.18 275);
  --color-vault-emerald: oklch(75% 0.15 165);

  --radius-vault: 12px;
}

@layer base {
  html { scroll-behavior: smooth; }
  body {
    margin: 0;
    min-height: 100vh;
    background: var(--bg);
    color: var(--fg);
    font-family: var(--font-body);
    -webkit-font-smoothing: antialiased;
  }
}

@layer utilities {
  .bg-vault-atmosphere {
    background:
      linear-gradient(rgba(255,255,255,.025) 1px, transparent 1px),
      linear-gradient(90deg, rgba(255,255,255,.025) 1px, transparent 1px),
      radial-gradient(circle at 18% 8%, rgba(212,157,94,.18), transparent 28%),
      radial-gradient(circle at 82% 16%, rgba(131,118,255,.15), transparent 32%),
      radial-gradient(circle at 68% 84%, rgba(65,185,149,.12), transparent 32%);
    background-size: 72px 72px, 72px 72px, auto, auto, auto;
  }
}
```

管理后台 `web/admin/src/styles.css` 迁移后结构：

```css
@import "tailwindcss";
@import "../../shared/base.css";
@import "../../shared/admin-theme.css";

@theme {
  --font-admin-display: "Fraunces", serif;
  --font-admin-body: "Manrope", system-ui, sans-serif;

  --color-admin-app: #eef2f4;
  --color-admin-card: #ffffff;
  --color-admin-subtle: #f8fafb;
  --color-admin-text: #1a2532;
  --color-admin-muted: #68788b;
  --color-admin-border: rgba(26, 37, 50, 0.08);
  --color-admin-primary: #5775b9;
  --color-admin-success: #5a9572;
  --color-admin-warning: #b88740;
  --color-admin-danger: #b85f54;
}

@layer base {
  html { background: var(--pg-admin-bg-app); }
  body {
    overflow: hidden;
    color: var(--pg-admin-text-main);
    font: 14px/1.5 "Manrope", system-ui, sans-serif;
    -webkit-font-smoothing: antialiased;
  }
}

@layer utilities {
  .bg-admin-grid {
    background:
      linear-gradient(90deg, rgba(87, 117, 185, 0.06) 1px, transparent 1px),
      linear-gradient(0deg, rgba(87, 117, 185, 0.045) 1px, transparent 1px),
      radial-gradient(circle at 78% 12%, rgba(87, 117, 185, 0.11), transparent 28%),
      var(--pg-admin-bg-app);
    background-size: 44px 44px, 44px 44px, auto, auto;
  }
}
```

说明：

- Tailwind utilities 优先直接写在 JSX。
- 复杂背景和全局 base 仍保留在 CSS。
- 不使用 CDN。
- 不引入 CSS Modules。
- 不推荐大规模使用 `@apply`，避免把 Tailwind 又写回大 CSS 文件。

### 3.3 class 拼接策略

新增轻量工具，不引入 `clsx` 也可以：

文件：`web/shared/classnames.ts`

```ts
export function cn(...items: Array<string | false | null | undefined>) {
  return items.filter(Boolean).join(' ')
}
```

如后续希望支持对象语法，可再考虑引入 `clsx`。初期建议不增加依赖。

组件里统一：

```tsx
className={cn(
  userButton.base,
  active ? userButton.primary : userButton.ghost,
  disabled && 'pointer-events-none opacity-60',
)}
```

禁止：

```tsx
className={`bg-${tone}-500`}
```

原因：Tailwind 扫描不到运行时拼出来的 class。所有 tone 都必须走静态映射。

## 4. 推荐新增 class 常量文件

### 4.1 用户端

新增：`web/user/src/ui/classes.ts`

建议内容：

```ts
export const userShell = {
  shell: 'flex h-screen overflow-hidden bg-[var(--bg)] text-[var(--fg)]',
  sidebar: 'z-20 flex w-[108px] shrink-0 flex-col items-center border-r border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_94%,black_6%)] py-5',
  main: 'flex h-screen min-w-0 flex-1 flex-col overflow-y-auto',
  topbar: 'sticky top-0 z-10 flex min-h-[76px] items-center justify-end gap-4 border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_80%,transparent)] px-10 backdrop-blur-[14px]',
}

export const userButton = {
  base: 'inline-flex items-center justify-center gap-2 rounded-full border border-[var(--border)] px-[18px] py-2.5 text-[var(--fg)] transition duration-200 ease-out hover:-translate-y-px hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))]',
  primary: 'border-[var(--accent)] bg-[var(--accent)] font-extrabold text-[var(--bg)]',
  secondary: 'bg-transparent text-[var(--fg)]',
  danger: 'text-[oklch(70%_.17_30)]',
  icon: 'inline-grid size-10 place-items-center rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)]',
}

export const userCard = {
  base: 'rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)]',
  padded: 'rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)] p-6',
}
```

### 4.2 管理后台

新增：`web/admin/src/ui/classes.ts`

建议内容：

```ts
export const adminShell = {
  root: 'grid h-screen overflow-hidden bg-admin-grid [grid-template-columns:var(--pg-sidebar-admin-width)_minmax(0,1fr)]',
  sidebar: 'flex h-screen flex-col gap-6 overflow-y-auto border-r border-[var(--line)] bg-[rgba(248,250,251,0.88)] px-5 pb-5 pt-7 backdrop-blur-[14px]',
  main: 'grid h-screen min-w-0 grid-rows-[auto_auto_minmax(0,1fr)] gap-3.5 overflow-hidden px-6 pb-6 pt-[18px]',
  topbar: 'flex min-h-[var(--pg-topbar-height)] items-center justify-between gap-4 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[rgba(255,255,255,0.82)] px-[18px] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]',
}

export const adminButton = {
  base: 'inline-flex min-h-9 items-center justify-center gap-2 rounded-[10px] border border-[var(--line)] px-3 py-2 text-sm font-bold transition hover:-translate-y-px disabled:pointer-events-none disabled:opacity-50',
  primary: 'border-[var(--blue)] bg-[var(--blue)] text-white',
  ghost: 'bg-white/55 text-[var(--text)] hover:border-[rgba(87,117,185,0.22)] hover:bg-[rgba(87,117,185,0.08)]',
  danger: 'border-[rgba(184,95,84,0.25)] bg-[rgba(184,95,84,0.1)] text-[var(--red)]',
  success: 'border-[rgba(90,149,114,0.25)] bg-[rgba(90,149,114,0.1)] text-[var(--green)]',
  small: 'min-h-8 px-2.5 py-1.5 text-xs',
}

export const adminSurface = {
  card: 'rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[rgba(255,255,255,0.82)] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]',
  lane: 'min-w-0 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white',
}
```

## 5. 用户端 CSS 替换明细

### 5.1 全局与壳布局

文件：`web/user/src/components.tsx`

当前选择器：

- `.vault-shell`
- `.vault-sidebar`
- `.vault-brand`
- `.brand-orb`
- `.vault-nav`
- `.vault-nav-bottom`
- `.vault-main`
- `.vault-topbar`
- `.quick-links`
- `.topbar-tools`
- `.top-chip`
- `.balance-chip`
- `.avatar-chip`
- `.avatar-menu-wrap`
- `.avatar-menu`
- `.content`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.vault-shell { display:flex; height:100vh; overflow:hidden; background:var(--bg); }` | `flex h-screen overflow-hidden bg-[var(--bg)]` |
| `.vault-sidebar { width:108px; ... }` | `z-20 flex w-[108px] shrink-0 flex-col items-center border-r border-[var(--border)] py-5` |
| `.vault-brand` | `grid w-full place-items-center gap-1.5 bg-transparent text-[var(--accent)] no-underline` |
| `.brand-orb` | `grid size-[42px] place-items-center rounded-full font-extrabold text-[#190f0a] shadow-[0_0_28px_rgba(212,157,94,.34)]`，背景保留为 `bg-[radial-gradient(...)]` |
| `.vault-nav` | `flex w-full flex-col gap-1` |
| `.vault-nav a` | `flex w-full flex-col items-center gap-1 border-r-2 border-transparent py-3.5 text-xs text-[var(--muted)] no-underline transition` |
| `.vault-nav a.active` | 条件 class：`border-r-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] text-[var(--accent)]` |
| `.vault-main` | `flex h-screen min-w-0 flex-1 flex-col overflow-y-auto` |
| `.vault-topbar` | `sticky top-0 z-10 flex min-h-[76px] items-center justify-end gap-4 border-b border-[var(--border)] px-10 backdrop-blur-[14px]` |
| `.quick-links` | `mr-auto flex items-center gap-3` |
| `.topbar-tools` | `flex items-center gap-2.5` |
| `.top-chip`, `.balance-chip`, `.avatar-chip` | `inline-flex min-h-9 items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)] px-3.5 py-1.5 text-sm` |
| `.avatar-menu` | `absolute right-0 top-[calc(100%+10px)] z-[60] w-46 rounded-xl border border-[var(--border)] p-2 shadow-[var(--pg-shadow-lg)] backdrop-blur` |
| `.content` | `mx-auto w-full max-w-[1200px] p-10` |

落地方式：

1. 先在 `web/user/src/ui/classes.ts` 建立 `userShell`。
2. 将 `Shell` 组件里的固定 class 替换为常量。
3. 导航 active 状态用 `cn()` 追加静态 class。
4. 删除 `styles.css` 中 52-204 行对应壳布局样式。

### 5.2 用户端按钮、表单、状态

当前选择器：

- `.btn`
- `.btn-primary`
- `.btn-secondary`
- `.btn-ghost`
- `.btn-code`
- `.create-btn`
- `.filter-btn`
- `.social-btn`
- `.feedback-btn`
- `.input`
- `.input-area`
- `.field`
- `.field-block`
- `.form-error`
- `.spinner`
- `.toast-stack`
- `.toast`
- `.state-line`
- `.empty-state`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.btn` base | `inline-flex items-center justify-center gap-2 rounded-full border border-[var(--border)] px-[18px] py-2.5 transition hover:-translate-y-px` |
| `.btn-primary` | `border-[var(--accent)] bg-[var(--accent)] font-extrabold text-[var(--bg)]` |
| `.btn-secondary`, `.btn-ghost` | `bg-transparent text-[var(--fg)]` |
| `.btn-danger` | `text-[oklch(70%_.17_30)]` |
| `.create-btn` | 在 button base 上追加 `h-[54px] w-full rounded-[14px] text-lg` |
| `.feedback-btn` | `size-11 rounded-full p-0` |
| `.input`, `input`, `textarea`, `select` | `w-full rounded-[10px] border border-[var(--border)] bg-[var(--surface)] px-3 py-2.5 text-[var(--fg)] outline-none focus:border-[var(--accent)] focus:ring-3 focus:ring-[color-mix(in_oklch,var(--accent)_18%,transparent)]` |
| `.input-area` | input base + `min-h-[120px] resize-y` |
| `.field` | `grid gap-2` |
| `.field label` | `text-xs text-[var(--muted)]` |
| `.spinner` | `size-3.5 animate-spin rounded-full border-2 border-current border-r-transparent` |
| `.toast-stack` | `fixed right-5 top-5 z-[100] grid w-[min(380px,calc(100vw-40px))] gap-3` |
| `.toast` | `grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-xl border p-3 shadow-[var(--pg-shadow-lg)] backdrop-blur` |
| `.empty-state` | `grid gap-2 rounded-[var(--radius)] border border-dashed border-[var(--border)] p-6 text-[var(--muted)]` |

落地方式：

1. 在 `web/user/src/components.tsx` 中优先迁移 `ToastViewport`、`EmptyState`、`Modal`、`FieldBlock` 一类共享组件。
2. 对页面里直接写 `<button className="btn primary">` 的位置，逐步替换为 `userButton` 常量。
3. 删除 `styles.css` 中 205-288 行的通用按钮、表单、Toast 样式。

### 5.3 Landing 与登录页

文件：

- `web/user/src/pages/LandingPage.tsx`
- `web/user/src/pages/LoginPage.tsx`

当前选择器：

- `.landing-page`
- `.topnav`
- `.container`
- `.topnav-inner`
- `.section`
- `.hero`
- `.h1`
- `.h2`
- `.lead`
- `.grid-3`
- `.feature-card`
- `.stat`
- `.split`
- `.ph-img`
- `.auth-page`
- `.auth-card`
- `.auth-logo`
- `.auth-tabs`
- `.auth-tabs-line`
- `.auth-field`
- `.password-input-wrap`
- `.password-toggle`
- `.forgot-password-link`
- `.auth-divider`
- `.btn-login`
- `.auth-social`
- `.social-login-btn`
- `.auth-footer`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.landing-page` | `min-h-screen bg-[var(--bg)]` |
| `.topnav` | `sticky top-0 z-[100] border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_82%,transparent)] backdrop-blur-2xl` |
| `.container` | `mx-auto w-[min(1180px,calc(100%-40px))]`，更推荐改成 `mx-auto w-full max-w-[1180px] px-5` |
| `.topnav-inner` | `flex items-center justify-between py-[18px]` |
| `.section` | `py-[clamp(64px,10vw,128px)]` |
| `.hero` | `text-center` |
| `.h1` | `m-0 font-vault-display text-[clamp(56px,10vw,128px)] leading-[.86] font-medium` |
| `.h2` | `m-0 font-vault-display text-[clamp(34px,6vw,72px)] leading-[.95] font-medium` |
| `.lead` | `max-w-[760px] text-lg leading-[1.7] text-[var(--muted)]` |
| `.grid-3` | `grid grid-cols-1 gap-6 md:grid-cols-3` |
| `.feature-card`, `.stat`, `.cta-strip` | `rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-7` |
| `.split` | `grid items-center gap-14 lg:grid-cols-[minmax(0,.82fr)_minmax(320px,1fr)]` |
| `.auth-page` | `grid min-h-screen place-items-center bg-[radial-gradient(...)] p-6` |
| `.auth-card` | `w-full max-w-[460px] rounded-3xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_80%,transparent)] p-12 shadow-[0_32px_64px_rgba(0,0,0,0.5)] backdrop-blur-2xl` |
| `.auth-tabs-line` | `mb-8 flex border-b border-[var(--border)]` |
| `.auth-tabs-line button` | `relative flex-1 bg-transparent pb-3 text-center text-sm text-[var(--muted)] transition after:absolute after:bottom-[-1px] after:left-0 after:right-0 after:h-0.5`；active 时追加 `text-[var(--fg)] font-bold after:bg-[var(--accent)]` |
| `.password-toggle` | `absolute right-2 top-1/2 -translate-y-1/2 rounded-lg px-2 py-1 text-xs text-[var(--muted)] hover:text-[var(--accent)]` |
| `.auth-divider` | `my-8 flex items-center text-center text-xs text-[var(--muted)] before:flex-1 before:border-b before:border-[var(--border)] after:flex-1 after:border-b after:border-[var(--border)]` |

落地方式：

1. 登录页先迁移，因为它独立、可快速截图验证。
2. Landing 页迁移时把 `.grid-3` 改成响应式 `md:grid-cols-3`，顺便修复移动端三列过窄问题。
3. 保留 `showcase-frame` 的图片展示类作为第一阶段例外，后续替换为 Tailwind。

### 5.4 工作台生图页

文件：`web/user/src/pages/WorkspacePage.tsx`

当前选择器：

- `.workspace`
- `.panel`
- `.canvas`
- `.panel-section`
- `.tabs`
- `.tab`
- `.panel-header`
- `.select-grid`
- `.select-item`
- `.upload-strip`
- `.ref-grid`
- `.ref-tile`
- `.ref-remove`
- `.canvas-placeholder`
- `.results-grid`
- `.result-card`
- `.floating-feedback`
- `.progress-band`
- `.generation-feed`
- `.generation-record`
- `.record-images`
- `.generated-image`
- `.record-pending`
- `.image-lightbox`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.workspace` | `grid h-[calc(100vh-var(--topbar-h))] grid-cols-[380px_minmax(0,1fr)] max-[1024px]:grid-cols-1 max-[1024px]:h-auto` |
| `.panel` | `flex flex-col overflow-y-auto border-r border-[var(--border)] bg-[var(--surface)]` |
| `.canvas` | `relative flex min-w-0 flex-col gap-4 overflow-y-auto p-7` |
| `.panel-section` | `border-b border-[var(--border)] p-6` |
| `.tabs` | `mb-6 grid grid-cols-2 gap-2` |
| `.tab` | `rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_78%,transparent)] px-3 py-2.5 text-center text-[var(--muted)]`，active 追加 `border-[var(--accent)] bg-[var(--accent)] text-[var(--bg)] font-extrabold` |
| `.select-grid` | `grid grid-cols-2 gap-2.5` |
| `.select-grid.three` | `grid-cols-3` |
| `.select-item` | `rounded-lg border border-[var(--border)] bg-[var(--bg)] p-2.5 text-center font-vault-mono text-sm`，active 追加 accent class |
| `.ref-grid` | `mt-3 grid grid-cols-3 gap-2` |
| `.ref-tile` | `group relative aspect-square overflow-hidden rounded-lg border border-[var(--border)] bg-[#05070d]` |
| `.ref-remove` | `absolute bottom-1.5 right-1.5 translate-y-1 rounded-md border border-white/20 bg-[#05070dcc] px-2 text-xs opacity-0 backdrop-blur transition group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100` |
| `.canvas-placeholder` | `grid min-h-[420px] flex-1 place-items-center rounded-3xl border border-[var(--border)] bg-[#0d1320] p-7 text-center text-[var(--muted)]`，复杂径向背景可保留 `bg-[radial-gradient(...)]` |
| `.results-grid` | `grid grid-cols-[repeat(auto-fit,minmax(240px,1fr))] gap-4` |
| `.floating-feedback` | `absolute bottom-11 right-11 flex gap-2.5 rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_88%,transparent)] p-2.5 backdrop-blur-2xl` |
| `.record-images` | `grid max-w-[1040px] grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3.5 pl-[52px]` |
| `.generated-image` | `group relative overflow-hidden rounded-lg border border-[var(--border)] bg-[#05070d]` |
| `.generated-image figcaption` | `absolute inset-x-2.5 bottom-2.5 flex translate-y-1 justify-end gap-1.5 opacity-0 transition group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100` |

落地方式：

1. 先迁移配置面板：tabs、select grid、reference upload。
2. 再迁移结果流：record、pending、image actions。
3. 最后迁移 lightbox 和 floating feedback。

### 5.5 图库、公开广场、详情弹窗

文件：

- `web/user/src/pages/GalleryPage.tsx`
- `web/user/src/pages/PublicGalleryPage.tsx`
- `web/user/src/components.tsx` 中 `PublicImageDetail`

当前选择器：

- `.gallery-grid`
- `.asset-card`
- `.gallery-card`
- `.asset-select`
- `.asset-thumb`
- `.gallery-preview`
- `.status-pill`
- `.pill`
- `.asset-info`
- `.gallery-meta`
- `.gallery-actions`
- `.modal-backdrop`
- `.modal-card`
- `.preview-drawer`
- `.preview-images`
- `.prompt-block`
- `.icon-action`
- `.public-detail`
- `.public-detail-media`
- `.public-detail-references`
- `.public-detail-image`
- `.public-detail-side`
- `.public-detail-prompt`
- `.public-detail-meta`
- `.public-detail-stats`
- `.public-detail-icon`
- `.delete-confirm`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.gallery-grid` | `grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-6` |
| `.asset-card`, `.gallery-card` | `relative overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)]` |
| `.asset-select` | `absolute left-3 top-3 z-10 grid size-7 place-items-center rounded-lg border border-white/15 bg-[#05070db8] backdrop-blur` |
| `.asset-thumb`, `.gallery-preview` | `grid aspect-[4/3] w-full place-items-center overflow-hidden border-0 bg-[var(--border)] p-0` |
| `.status-pill`, `.pill` | `inline-flex w-fit items-center gap-1 rounded-full bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] px-2 py-1 font-vault-mono text-[11px] text-[var(--muted)]` |
| `.status-pill.good` | `bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]` |
| `.asset-info`, `.gallery-meta` | `p-4` |
| `.gallery-actions`, `.row-actions` | `flex flex-wrap gap-2 px-4 pb-4` |
| `.modal-backdrop` | `fixed inset-0 z-[80] grid place-items-center bg-black/60 p-6` |
| `.modal-card` | `max-h-[90vh] w-[min(920px,100%)] overflow-auto rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6` |
| `.preview-drawer` | `grid grid-cols-[minmax(0,1.15fr)_minmax(280px,.85fr)] gap-5 max-[760px]:grid-cols-1` |
| `.public-detail` | `grid grid-cols-[minmax(0,1.15fr)_minmax(300px,.85fr)] items-start gap-6 max-[760px]:grid-cols-1` |
| `.public-detail-image` | `grid min-h-80 place-items-center overflow-hidden rounded-[18px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_82%,black_10%)]` |
| `.public-detail-meta` | `grid grid-cols-2 gap-2.5 rounded-[14px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3.5` |
| `.public-detail-stats` | `grid grid-cols-3 gap-2.5` |
| `.public-detail-icon`, `.icon-action` | 用 `userButton.icon` + tone 映射：liked/favorited/danger |

落地方式：

1. 先迁移 `PublicDetailIcon` 周边按钮，减少重复 icon button CSS。
2. 再迁移 `PublicImageDetail` 整体布局。
3. 最后迁移图库卡片和 modal。

### 5.6 API Keys、Profile、Docs、Checkout

文件：

- `web/user/src/pages/ApiKeysPage.tsx`
- `web/user/src/pages/ProfilePage.tsx`
- `web/user/src/pages/DocsPage.tsx`
- `web/user/src/pages/CheckoutPage.tsx`

当前选择器：

- `.card`
- `.table-panel`
- `.ledger-panel`
- `.profile-editor`
- `.balance-panel`
- `.docs-samples`
- `.docs-aside`
- `.endpoint-list`
- `.ds-table`
- `.key-table`
- `.table-head`
- `.code-block`
- `.profile-grid`
- `.stack`
- `.card-title`
- `.balance-num`
- `.docs-layout`
- `.endpoint-card`
- `.endpoint-examples`
- `.checkout-page`
- `.checkout-layout`
- `.checkout-panel`
- `.checkout-tabs`
- `.checkout-plan-grid`
- `.checkout-plan`
- `.checkout-method-grid`
- `.checkout-method`
- `.checkout-custom`
- `.checkout-order`
- `.checkout-payment-display`
- `.checkout-recent-row`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.card` 等 panel | `rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)]` |
| `.ds-table th, td` | `border-b border-[var(--border)] px-4 py-3.5 text-left` |
| `.key-table article` | `grid grid-cols-[1.2fr_1.4fr_.75fr_.5fr_.8fr_1fr] items-center gap-3 border-b border-[var(--border)] px-4 py-3.5` |
| `.code-block` | `overflow-x-auto rounded-[14px] border border-[var(--border)] bg-[#05070d] p-[18px] text-[oklch(88%_.05_90)]` |
| `.profile-grid` | `grid grid-cols-[minmax(280px,380px)_minmax(0,1fr)] gap-8 max-[760px]:grid-cols-1` |
| `.docs-layout` | `grid grid-cols-[minmax(0,1fr)_320px] gap-6 max-[1024px]:grid-cols-1` |
| `.endpoint-examples` | `mt-3 grid grid-cols-2 gap-3 max-[760px]:grid-cols-1` |
| `.checkout-layout` | `grid grid-cols-[minmax(0,1.35fr)_minmax(320px,.65fr)] items-start gap-6 max-[1024px]:grid-cols-1` |
| `.checkout-panel` | `grid min-w-0 gap-5.5 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)] p-6` |
| `.checkout-tabs` | `grid grid-cols-2 gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-1` |
| `.checkout-plan-grid` | `grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3` |
| `.checkout-plan`, `.checkout-method` | `grid cursor-pointer gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-4 text-left transition hover:border-[var(--accent)]`；active 追加 accent |
| `.checkout-custom` | `grid grid-cols-[minmax(0,1fr)_minmax(160px,.45fr)] items-end gap-4 p-[18px] max-[760px]:grid-cols-1` |
| `.checkout-recent-row` | `grid cursor-pointer gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3` |

落地方式：

1. 先把 `card/table/code` 这些可复用样式沉淀为常量。
2. Checkout 页由于样式集中且产品关键，单独一轮迁移并截图验收。
3. Docs 页中部分 inline style 同步替换为 Tailwind。

## 6. 管理后台 CSS 替换明细

### 6.1 管理后台壳布局

文件：`web/admin/src/components.tsx`

当前选择器：

- `.admin-shell`
- `.admin-sidebar`
- `.admin-brand`
- `.admin-nav`
- `.nav-group`
- `.nav-group a`
- `.admin-side-note`
- `.admin-main`
- `.admin-global-topbar`
- `.console-alert-row`
- `.console-meta-row`
- `.console-chip`
- `.console-provider-pill`
- `.avatar-widget`
- `.avatar-orb`
- `.ops-status-strip`
- `.status-cell`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.admin-shell` | `grid h-screen overflow-hidden bg-admin-grid [grid-template-columns:var(--pg-sidebar-admin-width)_minmax(0,1fr)]` |
| `.admin-sidebar` | `flex h-screen flex-col gap-6 overflow-y-auto border-r border-[var(--line)] bg-[rgba(248,250,251,0.88)] px-5 pb-5 pt-7 backdrop-blur-[14px]` |
| `.admin-brand` | `grid gap-0.5 text-[var(--blue)] no-underline` |
| `.admin-brand span` | `text-[11px] uppercase tracking-[.18em] text-[var(--soft)]` |
| `.admin-brand strong` | `text-[1.15rem] text-[var(--text)]` |
| `.admin-nav` | `grid gap-5` |
| `.nav-group` | `grid gap-1.5` |
| `.nav-group a` | `flex items-center justify-between gap-2.5 rounded-[var(--pg-radius-sm)] px-3 py-2.5 text-[var(--soft)] no-underline` |
| `.nav-group a.active` | `bg-[rgba(87,117,185,.09)] text-[var(--text)] shadow-[inset_3px_0_0_var(--blue)]` |
| `.admin-main` | `grid h-screen min-w-0 grid-rows-[auto_auto_minmax(0,1fr)] gap-3.5 overflow-hidden px-6 pb-6 pt-[18px]` |
| `.admin-global-topbar` | `flex min-h-[var(--pg-topbar-height)] items-center justify-between gap-4 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] px-[18px] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]` |
| `.console-alert-row`, `.console-meta-row` | `flex flex-wrap items-center gap-2` |
| `.console-meta-row` | `justify-end` |
| `.console-chip` | `inline-flex min-h-[34px] items-center gap-2 rounded-full border border-[var(--line)] bg-white/70 px-2.5 py-[7px] text-[var(--soft)]` |
| `.avatar-widget` | `flex items-center gap-2.5` |
| `.avatar-orb` | `grid size-10 place-items-center rounded-full bg-[var(--blue)] text-white` |
| `.ops-status-strip` | `grid grid-cols-5 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px] max-[1260px]:grid-cols-3 max-[920px]:grid-cols-2` |
| `.status-cell` | `min-w-0 border-r border-[var(--line)] px-4 py-3 last:border-r-0` |

落地方式：

1. 先迁移 `AdminLayout`，保留页面内容不动。
2. 验证 `/admin/` 登录页和登录后 overview shell。
3. 删除 `styles.css` 74-352 行对应壳布局样式。

### 6.2 管理后台按钮、Modal、表单

当前选择器：

- `.btn`
- `.ghost`
- `.btn.primary`
- `.btn.danger`
- `.btn.success`
- `.btn.small`
- `.ghost.small`
- `.modal-backdrop`
- `.modal-panel`
- `.modal-head`
- `.modal-body`
- `.modal-actions`
- `.form-grid`
- `.inline-control`
- `.field`
- `.field-label`
- `.field-hint`
- `.field-hint-popover`
- `.check-grid-scroll`
- `.check-option`
- `.tag-input-wrap`
- `.tag-list`
- `.input-tag`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.btn`, `.ghost` | `adminButton.base` |
| `.btn.primary` | `adminButton.primary` |
| `.btn.danger` | `adminButton.danger` |
| `.btn.success` | `adminButton.success` |
| `.ghost` | `adminButton.ghost` |
| `.btn.small`, `.ghost.small`, `.ghost.compact` | `adminButton.small` |
| `.modal-backdrop` | `fixed inset-0 z-[90] grid place-items-center bg-[rgba(26,37,50,0.24)] p-6 backdrop-blur-sm` |
| `.modal-panel` | `grid max-h-[92vh] w-[min(760px,calc(100vw-48px))] gap-5 overflow-auto rounded-[18px] border border-[var(--line)] bg-white p-5 shadow-[0_24px_80px_rgba(26,37,50,.18)]` |
| `.modal-head`, `.modal-actions` | `flex flex-wrap items-center justify-between gap-3` |
| `.form-grid` | `grid grid-cols-2 gap-3 max-[620px]:grid-cols-1` |
| `.form-grid .span-2` | `col-span-2 max-[620px]:col-span-1` |
| `.inline-control` | `flex items-center gap-2` |
| `.field` | `grid gap-1.5` |
| `.field-label` | `flex items-center justify-between gap-2 text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]` |
| `.field-hint` | `grid size-[18px] place-items-center rounded-full border border-[var(--line)] text-[10px] text-[var(--blue)]` |
| `.check-grid-scroll` | `grid max-h-[220px] gap-2 overflow-auto rounded-[10px] border border-[var(--line)] bg-white/60 p-2` |
| `.check-option` | `grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-[var(--line)] bg-white/70 p-2 text-sm` |
| `.check-option:has(input:checked)` | 推荐组件里给 label 加 `data-checked`，用 `data-[checked=true]:border-[var(--blue)] data-[checked=true]:bg-[rgba(87,117,185,.08)]` |
| `.tag-list` | `flex flex-wrap gap-1.5` |
| `.input-tag` | `inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-white px-2 py-1 text-xs` |

落地方式：

1. 迁移 `Button` 目前没有专用组件，建议新增 `AdminButton` 或先用 `adminButton` 常量。
2. 迁移 `Modal`、`Field`、`GroupOptionGrid`、`ConfirmDrawer`。
3. 对 `:has()` 样式改为显式 `data-checked`，减少浏览器差异和 CSS 复杂度。

### 6.3 管理后台页面骨架与运营面板

当前选择器：

- `.page-stack`
- `.page-header`
- `.page-actions`
- `.metric-grid`
- `.metric-card`
- `.overview-surface`
- `.ops-surface`
- `.config-motherboard`
- `.config-formboard`
- `.review-workspace`
- `.main-lane`
- `.full-main`
- `.signal-rail`
- `.config-side-rail`
- `.signal-section`
- `.side-strip`
- `.lane-head`
- `.section-head`
- `.lane-divider`
- `.provider-grid`
- `.queue-list`
- `.audit-list`
- `.cluster-grid`
- `.policy-stack`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.page-stack` | `grid min-h-0 gap-3 overflow-hidden` |
| `.page-header` | `flex items-center justify-between gap-4 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] p-5 shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]` |
| `.page-actions` | `flex flex-wrap items-center gap-2` |
| `.metric-grid` | `grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-3` |
| `.metric-card` | `grid gap-2 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white p-4` |
| `.overview-surface`, `.ops-surface` | `grid min-h-0 grid-cols-[minmax(0,1fr)_280px] gap-3 overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white` |
| `.ops-surface.full-main` | `grid-cols-1` |
| `.main-lane` | `min-w-0 overflow-auto p-4` |
| `.signal-rail`, `.config-side-rail` | `min-w-0 overflow-auto border-l border-[var(--line)] bg-[var(--pg-admin-bg-subtle)]` |
| `.signal-section`, `.side-strip` | `grid gap-2 border-b border-[var(--line)] p-4 last:border-b-0` |
| `.lane-head`, `.section-head` | `flex flex-wrap items-center justify-between gap-2 border-b border-[var(--line)] pb-3` |
| `.provider-row`, `.queue-row`, `.audit-row`, `.policy-line`, `.compact-item`, `.timeline-item` | `grid gap-1 border-t border-[var(--line)] py-3 first:border-t-0`，按具体结构追加 grid/flex |

落地方式：

1. `PageHeader`、`MetricGrid`、`EmptyBlock`、`LoadingBlock` 先迁移。
2. `OverviewPage`、`HealthPage`、`AuditPage` 迁移页面骨架。
3. 再迁移复杂的 `ConfigPage` 和 `CashierPage`。

### 6.4 管理后台 DataGrid

当前选择器：

- `.admin-data-grid`
- `.table-head`
- `.table-row`
- `.route-grid`
- `.price-grid`
- `.account-grid`
- `.route-model-grid`
- `.candidate-grid`
- `.route-price-grid`
- `.user-group-grid`
- `.review-grid`
- `.users-grid`
- `.redeem-grid`
- `.redeem-redemption-grid`
- `.health-grid`
- `.call-record-grid`
- `.readiness-grid`
- `.overview-readiness-grid`
- `.user-detail-bucket-grid`
- `.user-detail-ledger-grid`
- `.user-detail-order-grid`
- `.user-detail-task-grid`
- `.user-detail-api-key-grid`
- `.cashier-plan-grid`
- `.cashier-method-grid`
- `.editable-method-grid`
- `.cashier-instance-grid`
- `.cashier-order-grid`
- `.cashier-event-grid`
- `.config-board-grid`

迁移原则：

- 不建议在每个页面里直接堆很长的 `grid-cols-[...]`。
- 建议新增 `web/admin/src/ui/dataGrid.ts`，集中维护列模板。

示例：

```ts
export const adminDataGrid = {
  root: 'min-w-0 overflow-x-auto rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white',
  head: 'grid items-center gap-3 border-b border-[var(--line)] bg-[var(--pg-admin-bg-subtle)] px-4 py-3 text-[10px] font-extrabold uppercase tracking-[.12em] text-[var(--soft)]',
  row: 'grid items-center gap-3 border-b border-[var(--line)] px-4 py-3 last:border-b-0',
  cell: 'min-w-0 overflow-hidden text-ellipsis',
  actions: 'flex flex-wrap items-center justify-end gap-2',
}

export const adminGridCols = {
  users: 'min-w-[1040px] [grid-template-columns:minmax(240px,2.5fr)_minmax(80px,1fr)_minmax(120px,1fr)_minmax(80px,1fr)_minmax(80px,1fr)_minmax(80px,1fr)_minmax(280px,3fr)]',
  callRecords: 'min-w-[1180px] [grid-template-columns:minmax(210px,2fr)_minmax(130px,1.1fr)_minmax(120px,1fr)_minmax(120px,1fr)_minmax(84px,.7fr)_minmax(240px,2.2fr)_minmax(150px,1.3fr)_minmax(170px,1.5fr)]',
  cashierPlans: 'min-w-[1040px] [grid-template-columns:minmax(220px,2fr)_minmax(130px,1fr)_minmax(90px,.8fr)_minmax(90px,.8fr)_minmax(72px,.65fr)_minmax(86px,.75fr)_minmax(86px,.75fr)_minmax(76px,.7fr)]',
}
```

页面替换示例：

```tsx
<div className={cn(adminDataGrid.root, adminGridCols.users)}>
  <div className={cn(adminDataGrid.head, adminGridCols.users)}>...</div>
  {rows.map((row) => (
    <article className={cn(adminDataGrid.row, adminGridCols.users)}>...</article>
  ))}
</div>
```

注意：

- Tailwind arbitrary property 使用 `_` 代表空格，列模板必须静态字符串。
- 不要运行时拼接列模板。
- 每个原 `.xxx-grid` 先一比一搬到 `adminGridCols`，后续再考虑统一列宽。

### 6.5 管理后台 Badge、状态、Toast

当前选择器：

- `.badge`
- `.badge.success`
- `.badge.warning`
- `.badge.danger`
- `.badge.primary`
- `.inline-feedback`
- `.toast-rail`
- `.toast`
- `.toast.success`
- `.toast.warning`
- `.toast.danger`
- `.toast.neutral`
- `.loader`
- `.state-block`

替换策略：

| 当前 CSS | Tailwind 替换 |
| --- | --- |
| `.badge` | `inline-flex w-fit items-center rounded-full px-2 py-1 text-[11px] font-extrabold` |
| `.badge.success` | `bg-[rgba(90,149,114,.12)] text-[var(--green)]` |
| `.badge.warning` | `bg-[rgba(184,135,64,.13)] text-[var(--amber)]` |
| `.badge.danger` | `bg-[rgba(184,95,84,.13)] text-[var(--red)]` |
| `.badge.primary` | `bg-[rgba(87,117,185,.12)] text-[var(--blue)]` |
| `.inline-feedback` | `rounded-[10px] border px-3 py-2 text-sm` + tone map |
| `.toast-rail` | `fixed right-5 top-5 z-[120] grid w-[min(380px,calc(100vw-40px))] gap-2` |
| `.toast` | `grid rounded-xl border border-[var(--line)] bg-white p-3 text-left shadow-[var(--pg-shadow-sm)]` + tone border-left class |
| `.loader` | `size-4 animate-spin rounded-full border-2 border-[var(--line)] border-t-[var(--blue)]` |
| `.state-block` | `grid place-items-center gap-2 rounded-[var(--pg-radius-sm)] border border-dashed border-[var(--line)] p-8 text-center` |

落地方式：

1. `Badge`、`InlineFeedback`、`ToastRail` 先迁移。
2. 迁移所有页面状态块。
3. 删除 `styles.css` 中对应状态类。

### 6.6 管理后台复杂页面专项

#### CashierPage

文件：`web/admin/src/pages/CashierPage.tsx`

当前高频选择器：

- `.cashier-tabs`
- `.cashier-overview-grid`
- `.cashier-overview-card`
- `.cashier-section`
- `.cashier-config-form`
- `.cashier-toggle`
- `.cashier-amount-grid`
- `.cashier-method-toolbar`
- `.cashier-order-filter`
- `.cashier-risk-grid`
- `.cashier-risk-item`
- `.cashier-webhook-inspector`
- `.cashier-provider-guide`
- `.cashier-jeepay-template`
- `.cashier-structured-config`

替换方式：

- tabs：使用 `grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-2 rounded-xl border bg-white/70 p-1`。
- overview card：使用 `grid gap-2 rounded-xl border border-[var(--line)] bg-white p-4`。
- section：使用 `grid gap-4 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white p-4`。
- risk item：使用 tone map：
  - success：`border-[rgba(90,149,114,.24)] bg-[rgba(90,149,114,.08)]`
  - warning：`border-[rgba(184,135,64,.28)] bg-[rgba(184,135,64,.08)]`
  - danger：`border-[rgba(184,95,84,.28)] bg-[rgba(184,95,84,.08)]`
- webhook inspector：`grid gap-2 rounded-xl border border-[var(--line)] bg-[#111827] p-4 text-white`。

#### ConfigPage

文件：`web/admin/src/pages/ConfigPage.tsx`

当前高频选择器：

- `.config-motherboard`
- `.config-formboard`
- `.config-category-rail`
- `.config-form-lane`
- `.config-form-grid`
- `.config-form-item`
- `.config-kv-list`
- `.config-kv-row`
- `.config-sheet-lane`
- `.config-toolbar-band`
- `.config-mode-tabs`
- `.micro-tabs`
- `.config-sheet-row`
- `.has-conflict`
- `.reason-drawer`

替换方式：

- 保留“左分类 + 右表单/表格”的 layout，但用 Tailwind grid：
  - `grid grid-cols-[220px_minmax(0,1fr)] gap-3`
  - 移动端 `max-[920px]:grid-cols-1`
- 分类 rail button：`rounded-lg px-3 py-2 text-left text-sm text-[var(--soft)] hover:bg-[rgba(87,117,185,.08)]`，active 追加主色。
- 表单项：`grid gap-2 rounded-xl border border-[var(--line)] bg-white p-3`。
- 冲突行：`border-[rgba(184,135,64,.35)] bg-[rgba(184,135,64,.08)]`。
- reason drawer：`fixed bottom-5 right-5 z-[100] grid w-[min(420px,calc(100vw-40px))] gap-3 rounded-2xl border border-[var(--line)] bg-white p-4 shadow-[0_20px_70px_rgba(26,37,50,.18)]`。

## 7. 分阶段执行计划

### Phase 0：基线与保护

目标：迁移前拿到视觉和构建基线。

任务：

1. 确认当前全栈可访问：
   - 用户端：`http://127.0.0.1:8088/`
   - 管理后台：`http://127.0.0.1:8088/admin/`
2. 截图基线：
   - 用户端：landing、login、home、workspace、gallery、public gallery、checkout、docs。
   - 管理端：login、overview、users、cashier、config、call records。
3. 保存当前 CSS 规模：
   - `wc -l web/user/src/styles.css web/admin/src/styles.css`
   - `rg -n "className=" web/user/src web/admin/src`
4. 运行：
   - `npm --prefix web/user run typecheck`
   - `npm --prefix web/user run build`
   - `npm --prefix web/admin run typecheck`
   - `npm --prefix web/admin run build`

验收：

- 当前基线截图保存到 `.review/screenshots/tailwind-before/` 或等价目录。
- 构建通过。

### Phase 1：接入 Tailwind，不改视觉

目标：让两个 Vite 应用具备 Tailwind 能力，但不大规模改 JSX。

任务：

1. 安装 `tailwindcss` 和 `@tailwindcss/vite`。
2. 修改两个 `vite.config.ts`。
3. 在两个 `styles.css` 顶部加入 `@import "tailwindcss";`。
4. 增加 `@theme` tokens。
5. 新增 `web/shared/classnames.ts`。
6. 跑两个前端 build。

验收：

- Tailwind utility 在一个临时元素上可生效，随后删除临时元素。
- 原页面视觉无明显变化。

### Phase 2：迁移共享原子组件

目标：先消灭高频 class。

用户端：

- `Shell`
- `ToastViewport`
- `Modal`
- `EmptyState`
- `ImageLightbox`
- `PublicDetailIcon`
- `FieldBlock` 或等价表单小组件

管理端：

- `AdminLayout`
- `PageHeader`
- `StatusItem`
- `StatusCell`
- `ToastRail`
- `Badge`
- `InlineFeedback`
- `Modal`
- `Field`
- `GroupOptionGrid`
- `ConfirmDrawer`
- `MetricGrid`

验收：

- 高频旧 class 使用量显著下降：
  - `btn`
  - `ghost`
  - `card`
  - `pg-admin-card`
  - `badge`
  - `toast`
  - `field`
  - `modal-*`
- 用户端 shell 和管理端 shell 在桌面、920px、620px 下不破版。

### Phase 3：用户端页面迁移

建议顺序：

1. `LoginPage.tsx`
2. `LandingPage.tsx`
3. `ApiKeysPage.tsx`
4. `ProfilePage.tsx`
5. `DocsPage.tsx`
6. `CheckoutPage.tsx`
7. `GalleryPage.tsx`
8. `PublicGalleryPage.tsx`
9. `WorkspacePage.tsx`
10. `HomePage.tsx`

原因：

- 登录和 Landing 独立，风险最低。
- API Keys/Profile/Docs 样式偏静态。
- Checkout 是产品关键页，需要单独验收。
- Gallery/Public/Workspace 交互复杂，应在共享组件迁移稳定后做。

每个页面完成后：

1. 删除对应 CSS 段。
2. 跑 `npm --prefix web/user run typecheck`。
3. 跑 `npm --prefix web/user run build`。
4. 截图对比桌面和移动。

### Phase 4：管理后台页面迁移

建议顺序：

1. `OverviewPage.tsx`
2. `HealthPage.tsx`
3. `ReadinessPage.tsx`
4. `AuditPage.tsx`
5. `CallRecordsPage.tsx`
6. `UserGroupsPage.tsx`
7. `UsersPage.tsx`
8. `RedeemPage.tsx`
9. `RoutingPage.tsx`
10. `PricingPage.tsx`
11. `ProviderModelsPage.tsx`
12. `ReviewPage.tsx`
13. `CashierPage.tsx`
14. `ConfigPage.tsx`

原因：

- 先迁移结构简单的只读页。
- 中段迁移 data-grid 类页面。
- `CashierPage`、`ConfigPage` 结构最复杂，最后迁移。

每个页面完成后：

1. 将对应 `.xxx-grid` 列模板迁入 `adminGridCols`。
2. 删除对应 CSS 段。
3. 跑 `npm --prefix web/admin run typecheck`。
4. 跑 `npm --prefix web/admin run build`。
5. 截图对比。

### Phase 5：清理与准入规则

任务：

1. 删除已无引用的 CSS class。
2. 保留 CSS 文件中的内容应只包括：
   - Tailwind import。
   - shared theme import。
   - `@theme`。
   - `@layer base`。
   - 少量复杂背景 utilities。
   - 少量过渡期 legacy class，必须带注释和迁移 TODO。
3. 新增文档规则：
   - 新页面禁止新增页面级大段 CSS。
   - 新增状态样式必须走静态 class map。
   - DataGrid 新列模板统一写入 `adminGridCols`。
4. 可选：增加 lint 脚本扫描新增 CSS 行数或禁止新增非白名单 class。

验收：

- `web/user/src/styles.css` 小于 250 行。
- `web/admin/src/styles.css` 小于 300 行。
- 两端构建通过。
- 关键路由截图通过人工验收。

## 8. 不建议做的事

1. 不建议一次性全量替换所有 class。
   - 风险：视觉回归太大，review 无法判断问题来源。

2. 不建议大量使用 `@apply`。
   - 风险：只是把手写 CSS 换成 Tailwind CSS 文件，维护模型没有变。

3. 不建议保留动态拼接 class。
   - 错误示例：`bg-${tone}-500`。
   - 正确方式：`toneClass[tone]` 静态映射。

4. 不建议直接删除所有主题变量。
   - 这些变量目前承载品牌色、阴影、半径、动效曲线和跨应用设计一致性。

5. 不建议在页面里到处复制超长 class。
   - 高频组合应抽成 `userButton`、`adminButton`、`adminDataGrid` 这种常量。

## 9. 验收命令

每个阶段至少执行：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

如果同时修改 Docker、Nginx 或构建产物，再执行：

```bash
docker compose --env-file deployments/docker-compose/.env.example -f deployments/docker-compose/docker-compose.dev.yml up -d --build
curl -I http://127.0.0.1:8088/
curl -I http://127.0.0.1:8088/admin/
```

建议人工验收视口：

- Desktop：1440 x 1000
- Laptop：1280 x 800
- Tablet：920 x 1100
- Mobile：390 x 844

## 10. 预估工作量

按可安全交付的粒度拆分：

- Tailwind 接入与 token 基建：0.5 天。
- 用户端共享组件迁移：1 天。
- 用户端页面迁移：2 至 3 天。
- 管理端共享组件迁移：1 天。
- 管理端 DataGrid 抽象与普通页面迁移：2 天。
- CashierPage 与 ConfigPage 专项迁移：2 天。
- 视觉回归、截图验收、CSS 清理：1 至 1.5 天。

总计：约 9 至 11 个工作日。

如果接受“先迁移 80% 高频样式，保留复杂页面少量 legacy CSS”，第一阶段可压缩到约 4 至 5 个工作日。

## 11. 最终交付物

完成迁移后应具备：

- `web/user` 和 `web/admin` 均接入 Tailwind。
- `web/shared/classnames.ts`。
- `web/user/src/ui/classes.ts`。
- `web/admin/src/ui/classes.ts`。
- `web/admin/src/ui/dataGrid.ts`。
- 用户端大部分页面不再依赖页面级手写 CSS。
- 管理端 DataGrid 列模板集中管理。
- README 或开发指南补充 Tailwind 使用规则。
- 截图验收记录和构建结果记录。

## 12. 2026-06-07 当前实施快照

本节是基于当前工作区重新扫描后的状态，不再沿用早期“尚未迁移”的判断。后续执行以第 13 节为准。

### 12.1 当前前端项目

当前仓库只有两个真实前端应用，另有共享前端模块：

| 项目 | 入口 | 路由模式 | 样式入口 | Tailwind 状态 |
| --- | --- | --- | --- | --- |
| 用户端 | `web/user/src/App.tsx` | hash route，例如 `#/genpic` | `web/user/src/styles.css` | 已接入 Tailwind v4，仍有少量 legacy class |
| 管理后台 | `web/admin/src/App.tsx` | hash route，例如 `#/cashier`，Nginx 下通过 `/admin/` 访问 | `web/admin/src/styles.css` | 已接入 Tailwind v4，仍有较多过渡锚点 |
| 共享模块 | `web/shared/*` | 无独立路由 | `base.css`、`tokens.css`、`user-theme.css`、`admin-theme.css` | 继续保留为 token/base 层 |

当前依赖状态：

- `web/user/package.json`、`web/admin/package.json` 均已安装 `tailwindcss`、`@tailwindcss/vite`。
- `web/user/vite.config.ts`、`web/admin/vite.config.ts` 均已接入 Tailwind Vite plugin。
- `web/shared/classnames.ts` 已提供 `cn(...items)`。
- `web/user/src/ui/classes.ts` 已提供 `userShell`、`userButton`、`userForm`、`userState`、`userCard`。
- `web/admin/src/ui/classes.ts` 已提供 `adminShell`、`adminButton`、`adminSurface`。
- `web/admin/src/ui/dataGrid.ts` 已提供 `adminDataGrid`、`adminGridCols`，不需要重新创建。

### 12.2 当前样式规模

扫描命令：

```bash
wc -l web/user/src/styles.css web/admin/src/styles.css
rg -n 'className="(btn|btn |ghost|ghost |admin-data-grid|table-head|table-row|status-pill|pill|image-lightbox|delete-confirm|group-editor|config-|permission-note)' web/user/src web/admin/src
rg -n '"[^"]*(signal-rail|signal-section|muted-action|editable-row|wide|login-panel|list-toolbar|pagination-row|lane-head|user-detail-stack|user-detail-section|micro-tabs|danger-text|filter-band|page-stack|pg-admin-card|ops-surface|overview-surface|form-grid|ops-status-strip|status-cell|check-grid-scroll|check-option|admin-data-grid|table-head|table-row|btn btn|btn-primary|btn-ghost|status-pill|image-lightbox|delete-confirm|group-editor)[^"]*"' web/user/src web/admin/src -g '*.tsx' -g '*.ts'
```

结果：

- `web/user/src/styles.css`：94 行。
- `web/admin/src/styles.css`：116 行。
- 用户端 `btn/status-pill/image-lightbox/delete-confirm/group-editor` 等收尾锚点已清空。
- 管理端共享组件与 `ConfigPage`、`AuditPage` 的过渡锚点已清空。
- 管理端页面源码已清空 `page-stack`、`pg-admin-card`、`ops-surface`、`overview-surface`、`form-grid`、`ops-status-strip`、`status-cell`、`check-grid-scroll`、`check-option` 等页面骨架 legacy class。
- 管理端 `signal-rail/signal-section/muted-action/editable-row/wide/login-panel/list-toolbar/pagination-row/lane-head/user-detail-stack/user-detail-section/micro-tabs/danger-text/filter-band` 等剩余裸旧类已迁移到 `adminPage`、页面常量或 Tailwind utilities。
- 管理端 `web/admin/src/styles.css` 已只保留 Tailwind import、主题变量、base 表单/文本样式、滚动条、背景 utility 和移动端 body overflow 兼容。

结论：

- 用户端已完成第一轮收尾，`styles.css` 只保留 Tailwind import、主题变量、base、复杂背景、toast 动画和极少量全局工具样式。
- 管理端已完成 Tailwind 接入、DataGrid 抽象、Config/Audit 页面迁移、共享组件去锚点、页面骨架批量清理和最终 CSS 压缩。
- 当前目标不再是“接入 Tailwind”，后续若继续推进，应聚焦视觉回归截图、README/开发指南补充 Tailwind 约束，以及未来新页面禁止新增大段页面级 CSS。

### 12.3 路由到页面映射

用户端路由：

| 路由 | 页面文件 | 本轮迁移重点 |
| --- | --- | --- |
| `#/landing` | `web/user/src/pages/LandingPage.tsx` | 已清空裸 `btn btn-primary`、`btn btn-ghost` |
| `#/login` | `web/user/src/pages/LoginPage.tsx` | 已清理 `loginClasses` 中 `btn`、`btn-login`、`social-login-btn` 过渡锚点 |
| `#/home` | `web/user/src/pages/HomePage.tsx` | 已基本 Tailwind 化，仅保留类常量 |
| `#/genpic` | `web/user/src/pages/WorkspacePage.tsx` | 已替换 `status-pill` |
| `#/gallery` | `web/user/src/pages/GalleryPage.tsx` | 已替换 `delete-confirm*`、`group-editor*` |
| `#/public-gallery` | `web/user/src/pages/PublicGalleryPage.tsx` | 已基本 Tailwind 化 |
| `#/checkout` | `web/user/src/pages/CheckoutPage.tsx` | 已基本 Tailwind 化，复查自定义金额输入 |
| `#/api-keys` | `web/user/src/pages/ApiKeysPage.tsx` | 已替换列表操作裸 `btn btn-ghost`、scope/form 弹窗旧类 |
| `#/profile` | `web/user/src/pages/ProfilePage.tsx` | 已替换基本信息表单 `input` 旧类 |
| `#/docs` | `web/user/src/pages/DocsPage.tsx` | 已替换 `status-pill neutral` |

管理端路由：

| 路由 | 页面文件 | 本轮迁移重点 |
| --- | --- | --- |
| `#/login` | `web/admin/src/pages/LoginPage.tsx` | 已基本 Tailwind 化 |
| `#/overview` | `web/admin/src/pages/OverviewPage.tsx` | 已清理 `page-stack` 过渡锚点 |
| `#/config` | `web/admin/src/pages/ConfigPage.tsx` | 已迁移 `config-*`、`permission-note`、裸 `btn/ghost` |
| `#/readiness` | `web/admin/src/pages/ReadinessPage.tsx` | 已清理 `page-stack`、`pg-admin-card ops-surface full-main` |
| `#/routing` | `web/admin/src/pages/RoutingPage.tsx` | 已清理 `page-stack`、`form-grid`、`ops-surface` |
| `#/pricing` | `web/admin/src/pages/PricingPage.tsx` | 已清理 `page-stack`、`overview-surface`、`form-grid` |
| `#/reviews` | `web/admin/src/pages/ReviewPage.tsx` | 已清理 `page-stack`、`review-workspace` 过渡锚点 |
| `#/users` | `web/admin/src/pages/UsersPage.tsx` | 已清理 `page-stack`、`form-grid`、用户详情弹窗过渡类 |
| `#/user-groups` | `web/admin/src/pages/UserGroupsPage.tsx` | 已清理 `page-stack`、`form-grid` |
| `#/redeem` | `web/admin/src/pages/RedeemPage.tsx` | 已清理 `page-stack`、`form-grid` |
| `#/cashier` | `web/admin/src/pages/CashierPage.tsx` | 已完成主要迁移并清理 `form-grid`、`check-option` 过渡锚点 |
| `#/call-records` | `web/admin/src/pages/CallRecordsPage.tsx` | 已清理 `page-stack`、`ops-surface` |
| `#/provider-models` | `web/admin/src/pages/ProviderModelsPage.tsx` | 已清理 `page-stack`、`form-grid`、`check-grid-scroll/check-option` |
| `#/audit` | `web/admin/src/pages/AuditPage.tsx` | 已迁移 `ghost`、`btn`、`filter-row`、`timeline-item` |
| `#/health` | `web/admin/src/pages/HealthPage.tsx` | 已清理 `page-stack`、`pg-admin-card ops-surface full-main` |

## 13. 当前代码精确执行矩阵

目标：把当前代码中剩余的手写样式按批次迁走。每批都必须先改 TSX/常量，再删除对应 CSS，并用 `rg` 反查确认无引用。

### 13.1 用户端执行矩阵

#### 13.1.1 用户端类常量先去 legacy 锚点

文件：`web/user/src/ui/classes.ts`

| 当前常量 | 当前过渡锚点 | 替换方案 |
| --- | --- | --- |
| `userShell.shell` | `vault-shell` | 保留后面的 Tailwind：`flex h-screen overflow-hidden bg-[var(--bg)] text-[var(--fg)]` |
| `userShell.sidebar` | `vault-sidebar` | 保留 Tailwind，并补移动端：`max-[780px]:sticky max-[780px]:top-0 max-[780px]:h-auto max-[780px]:w-full max-[780px]:flex-row max-[780px]:overflow-x-auto max-[780px]:p-2` |
| `userShell.brand` | `vault-brand` | 保留 Tailwind；移动端宽度用 `max-[780px]:w-auto max-[780px]:min-w-16` |
| `userShell.nav/navBottom/navLink/navLinkActive` | `vault-nav*`、`active` | 全部改为纯 Tailwind；active 不再需要字符串 `active` |
| `userShell.main/topbar/quickLinks/topbarTools/content` | `vault-main`、`vault-topbar`、`quick-links`、`topbar-tools`、`content` | 把现有 CSS 媒体查询搬进常量：`max-[780px]:h-auto`、`max-[780px]:min-h-screen`、`max-[780px]:flex-wrap`、`max-[780px]:p-4` |
| `userButton.base/primary/ghost/danger` | `btn`、`btn-primary`、`btn-ghost`、`btn-danger` | 删除旧锚点，只保留 Tailwind utilities |
| `userForm.field/input/textarea` | `field-block`、`input`、`input-area` | 删除旧锚点；全局 `input,textarea,select` 样式删除前确保所有页面使用 `userForm` 或页面常量 |
| `userState.spinner/stateLine/empty/toastStack/toast/modalBackdrop/modalCard` | `spinner`、`state-line`、`empty-state`、`toast*`、`modal-*` | 删除旧锚点；Modal 头部和图片预览完成后再删 CSS |

完成后可删除用户端 CSS 中：

- `.vault-shell` 到 `.content` 的 shell 段。
- `.btn*`、`.input*`、`.field*`、`.spinner`、`.toast*`、`.state-line`、`.empty-state`。
- `@media (max-width: 780px)` 中针对上述类的规则。

#### 13.1.2 `web/user/src/components.tsx`

| 旧 class/结构 | 当前用途 | 替换方案 |
| --- | --- | --- |
| `image-lightbox` | 图片预览遮罩 | 直接改为 `fixed inset-0 z-[100] grid cursor-zoom-out place-items-center bg-black/85 p-8 backdrop-blur-[10px]` |
| `image-lightbox img` | 预览图片尺寸 | `<img>` 加 `max-h-[92vh] max-w-[min(100%,1440px)] cursor-default rounded-[10px] object-contain shadow-[0_24px_80px_rgba(0,0,0,.45)]` |
| `image-lightbox-close` | 图片预览关闭按钮 | 直接改为 `fixed right-5 top-5 grid size-[42px] place-items-center rounded-full border border-white/20 bg-[#05070db8] text-[28px] leading-none text-[var(--fg)]` |
| `modal-head` | Modal 标题行 | 改为 `flex items-center justify-between gap-5` |
| `StatusPill` 的 ``status-pill ${tone}`` | 用户端状态胶囊 | 在 `web/user/src/ui/classes.ts` 新增 `userPill`，并替换为 `cn(userPill.base, userPill[tone] ?? userPill.neutral)` |

`userPill` 建议定义：

```ts
export const userPill = {
  base: 'inline-flex w-fit items-center gap-1 rounded-full bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] px-2 py-1 font-vault-mono text-[11px] text-[var(--muted)]',
  neutral: 'bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] text-[var(--muted)]',
  good: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]',
  public: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]',
  warn: 'bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] text-[var(--accent)]',
  bad: 'bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] text-[oklch(76%_.14_35)]',
}
```

#### 13.1.3 `web/user/src/pages/LandingPage.tsx`

当前裸旧类：

- `btn btn-primary`
- `btn btn-ghost`
- `btn btn-primary px-12 py-[18px] text-lg`

替换：

```tsx
className={cn(userButton.base, userButton.primary)}
className={cn(userButton.base, userButton.ghost)}
className={cn(userButton.base, userButton.primary, 'px-12 py-[18px] text-lg')}
```

文件需导入：

```ts
import { cn } from '../../../shared/classnames'
import { userButton } from '../ui/classes'
```

#### 13.1.4 `web/user/src/pages/LoginPage.tsx`

当前问题不是裸 className，而是 `loginClasses` 中仍含 `btn`、`btn-login`、`social-login-btn`。

替换：

| 常量 | 当前旧锚点 | 替换方案 |
| --- | --- | --- |
| `codeButton` | `btn` | 删除 `btn`，保留其余 Tailwind |
| `submit` | `btn-login` | 删除 `btn-login`，保留 Tailwind |
| `socialButton` | `social-login-btn` | 删除 `social-login-btn`，保留 Tailwind |

完成后删除 CSS 中 `.auth-*` 里已经无引用的旧按钮规则。

#### 13.1.5 `web/user/src/pages/ApiKeysPage.tsx`

当前裸旧类：

- 4 个列表操作按钮：`className="btn btn-ghost"`。
- 删除按钮：`cn('btn btn-ghost', apiKeyClasses.dangerButton)`。
- 弹窗表单：`modal-form`、`scope-grid`。

替换：

| 旧写法 | 新写法 |
| --- | --- |
| `className="btn btn-ghost"` | `className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)}` |
| `cn('btn btn-ghost', apiKeyClasses.dangerButton)` | `cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction, apiKeyClasses.dangerButton)` |
| `modal-form` | `grid gap-4` |
| `scope-grid` | `grid gap-2` 或页面常量 `scopeGrid` |

Scope checkbox label 推荐：

```tsx
className="inline-flex items-center gap-2 rounded-lg border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] px-3 py-2 text-sm text-[var(--muted)] has-[:checked]:border-[var(--accent)] has-[:checked]:text-[var(--fg)]"
```

所有弹窗 input/select/textarea 统一使用 `userForm.input`、`userForm.textarea`。

#### 13.1.6 `web/user/src/pages/ProfilePage.tsx`

当前旧类：

- `<input className="input">`
- `<textarea className="input">`
- 兑换码 input 需要复查是否已绑定 `userForm.input`。

替换：

```tsx
<input className={userForm.input} ... />
<textarea className={userForm.textarea} ... />
```

#### 13.1.7 `web/user/src/pages/DocsPage.tsx` 和 `WorkspacePage.tsx`

| 文件 | 旧写法 | 替换方案 |
| --- | --- | --- |
| `DocsPage.tsx` | `status-pill neutral` | `cn(userPill.base, userPill.neutral)` |
| `WorkspacePage.tsx` | ``status-pill ${card.statusTone}`` | `cn(userPill.base, userPill[card.statusTone] ?? userPill.neutral)` |

完成后删除 `web/user/src/styles.css` 的 `.status-pill*`、`.pill*`，同时把 `code, .num, .badge, .status-pill, .pill` 改为只保留仍存在的 `.num` 或直接迁移 `.num`。

#### 13.1.8 `web/user/src/pages/GalleryPage.tsx`

替换矩阵：

| 旧 class | 新 Tailwind |
| --- | --- |
| `delete-confirm` | `grid grid-cols-[42px_minmax(0,1fr)] items-start gap-4` |
| `delete-confirm-mark` | `grid size-[42px] place-items-center rounded-xl border border-[color-mix(in_oklch,var(--accent-coral)_42%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)] text-[oklch(78%_.14_35)]` |
| `delete-confirm h3` | 给标题加 `m-0 mb-2 text-xl` |
| `delete-confirm p` | 给说明加 `m-0 leading-[1.65] text-[var(--muted)]` |
| `delete-confirm-list` | `col-span-full flex flex-wrap gap-2 rounded-[10px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3` |
| `delete-confirm-list span` | `max-w-full overflow-hidden text-ellipsis whitespace-nowrap rounded-full bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2 py-1 text-xs text-[var(--muted)]` |
| `delete-confirm-actions` | `col-span-full flex justify-end gap-2` |
| `group-editor` | `grid gap-4` |
| `group-editor label` | `grid gap-2 text-sm text-[var(--muted)]` |
| `group-editor input` | `userForm.input` |
| `group-editor p` | `m-0 text-[var(--muted)]` |

完成后删除 `.delete-confirm*`、`.group-editor*`。

### 13.2 管理端执行矩阵

#### 13.2.1 管理端类常量先去 legacy 锚点

文件：`web/admin/src/ui/classes.ts`

| 常量 | 当前过渡锚点 | 替换方案 |
| --- | --- | --- |
| `adminShell.root/sidebar/brand/nav/navGroup/navLink/navLinkActive/sideNote/main/topbar` | `admin-shell`、`admin-sidebar`、`admin-brand`、`nav-group`、`nav-item`、`admin-main`、`admin-global-topbar` | 删除旧锚点，只保留 Tailwind |
| `adminShell.metaRow/chip/avatarWidget/avatarOrb/statusStrip/statusCell` | `console-meta-row`、`console-chip`、`avatar-widget`、`avatar-orb`、`ops-status-strip`、`status-cell` | 删除旧锚点；`StatusCell` 子元素样式直接写进组件 |
| `adminButton.base/primary/ghost/danger/success/small` | `btn`、`primary`、`ghost`、`danger`、`success`、`small` | 删除旧锚点，避免 CSS `.btn/.ghost` 继续生效 |
| `adminSurface.card` | `pg-admin-card` | 删除旧锚点，页面需要卡片时直接使用 `adminSurface.card` |

删除前必须先完成 `components.tsx` 和页面里的裸旧类替换，否则 CSS 会被过早移除。

#### 13.2.2 `web/admin/src/components.tsx`

当前组件层剩余过渡锚点较多，应优先迁移，因为它们影响所有后台页面。

| 位置/旧 class | 替换方案 |
| --- | --- |
| `nav-label` | 给 `<p>` 直接加 `px-3 text-[10px] font-extrabold uppercase tracking-[.16em] text-[var(--soft)]` |
| `console-provider-pill` | 改为纯 Tailwind：`inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-white/70 px-3 py-2 text-[var(--soft)]` |
| `pill-dot`、`chip-dot` | 改为 `inline-block size-2 rounded-full bg-[var(--green)]`；warning/danger 用静态 tone map |
| `toast-rail` | 删除旧锚点，保留 `fixed right-5 top-5 z-[120] grid w-[min(380px,calc(100vw-40px))] gap-2` |
| `toast` | 删除旧锚点，保留 `grid rounded-xl border border-[var(--line)] bg-white p-3 text-left shadow-[var(--pg-shadow-sm)]` |
| `page-header`、`page-actions` | 删除旧锚点，保留现有 Tailwind |
| `state-block loading/error/empty` | 删除旧锚点，保留现有 Tailwind；颜色通过 tone map |
| `badge` | 删除旧锚点，保留 `inline-flex w-fit items-center rounded-full px-2 py-1 text-[11px] font-extrabold` |
| `inline-feedback` | 删除旧锚点，保留 `rounded-[10px] border px-3 py-2 text-sm` |
| `check-option` | 删除旧锚点，保留 `grid grid-cols-[auto_minmax(0,1fr)_auto] ... has-[:checked]:...` |
| `modal-backdrop/modal-panel/modal-head/modal-body/modal-actions` | 删除旧锚点，保留现有 Tailwind |
| `field/field-label/field-hint/field-hint-popover` | 删除旧锚点，保留现有 Tailwind |
| `reason-drawer` | 改为 `fixed bottom-5 right-5 z-[100] grid w-[min(420px,calc(100vw-40px))] gap-3 rounded-2xl border border-[var(--line)] bg-white p-4 shadow-[0_20px_70px_rgba(26,37,50,.18)]` |
| `drawer-actions` | 改为 `flex flex-wrap items-center justify-end gap-2` |
| `metric-grid`、`stat-card metric-card`、`stat-label/stat-value/stat-trend` | 删除旧锚点，补齐纯 Tailwind；tone 用 `metricToneClass` 静态映射 |

完成后可删除 `web/admin/src/styles.css` 中：

- `.nav-group p`
- `.console-*`
- `.chip-dot`、`.pill-dot`
- `.modal-*`
- `.field*`
- `.badge*`
- `.inline-feedback`
- `.check-option*`
- `.state-block*`
- `.toast-rail`
- `.reason-drawer`
- `.metric-grid`、`.metric-card`

#### 13.2.3 管理端页面骨架通用替换

很多页面仍写：

- `page-stack`
- `pg-admin-card ops-surface full-main`
- `pg-admin-card overview-surface`
- `form-grid`

统一抽常量，建议加入 `web/admin/src/ui/classes.ts`：

```ts
export const adminPage = {
  stack: 'grid min-h-0 gap-3 overflow-hidden',
  scrollStack: 'grid min-h-0 gap-3 overflow-auto',
  fullSurface: 'grid min-h-0 grid-cols-1 overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white',
  splitSurface: 'grid min-h-0 grid-cols-[minmax(0,1fr)_280px] gap-3 overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white max-[1260px]:grid-cols-1',
  formGrid: 'grid grid-cols-2 gap-3 max-[620px]:grid-cols-1',
  filterBand: 'rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] p-3 shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]',
  filterRow: 'flex flex-wrap items-center gap-2',
  mainLane: 'min-w-0 overflow-auto p-4',
}
```

页面替换：

| 旧写法 | 新写法 |
| --- | --- |
| `<section className="page-stack">` | `<section className={adminPage.stack}>` |
| `<section className="pg-admin-card ops-surface full-main">` | `<section className={adminPage.fullSurface}>` |
| `<section className="pg-admin-card overview-surface">` | `<section className={adminPage.splitSurface}>` |
| `<div className="form-grid">` | `<div className={adminPage.formGrid}>` |
| `<form className="form-grid">` | `<form className={adminPage.formGrid}>` |
| `<section className="pg-admin-card filter-band">` | `<section className={adminPage.filterBand}>` |
| `<form className="filter-row">` | `<form className={adminPage.filterRow}>` |

#### 13.2.4 `web/admin/src/pages/ConfigPage.tsx`

这是管理端当前剩余最明确的页面级样式债务。

需要新增导入：

```ts
import { cn } from '../../../shared/classnames'
import { adminButton, adminPage } from '../ui/classes'
```

建议在文件顶部新增：

```ts
const configClasses = {
  board: 'grid min-h-0 grid-cols-[220px_minmax(0,1fr)_280px] gap-0 overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white max-[1260px]:grid-cols-1',
  rail: 'grid content-start gap-1 border-r border-[var(--line)] bg-[var(--pg-admin-bg-subtle)] p-3 max-[1260px]:grid-cols-[repeat(4,minmax(0,1fr))] max-[1260px]:border-r-0 max-[1260px]:border-b max-[620px]:grid-cols-1',
  railButton: 'rounded-lg px-3 py-2 text-left text-sm font-bold text-[var(--soft)] hover:bg-[rgba(87,117,185,.08)] hover:text-[var(--text)]',
  railButtonActive: 'bg-[rgba(87,117,185,.1)] text-[var(--text)] shadow-[inset_3px_0_0_var(--blue)]',
  lane: 'min-w-0 overflow-auto p-4',
  head: 'mb-3 flex flex-wrap items-center justify-between gap-2 border-b border-[var(--line)] pb-3',
  permissionNote: 'rounded-xl border border-[rgba(184,135,64,.28)] bg-[rgba(184,135,64,.08)] px-3 py-2 text-sm text-[var(--amber)]',
  formGrid: 'grid grid-cols-[repeat(auto-fit,minmax(260px,1fr))] gap-3',
  formItem: 'grid gap-2 rounded-xl border border-[var(--line)] bg-white p-3',
  sideRail: 'min-w-0 overflow-auto border-l border-[var(--line)] bg-[var(--pg-admin-bg-subtle)] max-[1260px]:border-l-0 max-[1260px]:border-t',
  kvList: 'grid gap-2',
  kvRow: 'grid grid-cols-[minmax(120px,.7fr)_minmax(0,1fr)] gap-2',
}
```

替换矩阵：

| 旧 class | 新 class |
| --- | --- |
| `page-stack` | `adminPage.stack` |
| `ghost` | `cn(adminButton.base, adminButton.ghost)` |
| `btn primary` | `cn(adminButton.base, adminButton.primary)` |
| `pg-admin-card config-formboard` | `configClasses.board` |
| `config-category-rail` | `configClasses.rail` |
| 分类按钮 `active` | `cn(configClasses.railButton, activeTab === tab.key && configClasses.railButtonActive)` |
| `config-form-lane` | `configClasses.lane` |
| `section-head compact` | `configClasses.head` |
| `permission-note` | `configClasses.permissionNote` |
| `config-form-grid` | `configClasses.formGrid` |
| `config-form-item` | `configClasses.formItem` |
| `config-side-rail` | `configClasses.sideRail` |
| `config-kv-list` | `configClasses.kvList` |
| `config-kv-row` | `configClasses.kvRow` |
| `ghost small` | `cn(adminButton.base, adminButton.ghost, adminButton.small)` |

完成后删除 CSS：

- `.config-formboard`
- `.config-category-rail*`
- `.config-form-lane`
- `.config-form-grid`
- `.config-form-item`
- `.config-side-rail`
- `.config-kv-list`
- `.config-kv-row`
- `.permission-note`
- `@media` 中对应 config 规则。

#### 13.2.5 `web/admin/src/pages/AuditPage.tsx`

需要新增导入：

```ts
import { cn } from '../../../shared/classnames'
import { adminButton, adminPage } from '../ui/classes'
```

替换矩阵：

| 旧 class | 新 class |
| --- | --- |
| `page-stack` | `adminPage.stack` |
| `ghost` | `cn(adminButton.base, adminButton.ghost)` |
| `pg-admin-card filter-band` | `adminPage.filterBand` |
| `filter-row` | `adminPage.filterRow` |
| `btn` | `cn(adminButton.base, adminButton.primary)` 或刷新按钮用 `cn(adminButton.base, adminButton.ghost)` |
| `pg-admin-card timeline-surface` | `cn(adminSurface.card, 'grid gap-0 overflow-auto p-4')` |
| `timeline-item` | `grid gap-2 border-t border-[var(--line)] py-3 first:border-t-0` |
| `timeline-item > div` | `flex flex-wrap items-center gap-2` |
| `timeline-item strong` | `text-[var(--text)]` |
| `timeline-item p` | `m-0 text-sm text-[var(--soft)]` |
| `timeline-item small` | `text-xs text-[var(--soft)]` |

完成后删除 `.filter-row*` 和 `.timeline-item*`。

#### 13.2.6 其他管理页面

这些页面已完成主要表格迁移，下一步只清理通用骨架和表单过渡类。

| 文件 | 路由 | 剩余旧类 | 替换方案 |
| --- | --- | --- | --- |
| `HealthPage.tsx` | `#/health` | `page-stack`、`pg-admin-card ops-surface full-main` | `adminPage.stack`、`adminPage.fullSurface` |
| `ReadinessPage.tsx` | `#/readiness` | `page-stack`、`pg-admin-card ops-surface full-main` | 同上 |
| `CallRecordsPage.tsx` | `#/call-records` | `page-stack`；文件常量里有 `ops-surface full-main` | 改为 `adminPage.stack`、`adminPage.fullSurface` |
| `UserGroupsPage.tsx` | `#/user-groups` | `page-stack`、`pg-admin-card ops-surface full-main`、`form-grid` | `adminPage.stack`、`adminPage.fullSurface`、`adminPage.formGrid` |
| `RedeemPage.tsx` | `#/redeem` | `page-stack`、`form-grid`；文件常量里有 `ops-surface full-main` | `adminPage.stack`、`adminPage.formGrid`、`adminPage.fullSurface` |
| `RoutingPage.tsx` | `#/routing` | `page-stack`、`form-grid`；文件常量里有 `ops-surface full-main` | 同上 |
| `PricingPage.tsx` | `#/pricing` | `page-stack`、`pg-admin-card overview-surface`、`form-grid` | `adminPage.stack`、`adminPage.splitSurface`、`adminPage.formGrid` |
| `ProviderModelsPage.tsx` | `#/provider-models` | `page-stack`、`pg-admin-card ops-surface full-main`、`form-grid`、`check-grid-scroll`、`check-option` | `adminPage.stack`、`adminPage.fullSurface`、`adminPage.formGrid`、纯 Tailwind checkbox |
| `ReviewPage.tsx` | `#/reviews` | `page-stack`、`pg-admin-card ops-surface full-main review-workspace` | `adminPage.stack`、`adminPage.fullSurface` |
| `UsersPage.tsx` | `#/users` | `page-stack`、`pg-admin-card ops-surface full-main`、`lane-head compact`、`form-grid` | `adminPage.stack`、`adminPage.fullSurface`、页面常量 `sectionHead`、`adminPage.formGrid` |
| `CashierPage.tsx` | `#/cashier` | `page-stack`、`pg-admin-card ops-surface full-main`、`form-grid`、`check-option` | `adminPage.stack`、`adminPage.fullSurface`、`adminPage.formGrid`、纯 Tailwind checkbox |
| `OverviewPage.tsx` | `#/overview` | 常量里含 `content page-stack` | 删除 `content page-stack`，保留纯 Tailwind |

#### 13.2.7 管理端 DataGrid 删除准入

`web/admin/src/ui/dataGrid.ts` 已经集中维护列模板。当前阶段不要再新增页面级 `.xxx-grid` CSS。

删除以下 CSS 前必须确认：

```bash
rg -n 'admin-data-grid|table-head|table-row|route-grid|price-grid|account-grid|route-model-grid|candidate-grid|route-price-grid|user-group-grid|review-grid|users-grid|redeem-grid|redeem-redemption-grid|health-grid|call-record-grid|readiness-grid|overview-readiness-grid|user-detail-|cashier-' web/admin/src
```

如果只在 `web/admin/src/ui/dataGrid.ts` 的常量名中出现，不算 legacy 引用；如果 TSX 里仍有裸字符串，则先迁移。

### 13.3 CSS 删除顺序

用户端删除顺序：

1. 先删 `.image-lightbox*`、`.modal-head*`、`.status-pill*`、`.pill*`。
2. 再删 `.delete-confirm*`、`.group-editor*`。
3. 再从 `web/user/src/ui/classes.ts` 移除过渡锚点后删除 `.vault-*`、`.btn*`、`.input*`、`.field*`、`.toast*`、`.modal-*`、`.empty-state`、`.state-line`、`.spinner`。
4. 最后处理移动端 media query 中只服务旧类的规则。

管理端删除顺序：

1. 先迁移 `ConfigPage.tsx`、`AuditPage.tsx`，删除 `.config-*`、`.permission-note`、`.filter-row*`、`.timeline-item*`。
2. 迁移 `components.tsx`，删除 `.modal-*`、`.field*`、`.badge*`、`.check-option*`、`.toast-rail`、`.state-block*`、`.reason-drawer`。
3. 迁移所有页面骨架，删除 `.page-stack`、`.pg-admin-card`、`.ops-surface`、`.overview-surface`、`.form-grid`。
4. 从 `web/admin/src/ui/classes.ts` 移除 `btn/ghost/admin-shell/...` 后删除 `.btn*`、`.ghost*`、shell 旧选择器。
5. 最后确认 DataGrid 无裸引用后删除 `.admin-data-grid`、`.table-head`、`.table-row` 及旧 `.xxx-grid` 列模板。

每次删除前必须执行：

```bash
rg -n '目标class名' web/user/src web/admin/src
```

### 13.4 分批执行计划

#### Batch A：用户端收尾

状态：已完成。当前扫描未发现用户端 `btn/status-pill/image-lightbox/delete-confirm/group-editor` 等旧锚点，`web/user/src/styles.css` 已降至 94 行。

文件：

- `web/user/src/ui/classes.ts`
- `web/user/src/components.tsx`
- `web/user/src/pages/LandingPage.tsx`
- `web/user/src/pages/LoginPage.tsx`
- `web/user/src/pages/ApiKeysPage.tsx`
- `web/user/src/pages/ProfilePage.tsx`
- `web/user/src/pages/DocsPage.tsx`
- `web/user/src/pages/WorkspacePage.tsx`
- `web/user/src/pages/GalleryPage.tsx`
- `web/user/src/styles.css`

验收：

```bash
rg -n 'className="(btn|btn |status-pill|pill|image-lightbox|delete-confirm|group-editor)' web/user/src
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

目标：`web/user/src/styles.css` 降到 250 行以内。

#### Batch B：管理端 Config 和 Audit

状态：已完成。`ConfigPage.tsx` 与 `AuditPage.tsx` 已迁移到 `adminPage`、`adminButton`、页面常量和 Tailwind utilities；后续只需在 Batch E 删除确认无引用的 CSS 残段。

文件：

- `web/admin/src/ui/classes.ts`
- `web/admin/src/pages/ConfigPage.tsx`
- `web/admin/src/pages/AuditPage.tsx`
- `web/admin/src/styles.css`

验收：

```bash
rg -n 'className="(btn|btn |ghost|ghost |config-|permission-note)' web/admin/src/pages/ConfigPage.tsx web/admin/src/pages/AuditPage.tsx
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

目标：删除 `.config-*`、`.permission-note`、`.filter-row*`、`.timeline-item*`。

#### Batch C：管理端共享组件去锚点

状态：已完成组件 TSX 迁移，`components.tsx` 和 `ui/classes.ts` 已不再依赖本批目标锚点。剩余工作是清理 `web/admin/src/styles.css` 中已无源代码引用的旧 CSS 段。

文件：

- `web/admin/src/components.tsx`
- `web/admin/src/ui/classes.ts`
- `web/admin/src/styles.css`

验收：

```bash
rg -n '\b(nav-label|console-provider-pill|pill-dot|chip-dot|toast-rail|state-block|modal-|field-|badge|check-option|reason-drawer|metric-grid)\b' web/admin/src/components.tsx web/admin/src/ui/classes.ts
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

目标：共享组件不再依赖管理端 legacy CSS。

#### Batch D：管理端页面骨架批量清理

状态：TSX 迁移已完成。当前扫描未发现各页面、`components.tsx`、`ui/classes.ts` 中继续使用 `page-stack`、`pg-admin-card`、`ops-surface`、`overview-surface`、`form-grid`、`ops-status-strip`、`status-cell`、`check-grid-scroll`、`check-option`。后续仅需在 Batch E 中删除 CSS 残留段。

文件：

- `web/admin/src/pages/HealthPage.tsx`
- `web/admin/src/pages/ReadinessPage.tsx`
- `web/admin/src/pages/CallRecordsPage.tsx`
- `web/admin/src/pages/UserGroupsPage.tsx`
- `web/admin/src/pages/RedeemPage.tsx`
- `web/admin/src/pages/RoutingPage.tsx`
- `web/admin/src/pages/PricingPage.tsx`
- `web/admin/src/pages/ProviderModelsPage.tsx`
- `web/admin/src/pages/ReviewPage.tsx`
- `web/admin/src/pages/UsersPage.tsx`
- `web/admin/src/pages/CashierPage.tsx`
- `web/admin/src/pages/OverviewPage.tsx`
- `web/admin/src/styles.css`（Batch E 清理）

验收：

```bash
rg -n '\b(page-stack|pg-admin-card|ops-surface|overview-surface|form-grid|ops-status-strip|status-cell|check-grid-scroll|check-option)\b' web/admin/src/pages web/admin/src/ui web/admin/src/components.tsx
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

目标：管理端页面不再使用通用页面骨架 legacy class。

#### Batch E：最终 CSS 压缩和视觉验收

状态：CSS 压缩已完成。Batch D 的 TSX 清理已完成，`web/admin/src/styles.css` 已从 1296 行压缩到 116 行；删除时已按选择器分组确认，保留登录页、页面常量和全局基础样式需要的 token/base 规则。

文件：

- `web/user/src/styles.css`
- `web/admin/src/styles.css`
- 必要时更新 `README.md` 或开发指南中 Tailwind 约束。

验收：

```bash
wc -l web/user/src/styles.css web/admin/src/styles.css
rg -n 'className="(btn|btn |ghost|ghost |admin-data-grid|table-head|table-row|status-pill|pill|image-lightbox|delete-confirm|group-editor|config-|permission-note)' web/user/src web/admin/src
rg -n '\b(page-stack|pg-admin-card|ops-surface|overview-surface|form-grid|ops-status-strip|status-cell|check-grid-scroll|check-option)\b' web/admin/src/pages web/admin/src/ui web/admin/src/components.tsx
rg -n '"[^"]*(signal-rail|signal-section|muted-action|editable-row|wide|login-panel|list-toolbar|pagination-row|lane-head|user-detail-stack|user-detail-section|micro-tabs|danger-text|filter-band)[^"]*"' web/admin/src -g '*.tsx' -g '*.ts'
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
git diff --check
```

目标：

- `web/user/src/styles.css` 小于 250 行。
- `web/admin/src/styles.css` 小于 300 行。
- TSX 中不再出现新写的裸 `btn`、`ghost`、`table-head`、`table-row`、`admin-data-grid`。

### 13.5 视觉验收路由

每批完成后至少用桌面 1440x1000 和移动 390x844 验收以下路由：

用户端：

- `http://127.0.0.1:8088/#/landing`
- `http://127.0.0.1:8088/#/login`
- `http://127.0.0.1:8088/#/home`
- `http://127.0.0.1:8088/#/genpic`
- `http://127.0.0.1:8088/#/gallery`
- `http://127.0.0.1:8088/#/public-gallery`
- `http://127.0.0.1:8088/#/checkout`
- `http://127.0.0.1:8088/#/api-keys`
- `http://127.0.0.1:8088/#/profile`
- `http://127.0.0.1:8088/#/docs`

管理端：

- `http://127.0.0.1:8088/admin/#/login`
- `http://127.0.0.1:8088/admin/#/overview`
- `http://127.0.0.1:8088/admin/#/config`
- `http://127.0.0.1:8088/admin/#/cashier`
- `http://127.0.0.1:8088/admin/#/users`
- `http://127.0.0.1:8088/admin/#/provider-models`
- `http://127.0.0.1:8088/admin/#/audit`

重点检查：

- 顶栏和侧栏在移动端是否换行且不遮挡内容。
- 表格容器是否自身横向滚动，而不是撑破页面。
- Modal、抽屉、Toast 的 z-index 和滚动区域是否正常。
- 按钮文字是否溢出，尤其是中文长按钮和后台操作按钮。
- `ConfigPage` 三栏布局在 1260px 以下是否能顺利变为单列。
