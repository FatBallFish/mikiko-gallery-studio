# Pic Gallery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 从零搭建可部署的图片生成平台 MVP，覆盖用户登录、文生图/图生图、积分计费、OpenAI 兼容接口、OpenRouter/OpenAI 双平台路由、管理后台和开发文档页。

**Architecture:** 采用 contract-first + domain-first 的实现方式。先固化仓库骨架、配置体系、OpenAPI 协议和基础领域模型，再把后端按认证、计费、资产、任务、Provider、管理后台拆成低耦合模块，同时让用户 Web、管理后台 Web、开发文档页尽早基于接口契约并行开发。部署上默认支持单机 Docker Compose，并在实现层保留 API 集群 + Worker 集群 + Redis 共享状态的能力。

**Tech Stack:** Go + Gin + Ent + PostgreSQL + Redis + Local FS/S3-compatible + React + Vite + React Router + TanStack Query + Zustand + OpenAPI 3.1

---

> **Progress Note (2026-05-20):** 本文档继续保留为原始任务拆解与依赖关系说明；当前实际完成度请以 `docs/plans/2026-05-20-project-progress-review.md` 为准。

## 0. 执行原则

- 先做接口契约和共享模型，再做页面和业务逻辑，减少前后端反复返工。
- 所有积分、倍率、价格字段统一按 `numeric(20,5)` 思维设计，避免后续二次迁移。
- 图片生成链路统一落到“参考图资产 -> 任务 -> 执行 -> 结果图片 -> 历史记录”模型，不分散实现。
- `OpenAI compat`、`Open API`、`Web API` 共用同一套内部 service，不做三套业务逻辑。
- 配置中心、能力矩阵、错误码策略都走数据化配置，避免把产品规则写死在 handler。

## 1. 推荐并行方式

### Track A：基础设施与公共底座
- T01-T06

### Track B：核心后端领域
- T07-T15

### Track C：用户 Web
- T16-T19

### Track D：管理后台 Web
- T20-T22

### Track E：部署、验证与交付
- T23-T26

## 2. 最小依赖图

- T01 是所有任务起点。
- T02/T03/T04/T05/T06 在 T01 后可并行。
- T07/T08/T09/T10 依赖 T02-T06，可并行推进。
- T11/T12 依赖 T09/T10。
- T13/T14/T15 依赖 T07-T12。
- T16/T17 依赖 T05，可先用 mock 数据开发；接入真实 API 依赖 T13/T14。
- T18/T19 依赖 T05 + T13/T14。
- T20/T21/T22 依赖 T05，可先用 mock；接入真实 API 依赖 T15。
- T23/T24 可在 T01 后启动，和主业务并行。
- T25/T26 放在主链路联调后执行。

## 3. 任务清单

### T01 仓库骨架与工程约定

**目标**
- 建立后端、用户 Web、管理后台 Web、部署、脚本、文档的标准目录，形成统一开发入口。

**建议目录**
- `cmd/api/`
- `internal/app/`
- `internal/domain/`
- `internal/service/`
- `internal/repository/`
- `internal/provider/`
- `internal/http/`
- `internal/worker/`
- `internal/config/`
- `pkg/`
- `api/openapi/`
- `deployments/docker-compose/`
- `deployments/nginx/`
- `web/user/`
- `web/admin/`
- `docs/plans/`

**步骤**
1. 初始化 Go module、基础 Makefile、`.env.example`、`README` 开发章节。
2. 初始化后端分层目录与空 package。
3. 初始化 `web/user` 和 `web/admin` 两个 Vite 工程。
4. 约定统一脚本入口：`make dev`、`make test`、`make lint`、`make openapi`。
5. 写清楚目录责任，避免后续把 domain/service/http 混写。

**完成标志**
- 新同学 clone 后能一眼知道各模块放在哪里。

### T02 本地开发依赖与容器编排

**目标**
- 提供一套本地可跑的基础设施：PostgreSQL、Redis、对象存储、邮件捕获、可选的反向代理。

**文件**
- `deployments/docker-compose/docker-compose.dev.yml`
- `deployments/docker-compose/.env.example`
- `scripts/dev/up.sh`
- `scripts/dev/down.sh`

**步骤**
1. 编排 PostgreSQL、Redis、MinIO、Mailpit/Mailhog。
2. 预留 API、user-web、admin-web 服务位。
3. 补齐健康检查、默认端口、持久卷和初始化账号。
4. 记录本地访问地址与默认账号。

