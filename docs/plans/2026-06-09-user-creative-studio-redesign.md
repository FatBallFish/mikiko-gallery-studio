# User Creative Studio Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 重构客户端侧页面视觉、组件样式和生图流程反馈，让 Pic Gallery 用户端呈现“现代高阶创作者工坊（Creative Studio）”的专业质感，同时为后续提示词优化、审核前置、队列细分等生成流程节点预留前后端契约。

**Architecture:** 本方案只覆盖 `web/user` 客户端体验，不改管理后台视觉。前端采用“主题 token + 可复用 Studio 组件 + 页面级组合”的方式落地，避免多个页面重复实现同类卡片、按钮、空状态和生图进度组件。后端本期可以先不改接口，前端用现有 `ImageTask.status`、`attempts`、`results` 计算进度；后续通过 `ImageTask.progress_nodes` / SSE `task_phase` 事件增强分步骤状态。

**Tech Stack:** React 19、Vite 7、Tailwind CSS v4、共享 CSS token、现有 Go API/SSE 任务流、现有 `web/shared/api-types.ts` / `web/shared/user-api.ts`。

---

## 1. Scope and Non-Scope

### 1.1 本期 Scope

- 只重构客户端侧：`web/user/**` 和客户端引用的 `web/shared/user-theme.css`、`web/shared/tokens.css`、`web/shared/base.css`。
- 保留现有路由、鉴权、API 调用、SSE 任务流、图库操作、充值/API Key/文档功能。
- 以现有页面为基础升级：
  - `Shell`
  - `LandingPage`
  - `LoginPage`
  - `HomePage`
  - `WorkspacePage`
  - `GalleryPage`
  - `PublicGalleryPage`
  - `CheckoutPage`
  - `ApiKeysPage`
  - `ProfilePage`
  - `DocsPage`
- 抽象共享 Studio 组件和样式契约，禁止页面各自复制按钮、卡片、空状态、图片卡、进度组件样式。
- 设计并落地“前端本地进度节点解析器”，先兼容当前后端，再为后续后端节点事件预留。
- 在方案里列出后端接口扩展建议、修改文件和伪代码，但本轮不强制实现后端。

### 1.2 本期 Non-Scope

- 不重构管理后台 `web/admin/**`。
- 不引入新 UI 库、图标库或动画库。当前 `web/user/package.json` 只有 React/Vite/Tailwind；除非后续单独批准，不添加 `lucide-react` 等依赖。
- 不照搬 `docs/template/gemini_template.jsx` 的 API 实现、Gemini/Imagen 直接调用、模拟生成图逻辑。
- 不改变生成任务计费、模型路由、OpenAPI 兼容层业务规则。
- 不创建新的后端 prompt 优化模型调用。本方案只预留契约。

## 2. Inputs and Reference Decisions

### 2.1 用户提供参考图结论

参考图核心特征：

- 深色低亮度画布，不使用纯黑，整体背景接近 `#07080d` / `#090a11`。
- 左侧窄导航骨架明确，当前页面 active 态明显。
- 面板使用磨砂、细边框、轻微内阴影，不靠厚重投影。
- 视觉亮点来自少量蓝紫/品红点光，而不是满屏渐变。
- 图片卡片大、内容图像承担主要高级感。
- 空状态被设计成场景，而不是只显示一句“暂无数据”。

### 2.2 Gemini 模板可参考项

文件：`docs/template/gemini_template.jsx`

可借鉴：

- 生成工作台采用“控制面板 + 画布区”结构。
- 模型选择卡包含封面、类型、标签和示例 Prompt。
- 生成中采用分阶段进度：准备、加载模型、扩散/上游调用、增强、完成。
- 生成结果提供悬浮操作条：下载、复制 Prompt、全屏预览、继续编辑。
- 图库 hover 展示 Prompt、复用参数、预览等快捷操作。
- 设置/底部信息使用小型状态看板，增强系统可靠感。

不可照搬：

- `lucide-react` 图标依赖。
- 模板直接调用 Gemini/Imagen API 的代码。
- 模拟 fallback 图片作为真实生成结果。
- 单页 tab 导航替换当前业务路由。
- 文案中“魔法”“画卷”等偏 demo 的表达需要降噪，换成更专业的创作者语气。

## 3. Current Code Reality

### 3.1 前端现状

- 用户端入口：`web/user/src/App.tsx`
- 用户端壳组件：`web/user/src/components.tsx`
- 用户端页面：
  - `web/user/src/pages/HomePage.tsx`
  - `web/user/src/pages/WorkspacePage.tsx`
  - `web/user/src/pages/GalleryPage.tsx`
  - `web/user/src/pages/PublicGalleryPage.tsx`
  - 其他账户/充值/API/文档页
- 当前共享样式：
  - `web/user/src/ui/classes.ts`
  - `web/user/src/styles.css`
  - `web/shared/user-theme.css`
  - `web/shared/tokens.css`
  - `web/shared/base.css`
- 当前任务流：
  - `WorkspacePage.createTask()` 调 `userApi.createTask()`
  - `userApi.createTask()` 调 `POST /api/agent/image/v1/tasks`
  - `WorkspacePage` 通过 `EventSource(userApi.taskStreamUrl(token))` 监听 `history` 和 `task`
  - 任务状态记录进入 `records`

### 3.2 后端现状

- 任务领域类型：`internal/domain/imagetask/types.go`
- 创建任务：`internal/service/imagetask/service.go:155`
- 执行任务：`internal/service/imagetask/service.go:309`
- provider 调用与结果持久化：`internal/service/imagetask/service.go:461`
- Agent 任务接口：`internal/http/handlers/api.go:2150`
- Agent SSE：
  - 全量任务流：`internal/http/handlers/api.go:2254`
  - 单任务事件：`internal/http/handlers/api.go:2214`
- Ent schema：`internal/repository/ent/schema/imagetask.go`
- Ent store：`internal/repository/entstore/imagetask_store.go`

当前后端只有粗粒度 `status`：

```ts
type ImageTaskStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'partial_failed'
  | 'failed'
  | 'cancelled'
  | 'rejected'
  | 'deleted'
```

已有可用字段：

```ts
type ImageTask = {
  id: string
  prompt: string
  task_type: ImageTaskType
  status: ImageTaskStatus
  progress?: number
  provider?: string
  attempts?: Attempt[]
  results: ImageResult[]
  created_at: string
  updated_at: string
  error_code?: string
  error_message?: string
}
```

## 4. Visual System Contract

### 4.1 主题方向

采用“深色 Creative Studio”：

- Base：墨黑、蓝黑、低饱和中性灰。
- Primary accent：低饱和电感蓝紫。
- Secondary accent：少量玫紫只用于 active glow、品牌标识或高优先级 CTA。
- Functional accents：success/warning/danger 使用低饱和绿色/琥珀/珊瑚红，只用于状态。
- Surface：`solid`、`glass`、`raised` 三层，不允许每个页面自造 surface。

### 4.2 CSS token

修改：`web/shared/user-theme.css`

目标 token：

```css
:root {
  --pg-user-bg-deep: #07080d;
  --pg-user-bg-canvas: #090a12;
  --pg-user-bg-panel: rgba(17, 18, 29, 0.82);
  --pg-user-bg-panel-solid: #11121d;
  --pg-user-bg-elevated: rgba(25, 26, 40, 0.9);
  --pg-user-bg-input: rgba(7, 8, 13, 0.72);

  --pg-user-text-main: #f4f0ea;
  --pg-user-text-soft: rgba(244, 240, 234, 0.72);
  --pg-user-text-dim: rgba(244, 240, 234, 0.48);
  --pg-user-text-faint: rgba(244, 240, 234, 0.32);

  --pg-user-border-thin: rgba(255, 255, 255, 0.08);
  --pg-user-border-strong: rgba(255, 255, 255, 0.16);
  --pg-user-border-glow: rgba(122, 118, 255, 0.48);

  --pg-user-accent: #7a76ff;
  --pg-user-accent-2: #c75bd7;
  --pg-user-accent-warm: #d6a766;
  --pg-user-success: #65b891;
  --pg-user-warning: #c79a56;
  --pg-user-danger: #d26c68;

  --pg-user-shadow-soft: 0 24px 80px rgba(0, 0, 0, 0.28);
  --pg-user-shadow-glow: 0 0 36px rgba(122, 118, 255, 0.18);
  --pg-user-inner-line: inset 0 1px 0 rgba(255,255,255,.06);
}
```

修改：`web/user/src/styles.css`

全局语义变量映射：

