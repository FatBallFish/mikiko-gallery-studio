# 多媒体创作平台一期实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在保持现有图片生成完全兼容的前提下，交付 Seedance 2.5/2.0 与 MiniMax H3 视频生成、可恢复计费与任务链路、统一多媒体资产与 1 GiB 上传、支持生图/生视频的创意画布，以及对应管理后台和发布门禁。

**Architecture:** 继续采用当前 Go 模块化单体，在既有用户、项目、路由、钱包、存储和 Worker 基础上新增独立 `video`、`media`、`canvas` 领域。视频使用持久化异步状态机和逐结果 item，媒体正文由浏览器或 Worker 与存储直传直取，画布使用服务端 revision 文档与 IndexedDB 草稿；历史图片通过同 UUID 投影和幂等回填进入统一资产，不复制对象。

**Tech Stack:** Go 1.24、Ent/PostgreSQL、Redis、S3/MinIO/Local filesystem、FFmpeg/ffprobe、React 19、TypeScript、Vite、Zustand、localForage、Dagre、Playwright。

---

## 执行规则

- 需求源：`docs/prd/2026-08-12-multimedia-creation-phase1-prd.md`。
- 技术源：`docs/tech/2026-08-12-multimedia-creation-phase1-tech-design.md`。
- 交互参考：`docs/prototypes/multimedia-creation-phase1-demo.html`，只参考流程，不复制视觉。
- 每项行为先写最小失败测试并实际观察 RED，再写生产代码，随后运行目标测试观察 GREEN。
- Ent schema 修改后运行 `go generate ./internal/repository/ent`，生成代码与 schema 同批提交。
- 新 HTTP 路径必须同时更新 `api/openapi/openapi.yaml`、`api/openapi/routes.go`、路由契约测试和共享前端类型。
- 每个工作包完成后运行相关 Go 测试、两端 typecheck/build；W9 执行完整 ship gate、API smoke、浏览器与媒体验收。
- 不提交 Provider 密钥、签名 URL、真实 Prompt、媒体正文或线下商业授权的虚构证明。

## Task 0：PoC 契约、依赖与黄金样本

**Files:**
- Create: `docs/tech/contracts/video-provider-capabilities-v1.json`
- Create: `docs/tech/contracts/video-provider-pricing-v1.json`
- Create: `docs/licenses/nova-image-studio-third-party.md`
- Create: `testdata/media/README.md`
- Create: `internal/provider/video/contract_fixtures_test.go`
- Modify: `Dockerfile.worker`
- Modify: `deployments/devops/run-worker.sh`
- Modify: `scripts/workflow/api-smoke.sh`

**Steps:**

1. 从已确认的三个模型代码建立版本化 capability/cost fixture；未知真实账号事实标记 `untested`，不得伪造已验证状态。
2. 盘点 Nova 画布拟复用文件及其 npm 依赖许可证，记录来源 commit `7768f3f` 和改写边界。
3. 写失败测试，要求 capability fixture 包含三个模型、版本、任务类型、格式、原生 `n`、回调/轮询和验证状态。
4. 运行 `go test ./internal/provider/video -run ContractFixtures -count=1`，确认因实现或 fixture 缺失而失败。
5. 实现强类型 fixture loader 和 schema 校验；对未完成真实账号 PoC 的组合保持不可启用。
6. 固定 Worker FFmpeg/ffprobe 安装版本，加入 `ffmpeg -version`/`ffprobe -version` readiness 合约。
7. 加入最小合法/损坏图片、MP4/MOV、MP3/M4A/WAV 生成脚本或小型 fixture，测试 probe、H.264/AAC faststart proxy 和超时。
8. 运行 Provider fixture 与媒体黄金样本测试并提交：`test: freeze multimedia provider and media contracts`。

## Task 1：领域契约、状态机与计价纯函数

**Files:**
- Create: `internal/domain/video/types.go`
- Create: `internal/domain/video/capability.go`
- Create: `internal/domain/video/state.go`
- Create: `internal/domain/video/pricing.go`
- Create: `internal/domain/video/*_test.go`
- Create: `internal/domain/media/types.go`
- Create: `internal/domain/media/policy.go`
- Create: `internal/domain/media/*_test.go`
- Create: `internal/domain/canvas/document.go`
- Create: `internal/domain/canvas/graph.go`
- Create: `internal/domain/canvas/*_test.go`

