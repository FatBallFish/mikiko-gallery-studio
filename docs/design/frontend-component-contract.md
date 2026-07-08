# Pic Gallery Frontend Component Contract

## Status

- 文档类型:组件级实施契约
- 适用范围:`web/user`、`web/admin`
- 上游约束:`docs/design/frontend-design-spec.md` 第 15、17 节
- 生效原则:任何 Button/Modal/Field/State/Card 的实现必须以本契约为准

## 1. 设计 Token 层

唯一来源:`web/shared/tokens.css` + 各端 `styles.css` 的语义 token 映射。组件内禁止硬编码色值。

### 1.1 语义 Token

| Token | 用途 |
|---|---|
| `--bg` | 页面背景 |
| `--canvas` | 画布层 |
| `--surface` | 表面层 (半透明) |
| `--surface-solid` | 实色表面 |
| `--elevated` | 抬升层 |
| `--fg` | 主文本 |
| `--muted` | 次级文本 |
| `--dim` | 弱化文本 / placeholder |
| `--border` | 边界线 |
| `--accent` / `--accent-rgb` | 强调色 |
| `--accent-coral` / `--accent-purple` / `--accent-emerald` | 备用强调 |
| `--atmosphere-grid` / `--atmosphere-glow` | 背景氛围层 |

### 1.2 主题切换

- 显式:`html[data-theme-mode="dark|light"]` 覆盖
- 系统跟随:`@media (prefers-color-scheme: ...)` 在 `data-theme-mode` 未设置时生效
- Accent:`html[data-accent-theme="amber|violet|emerald|coral"]`

### 1.3 圆角档位 (强制统一)

| 档位 | 值 | 用途 |
|---|---|---|
| sm | 12px (`rounded-xl`) | 输入框、小按钮内部元素 |
| md | 16px (`rounded-2xl`) | 按钮、卡片、表单元素 |
| lg | 24px (`rounded-3xl`) | 大卡片、模态、状态容器 |
| xl | 32px (`rounded-[2rem]`) | hero 封面、billding 大封面 |
| full | 9999px (`rounded-full`) | pill、圆形按钮、头像 |

**禁止出现 `rounded-[2.5rem]` / `rounded-[3rem]` / `rounded-[10px]` / `rounded-[14px]` / `rounded-[18px]` / `rounded-[20px]`**。统一用上述档位。

### 1.4 动效曲线

| Token | 曲线 | 用途 |
|---|---|---|
| `--pg-ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | 标准出场 |
| `--pg-ease-in-out` | `cubic-bezier(0.65, 0, 0.35, 1)` | 双向过渡 |
| `--pg-ease-spring` | `cubic-bezier(0.32, 0.72, 0, 1)` | 进场弹性 |
| `--pg-duration-instant` | 80ms | 即时反馈 |
| `--pg-duration-fast` | 140ms | hover 微交互 |
| `--pg-duration-base` | 220ms | 状态切换 |
| `--pg-duration-slow` | 480ms | 入场动画 |

`prefers-reduced-motion: reduce` 时所有动画降级为即时。`pg-enter-*` 工具类已内置降级。

## 2. Button

### 2.1 Variant 规格

| Variant | 圆角 | 高度 | padding | 字号 | font-weight |
|---|---|---|---|---|---|
| base (默认) | `rounded-xl` (12px) | 40px | `px-[18px] py-2.5` | `text-sm` | `font-bold` |
| primary | 同上 | 同上 | 同上 | 同上 | 同上 |
| ghost | 同上 | 同上 | 同上 | 同上 | 同上 |
| danger | 同上 | 同上 | 同上 | 同上 | 同上 |
| icon | `rounded-xl` | 40px | 0 (正方形) | - | - |
| iconSm | `rounded-lg` (8px) | 32px | 0 | - | - |

### 2.2 状态

- **默认**:见 variant
- **hover**:`-translate-y-px`,accent 染色边框与背景
- **active**:`scale-[0.98]`,translate 归零
- **disabled**:`opacity-50`, `pointer-events-none`
- **busy**:前置 `spinner`,按钮 `disabled`

### 2.3 颜色

- base:`bg-[var(--surface)]` + `text-[var(--fg)]` + `border-[var(--border)]`
- primary:`bg-[var(--accent)]` + `text-[var(--bg)]` + `border-[var(--accent)]` + hover accent 阴影
- ghost:透明背景 + accent 染色 hover
- danger:`text-[oklch(74%_.16_35)]` + coral 染色 hover

### 2.4 禁止

- 一个页面同时存在两个意图相同的 CTA (如 "联系" + "Contact us")
- CTA 文案在桌面端折行 (3 词以内,主 CTA 1-2 词)
- 按钮文字与背景对比度不足 WCAG AA (4.5:1)

## 3. Modal

### 3.1 结构

```
backdrop (fixed inset-0 z-[80] grid place-items-center bg-black/60 backdrop-blur-md p-6)
  └ card (max-h-[90vh] w-[min(920px,100%)] overflow-auto rounded-3xl border bg-[var(--surface)] p-6)
      ├ header (flex items-center justify-between gap-5)
      │   ├ h2 (text-xl)
      │   └ close button (size-38px rounded-full)
      └ children
