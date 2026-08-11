# v0.0.12 体验问题修复与提示词模板实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标：** 完成已确认的 v0.0.12 需求，在完整验证和代码评审通过后合并到 `main`，创建 `v0.0.12` 标签并确认发布 Action 成功。

**架构：** 先以兼容方式扩展 Ent Schema、领域类型和共享 API 契约，再修正以图片结果行为权威来源的资产操作、订单状态机及服务端提示词模板展开。用户端使用 Lexical 作为编辑呈现层，但系统间只传纯文本模板和结构化绑定；旧 Worker 通过活动任务的展开后 `prompt` 保持兼容，终态再清除变量值。

**技术栈：** Go、Ent、PostgreSQL、Worker、React 19、TypeScript、Lexical、Vite、S3 兼容对象存储、GitHub Actions。

---

## 执行规则

- Go 变更遵循 `dev-go-patterns`，React/TypeScript/CSS 变更遵循 `dev-react-patterns`。
- 所有行为变更先写失败测试，再做最小实现并运行聚焦测试。
- Ent Schema 先写 Schema/存储契约测试，再运行 `go generate ./internal/repository/ent` 更新生成代码。
- 不修改平台现有图片数量自动拆分逻辑；不接入 stream、partial images 或重新设计 edits。
- 不恢复 `/docs`，不复制图库对象，不在图片迁移时同步迁移任务。
- 每个批次形成独立提交；发布标签只能创建在 PR 合并后的 `main` 提交上。

### 任务 1：数据结构、领域类型与共享契约

**文件：**
- 修改：`internal/repository/ent/schema/referenceasset.go`
- 修改：`internal/repository/ent/schema/imagetask.go`
- 修改：`internal/repository/ent/schema/schema_test.go`
- 修改：`internal/domain/assets/types.go`
- 修改：`internal/domain/imagetask/types.go`
- 修改：`internal/domain/billing/types.go`
- 修改：`web/shared/api-types.ts`
- 生成：`internal/repository/ent/**`

**步骤：**

1. 增加失败测试，覆盖引用资产名称/标准名称，以及任务原始模板、模板版本和绑定快照字段。
2. 运行 `go test ./internal/repository/ent/schema ./internal/domain/assets ./internal/domain/imagetask ./internal/domain/billing`，确认新契约尚不存在。
3. 添加可滚动升级的可空字段、JSON 契约、名称唯一性约束和任务模板投影；旧记录保持可读。
4. 运行 `go generate ./internal/repository/ent`，再重复聚焦测试并执行共享 TypeScript 类型检查。
5. 提交：`feat: add v012 template data contracts`。

### 任务 2：图片归属、跨项目引用与无条件别名

**文件：**
- 修改：`internal/repository/entstore/imagetask_store.go`
- 修改：`internal/repository/entstore/imagetask_store_test.go`
- 修改：`internal/service/imagetask/service.go`
- 修改：`internal/service/imagetask/service_test.go`
- 修改：`internal/service/imagetask/gallery_batch.go`
- 修改：`internal/service/imagetask/gallery_batch_test.go`
- 修改：`internal/service/assets/service.go`
- 修改：`internal/service/assets/store.go`
- 修改：`internal/service/assets/gallery_import.go`
- 修改：`internal/service/assets/service_test.go`
- 修改：`internal/repository/entstore/assets_store.go`
- 删除：`internal/repository/entstore/alias_rollout_store.go`
- 删除或改写：`internal/repository/entstore/alias_rollout_store_test.go`
- 修改：`internal/http/handlers/api.go`
- 修改：`internal/http/router/gallery_api_test.go`
- 修改：`internal/http/router/gallery_batch_api_test.go`
- 改写：`internal/http/router/gallery_import_copy_api_test.go`
- 修改：`internal/http/router/tasks_api_test.go`

**步骤：**

1. 增加迁移图片回归测试：任务项目保持不变，结果项目分别正确；公开、取消公开、分组、删除、下载和引用都按图片 ID + 用户 ID 成功。
2. 增加对象存储探针测试，证明跨项目引用不校验项目、不调用 Put/Copy，且缺少灰度配置不会返回 409。
3. 运行 `go test ./internal/service/assets ./internal/service/imagetask ./internal/repository/entstore ./internal/http/router -run 'Gallery|Image|Publish|Import|Alias|Project'`，确认失败。
4. 修正 `mapGalleryImageEntity` 及任务结果投影，改用图片结果项目；图片变更走直接用户范围查询。
5. 移除运行时 `AliasCreationEnabled` 门控、Ops 路由和配置目录接线；兼容请求只解析并忽略旧 `project_id`。
6. 重复聚焦测试，确认跨用户仍统一 404、批量冲突逐项返回、对象存储写入为零。
7. 提交：`fix: make image ownership and aliases authoritative`。

