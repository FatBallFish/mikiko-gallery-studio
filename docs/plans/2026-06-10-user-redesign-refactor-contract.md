# User Redesign Refactor Contract Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 基于 `web/user/src/pages/RedesignDemo.tsx` 的原型视觉和交互，重构现有用户端前端页面，并补齐原型中存在但当前产品缺失的设置页、明暗主题、主题色切换、头像菜单、图库导入参考图等能力。

**Architecture:** 前端采用“设计 token + Shell 布局 + 页面容器 + 可复用业务组件 + 页面模型 contract”的方式落地，不直接把原型单文件照搬进业务页。现有业务 API、鉴权、任务流、图库、充值、API Key、开发文档等真实能力必须迁移到新视觉组件中；原型里的 mock 数据只作为视觉参考，不作为业务数据源。涉及新增功能的后端契约在本文中同步定死，执行时必须先补类型和模型契约，再改 UI。

**Tech Stack:** React 19、Vite 7、Tailwind CSS v4、现有 `web/shared/api-types.ts` / `web/shared/user-api.ts`、Go 后端 Agent API、现有仓库 `*.contract.ts` 测试契约模式。

---

## 0. 文档用途和硬性约束

本文不是视觉建议稿，而是后续实施的重构合同。执行者必须按本文的模块边界、命名、数据契约、验收点落地，不能在执行中临时改成另一套交互或另一套路由。

硬性约束：

1. 新用户端样式必须高度还原 `RedesignDemo.tsx` 的原型气质，但业务数据、路由、API 调用必须迁移现有真实实现。
2. 不允许直接把 `RedesignDemo.tsx` 当成生产页面整体替换现有用户端。原型只能拆解为 token、组件样式、布局和交互参考。
3. 不允许新增 UI 依赖库，除非另开方案审批。图标继续使用当前项目内联 SVG 或抽象 `UserIcon` 组件。
4. 所有页面模型逻辑必须优先写在 `*.ts` 中，并为关键逻辑补 `*.contract.ts`，延续当前项目风格。
5. 页面可见文案默认中文；英文只允许出现在模型名、API 字段名、代码示例、协议名、HTTP method、curl 示例中。
6. “图库”在用户端导航和页面标题中统一改名为“资产”。接口和内部类型可以继续用 `gallery`，但 UI 文案不再展示“图库”作为一级菜单名。
7. API Key 不展示明文 Secret。创建或重置时如果后端返回一次性 secret，只能在一次性提示区展示；列表常态必须展示首尾明文 + 中间掩码。
8. 主题相关能力需要持久化到用户偏好；未登录或接口失败时允许先使用 localStorage fallback，但不能只做纯本地状态。
9. 从资产导入参考图如果需要把历史生成图作为参考图使用，必须通过后端明确契约转换或授权，不能把 `gallery_image.id` 冒充 `reference_asset.id`。
10. 站点正式名称为 `Mikiko Studio`。所有用户可见的站点名称、页面 title、登录页品牌、页脚版权、toast 欢迎语、代码示例中的产品名、文档说明中的产品名，都必须从旧默认名 `Pic Gallery` / `Vault Platform` / `Vault` 统一替换为 `Mikiko Studio`。模型名、API path、数据库表名、历史内部包名不在本轮强制改名范围。
11. 站点 Logo 已确定，源文件为 `docs/template/mikiko-studio.png`。生产用户端不得继续使用字母 `V`、默认渐变圆点或文字占位作为主 Logo。

---

## 0.1 品牌命名和 Logo 迁移契约

正式品牌：

```ts
export const siteBrand = {
  name: 'Mikiko Studio',
  logoSource: 'docs/template/mikiko-studio.png',
  logoAssetName: 'mikiko-studio.png',
} as const
```

### 0.1.1 必须替换的用户可见名称

执行重构时必须搜索并处理这些旧品牌文案：

```bash
rg -n "Pic Gallery|Vault Platform|Vault|picgallery|pic-gallery" web/user web/shared docs/template docs/design
```

替换规则：

| 旧内容 | 新内容 | 范围 |
|---|---|---|
| `Pic Gallery` | `Mikiko Studio` | 用户可见站点名称、页面标题、登录/首页/页脚/文档描述 |
| `Vault Platform` | `Mikiko Studio` | 页脚版权、产品描述 |
| `Vault` | `Mikiko Studio` | 仅限用户可见品牌词；不得替换 CSS token、内部 class、demo 标识中的技术名 |
| `api.picgallery.ai` | 待正式域名确认前保留或替换为配置项 | API 示例域名不能硬编码新假域名 |
| `pic-gallery-*` | 保留 | npm package、localStorage key、内部 route、仓库名不在本轮改名范围 |

注意：

- 不能盲目全局替换 `Vault`，因为当前样式里存在 `font-vault-*`、`bg-vault-*` 等技术 token。技术 token 本轮可以保留，避免扩大重构范围。
- API 示例里的域名如果正式域名未确定，必须抽成常量，例如 `OPEN_API_BASE_URL_LABEL`，不要编造 `api.mikiko.studio`。
- localStorage key 如 `pic-gallery-user-session` 可以暂时保留，避免影响已有登录态；如果后续要迁移，需要单独写兼容迁移逻辑。

### 0.1.2 Logo 文件落地

源文件：

```text
docs/template/mikiko-studio.png
```

建议生产资产位置：

```text
web/user/src/assets/mikiko-studio.png
```

执行时使用复制，不移动源文件：

```bash
mkdir -p web/user/src/assets
cp docs/template/mikiko-studio.png web/user/src/assets/mikiko-studio.png
```

React 使用契约：

```tsx
import mikikoLogoUrl from './assets/mikiko-studio.png'

export function BrandMark({ withText = false }: { withText?: boolean }) {
  return (
    <span className={userShell.brandMark}>
      <img className={userShell.brandLogo} src={mikikoLogoUrl} alt="Mikiko Studio" />
      {withText ? <span className={userShell.brandName}>Mikiko Studio</span> : null}
    </span>
  )
}
```

Shell 侧边栏默认只展示 Logo 图形，不展示 `Mikiko Studio` 文字：

```tsx
<button className={userShell.brand} type="button" onClick={() => app.navigate('home')} aria-label="Mikiko Studio 首页">
  <BrandMark />
</button>
```

登录页、落地页或页脚可以展示 `Mikiko Studio` 文字，但必须与 Logo 组合，不能继续显示旧默认项目名。

CSS 契约：

```ts
export const userShell = {
  brandMark: 'inline-flex items-center gap-2',
  brandLogo: 'block size-12 rounded-2xl object-cover shadow-[0_0_30px_rgba(var(--accent-rgb),0.22)]',
  brandName: 'text-sm font-black tracking-normal text-[var(--fg)]',
}
```

浏览器 title 契约：

```tsx
useEffect(() => {
  document.title = routeTitle ? `${routeTitle} - Mikiko Studio` : 'Mikiko Studio'
}, [routeTitle])
```

后端/文档契约：

- OpenAPI 文档页面中的产品标题展示 `Mikiko Studio API`。
- 后端如有邮件模板、登录验证码邮件、支付订单商品名、OAuth app name、OpenAPI `info.title` 使用旧名，需纳入后端重构范围。
- 本前端重构至少要在方案执行时列出后端品牌残留清单；是否同批修改由后端任务范围决定。

---

## 1. 原型主题样式规范提取

参考文件：

- `web/user/src/pages/RedesignDemo.tsx`
- `web/user/src/ui/redesign-classes.ts`
- `web/user/src/styles.css`

### 1.1 整体气质

原型是深色创作者工作台，而不是传统 SaaS 表格后台。重构后的用户端应具备这些稳定特征：

- 深色背景为主，使用低亮度黑蓝色画布，不使用纯黑大面积硬底。
- 主内容区域有“工作台/画布”感觉：左侧窄导航、顶部工具区、主体内容约束在宽画布中。
- 卡片边框细、圆角大、背景有轻微透明和 blur，但不能变成高亮玻璃拟物。
- 主题色是清晰的品牌信号，只用于激活态、CTA、余额、选中态、焦点态和少量图标，不铺满整页。
- 图片是视觉中心。资产卡、生成结果、全屏预览必须让图片承担主要视觉重量。
- 页面标题用粗体、较大字号；面板内标题要克制，不使用英文 eyebrow。
- 控件选中态统一为：主题色边框 + 轻微主题色背景 + 内部 ring/高光，而不是完全填充或模糊羽化色块。

### 1.2 色彩 token

生产样式必须把原型中的变量沉淀为稳定 token，不能在页面里散写 `oklch(...)` 或 `rgba(...)`。

修改文件：

- `web/shared/user-theme.css`
- `web/user/src/styles.css`
- `web/user/src/ui/classes.ts`