**依赖**
- T01

### T03 配置体系与环境变量模型

**目标**
- 建立后端配置加载、配置分类和默认值机制，支撑后续“配置中心 + 本地 env + 部署覆盖”三层配置。

**文件**
- `internal/config/*.go`
- `configs/config.dev.yaml`
- `configs/config.example.yaml`

**步骤**
1. 设计配置结构：server、db、redis、storage、auth、billing、provider、routing、docs。
2. 明确“静态配置”和“可运营动态配置”的边界。
3. 动态配置定义存储键空间，和后台 Tab 分类一一对应。
4. 预埋 `auto_quality_default_by_group`、用户分组倍率、错误策略等默认项。

**依赖**
- T01

### T04 通用协议、错误码与响应封装

**目标**
- 统一 Web API、Open API、OpenAI compat 的响应封装、错误结构、request id 和审计字段。

**文件**
- `pkg/errs/`
- `pkg/httpx/`
- `internal/http/middleware/`
- `api/openapi/components/*.yaml`

**步骤**
1. 设计平台错误码枚举，包括鉴权、参数、余额、任务、Provider、审核、存储等。
2. 定义上游错误分类：自动重试、包装透传、直出、平台内部错误。
3. 统一日志和响应中的 `request_id`、`trace_id`、`code`、`message`、`details`。
4. 预留 OpenAI compat 的错误适配层。

**依赖**
- T01

### T05 OpenAPI 契约基线

**目标**
- 先把协议定出来，让前后端、文档页、测试都能围绕统一契约推进。

**文件**
- `api/openapi/openapi.yaml`
- `api/openapi/paths/*.yaml`
- `api/openapi/components/schemas/*.yaml`

**步骤**
1. 先覆盖 P0 契约：auth、profile、balance、estimate、reference-assets、tasks、history、api-keys、admin config、OpenAI compat。
2. 把 `auto` 分辨率、图片数量、参考图数量、5 位小数积分、倍率字段放进 schema。
3. 为错误码、分页、幂等头、鉴权头定义通用组件。
4. 预留 P1 图片广场和审核接口。

**依赖**
- T01、T04

### T06 可观测性底座

**目标**
- 从一开始就统一日志、metrics、health、pprof、trace 埋点入口。

**文件**
- `internal/app/observability/`
- `internal/http/handlers/health.go`
- `deployments/monitoring/`

**步骤**
1. 接入 structured logging。
2. 暴露 `/healthz`、`/readyz`、`/metrics`。
3. 约定关键指标：任务成功率、Provider 失败率、自动重试命中率、积分扣减失败率、refresh 重放次数。
4. 预留集群 worker lease 与任务队列指标。

**依赖**
- T01

### T07 数据模型与 Ent Schema

**目标**
- 一次性把核心表结构搭稳，避免中途大改。

**覆盖对象**
- `users`
- `user_groups`
- `admin_users`
- `refresh_sessions`
- `api_keys`
- `redeem_codes`
- `point_ledgers`
- `model_providers`
- `model_routes`
- `provider_error_policies`
- `config_items`
- `reference_assets`
- `image_tasks`
- `image_results`
- `audit_logs`

**步骤**
1. 将 PRD/技术方案里的字段完整映射到 Ent Schema。
2. 所有积分/倍率字段统一 `numeric(20,5)`。
3. 给任务表补齐 `requested_quality`、`resolved_quality_bucket`、`requested_output_image_count`、`reference_image_count`、`lease_owner`、`lease_expires_at`。
4. 生成 migration 与初始化种子数据。

**依赖**
- T02、T03

### T08 认证与会话域

**目标**
- 实现邮箱验证码登录、双 Token、静默续期、回跳和 replay block。

**文件**
- `internal/domain/auth/`
- `internal/service/auth/`
- `internal/http/agent/auth/`

**步骤**
1. 实现验证码发送、验证码校验、用户自动注册。
2. 实现 Access Token 10 分钟、Refresh Token 2 小时的签发与轮换。
3. 实现 refresh family、重放检测、注销和 token_version 吊销。
4. 输出前端需要的 cookie/header 约定。

**依赖**
- T04、T07

### T09 计费、余额与用户分组倍率域

**目标**
- 把价格规则独立成纯领域模块，避免散落在 handler / worker / 前端。

