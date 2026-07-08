# Pic Gallery 前端重设计:设计系统底座 + 用户核心页面改造

## Context

`pic-gallery` 是一个自托管 AI 图像生成平台,前端为 `web/user` 与 `web/admin` 两个 Vite + React 19 + Tailwind 4 应用,共享 `web/shared` 下的 token 与基础 CSS。当前用户端页面"看起来不够专业"的根本原因不是色彩或字体不对,而是**系统没有真正落地**:

- `web/user/src/ui/classes.ts` (旧生产语言) 与 `redesign-classes.ts` (新设计语言) **并存**,`Shell` 已迁到新系但 `Button/Modal/Field/EmptyState/LoadingState/Surface/PageIntro/StatusPill/Toast` 仍用旧 token,造成同一页内两套圆角、两套高度、两套动效词汇
- `ProfilePage` / `SettingsPage` 整页没碰过新系,与 Landing/Workspace 视觉断裂
- Light mode 在 `web/user/src/styles.css` 里用 `html[data-theme-mode]` 属性选择器做了 ~25 个组件 token 的逐项反转,新增 token 必须在三处同步维护,且 `body::before` 网格与光晕在 light 下未反转
- 图标全是内联 SVG,无统一描边/视觉语言;`PublicDetailIcon` 这类文件让图标散落各处
- 没有动效契约,只有零星 `animate-in fade-in` 与 `active:scale-95`,hover/状态切换/列表入场都缺
- `docs/design/frontend-design-spec.md` 与 `frontend-visual-directions.md` 给出了**方向**,但缺少组件级实施契约(具体到 Button 三种 variant 的规格、Modal 进出场曲线、EmptyState 的视觉骨架),因此每次实现都会"跑偏"

仓库已存在的 `AGENTS.md` 工作流要求:非 trivial 任务前需 `dev-start-coding` 建立 `.coding-context.json`,React 改动遵循 `dev-react-patterns`,交付前走 `dev-verify` + `dev-review-gate` + `dev-ship`。本计划遵循该工作流。

用户选择:**先做设计系统底座并做好文档规范契约,再尽可能完成用户核心页面改造**。本计划分两个阶段对应。

## 设计读

Reading this as: 用户端"Luminous Vault"高奢数字艺廊创作台,对创意消费者,深色 OKLCH 调性 + Cormorant/Manrope 字体,redesign-preserve 模式 (保留品牌,精修呈现)。

Dials: `DESIGN_VARIANCE 7 / MOTION_INTENSITY 5 / VISUAL_DENSITY 4` (landing 偏 7/6/3, workspace 偏 6/4/5)。Motion 不超过 6,因 spec 第 14 节明确"动效用于反馈与节奏,不用于炫技"。

## 阶段 A:设计系统底座 + 文档契约

### A1. 合并 class 系统

**目标**:消除 `classes.ts` 与 `redesign-classes.ts` 并存,只剩一套"系统"层。

**改动**:
- `web/user/src/ui/classes.ts`:**删除** `userShell`/`userButton`/`userForm`/`userState`/`userPill`/`userCard`/`userText` 旧导出
- `web/user/src/ui/redesign-classes.ts`:重命名导出为 `shell`/`button`/`form`/`state`/`pill`/`card`/`text`,补齐缺失的语义(`button` 三 variant:primary/ghost/danger;`form` field/input/textarea;`state` spinner/empty/toast/modalBackdrop/modalCard;`pill` 6 个 tone;`card` base/padded;`text` eyebrow/mono)
- `web/user/src/components.tsx`:重写 `Button`/`Field`/`Surface`/`PageIntro`/`LoadingState`/`ErrorState`/`EmptyState`/`Modal`/`StatusPill`/`ToastViewport`/`ToastItem` 全部改用新 `redesign-classes` 的 token,圆角统一 `rounded-2xl` (16px) 起步,卡片 `rounded-3xl` (24px),大封面 `rounded-[2rem]` (32px) — 不再出现 `rounded-[2.5rem]`/`rounded-[3rem]` 混用
- 删除 `web/user/src/components.tsx` 中 `PublicDetailIcon` 内联 SVG,改用图标库(见 A3)
- 受影响引用方:`LandingPage`/`HomePage`/`WorkspacePage`/`GalleryPage`/`CheckoutPage`/`ProfilePage`/`SettingsPage`/`ApiKeysPage`/`DocsPage`/`PublicGalleryPage` 全部改导入路径