```css
:root {
  --bg: var(--pg-user-bg-deep);
  --canvas: var(--pg-user-bg-canvas);
  --surface: var(--pg-user-bg-panel);
  --surface-solid: var(--pg-user-bg-panel-solid);
  --elevated: var(--pg-user-bg-elevated);
  --input: var(--pg-user-bg-input);
  --fg: var(--pg-user-text-main);
  --muted: var(--pg-user-text-soft);
  --dim: var(--pg-user-text-dim);
  --faint: var(--pg-user-text-faint);
  --border: var(--pg-user-border-thin);
  --border-strong: var(--pg-user-border-strong);
  --accent: var(--pg-user-accent);
  --accent-2: var(--pg-user-accent-2);
  --accent-warm: var(--pg-user-accent-warm);
  --accent-emerald: var(--pg-user-success);
  --accent-coral: var(--pg-user-danger);
}
```

全局背景：

```css
body::before {
  content: '';
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: -2;
  background:
    radial-gradient(circle at 16% 8%, rgba(122,118,255,.13), transparent 30%),
    radial-gradient(circle at 86% 10%, rgba(199,91,215,.10), transparent 28%),
    linear-gradient(rgba(255,255,255,.018) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,.018) 1px, transparent 1px),
    var(--bg);
  background-size: auto, auto, 72px 72px, 72px 72px, auto;
}

body::after {
  content: '';
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  opacity: .055;
  background-image: url("data:image/svg+xml,...noise...");
}
```

约束：

- 不使用纯 `#000`。
- 不再同时以金色、紫色、绿色、蓝色作为同等级强调色。
- 大面积 CTA 不使用模板里 `from-violet via-fuchsia to-indigo` 的强 AI 渐变，改为深色按钮 + 细微边缘 glow，主 CTA 可保留很克制的 `linear-gradient(135deg, #7470f5, #b758c7)`。

### 4.3 Typography

保留当前在线字体方向，但调整使用规则：

- Display：`Cormorant Garamond` 只用于 Landing/Hero 大标题和少量作品标题。
- Body/UI：`Manrope` 用于所有控件、表格、正文。
- Mono：`JetBrains Mono` 用于数值、状态码、模型 code、积分。

规范：

```ts
const typeScale = {
  hero: 'font-vault-display text-[clamp(3.2rem,8vw,7rem)] leading-[.86]',
  pageTitle: 'font-vault-display text-[clamp(2.6rem,5vw,4.8rem)] leading-[.9]',
  sectionTitle: 'font-vault-body text-[22px] font-semibold leading-tight',
  panelTitle: 'font-vault-body text-[15px] font-semibold',
  label: 'font-vault-mono text-[10px] uppercase tracking-[.14em]',
  body: 'text-sm leading-[1.7]',
  micro: 'text-[11px] leading-[1.5]',
}
```

## 5. Shared Component Abstraction Contract

### 5.1 文件组织

新增：

- `web/user/src/ui/studio.tsx`
- `web/user/src/ui/studioProgress.ts`
- `web/user/src/ui/studio.contract.ts`
- `web/user/src/ui/studioProgress.contract.ts`

保留并改造：

- `web/user/src/ui/classes.ts`

规则：

- `classes.ts` 只放 class token，不放业务组件。
- `studio.tsx` 放跨页面可复用 UI 组件。
- `studioProgress.ts` 放进度节点解析纯函数，方便 contract test。
- 页面文件只能组合 `studio.tsx` 和业务模型，不允许复制同类 class 组合。

### 5.2 Studio class tokens

修改：`web/user/src/ui/classes.ts`

新增 `studio` object：

```ts
export const studio = {
  layout: {
    content: 'mx-auto w-full max-w-[1480px] px-8 py-8 max-[760px]:px-4 max-[760px]:py-5',
    pageStack: 'grid gap-8',
    split: 'grid grid-cols-[minmax(340px,420px)_minmax(0,1fr)] gap-5 max-[980px]:grid-cols-1',
    toolbar: 'flex flex-wrap items-center justify-between gap-3',
  },
  surface: {
    base: 'border border-[var(--border)] bg-[var(--surface)] shadow-[var(--pg-user-shadow-soft)] backdrop-blur-xl',
    solid: 'border border-[var(--border)] bg-[var(--surface-solid)]',
    elevated: 'border border-[var(--border-strong)] bg-[var(--elevated)] shadow-[var(--pg-user-shadow-soft)]',
    glass: 'border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_82%,transparent)] shadow-[var(--pg-user-inner-line)] backdrop-blur-2xl',
  },
  radius: {
    panel: 'rounded-[22px]',
    card: 'rounded-[18px]',
    control: 'rounded-[12px]',
    tile: 'rounded-[10px]',
  },
  button: {
    base: 'inline-flex min-h-10 items-center justify-center gap-2 border px-4 text-sm font-semibold transition duration-200 ease-out hover:-translate-y-px active:translate-y-0 active:scale-[.985] disabled:pointer-events-none disabled:opacity-50',
    primary: 'border-[color-mix(in_oklch,var(--accent)_58%,white_8%)] bg-[linear-gradient(135deg,color-mix(in_oklch,var(--accent)_92%,black_8%),color-mix(in_oklch,var(--accent-2)_76%,black_18%))] text-white shadow-[0_0_28px_rgba(122,118,255,.22)]',
    secondary: 'border-[var(--border-strong)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] text-[var(--fg)] hover:border-[color-mix(in_oklch,var(--accent)_42%,var(--border-strong))]',
    ghost: 'border-transparent bg-transparent text-[var(--muted)] hover:bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] hover:text-[var(--fg)]',
    danger: 'border-[color-mix(in_oklch,var(--accent-coral)_42%,var(--border))] text-[color-mix(in_oklch,var(--accent-coral)_88%,white_10%)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)]',
    icon: 'grid size-10 place-items-center rounded-[12px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] text-[var(--muted)] transition hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:text-[var(--fg)] active:scale-[.96]',
  },
  form: {
    label: 'mb-2 flex items-center justify-between gap-3 text-[12px] font-semibold text-[var(--muted)]',
    input: 'w-full rounded-[12px] border border-[var(--border)] bg-[var(--input)] px-3.5 py-2.5 text-sm text-[var(--fg)] outline-none transition placeholder:text-[var(--faint)] focus:border-[var(--accent)] focus:ring-3 focus:ring-[color-mix(in_oklch,var(--accent)_18%,transparent)]',
    textarea: 'w-full min-h-[138px] resize-y rounded-[14px] border border-[var(--border)] bg-[var(--input)] px-4 py-3 text-sm leading-[1.7] text-[var(--fg)] outline-none transition placeholder:text-[var(--faint)] focus:border-[var(--accent)] focus:ring-3 focus:ring-[color-mix(in_oklch,var(--accent)_18%,transparent)]',
  },
  image: {
    card: 'group relative overflow-hidden rounded-[18px] border border-[var(--border)] bg-[var(--surface-solid)] transition duration-300 hover:-translate-y-1 hover:border-[color-mix(in_oklch,var(--accent)_32%,var(--border-strong))]',
    media: 'block size-full object-cover transition duration-500 group-hover:scale-[1.035]',
    overlay: 'absolute inset-0 bg-[linear-gradient(to_top,rgba(7,8,13,.92),rgba(7,8,13,.34)_42%,transparent_74%)]',
  },
  state: {
    empty: 'grid min-h-[320px] place-items-center rounded-[22px] border border-dashed border-[var(--border-strong)] bg-[radial-gradient(circle_at_42%_28%,rgba(122,118,255,.16),transparent_28%),var(--surface)] p-8 text-center',
    skeleton: 'animate-pulse rounded-[14px] bg-[color-mix(in_oklch,var(--fg)_8%,transparent)]',
  },
}
```

### 5.3 Studio components

新增：`web/user/src/ui/studio.tsx`

#### 5.3.1 `StudioButton`

```ts
type StudioButtonTone = 'primary' | 'secondary' | 'ghost' | 'danger'

type StudioButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: StudioButtonTone
  size?: 'sm' | 'md' | 'lg'
  icon?: React.ReactNode
  fullWidth?: boolean
}

function StudioButton(props: StudioButtonProps) {
  const sizeClass = {
    sm: 'min-h-8 rounded-[10px] px-3 text-xs',
    md: 'min-h-10 rounded-[12px] px-4 text-sm',
    lg: 'min-h-[52px] rounded-[16px] px-5 text-base',
  }[props.size ?? 'md']

  return (
    <button
      {...props}
      className={cn(
        studio.button.base,
        studio.button[props.tone ?? 'secondary'],
        sizeClass,
        props.fullWidth && 'w-full',
        props.className,
      )}
    >
      {props.icon}
      {props.children}
    </button>
  )
}
```