**Steps:**

1. 写视频能力 RED：完整组合匹配 task type、时长、清晰度、比例、音频、首尾帧、格式/大小，且 `provider_native_max_n` 只限制 1-10，不限制平台 1-4 item 拆分。
2. 运行 `go test ./internal/domain/video -run Capability -count=1` 确认 RED；实现 capability v1 解析、规范化、候选并集和完整候选过滤后确认 GREEN。
3. 写状态机 RED，覆盖单调推进、过期 poll、重复 callback、取消晚于成功、artifact recovery；实现纯状态转换函数并确认 GREEN。
4. 写 decimal 计价 RED，覆盖精确/计量报价、最低价、首尾帧、音频附加、1-4 结果、25% 毛利安全线、1.00/1.15 reserve markup、部分成功结算；实现后确认 GREEN。
5. 写媒体策略 RED，覆盖扩展名/MIME/实际 probe 分层、1 GiB 硬上限、派生计划、受控 object prefix；实现后确认 GREEN。
6. 写画布文档 RED，覆盖七类节点、合法/非法连接、首尾帧角色冲突、循环、500/1000/1 MiB 上限、稳定结果节点 ID 和引用提取；实现后确认 GREEN。
7. 运行 `go test ./internal/domain/video ./internal/domain/media ./internal/domain/canvas -count=1` 并提交：`feat(domain): add multimedia contracts and state machines`。

## Task 2：Ent schema、迁移与事务端口