新增/调整 token 契约：

```css
:root {
  --pg-user-dark-bg: #080910;
  --pg-user-dark-canvas: #0d0f18;
  --pg-user-dark-surface: #12141f;
  --pg-user-dark-elevated: #181b29;

  --pg-user-light-bg: #f6f2ea;
  --pg-user-light-canvas: #fbf8f1;
  --pg-user-light-surface: #ffffff;
  --pg-user-light-elevated: #f1ece3;

  --pg-user-text-main-dark: #f5f0e8;
  --pg-user-text-muted-dark: rgba(245, 240, 232, 0.68);
  --pg-user-text-dim-dark: rgba(245, 240, 232, 0.46);

  --pg-user-text-main-light: #1f2129;
  --pg-user-text-muted-light: rgba(31, 33, 41, 0.62);
  --pg-user-text-dim-light: rgba(31, 33, 41, 0.42);

  --pg-user-border-dark: rgba(255, 255, 255, 0.1);
  --pg-user-border-light: rgba(31, 33, 41, 0.12);

  --pg-accent-amber: #d49d5e;
  --pg-accent-violet: #8376ff;
  --pg-accent-emerald: #41b995;
  --pg-accent-coral: #cb7c4d;
}

[data-theme-mode="dark"] {
  --bg: var(--pg-user-dark-bg);
  --canvas: var(--pg-user-dark-canvas);
  --surface: color-mix(in oklch, var(--pg-user-dark-surface) 92%, transparent);
  --surface-solid: var(--pg-user-dark-surface);
  --elevated: var(--pg-user-dark-elevated);
  --fg: var(--pg-user-text-main-dark);
  --muted: var(--pg-user-text-muted-dark);
  --dim: var(--pg-user-text-dim-dark);
  --border: var(--pg-user-border-dark);
}

[data-theme-mode="light"] {
  --bg: var(--pg-user-light-bg);
  --canvas: var(--pg-user-light-canvas);
  --surface: color-mix(in oklch, var(--pg-user-light-surface) 90%, transparent);
  --surface-solid: var(--pg-user-light-surface);
  --elevated: var(--pg-user-light-elevated);
  --fg: var(--pg-user-text-main-light);
  --muted: var(--pg-user-text-muted-light);
  --dim: var(--pg-user-text-dim-light);
  --border: var(--pg-user-border-light);
}

[data-accent-theme="amber"] {
  --accent: var(--pg-accent-amber);
  --accent-rgb: 212 157 94;
}

[data-accent-theme="violet"] {
  --accent: var(--pg-accent-violet);
  --accent-rgb: 131 118 255;
}

[data-accent-theme="emerald"] {
  --accent: var(--pg-accent-emerald);
  --accent-rgb: 65 185 149;
}

[data-accent-theme="coral"] {
  --accent: var(--pg-accent-coral);
  --accent-rgb: 203 124 77;
}
```

### 1.3 字体和字号

当前项目已经使用：

- Display：`Cormorant Garamond`
- Body：`Manrope`
- Mono：`JetBrains Mono`

重构后继续保留，但使用规则调整：

- 大页面标题：使用 `Manrope` 或现有 display 字体均可，但必须粗壮、稳定，不能出现营销页式花体泛滥。
- 面板标题、按钮、表格：使用 `Manrope`。
- 金额、积分、密钥、API path、状态码：使用 `JetBrains Mono`。
- 不允许在操作台选项中使用“中文 / 英文”的双 Label，例如 `提示词 / Prompt`、`质量 / Quality`。

字号契约：

```ts
export const typographyContract = {
  pageTitle: 'text-4xl md:text-6xl font-black leading-none',
  sectionTitle: 'text-sm font-bold text-[var(--muted)]',
  panelTitle: 'text-xl md:text-2xl font-black',
  body: 'text-sm leading-relaxed',
  monoMetric: 'font-vault-mono tabular-nums',
} as const
```

### 1.4 组件圆角、边框、焦点

圆角规则：

- Shell/sidebar/topbar：不使用卡片圆角。
- 页面主卡片：`rounded-[2rem]` 到 `rounded-[2.5rem]`。
- 按钮/输入框/选择项：`rounded-xl` 或 `rounded-2xl`。
- 图片卡：`rounded-[1.5rem]` 到 `rounded-[2rem]`。
- 小 icon button：`rounded-xl`。

焦点规则：

```css
.pg-user-scope :is(input, textarea, select):focus,
.pg-user-scope :is(input, textarea, select):focus-visible {
  outline: none;
  border-color: color-mix(in oklch, var(--accent) 62%, var(--border));
  box-shadow: 0 0 0 1px color-mix(in oklch, var(--accent) 55%, transparent);
}

.pg-prompt-field:focus,
.pg-prompt-field:focus-visible {
  box-shadow: none;
  border-color: transparent;
}
```

提示词输入框采用“外层容器 focus-within 显示主题色边框，textarea 自身不显示边框”的契约，避免双色边框。

### 1.5 明暗主题下的图片类组件

图片卡 hover 蒙版、操作按钮、checkbox、全屏预览背景必须使用 token：

```css
[data-theme-mode="dark"] {
  --image-overlay: linear-gradient(to top, rgb(0 0 0 / .88), rgb(0 0 0 / .18), rgb(0 0 0 / .35));
  --image-action-bg: rgb(0 0 0 / .45);
  --image-action-text: rgb(255 255 255 / .78);
  --lightbox-backdrop: rgb(0 0 0 / .9);
  --lightbox-stage-bg: rgb(0 0 0);
  --lightbox-close-bg: rgb(0 0 0 / .5);
}

[data-theme-mode="light"] {
  --image-overlay: linear-gradient(to top, rgb(255 255 255 / .9), rgb(255 255 255 / .22), rgb(255 255 255 / .36));
  --image-action-bg: rgb(255 255 255 / .78);
  --image-action-text: rgb(31 33 41 / .78);
  --lightbox-backdrop: rgb(31 33 41 / .56);
  --lightbox-stage-bg: rgb(255 255 255 / .72);
  --lightbox-close-bg: rgb(255 255 255 / .84);
}
```

---

## 2. 现有用户端与原型差异

### 2.1 已存在但需要迁移视觉的页面

| 当前页面 | 当前路由 | 原型对应 | 迁移策略 |
|---|---|---|---|
| `HomePage` | `home` | `HomeView` | 迁移原型首页英雄、推荐资产卡、去除 `New Model Released` 类营销块 |
| `WorkspacePage` | `genpic` | `StudioView` | 保留真实 capability、estimate、createTask、SSE；重做控制台和输出台样式 |
| `GalleryPage` | `gallery` | `GalleryView` | UI 改名“资产”；迁移筛选、批量选择、图片卡 hover、全屏预览 |
| `CheckoutPage` | `checkout` | `BillingView` | 保留真实 cashier options/order；套用积分包、支付方式、订单面板样式 |
| `ApiKeysPage` | `api-keys` | `ApiKeysView` | 保留真实 API Key CRUD；改成原型卡片和 masked key 样式 |
| `ProfilePage` | `profile` | `ProfileView` | 去掉英文 eyebrow；保留积分、兑换码、流水、资料编辑 |
| `DocsPage` | `docs` | `DocsView` | 保留 OpenAPI 文档；去掉英文 eyebrow；改为新视觉 |
| `Shell` | 全局 | 原型 shell | 迁移左侧菜单、顶部工具栏、头像菜单、余额、主题切换 |

### 2.2 原型有但现有项目缺失的功能

必须补齐：

1. 设置页 `settings`
2. 光暗主题切换
3. 主题色切换
4. 从资产导入参考图
5. 资产导入弹窗中的搜索和筛选
6. 全屏图片预览基础信息：像素、比例、模型、来源、提示词长文本限高
7. 输出台每张图片级别失败展示，支持部分成功部分失败
8. 头像菜单固定项：个人中心、积分充值、API 密钥、开发文档、分隔符、退出登录

### 2.3 原型 mock 不能直接落地的点

这些内容必须替换为真实业务逻辑：

- `MOCK_GALLERY_ASSETS` -> `userApi.listGalleryImages()`
- `MOCK_API_KEYS` -> `userApi.listApiKeys()`
- 原型内 `setTimeout` 模拟生成 -> `userApi.createTask()` + SSE `taskStreamUrl()`
- 原型内固定积分包 -> `userApi.getCashierOptions()`
- 原型内固定支付方式 -> `CashierOptions.visible_methods`
- 原型内自定义金额单价 `0.03125` -> 优先使用 `options.custom_amount.cny_per_point`，仅当前端 fallback 时使用 `0.03125`

---

## 3. 路由和应用状态契约

### 3.1 RouteId 扩展

修改文件：