Contract:

- 所有页面 CTA 使用 `StudioButton`。
- 图标按钮用 `IconButton`，不再手写 `rounded-full border ...`。
- 禁止页面内直接使用 `userButton.base` 新增按钮，旧代码迁移后只保留兼容用途。

#### 5.3.2 `StudioPanel`

```ts
type StudioPanelProps = {
  title?: React.ReactNode
  eyebrow?: React.ReactNode
  action?: React.ReactNode
  children: React.ReactNode
  className?: string
  density?: 'comfortable' | 'compact'
}

function StudioPanel({ eyebrow, title, action, children, density = 'comfortable', className }: StudioPanelProps) {
  return (
    <section className={cn(studio.surface.glass, studio.radius.panel, density === 'compact' ? 'p-4' : 'p-6', className)}>
      {(eyebrow || title || action) && (
        <header className="mb-5 flex items-start justify-between gap-4">
          <div className="min-w-0">
            {eyebrow && <p className="mb-1 font-vault-mono text-[10px] uppercase tracking-[.14em] text-[var(--accent)]">{eyebrow}</p>}
            {title && <h2 className="m-0 text-[15px] font-semibold text-[var(--fg)]">{title}</h2>}
          </div>
          {action}
        </header>
      )}
      {children}
    </section>
  )
}
```

#### 5.3.3 `SegmentedControl`

```ts
type SegmentOption<T extends string> = {
  value: T
  label: string
  hint?: string
  icon?: React.ReactNode
}

function SegmentedControl<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: SegmentOption<T>[]
  onChange: (value: T) => void
}) {
  return (
    <div className="grid grid-cols-[repeat(var(--segment-count),minmax(0,1fr))] gap-1 rounded-[14px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_68%,transparent)] p-1">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={cn(
            'min-h-10 rounded-[10px] px-3 text-sm font-semibold text-[var(--muted)] transition',
            value === option.value && 'bg-[var(--elevated)] text-[var(--fg)] shadow-[var(--pg-user-inner-line)]',
          )}
          onClick={() => onChange(option.value)}
        >
          {option.icon}
          {option.label}
        </button>
      ))}
    </div>
  )
}
```

#### 5.3.4 `StudioModelCard`

```ts
type StudioModelCardModel = {
  code: string
  name: string
  displayPoints?: string
  multiplier?: string
  taskTypes: ImageTaskType[]
  qualities: string[]
  cover?: string
  tag?: string
  description?: string
}

function StudioModelCard({
  model,
  selected,
  onSelect,
}: {
  model: StudioModelCardModel
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      className={cn(
        studio.surface.solid,
        studio.radius.card,
        'grid min-h-[92px] grid-cols-[56px_minmax(0,1fr)_auto] items-center gap-3 p-3 text-left transition hover:-translate-y-px',
        selected && 'border-[var(--border-glow)] bg-[color-mix(in_oklch,var(--accent)_12%,var(--surface-solid))] shadow-[var(--pg-user-shadow-glow)]',
      )}
      onClick={onSelect}
    >
      <ModelCover src={model.cover} name={model.name} />
      <span className="min-w-0">
        <strong className="block truncate text-sm text-[var(--fg)]">{model.name}</strong>
        <small className="mt-1 block truncate text-xs text-[var(--dim)]">{model.description ?? model.code}</small>
      </span>
      <em className="font-vault-mono text-[11px] not-italic text-[var(--accent)]">
        {model.displayPoints ? `${model.displayPoints} ◈` : model.multiplier ? `${model.multiplier}x` : ''}
      </em>
    </button>
  )
}
```

Important:

- `cover` 首版前端不要求后端提供。没有 cover 时使用 `ModelCover` 的抽象渐变/字母 tile。
- 后续如果后台给模型配置封面，再接 `cover_image_url` 字段。

#### 5.3.5 `AspectRatioTile`

```ts
type AspectRatioTileProps = {
  value: string
  selected: boolean
  disabled?: boolean
  onSelect: () => void
}

function ratioShape(value: string) {
  const [w, h] = value.split(':').map(Number)
  if (!w || !h) return { width: '26px', height: '26px' }
  if (w > h) return { width: '32px', height: '20px' }
  if (w < h) return { width: '20px', height: '32px' }
  return { width: '26px', height: '26px' }
}

function AspectRatioTile({ value, selected, disabled, onSelect }: AspectRatioTileProps) {
  return (
    <button
      type="button"
      disabled={disabled}
      className={cn(
        'grid min-h-[74px] place-items-center gap-1 rounded-[14px] border bg-[var(--input)] p-2 transition',
        selected ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-[var(--border)] text-[var(--muted)] hover:border-[var(--border-strong)] hover:text-[var(--fg)]',
      )}
      onClick={onSelect}
    >
      <span className="grid place-items-center rounded-[6px] border border-current/55" style={ratioShape(value)} />
      <span className="font-vault-mono text-[11px]">{value}</span>
    </button>
  )
}
```

#### 5.3.6 `StudioImageCard`

```ts
type StudioImageCardAction = {
  key: string
  label: string
  icon: React.ReactNode
  onClick: () => void
  disabled?: boolean
  tone?: 'normal' | 'danger' | 'accent'
}

type StudioImageCardProps = {
  imageUrl?: string
  title: string
  meta?: string
  prompt?: string
  aspectRatio?: string
  selected?: boolean
  actions?: StudioImageCardAction[]
  onOpen?: () => void
}

function StudioImageCard(props: StudioImageCardProps) {
  return (
    <article className={studio.image.card}>
      <button type="button" className="block w-full bg-transparent p-0 text-left" onClick={props.onOpen}>
        <div className="relative aspect-[var(--studio-card-ratio,1/1)] overflow-hidden bg-[var(--canvas)]">
          {props.imageUrl ? <img src={props.imageUrl} alt={props.title} className={studio.image.media} /> : <ImagePlaceholder />}
          <div className={studio.image.overlay} />
          {props.prompt && <p className="absolute bottom-3 left-3 right-3 line-clamp-3 text-xs leading-[1.55] text-white/82 opacity-0 transition group-hover:opacity-100">{props.prompt}</p>}
        </div>
      </button>
      <footer className="grid gap-2 p-4">
        <strong className="truncate text-sm">{props.title}</strong>
        {props.meta && <span className="font-vault-mono text-[11px] text-[var(--dim)]">{props.meta}</span>}
        {props.actions?.length ? <ActionBar actions={props.actions} /> : null}
      </footer>
    </article>
  )
}
```

#### 5.3.7 `StudioEmptyState`

```ts
type StudioEmptyAction = {
  label: string
  onClick: () => void
  tone?: StudioButtonTone
}

function StudioEmptyState({
  title,
  detail,
  actions,
  suggestions,
}: {
  title: string
  detail: string
  actions?: StudioEmptyAction[]
  suggestions?: Array<{ label: string; value: string; onApply: () => void }>
}) {
  return (
    <section className={studio.state.empty}>
      <div className="mx-auto max-w-[520px]">
        <IconMark />
        <h2 className="mt-5 text-[22px] font-semibold">{title}</h2>
        <p className="mt-2 text-sm leading-[1.7] text-[var(--muted)]">{detail}</p>
        {suggestions?.length ? <SuggestionRow suggestions={suggestions} /> : null}
        {actions?.length ? <ButtonRow actions={actions} /> : null}
      </div>
    </section>
  )
}
```

Use cases:

- Workspace no task
- Gallery empty
- Public gallery empty
- Checkout empty plan/order
- API Key empty
- Docs load failure fallback

## 6. Generation Progress Design

This is the most important interaction contract for future extensibility.

### 6.1 Current constraints

Current backend does not expose detailed steps. It sends:

- `event: history` with `ImageTask[]`
- `event: task` with `ImageTask`
- optional `progress` derived in `user-api.ts` from status

Therefore the first frontend redesign must compute a stable local progress view from available task fields.

### 6.2 Frontend phase model

新增：`web/user/src/ui/studioProgress.ts`

