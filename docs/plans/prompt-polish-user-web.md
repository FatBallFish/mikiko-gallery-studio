# Prompt: 用户端页面进一步打磨

> 把这段完整贴给负责用户端的 AI agent(Claude Code / Codex / 其他)。

---

你是 pic-gallery 项目用户端(`web/user`)的视觉打磨工程师。前一轮已经建立了设计系统底座并改造了核心页面,现在你要做的是**进一步精修**,不是重做。

## 项目位置

仓库根目录:`/Users/fatballfish/Documents/Projects/GoProjects/Personal/pic-gallery`
你负责的代码范围:`web/user/`

## 必读上下文(开工前完整读完,不要跳读)

1. `docs/plans/2026-07-03-frontend-redesign-baseline.md` — 上一轮重设计的完整计划与执行记录
2. `docs/design/frontend-design-spec.md` — 母设计规范(第 15 节禁止项、第 17 节验收清单是硬约束)
3. `docs/design/frontend-visual-directions.md` — 视觉方向("Luminous Vault"高奢数字艺廊)
4. `docs/design/frontend-component-contract.md` — 组件级实施契约(Button/Modal/Field/EmptyState/Toast/StatusPill/Card/动效词汇表/验收清单)
5. `web/user/src/ui/redesign-classes.ts` — 当前唯一 class 系统
6. `web/user/src/styles.css` — 主题 token 与氛围层
7. `web/shared/tokens.css` — 全局 token(间距/圆角/动效曲线)
8. `AGENTS.md` — 仓库 AI 工作流(必须遵循)

## 关键约束:与 admin agent 并行工作,严禁越界

**本次任务与另一个负责 `web/admin` 的 AI agent 在同一分支并行进行。你必须严格遵守以下隔离协议,否则会产生冲突:**

### 你可以读、可以写的目录与文件

- ✅ `web/user/**`(全部)
- ✅ `docs/plans/2026-07-03-frontend-redesign-baseline.md`(只读,参考)
- ✅ `docs/design/frontend-component-contract.md`(只读,契约)
- ✅ `docs/design/frontend-design-spec.md`(只读)
- ✅ `docs/design/frontend-visual-directions.md`(只读)
- ✅ `docs/plans/user-polish-notes.md`(你自己的笔记文件,新建)
- ✅ `.review/user-gate.json`(你自己的验收产物)

### 你绝对不能写的目录与文件(归 admin agent 或共享层)

- ❌ `web/admin/**` — admin agent 的领地,任何修改都视为越界
- ❌ `web/shared/**` — 共享 token 层,如果你觉得需要改 token,在 `docs/plans/user-polish-notes.md` 里写"提议"段落,不要直接改文件
- ❌ `docs/design/frontend-design-spec.md`、`frontend-visual-directions.md`、`frontend-component-contract.md` — 契约文档,只读
- ❌ `docs/plans/admin-polish-notes.md` — admin agent 的笔记
- ❌ `.review/admin-gate.json` — admin agent 的验收产物
- ❌ `Makefile`、`scripts/**`、`.githooks/**`、`.hook-scripts/**` — CI/构建脚本
- ❌ `cmd/**`、`internal/**`、`pkg/**` — 后端 Go 代码
- ❌ `go.mod`、`go.sum`

### 共享文件的冲突避免

`web/shared/tokens.css`、`web/shared/base.css`、`web/shared/user-theme.css`、`web/shared/admin-theme.css` 是两端共用的。如果你确实需要调整某个共享 token(比如改 `--pg-ease-spring` 的曲线),**不要直接编辑**,而是在 `docs/plans/user-polish-notes.md` 里写:

```
## 共享层提议
- 文件: web/shared/tokens.css
- 改动: --pg-ease-spring 从 cubic-bezier(0.32,0.72,0,1) 改为 cubic-bezier(0.34,1.56,0.64,1)
- 理由: 用户端 hero 入场需要更明显的回弹
- 对 admin 的影响: admin 的 modal 进场也会用这个曲线,可能略增弹性,应可接受
```

我会人工评估后再决定是否合并。**任何对 `web/shared/**` 的直接编辑都会被 git diff 视为越界并要求回滚。**

## 你的任务范围

聚焦 `web/user` 的以下精修点(不限于这些,但优先级最高):

### 1. 视觉一致性巡检
- 扫描 `web/user/src/` 全目录,找出所有 `rounded-[数字]` 出现的地方,确认只用契约规定的四档:`rounded-xl`(12px)、`rounded-2xl`(16px)、`rounded-3xl`(24px)、`rounded-[2rem]`(32px,仅 hero 大封面)、`rounded-full`。其他如 `rounded-[10px]`、`rounded-[14px]`、`rounded-[18px]`、`rounded-[22px]`、`rounded-[2.5rem]`、`rounded-[3rem]`、`rounded-[1.4rem]` 等一律替换到最近档位。
- 扫描所有硬编码色值(`#xxxxxx`、`rgb(...)`、`rgba(...)` 不带 var),改为 `var(--accent)`/`var(--bg)`/`var(--fg)` 等 token。品牌 mark(`brand.tsx` 的 logo 图)除外。
- 扫描所有 `!important` / `!` 前缀的 Tailwind 强制覆盖,改为正确的 class 层级。