- `web/user/src/types.ts`
- `web/user/src/routeState.ts`
- `web/user/src/App.tsx`
- `web/user/src/routeState.contract.ts`

契约：

```ts
export type RouteId =
  | 'landing'
  | 'login'
  | 'home'
  | 'genpic'
  | 'gallery'
  | 'public-gallery'
  | 'checkout'
  | 'api-keys'
  | 'profile'
  | 'docs'
  | 'settings'
  | 'redesign-demo'
```

`protectedRoutes` 必须包含：

```ts
export const protectedRoutes: RouteId[] = [
  'home',
  'genpic',
  'gallery',
  'checkout',
  'api-keys',
  'profile',
  'docs',
  'settings',
]
```

`App.tsx` switch 必须新增：

```tsx
case 'settings':
  return <Shell><SettingsPage /></Shell>
```

验收：

- `#/settings` 未登录会跳登录。
- 登录后头像菜单和侧边栏都能进入设置页。
- 刷新 `#/settings` 能保持路由。

### 3.2 AppContext 扩展主题状态

修改文件：

- `web/user/src/types.ts`
- `web/user/src/App.tsx`
- `web/user/src/themePreferences.ts` 新建
- `web/user/src/themePreferences.contract.ts` 新建

类型契约：

```ts
export type ThemeMode = 'dark' | 'light'
export type AccentTheme = 'amber' | 'violet' | 'emerald' | 'coral'

export type UserThemePreference = {
  mode: ThemeMode
  accent: AccentTheme
}

export type AppContextValue = {
  // existing fields...
  themePreference: UserThemePreference
  setThemePreference: (patch: Partial<UserThemePreference>) => Promise<void>
}
```

默认值：

```ts
export const defaultThemePreference: UserThemePreference = {
  mode: 'dark',
  accent: 'amber',
}
```

DOM 写入契约：

```ts
export function applyThemePreference(pref: UserThemePreference) {
  document.documentElement.dataset.themeMode = pref.mode
  document.documentElement.dataset.accentTheme = pref.accent
}
```

状态初始化顺序：

1. 读取 localStorage。
2. 如果有登录 profile 且 profile 包含新偏好，使用 profile。
3. 用户切换后立即乐观更新 DOM 和 localStorage。
4. 登录态下异步保存到后端。
5. 后端保存失败时保留本地显示，但 toast 提示“偏好未同步”。

伪代码：

```ts
const themeStorageKey = 'pic-gallery-user-theme'

function readLocalTheme(): UserThemePreference {
  return parseTheme(localStorage.getItem(themeStorageKey)) ?? defaultThemePreference
}

async function setThemePreference(patch: Partial<UserThemePreference>) {
  const next = normalizeTheme({ ...themePreference, ...patch })
  setThemePreferenceState(next)
  applyThemePreference(next)
  localStorage.setItem(themeStorageKey, JSON.stringify(next))

  if (!sessionRef.current?.token) return

  try {
    const profile = await userApi.updatePreferences({
      theme_mode: next.mode,
      accent_theme: next.accent,
    })
    setProfile(profile)
  } catch (error) {
    notify('error', '主题偏好已应用到本机，但暂未同步到账户')
  }
}
```

注意：当前 `userApi.updatePreferences(theme, default_locale)` 签名不满足该契约，需要调整，详见第 11 章后端契约。

---

## 4. Shell 重构契约

修改文件：

- `web/user/src/components.tsx`
- `web/user/src/avatarMenu.ts`
- `web/user/src/avatarMenu.contract.ts`
- `web/user/src/ui/classes.ts`

### 4.1 左侧导航

生产导航项必须和原型一致：

```ts
export const userNavItems: Array<{
  route: RouteId
  label: string
  icon: UserIconName
}> = [
  { route: 'home', label: '首页', icon: 'home' },
  { route: 'genpic', label: '创作', icon: 'sparkles' },
  { route: 'gallery', label: '资产', icon: 'grid' },
  { route: 'checkout', label: '积分', icon: 'credit-card' },
  { route: 'api-keys', label: '密钥', icon: 'key' },
  { route: 'settings', label: '设置', icon: 'settings' },
]
```

不再展示：

- `short: HOME / GEN / IMG`
- Logo 下 `Pic Gallery` 或 `VAULT`
- `public-gallery` 作为一级导航。公开广场可以保留在首页入口或二级入口。
- `redesign-demo` 作为生产导航。

品牌区契约：

```tsx
<button className={userShell.brand} aria-label="返回首页">
  <BrandMark />
</button>
```

`BrandMark` 必须使用 `web/user/src/assets/mikiko-studio.png`，源文件来自 `docs/template/mikiko-studio.png`。侧边栏只展示 Logo 图形，不展示 `Mikiko Studio` 文字；登录页、首页 hero、页脚可以展示完整品牌名。

### 4.2 顶部工具区

必须包含：

1. 光暗主题 icon button。
2. 余额 pill，文案为 `◈` + 积分数 + `+` 充值按钮。
3. 头像按钮，点击展开菜单。

不再展示：

- 顶部 quick links：`灵感模板`、`公开广场`、`开发文档`。
- `Balance` 英文。

伪代码：

```tsx
function ShellTopbar() {
  const app = useApp()
  const isDark = app.themePreference.mode === 'dark'

  return (
    <header className={userShell.topbar}>
      <button
        type="button"
        aria-label={isDark ? '切换浅色主题' : '切换深色主题'}
        title={isDark ? '切换浅色主题' : '切换深色主题'}
        className={userShell.themeToggle}
        onClick={() => void app.setThemePreference({ mode: isDark ? 'light' : 'dark' })}
      >
        {isDark ? <MoonIcon /> : <SunIcon />}
      </button>

      <div className={userShell.balancePill}>
        <span>◈</span>
        <b>{formatBalance(app.balance?.available_points)}</b>
        <button type="button" onClick={() => app.navigate('checkout')}>+</button>
      </div>

      <AvatarMenu />
    </header>
  )
}
```

### 4.3 头像菜单

菜单项固定：

```ts
export const avatarMenuItems = [
  { key: 'profile', label: '个人中心', route: 'profile', icon: 'profile' },
  { key: 'checkout', label: '积分充值', route: 'checkout', icon: 'credit-card' },
  { key: 'api-keys', label: 'API 密钥', route: 'api-keys', icon: 'key' },
  { key: 'docs', label: '开发文档', route: 'docs', icon: 'docs' },
] as const
```

退出登录必须在分隔线后。

验收：

- 点击菜单外部关闭。
- 点击任一菜单项关闭并导航。
- 按 `Escape` 关闭菜单。
- 退出登录调用 `app.logout()`。

---

## 5. 设置页契约

新建文件：

- `web/user/src/pages/SettingsPage.tsx`
- `web/user/src/pages/settingsThemeModel.ts`
- `web/user/src/pages/settingsThemeModel.contract.ts`

### 5.1 页面结构

页面标题：

- `设置`
- 不展示英文 eyebrow。

页面分区：

1. 光暗主题
2. 主题色
3. 账户偏好同步状态

UI 必须接近原型：

- 主标题大而粗。
- 内容为两列卡片，大屏 `grid-cols-[0.9fr_1.1fr]`，小屏单列。
- 光暗主题使用两个大按钮，暗色显示 Moon，亮色显示 Sun。
- 主题色使用颜色 swatch，不使用文字色块堆叠。

### 5.2 模型契约

```ts
export type ThemeModeOption = {
  value: ThemeMode
  label: string
  icon: 'moon' | 'sun'
}

export type AccentThemeOption = {
  value: AccentTheme
  label: string
  color: string
}

export function settingsThemeModeOptions(): ThemeModeOption[] {
  return [
    { value: 'dark', label: '暗色', icon: 'moon' },
    { value: 'light', label: '亮色', icon: 'sun' },
  ]
}

export function settingsAccentThemeOptions(): AccentThemeOption[] {
  return [
    { value: 'amber', label: '琥珀', color: 'var(--pg-accent-amber)' },
    { value: 'violet', label: '紫罗兰', color: 'var(--pg-accent-violet)' },
    { value: 'emerald', label: '翡翠', color: 'var(--pg-accent-emerald)' },
    { value: 'coral', label: '珊瑚', color: 'var(--pg-accent-coral)' },
  ]
}
```

### 5.3 验收

- 切换设置页的光暗主题后，右上角 icon 同步改变。
- 右上角切换后，设置页选中态同步改变。
- 切换主题色后，全站 CTA、选中态、焦点态立即改变。
- 刷新页面后保留本地主题。
- 登录用户重新打开后优先使用后端偏好。

---

## 6. 创作页重构契约

修改文件：