```ts
export type GenerationPhaseKey =
  | 'draft'
  | 'prompt_prepare'
  | 'prompt_optimize'
  | 'estimate'
  | 'queue'
  | 'route'
  | 'provider_call'
  | 'storage'
  | 'finalize'
  | 'complete'
  | 'failed'

export type GenerationPhaseStatus =
  | 'pending'
  | 'active'
  | 'done'
  | 'failed'
  | 'skipped'

export type GenerationPhase = {
  key: GenerationPhaseKey
  label: string
  detail: string
  status: GenerationPhaseStatus
  progress: number
  startedAt?: string
  finishedAt?: string
}

export type GenerationProgressView = {
  taskId?: string
  headline: string
  detail: string
  percent: number
  tone: 'idle' | 'active' | 'success' | 'warning' | 'danger'
  currentKey: GenerationPhaseKey
  phases: GenerationPhase[]
}
```

### 6.3 Frontend resolver pseudocode

```ts
export function generationProgressView(task?: ImageTask | null, context?: {
  localStage?: 'drafting' | 'submitting' | 'estimating'
  promptOptimizationEnabled?: boolean
}): GenerationProgressView {
  const basePhases = [
    phase('prompt_prepare', '整理提示词', '检查输入、参考图和生成参数', 8),
    phase('prompt_optimize', '提示词优化', '预留节点：可在生成前优化描述', 18),
    phase('estimate', '费用预估', '计算模型、清晰度和图片数量消耗', 28),
    phase('queue', '进入队列', '任务已创建，等待执行器领取', 38),
    phase('route', '模型路由', '匹配可用模型账号和降级策略', 52),
    phase('provider_call', '图像生成', '上游模型正在返回图像结果', 72),
    phase('storage', '结果入库', '保存图片并同步历史图库', 88),
    phase('finalize', '结算完成', '处理积分扣减和任务状态', 96),
    phase('complete', '生成完成', '可以预览、下载或继续编辑', 100),
  ]

  if (!task && context?.localStage === 'submitting') {
    return markActive(basePhases, 'prompt_prepare', 8)
  }

  if (!task) {
    return idleView(basePhases)
  }

  if (task.status === 'queued') {
    return markActive(basePhases, 'queue', 38)
  }

  if (task.status === 'running') {
    if (task.attempts?.length) return markActive(basePhases, 'provider_call', 72)
    return markActive(basePhases, 'route', 52)
  }

  if (task.status === 'succeeded') {
    return markDone(basePhases, 'complete')
  }

  if (task.status === 'partial_failed') {
    return warningView(basePhases, 'complete', '部分图片生成成功')
  }

  if (['failed', 'cancelled', 'rejected'].includes(task.status)) {
    return failedView(basePhases, task.error_message ?? task.failure_reason ?? '生成失败')
  }

  return idleView(basePhases)
}
```

Rules:

- `prompt_optimize` 首版默认 `skipped` 或 `pending`，由 `promptOptimizationEnabled` 控制展示。
- 当前前端先不真的优化 prompt，但 UI 可以显示“已跳过”或隐藏该节点。
- 后续后端接入时，SSE 返回节点状态即可覆盖本地推断。

### 6.4 `GenerationProgressPanel`

新增组件：`web/user/src/ui/studio.tsx`

```ts
function GenerationProgressPanel({ view }: { view: GenerationProgressView }) {
  return (
    <StudioPanel className="min-h-[360px]">
      <div className="grid gap-6">
        <div className="grid justify-items-center gap-4 text-center">
          <ProgressOrb tone={view.tone} percent={view.percent} />
          <div>
            <h3 className="text-lg font-semibold">{view.headline}</h3>
            <p className="mt-1 text-xs text-[var(--dim)]">{view.detail}</p>
          </div>
        </div>
        <div className="h-2 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--input)]">
          <div className="h-full rounded-full bg-[linear-gradient(90deg,var(--accent),var(--accent-2))] transition-[width] duration-500" style={{ width: `${view.percent}%` }} />
        </div>
        <ol className="grid gap-2">
          {view.phases.map((phase) => <GenerationPhaseRow key={phase.key} phase={phase} />)}
        </ol>
      </div>
    </StudioPanel>
  )
}
```

### 6.5 Future backend progress contract

#### 6.5.1 API type extension

Modify later: `web/shared/api-types.ts`

```ts
export type ImageTaskProgressNodeStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'skipped'

export type ImageTaskProgressNode = {
  key:
    | 'prompt_prepare'
    | 'prompt_optimize'
    | 'estimate'
    | 'queue'
    | 'route'
    | 'provider_call'
    | 'storage'
    | 'billing'
    | 'complete'
  label?: string
  status: ImageTaskProgressNodeStatus
  progress: number
  message?: string
  input_excerpt?: string
  output_excerpt?: string
  started_at?: string | null
  finished_at?: string | null
  error_code?: string | null
  error_message?: string | null
}

export type ImageTask = {
  // existing fields...
  progress_nodes?: ImageTaskProgressNode[]
  current_progress_node?: string
  optimized_prompt?: string
  original_prompt?: string
}
```

#### 6.5.2 SSE extension

Existing:

```txt
event: task
data: ImageTask
```

Add later:

```txt
event: task_phase
data: {
  "task_id": "uuid",
  "node": {
    "key": "prompt_optimize",
    "status": "running",
    "progress": 18,
    "message": "正在优化提示词结构"
  },
  "task": ImageTask // optional, if backend wants to refresh snapshot
}
```

Frontend handling:

```ts
source.addEventListener('task_phase', (event) => {
  const payload = JSON.parse((event as MessageEvent).data) as TaskPhaseEvent
  setProgressOverrides((current) => upsertNode(current, payload.task_id, payload.node))
  if (payload.task) setRecords((items) => mergeGenerationRecord(items, toTask(payload.task)))
})
```

#### 6.5.3 Backend domain extension

Modify later: `internal/domain/imagetask/types.go`

```go
type ProgressNode struct {
	Key           string     `json:"key"`
	Label         string     `json:"label,omitempty"`
	Status        string     `json:"status"`
	Progress      int        `json:"progress"`
	Message       string     `json:"message,omitempty"`
	InputExcerpt  string     `json:"input_excerpt,omitempty"`
	OutputExcerpt string     `json:"output_excerpt,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
}

type Task struct {
	// existing fields...
	OriginalPrompt       string         `json:"original_prompt,omitempty"`
	OptimizedPrompt      string         `json:"optimized_prompt,omitempty"`
	CurrentProgressNode  string         `json:"current_progress_node,omitempty"`
	ProgressNodes        []ProgressNode `json:"progress_nodes,omitempty"`
}

type CreateRequest struct {
	// existing fields...
	PromptOptimizeMode string `json:"prompt_optimize_mode,omitempty"` // off | auto | required
}
```

#### 6.5.4 Backend persistence extension

Modify later: `internal/repository/ent/schema/imagetask.go`

```go
field.Text("original_prompt").Optional().Nillable(),
field.Text("optimized_prompt").Optional().Nillable(),
field.String("current_progress_node").MaxLen(64).Optional().Nillable(),
field.JSON("progress_nodes", []map[string]any{}).Optional(),
```

Modify later: `internal/repository/entstore/imagetask_store.go`

- `createImageTask(...)`:

```go
builder.
  SetProviderTrace(trace).
  SetProgressNodes(progressNodesJSON(task.ProgressNodes))

if strings.TrimSpace(task.OriginalPrompt) != "" {
  builder.SetOriginalPrompt(task.OriginalPrompt)
}
if strings.TrimSpace(task.OptimizedPrompt) != "" {
  builder.SetOptimizedPrompt(task.OptimizedPrompt)
}
if strings.TrimSpace(task.CurrentProgressNode) != "" {
  builder.SetCurrentProgressNode(task.CurrentProgressNode)
}
```

- `updateImageTask(...)` 同步更新这几个字段。
- `mapImageTaskEntity(...)` 反序列化字段到 `Task`。

#### 6.5.5 Backend service extension

Modify later: `internal/service/imagetask/service.go`

Add helper:

```go
func (s *Service) markProgress(ctx context.Context, task *domainimagetask.Task, owner string, node domainimagetask.ProgressNode) error {
	now := time.Now().UTC()
	node = normalizeProgressNode(node, now)
	task.ProgressNodes = upsertProgressNode(task.ProgressNodes, node)
	task.CurrentProgressNode = node.Key
	if owner == "" {
		return s.store.Save(ctx, *task)
	}
	return s.saveOwnedTask(ctx, *task, owner)
}
```

Hook points:

```go
func (s *Service) CreateTask(ctx context.Context, req CreateRequest) (Task, error) {
  markLocal(&task, node("prompt_prepare", "succeeded", 8))

  if req.PromptOptimizeMode == "auto" || req.PromptOptimizeMode == "required" {
    markLocal(&task, node("prompt_optimize", "running", 18))
    optimized, err := s.promptOptimizer.Optimize(ctx, req.Prompt)
    if err != nil && req.PromptOptimizeMode == "required" { return failed }
    if err == nil {
      req.OriginalPrompt = req.Prompt
      req.Prompt = optimized
      task.OriginalPrompt = req.OriginalPrompt
      task.OptimizedPrompt = optimized
      markLocal(&task, node("prompt_optimize", "succeeded", 20))
    } else {
      markLocal(&task, node("prompt_optimize", "skipped", 20))
    }
  }

  markLocal(&task, node("estimate", "succeeded", 30))
  save queued task
}