**Files:**
- Create: `internal/repository/ent/schema/videomodelcapability.go`
- Create: `internal/repository/ent/schema/videorouteconfig.go`
- Create: `internal/repository/ent/schema/videopricingstrategy.go`
- Create: `internal/repository/ent/schema/videopricerule.go`
- Create: `internal/repository/ent/schema/videoprovidercostrule.go`
- Create: `internal/repository/ent/schema/videotask.go`
- Create: `internal/repository/ent/schema/videotaskitem.go`
- Create: `internal/repository/ent/schema/videotaskinput.go`
- Create: `internal/repository/ent/schema/videotaskattempt.go`
- Create: `internal/repository/ent/schema/mediaasset.go`
- Create: `internal/repository/ent/schema/mediaderivative.go`
- Create: `internal/repository/ent/schema/mediauploadsession.go`
- Create: `internal/repository/ent/schema/mediaprocessingjob.go`
- Create: `internal/repository/ent/schema/mediaassetreference.go`
- Create: `internal/repository/ent/schema/creativecanvas.go`
- Create: `internal/repository/ent/schema/creativecanvasrevision.go`
- Create: `internal/repository/ent/schema/canvasgenerationrun.go`
- Modify: `internal/repository/ent/schema/routemodel.go`
- Modify: `internal/repository/ent/schema/pointledger.go`
- Modify: `internal/repository/db/migrate_test.go`
- Modify: `internal/repository/db/schema_v2_migration_test.go`
- Modify: `internal/service/billing/store.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Modify: `internal/repository/entstore/billing_store_test.go`

**Steps:**

1. 写 schema RED，断言所有表、默认值、软删除、唯一键和调度索引符合技术方案，旧图片行可在新列默认值下插入。
2. 运行目标 schema/migration tests 确认 RED。
3. 添加 Ent schemas，使用 UUID/decimal string/JSON/TimeMixin 约定；生成 Ent 代码。
4. 运行迁移测试确认 expand-first 和重复迁移 GREEN。
5. 写事务 RED：创建视频 task/items/inputs 与钱包预留任一失败时全部回滚；同 task finalize 只发生一次。
6. 抽出可接收 `*ent.Tx` 的钱包 reservation/finalize 端口，保留现有图片 Service 的公共行为；实现视频 task transaction store。
7. 运行 `go test ./internal/repository/db ./internal/repository/ent/schema ./internal/repository/entstore ./internal/service/billing -count=1`。
8. 提交：`feat(store): add multimedia schema and transactional billing port`。

## Task 3：存储分片上传与媒体处理

**Files:**
- Modify: `internal/storage/backend.go`
- Modify: `internal/storage/router.go`
- Create: `internal/storage/multipart.go`
- Create: `internal/storage/local_multipart.go`
- Create: `internal/storage/multipart_test.go`
- Create: `internal/service/mediaasset/store.go`
- Create: `internal/service/mediaasset/service.go`
- Create: `internal/service/mediaasset/*_test.go`
- Create: `internal/service/mediaprocess/probe.go`
- Create: `internal/service/mediaprocess/derivatives.go`
- Create: `internal/service/mediaprocess/*_test.go`
- Create: `internal/repository/entstore/media_store.go`
- Create: `internal/repository/entstore/media_store_test.go`
- Modify: `internal/app/object_cleanup.go`
- Modify: `internal/domain/objectcleanup/types.go`

**Steps:**

1. 写 S3/MinIO multipart RED：create/sign/complete/abort、part checksum、重放完成、缺 part、短时签名，不代理正文。
2. 写 Local RED：8-32 MiB chunk、断点恢复、单块重试、流式合并、原子 rename、磁盘预算 `size*2+2GiB`、取消/过期清理、RSS 不随 1 GiB 线性增长。
3. 扩展 storage optional capabilities 和 router，保持旧 Backend 接口兼容；跑存储测试 GREEN。
4. 写上传 Service RED，覆盖项目 owner、配额预留/确认/释放、同 key 同 body 幂等、同 key 异 body 409、重名不覆盖。
5. 实现上传会话和媒体资产 repository/service。
6. 写 probe/derivative RED，覆盖伪装扩展名、损坏容器、受限 codec、图片缩略图、视频 poster/hover/proxy、音频 waveform/proxy 和处理重试幂等。
7. 实现受限 ffprobe/FFmpeg runner，使用 context timeout、`-nostdin`、网络协议禁用、隔离临时目录和输出限额。
8. 扩展对象清理受控 prefix，并在物理删除前重查活动引用。
9. 运行 `go test ./internal/storage ./internal/service/mediaasset ./internal/service/mediaprocess ./internal/service/objectcleanup -count=1` 并提交：`feat(media): add resumable upload and media processing`。

## Task 4：统一资产 API、历史图片投影与回填

**Files:**
- Create: `internal/http/handlers/media_assets.go`
- Create: `internal/http/handlers/media_uploads.go`
- Create: `internal/http/router/media_assets_api_test.go`
- Create: `internal/http/router/media_uploads_api_test.go`
- Create: `internal/repository/db/media_asset_backfill.go`
- Create: `internal/repository/db/media_asset_backfill_test.go`
- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/service/assets/service.go`
- Modify: `internal/service/assets/media_access.go`
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/api.go`
- Modify: `api/openapi/openapi.yaml`
- Modify: `api/openapi/routes.go`

**Steps:**

1. 写 API RED，覆盖 list/detail/patch/delete/transfer/retry/access、游标/筛选/排序、owner 隔离、已删除但被画布引用访问和批量逐项结果。
2. 实现 Media Service handler 与 router，复用统一 envelope 和 request ID；access 只返回短时 URL/Range 能力。
3. 写上传 API RED 并实现 init/status/sign-parts/local-part/complete/abort，限制 body 和 part number。
4. 写历史投影 RED：未回填 `task_images` 在统一列表可读，ID 等于原结果 UUID，项目取结果自身；回填不读/复制对象且可 checkpoint/retry。
5. 实现兼容投影、幂等 backfill 与新图片结果事务双写；保留旧 gallery/reference API。
6. 更新 OpenAPI 和 route/method/CORS contract tests。
7. 运行相关 Go 测试与 API smoke 子集，提交：`feat(api): expose unified media assets and uploads`。

## Task 5：视频配置、能力、价格与报价 API

**Files:**
- Create: `internal/service/videorouting/store.go`
- Create: `internal/service/videorouting/service.go`
- Create: `internal/service/videorouting/*_test.go`
- Create: `internal/service/videopricing/store.go`
- Create: `internal/service/videopricing/service.go`
- Create: `internal/service/videopricing/*_test.go`
- Create: `internal/service/videotask/quote.go`
- Create: `internal/service/videotask/quote_test.go`
- Create: `internal/repository/entstore/video_config_store.go`
- Create: `internal/http/handlers/video.go`
- Create: `internal/http/router/video_capability_api_test.go`
- Create: `internal/http/router/video_estimate_api_test.go`
- Modify: `internal/domain/modeladmin/types.go`
- Modify: `internal/service/modeladmin/service.go`
- Modify: `internal/repository/entstore/modeladmin_store.go`

**Steps:**

1. 写配置 RED：capability JSON 强类型校验、真实模型成本规则版本、视频路由默认/可见组合、最大输出 1-4、缺候选/缺价/低于安全线不能启用。
2. 实现视频路由和价格 Service，按所有可用候选最大成本计算保护线；读取有效积分商品推导净积分收入下限。
3. 写 quote RED：规范化 fingerprint、HMAC、120s TTL、能力/价格版本、篡改/过期/旧组合拒绝、decimal 五位精度。
4. 实现用户 capability/estimate API；返回候选并集，但每次组合验证必须存在完整候选。
5. 写并实现 Ops video capability/cost/pricing/simulate/recalculate/route config/impact API 和审计。
6. 更新 OpenAPI、共享类型和管理端 API client contract。
7. 运行 `go test ./internal/domain/video ./internal/service/videorouting ./internal/service/videopricing ./internal/service/videotask ./internal/http/router -count=1`。
8. 提交：`feat(video): add capability routing and safe pricing`。

## Task 6：视频 Provider、任务执行、转存和结算

**Files:**
- Create: `internal/provider/video/contracts.go`
- Create: `internal/provider/video/errors.go`
- Create: `internal/provider/video/seedance/client.go`
- Create: `internal/provider/video/seedance/client_test.go`
- Create: `internal/provider/video/minimax/client.go`
- Create: `internal/provider/video/minimax/client_test.go`
- Create: `internal/service/videotask/store.go`
- Create: `internal/service/videotask/service.go`
- Create: `internal/service/videotask/service_test.go`
- Create: `internal/repository/entstore/video_task_store.go`
- Create: `internal/repository/entstore/video_task_store_test.go`
- Create: `internal/worker/video/runner.go`
- Create: `internal/worker/video/runner_test.go`
- Create: `internal/http/handlers/video_callbacks.go`
- Create: `internal/http/router/video_tasks_api_test.go`
- Create: `internal/http/router/video_callbacks_api_test.go`
- Modify: `internal/worker/runner.go`
- Modify: `internal/app/worker.go`

**Steps:**

1. 为 Seedance/MiniMax 写 golden RED：submit/get/cancel/callback challenge+verify/status/error/usage，未知 option 拒绝，context timeout 生效。
2. 实现两个 adapter；真实 endpoint/model/capability 只来自已验证配置，不在请求时猜测。
3. 写 task RED：quote+Idempotency-Key 创建 1-4 item、预留同事务；同 key 同 body返回原任务，不同 body 409。
4. 实现 task create/list/detail/cancel/SSE projection，复用提示词 resolver 并保存模板/绑定/执行快照。
5. 写 Worker RED：步骤级租约、submit unknown -> reconcile、due poll、单调状态、callback/poll 竞态、取消晚于成功、artifact host/redirect/size/checksum 校验。
6. 实现视频角色 runner，等待 Provider 时持久化 `next_action_at` 并释放租约；转存使用 `PutReader`，不把正文读入内存。
7. 写结算 RED：0 成功全退、部分成功逐项收费、重复终态只 finalize 一次、转存未完成不 finalize、不可恢复转存全退并记录平台成本。
8. 实现 finalize、账本 `task_media_type=video` 与 `usage_summary`，派生处理与生成终态分离。
9. 实现回调 handler 3 秒内验签/幂等落库，不在 handler 下载或结算。
10. 运行 Provider、Service、Worker、router 和 billing tests，提交：`feat(video): execute provider jobs and settle successful outputs`。

## Task 7：快捷视频创作前端

**Files:**
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Create: `web/user/src/features/creation/CreationPage.tsx`
- Create: `web/user/src/features/creation/ImageCreationPanel.tsx`
- Create: `web/user/src/features/creation/VideoCreationPanel.tsx`
- Create: `web/user/src/features/creation/videoDraft.ts`
- Create: `web/user/src/features/creation/*.contract.ts`
- Modify: `web/user/src/pages/WorkspacePage.tsx`
- Modify: `web/user/src/App.tsx`
- Modify: `web/user/src/routeState.ts`
- Modify: `web/user/src/styles.css`

**Steps:**

1. 写 TS contract RED：图片/视频模式记忆、从资产定向进入、能力 reducer 只重置失效字段且列出变化、旧 quote 失效、错误保留草稿。
2. 实现媒体模式壳并保持图片默认及现有深链；拆出图片面板但不改变图片生成 wire contract。
3. 实现视频模型分组、生成方式、首尾帧、提示词/变量复用、时长/比例/清晰度/音频/数量和服务端报价。
4. 实现 1-4 item 任务、真实阶段、取消、详情、结果、费用核对和复用参数不带变量值/首帧。
5. 用平台现有样式、ModelGroupSelect、Lucide、Tooltip、Toast 和响应式规则完成桌面/手机体验。
6. 运行 user typecheck/build 与所有 creation contract tests；回归现有 Workspace contract。
7. 提交：`feat(web): add quick video creation workflow`。

## Task 8：创意画布后端与生成编排

**Files:**
- Create: `internal/service/canvas/store.go`
- Create: `internal/service/canvas/service.go`
- Create: `internal/service/canvas/service_test.go`
- Create: `internal/repository/entstore/canvas_store.go`
- Create: `internal/repository/entstore/canvas_store_test.go`
- Create: `internal/http/handlers/canvases.go`
- Create: `internal/http/router/canvas_api_test.go`
- Modify: `internal/service/project/service.go`
- Modify: `internal/service/project/store.go`
- Modify: `internal/repository/entstore/project_store.go`

**Steps:**

1. 写 CRUD RED：按项目列表、创建模板、重命名、复制、软删、预览摘要和 owner 隔离。
2. 写 document RED：expected revision 成功/409、规范化 JSON 上限、服务端重建 asset references、周期快照。
3. 实现 Canvas Service/store/API；禁止信任客户端引用计数或隐藏输入。
4. 写生成 run RED：从当前服务端节点读取输入、主动生成当前节点、稳定 attach、重复 attach 幂等、生成节点已删 -> unplaced、恢复不再计费。
5. 接入现有图片生成 port 和新视频任务 port，实现结果资产自动附着与未附着 run 查询。
6. 写项目转移 RED：锁元数据行、running_task_count 与活动 run 双校验、只转画布归属、不迁移引用/任务/资产；项目删除复用同一约束。
7. 实现 SSE/轮询状态映射和 running count 对账。
8. 更新 OpenAPI 并运行 canvas/project/router tests，提交：`feat(canvas): add persisted canvases and generation runs`。

## Task 9：创意画布前端

**Files:**
- Modify: `web/user/package.json`
- Modify: `web/user/package-lock.json`
- Create: `web/user/src/features/canvas/CanvasListPage.tsx`
- Create: `web/user/src/features/canvas/CanvasEditorPage.tsx`
- Create: `web/user/src/features/canvas/core/*`
- Create: `web/user/src/features/canvas/nodes/*`
- Create: `web/user/src/features/canvas/store/*`
- Create: `web/user/src/features/canvas/persistence/*`
- Create: `web/user/src/features/canvas/*.contract.ts`
- Modify: `web/user/src/App.tsx`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/routeState.ts`
- Modify: `web/user/src/styles.css`

**Steps:**

1. 引入固定版本 Zustand/localForage/Dagre；只移植 Nova 授权范围内 DOM/SVG/Pointer 核心和必要依赖，不复制其 API、存储或视觉。
2. 写 store/graph RED：命令式 undo/redo、复制粘贴、框选、合法连接、循环拒绝、视口、稳定 attach 与 revision conflict。
3. 实现 Canvas store、CSS world transform、SVG edges、Pointer Events 和 IndexedDB 草稿；媒体 URL 不写入文档/undo。
4. 实现列表、空白/图片探索/图片转视频模板、七类节点、资产抽屉、主动估价/生成、取消、任务恢复和结果附着。
5. 实现小地图、节点搜索、自动整理选中节点、适应视图、工具 tooltip 和快捷键焦点隔离。
6. 实现手机只读与平板横屏完整编辑；触控命中 >=44px、双指缩放/平移、长按菜单和软键盘可视区域。
7. 写 Playwright/contract 验收：200 节点/300 边/50 媒体、nonblank canvas pixel、拖拽/框选/连线、两标签冲突、离线恢复、暗亮主题、手机和平板横屏。
8. 运行 user tests/typecheck/build 并提交：`feat(web): add project-based creative canvas`。

## Task 10：统一资产前端与上传托盘

**Files:**
- Create: `web/user/src/features/media/MediaAssetsPage.tsx`
- Create: `web/user/src/features/media/MediaAssetCard.tsx`
- Create: `web/user/src/features/media/MediaPreviewDialog.tsx`
- Create: `web/user/src/features/media/UploadTray.tsx`
- Create: `web/user/src/features/media/uploadManager.ts`
- Create: `web/user/src/features/media/*.contract.ts`
- Modify: `web/user/src/pages/GalleryPage.tsx`
- Modify: `web/user/src/App.tsx`
- Modify: `web/user/src/components.tsx`
- Modify: `web/user/src/styles.css`

**Steps:**

1. 写列表 RED：项目/类型/来源/分组/状态/搜索/排序/刷新与游标；历史图片和新媒体统一 projection。
2. 写上传 RED：多文件逐项校验、multipart 并发/重试/恢复/取消、Local chunks、页面切换后托盘状态、失败不影响合法文件。
3. 实现统一卡片与详情：图片 thumbnail/preview，视频 poster + 200ms hover + cancel + 全局并发 2 + MP4 proxy，音频 waveform + 单实例按需播放。
4. 实现重命名、分组、项目转移、删除、下载、继续创作、添加到现有画布。
5. 扩展批量全选/反选/下载/分组/转移/删除，逐项失败保留选择；公开仅适用于图片。
6. 写网络 contract，证明列表不请求原件、手机不 hover、签名过期刷新一次。
7. 运行 user typecheck/build 和 gallery/image 回归，提交：`feat(web): unify multimedia assets and local uploads`。

## Task 11：管理后台、配置联动与就绪检查

**Files:**
- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/admin-api.ts`
- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Create: `web/admin/src/pages/VideoTasksPage.tsx`
- Create: `web/admin/src/pages/MediaPolicyPage.tsx`
- Create: `web/admin/src/pages/video*.contract.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/layout/admin-navigation.ts`
- Modify: `internal/http/handlers/health.go`
- Modify: `internal/app/storage_validation.go`

**Steps:**

1. 写后台 contract RED，覆盖真实模型 capability/cost、价格保护参数/规则/模拟/重算、路由 visible combination/impact、独立版本和互相跳转摘要。
2. 实现三个既有板块的视频扩展；价格低于安全线、缺价、积分商品净收入不足时明确阻止启用。
3. 实现视频任务运营页：筛选、attempt/usage/cost/settlement、只重试转存/派生、手动刷新。
4. 实现媒体策略页：格式/大小/时长/配额/派生/保留期，并明确变更只影响新对象/新版本。
5. 扩展 readiness：视频路由/价格、存储读写签名、媒体 Worker/FFmpeg、转存/派生/结算积压；风险只关闭视频/上传/画布入口，不破坏图片。
6. 运行 admin typecheck/build、Go readiness/router tests，提交：`feat(admin): manage video pricing routing and media operations`。

## Task 12：Worker 角色、部署、观测与迁移硬化

**Files:**
- Modify: `internal/config/runtime_schema.go`
- Modify: `internal/config/runtime_components.go`
- Modify: `internal/app/worker_bootstrap.go`
- Modify: `cmd/worker/main.go`
- Modify: `Dockerfile.worker`
- Modify: `deployments/docker-compose/docker-compose.prod.yml`
- Modify: `deployments/docker-compose/docker-compose.local.yml`
- Modify: `deployments/devops/env/frontend.env.example`
- Modify: `internal/app/observability/metrics.go`
- Modify: `deployments/monitoring/prometheus.yml`
- Modify: `internal/mgsctl/doctor.go`
- Modify: `internal/mgsctl/doctor_test.go`
- Create: `docs/ops/multimedia-operations.md`

**Steps:**

1. 写配置 RED：`WORKER_ROLES=image,video,media,cleanup`、独立并发、FFmpeg/ffprobe、temp dir/disk 水位，未知 role/无工具明确失败。
2. 实现角色启动和公平领取，确保视频 poll 与媒体转码不占图片槽；Redis 故障时停止无法安全限流的视频新领取。
3. 添加视频阶段、转存、结算、上传、派生、画布保存、临时盘和对象字节指标及脱敏结构化日志。
4. 扩展 Docker/native/mgsctl doctor 和 compose；确认 full/single 默认启用全部角色且数据卷明确。
5. 实现对账：超时 Provider、上游成功无资产、终态未结算、媒体对象/派生、canvas run、过期 multipart；所有修复幂等且不重新生成。
6. 做历史图片 backfill dry-run/中断恢复/抽样核对与 expand-first rollback 演练。
7. 运行 config/app/mgsctl/deployment contract tests，提交：`feat(ops): harden multimedia workers and observability`。

## Task 13：全链路测试、视觉验收和发布

**Files:**
- Modify: `scripts/workflow/api-smoke.sh`
- Create: `scripts/e2e/multimedia-e2e.mjs`
- Create: `scripts/e2e/fake-video-provider.*`
- Create: `docs/reviews/multimedia-phase1-visual-acceptance.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Steps:**

1. 增加 fake Seedance/MiniMax、MinIO Multipart、媒体处理、画布生成与计费对账 smoke；覆盖重复 callback/poll、部分成功、转存恢复和同 key 幂等。
2. 回归固定积分包有效期：后台开关开启时要求正整数天数，关闭时隐藏并忽略天数；订单快照不可变；购买/赠送 grant 同为永久或同一到期时间；自定义充值永久；用户订单与余额展示快照结果；视频预留/结算不改变 grant 到期时间。
3. 执行 1 GiB S3/MinIO 与 Local 上传测试，记录 API RSS、临时盘、断点/取消/清理；不把 1 GiB fixture 提交仓库。
4. 执行 100/500 estimate、1000 due poll、50 callback 竞态、FFmpeg 资源水位和 Canvas 200/300 性能测试。
5. 启动本地服务，使用 Playwright 验收桌面、手机、平板横屏、暗亮主题；截图并做画布 nonblank/canvas-pixel 检查，修复重叠、溢出、不可操作和媒体误加载。
6. 完整执行：

   ```bash
   ./scripts/workflow/verify.sh
   ./scripts/workflow/review-local.sh --scope committed
   ./scripts/workflow/check-review-gate.sh
   ./scripts/workflow/api-smoke.sh
   ```

7. 对照 PRD 25.1-25.4、2.6 与技术方案 5.6、1.6 逐项记录证据；不得以部分实现宣称完成。
8. 提交最终测试/文档批次，重新运行 committed-scope review marker，确保 tree SHA 与 HEAD 一致。
9. 推送 `codex/multimedia-creation-phase1`，创建面向 `main` 的 ready PR，等待 CI 全绿并完成 review。
10. 合并 PR 到 `main`，拉取远端 merge commit，再创建下一个语义版本 tag（基于 `v0.0.12` 应为 `v0.0.13`，若合并前已有新 tag 则递增最新 patch）。
11. 推送 tag，等待并验证 release Action、镜像和 release manifest 均指向该 tag；只有发布成功才将目标标记完成。