- `web/user/src/pages/WorkspacePage.tsx`
- `web/user/src/pages/workspaceGenerateReadiness.ts`
- `web/user/src/pages/workspaceTaskFailure.ts`
- `web/user/src/pages/workspaceTaskProgress.ts` 新建
- `web/user/src/pages/workspaceTaskProgress.contract.ts` 新建
- `web/user/src/pages/workspaceGalleryImport.ts` 新建
- `web/user/src/pages/workspaceGalleryImport.contract.ts` 新建

### 6.1 页面结构

最终结构固定为：

```tsx
<div className={workspaceClasses.root}>
  <aside className={workspaceClasses.sidebar}>
    <StudioControlPanel />
  </aside>
  <section className={workspaceClasses.outputCanvas}>
    <GenerationOutput />
  </section>
  <ImageLightbox />
  <GalleryImportModal />
</div>
```

布局：

- 左侧操作台宽度：`380px` 到 `420px`。
- 右侧输出台填满剩余空间。
- 小屏下操作台在上、输出台在下。
- 输出台背景必须随明暗主题变化，使用 `--canvas`，不能固定 `#05070d`。

### 6.2 操作台文案

禁止英文 Label：

- `选择模型`
- `参考图片`
- `提示词`
- `比例`
- `质量`
- `数量`
- `预计消耗`
- `开始创作`

允许英文：

- 模型名，如 `Flux Pro`。
- API/provider 名称。

### 6.3 质量选项展示

后端 capability 可能返回 `auto`、`standard`、`high`、`ultra` 或其他原始值。UI 必须映射为中文，但提交给后端仍使用原值。

```ts
const qualityLabelMap: Record<string, string> = {
  auto: '自动',
  standard: '标准',
  low: '标准',
  medium: '高清',
  high: '高清',
  ultra: '超清',
  hd: '高清',
}

export function workspaceQualityLabel(value: string) {
  return qualityLabelMap[value.toLowerCase()] ?? value
}
```

### 6.4 参考图片：本地上传 + 从资产导入

当前真实逻辑：

- 本地上传通过 `userApi.uploadReferenceAsset(file)` 得到 `ReferenceAsset`。
- 创建任务提交 `reference_asset_ids`。

新增“从资产导入”后，不能直接把 `GalleryImage.id` 塞入 `reference_asset_ids`。需要以下二选一方案，推荐方案 A。

方案 A，推荐：后端提供资产转参考图接口。

```ts
POST /api/agent/image/v1/reference-assets:import-from-gallery

type ImportReferenceAssetsFromGalleryRequest = {
  gallery_image_ids: string[]
}

type ImportReferenceAssetsFromGalleryResponse = {
  items: ReferenceAsset[]
}
```

前端新增：

```ts
userApi.importReferenceAssetsFromGallery = async (galleryImageIds: string[]) => {
  const response = await sharedApiClient.request<{ items: any[] }>(
    API_PATHS.agent.importReferenceAssetsFromGallery,
    { method: 'POST', body: { gallery_image_ids: galleryImageIds } },
  )
  return response.items.map(toReferenceAsset)
}
```

方案 B：扩展创建任务接口允许 `reference_sources`。

```ts
type CreateTaskRequest = EstimateRequest & {
  prompt: string
  reference_asset_ids?: string[]
  reference_sources?: Array<
    | { type: 'reference_asset'; id: string }
    | { type: 'gallery_image'; id: string }
  >
}
```

不推荐 B 的原因：它会把创建任务接口和资源授权/复制逻辑耦合，错误处理更复杂。

### 6.5 资产导入弹窗

弹窗打开入口：

```tsx
<button type="button" onClick={() => setGalleryImportOpen(true)}>
  从资产导入
</button>
```

弹窗功能：

- 默认开启批量选择。
- 数据源为 `userApi.listGalleryImages()`。
- 搜索 prompt、模型、分组。
- 筛选项与资产页一致：分组、公开状态、模型、比例。
- 最多可选数量 = `maxReferenceImages - currentReferenceCount`。
- 底部固定操作栏：已选数量、取消、确定。
- 点击确定后调用 `importReferenceAssetsFromGallery`，返回 `ReferenceAsset[]`，合并进当前 `refs` 或 `editRefs`。

模型契约：

```ts
export type GalleryImportFilter = {
  query: string
  group: string
  publishStatus: string
  model: string
  ratio: string
}

export function filterGalleryImportImages(
  images: GalleryImage[],
  filter: GalleryImportFilter,
): GalleryImage[] {
  const queryTerms = filter.query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return images.filter((image) => {
    const haystack = [
      image.prompt,
      image.route_model_code,
      image.abstract_model,
      image.image_group,
      image.aspect_ratio,
      image.visibility_status,
    ].filter(Boolean).join(' ').toLowerCase()

    return (
      queryTerms.every((term) => haystack.includes(term))
      && (filter.group === 'all' || (image.image_group || '') === filter.group)
      && (filter.publishStatus === 'all' || image.visibility_status === filter.publishStatus)
      && (filter.model === 'all' || image.route_model_code === filter.model || image.abstract_model === filter.model)
      && (filter.ratio === 'all' || image.aspect_ratio === filter.ratio)
      && Boolean(image.url || image.download_url)
    )
  })
}
```

确定逻辑：

```ts
async function confirmGalleryImport(selected: GalleryImage[]) {
  if (!selected.length) return
  if (selected.length > remainingLimit) {
    notify('error', `最多还能选择 ${remainingLimit} 张参考图`)
    return
  }

  setImportBusy(true)
  try {
    const assets = await userApi.importReferenceAssetsFromGallery(selected.map((item) => item.id))
    setRefs((items) => mergeReferenceAssets(items, assets, maxReferenceImages))
    setGalleryImportOpen(false)
  } catch (error) {
    notify('error', errorMessage(error))
  } finally {
    setImportBusy(false)
  }
}
```

### 6.6 输出台失败展示

当前 `ImageTask` 只有任务级 `results` 和任务级错误，原型要求“生成多张时可能部分成功、部分失败，失败展示跟每张图片走”。这需要前后端契约增强。

短期前端兼容规则：

```ts
export type GenerationSlot =
  | { kind: 'image'; index: number; image: ImageResult }
  | { kind: 'pending'; index: number; label: string }
  | { kind: 'failed'; index: number; title: string; reason: string; code?: string }

export function generationSlots(task: ImageTask): GenerationSlot[] {
  const requested = Math.max(Number(task.image_count || 1), task.results.length)
  const slots: GenerationSlot[] = []

  for (let index = 0; index < requested; index += 1) {
    const image = task.results[index]
    if (image) {
      slots.push({ kind: 'image', index, image })
      continue
    }

    if (['failed', 'partial_failed', 'rejected', 'cancelled'].includes(task.status)) {
      slots.push({
        kind: 'failed',
        index,
        title: '生成失败',
        reason: task.error_message || task.failure_reason || '该图片未生成成功',
        code: task.error_code,
      })
      continue
    }

    slots.push({ kind: 'pending', index, label: '生成中' })
  }

  return slots
}
```

后端增强契约见第 11.3 节。

### 6.7 任务阶段进度

新增前端模型，先由 `ImageTask.status/progress/attempts` 推导，后续接后端节点。

```ts
export type WorkspaceProgressPhase =
  | 'validating'
  | 'routing'
  | 'queued'
  | 'generating'
  | 'storing'
  | 'settling'
  | 'done'
  | 'failed'

export type WorkspaceProgressNode = {
  phase: WorkspaceProgressPhase
  label: string
  status: 'idle' | 'active' | 'done' | 'failed'
}

export function workspaceProgressNodes(task: ImageTask): WorkspaceProgressNode[] {
  const progress = Number(task.progress ?? 0)
  return [
    node('validating', '参数校验', progress >= 5),
    node('routing', '模型路由', progress >= 18),
    node('queued', '队列调度', progress >= 30),
    node('generating', '图像生成', progress >= 78),
    node('storing', '结果入库', progress >= 92),
    node('settling', '积分结算', ['succeeded', 'partial_failed'].includes(task.status)),
  ].map((item) => resolveNodeStatus(item, task))
}
```

### 6.8 创作页验收

- 原真实生图流程仍可创建任务。
- SSE 回填仍能更新输出台。
- 部分成功时成功图片和失败 slot 同屏展示。
- 失败不再用整台遮罩盖住所有结果。
- 提示词聚焦只出现一层主题色边框。
- 上传参考图和从资产导入都能进入生成请求。
- 资产导入不超过后端限制。

---

## 7. 资产页重构契约

修改文件：

- `web/user/src/pages/GalleryPage.tsx`
- `web/user/src/pages/galleryRows.ts`
- `web/user/src/pages/galleryRows.contract.ts`
- `web/user/src/components.tsx` 中 `ImageLightbox`

### 7.1 命名

UI 统一使用“资产”：