func (s *Service) ExecuteLeasedTask(...) {
  s.markProgress(ctx, &task, owner, node("route", "running", 52))
  resolved, err := s.resolveTask(...)
  s.markProgress(ctx, &task, owner, node("route", "succeeded", 60))

  s.markProgress(ctx, &task, owner, node("provider_call", "running", 72))
  resp, err := s.executeProviderRequest(...)
  s.markProgress(ctx, &task, owner, node("provider_call", "succeeded", 82))

  s.markProgress(ctx, &task, owner, node("storage", "running", 88))
  persisted, err := s.persistImageResults(...)
  s.markProgress(ctx, &task, owner, node("storage", "succeeded", 92))

  s.markProgress(ctx, &task, owner, node("billing", "running", 96))
  settle billing
  s.markProgress(ctx, &task, owner, node("complete", "succeeded", 100))
}
```

#### 6.5.6 SSE backend extension

Modify later: `internal/http/handlers/api.go`

Current `taskStreamSignature`:

```go
return fmt.Sprintf("%s:%s:%s:%d", task.ID, task.Status, task.UpdatedAt.Format(time.RFC3339Nano), len(task.Results))
```

Change later:

```go
return fmt.Sprintf(
  "%s:%s:%s:%d:%s:%d",
  task.ID,
  task.Status,
  task.UpdatedAt.Format(time.RFC3339Nano),
  len(task.Results),
  task.CurrentProgressNode,
  len(task.ProgressNodes),
)
```

Optional event writer:

```go
func writeTaskPhaseSSE(w io.Writer, task domainimagetask.Task) {
  if len(task.ProgressNodes) == 0 { return }
  node := task.ProgressNodes[len(task.ProgressNodes)-1]
  writeSSE(w, "task_phase", map[string]any{
    "task_id": task.ID,
    "node": node,
  })
}
```

## 7. Page-Level Redesign Contracts

### 7.1 Shell

Modify:

- `web/user/src/components.tsx`
- `web/user/src/ui/classes.ts`

Current issue:

- 左侧导航可用，但品牌 orb 偏“渐变小图标”，不够稳定。
- topbar 按钮、状态 chip 和页面内容视觉层级不够统一。
- 快捷入口文字略分散。

Target:

- 继续使用左侧窄导航，贴近用户参考图。
- 品牌区改为 “Pic Gallery / Studio” 符号化。
- Sidebar active 使用暗面板 + 蓝紫发光下边缘，不用大面积填充。
- Topbar 改成“Studio command rail”：左侧显示当前页上下文，右侧显示额度、模型状态、账号。

Pseudocode:

```tsx
function Shell({ children }) {
  const current = routeMeta[app.route]

  return (
    <div className={userShell.shell}>
      <aside className={userShell.sidebar}>
        <BrandMark />
        <PrimaryNav items={navItems} active={app.route} />
        <SecondaryNav />
      </aside>
      <main className={userShell.main}>
        <header className={userShell.topbar}>
          <div className="min-w-0">
            <p className="studio-eyebrow">{current.eyebrow}</p>
            <h1 className="studio-topbar-title">{current.title}</h1>
          </div>
          <TopbarActions balance={app.balance} profile={app.profile} />
        </header>
        <div className={userShell.routeSurface}>{children}</div>
      </main>
    </div>
  )
}
```

Route metadata:

```ts
const routeMeta: Record<RouteId, { eyebrow: string; title: string }> = {
  home: { eyebrow: 'Studio overview', title: '创意中枢' },
  genpic: { eyebrow: 'Generation workspace', title: '创作工坊' },
  gallery: { eyebrow: 'Private archive', title: '私人图库' },
  'public-gallery': { eyebrow: 'Inspiration market', title: '公开广场' },
  checkout: { eyebrow: 'Credits and plans', title: '积分与订阅' },
  'api-keys': { eyebrow: 'Developer access', title: 'API 密钥' },
  profile: { eyebrow: 'Account studio', title: '账户中心' },
  docs: { eyebrow: 'Open API', title: '开发文档' },
}
```

### 7.2 WorkspacePage

Modify:

- `web/user/src/pages/WorkspacePage.tsx`
- `web/user/src/pages/workspaceGenerateReadiness.ts`
- `web/user/src/pages/workspaceTaskFailure.ts`
- `web/user/src/pages/workspaceImageActions.ts`
- `web/user/src/ui/studio.tsx`
- `web/user/src/ui/studioProgress.ts`

Target layout:

```txt
┌──────────────────────────────────────────────────────────────┐
│ Workspace                                                    │
├────────────── Control Panel ─────────────┬── Canvas Feed ────┤
│ mode segmented                           │ latest active task │
│ edit/reference source                    │ progress panel     │
│ prompt composer                          │ generated images   │
│ model cards                              │ task history feed   │
│ quality/ratio/count tiles                │ floating actions    │
│ estimate + create CTA                    │                    │
└──────────────────────────────────────────┴────────────────────┘
```

#### 7.2.1 Control panel contract

```tsx
<StudioPanel eyebrow="Create" title={mode === 'text' ? '文生图' : '参考生图'}>
  <SegmentedControl value={mode} options={modeOptions} onChange={setMode} />
  <ReferenceSourcePanel visible={mode === 'reference'} />
  <EditSourcePanel visible={mode === 'text'} />
  <PromptComposer
    prompt={prompt}
    negative={negative}
    onPromptChange={setPrompt}
    onNegativeChange={setNegative}
    onClear={() => setPrompt('')}
    optimizeState="reserved"
  />
  <ModelCardList models={availableModels} selected={model} onSelect={setModel} />
  <GenerationParams qualities={qualities} ratios={ratios} counts={counts} />
  <EstimateFooter estimate={estimate} readiness={generateReadiness} onCreate={createTask} />