**文件**
- `internal/domain/billing/`
- `internal/service/billing/`
- `internal/http/agent/billing/`
- `internal/http/open/billing/`

**步骤**
1. 实现价格规则解析：模型分组、质量桶、任务类型、参考图附加、输出张数、用户分组倍率。
2. 落地完整费用伪公式为可执行代码。
3. 实现预估、预扣、结算、失败返还、部分成功按成功张数结算。
4. 实现余额查询、流水查询、兑换码核销。

**依赖**
- T03、T04、T07

### T10 参考图资产与对象存储域

**目标**
- 先把图生图链路里最容易漏掉的“参考图资产”打通。

**文件**
- `internal/domain/assets/`
- `internal/service/assets/`
- `internal/repository/storage/`
- `internal/http/agent/assets/`
- `internal/http/open/assets/`

**步骤**
1. 支持 Agent API 中转上传。
2. 支持 Open API 预签名上传会话与中转上传双模式。
3. 实现 hash 去重、格式/尺寸校验、用户归属校验、状态查询。
4. 为 OpenAI `edits` multipart 文件转存成内部 `reference_assets`。

**依赖**
- T02、T03、T07

### T11 模型能力矩阵、路由与配置域

**目标**
- 将“可选模型、支持的任务类型、支持的质量/尺寸/图片数”做成动态能力矩阵。

**文件**
- `internal/domain/modelhub/`
- `internal/service/modelhub/`
- `internal/http/agent/capabilities/`
- `internal/http/open/capabilities/`

**步骤**
1. 定义抽象模型组、具体 provider model、优先级、AB、降级链。
2. 为每个 route 建能力矩阵：text_to_image、image_edit、reference_generate、mask 支持、输入图上限、输出图上限。
3. 输出前端可直接消费的 `/capabilities`。
4. 支持 `auto` 分辨率默认值按用户分组/模型分组解析。

**依赖**
- T03、T07

### T12 Provider 抽象层：OpenAI + OpenRouter

**目标**
- 用统一 Provider 接口封装 OpenAI 和 OpenRouter，避免后续继续分叉。

**文件**
- `internal/provider/contracts.go`
- `internal/provider/openai/`
- `internal/provider/openrouter/`
- `internal/provider/normalize/`

**步骤**
1. 定义统一请求结构：任务类型、prompt、参考图、mask、质量、尺寸、数量。
2. 实现 OpenAI 图片生成/编辑调用。
3. 实现 OpenRouter 多模态 `chat/completions` 转换。
4. 统一响应归一：结果图 URL、base64、usage、finish 状态、错误对象。
5. 实现上游错误策略分类与自动重试判定。

**依赖**
- T04、T10、T11

### T13 任务编排、状态机与 Worker 集群

**目标**
- 建立统一图片任务域，支持同步等待、异步轮询、集群抢占和失败补偿。

**文件**
- `internal/domain/imagetask/`
- `internal/service/imagetask/`
- `internal/worker/imagetask/`
- `internal/repository/queue/`
- `internal/http/agent/tasks/`
- `internal/http/open/tasks/`

**步骤**
1. 实现任务状态机：queued/running/succeeded/partial_failed/failed/cancelled。
2. 实现创建任务 -> 预扣积分 -> 入队/调度 -> 执行 -> 存图 -> 结算。
3. 实现 lease、heartbeat、超时回收、补偿扫描。
4. 支持同步等待超时后回落异步查询。
5. 支持历史记录、删除历史、结果下载签名。

**依赖**
- T07、T09、T10、T11、T12

### T14 Open API 与 OpenAI Compat 接入层

**目标**
- 把平台原生 API 和 OpenAI 兼容接口都接入到统一 task service。

**文件**
- `internal/http/open/`
- `internal/http/openai/`
- `internal/service/compat/`

**步骤**
1. 实现 AK/SK 鉴权、签名校验、幂等键校验。
2. 实现 `/api/open/image/v1/*` 的任务、预估、余额、能力接口。
3. 实现 `/v1/images/generations`、`/v1/images/edits`、`/v1/models`。
4. 完成 OpenAI 协议到内部模型、再到 OpenRouter/OpenAI Provider 的双向转换。
5. 对 OpenAI compat 返回结构做标准化，包括 `b64_json` 和 `url`。

**依赖**
- T08、T09、T10、T11、T12、T13