### 2. 交互反馈精修
- 所有可点击元素加 `active:scale-[0.98]` 或 `active:translate-y-0` 触觉反馈(契约 Section 2.2)。
- 卡片 hover 用 `transition duration-300 ease-out`,不要突变(契约 Section 8)。
- 列表/网格入场用 `.pg-enter` 或 `useReveal`,但**注意**:如果父组件有 `loading ? <LoadingState/> : <真实内容>` 的早返回模式,不要用 `useReveal`(会因 effect 依赖不重跑导致永久隐藏),改用纯 CSS `.pg-enter` 类。
- `prefers-reduced-motion: reduce` 下所有动画即时降级(契约已内置,但要确认你的新动画也遵守)。

### 3. EmptyState / LoadingState 升级
- 所有页面的空状态用 `<EmptyState>` 组件 + 图标(不要裸 dashed border)。
- 数据加载用 `.pg-skeleton` 骨架占位,而不是单一 spinner(至少在 Home、Gallery、Profile ledger、ApiKeys 列表 这些数据密度高的地方)。

### 4. 主题一致性
- 确认 `themePreferences.ts` 默认 `mode: 'light'`(已改,不要回退)。
- Light 与 Dark 两种模式下逐页核对对比度、accent 可见性、`body::before` 氛围层。
- 切换主题时不应有闪烁或布局位移。

### 5. 移动端(375px)巡检
- 每个页面在 `max-[760px]` / `max-[420px]` 断点下的布局是否合理。
- 侧栏在移动端是否正确转为底部导航。
- 长文本是否折行合理,按钮是否溢出。

### 6. 文案精修(可选,低优先级)
- 扫描所有页面可见文案,删掉"数千名创作者""放飞灵感"这类空泛营销话术,改为功能性短句。
- 中英文混用要统一(HomePage hero 不要英文标题配中文正文)。

## 工作流程(严格遵守 AGENTS.md)

1. **开工前**:运行 `./scripts/workflow/start-coding.sh --task "用户端页面视觉精修"` 建立 `.coding-context.json`(如果已存在且 task 不符,先备份再重建)。
2. **不要**碰 Go 代码,所以不需要 `dev-go-patterns`。
3. **碰 React 前必读** `.claude/skills/dev-react-patterns/SKILL.md` 或 `.agents/skills/dev-react-patterns/SKILL.md`。
4. **每改完一个页面**就跑 `npm --prefix web/user run typecheck`,确保不破坏。
5. **全部改完后**跑:
   - `./scripts/workflow/verify.sh`(全量,包含 admin 构建——admin agent 的改动也会被验证,但你不应该让 admin 构建失败)
   - `./scripts/workflow/review-local.sh --scope committed`
6. **把你的验收结果写到** `.review/user-gate.json`,格式:
   ```json
   {
     "scope": "web/user",
     "status": "PASS" | "FAIL",
     "checked_at": "<ISO 时间>",
     "notes": "本次改动的页面清单与遗留问题"
   }
   ```
7. **不要提交 git commit**,除非用户明确要求。你的改动留在工作区,由用户人工审阅合并。

## 与 admin agent 的协作信号

如果你在 `web/shared/**` 或 `docs/design/**` 发现需要共享层调整的问题,写到 `docs/plans/user-polish-notes.md` 的"共享层提议"段落。我会定期检查两个 agent 的 notes 文件,合并不冲突的提议。

**不要**主动去读 `docs/plans/admin-polish-notes.md`。两个 agent 的 notes 是独立的,不需要互相参照。

## 验收清单(改完自检)

对照 `docs/design/frontend-component-contract.md` Section 11 的验收清单逐条过一遍。重点:
- [ ] 圆角只剩 12/16/24/32/full 五档
- [ ] 无硬编码色值(除品牌 mark)
- [ ] 无 `!important` 残留
- [ ] EmptyState 有 icon 容器,不裸 dashed
- [ ] 数据加载页有 skeleton
- [ ] Light/Dark 双模式对比度通过 WCAG AA
- [ ] 移动端 375px 布局不溢出
- [ ] `prefers-reduced-motion` 下降级
- [ ] `web/admin/**` 与 `web/shared/**` 零改动(用 `git diff --name-only` 自检)

开始吧。先读必读上下文,再开工。任何不确定的地方优先在 `user-polish-notes.md` 记录疑问,不要擅自越界。