- 侧边栏：资产
- 页面标题：历史资产
- 空状态：暂无资产
- 批量栏：已选择 N 个资产

内部保留 `GalleryPage` 文件名和路由 `gallery`，避免大范围后端和路由变更。

### 7.2 筛选交互

现有资产页使用按钮筛选。原型中有 `CustomSelect`，并修过“鼠标移出后下拉消失”的问题。生产实现必须避免依赖 `onMouseLeave` 判断关闭。

契约：

```tsx
function FilterSelect({ value, options, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!open) return
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }
    window.addEventListener('pointerdown', close)
    return () => window.removeEventListener('pointerdown', close)
  }, [open])

  return (
    <div ref={rootRef}>
      <button onClick={() => setOpen((next) => !next)}>{current.label}</button>
      {open ? (
        <div role="listbox">
          {options.map((option) => (
            <button
              role="option"
              aria-selected={option.value === value}
              onClick={() => {
                onChange(option.value)
                setOpen(false)
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
```

禁止：

- 用 trigger 的 `onMouseLeave` 关闭菜单。
- 下拉菜单和触发按钮之间留不可穿越空隙。

### 7.3 资产卡

资产卡必须包含：

- 图片。
- hover 蒙版。
- 操作按钮：预览、下载、复制提示词、继续编辑、申请公开、删除。
- 批量模式 checkbox。
- 分组/公开状态/模型/比例元信息。

轻量卡模型：

```ts
export type AssetCardModel = {
  id: string
  imageUrl: string
  title: string
  prompt: string
  modelLabel: string
  ratioLabel: string
  groupLabel: string
  publishLabel: string
  canPreview: boolean
  canDownload: boolean
  canEdit: boolean
  canPublish: boolean
}
```

### 7.4 全屏图片预览

现有 `ImageLightbox` 需要替换成原型增强版。

Props 契约：

```ts
export type ImageLightboxPayload = {
  url: string
  alt: string
  prompt?: string
  width?: number
  height?: number
  ratio?: string
  model?: string
  source?: string
}

export function ImageLightbox({
  image,
  onClose,
  onReuseParameters,
}: {
  image: ImageLightboxPayload | null
  onClose: () => void
  onReuseParameters?: (image: ImageLightboxPayload) => void
}) {}
```

行为契约：

- 按 `Escape` 关闭。
- 点击空白 backdrop 关闭。
- 点击图片和信息面板不关闭。
- 关闭按钮背景随明暗主题变化。
- 背景蒙版随明暗主题变化。
- 右侧信息包含：像素、比例、模型、来源、提示词。
- 提示词区域最大高度，例如 `max-h-48 overflow-y-auto`。
- 无宽高时显示 `未知`，不能显示 `0 x 0`。

像素和比例模型：

```ts
export function imagePixelsLabel(width?: number, height?: number) {
  return width && height ? `${width} x ${height}` : '未知'
}

export function imageRatioLabel(width?: number, height?: number, fallback?: string) {
  if (fallback) return fallback
  if (!width || !height) return '未知'
  const divisor = gcd(width, height)
  return `${width / divisor}:${height / divisor}`
}
```

---

## 8. 积分页重构契约

修改文件：

- `web/user/src/pages/CheckoutPage.tsx`
- `web/user/src/pages/checkoutPlans.ts`
- `web/user/src/pages/checkoutPaymentDisplay.ts`
- `web/user/src/pages/checkoutOrderState.ts`

### 8.1 页面结构

保留真实逻辑：

- `userApi.getCashierOptions()`
- `userApi.createCashierOrder()`
- `userApi.getCashierOrder()`
- `userApi.mockPayCashierOrder()`
- `userApi.cancelCashierOrder()`
- 订单轮询和余额刷新

视觉结构迁移为：

```tsx
<div className={billingClasses.page}>
  <header>
    <h1>积分充值</h1>
  </header>

  <div className={billingClasses.layout}>
    <section className={billingClasses.mainStack}>
      <BillingCard title="选择积分包">
        <PlanGrid />
      </BillingCard>
      <BillingCard title="支付方式">
        <PaymentMethodGrid />
      </BillingCard>
    </section>

    <aside className={billingClasses.orderPanel}>
      <OrderSummary />
    </aside>
  </div>
</div>
```

禁止展示英文小 Label：

- `Selection`
- `Payment`
- `Checkout`

### 8.2 积分包与支付方式选中态

积分包和支付方式必须共用同一 selected class：

```ts
export const selectableCard = {
  base: 'rounded-[2rem] border border-[var(--border)] bg-[var(--bg)]/50 p-6 transition-all hover:border-[var(--accent)]',
  active: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_10%,var(--surface))] shadow-[inset_0_0_0_1px_color-mix(in_oklch,var(--accent)_36%,transparent),0_0_30px_rgba(var(--accent-rgb),0.12)]',
}
```

支付方式不得使用羽化大背景。

### 8.3 自定义金额

当前 `CheckoutPage` 已有 `purchaseType === 'custom_amount'` 和 `customAmount`。重构需要把原型里的“自定义金额卡片”迁移到真实逻辑。

契约：

```ts
export const fallbackCnyPerPoint = 0.03125

export function normalizeCustomAmount(input: string): {
  value: string
  valid: boolean
  amount: number
  error?: string
} {
  const amount = Number(input)
  if (!Number.isFinite(amount)) return { value: input, valid: false, amount: 0, error: '请输入有效金额' }
  if (amount < 1) return { value: input, valid: false, amount, error: '自定义金额不能低于 1 元' }
  if (amount > 10000) return { value: input, valid: false, amount, error: '自定义金额不能超过 10000 元' }
  return { value: amount.toFixed(2), valid: true, amount }
}

export function customAmountPoints(amountCny: number, cnyPerPoint?: string) {
  const unit = Number(cnyPerPoint || fallbackCnyPerPoint)
  if (!Number.isFinite(unit) || unit <= 0) return '0.00'
  return (amountCny / unit).toFixed(2)
}
```

展示要求：

- 原来价格位置展示 `Custom` 的地方改成 `¥0.03125 / 积分` 或 `0.03125 元/积分`。
- 输入范围 1 到 10000。
- 创建订单时提交 `amount_cny`。
- 如果 `options.custom_amount.enabled === false`，隐藏或禁用自定义金额入口，并展示后端原因。

---

## 9. API 密钥页重构契约

修改文件：

- `web/user/src/pages/ApiKeysPage.tsx`
- `web/user/src/pages/apiKeyRows.ts`
- `web/user/src/pages/apiKeyRows.contract.ts`

### 9.1 密钥展示安全规则

常态列表：

```ts
export function maskToken(value: string, head = 6, tail = 4) {
  if (!value) return '-'
  if (value.length <= head + tail) return `${value.slice(0, 2)}••••`
  return `${value.slice(0, head)}••••••••${value.slice(-tail)}`
}
```

规则：

- `access_key` 可以展示首尾掩码。
- `secret` / `secret_preview` 常态只展示掩码。
- 创建/重置后后端返回一次性 secret 时，可以展示在 modal 或一次性提示卡中。
- 一次性 secret 区域必须有“复制”按钮。
- 关闭一次性区域后，不再通过前端状态恢复明文。
- 不能有“显示/隐藏 Secret 明文”的常态按钮。

需要改掉当前行为：

```tsx
{revealed[key.id] ? (secretPreview ?? '仅创建或重置时展示') : 'sk_••••••••••••'}
```

改为：

```tsx
<code>{maskSecretPreview(key.secret_preview)}</code>
```

`revealed` 状态仅允许用于“一次性 secret toast/modal”，不能控制列表明文。

### 9.2 页面视觉

原型 API Key 页有：

- 统计卡：密钥数、本月调用、限流策略等。
- 密钥卡片列表。
- 快速接入代码块。
- 安全提示。

生产迁移时保留真实 CRUD，但表格可以改成响应式卡片 + 桌面表格混合。密钥多时必须可扫描。

### 9.3 API Key 快速接入

当前 `apiKeyQuickstart` 使用 Secret 作为 Bearer 示例。后续需要和真实开放 API 鉴权一致。如果真实协议仍为 Bearer SK，则保留；如果 AK/SK 签名，则 quickstart 必须展示签名示例。

文档里先定前端函数契约：

```ts
export function apiKeyQuickstart(key: Pick<ApiKey, 'access_key' | 'secret_preview'> | null) {
  const ak = key?.access_key ? maskToken(key.access_key) : 'ak_live_xxx'
  const sk = key?.secret_preview ? maskToken(key.secret_preview) : 'sk_live_xxx'
  return {
    title: '示例请求',
    code: `curl ...`,
    visibleCredentials: { accessKey: ak, secretKey: sk },
  }
}
```

---

## 10. 个人中心和开发文档页重构契约

