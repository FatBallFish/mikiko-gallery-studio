# Prompt: 管理端页面进一步打磨

> 把这段完整贴给负责管理端的 AI agent(Claude Code / Codex / 其他)。

---

你是 pic-gallery 项目管理端(`web/admin`)的视觉打磨工程师。前一轮重设计聚焦用户端,管理端基本没动;现在轮到管理端做对应精修。你的目标是让管理端从"功能能用"升级到"专业、克制、可扫读"。

## 项目位置

仓库根目录:`/Users/fatballfish/Documents/Projects/GoProjects/Personal/pic-gallery`
你负责的代码范围:`web/admin/`

## 必读上下文(开工前完整读完,不要跳读)

1. `docs/plans/2026-07-03-frontend-redesign-baseline.md` — 上一轮用户端重设计的完整计划(含设计系统底座说明,管理端应遵循同一套 token 与契约精神)
2. `docs/design/frontend-design-spec.md` — 母设计规范,**特别看第 4.2 节"管理端主题:Soft Grid Ops"**、第 8.2 节管理端布局、第 9.3 节 TopBar、第 15 节禁止项
3. `docs/design/frontend-visual-directions.md` — 视觉方向,**特别看第 3 节"Soft Grid Ops"**
4. `docs/design/frontend-component-contract.md` — 组件级实施契约(Button/Modal/Field/EmptyState/Toast/StatusPill/Card/动效词汇表)。这份契约原本针对用户端,但**圆角四档、动效曲线、EmptyState 骨架、Light/Dark 一致性这些通用规则同样适用于管理端**。
5. `web/admin/src/styles.css`、`web/admin/src/ui/classes.ts` — 当前 admin 样式与 class 系统
6. `web/shared/tokens.css`、`web/shared/admin-theme.css` — 共享 token 与管理端主题变量
7. `AGENTS.md` — 仓库 AI 工作流(必须遵循)

## 管理端的设计气质(与用户端不同,不要照搬)

用户端是"Luminous Vault":高奢、深色、艺术、玻璃光晕。
管理端是"Soft Grid Ops":**极简、克制、可扫读、可运维**。

具体差异:
- **不要**把用户端的深色玻璃面板、accent 暖光晕、Cormorant Garamond 大标题搬到管理端。管理端标题用 `Fraunces`(轻度区分),正文用 `Manrope`,整体克制。
- 管理端主信息必须落在**主表区、行区、时间线区、审核区**这些高扫描效率容器,不要堆大卡片。
- 右侧只允许**窄反馈区、队列区、状态摘要区**,不膨胀成第二主区。
- 状态色按语义用:成功绿、告警琥珀、错误红、中性灰,不要装饰性彩色。
- 动效比用户端更克制:状态切换、hover 高亮、加载骨架即可,不要 hero 入场、不要 scroll-pinned、不要 magnetic hover。

## 关键约束:与 user agent 并行工作,严禁越界

**本次任务与另一个负责 `web/user` 的 AI agent 在同一分支并行进行。你必须严格遵守以下隔离协议,否则会产生冲突:**

### 你可以读、可以写的目录与文件

- ✅ `web/admin/**`(全部)
- ✅ `docs/plans/2026-07-03-frontend-redesign-baseline.md`(只读,参考)
- ✅ `docs/design/frontend-component-contract.md`(只读,契约)
- ✅ `docs/design/frontend-design-spec.md`(只读)
- ✅ `docs/design/frontend-visual-directions.md`(只读)
- ✅ `docs/plans/admin-polish-notes.md`(你自己的笔记文件,新建)
- ✅ `.review/admin-gate.json`(你自己的验收产物)

### 你绝对不能写的目录与文件(归 user agent 或共享层)