</StudioPanel>
```

`PromptComposer`:

```ts
type PromptComposerProps = {
  prompt: string
  negative: string
  onPromptChange: (value: string) => void
  onNegativeChange: (value: string) => void
  onClear: () => void
  optimizeState: 'hidden' | 'reserved' | 'available' | 'optimizing'
  onOptimize?: () => void
}
```

首版：

- `optimizeState="reserved"` 显示“提示词优化（即将开放）”弱按钮或 info badge。
- 不调用后端。

后续：

- `optimizeState="available"` 时点击调用 `POST /api/agent/image/v1/prompts/optimize` 或随 `createTask` 请求 `prompt_optimize_mode=auto`。

#### 7.2.2 Model card adapter

Current `CapabilityModelGroup` does not provide cover image. Adapter:

```ts
function studioModelFromCapability(model: CapabilityModelGroup): StudioModelCardModel {
  return {
    code: model.code,
    name: model.name,
    displayPoints: model.display_points,
    multiplier: model.effective_multiplier,
    taskTypes: model.task_types,
    qualities: model.qualities ?? [],
    tag: model.max_output_image_count > 1 ? 'Multi' : 'Single',
    description: [
      model.task_types.includes('image_edit') ? '支持图像编辑' : '',
      model.task_types.includes('reference_to_image') ? '支持参考图' : '',
      `${model.qualities?.length ?? 0} 个质量档位`,
    ].filter(Boolean).join(' · '),
  }
}
```

#### 7.2.3 Canvas contract

```tsx
function WorkspaceCanvas({
  records,
  activeTask,
  busy,
  progressView,
}) {
  if (!records.length && !busy) {
    return (
      <StudioEmptyState
        title="还没有生成任务"
        detail="写下一个画面、上传参考图，或从下方提示词开始。"
        suggestions={workspacePromptSuggestions}
      />
    )
  }

  return (
    <section className="grid gap-5">
      {activeTask && !isTerminalStatus(activeTask.status) && (
        <GenerationProgressPanel view={progressView} />
      )}
      <TaskFeed records={records} />
    </section>
  )
}
```

Active task selection:

```ts
const activeTask = useMemo(() => {
  return [...records].reverse().find((task) => !isTerminalStatus(task.status))
}, [records])
```

#### 7.2.4 Generated record card

Replace current timeline-like record with Studio card:

```tsx
function GenerationRecord({ task }) {
  const view = workspaceTaskCardView(task)
  const progress = generationProgressView(task)

  return (
    <StudioPanel className="overflow-hidden">
      <RecordHeader
        title={view.title}
        status={task.status}
        meta={`${task.route_model_code} · ${task.quality} · ${task.aspect_ratio} · ${displayTaskPoints(task)} ◈`}
      />
      {isPending(task.status) && <GenerationProgressPanel view={progress} />}
      {isFailure(task.status) && <TaskFailureBlock task={task} />}
      {task.reference_assets?.length ? <ReferenceStrip assets={task.reference_assets} /> : null}
      {task.results.length ? (
        <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3">
          {task.results.map((image) => (
            <StudioImageCard
              key={image.id || image.url}
              imageUrl={assetUrl(image.url)}
              title={image.id || '生成结果'}
              prompt={task.prompt}
              actions={imageActions(task, image)}
            />
          ))}
        </div>
      ) : null}
    </StudioPanel>
  )
}
```

### 7.3 HomePage

Modify:

- `web/user/src/pages/HomePage.tsx`
- `web/user/src/pages/homeGalleryModel.ts`
- `web/user/src/pages/homeReadinessModel.contract.ts`

Target:

- 首页是 Studio overview，不是普通 dashboard。
- 顶部大 Hero 展示“创作工坊入口 + 当前账户状态 + 最近公开灵感”。
- readiness strip 改为更轻的状态 rail。
- 公开图库保持 masonry，但卡片改用 `StudioImageCard`。

Pseudocode:

```tsx
function HomePage() {
  return (
    <div className={studio.layout.content}>
      <section className="grid grid-cols-[minmax(0,1.15fr)_360px] gap-5 max-[980px]:grid-cols-1">
        <HeroShowcase image={heroImage} onGenerate={() => app.navigate('genpic')} />
        <StudioStatusPanel balance={app.balance} capability={capability.data} />
      </section>
      <QuickCreateStrip suggestions={promptSuggestions} />
      <InspirationMasonry cards={cards} />
    </div>
  )
}
```

`HeroShowcase`:

```tsx
function HeroShowcase() {
  return (
    <section className="relative min-h-[420px] overflow-hidden rounded-[28px] border border-[var(--border)]">
      <img className="absolute inset-0 size-full object-cover" />
      <div className="absolute inset-0 bg-[linear-gradient(to_top,var(--bg),rgba(7,8,13,.48),transparent)]" />
      <div className="relative z-10 flex min-h-[420px] max-w-[760px] flex-col justify-end p-8">
        <p className="studio-eyebrow">Creative Studio</p>
        <h1 className="font-vault-display text-[clamp(3.2rem,7vw,6.4rem)] leading-[.86]">把提示词变成可用视觉资产</h1>
        <p className="mt-5 max-w-[58ch] text-sm leading-[1.8] text-[var(--muted)]">...</p>
        <div className="mt-6 flex gap-3">
          <StudioButton tone="primary">进入创作工坊</StudioButton>
          <StudioButton tone="secondary">浏览公开广场</StudioButton>
        </div>
      </div>
    </section>
  )
}
```

### 7.4 GalleryPage

Modify:

- `web/user/src/pages/GalleryPage.tsx`
- `web/user/src/pages/galleryRows.ts`
- `web/user/src/pages/galleryRows.contract.ts`

Target:

- 私人图库像“作品档案库”，卡片重心放图片。
- 筛选区改为 `StudioPanel` 内的 toolbar，减少浮动碎片。
- 批量栏统一成 `StudioActionBar`。
- 图片卡统一 `StudioImageCard`，私有/审核状态用 `StatusBadge`。

Pseudocode:

```tsx
function GalleryPage() {
  return (
    <div className={studio.layout.content}>
      <PageHero title="私人图库" detail="管理、分组、下载和继续编辑你的生成结果。" />
      <StudioPanel density="compact">
        <GalleryFilters ... />
      </StudioPanel>
      {selectedIds.size > 0 && <BatchActionBar />}
      {loading ? <StudioSkeletonGrid /> : null}
      {!loading && !filtered.length ? <StudioEmptyState ... /> : (
        <StudioImageGrid>
          {filtered.map(image => <PrivateImageCard image={image} />)}
        </StudioImageGrid>
      )}
    </div>
  )
}
```

### 7.5 PublicGalleryPage

Modify:

- `web/user/src/pages/PublicGalleryPage.tsx`
- `web/user/src/pages/publicGalleryModel.ts`

Target:

- 更接近参考图 2 的“创意广场”。
- 图片卡可变高度或大图网格；不强制所有卡同样尺寸。
- 未登录用户可以浏览缩略图，但详情/完整 prompt 保持现有登录门槛。

Pseudocode:

```tsx
function PublicGalleryPage() {
  return (
    <div className={studio.layout.content}>
      <PageHero title="公开广场" detail="从真实作品中寻找模型、光影和构图灵感。" action={<StudioButton>发布我的创意</StudioButton>} />
      <StudioPanel density="compact"><SearchAndFilters /></StudioPanel>
      <MasonryGrid>
        {filtered.map(image => (
          <StudioImageCard
            imageUrl={assetUrl(image.url)}
            title={card.title}
            meta={`${card.author} · ${card.model}`}
            prompt={card.promptExcerpt}
            actions={publicActions(image)}
          />
        ))}
      </MasonryGrid>
    </div>
  )
}
```

### 7.6 LandingPage and LoginPage

Modify:

- `web/user/src/pages/LandingPage.tsx`
- `web/user/src/pages/LoginPage.tsx`

Target:

- Landing 第一屏必须直接传达 Pic Gallery 是创作者图像工作台，不要营销站空话。
- Login 改成同一 Studio 背景下的双栏：
  - 左侧作品/品牌场景
  - 右侧登录卡
- 保留现有邮箱验证码/密码登录逻辑。

Landing pseudocode:

```tsx
function LandingPage() {
  return (
    <main className="min-h-dvh bg-[var(--bg)]">
      <LandingNav />
      <section className="grid min-h-[calc(100dvh-72px)] grid-cols-[1fr_440px]">
        <HeroCopy />
        <HeroStudioPreview />
      </section>
      <FeatureBand />
      <GalleryPreview />
    </main>
  )
}
```

Login pseudocode:

```tsx
function LoginPage() {
  return (
    <main className="grid min-h-dvh grid-cols-[minmax(0,1fr)_440px] max-[900px]:grid-cols-1">
      <LoginArtworkPanel />
      <StudioPanel className="m-6 self-center">
        <AuthTabs />
        <EmailCodeForm />
        <PasswordForm />
      </StudioPanel>
    </main>
  )
}
```

### 7.7 Secondary Pages

Pages:

- `CheckoutPage`
- `ApiKeysPage`
- `ProfilePage`
- `DocsPage`

Contract:

- 只做视觉系统迁移，不改业务流程。
- 页面顶层统一：

```tsx
<div className={studio.layout.content}>
  <PageHero title="..." detail="..." action={...} />
  <div className="grid gap-5">...</div>
</div>
```

- 空状态统一 `StudioEmptyState`。
- 表格/列表卡统一 `StudioPanel` + `StudioDataRow`。

## 8. Backend API Change Plan for Future Nodes

This section is not required for the first visual-only implementation, but defines how to change backend when prompt optimization and richer progress are implemented.

### 8.1 New prompt optimization endpoint

Optional later:

Path:

```txt
POST /api/agent/image/v1/prompts/optimize
```

Modify:

- `web/shared/api-types.ts`
- `web/shared/user-api.ts`
- `internal/http/router/router.go`
- `internal/http/handlers/api.go`
- new service package: `internal/service/promptopt`
- optional config: `internal/config/config.go`

Request:

```ts
type OptimizePromptRequest = {
  prompt: string
  locale?: 'zh-CN' | 'en-US' | string
  task_type?: ImageTaskType
  route_model_code?: string
  style_hint?: string
  reference_asset_ids?: string[]
}
```

Response:

```ts
type OptimizePromptResponse = {
  original_prompt: string
  optimized_prompt: string
  negative_prompt_suggestion?: string
  changes: Array<{
    type: 'lighting' | 'composition' | 'style' | 'subject' | 'quality' | string
    label: string
    detail: string
  }>
  provider?: string
  model?: string
}
```

Handler pseudocode:

```go
func (a *API) HandleAgentPromptOptimize(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost { writeMethodNotAllowed(w, r); return }
  user, appErr := a.requireUser(r)
  if appErr != nil { httpx.WriteError(w, r, appErr); return }

  var req promptopt.Request
  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
    return
  }
  req.UserID = user.ID

  result, err := a.promptOptimizer.Optimize(r.Context(), req)
  if err != nil { httpx.WriteError(w, r, normalizeAppError(err)); return }
  httpx.WriteSuccess(w, r, http.StatusOK, result)
}
```

Router:

```go
mux.HandleFunc("/api/agent/image/v1/prompts/optimize", api.HandleAgentPromptOptimize)
```

Frontend:

```ts
optimizePrompt: (input: OptimizePromptRequest) =>
  sharedApiClient.request<OptimizePromptResponse>('/api/agent/image/v1/prompts/optimize', {
    method: 'POST',
    body: input,
  })