### 10.1 通用 PageIntro

当前 `PageIntro` 强制接收 `eyebrow`。重构后必须改为可选，且用户端页面默认不传英文 eyebrow。

契约：

```tsx
export function PageIntro({
  title,
  detail,
  eyebrow,
  action,
}: {
  title: string
  detail?: string
  eyebrow?: string
  action?: React.ReactNode
}) {
  return (
    <div className={userShell.pageIntro}>
      <div>
        {eyebrow ? <p className={userText.eyebrow}>{eyebrow}</p> : null}
        <h1>{title}</h1>
        {detail ? <p>{detail}</p> : null}
      </div>
      {action}
    </div>
  )
}
```

### 10.2 个人中心

必须去掉：

- `ACCOUNT & CREDITS`
- 卡片标题里的不必要英文。

保留：

- 积分余额。
- 充值入口。
- 兑换码。
- 积分桶。
- 基本资料编辑。
- 积分流水。

### 10.3 开发文档

必须去掉页面英文小标题，但保留技术内容中的英文：

- HTTP method
- endpoint path
- request/response
- curl
- OpenAI
- API field

允许在代码示例内出现英文。

---

## 11. 后端/API 契约

如果只做视觉迁移，以下后端变更可分阶段落地；但对应前端类型和 fallback 必须按本文设计。

### 11.1 用户主题偏好

当前类型：

```ts
export type UpdatePreferencesRequest = {
  theme?: string
  default_locale?: string
}
```

目标类型：

```ts
export type ThemeMode = 'dark' | 'light'
export type AccentTheme = 'amber' | 'violet' | 'emerald' | 'coral'

export type UserPreferences = {
  model_group: string
  quality: string
  aspect_ratio: string
  image_count: number
  theme_mode: ThemeMode
  accent_theme: AccentTheme
  default_locale?: string
}

export type UpdatePreferencesRequest = Partial<{
  model_group: string
  quality: string
  aspect_ratio: string
  image_count: number
  theme_mode: ThemeMode
  accent_theme: AccentTheme
  default_locale: string
}>
```

API：

```http
PUT /api/agent/user/v1/preferences
Content-Type: application/json

{
  "theme_mode": "dark",
  "accent_theme": "amber"
}
```

响应：

```json
{
  "id": "123",
  "email": "user@example.com",
  "preferences": {
    "model_group": "plus-image",
    "quality": "auto",
    "aspect_ratio": "16:9",
    "image_count": 1,
    "theme_mode": "dark",
    "accent_theme": "amber"
  }
}
```

后端必须校验枚举，不认识的值返回 `400 invalid_preference`。

### 11.2 从历史资产导入参考图

推荐新增：

```http
POST /api/agent/image/v1/reference-assets:import-from-gallery
Content-Type: application/json

{
  "gallery_image_ids": ["img_1", "img_2"]
}
```

响应：

```json
{
  "items": [
    {
      "id": "ref_1",
      "name": "img_1.png",
      "preview_url": "/api/agent/image/v1/reference-assets/ref_1/download",
      "download_url": "/api/agent/image/v1/reference-assets/ref_1/download",
      "status": "ready",
      "size_bytes": 123456,
      "created_at": "2026-06-10T00:00:00Z"
    }
  ]
}
```

后端行为：

- 只能导入当前用户有权限访问的私有或公开图片。
- 图片必须存在可下载对象。
- 可以复用对象存储 key，但必须创建新的 reference asset 记录，便于任务引用和审计。
- 单次导入数量必须遵守 reference image 上限。
- 返回失败时需要指出失败图片：

```json
{
  "error": {
    "code": "REFERENCE_IMPORT_PARTIAL_FAILED",
    "message": "部分资产无法导入",
    "details": {
      "failed_items": [
        { "gallery_image_id": "img_x", "reason": "not_found" }
      ]
    }
  }
}
```

### 11.3 图片级生成结果/失败

当前 `ImageTask.results` 只能表达成功图片，任务级错误只能表达整体失败。为支持部分失败，需要新增 slot。

目标类型：

```ts
export type ImageTaskSlotStatus = 'pending' | 'succeeded' | 'failed'

export type ImageTaskOutputSlot = {
  index: number
  status: ImageTaskSlotStatus
  image?: ImageResult
  error_code?: string
  error_message?: string
  provider_request_id?: string
}

export type ImageTask = {
  // existing fields...
  output_slots?: ImageTaskOutputSlot[]
}
```

SSE `task` event 中也必须包含 `output_slots`。

前端兼容：

```ts
export function taskOutputSlots(task: ImageTask): ImageTaskOutputSlot[] {
  if (task.output_slots?.length) return task.output_slots
  return generationSlots(task).map(toOutputSlot)
}
```

### 11.4 任务阶段事件

可选增强：

```ts
export type ImageTaskPhase =
  | 'validated'
  | 'routed'
  | 'queued'
  | 'provider_started'
  | 'provider_completed'
  | 'stored'
  | 'settled'

export type ImageTaskProgressEvent = {
  task_id: string
  phase: ImageTaskPhase
  progress: number
  message?: string
  occurred_at: string
}
```

SSE：

```text
event: task_phase
data: {"task_id":"1","phase":"provider_started","progress":45,"message":"上游模型已开始生成"}
```

---

## 12. 文件组织目标

重构完成后的用户端建议结构：

```text
web/user/src/
  App.tsx
  components.tsx
  icons.tsx
  themePreferences.ts
  themePreferences.contract.ts
  ui/
    classes.ts
    redesign-classes.ts       # 迁移完成后删除，不进入生产引用
  pages/
    HomePage.tsx
    WorkspacePage.tsx
    GalleryPage.tsx
    CheckoutPage.tsx
    ApiKeysPage.tsx
    ProfilePage.tsx
    DocsPage.tsx
    SettingsPage.tsx
    settingsThemeModel.ts
    settingsThemeModel.contract.ts
    workspaceGalleryImport.ts
    workspaceGalleryImport.contract.ts
    workspaceTaskProgress.ts
    workspaceTaskProgress.contract.ts
```

最终目标：

- `RedesignDemo.tsx` 仍可作为 demo 页面保留一段时间，但生产路由不得依赖它。
- `redesign-classes.ts` 迁移完后应删除或只保留 demo 使用。
- 新生产组件样式收敛到 `ui/classes.ts` 或页面局部 classes。

---

## 13. 分阶段实施计划

### Phase 1：主题和 Shell 基座

目标：先让全站具备原型骨架和主题能力，但不改业务页面内部逻辑。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- 已复制正式 Logo 到 `web/user/src/assets/mikiko-studio.png`，并新增 `web/user/src/brand.tsx` 作为生产品牌入口。
- 用户端 Shell 已接入真实 Logo、右上角光暗主题 icon、余额 `◈` 组件、头像下拉菜单、设置导航。
- `RouteId`、hash 解析和页面路由已加入 `settings`，并新增 `SettingsPage` 作为主题/主题色设置页。
- `AppContext` 已加入 `themePreference` / `setThemePreference`，主题偏好会写入 `documentElement.dataset.themeMode` 和 `documentElement.dataset.accentTheme`，登录状态下同步到用户偏好接口。
- 共享 API 类型已新增 `ThemeMode`、`AccentTheme`、`UserThemePreference`，并扩展 `UpdatePreferencesRequest` 与用户偏好映射。
- 用户可见品牌已迁移为 `Mikiko Studio`，生产 Shell、登录页、落地页不再使用旧默认站点名作为品牌展示。
- 左侧导航已固定为：首页、创作、资产、积分、密钥、设置。

本阶段新增/更新契约：

- `web/user/src/themePreferences.contract.ts`
- `web/user/src/pages/settingsThemeModel.contract.ts`
- `web/user/src/routeState.contract.ts`
- `web/user/src/components.contract.ts`
- `web/user/src/pages/workspaceImageActions.contract.ts`

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 2 开始，不应重复改动 Phase 1 的基础契约，除非后续阶段发现必须补充的兼容问题。

步骤：

1. 复制 `docs/template/mikiko-studio.png` 到 `web/user/src/assets/mikiko-studio.png`。
2. 新建 `brand.ts` 或在 `components.tsx` 中导出 `siteBrand` / `BrandMark`。
3. 搜索用户端旧品牌名，替换用户可见的 `Pic Gallery`、`Vault Platform`、可见品牌词 `Vault` 为 `Mikiko Studio`。
4. 新建 `themePreferences.ts` 和 contract。
5. 扩展 `RouteId`，加入 `settings`。
6. 新建 `SettingsPage` 空页面，接入路由。
7. 调整 `AppContext`，加入 `themePreference` 和 `setThemePreference`。
8. 将 `data-theme-mode`、`data-accent-theme` 写到 `documentElement`。
9. 重构 `Shell`：侧边栏导航、顶部主题按钮、余额 pill、头像菜单、真实 Logo。
10. 更新 `avatarMenu.ts` 和 contract。
11. 跑：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