- ❌ `web/user/**` — user agent 的领地,任何修改都视为越界
- ❌ `web/shared/**` — 共享 token 层,如果你觉得需要改 token,在 `docs/plans/admin-polish-notes.md` 里写"提议"段落,不要直接改文件
- ❌ `docs/design/frontend-design-spec.md`、`frontend-visual-directions.md`、`frontend-component-contract.md` — 契约文档,只读
- ❌ `docs/plans/user-polish-notes.md` — user agent 的笔记
- ❌ `.review/user-gate.json` — user agent 的验收产物
- ❌ `docs/plans/prompt-polish-user-web.md`、`docs/plans/prompt-polish-admin-web.md` — 这两份 prompt 文档
- ❌ `Makefile`、`scripts/**`、`.githooks/**`、`.hook-scripts/**` — CI/构建脚本
- ❌ `cmd/**`、`internal/**`、`pkg/**` — 后端 Go 代码
- ❌ `go.mod`、`go.sum`

### 共享文件的冲突避免

`web/shared/tokens.css`、`web/shared/base.css`、`web/shared/user-theme.css`、`web/shared/admin-theme.css` 是两端共用的。如果你确实需要调整某个共享 token,在 `docs/plans/admin-polish-notes.md` 里写:

```
## 共享层提议
- 文件: web/shared/tokens.css
- 改动: 新增 --pg-radius-xs: 8px
- 理由: admin 表格内联元素的圆角需要比 12px 更小
- 对 user 的影响: user 暂未使用 xs 档,无影响
```

我会人工评估后再决定是否合并。**任何对 `web/shared/**` 的直接编辑都会被 git diff 视为越界并要求回滚。**

## 你的任务范围

### 1. 建立管理端 class 系统(如果还没有)
检查 `web/admin/src/ui/classes.ts` 是否有类似用户端 `redesign-classes.ts` 的统一 class 系统。如果没有,参照用户端结构建立 `adminShell`/`button`/`form`/`state`/`pill`/`card`/`table`/`chart` 等原语,圆角沿用契约四档(12/16/24/32 + full),但管理端**以 12/16 为主**,24 仅用于大面板,32 与 `rounded-[2rem]` 不要用(管理端不做 hero 大封面)。

### 2. 表格与数据密度精修
- 管理端核心是表格。所有 `<table>` 用统一 `tableWrapper`/`table`/`th`/`td`/`tr` class:
  - `th`: `text-[11px] font-bold uppercase tracking-wider text-muted`,底边框
  - `td`: `text-sm`,行间用 `border-b border-[var(--border)]` 但**不要**每行都加粗边框,隔行或末行去掉
  - `tr:hover:bg-[var(--surface)]` 高亮
  - 表格容器 `rounded-2xl border overflow-hidden`,表头 sticky
- 长列表用分组 + 单分隔线,不要每行都 `border-t + border-b`(契约第 9.F 节)。

### 3. 状态反馈统一
- 用统一 `StatusChip`/`Badge` 组件替代各页手写的 status 标签,语义色严格映射:
  - success → 绿(`--pg-admin-success` 或 emerald)
  - warning → 琥珀
  - error → 红(独立 token,不要复用琥珀)
  - neutral → 灰
- 审核队列、订单状态、用户状态都用同一套 chip。

### 4. EmptyState / LoadingState 升级
- 管理端空状态比用户端更常见(无订单、无审核项、无用户)。每个空状态要有 icon + 简短说明 + (可选)主操作,不要裸文字。
- 数据加载用 `.pg-skeleton` 骨架(可复用用户端定义的 keyframe,或在 admin styles.css 里加同等定义)。
- 表格加载用行骨架,卡片网格加载用卡片骨架。

### 5. 侧栏与 TopBar 一致性
- 左侧栏宽度 216px(`--pg-sidebar-admin-width`),不要每页自己改。
- TopBar 高度 ≤ 80px,只放:管理员头像、告警计数、待处理计数、Provider 状态摘要、主题切换。
- TopBar **不放**页面标题、不放 slogan、不放长文案(spec 第 9 节硬约束)。
- 第二带状态条(环境/Provider/集群/待审数)可以放在 TopBar 下方,但必须是单行,不换行。

### 6. 图标统一
- 安装 `lucide-react` 到 `web/admin/package.json`(如果还没装)。
- 新建 `web/admin/src/ui/icons.ts`,统一 `strokeWidth: 1.5`。
- 替换 `web/admin/src/components.tsx` 与各页面里的内联 `<svg>` 图标。
- 管理端常用图标:Dashboard/Monitoring/Users/UserGroups/Receipt/Tags/ShieldCheck/Orders/Package/Cashier/Route/Plug/PriceList/ScrollText/SystemUsers/Settings 等。