### A2. 修复 Light Mode 为 token-driven

**目标**:消除 `web/user/src/styles.css` 的属性选择器逐项反转,改用"语义 token 在两套主题块里完整映射"。

**改动**:
- `web/user/src/styles.css`:
  - `:root` 与 `html[data-theme-mode="dark"]` 块只声明**原始调色板** (`--pg-user-dark-bg` 等),保留现有
  - 新增**单一语义层** `--bg/--canvas/--surface/--elevated/--fg/--muted/--dim/--border/--accent/--accent-rgb`,在 dark 块映射到 dark 原始色,在 light 块映射到 light 原始色
  - 删除 `--image-overlay`/`--image-overlay-selected`/`--image-card-text`/`--image-card-muted`/`--sidebar-bg`/`--toast-ring` 等 25+ 组件级 token — 这些是"组件自管"的,不该塞进全局主题层
  - `body::before` 网格与三色光晕按 `[data-theme-mode="light"]` 反转(白线变深灰线,暖光降饱和),不再共用一套
  - 加入 `@media (prefers-color-scheme: dark)` 与 `(prefers-color-scheme: light)` 媒体查询,在 `data-theme-mode` 未显式设置时跟随系统,修复"属性缺失时静默 dark+amber"
  - 加入 `@media (prefers-reduced-motion: reduce)` 全局降级
- `web/user/src/themePreferences.ts`:保留显式覆盖能力,但默认改为"system",由 CSS 媒体查询驱动
- 受影响组件:把所有 `bg-[#05070d]`/`text-white/80` 等硬编码改为 `bg-[var(--bg)]`/`text-[var(--fg)]` — 这部分随 A1 重写时一并完成

### A3. 引入图标库

**目标**:消除内联 SVG 散落,统一描边粗细。

**改动**:
- `web/user/package.json`:新增 `lucide-react` (项目无 npm 图标依赖,选 lucide 是因为 spec 已无强烈倾向,且 lucide 与 Cormorant/Manrope 静态字体的视觉粗细搭配最稳;`strokeWidth` 全局设 `1.5`)
- 新建 `web/user/src/ui/icons.ts`:re-export 用到的图标子集,统一 `size={18} strokeWidth={1.5}`
- 替换 `components.tsx` 与 `pages/*.tsx` 中的内联 SVG:`navItems` 的 home/grid/sparkles/settings、`PublicDetailIcon` 全套、各页内的 chevron/close/eye/copy/check 图标
- `brand.tsx` 保留自定义 mark(这是品牌资产,不是图标)

### A4. 动效契约

**目标**:补齐 spec 第 14 节的具体曲线与时长。

**改动**:
- `web/shared/tokens.css`:已有 `--pg-ease-out: cubic-bezier(0.16, 1, 0.3, 1)`,补 `--pg-ease-in-out: cubic-bezier(0.65, 0, 0.35, 1)`、`--pg-ease-spring: cubic-bezier(0.32, 0.72, 0, 1)` (与 redesign-demo 一致)、`--pg-duration-instant: 80ms`
- `web/user/src/styles.css`:新增 `@keyframes pg-fade-up`、`pg-fade-in`、`pg-scale-in`、`pg-shimmer`,以及 `.pg-enter` 工具类(可加 `data-enter="on-scroll"` 配合 IntersectionObserver)
- 不引入 `motion/react` 全栈库,理由:spec 限制动效轻量,`animate-in` + CSS keyframes + 一个轻量 IntersectionObserver hook (`web/user/src/ui/useReveal.ts`,~30 行) 即可覆盖需求,避免给 monorepo 加重量依赖。如未来出现真正需要 spring 物理的 hover/拖拽,再单独按 leaf 引入

### A5. 组件级文档契约

**目标**:把 spec 第 15 节"禁止项"和第 17 节"验收清单"补到可执行级别。

