# v0.0.12 Workspace Prompt Follow-ups Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复 v0.0.12 创作页模型选择、引用资产命名、Prompt Tag 自动补全和首页 Prompt 复制体验。

**Architecture:** 保持现有 API 不变，在用户端增加可访问的模型 Listbox、引用重命名弹窗和 Lexical Token 交互状态。首页仍使用匿名摘要列表，通过已登录详情请求获得完整 Prompt。

**Tech Stack:** React 19、TypeScript、Lexical、Lucide、Vite、仓库 contract tests。

---

### Task 1: 建立编码上下文

**Files:**
- Read: `docs/prd/2026-08-11-v012-workspace-prompt-followups-requirements.md`
- Read: `docs/tech/2026-08-11-v012-workspace-prompt-followups-tech-design.md`

1. 运行 `./scripts/workflow/start-coding.sh --task "修复 v0.0.12 创作页模型下拉、引用命名、提示词 Tag 与首页 Prompt 复制体验"`。
2. 确认生成 `.coding-context.json` 且 requirement/design 指向本次文档。

### Task 2: 模型分组 Listbox

**Files:**
- Create: `web/user/src/pages/ModelGroupSelect.tsx`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/styles.css`
- Test: `web/user/src/pages/workspacePage.contract.ts`

1. 先修改契约，要求不再出现模型原生 select，并要求 Title、SubTitle、`◈` 与 listbox 标记。
2. 运行 `bash scripts/workflow/verify-contracts.sh`，确认契约因组件缺失失败。
3. 实现 `ModelGroupSelect` 并接入 Workspace。
4. 再运行契约，确认通过。

### Task 3: 引用资产名称布局和弹窗

**Files:**
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/styles.css`
- Test: `web/user/src/pages/workspacePage.contract.ts`

1. 先写名称位于预览下方、弹窗编辑和完整名称可访问的契约断言。
2. 运行契约并确认因旧内嵌编辑器失败。
3. 调整卡片结构，使用现有 Dialog 和重命名状态实现弹窗。
4. 运行契约并确认通过。

### Task 4: Prompt Tag 删除与单次取消

**Files:**
- Modify: `web/user/src/pages/PromptTemplateEditor.tsx`
- Create/Modify: `web/user/src/pages/promptAutocompleteModel.ts`
- Modify: `web/user/src/styles.css`
- Test: `web/user/src/pages/promptTemplateEditor.contract.ts`

1. 先新增测试：Tag 视觉只显示 key、关闭只删除当前节点、相同 trigger 被取消后不再弹出、文本变化后可再次触发。
2. 运行契约并确认失败原因是缺少新行为。
3. 实现可交互 Token 渲染、按 node key 删除和 dismissed trigger 状态模型。
4. 运行契约并确认通过，同时回归模板标准文本 round-trip。

### Task 5: 首页复制完整 Prompt

**Files:**
- Modify: `web/user/src/pages/HomePage.tsx`
- Modify: `web/user/src/pages/homeGalleryModel.ts`
- Test: `web/user/src/pages/homePublicAssets.contract.ts`
- Test: `web/user/src/pages/homeGalleryCard.contract.ts`

1. 先写契约要求首页使用会话 token 拉详情、复制详情 prompt，同时继续匿名拉列表。
2. 运行契约并确认因首页未拉详情而失败。
3. 实现带竞态保护的详情请求和错误提示。
4. 运行契约并确认通过。

### Task 6: 验证、浏览器验收与交付

**Files:**
- Review: all changed files

1. 运行 `npm --prefix web/user run typecheck` 和 `npm --prefix web/user run build`。
2. 启动本地用户端，使用浏览器检查桌面和移动端布局、键盘行为、outside click、长名称、重复 Tag 与完整复制。
3. 运行 `./scripts/workflow/verify.sh`。
4. 提交全部改动后运行 `./scripts/workflow/review-local.sh --scope committed` 与 `./scripts/workflow/check-review-gate.sh`。
5. review 通过后推送 `codex/v012-workspace-prompt-followups`；不创建或合并 PR。