```

### 3.2 进出场

- 进场:`animate-in fade-in duration-200` (backdrop) + `zoom-in-95 duration-200` (card)
- 出场:当前不实现退出动画 (单向,spec 第 14 节克制原则)
- 曲线:进场 `--pg-ease-spring`,内部状态切换 `--pg-ease-out`

### 3.3 尺寸档位

| 档位 | max-width |
|---|---|
| sm | 384px |
| md | 480px |
| lg | 640px |
| xl | 920px (默认) |

### 3.4 禁止

- 模态内再套模态
- 模态标题与按钮文字重复
- backdrop 不可点击关闭 (除非是确认对话框)

## 4. Field (表单)

### 4.1 结构

```
label (grid gap-2)
  ├ <span class="fieldLabel">label</span>
  ├ input / textarea / select
  ├ <em>hint</em> (可选)
  └ error text (可选, 在 input 下方)
```

### 4.2 规格

- input/textarea:`rounded-xl` + `bg-[var(--surface)]` + `border-[var(--border)]` + `px-3.5 py-2.5` + `text-sm`
- focus:`border-[var(--accent)]` + `ring-2 ring-[color-mix(in_oklch,var(--accent)_22%,transparent)]`
- placeholder:`text-[var(--dim)]`,**不得代替 label**
- label 在 input **上方**,helper/error 在 input **下方**

### 4.3 禁止

- placeholder 充当 label
- label 在 input 左侧 (移动端除外)
- error 文本与 input 之间无间距

## 5. EmptyState / LoadingState

### 5.1 EmptyState

```
container (rounded-3xl border bg-[var(--surface)] p-12 text-center grid place-items-center gap-3)
  ├ iconWrap (size-14 rounded-2xl bg-[accent/8%] text-[accent] grid place-items-center) — 可选
  ├ title (text-base font-bold text-[var(--fg)])
  ├ detail (text-sm text-[var(--muted)] max-w-[42ch])
  └ action (可选 Button)
```

**禁止裸 dashed border**。空状态要有视觉骨架,不能只是文字 + 虚线框。

### 5.2 LoadingState

- 简单场景:`state.stateLine` + spinner + 文字
- 内容骨架:用 `.pg-skeleton` 占位 (背景 + shimmer 动画)
- 表格骨架:`pg-skeleton` 行,高度匹配实际行高
- **禁止**所有页面都用同一个 "正在读取实时数据..." 文案

### 5.3 Skeleton 用法

```tsx
<div className="pg-skeleton h-4 w-24 rounded-md" />  // 一行文字
<div className="pg-skeleton aspect-[4/3] w-full rounded-xl" />  // 一张图
```

## 6. Toast

### 6.1 容器

`fixed right-5 top-5 z-[100] grid w-[min(380px,calc(100vw-40px))] gap-3`

### 6.2 单条结构

```
toast (grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-2xl border bg-[surface/92%] p-3.5 shadow-lg backdrop-blur-xl)
  ├ icon (size-7 rounded-full bg-[ring/18%] text-[ring])
  └ message