```

### 8.2 Extend create task with prompt optimization mode

Path remains:

```txt
POST /api/agent/image/v1/tasks
```

Modify request struct in `internal/http/handlers/api.go:6051`:

```go
var req struct {
  TaskType string `json:"task_type"`
  Prompt string `json:"prompt"`
  NegativePrompt string `json:"negative_prompt"`
  PromptOptimizeMode string `json:"prompt_optimize_mode"`
  // existing fields...
}
```

Pass to domain:

```go
result, err := a.tasks.CreateTask(r.Context(), domainimagetask.CreateRequest{
  Prompt: req.Prompt,
  NegativePrompt: req.NegativePrompt,
  PromptOptimizeMode: req.PromptOptimizeMode,
  // existing fields...
})
```

Important current bug/opportunity:

- `web/shared/user-api.ts` currently merges negative prompt into `prompt`:

```ts
prompt: req.negative_prompt ? `${req.prompt}\n\nNegative prompt: ${req.negative_prompt}` : req.prompt
```

Better future contract:

```ts
function toBackendTask(req: CreateTaskRequest) {
  return {
    task_type: toBackendTaskType(req.task_type),
    prompt: req.prompt,
    negative_prompt: req.negative_prompt,
    prompt_optimize_mode: req.prompt_optimize_mode ?? 'off',
    // existing fields...
  }
}
```

Backend already has `NegativePrompt` in `domainimagetask.CreateRequest` and `Task`, but handler does not decode it. Add this before implementing prompt optimization.

### 8.3 Store progress nodes

Modify Ent schema later:

- `internal/repository/ent/schema/imagetask.go`

Add:

```go
field.Text("original_prompt").Optional().Nillable(),
field.Text("optimized_prompt").Optional().Nillable(),
field.String("current_progress_node").MaxLen(64).Optional().Nillable(),
field.JSON("progress_nodes", []map[string]any{}).Optional(),
```

Run Ent generation according to repo practice. If no script exists, use the repository's existing Ent generation command from docs or `go generate ./internal/repository/ent`.

Update store:

- `internal/repository/entstore/imagetask_store.go:createImageTask`
- `internal/repository/entstore/imagetask_store.go:updateImageTask`
- `internal/repository/entstore/imagetask_store.go:mapImageTaskEntity`

Pseudo helpers:

```go
func progressNodesJSON(nodes []domainimagetask.ProgressNode) ([]map[string]any, error) {
  if len(nodes) == 0 { return []map[string]any{}, nil }
  value, err := jsonRoundTrip(nodes)
  if err != nil { return nil, err }
  rows, _ := value.([]map[string]any)
  return rows, nil
}

func decodeProgressNodes(value any) ([]domainimagetask.ProgressNode, error) {
  var nodes []domainimagetask.ProgressNode
  if value == nil { return nodes, nil }
  return nodes, decodeJSONValue(value, &nodes)
}
```

### 8.4 Progress node state machine

Later backend node order:

```go
var defaultProgressNodes = []ProgressNode{
  {Key: "prompt_prepare", Status: "pending", Progress: 8},
  {Key: "prompt_optimize", Status: "pending", Progress: 18},
  {Key: "estimate", Status: "pending", Progress: 28},
  {Key: "queue", Status: "pending", Progress: 38},
  {Key: "route", Status: "pending", Progress: 52},
  {Key: "provider_call", Status: "pending", Progress: 72},
  {Key: "storage", Status: "pending", Progress: 88},
  {Key: "billing", Status: "pending", Progress: 96},
  {Key: "complete", Status: "pending", Progress: 100},
}
```

State transition rule:

```go
func upsertProgressNode(nodes []ProgressNode, next ProgressNode) []ProgressNode {
  for i := range nodes {
    if nodes[i].Key == next.Key {
      nodes[i] = mergeProgressNode(nodes[i], next)
      return nodes
    }
  }
  return append(nodes, next)
}
```

Failure rule:

```go
func failCurrentNode(task *Task, err error) {
  key := task.CurrentProgressNode
  if key == "" { key = "provider_call" }
  task.ProgressNodes = upsertProgressNode(task.ProgressNodes, ProgressNode{
    Key: key,
    Status: "failed",
    ErrorCode: errorCode(err),
    ErrorMessage: errorMessage(err),
    FinishedAt: nowPtr(),
  })
}
```

## 9. Implementation Tasks

### Task 1: Add Studio Tokens and Base Classes

**Files:**

- Modify: `web/shared/user-theme.css`
- Modify: `web/user/src/styles.css`
- Modify: `web/user/src/ui/classes.ts`
- Test: `web/user/src/ui/studio.contract.ts`

**Step 1: Write contract test**

Create `web/user/src/ui/studio.contract.ts`:

```ts
import { studio } from './classes'

function assert(value: unknown, message: string) {
  if (!value) throw new Error(message)
}

assert(studio.surface.glass.includes('backdrop-blur'), 'glass surface must use blur')
assert(studio.button.primary.includes('var(--accent)'), 'primary button must use theme accent')
assert(studio.form.textarea.includes('var(--input)'), 'textarea must use shared input token')
assert(studio.image.card.includes('group'), 'image card must support hover children')
```

**Step 2: Run contract**

Run:

```bash
npm --prefix web/user run typecheck
```

Expected: fail until `studio` is exported.

**Step 3: Implement tokens/classes**

Apply token changes from section 4 and class additions from section 5.2.

**Step 4: Verify**

Run:

```bash
npm --prefix web/user run typecheck
```

Expected: pass.

### Task 2: Add Studio Component Primitives

**Files:**

- Create: `web/user/src/ui/studio.tsx`
- Modify: `web/user/src/components.tsx` only where shared components should be re-exported or replaced.
- Test: `web/user/src/ui/studio.contract.ts`

**Step 1: Extend contract**

Add imports:

```ts
import { StudioButton, StudioPanel, SegmentedControl, AspectRatioTile } from './studio'

assert(typeof StudioButton === 'function', 'StudioButton must exist')
assert(typeof StudioPanel === 'function', 'StudioPanel must exist')
assert(typeof SegmentedControl === 'function', 'SegmentedControl must exist')
assert(typeof AspectRatioTile === 'function', 'AspectRatioTile must exist')
```

**Step 2: Implement components**

Use pseudocode in section 5.3.

**Step 3: Verify**

```bash
npm --prefix web/user run typecheck
```

### Task 3: Add Generation Progress Resolver

**Files:**

- Create: `web/user/src/ui/studioProgress.ts`
- Create: `web/user/src/ui/studioProgress.contract.ts`
- Modify: `web/user/src/ui/studio.tsx`

**Step 1: Write contract tests**

```ts
import type { ImageTask } from '../../../shared/api-types'
import { generationProgressView } from './studioProgress'

const queued = generationProgressView(task({ status: 'queued' }))
if (queued.currentKey !== 'queue' || queued.percent < 30) {
  throw new Error(`queued task should show queue phase, got ${queued.currentKey}/${queued.percent}`)
}

const running = generationProgressView(task({ status: 'running', attempts: [{ provider: 'openai' }] }))
if (running.currentKey !== 'provider_call') {
  throw new Error(`running task with attempts should show provider_call, got ${running.currentKey}`)
}

const done = generationProgressView(task({ status: 'succeeded', results: [{ id: 'img1', url: '/x.png' }] }))
if (done.percent !== 100 || done.tone !== 'success') {
  throw new Error('succeeded task must show complete success')
}

const failed = generationProgressView(task({ status: 'failed', error_message: 'upstream failed' }))
if (failed.tone !== 'danger' || failed.currentKey !== 'failed') {
  throw new Error('failed task must show danger failed phase')
}

