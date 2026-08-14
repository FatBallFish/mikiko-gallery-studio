# v0.0.21 Admin Media Tabs And Canvas Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 完成管理后台四类媒体模型配置和视频完整层级管理，并修复新旧空白画布的 `null` 数组崩溃。

**Architecture:** 后端继续复用统一模型账号、模型、路由和视频价格结构；管理端按媒体类型组合现有 API 并提供结构化主从界面。画布在 Go 服务/API 与 TypeScript API 边界双重归一化，兼容历史坏数据而不执行数据库迁移。

**Tech Stack:** Go, Ent, React, TypeScript, Vite

---

### Task 1: 固化画布空数组契约

**Files:**

- Modify: `internal/service/canvas/service_test.go`
- Modify: `internal/http/router/canvas_api_test.go`
- Modify: `web/user/src/features/canvas/core/canvasState.contract.ts`
- Modify: `web/user/src/features/canvas/CanvasList.contract.ts`

**Steps:**

1. 增加新建空白画布返回 `nodes:[]/edges:[]` 的 Go 服务测试。
2. 增加 API JSON 不得出现 `nodes:null/edges:null` 的测试。
3. 增加前端历史 null 文档归一化、列表和编辑器不崩溃的 contract。
4. 分别运行定向测试，确认测试先失败并记录 RED 原因。

### Task 2: 实现画布三层归一化

**Files:**

- Modify: `internal/service/canvas/service.go`
- Modify: `internal/repository/entstore/canvas_store.go`
- Modify: `web/user/src/features/canvas/core/canvasState.ts`
- Modify: `web/user/src/features/canvas/canvasApi.ts`
- Modify: `web/user/src/features/canvas/CanvasEditorPage.tsx`
- Modify: `web/user/src/features/canvas/CanvasListPage.tsx`

**Steps:**

1. 新增 Go `normalizeDocument` 空切片不变量，并覆盖创建、保存和读取路径。
2. 新增 TypeScript `normalizeCanvasDocument` 并在 API 边界使用。
3. 列表预览保留局部防御，错误提示区分网络错误和本地数据错误。
4. 运行定向 Go/TypeScript 测试，确认 GREEN。
5. 提交独立修复提交。

### Task 3: 统一管理后台媒体 Tab

**Files:**

- Create: `web/admin/src/pages/adminModelMedia.ts`
- Create: `web/admin/src/pages/AdminMediaTabs.tsx`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `web/admin/src/pages/SystemSettingsPage.tsx`
- Modify: `web/admin/src/App.tsx`
- Test: `web/admin/src/pages/adminModelMedia.contract.ts`

**Steps:**

1. 编写 URL 解析、回退、三个页面四类 Tab 和旧文本入口跳转的失败 contract。
2. 实现共享媒体类型解析与 `AdminMediaTabs`。
3. 将既有图片页面包入 image 面板，加入 audio 明确空状态。
4. 把文本模型页面改为可嵌入模式，迁移系统设置入口并保留兼容跳转。
5. 运行 admin contract、typecheck、build，确认 GREEN。

### Task 4: 视频账号与真实模型主从管理

**Files:**

- Create: `web/admin/src/pages/VideoProviderAccountsPanel.tsx`
- Modify: `web/admin/src/adminApi.ts`
- Modify: `web/admin/src/adminTaskTypes.ts`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Test: `web/admin/src/pages/videoProviderAccounts.contract.ts`

**Steps:**

1. 编写多账号、多真实模型、Seedance/MiniMax 过滤和结构化能力/成本编辑 contract，确认 RED。
2. 复用现有 `ModelAccount`、`ModelAccountModel` API 实现账号主列表和模型从列表。
3. 将视频能力、成本字段转换为结构化表单；厂商扩展字段保留高级编辑区。
4. 补齐加载、空、错误、刷新、启停和删除状态。
5. 运行 contract、typecheck、build，确认 GREEN。

### Task 5: 视频路由候选与参数组合管理

**Files:**

- Create: `web/admin/src/pages/VideoRoutingPanel.tsx`
- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Modify: `web/admin/src/pages/VideoConfigurationWorkspace.tsx`
- Test: `web/admin/src/pages/videoRoutingPanel.contract.ts`

**Steps:**

1. 编写路由媒体过滤、有效视频候选、多候选和自动加载路由配置 contract，确认 RED。
2. 实现视频路由列表与详情，复用既有 route/candidate API。
3. 把可见组合、默认参数和候选权重改为结构化控件。
4. 去除正常流程中的路由 ID、模型 ID 和组合 JSON 手填。
5. 运行定向测试和 admin 构建，确认 GREEN。

### Task 6: 视频价格策略与参数绑定

**Files:**

- Create: `web/admin/src/pages/VideoPricingPanel.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `internal/domain/video/config.go`
- Modify: `internal/service/videotask/quote.go`
- Modify: `internal/service/videotask/quote_test.go`
- Test: `web/admin/src/pages/videoPricingPanel.contract.ts`

**Steps:**

1. 编写策略列表、规则编辑、自动 ID、试算和参数策略绑定的失败测试。
2. 实现视频策略和规则的结构化列表/编辑界面。
3. 在 `visible_options` 中解析兼容的 `pricing_bindings`，运行时先匹配绑定策略、再回退默认策略。
4. 确保最终命中策略和规则写入既有计费快照。
5. 运行 Go 定向测试、admin contract/typecheck/build，确认 GREEN。

### Task 7: 集成、视觉与回归验证

**Files:**

- Modify tests/contracts only when a real uncovered regression is found.

**Steps:**

1. 运行 `./scripts/workflow/verify.sh`。
2. 启动本地服务并运行 `./scripts/workflow/api-smoke.sh`。
3. 用浏览器验证管理后台三个页面的四类 Tab、视频层级操作、画布新建/列表/编辑器。
4. 检查桌面和 1024px 平板横屏，无白屏、溢出、遮挡或原生突兀控件。
5. 对发现的问题补测试后修复，重新验证。

### Task 8: Review、PR 与发布

**Steps:**

1. 审查当前分支相对 `origin/main` 的全部 diff，修复 P0-P2 问题。
2. 提交全部变更并运行 `./scripts/workflow/review-local.sh --scope committed`。
3. 运行 `./scripts/workflow/check-review-gate.sh` 和 `./scripts/workflow/api-smoke.sh`。
4. 推送 `codex/v021-admin-media-tabs-canvas-fix` 并创建 PR。
5. 等待 CI 通过后合入 `main`。
6. 基于合入后的 `main` 创建并推送 `v0.0.21`。
7. 等待 tag Action 和镜像/制品发布成功，记录最终链接和版本。