验收：

- `home/genpic/gallery/checkout/api-keys/profile/docs/settings` 都能路由。
- 用户可见站点名为 `Mikiko Studio`。
- 侧边栏使用 `mikiko-studio.png` Logo，不再使用 `V` 字母占位。
- 光暗主题 icon 状态和设置页一致。
- 余额显示 `◈`。
- 左侧导航为：首页、创作、资产、积分、密钥、设置。

### Phase 2：设计 token 和基础组件

目标：让所有页面共享新视觉基础。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- `web/user/src/styles.css` 已补齐生产态明暗主题 token、主题色 token、图片 hover/lightbox token，并新增全局输入焦点规则，统一去除浏览器默认异常 outline。
- `web/user/src/components.tsx` 中 `PageIntro` 已改为 `eyebrow` / `detail` 可选，后续页面可不展示英文小标题。
- `ImageLightbox` 已替换为增强版，支持 `Escape` 关闭、点击空白关闭、图片/信息面板阻止冒泡关闭。
- 全屏图片预览已展示像素、比例、模型、来源、提示词；提示词区域使用 `max-h-48 overflow-y-auto` 限制超长文本展示。
- 创作页与资产详情页的预览调用已升级为 `ImageLightboxPayload`，传入像素、比例、模型、来源、提示词等基础信息。
- `components.contract.ts` 已补充 lightbox 像素/比例模型断言。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 3 创作页重构开始。

步骤：

1. 重写 `web/shared/user-theme.css` token。
2. 重写 `web/user/src/styles.css` 全局背景、焦点、scrollbar、图片 token。
3. 调整 `ui/classes.ts` 中 shell/card/button/form/pill/state。
4. 改 `PageIntro` 让 eyebrow 可选。
5. 改 `ImageLightbox` 为增强版。
6. 补 `components.contract.ts` 中 lightbox、PageIntro、mask/focus 相关测试。

验收：

- 输入框没有浏览器默认黄色/双色异常边框。
- lightbox 支持 ESC、空白关闭、长 prompt 限高。
- 亮色主题下图片 hover 和 lightbox 不灰黑压暗。

### Phase 3：创作页重构

目标：把原型工作台体验迁移到真实生成流程。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- 已新增 `workspaceTaskProgress.ts` / `workspaceTaskProgress.contract.ts`，固定质量中文展示、任务阶段节点和生成结果 slot 模型。
- 创作页质量选项展示已改为中文映射，提交给后端仍保留原始 quality 值。
- 创作页操作台已去除 `提示词 (PROMPT)`、`清晰度 (QUALITY)`、`比例 (ASPECT RATIO)`、`限制词 (NEGATIVE PROMPT)` 等中英双 Label。
- 生成结果输出已改为 `generationSlots(task)` 渲染；多图部分失败时，成功图片和失败 slot 同级展示，不再用任务级失败遮罩盖住所有结果。
- 已新增 `workspaceGalleryImport.ts` / `workspaceGalleryImport.contract.ts`，固定资产导入筛选、选项派生和 reference asset 合并契约。
- 创作页已新增“从资产导入”入口和弹窗，支持搜索、分组、公开状态、模型、比例筛选，默认批量勾选。
- 确认导入时调用 `userApi.importReferenceAssetsFromGallery(galleryImageIds)`，等待后端返回 `ReferenceAsset[]` 后再合并进参考图列表，不直接把 `GalleryImage.id` 塞入 `reference_asset_ids`。
- 共享 API path 已加入 `API_PATHS.agent.importReferenceAssetsFromGallery = /api/agent/image/v1/reference-assets:import-from-gallery`。

后端依赖：

- 需要实现 `POST /api/agent/image/v1/reference-assets:import-from-gallery`，入参 `{ gallery_image_ids: string[] }`，出参 `{ items: ReferenceAsset[] }`。前端已按此契约调用；若后端未实现，导入确认会展示真实接口错误。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 4 资产页重构开始。

步骤：

1. 抽 `workspaceQualityLabel`，补 contract。
2. 抽 `workspaceProgressNodes`，补 contract。
3. 抽 `generationSlots/taskOutputSlots`，补 contract。
4. 重构 `WorkspacePage` 左侧控制台样式。
5. 重构 `GenerationRecord` 和 `GeneratedImage`，改为图片级 slot。
6. 新增 `GalleryImportModal`。
7. 前端先接 `userApi.importReferenceAssetsFromGallery`，如果后端未实现，则按钮显示后端能力不可用提示。
8. 确认 `createTask` 仍提交真实 `reference_asset_ids`。

验收：

- 真实文生图、参考生图、图片编辑都能创建任务。
- 估价正常。
- 结果正常进资产页。
- 部分失败展示为图片卡级失败。
- 从资产导入不会伪造 reference id。

### Phase 4：资产页重构

目标：迁移原型资产页面视觉和筛选体验。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- 资产页顶部已去除英文小标题，页面标题保留“历史资产”。
- 筛选项已从多行按钮改为 `FilterSelect` 下拉组件；关闭逻辑使用 `window.pointerdown` + `rootRef.contains`，不依赖 `onMouseLeave`，避免鼠标移向选项时下拉消失。
- 资产页空状态与批量栏文案已统一为“资产”：`暂无资产`、`已选择 N 个资产`、`共 N 个资产`。
- `galleryRows.ts` 已扩展 `AssetCardModel`，包含 `imageUrl`、`prompt`、`modelLabel`、`ratioLabel`、`canPreview`、`canEdit` 等字段，并更新 contract。
- 资产卡已接入主题化 hover 蒙版、主题化 checkbox/操作按钮 token，亮色主题下不再固定黑色遮罩。
- 资产卡操作区已补齐预览、复制提示词、继续编辑、下载、申请公开、设置分组、删除等图片级操作入口。
- 资产卡预览使用增强 `ImageLightbox`，传入像素、比例、模型、来源、提示词。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 5 积分页重构开始。

步骤：

1. 改 UI 文案“图库”为“资产”。
2. 新建 `FilterSelect` 组件或页面内组件。
3. 筛选关闭逻辑改为 pointer outside，不用 mouseleave。
4. 重构资产卡 hover 蒙版和操作按钮。
5. 批量栏套用新视觉。
6. 详情弹窗复用增强 lightbox。

验收：

- 鼠标从筛选按钮移动到下拉选项不会消失。
- 批量选择、下载、分组、删除、申请公开仍可用。
- 资产卡在明暗主题 hover 样式正确。

### Phase 5：积分页重构

目标：迁移原型积分页样式并保留真实收银台。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- 积分页已去除 `CHECKOUT` 英文 eyebrow，保留中文标题“积分充值”。
- 积分包与支付方式共用同一组选中态 class：主题色边框、轻微主题色背景、内 inset ring 和低强度 glow；支付方式不再使用单独羽化大背景。
- 已新增 `checkoutCustomAmount.ts` / `checkoutCustomAmount.contract.ts`，固定自定义金额 1 到 10000 元校验、积分计算和单价展示契约。
- 自定义金额创建订单前会校验金额范围，提交给后端的 `amount_cny` 使用标准化两位小数字符串。
- 自定义金额的价格/说明位置展示后端 `cny_per_point`，无后端配置时使用 `0.03125 元/积分` fallback。
- 真实收银台逻辑保留：收银台配置读取、创建订单、订单轮询、模拟支付、取消订单、余额刷新、最近订单刷新均未移除。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 6 API Key、个人中心、文档页重构开始。

步骤：

1. 重构 `checkoutClasses`。
2. 积分包与支付方式共用 selected class。
3. 自定义金额范围校验 1 到 10000。
4. 单价展示 `0.03125 元/积分`，优先后端配置。
5. 订单详情面板迁移为原型右侧摘要。
6. 保留轮询、模拟支付、取消订单、余额刷新。

验收：

- 固定积分包能创建订单。
- 自定义金额能创建订单。
- 支付方式选中态和积分包一致。
- 没有 `Selection`、`Payment` 英文小 Label。

### Phase 6：API Key、个人中心、文档页重构