function task(patch: Partial<ImageTask>): ImageTask {
  return {
    id: 'task-1',
    title: 'task',
    prompt: 'prompt',
    task_type: 'text_to_image',
    status: 'queued',
    route_model_code: 'plus',
    model_group: 'plus',
    quality: 'auto',
    aspect_ratio: '1:1',
    image_count: 1,
    estimate_points: '1.00000',
    created_at: '2026-06-09T00:00:00Z',
    updated_at: '2026-06-09T00:00:00Z',
    results: [],
    ...patch,
  }
}
```

**Step 2: Implement resolver**

Use section 6.3.

**Step 3: Wire `GenerationProgressPanel`**

Add component to `studio.tsx`.

**Step 4: Verify**

```bash
npm --prefix web/user run typecheck
```

### Task 4: Redesign Shell

**Files:**

- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/ui/classes.ts`

**Step 1: Add route metadata**

Add `routeMeta` near `navItems`.

**Step 2: Replace topbar layout**

Use section 7.1 pseudocode.

**Step 3: Keep behavior**

Must preserve:

- `app.navigate(...)`
- avatar menu open/close
- logout behavior
- balance chip
- profile menu

**Step 4: Verify**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

### Task 5: Redesign WorkspacePage

**Files:**

- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/pages/workspaceTaskFailure.ts`
- Modify: `web/user/src/pages/workspaceImageActions.ts`
- Modify: `web/user/src/pages/workspaceGenerateReadiness.ts` only if display text needs alignment.

**Step 1: Extract adapters**

Add pure functions near `workspaceClasses` or move later:

```ts
function studioModelFromCapability(model: CapabilityModelGroup): StudioModelCardModel
function activeGenerationTask(records: ImageTask[]): ImageTask | null
function taskImageActions(task: ImageTask, image: ImageResult): StudioImageCardAction[]
```

**Step 2: Replace local class object with Studio classes**

Keep page-specific classes minimal:

```ts
const workspaceClasses = {
  root: cn(studio.layout.content, 'grid gap-5'),
  split: studio.layout.split,
  controlPanel: 'sticky top-[calc(var(--topbar-h)+20px)] self-start max-[980px]:static',
  canvas: 'grid min-w-0 gap-5',
}
```

**Step 3: Rebuild JSX structure**

Follow section 7.2.

**Step 4: Preserve API behavior**

Do not change:

- `uploadReference`
- `createTask`
- SSE setup
- `applyAsEditSource`
- publish/download/copy behavior

**Step 5: Verify**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

### Task 6: Redesign HomePage

**Files:**

- Modify: `web/user/src/pages/HomePage.tsx`
- Modify: `web/user/src/pages/homeGalleryModel.ts` only for display labels if needed.

**Step 1: Replace readiness and carousel with Hero + Status panel**

Use section 7.3.

**Step 2: Use `StudioImageCard` for masonry**

Do not duplicate image card styles.

**Step 3: Verify**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

### Task 7: Redesign Private and Public Gallery

**Files:**

- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Modify: `web/user/src/pages/galleryRows.ts` only if card metadata needs stable shape.
- Modify: `web/user/src/pages/publicGalleryModel.ts` only if card metadata needs stable shape.

**Step 1: Replace page shells with `PageHero` + `StudioPanel` filters**

**Step 2: Replace cards with `StudioImageCard`**

**Step 3: Preserve modals and actions**

Must preserve:

- private preview
- continue edit
- publish
- group edit
- delete
- batch selection
- public like/favorite/download
- public detail login gate

**Step 4: Verify**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

### Task 8: Redesign Landing/Login and Secondary Pages

**Files:**

- Modify: `web/user/src/pages/LandingPage.tsx`
- Modify: `web/user/src/pages/LoginPage.tsx`
- Modify: `web/user/src/pages/CheckoutPage.tsx`
- Modify: `web/user/src/pages/ApiKeysPage.tsx`
- Modify: `web/user/src/pages/ProfilePage.tsx`
- Modify: `web/user/src/pages/DocsPage.tsx`

**Step 1: Migrate pages to Studio wrappers**

**Step 2: Replace duplicate empty/loading/error blocks**

Use `StudioEmptyState`, `StudioPanel`, `StudioButton`.

**Step 3: Verify**

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

### Task 9: Browser QA

**Files:** no edits unless visual bugs are found.

**Step 1: Start dev server**

```bash
npm --prefix web/user run dev
```

**Step 2: Open browser**

Use Browser plugin or Playwright:

- `http://localhost:5173/#/landing`
- `http://localhost:5173/#/login`
- `http://localhost:5173/#/home`
- `http://localhost:5173/#/genpic`
- `http://localhost:5173/#/gallery`
- `http://localhost:5173/#/public-gallery`
- `http://localhost:5173/#/checkout`
- `http://localhost:5173/#/api-keys`
- `http://localhost:5173/#/profile`
- `http://localhost:5173/#/docs`

**Step 3: Check viewports**

- Desktop: `1440 x 1000`
- Wide: `2048 x 1100`
- Tablet: `900 x 1100`
- Mobile: `390 x 844`

**Acceptance:**

- No text overlap.
- Sidebar/topbar usable on mobile.
- Workspace control panel does not squeeze canvas below usable size.
- Buttons have hover/active/focus states.
- Empty states look intentional.
- Generated image cards keep stable aspect and action bars do not shift layout.

### Task 10: Final Verification

Run:

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify.sh
```

If only user frontend changed and full verify is expensive, at minimum run:

```bash
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

Before marking complete, run review gate per repository workflow if preparing push/PR:

```bash
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
```

## 10. Acceptance Criteria

### 10.1 Visual acceptance

- 客户端所有页面都属于同一套 Creative Studio 主题。
- 不再出现页面各自散落的 card/button/filter 样式。
- 首页、工作台、公开广场第一屏具有明确高级感和产品可信度。
- 生成工作台接近参考图的“左控制/右画布”专业结构。
- 图库卡片以图片为主，hover 操作高级但不过度花哨。
- 移动端没有文字挤压、按钮溢出、导航不可用问题。

### 10.2 Code acceptance

- 新增共享组件后，页面使用 `StudioButton`、`StudioPanel`、`StudioImageCard`、`StudioEmptyState`、`GenerationProgressPanel`。
- `WorkspacePage` 不再内联大量重复控件样式。
- `GalleryPage` 和 `PublicGalleryPage` 不再各自实现不同图片卡样式。
- `studioProgress.ts` 是纯函数，有 contract tests。
- 不新增未经确认的 npm dependency。

### 10.3 Backend readiness acceptance

- 前端首版可在当前后端接口下工作。
- 方案明确后续后端需要改：
  - `internal/domain/imagetask/types.go`
  - `internal/repository/ent/schema/imagetask.go`
  - `internal/repository/entstore/imagetask_store.go`
  - `internal/service/imagetask/service.go`
  - `internal/http/handlers/api.go`
  - `internal/http/router/router.go`
  - `web/shared/api-types.ts`
  - `web/shared/user-api.ts`
- 提示词优化节点可通过独立 endpoint 或 create task 内联模式接入。
- SSE 可逐步从 `task` 快照升级到 `task_phase` 细粒度事件，不破坏现有客户端。

## 11. Risks and Guardrails

- Risk: 视觉过度依赖渐变，变成普通 AI 模板。
  - Guardrail: 主色只做少量 active/glow，surface 以深色、边线、图片内容为主。
- Risk: 抽象组件过大，页面难以做特殊布局。
  - Guardrail: 抽象基础控件和卡片，不抽象业务流程。
- Risk: 生成进度前端推断与真实后端阶段不一致。
  - Guardrail: 首版文案使用“正在准备/处理中”等不误导表达；后续以后端 `progress_nodes` 覆盖。
- Risk: 后端扩展引入任务频繁写库。
  - Guardrail: 后端节点持久化只在关键节点变化时写；SSE 可发送内存事件但定期落库。
- Risk: 移动端工作台双栏不适配。
  - Guardrail: `studio.layout.split` 在 `max-[980px]` 下单列，控制面板不 sticky。

## 12. Execution Recommendation

推荐先执行前端视觉重构，不同步改后端：

1. Task 1-3 建立主题、组件和进度解析器。
2. Task 4-5 先打通 Shell + Workspace，这是视觉收益最大、也是风险最高的核心路径。
3. Task 6-8 扩展到其它页面。
4. Task 9-10 做浏览器和构建验证。
5. 后续单独开后端 progress/prompt optimization 任务，按第 8 节扩展接口。