### 任务 3：订单过期、后台订单与套餐管理

**文件：**
- 修改：`internal/domain/billing/types.go`
- 修改：`internal/service/billing/store.go`
- 修改：`internal/service/billing/service.go`
- 修改：`internal/service/billing/service_test.go`
- 修改：`internal/repository/entstore/billing_store.go`
- 修改：`internal/repository/entstore/billing_store_test.go`
- 修改：`internal/http/handlers/api.go`
- 修改：`internal/http/router/admin_cashier_api_test.go`
- 修改：`internal/http/router/cashier_api_test.go`
- 修改：`internal/worker/runner.go`
- 修改：`internal/worker/runner_test.go`
- 修改：`internal/app/worker.go`
- 修改：`internal/app/worker_bootstrap.go`
- 修改：`web/shared/api-types.ts`
- 修改：`web/admin/src/pages/OrdersPage.tsx`
- 修改：`web/admin/src/pages/PackagesPage.tsx`
- 修改：`web/admin/src/pages/ordersPage.contract.ts`
- 修改：`web/admin/src/pages/cashierPlanPurchase.ts`
- 修改：`web/admin/src/pages/cashierPlanPurchase.contract.ts`

**步骤：**

1. 增加失败测试，覆盖动态默认 900 秒、创建时固化 `expires_at`、`pending -> expired` 条件流转、并发幂等、读取时兜底和 `expired -> completed` 延迟支付只入账一次。
2. 增加后台订单用户批量投影及基础/赠送/总积分测试；增加套餐模糊搜索、类型/状态筛选、数值排序、启停、归档和恢复测试。
3. 运行 `go test ./internal/service/billing ./internal/repository/entstore ./internal/http/router ./internal/worker -run 'Order|Expiry|Plan|Cashier'`，确认失败。
4. 在存储层实现有限批次条件过期；Worker 每 30 秒处理最多 500 条，列表/详情/同步复用同一惰性过期方法。
5. 后台订单用户信息按页批量查；套餐列表在数据库层筛选、排序和分页，不产生 N+1 或内存全量分页。
6. 先补前端契约，再完成订单列、套餐搜索/排序和生命周期按钮。
7. 运行聚焦 Go 测试及 admin typecheck/build。
8. 提交：`feat: close order expiry and plan workflows`。

### 任务 4：生成参数错误、自动默认值与创作页布局

**文件：**
- 修改：`internal/domain/modelhub/image_size.go`
- 修改：`internal/domain/modelhub/image_size_test.go`
- 修改：`internal/domain/imagetask/types.go`
- 修改：`internal/service/imagetask/service.go`
- 修改：`internal/service/imagetask/service_test.go`
- 修改：`internal/http/handlers/api.go`
- 修改：`web/shared/api-types.ts`
- 修改：`web/shared/user-api.ts`
- 修改：`web/shared/user-api-generation.contract.ts`
- 修改：`web/user/src/pages/workspaceGenerateReadiness.ts`
- 修改：`web/user/src/pages/workspaceGenerateReadiness.contract.ts`
- 修改：`web/user/src/pages/workspaceCreationDraft.ts`
- 修改：`web/user/src/pages/workspaceCreationDraft.contract.ts`
- 修改：`web/user/src/pages/workspaceParameters.ts`
- 修改：`web/user/src/pages/workspaceParameters.contract.ts`
- 修改：`web/user/src/pages/WorkspacePage.tsx`
- 修改：`web/user/src/pages/workspacePage.contract.ts`

**步骤：**

1. 增加失败测试，覆盖字段级非法像素/比例详情优先于 `等待参数`，以及明确草稿 > 用户选择 > `auto` > 现有回退顺序。
2. 增加接口与前端契约，要求模型分组下拉框展示最低积分，页面顺序为模型分组、引用资产、提示词、其他参数，并彻底移除新任务 `negative_prompt`。
3. 运行模型尺寸、图片任务和用户端契约测试，确认失败。
4. 复用服务端权威尺寸校验器返回安全 `details`；前端共享校验器返回结构化错误并接入生成就绪状态。
5. 调整创作页布局和默认值；不改变透明背景、输出格式或图片数量自动拆分规则。
6. 运行聚焦 Go 测试、user typecheck/build。
7. 提交：`fix: clarify generation parameters and workspace layout`。

### 任务 5：提示词模板解析、引用命名与服务端展开