### T15 管理后台后端能力

**目标**
- 先补足运营与超管的服务端能力，保证 Web 后台不是空壳。

**文件**
- `internal/domain/admin/`
- `internal/service/admin/`
- `internal/http/ops/`

**步骤**
1. 实现管理员登录和独立会话体系。
2. 实现用户管理、用户分组倍率调整、人工调积分。
3. 实现 Provider、路由、价格表、错误策略、配置 Tab 的 CRUD。
4. 实现任务记录查询、成本分析、审计日志、大盘聚合。
5. 预留 P1 图片审核接口。

**依赖**
- T07、T08、T09、T11、T12、T13

### T16 用户 Web 工程底座

**目标**
- 提前搭好用户侧前端壳子，让功能页能并行开发。

**文件**
- `web/user/src/app/`
- `web/user/src/routes/`
- `web/user/src/components/`
- `web/user/src/lib/api/`
- `web/user/src/lib/query/`
- `web/user/src/store/`

**步骤**
1. 初始化路由、布局、请求层、鉴权拦截、主题和通知系统。
2. 实现 Access Token 失效后的单飞 refresh。
3. 实现登录回跳态保存与恢复。
4. 建立 UI 基础组件：表单、表格、卡片、抽屉、上传器、代码块。

**依赖**
- T01、T05

### T17 用户 Web：认证、账户、API Key

**目标**
- 完成登录闭环和账户中心的非生图页面。

**页面**
- `/`
- `/auth/login`
- `/account/billing`
- `/account/api-keys`

**步骤**
1. 首页与登录页联调验证码登录。
2. 完成余额、流水、用户分组倍率展示。
3. 完成 API Key 创建、复制、禁用、删除、重置。
4. 补齐 401/refresh/回跳/失效重登体验。

**依赖**
- T08、T09、T14、T16

### T18 用户 Web：生成工作台与历史图库

**目标**
- 完成最核心的用户侧生图体验。

**页面**
- `/workspace/generate`
- `/history`

**步骤**
1. 接入 `/capabilities` 动态渲染模型组、质量、数量和参考图规则。
2. 实现参考图上传、mask 上传、价格预估、提交任务。
3. 实现同步等待 + 异步轮询 + 结果展示。
4. 实现历史筛选、详情、下载、删除、公开申请。
5. 针对 `auto` 分辨率、数量上限、图生图计费差异做前端提示。

**依赖**
- T10、T11、T13、T16

### T19 用户 Web：开发文档页与 P1 广场壳子

**目标**
- 让开发文档页成为正式交付物，而不是仅依赖 swagger 文件。

**页面**
- `/developers/docs`
- `/gallery`（P1）

**步骤**
1. 读取 OpenAPI 规范并按接口分类展示。
2. 补充接口用途、鉴权方式、参数说明、错误码说明、示例代码 Tab。
3. 为 OpenAI compat、Open API、Agent API 做明显分区。
4. 预留 P1 图片广场页面壳子和瀑布流组件。

**依赖**
- T05、T14、T16

### T20 管理后台 Web 工程底座

**目标**
- 独立初始化后台前端工程和后台权限框架。

**文件**
- `web/admin/src/app/`
- `web/admin/src/routes/`
- `web/admin/src/components/`
- `web/admin/src/lib/api/`

**步骤**
1. 初始化后台布局、登录拦截、菜单权限。
2. 建立表格、表单、图表、JSON 配置编辑器等后台通用组件。
3. 预留超级管理员/运营管理员角色路由保护。

**依赖**
- T01、T05

### T21 管理后台 Web：配置与运营页面

**目标**
- 把“可配置”真正落到页面，而不是停留在数据库里。

**页面**
- `/admin/users`
- `/admin/model-routing`
- `/admin/pricing`
- `/admin/config`

**步骤**
1. 用户管理与用户分组倍率编辑。
2. Provider、路由、AB、降级策略编辑。
3. 价格矩阵、参考图附加倍率、`auto` 默认档位配置。
4. 配置中心按分类 Tab 展示并支持 typed form / JSON 双模式。

**依赖**
- T15、T20

### T22 管理后台 Web：任务、监控、审核页面

**目标**
- 把运维侧与内容运营侧的可见性补齐。

**页面**
- `/admin/dashboard`
- `/admin/tasks`
- `/admin/audits`
- `/admin/reviews`（P1）