目标：统一账户类页面视觉和文案规则。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- API Key 页已移除页面英文小标题，保留中文标题“API 密钥”。
- API Key 列表常态已移除 `revealed` 明文切换状态，不再提供“显示/隐藏”按钮，不再复制常态 Secret 明文。
- `apiKeyRows.ts` 已新增 `maskToken` / `maskSecretPreview`，Access Key 与 Secret 常态均使用首尾明文 + 中间掩码展示。
- 创建密钥仍在创建弹窗内展示后端返回的一次性 Secret，并提供复制按钮。
- 重置 Secret 后页面显示一次性提示卡，可复制明文 Secret；关闭后不再通过列表状态恢复明文。
- 快速接入示例不再硬编码旧品牌域名 `api.picgallery.ai`，在正式域名确认前使用 `https://api.example.com` 占位，并只展示掩码后的 Secret。
- 个人中心已移除 `ACCOUNT & CREDITS` 英文 eyebrow，保留积分余额、充值、兑换码、积分桶、资料编辑和积分流水。
- 开发文档页已移除页面/侧栏英文小标题；HTTP method、endpoint path、request/response、curl、OpenAI 等技术内容保持不翻译。
- `apiKeyRows.contract.ts` 已补充密钥掩码、禁止明文 Secret、禁止旧品牌域名和移除显示/隐藏按钮文案的断言。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续继续从 Phase 7 首页收口和 demo 清理开始。

步骤：

1. API Key 页面改卡片/表格混合布局。
2. 移除常态 Secret 明文展示能力。
3. 创建/重置后的 Secret 一次性提示区保留复制。
4. Profile 移除英文 eyebrow，保留真实功能。
5. Docs 移除页面英文 eyebrow，保留技术内容英文。
6. 补 `apiKeyRows.contract.ts` 的 mask 测试。

验收：

- API Key Secret 常态不可明文展示。
- 快速接入仍可复制示例。
- Profile 保存资料、兑换积分、刷新流水可用。
- Docs 搜索、分组、复制示例可用。

### Phase 7：首页收口和 demo 清理

目标：最后统一首页和删除 demo 依赖。

实施状态（2026-06-10）：已完成并验证通过。

已落地范围：

- 登录后首页保持产品工作台入口形态：顶部账户/模型就绪条、可点击进入创作的主题 hero、公开广场灵感瀑布流和创作/API/账户快捷入口。
- 首页中 `FEATURED SHOWCASE`、`Cinematic Luxury Watch` 等英文展示已轻量替换为中文，且未发现 `New Model Released` 残留。
- 公开广场入口在首页作为次级入口保留，不作为主 CTA 抢占创作入口。
- 生产 `RouteId`、`routeState` 和 `App` 已移除 `redesign-demo` 路由；`App.tsx` 不再静态 import `RedesignDemo`。
- `routeState.contract.ts` 已补充断言：`#/redesign-demo` 在生产用户路由中回落到 `landing`。
- 独立原型预览仍可通过 `web/user/demo.html` -> `src/demo-main.tsx` 使用 `RedesignDemo`，但不会被生产入口引用。
- 独立 demo 中可见旧品牌词已同步替换为 `Mikiko Studio`，旧示例域名替换为 `https://api.example.com` 占位。

验证证据：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify-contracts.sh
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

上述命令均已通过。后续进入最终准出检查。

步骤：

1. 首页迁移原型 hero、灵感发现、资产预览。
2. 去掉 `New Model Released`。
3. 公开广场入口放在首页次级位置。
4. 检查生产路由不引用 `RedesignDemo`。
5. 根据需要保留 `#/redesign-demo` 为开发预览，或从 `protectedRoutes/navItems` 中完全移除。

验收：

- 首页第一屏不是营销卡片，而是可直接进入创作的产品界面。
- 生产导航没有“重设计”入口。
- 原型文件不影响生产 bundle 或只在 demo 入口按需引用。

---

## 14. 测试契约

必须新增/更新的 contract：

```text
web/user/src/themePreferences.contract.ts
web/user/src/routeState.contract.ts
web/user/src/avatarMenu.contract.ts
web/user/src/components.contract.ts
web/user/src/pages/settingsThemeModel.contract.ts
web/user/src/pages/workspaceGalleryImport.contract.ts
web/user/src/pages/workspaceTaskProgress.contract.ts
web/user/src/pages/workspaceTaskFailure.contract.ts
web/user/src/pages/galleryRows.contract.ts
web/user/src/pages/checkoutPlans.contract.ts
web/user/src/pages/checkoutPaymentDisplay.contract.ts
web/user/src/pages/apiKeyRows.contract.ts
```

关键测试点：

```ts
// themePreferences.contract.ts
expect(normalizeThemePreference({ mode: 'bad', accent: 'bad' })).toEqual({
  mode: 'dark',
  accent: 'amber',
})

// workspaceGalleryImport.contract.ts
expect(filterGalleryImportImages(images, { query: 'flux product', group: 'all', publishStatus: 'all', model: 'all', ratio: 'all' }))
  .toHaveLength(1)

// workspaceTaskProgress.contract.ts
expect(taskOutputSlots(partialFailedTask)).toEqual([
  { index: 0, status: 'succeeded', image: expect.any(Object) },
  { index: 1, status: 'failed', error_message: expect.any(String) },
])

// checkoutPlans.contract.ts
expect(normalizeCustomAmount('0.5').valid).toBe(false)
expect(normalizeCustomAmount('10001').valid).toBe(false)
expect(customAmountPoints(31.25, '0.03125')).toBe('1000.00')

// apiKeyRows.contract.ts
expect(maskToken('sk_abcdefghijklmnopqrstuvwxyz')).toBe('sk_abc••••••••wxyz')
```

运行命令：

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

如果后端也改了：

```bash
./scripts/workflow/verify.sh
./scripts/workflow/api-smoke.sh
```

---

## 15. 浏览器验收清单

使用 `http://localhost:5173/` 或当前 dev server。

### 全局

- 左侧 Logo 使用 `docs/template/mikiko-studio.png` 复制而来的真实 Mikiko Studio 图标，并且只展示图标。
- 网站用户可见名称统一为 `Mikiko Studio`，不再出现 `Pic Gallery`、`Vault Platform` 或可见品牌词 `Vault`。
- 左侧菜单是：首页、创作、资产、积分、密钥、设置。
- 右上角余额显示 `◈` 和余额。
- 光暗主题 icon：暗色显示月亮，亮色显示太阳，和设置页一致。
- 头像菜单项完整且顺序正确。
- 所有 input/textarea/select 聚焦没有黄色边框和双色异常边框。

### 创作页

- 操作台 Label 无英文辅助标题。
- 提示词聚焦只出现外层主题边框。
- 参考图片支持本地上传和从资产导入。
- 输出台背景跟随主题。
- 失败按图片 slot 展示。

### 资产页

- 页面标题是“历史资产”。
- 筛选下拉不会因为鼠标移出 trigger 而消失。
- 图片 hover 蒙版和按钮在亮色主题可读。
- 全屏预览支持 ESC 和点击空白关闭。
- 全屏预览展示像素、比例、模型、来源，长 prompt 限高滚动。

### 积分页

- “选择积分包”和“支付方式”无英文 Label。
- 支付方式选中态和积分包一致。
- 自定义金额 1 到 10000 校验。
- 单价展示 `0.03125 元/积分` 或后端配置值。

### API Key 页

- 无英文小标题。
- 密钥常态只显示首尾明文 + 中间掩码。
- Secret 不可常态明文展示。
- 创建/重置一次性 Secret 能复制。

### 设置页

- 光暗主题切换实时生效。
- 主题色切换实时生效。
- 刷新后本地偏好保留。
- 登录后偏好同步到账户。

---

## 16. 风险和决策记录

### 16.1 不能只做 CSS 覆盖

当前页面组件结构和原型差异较大，特别是创作页、资产页、积分页。如果只用 CSS 覆盖，会留下旧 DOM 的交互负担，后续很难维护。因此必须按页面拆组件迁移。

### 16.2 不能绕过后端做资产导入

历史资产图片和 reference asset 的权限、审计、对象存储生命周期不同。前端必须通过后端转换接口拿到 `ReferenceAsset`，否则任务引用语义错误。

### 16.3 API Key 安全优先于原型便利

原型可以展示 mock 密钥，但生产页不能提供 Secret 常态显示开关。后续所有执行都以“创建/重置仅展示一次”为安全基线。

### 16.4 主题偏好需要后端持久化

纯 localStorage 不满足多端一致。先本地 fallback 是为了交互即时性，不是最终存储方案。

---

## 17. 完成定义

本重构完成必须同时满足：

1. 所有用户端主页面视觉高度接近 `RedesignDemo.tsx`。
2. 用户可见品牌统一为 `Mikiko Studio`，并使用真实 Logo `mikiko-studio.png`。
3. 所有现有真实业务能力仍可用。
4. 原型新增能力已落地或有明确后端接口并完成前端兼容态。
5. 全部 contract 测试通过。
6. `npm --prefix web/user run typecheck` 通过。
7. `npm --prefix web/user run build` 通过。
8. 若改后端，`./scripts/workflow/verify.sh` 和 API smoke 通过。
9. 浏览器验收清单逐项通过。