```

### 6.3 Tone 颜色

| Tone | ring color |
|---|---|
| success | `var(--accent-emerald)` |
| error | `oklch(72% .18 32)` |
| info (默认) | `var(--accent)` |

## 7. StatusPill

### 7.1 Tone 映射

| Tone | 语义 | 用例 |
|---|---|---|
| good | 成功 / 已生效 | succeeded, public, active |
| warn | 待确认 / 处理中 | running, queued, reviewing |
| bad | 失败 / 拒绝 | failed, rejected, disabled |
| neutral | 默认 / 未选择 | private, cancelled |

### 7.2 规格

`rounded-full px-2.5 py-1 font-vault-mono text-[11px] font-bold`

## 8. Card

| 层级 | 圆角 | padding | 用途 |
|---|---|---|---|
| base | `rounded-2xl` (16px) | 0 | 内容容器,内部自管 padding |
| padded | `rounded-3xl` (24px) | `p-6` | 标准卡片 |
| hero | `rounded-[2rem]` (32px) | 自定 | 页面 hero / 大封面 |

**禁止卡片套卡片**。如果需要嵌套层级,用 `border-t` / `divide-y` / 负边距。

## 9. 图标

- 库:`lucide-react`
- 全局 `strokeWidth: 1.5`
- 导入:从 `web/user/src/ui/icons.ts` re-export,禁止页面直接 `import { ... } from 'lucide-react'`
- `brand.tsx` 的 BrandMark 是品牌资产,不用图标库
- 禁止内联 `<svg>` 画图标 (除非是品牌 mark)

## 10. 动效词汇表

| 工具类 | 用途 | 时长 |
|---|---|---|
| `.pg-enter` | 区块 fade-up 入场 | 480ms |
| `.pg-enter-fade` | 简单 fade 入场 | 360ms |
| `.pg-enter-scale` | 模态/弹层 scale 入场 | 320ms |
| `.pg-shimmer` | 加载骨架流光 | 1.6s 循环 |
| `.pg-skeleton` | 占位骨架 (含内置 shimmer) | 1.8s 循环 |

`useReveal()` hook 提供 `IntersectionObserver` 触发,`revealStyle(visible, delayMs)` 提供错峰入场样式。

**场景**:
- 首屏区块 → `.pg-enter`
- 列表项错峰 → `useReveal` + `revealStyle(visible, i * 60)`
- 模态/弹层 → `.pg-enter-scale` (与 animate-in 配合)
- 加载占位 → `.pg-skeleton`
- 生成中按钮 → `.pg-shimmer`

## 11. 验收清单

### 11.1 圆角统一
- [ ] 全页 `rounded-[` 出现的值仅有 12/16/24/32 四档 + `rounded-full`
- [ ] 无 `rounded-[2.5rem]` / `rounded-[3rem]` / `rounded-[10px]` 等

### 11.2 色彩一致
- [ ] 一个页面只有一个 accent
- [ ] 无硬编码 `#xxxxxx` 或 `rgb(...)` 在组件 class 里 (品牌 mark 除外)
- [ ] light / dark 两种模式对比度均通过 WCAG AA

### 11.3 动效克制
- [ ] 所有动画在 `prefers-reduced-motion: reduce` 下即时降级
- [ ] 无高频闪烁、无超过 1s 的循环装饰动画 (shimmer/skeleton 除外)
- [ ] 每个动画都能一句话说明其反馈意图

### 11.4 组件一致
- [ ] Button 全站统一三 variant + icon/iconSm
- [ ] Modal 圆角 `rounded-3xl`,backdrop blur
- [ ] Field label 在上,error 在下,placeholder 不当 label
- [ ] EmptyState 有视觉骨架,不裸 dashed border
- [ ] StatusPill 用统一组件,不在页面内手写 badge

### 11.5 主题一致
- [ ] light mode 下 `body::before` 网格与光晕反转
- [ ] `data-theme-mode` 缺失时跟随 `prefers-color-scheme`
- [ ] accent 切换全站生效