**步骤**
1. 任务查询、错误聚合、成本视图。
2. 稳定性与运营大盘图表。
3. 审计日志查询。
4. P1 图片公开审核队列。

**依赖**
- T15、T20

### T23 集群部署与发布资产

**目标**
- 在代码层之外把部署方式真正交付出来。

**文件**
- `deployments/docker-compose/docker-compose.prod.yml`
- `deployments/k8s/` 或 `deployments/compose-cluster/`
- `deployments/nginx/`

**步骤**
1. 提供单机 Compose 版部署。
2. 提供集群部署示意：API 多副本、Worker 多副本、共享 Redis/PostgreSQL、对象存储外置。
3. 写明环境变量、滚动发布、回滚和扩缩容说明。
4. 配置健康检查、日志输出、资源建议值。

**依赖**
- T02、T03、T06、T13

### T24 测试资产与 Provider Mock

**目标**
- 尽早把难测部分标准化，避免后期全靠人工点页面。

**文件**
- `internal/provider/mock/`
- `tests/integration/`
- `tests/e2e/`
- `scripts/test/`

**步骤**
1. 为 OpenAI、OpenRouter 构造 mock server。
2. 覆盖 `auto` 分辨率、图片数量、图生图计费、错误策略、refresh 轮换的集成测试。
3. 为用户 Web 和管理后台准备基础 E2E 脚本。
4. 为对象存储和 worker lease 增加集成验证。

**依赖**
- T12、T13、T14、T15、T16、T20

### T25 联调与验收回归

**目标**
- 按 PRD A 系列/B 系列验收项做一轮端到端收口。

**步骤**
1. 联调登录、续期、回跳。
2. 联调文生图、图生图、参考图、部分成功、失败返还。
3. 联调 Open API 与 OpenAI compat SDK 调用。
4. 联调后台配置变更对新任务即时生效。
5. 验收开发文档页内容完整性。

**依赖**
- T17-T24

### T26 上线准备与交付文档

**目标**
- 形成真正可交付包，而不是“代码能跑”。

**文件**
- `README.md`
- `docs/runbooks/`
- `docs/release/`
- `docs/ops/`

**步骤**
1. 输出启动文档、配置文档、部署文档、回滚文档。
2. 输出错误码手册、Provider 接入说明、开发文档维护说明。
3. 输出首版已知限制清单：P1 能力、Provider 差异、容量上限。

**依赖**
- T23、T24、T25

## 4. 推荐实施顺序

### Phase 0：先把“能并行”的底座铺开
- T01 -> T04/T05/T06
- 同时启动 T02、T03

### Phase 1：核心后端分 5 条线并行
- 线 A：T07 数据模型
- 线 B：T08 认证会话
- 线 C：T09 计费结算
- 线 D：T10 参考图资产
- 线 E：T11 模型路由

### Phase 2：把生成主链路闭环
- T12 Provider
- T13 任务编排
- T14 Open API + OpenAI compat

### Phase 3：前端双线并行
- 用户 Web：T16 -> T17/T18/T19
- 管理后台：T20 -> T21/T22

### Phase 4：上线前收口
- T23 部署
- T24 测试
- T25 联调
- T26 交付文档

## 5. 建议的人员并行拆法

- 后端 1：T07 + T08
- 后端 2：T09 + T10
- 后端 3：T11 + T12
- 后端 4：T13 + T14 + T15
- 前端 1：T16 + T17 + T18
- 前端 2：T20 + T21 + T22
- 前端 3/全栈：T19 开发文档页
- QA/平台：T23 + T24 + T25

## 6. 高风险项优先级提醒

- 第一优先：T09 计费域，避免价格公式后置导致全链路返工。
- 第二优先：T10 + T12，图生图/参考图和 OpenRouter 差异是最容易出问题的部分。
- 第三优先：T13，任务状态机、lease 和补偿一旦设计不稳，后面会反复修。
- 第四优先：T14，OpenAI compat 如果后做，接口形态容易和内部模型脱节。

## 7. 建议的验收门槛

- P0 不允许手工改数据库才能跑通主流程。
- 所有价格计算都必须可复现、可审计、可回放。
- 所有上游错误都必须命中明确策略，不能直接 `500` 裸透传。
- 用户 Web 和管理后台都必须跑在最新契约上，不能长期依赖 mock。