**改动**:新建 `docs/design/frontend-component-contract.md`,内容覆盖:
- Button:三 variant 的尺寸/圆角/字号/padding/active 态/hover 态/disabled 态精确规格
- Modal:进出场曲线(`pg-ease-spring` 进 `pg-ease-out` 出)、backdrop blur 度、圆角 `rounded-3xl`、最大宽度档位
- Field:label 在上、helper 在下、error 在下、focus ring 用 `--accent`、placeholder 不当 label
- EmptyState / LoadingState:视觉骨架(不再裸 dashed border),最小尺寸,图标位置
- Toast:堆叠位置、计数倒计时、tone 色 ring
- StatusPill:6 个 tone 的语义映射
- Card:三种层级(base / padded / 大封面)各自的圆角与 padding
- 动效词汇表:fade-up/fade-in/scale-in/shimmer 各自何时用
- Light/Dark 一致性:token 层唯一来源,禁止组件级硬编码色

## 阶段 B:用户核心页面改造

每页改完单独跑 `dev-verify`,所有页改完跑 `dev-ship`。页面间共享的 Shell 不重做(已用 `rdShell`),只修 Shell 内残留的小问题。

### B1. LandingPage (`web/user/src/pages/LandingPage.tsx`)
- 移除 `userButton` 导入,用统一 `button`
- hero 圆角统一 `rounded-[2rem]`(不再 3rem),stat 卡 `rounded-2xl`
- 删除"数千名创作者""放飞灵感"等模板化文案,换成功能性短句(≤ 20 字 subtext,遵守 design-taste hero stack discipline)
- 加入 `useReveal` 让 hero 下方三段以 fade-up 错峰入场(60ms stagger),`prefers-reduced-motion` 降级为即时显示
- 三列 feature grid 改为 2+1 不对称(2 个并列 + 1 个跨列),避免 3 等宽 AI tell
- 顶部 nav 单行,高度 ≤ 72px,移除版本标签式装饰

### B2. HomePage (`web/user/src/pages/HomePage.tsx`)
- 把 hero carousel 的英文 "Cinematic Product Visualization" 改中文,与全站一致
- 局部 `rounded-[20px]` 覆盖删掉,统一走 `rounded-2xl`
- masonry 入场用 `useReveal` stagger(图片错峰 40ms),无限滚动 sentinel 改为骨架 shimmer
- filter 按钮用统一 `button` ghost variant

### B3. WorkspacePage (`web/user/src/pages/WorkspacePage.tsx`)
- 删除 `!important` 覆盖链(`!border-[var(--accent)]`/`!bg-[var(--accent)]`/`!text-white`),改用 `button` primary variant 与 `selectItem` active 态
- 选中态用 token `--accent`,不再用 `!` 强制
- 输出区空状态用新 `EmptyState`(骨架化)
- 删除 `floatingFeedback` 死代码(line 166 `hidden`)
- `rounded-[2.5rem]` 输出面板改 `rounded-3xl` 与全系统一致
- 生成中用 shimmer 占位,而非仅 spinner

### B4. GalleryPage (`web/user/src/pages/GalleryPage.tsx`)
- card hover overlay 加显式 `transition duration-300 pg-ease-out`,不再 opacity-0 → 100 突变
- `userButton.icon` + `!gap-1 !rounded-md !p-0.5` 强制覆盖删掉,用 `button.icon` 小尺寸 variant
- publish 状态用 `StatusPill` 组件(而非 `rdGallery.itemBadge` 8px 大写)
- 删除/分组/分享 modal 用新 `Modal`

### B5. CheckoutPage (`web/user/src/pages/CheckoutPage.tsx`)
- recent orders 区从 `userCard.padded` 改为 `card.padded`
- 支付方式图标(`微`/`支`/`¥` 文字 glyph)改为 lucide `Wallet`/`CreditCard`/`QrCode`
- 空状态用新 `EmptyState`
- 支付 modal 按钮 `Button` primary variant,圆角一致

### B6. ProfilePage (`web/user/src/pages/ProfilePage.tsx`) — **重点**,整页未迁新系
- 整页重写视觉,但保留全部业务逻辑(API 调用、字段、ledger 数据流不动)
- 修 `font-[var(--font-display)]` 无效 Tailwind 语法,改 `font-[family-name:var(--font-display)]` 或 `style={{fontFamily:'var(--font-display)'}}`
- ledger tag 颜色从硬编码 `rgba(191,161,106,...)` 改 `var(--accent)`
- save 按钮用 `button.primary`,主操作要强调
- 加入 `useReveal` 让余额卡 + 编辑卡 fade-up 入场
- LoadingState 改为骨架(余额骨架 + ledger 行骨架)