### 7. 圆角与色彩巡检
- 扫描 `web/admin/src/` 全目录,圆角统一到契约四档。管理端**不要出现 `rounded-[2rem]` 或 `rounded-[3rem]`**。
- 扫描硬编码色值,改为 `var(--bg)`/`var(--surface)`/`var(--fg)`/`var(--border)`/`var(--accent)` 等 admin 主题 token。
- 管理端 accent 默认用 `oklch(65% 0.18 280)`(蓝紫),不要用用户端的 amber/gold。

### 8. Light/Dark 双模式
- 管理端默认可以是 light(Soft Grid Ops 气质偏浅灰),但要支持 dark。
- 用 `data-theme="dark|light"` 属性切换(沿用现有 `useAdminTheme.ts` 机制)。
- **不要**用属性选择器 `[class*='bg-[#xxx]']` 这种脆弱 patch,改用语义 token 在 dark/light 块里完整映射(参考用户端 `styles.css` 的 `--bg/--surface/--fg` 模式)。

### 9. 移动端巡检
- 管理端移动端不是重点,但至少在 `768px` 以下侧栏能转横向滚动带,表格能横向滚动不破坏布局。

## 工作流程(严格遵守 AGENTS.md)

1. **开工前**:运行 `./scripts/workflow/start-coding.sh --task "管理端页面视觉精修"` 建立 `.coding-context.json`。
   - **注意**:如果 user agent 已经跑过这个命令并写了 `.coding-context.json`,你需要在你的 notes 文件里记录"`.coding-context.json` 已被 user agent 占用,我在 admin-polish-notes.md 里维护自己的 task 描述",**不要覆盖** `.coding-context.json`。
2. **碰 React 前必读** `.claude/skills/dev-react-patterns/SKILL.md` 或 `.agents/skills/dev-react-patterns/SKILL.md`。
3. **每改完一个页面**就跑 `npm --prefix web/admin run typecheck`,确保不破坏。
4. **全部改完后**跑:
   - `./scripts/workflow/verify.sh`(全量,包含 user 构建——user agent 的改动也会被验证,但你不应该让 user 构建失败)
   - `./scripts/workflow/review-local.sh --scope committed`
5. **把你的验收结果写到** `.review/admin-gate.json`,格式:
   ```json
   {
     "scope": "web/admin",
     "status": "PASS" | "FAIL",
     "checked_at": "<ISO 时间>",
     "notes": "本次改动的页面清单与遗留问题"
   }
   ```
6. **不要提交 git commit**,除非用户明确要求。

## 与 user agent 的协作信号

如果你在 `web/shared/**` 或 `docs/design/**` 发现需要共享层调整的问题,写到 `docs/plans/admin-polish-notes.md` 的"共享层提议"段落。我会定期检查两个 agent 的 notes 文件,合并不冲突的提议。

**不要**主动去读 `docs/plans/user-polish-notes.md`。两个 agent 的 notes 是独立的,不需要互相参照。

## 验收清单(改完自检)

对照 `docs/design/frontend-design-spec.md` 第 17 节 + `frontend-component-contract.md` Section 11 的通用规则。重点:
- [ ] 管理端无 `rounded-[2rem]`/`rounded-[3rem]`(hero 圆角,管理端禁用)
- [ ] 无硬编码色值
- [ ] 无 `!important` 残留
- [ ] 表格用统一 class,行 hover 高亮,隔行分隔
- [ ] 状态 chip 统一,语义色严格映射
- [ ] EmptyState 有 icon,数据加载有 skeleton
- [ ] TopBar ≤ 80px,不放页面标题/slogan
- [ ] 侧栏 216px 固定
- [ ] Light/Dark 双模式 token-driven,无属性选择器 patch
- [ ] 图标统一 lucide,strokeWidth 1.5
- [ ] `web/user/**` 与 `web/shared/**` 零改动(用 `git diff --name-only` 自检)

开始吧。先读必读上下文,再开工。任何不确定的地方优先在 `admin-polish-notes.md` 记录疑问,不要擅自越界。