**文件：**
- 新增：`internal/domain/prompttemplate/parser.go`
- 新增：`internal/domain/prompttemplate/parser_test.go`
- 新增：`internal/domain/prompttemplate/resolver.go`
- 新增：`internal/domain/prompttemplate/resolver_test.go`
- 新增：`testdata/prompt-template-fixtures.json`
- 修改：`internal/domain/assets/types.go`
- 修改：`internal/service/assets/service.go`
- 修改：`internal/service/assets/service_test.go`
- 修改：`internal/repository/entstore/assets_store.go`
- 修改：`internal/service/imagetask/service.go`
- 修改：`internal/service/imagetask/service_test.go`
- 修改：`internal/repository/entstore/imagetask_store.go`
- 修改：`internal/repository/entstore/imagetask_store_test.go`
- 修改：`internal/http/handlers/api.go`
- 修改：`internal/http/router/tasks_api_test.go`
- 修改：`internal/service/promptoptimizer/service.go`
- 修改：`internal/service/promptoptimizer/service_test.go`

**步骤：**

1. 写语言无关语料和失败测试，覆盖资源/变量、转义、Unicode NFC、重复项、非法/未闭合/嵌套语法、偏移和数量/长度限制。
2. 写失败测试，覆盖引用默认命名、用户范围唯一、并发分配、软删除后复用和重命名冲突。
3. 写失败测试，覆盖资源按 `reference_asset_ids` 展开为 `图片N`、变量值不递归解析、额外已选图片保留、改名竞态 409、跨用户 404。
4. 实现纯解析器和展开器；创建事务同时保存展开后 `prompt`、原始 `prompt_template` 和不含变量值的绑定快照。
5. 所有终态路径用原始模板覆盖 `prompt`；增加有限修复扫描和未清理指标。
6. 为提示词优化增加哨兵保护和精确多重集合校验，破坏占位符时返回 `INVALID_OPTIMIZATION_RESULT`。
7. 运行 `go test ./internal/domain/prompttemplate ./internal/service/assets ./internal/service/imagetask ./internal/service/promptoptimizer ./internal/repository/entstore ./internal/http/router`。
8. 提交：`feat: resolve prompt templates safely`。

### 任务 6：Lexical 提示词编辑器与变量表单

**文件：**
- 修改：`web/user/package.json`
- 修改：根目录 npm lock 文件
- 新增：`web/user/src/pages/promptTemplateParser.ts`
- 新增：`web/user/src/pages/promptTemplateParser.contract.ts`
- 新增：`web/user/src/pages/PromptTemplateEditor.tsx`
- 新增：`web/user/src/pages/promptTemplateEditor.contract.ts`
- 新增：`web/user/src/pages/PromptVariableForm.tsx`
- 修改：`web/user/src/pages/WorkspacePage.tsx`
- 修改：`web/user/src/pages/workspacePromptOptimization.ts`
- 修改：`web/user/src/pages/workspacePromptOptimization.contract.ts`
- 修改：`web/user/src/pages/workspaceTaskHistory.ts`
- 修改：`web/user/src/pages/workspaceTaskHistory.contract.ts`
- 修改：`web/user/src/styles.css`

**步骤：**

1. 安装 Lexical 相关包并锁定版本；先写 TypeScript 解析器契约，消费与 Go 完全相同的语料。
2. 写失败契约，覆盖纯文本序列化、蓝/红 Tag、资源预览、变量值预览、键盘焦点、粘贴、撤销/重做、中文 IME 和紧凑/弹窗同步。
3. 实现 `PromptTokenNode`、工具栏插入、`@` 资源菜单、`$` 变量选择/创建，以及关闭后焦点恢复。
4. 实现按首次出现去重的变量表单；未赋值时阻止创建，但不阻止价格预估。
5. 接入引用名称重命名；只更新当前草稿，不改历史模板。
6. 历史接口和复用只恢复原始模板及兼容参数，不恢复引用资产或变量值。
7. 运行 user typecheck/build 和相关契约测试。
8. 提交：`feat: add prompt template editor`。

### 任务 7：资产选择、刷新与支付方式展示

**文件：**
- 修改：`web/user/src/pages/GalleryPage.tsx`
- 修改：`web/user/src/pages/galleryExperience.ts`
- 修改：`web/user/src/pages/galleryExperience.contract.ts`
- 修改：`web/user/src/pages/galleryBatchActions.ts`
- 修改：`web/user/src/pages/galleryBatchActions.contract.ts`
- 修改：`web/user/src/pages/ProjectsPage.tsx`
- 修改：`web/user/src/pages/CheckoutPage.tsx`
- 新增或修改：`web/user/src/pages/checkoutPaymentMethods.ts`
- 修改：`web/user/src/pages/checkoutPaymentDisplay.ts`
- 修改：`web/user/src/styles.css`
- 修改：`web/shared/api-types.ts`
- 修改：`internal/http/handlers/api.go`
- 修改：`internal/http/router/cashier_api_test.go`

**步骤：**