### B7. SettingsPage (`web/user/src/pages/SettingsPage.tsx`)
- mode 按钮 + accent swatch 用统一 `button` ghost + active ring
- 加入 accent 选中后的 mini 预览(显示当前 accent 在 Button/Card 上的样子)
- 卡片用新 `card.padded` + `useReveal`

### B8. PublicGalleryPage / ApiKeysPage / DocsPage
- 同步统一 `button`/`card`/`Modal`,替换内联 SVG 为 lucide
- 这些页结构已可用,改动量小于 Profile/Settings,作为收尾

## 关键文件清单

| 文件 | 阶段 | 动作 |
|---|---|---|
| `web/user/src/ui/classes.ts` | A1 | 删除旧导出 |
| `web/user/src/ui/redesign-classes.ts` | A1 | 扩展为完整系统,重命名导出 |
| `web/user/src/components.tsx` | A1/A3 | 重写 Button/Field/Modal/State/Pill/Toast,替换内联 SVG |
| `web/user/src/styles.css` | A2 | 主题层重构,light mode token-driven |
| `web/user/src/themePreferences.ts` | A2 | 默认 system,保留显式覆盖 |
| `web/shared/tokens.css` | A4 | 补 ease/duration token |
| `web/user/src/ui/icons.ts` | A3 | 新建,lucide re-export |
| `web/user/src/ui/useReveal.ts` | A4 | 新建,IntersectionObserver hook |
| `web/user/package.json` | A3 | 加 `lucide-react` |
| `docs/design/frontend-component-contract.md` | A5 | 新建,组件契约 |
| `web/user/src/pages/LandingPage.tsx` | B1 | 改 |
| `web/user/src/pages/HomePage.tsx` | B2 | 改 |
| `web/user/src/pages/WorkspacePage.tsx` | B3 | 改 |
| `web/user/src/pages/GalleryPage.tsx` | B4 | 改 |
| `web/user/src/pages/CheckoutPage.tsx` | B5 | 改 |
| `web/user/src/pages/ProfilePage.tsx` | B6 | 重写视觉,逻辑保留 |
| `web/user/src/pages/SettingsPage.tsx` | B7 | 改 |
| `web/user/src/pages/PublicGalleryPage.tsx` | B8 | 改 |
| `web/user/src/pages/ApiKeysPage.tsx` | B8 | 改 |
| `web/user/src/pages/DocsPage.tsx` | B8 | 改 |

`web/admin` 本次**不动**,留待后续单独一阶段(用户已选择用户端优先)。

## 执行顺序

1. 先按 `AGENTS.md` 调 `dev-start-coding` 建立 `.coding-context.json`(requirement source = 本计划,design source = `frontend-design-spec.md` + `frontend-visual-directions.md` + 新建 component-contract)
2. 阶段 A1 → A2 → A3 → A4 → A5(底座先稳,文档与代码并行)
3. 跑 `dev-verify` 确认 typecheck + build 通过(此时页面会因 import 路径变化有 broken 引用,A1 完成时一并修)
4. 阶段 B1 → B2 → ... → B8,每页改完跑一次 `dev-verify`
5. 全部完成后跑 `dev-review-gate` + `dev-ship`
6. `dev-api-smoke` 在本任务不跑(后端 API 未改)

## 验证

- `./scripts/workflow/verify.sh` — Go test/vet + user/admin typecheck + build 全绿
- 手动 `make user-web-dev` 后浏览器开 `http://localhost:5173`,逐页核对:dark / light 两种模式、移动端 375px 宽、prefers-reduced-motion 开启
- 核对清单对齐 `docs/design/frontend-design-spec.md` 第 17 节 + 新建 component-contract 的逐条规格
- 重点核对:圆角是否统一(扫一遍所有 `rounded-[` 出现的值,应只有 12/16/24/32 四档)、light mode 下 `body::before` 是否还有白线、所有 `!important` 是否清零、Profile/Settings 是否与其他页视觉一致

## 不做的事

- 不改 `web/admin`(留待后续阶段)
- 不引入 `motion/react` / `gsap` / `three.js`(spec 限制 + 重量依赖,本任务不需要)
- 不改后端 Go 代码、API、路由
- 不改路由结构(保留 hash router)
- 不改 `.coding-context.json` 已记录的业务字段名(analytics 不受影响)
- 不改 brand mark(`brand.tsx` 的 logo)
- 不改 `redesign-demo` 应用本身(它是参考,不是交付目标)