1. 写失败契约，覆盖高对比选中图标、选择模式点击切换、桌面拖框替换/追加、触屏滚动和卡片操作排除。
2. 写刷新状态测试，要求资产页和项目页图标刷新保留项目/筛选，失败保留数据，旧响应不能覆盖新响应。
3. 写支付 DTO 和界面契约，确保用户响应/文案不出现 JeePay、EasyPay、内部实例或调度策略，并使用本地公开品牌图标和紧凑网格。
4. 实现框选动画帧计算、6 px 阈值、边缘滚动和点击抑制；粗指针禁用。
5. 分离用户端与后台支付投影并兼容历史 `visible_method`。
6. 运行用户端契约、typecheck/build 及收银台 API 测试。
7. 提交：`feat: improve gallery selection and checkout privacy`。

### 任务 8：管理后台所有列表统一刷新

**文件：**
- 修改：`web/admin/src/components.tsx`
- 修改：`web/admin/src/components.contract.ts`
- 修改：`web/admin/src/App.tsx`
- 修改：`web/admin/src/pages/OverviewPage.tsx`
- 修改：`web/admin/src/pages/MonitoringPage.tsx`
- 修改：`web/admin/src/pages/ClusterPage.tsx`
- 修改：`web/admin/src/pages/UsersPage.tsx`
- 修改：`web/admin/src/pages/UserGroupsPage.tsx`
- 修改：`web/admin/src/pages/ReviewPage.tsx`
- 修改：`web/admin/src/pages/RedeemPage.tsx`
- 修改：`web/admin/src/pages/OrdersPage.tsx`
- 修改：`web/admin/src/pages/PackagesPage.tsx`
- 修改：`web/admin/src/pages/CashierPage.tsx`
- 修改：`web/admin/src/pages/ProviderModelsPage.tsx`
- 修改：`web/admin/src/pages/RoutingPage.tsx`
- 修改：`web/admin/src/pages/PricingPage.tsx`
- 修改：`web/admin/src/pages/CallRecordsPage.tsx`
- 修改：`web/admin/src/pages/SystemUsersPage.tsx`
- 修改：`web/admin/src/pages/AuditPage.tsx`
- 修改：`web/admin/src/pages/SystemSettingsPage.tsx`
- 新增：`web/admin/src/pages/listRefresh.contract.ts`

**步骤：**

1. 写失败契约，枚举每个列表菜单页并要求存在共享图标刷新控件；重复点击筛选必须重新请求。
2. 实现共享 `RefreshIconButton` 和请求代次辅助函数；加载时保留旧数据，失败时保留页面状态。
3. 逐页接入，保留筛选、排序、分页、选择、活动页签和仍有效的弹窗。
4. 运行 admin 契约、typecheck/build，并通过桌面/移动截图检查无文字重叠。
5. 提交：`feat: add consistent admin list refresh`。

### 任务 9：完整验证、代码评审与问题修复

**文件：**
- 按失败结果修改相关生产代码和测试。
- 必要时更新：`docs/prd/2026-08-11-v012-post-release-experience-remediation-requirements.md`
- 必要时更新：`docs/tech/2026-08-11-v012-post-release-experience-remediation-tech-design.md`

**步骤：**

1. 运行所有聚焦测试并修复失败。
2. 运行 `./scripts/workflow/verify.sh`，要求 Go test/vet、用户端和后台 typecheck/build 全部通过。
3. 运行 `./scripts/workflow/api-smoke.sh`，验证真实 API、Worker、PostgreSQL、Redis 和假供应商契约。
4. 启动本地服务，使用浏览器验证创作模板、迁移图片、资产框选、收银台、订单和套餐；覆盖桌面和移动视口。
5. 执行本地代码 Review，修复所有 P0/P1/P2 发现，重新运行受影响测试。
6. 将实现提交完毕后运行 `./scripts/workflow/review-local.sh --scope committed` 和 `./scripts/workflow/check-review-gate.sh`。
7. 提交：`test: verify v012 remediation release`。

### 任务 10：PR、合并、标签与发布

**文件：**
- 更新：发布说明或 changelog（以仓库现有发布流程为准）。

**步骤：**

1. 确认工作树干净、Review Gate 与当前 HEAD tree 一致，并推送 `codex/v012-experience-remediation`。
2. 创建面向 `main` 的非草稿 PR，包含需求映射、迁移/回滚说明和验证证据。
3. 等待 CI 和代码评审通过；修复所有失败后重新执行本地验证并推送。
4. 合并 PR，更新本地 `main`，确认合并提交包含全部 v0.0.12 变更。
5. 在该 `main` 合并提交创建并推送注解标签 `v0.0.12`，不得给功能分支提交打标签。
6. 观察标签触发的 GitHub Actions，确认所有构建和发布 Job 成功，并核对发布产物版本为 v0.0.12。
7. 只有 PR 已合并、标签已推送且发布 Action 成功后，目标才标记完成。
