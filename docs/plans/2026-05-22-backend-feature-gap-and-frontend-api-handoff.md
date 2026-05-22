# Pic Gallery 后端功能盘点、需求/技术方案差距与前端对接清单

更新日期：2026-05-22

## 1. 分析范围与判定口径

本次只分析当前仓库后端能力，包括 Go API、Worker、Provider、OpenAPI 契约、Ent schema、持久化 store、部署/观测底座，不评价 `web/user` 与 `web/admin` 前端页面完成度。

对照资料：

- 需求文档：`docs/prd/pic-gallery-prd.md`
- 技术方案：`docs/tech/pic-gallery-tech-design.md`
- OpenAPI 契约：`api/openapi/openapi.yaml`
- 后端入口：`cmd/api/main.go`、`cmd/worker/main.go`
- Runtime 路由：`internal/http/router/router.go`
- 主要 Handler：`internal/http/handlers/api.go`
- 数据模型：`internal/repository/ent/schema/*.go`

完成度定义：

| 标识 | 含义 |
|---|---|
| 已完成 | 已有可运行后端链路，通常包含路由/服务/持久化/测试，能支撑真实联调或后端闭环。 |
| 部分完成 | 主链路或重要子能力已具备，但与 PRD/技术方案相比仍缺产品闭环、管理能力、权限/筛选/边界或动态配置。 |
| 框架已搭建 | schema、配置项、默认数据、接口壳或基础 service 已存在，但还不能独立支撑完整业务场景。 |
| 未开始 | 当前没有可用后端业务实现，或只在 PRD/技术方案中出现。 |

## 2. 总体结论

当前后端已经具备“可跑起来并与前端开始主链路联调”的基础，尤其是用户邮箱验证码登录、双 Token 会话、图片任务创建/查询/历史列表、参考图上传、计费预估/预扣/结算、API Key 管理、Open API、OpenAI 兼容接口、Worker 抢占执行、OpenAI/OpenRouter Provider 适配、生成结果本地落库下载、后台用户/积分、兑换码、调用记录、模型供应商/路由管理等能力。

但它还不是 PRD 中完整的 P0 产品闭环。主要短板集中在：充值/支付/套餐购买、完整 provider model 能力矩阵与成本字段、AB 权重分流、公开审核/图片广场、真正 Redis 化的验证码/限流/会话热缓存、配置中心对 billing/generation limits 的 runtime 热生效、支付订单闭环、完整监控大盘与成本核算。

对前端而言，用户侧可以立即开始联调登录、个人资料、余额/流水、估价、capabilities、参考图、任务、历史、生成图片下载、API Key、兑换码尝试、开发文档拉取。管理后台可以开始联调登录、配置 Tab、审计日志、用户管理、积分调整、兑换码管理、调用记录、模型供应商与路由管理。

本轮 P0 gap fill 更新：

- P0-1 生成结果图片落库与下载：已完成本地 `b64_json` 结果持久化与 owner-scoped 下载；远程 URL 结果会保存并返回 URL，但 P0 暂不主动抓取镜像到本地。
- P0-2 后台用户管理与积分调整：已完成列表、详情、启停、积分调整、幂等与审计。
- P0-3 后台兑换码管理：已完成列表、单个创建、批量创建、状态更新、核销记录与审计。
- P0-4 调用记录真实查询：已改为从 `image_tasks` 读取真实调用记录并支持筛选分页。
- P0-5 模型供应商/路由后台 CRUD 与 runtime 生效：已完成最小 CRUD；DB provider enablement 和 route priority/fallback order 会影响新任务 provider 候选顺序。
- P0-6 原生任务响应模式契约：已明确产品化为 async-only；Agent/Open API 创建任务只接受省略或 `async`，`sync` 返回 400；OpenAI-compatible 接口仍保持同步响应。

## 3. 已完成能力

| 能力 | 当前实现 | 对照 PRD / 技术方案 | 备注 |
|---|---|---|---|
| API 服务基础 | `cmd/api/main.go` 读取配置、打开 DB、自动建表、组装 Ent store 与各 service，挂载 HTTP 路由。 | 覆盖容器化/API 服务基础。 | 生产环境会校验弱 API Key 加密密钥；默认管理员可通过环境变量种子化。 |
| Worker 基础 | `cmd/worker/main.go` / `internal/app/worker.go` 启动任务 worker，按 provider preference 执行任务。 | 对应技术方案“内嵌/独立 worker + DB 抢占”的执行模型。 | 目前未引入 MQ。 |
| 健康与观测基础 | `/`、`/healthz`、`/readyz`、`/metrics`、`/debug/pprof/*`，中间件包含 request id、metrics、recovery、request logger。 | 覆盖部署与稳定性底座的一部分。 | 监控大盘与业务指标聚合仍未完成。 |
| 用户邮箱验证码登录/注册 | `POST /api/agent/auth/v1/email/send-code`、`POST /api/agent/auth/v1/login/email-code`。支持验证码发送/校验、用户创建或登录。 | 覆盖用户模块“邮箱验证码注册/登录”的核心。 | 邮件发送在生产配置上有校验；具体验证码存储实现由 auth service/store 决定。 |
| 双 Token 会话与刷新 | Access JWT + Refresh Cookie，`POST /api/agent/auth/v1/session/refresh` 支持 refresh rotation/replay 检测，`logout` 可失效 refresh。 | 覆盖 PRD 登录会话续期流程和技术方案 Web 会话模型核心。 | Access 放响应体，Refresh 走 HttpOnly Cookie。 |
| 用户资料与偏好 | `GET/PUT /api/agent/user/v1/profile`、`PUT /api/agent/user/v1/preferences`、`POST /api/agent/user/v1/avatar`。 | 覆盖个人资料、偏好设置占位与头像上传基础。 | 修改密码/忘记密码接口存在但返回未实现。 |
| 计费估价 | `GET /api/agent/billing/v1/estimate`、`GET /api/open/image/v1/estimate` 复用 billing calculator。 | 覆盖生成前预估积分。 | 估价基于配置文件中的模型/质量/任务倍率。 |
| 余额与流水查询 | `GET /api/agent/billing/v1/balance`、`GET /api/agent/billing/v1/ledger`，Open API 也有 `GET /api/open/image/v1/balance`。 | 覆盖余额展示和积分流水查询基础。 | 不含套餐、充值来源拆分、过期积分等产品视图。 |
| 生成任务创建与查询 | 用户侧 `POST/GET /api/agent/image/v1/tasks`、`GET /api/agent/image/v1/tasks/{task_id}`；Open API `POST /api/open/image/v1/tasks`、`GET /api/open/image/v1/tasks/{task_id}`。 | 覆盖文生图/图生图/参考图统一任务模型和异步查询基础。 | Agent/Open API 创建已明确 async-only；`response_mode=sync` 返回 400，前端应轮询任务详情。 |
| 历史任务与生成图片下载 | `GET /api/agent/image/v1/history/tasks`、`GET/DELETE /api/agent/image/v1/history/tasks/{task_id}`、`GET /api/agent/image/v1/images/{image_id}`。 | 覆盖历史记录列表、详情、删除与本地生成结果下载基础。 | 本地 `b64_json` 结果可下载；远程 URL 结果 P0 不镜像本地。 |
| 参考图资产 | 用户侧 multipart 上传/获取/下载/删除；Open API 支持 inline base64、multipart 上传与 metadata 获取。 | 覆盖图生图/参考图上传和 API 输入资产基础。 | 支持本地文件系统，支持去重、大小/格式校验、Ent 持久化。 |
| 任务执行状态机 | `imagetask.Service` 支持 queued/running/succeeded/failed/deleted，DB lease、heartbeat、过期抢占、stale owner 防护。 | 覆盖技术方案任务编排核心。 | PRD 中“待校验/已拒绝/部分成功”等状态没有完全显式化。 |
| 预扣/结算/失败退款 | 创建任务时 reserve，执行成功按实际图片数 consume，失败 refund。账务 5 位小数，幂等和并发场景有测试。 | 覆盖积分计费核心规则。 | 充值、套餐、积分过期、支付订单未完成。 |
| API Key 管理 | 用户可创建、列表、详情、更新、启停/撤销、删除、重置 secret；支持 total/daily quota、RPM、过期时间、last_used。 | 覆盖 AK/SK 管理大部分 P0 要求。 | group_code 当前创建/更新不允许用户自定义，实际使用用户组；后台管理 API Key 暂缺。 |
| Open API HMAC 鉴权 | `X-Access-Key`、`X-Timestamp`、`X-Body-SHA256`、`X-Signature`，带 body hash 和时间漂移校验。 | 覆盖 API 开发者安全调用基础。 | GET 请求也需要空 body hash 参与签名。 |
| OpenAI 兼容接口 | `POST /v1/images/generations`、`POST /v1/images/edits`、`GET /v1/models`，Bearer `sk-*` 鉴权。 | 覆盖 OpenAI 兼容图片接口 P0 核心。 | 兼容细节仍是最小可用，不等于完整 SDK 兼容。 |
| Provider 抽象 | OpenAI Images API 与 OpenRouter 多模态图片适配；支持错误归一、provider fallback。 | 覆盖技术方案 Provider 适配核心。 | Provider 健康检查、成本核算、完整 provider model 能力矩阵仍未完成。 |
| Capabilities | 用户侧与 Open API 均可获取能力矩阵。 | 覆盖前端展示模型/任务/质量/数量上限的基础。 | 来源是配置文件，不是后台配置实时生效。 |
| 管理员登录 | 独立 admin 用户体系、bcrypt、失败锁定、JWT admin access token、环境变量种子管理员。 | 覆盖“管理后台与 C 端账号独立”的核心。 | 管理员 refresh/logout session 持久化较轻，后台角色权限粒度未完整展开。 |
| 后台用户管理与积分调整 | `GET /api/ops/admin/v1/users`、`GET /api/ops/admin/v1/users/{user_id}`、`POST /status`、`POST /points-adjustments`。 | 覆盖后台用户查看、禁用/启用、人工调积分 P0 核心。 | 积分调整要求 `Idempotency-Key`；状态变更会递增 `token_version`。 |
| 后台兑换码管理 | `GET/POST /api/ops/admin/v1/redeem-codes`、`POST /api/ops/admin/v1/redeem-codes:batch-create`、状态更新、核销记录。 | 覆盖兑换码运营闭环。 | 批量创建 P0 下非事务全有全无，后续可增强。 |
| 后台调用记录 | `GET /api/ops/admin/v1/call-records` 从真实 `image_tasks` 返回调用记录。 | 覆盖调用记录页基础。 | 成本、毛利、成功率聚合仍未完成。 |
| 后台模型供应商与路由管理 | `model-providers` 与 `model-routes` CRUD，写操作审计，运行时 provider enablement/priority/fallback order 生效。 | 覆盖模型接入与调度后台最小闭环。 | 未新增 `provider_models`；当前 route 的 `provider_model_id` 复用 provider id，成本/能力矩阵/AB 权重仍未完整。 |
| 配置中心 Tab | `GET /api/ops/admin/v1/config-tabs`、`PUT /api/ops/admin/v1/config-tabs/{tab_key}`，支持版本冲突和持久化 override。 | 覆盖系统配置中心的基础界面数据与保存。 | 更新后未实际热更新 runtime service 行为；更像配置草稿/后台管理数据。 |
| 审计日志基础 | API Key 创建/更新/删除/reset、用户 logout、admin login/logout、config update 会写审计；`GET /api/ops/admin/v1/audit-logs` 可查。 | 覆盖关键操作审计的部分要求。 | 审计覆盖面不足，列表无分页/筛选。 |
| 开发文档后端接口 | `/docs/openapi.yaml`、`/docs/openapi.json`、`/docs/examples`、`/docs/errors`。 | 覆盖开发文档页面的数据源基础。 | 不是完整文档站 UI。 |
| Ent 数据模型基线 | 已有 user、refresh_session、api_key、quota_reservation、redeem_code、point_ledger、model_provider、model_route、provider_error_policy、config_item、reference_asset、image_task、image_result、audit_log 等 schema。 | 覆盖技术方案主要数据模型。 | 部分 schema 还没有配套业务 API。 |

## 4. 部分完成能力

| 能力 | 已有部分 | 缺口 |
|---|---|---|
| 用户账号完整生命周期 | 有注册/登录、状态校验、资料更新、token_version 校验，后台可禁用/启用用户。 | 缺邮箱验证注册细分状态、账号注销、修改密码/忘记密码真实实现、后台重置密码。 |
| 图生图/参考图生成 | 任务可带 `reference_asset_ids`，worker 能加载参考图，OpenAI edit 支持 multipart image/mask。 | Web/API 侧 mask 管理、参考强度等参数产品化不足；compat edit 输入没有统一沉淀为 reference_asset。 |
| 历史图库 | 任务可列表/详情/软删，本地生成结果可写入 `task_images` 并通过 `/api/agent/image/v1/images/{image_id}` 下载。 | 远程 URL 结果 P0 只保存 URL，不镜像本地；公开图库/审核仍缺。 |
| 生成同步模式 | compat 接口同步返回 OpenAI 格式；Agent/Open API 已明确 async-only。 | PRD 原期望 Web 默认同步等待，本轮先收敛为异步提交 + 前端轮询；如要同步等待需后续另立契约。 |
| API Key 额度与限速 | API Key 有 total/daily quota、RPM、过期、状态；Open API/compat 创建任务前会 reserve quota。 | quota 与 billing 双路径存在一定重复；后台调整、分组策略、账号级并发/限速未完成。 |
| 兑换码兑换 | 用户兑换接口、后台管理接口、幂等键与核销记录已具备。 | 还缺批量导出、适用用户范围、活动规则等高级运营能力。 |
| 管理后台配置 | 配置 Tab 可读写，含 auth、generation_limits、billing_pricing、routing_models、docs。 | 配置修改没有驱动实际运行中的 billing/model resolver/provider；缺发布/灰度/回滚/审计详情。 |
| 管理后台审计 | 审计 store 和基础查询已具备。 | 缺分页、筛选、失败操作记录、更多动作覆盖；不能满足完整审计检索。 |
| 调用记录/成本核算 | `GET /api/ops/admin/v1/call-records` 可查询真实 `image_tasks`，包含 provider、积分、错误、时间和 attempt count。 | 缺成本、毛利、失败原因聚合与监控大盘。 |
| 模型路由 | 配置文件能力矩阵仍是能力校验来源；DB provider enablement 与 route priority/fallback order 已接入 runtime。 | 未实现完整 `provider_models` 能力矩阵、成本字段、健康检查跳过与 AB 权重分流。 |
| 部署交付 | Dockerfile、compose、nginx、Prometheus 配置、runbook 存在。 | 集群模式下本地文件存储要求共享卷，Redis 使用有限；S3 直传/对象存储生产能力未完整实现。 |

## 5. 只搭建了框架的能力

| 能力 | 当前框架 | 还差什么 |
|---|---|---|
| 支付与订单 | PRD/技术方案中有支付与订单闭环要求。 | 当前没有订单 schema、支付 webhook、购买套餐、支付状态流转后端实现。 |
| 套餐/订阅 | 余额和积分流水已有。 | 没有套餐商品、订阅周期、套餐权益、购买入口后端。 |
| 用户分组运营 | `user_groups` schema 和 billing multiplier 配置存在。 | 没有后台用户分组 CRUD、用户分组调整接口，用户实际 group 主要来自默认/配置。 |
| 模型供应商运营 | `model_providers`、`model_routes` schema 已接后台 CRUD 与 runtime 读取。 | `provider_models` 能力矩阵、密钥加密配置明细、健康检查、成本字段还未完整产品化。 |
| 图片结果表 | `task_images` schema 已用于本地生成结果记录，包含公开状态字段。 | 远程 URL 结果本地镜像、公开审核状态流转还未完成。 |
| 公开审核与图片广场 | `ImageResult.visibility_status`、`review_reason`、`published_at` 已预留。 | 没有申请公开、审核、下架、公开瀑布流接口。 |
| 多语言配置 | 用户 profile 有 `default_locale`，配置中心有 docs title/base path。 | 没有完整 i18n 文案配置、后台文案管理或语言包接口。 |
| Redis 化支撑 | 配置中有 Redis URL。 | 当前核心会话/验证码/限流/配置缓存未看到完整 Redis 实现。 |

## 6. 未开始能力

| 能力 | 说明 |
|---|---|
| 真实支付闭环 | 支付订单创建、第三方支付、支付 webhook、支付成功入账、订单查询均未开始。 |
| 订阅套餐购买 | 套餐展示可由前端 mock，但后端没有商品/套餐/权益/订单接口。 |
| 后台用户重置密码/限速设置 | 用户列表、禁用/启用、积分调整已完成；重置密码、并发/RPM 设置未完成。 |
| 兑换码高级运营 | 创建、批量创建、状态更新、核销记录已完成；批量导出、适用范围、活动规则未完成。 |
| 后台模型接入完整管理 | 供应商与路由 CRUD 已完成；底层 provider model、成本字段、健康状态、AB 权重闭环未完成。 |
| 图片公开审核/广场 | P1 数据字段预留，但接口未开始。 |
| 成本核算与监控大盘 | 调用记录、成功率、成本、毛利、失败原因聚合未完成。 |
| 内容安全/审核 | 没有文本/图片内容安全策略与机审接入。 |

## 7. 当前可与前端开始对接的接口清单

### 7.1 用户 Web：建议立即联调

| 模块 | 方法与路径 | 鉴权 | 前端用途 | 对接状态 |
|---|---|---|---|---|
| 健康检查 | `GET /healthz`、`GET /readyz` | 无 | 本地/部署状态探测。 | 可直接用 |
| 发送邮箱码 | `POST /api/agent/auth/v1/email/send-code` | 无 | 登录/注册前发送验证码。 | 可直接用 |
| 邮箱码登录 | `POST /api/agent/auth/v1/login/email-code` | 无 | 登录或自动注册，返回 access token，并设置 refresh cookie。 | 可直接用 |
| 刷新会话 | `POST /api/agent/auth/v1/session/refresh` | Refresh Cookie | 静默续期。 | 可直接用 |
| 退出登录 | `POST /api/agent/auth/v1/logout` | Bearer | 清理 refresh cookie，写审计。 | 可直接用 |
| 获取资料 | `GET /api/agent/user/v1/profile` | Bearer | 账户中心、页面 bootstrap。 | 可直接用 |
| 更新资料 | `PUT /api/agent/user/v1/profile` | Bearer | 昵称、简介、头像 key、语言、主题。 | 可直接用 |
| 更新偏好 | `PUT /api/agent/user/v1/preferences` | Bearer | 主题、语言偏好。 | 可直接用 |
| 上传头像 | `POST /api/agent/user/v1/avatar` | Bearer + multipart `file` | 头像上传。 | 可直接用 |
| 余额 | `GET /api/agent/billing/v1/balance` | Bearer | 余额卡片、生成按钮可用性。 | 可直接用 |
| 积分流水 | `GET /api/agent/billing/v1/ledger?page=&page_size=` | Bearer | 账户流水列表。 | 可直接用 |
| 生成估价 | `GET /api/agent/billing/v1/estimate?...` | Bearer | 表单实时费用预估。 | 可直接用 |
| 能力矩阵 | `GET /api/agent/image/v1/capabilities` | Bearer | 模型、任务类型、质量、数量上限渲染。 | 可直接用 |
| 上传参考图 | `POST /api/agent/image/v1/reference-assets` | Bearer + multipart `file` | 图生图/参考图上传。 | 可直接用 |
| 获取参考图 metadata | `GET /api/agent/image/v1/reference-assets/{asset_id}` | Bearer | 上传后详情回显。 | 可直接用 |
| 下载参考图 | `GET /api/agent/image/v1/reference-assets/{asset_id}/download` | Bearer | 参考图预览/下载。 | 可直接用 |
| 删除参考图 | `DELETE /api/agent/image/v1/reference-assets/{asset_id}` | Bearer | 删除未使用素材。 | 可直接用 |
| 创建任务 | `POST /api/agent/image/v1/tasks` | Bearer | 文生图/图生图/参考图生成提交。 | 可直接用，返回 202 异步任务 |
| 任务列表 | `GET /api/agent/image/v1/tasks` | Bearer | 工作台任务列表。 | 可直接用 |
| 任务详情 | `GET /api/agent/image/v1/tasks/{task_id}` | Bearer | 轮询任务状态和结果。 | 可直接用 |
| 下载生成图片 | `GET /api/agent/image/v1/images/{image_id}` | Bearer | 下载本地持久化的生成结果。 | 可直接用；仅本地 `b64_json` 结果，远程 URL 结果直接使用返回 URL |
| 历史任务列表 | `GET /api/agent/image/v1/history/tasks` | Bearer | 历史图库任务维度列表。 | 可直接用 |
| 历史任务详情 | `GET /api/agent/image/v1/history/tasks/{task_id}` | Bearer | 历史详情。 | 可直接用 |
| 删除历史任务 | `DELETE /api/agent/image/v1/history/tasks/{task_id}` | Bearer | 隐藏/删除历史记录。 | 可直接用 |
| API Key 列表 | `GET /api/agent/account/v1/api-keys` | Bearer | 开发者密钥页。 | 可直接用 |
| 创建 API Key | `POST /api/agent/account/v1/api-keys` | Bearer | 创建 AK/SK，secret 仅返回一次。 | 可直接用 |
| API Key 详情 | `GET /api/agent/account/v1/api-keys/{key_id}` | Bearer | 密钥详情。 | 可直接用 |
| 更新 API Key | `PUT /api/agent/account/v1/api-keys/{key_id}` | Bearer | 名称、状态、额度、RPM、过期时间。 | 可直接用 |
| 删除 API Key | `DELETE /api/agent/account/v1/api-keys/{key_id}` | Bearer | 删除/撤销密钥。 | 可直接用 |
| 重置 API Key Secret | `POST /api/agent/account/v1/api-keys/{key_id}/reset-secret` | Bearer | 轮换 secret。 | 可直接用 |
| 兑换码兑换 | `POST /api/agent/billing/v1/redeem-codes/redeem` | Bearer + `Idempotency-Key` | 用户兑换积分。 | 可直接用，后台已有发码接口 |
| OpenAPI 文档 | `GET /docs/openapi.yaml`、`GET /docs/openapi.json`、`GET /docs/examples`、`GET /docs/errors` | 无 | 开发文档页数据源。 | 可直接用 |

用户 Web 暂不建议依赖：

| 接口/能力 | 原因 |
|---|---|
| `POST /api/agent/auth/v1/password/change` | 当前返回 501/未启用。 |
| `POST /api/agent/auth/v1/password/reset` | 当前返回 501/未启用。 |
| 公开申请/图片广场 | 后端接口未开始。 |
| 购买套餐/支付 | 后端接口未开始。 |

### 7.2 API 开发者 / 开放平台：可对接接口

Open API 使用 HMAC 鉴权，请求头必须包含：

- `X-Access-Key`
- `X-Timestamp`：RFC3339 或 Unix 秒
- `X-Body-SHA256`：请求体原始字节的 SHA-256，base64url 无 padding；GET 使用空 body hash
- `X-Signature`：用 API Key secret 对 canonical payload 做 HMAC-SHA256

| 模块 | 方法与路径 | 用途 | 对接状态 |
|---|---|---|---|
| Open API 估价 | `GET /api/open/image/v1/estimate` | API 调用前估算积分。 | 可直接用 |
| Open API 创建任务 | `POST /api/open/image/v1/tasks` | 创建图片任务。 | 可直接用 |
| Open API 查任务 | `GET /api/open/image/v1/tasks/{task_id}` | 查询异步任务状态和结果。 | 可直接用 |
| Open API 余额 | `GET /api/open/image/v1/balance` | 查询所属用户余额。 | 可直接用 |
| Open API capabilities | `GET /api/open/image/v1/capabilities` | 查询能力矩阵。 | 可直接用 |
| Open API 参考图 inline | `POST /api/open/image/v1/reference-assets/uploads` | base64 上传参考图。 | 可直接用 |
| Open API 参考图 multipart | `POST /api/open/image/v1/reference-assets` | multipart 上传参考图。 | 可直接用 |
| Open API 参考图 metadata | `GET /api/open/image/v1/reference-assets/{asset_id}` | 获取参考图信息。 | 可直接用 |
| OpenAI 生成 | `POST /v1/images/generations` | OpenAI-compatible 文生图。 | 可直接用 |
| OpenAI 编辑 | `POST /v1/images/edits` | OpenAI-compatible 图片编辑。 | 可直接用 |
| OpenAI models | `GET /v1/models` | OpenAI-compatible 模型列表。 | 可直接用 |

### 7.3 管理后台：可对接接口

| 模块 | 方法与路径 | 鉴权 | 前端用途 | 对接状态 |
|---|---|---|---|---|
| 管理员登录 | `POST /api/ops/admin/v1/auth/login` | 无 | 后台登录。 | 可直接用 |
| 管理员退出 | `POST /api/ops/admin/v1/auth/logout` | Admin Bearer | 后台退出。 | 可直接用 |
| 配置 Tab 列表 | `GET /api/ops/admin/v1/config-tabs` | Admin Bearer | 配置中心首页。 | 可直接用 |
| 更新配置 Tab | `PUT /api/ops/admin/v1/config-tabs/{tab_key}` | Admin Bearer | 保存配置 Tab。 | 可直接用，但运行时不一定生效 |
| 审计日志 | `GET /api/ops/admin/v1/audit-logs` | Admin Bearer | 审计列表。 | 可直接用，但无分页/筛选 |
| 用户列表 | `GET /api/ops/admin/v1/users` | Admin Bearer | 用户管理列表与筛选。 | 可直接用 |
| 用户详情 | `GET /api/ops/admin/v1/users/{user_id}` | Admin Bearer | 用户资料、余额、近期流水。 | 可直接用 |
| 更新用户状态 | `POST /api/ops/admin/v1/users/{user_id}/status` | Admin Bearer | 禁用/启用用户。 | 可直接用 |
| 调整用户积分 | `POST /api/ops/admin/v1/users/{user_id}/points-adjustments` | Admin Bearer + `Idempotency-Key` | 人工加减积分。 | 可直接用 |
| 兑换码列表/创建 | `GET/POST /api/ops/admin/v1/redeem-codes` | Admin Bearer | 兑换码列表、单个创建。 | 可直接用 |
| 批量创建兑换码 | `POST /api/ops/admin/v1/redeem-codes:batch-create` | Admin Bearer | 运营批量发码。 | 可直接用；批量写入暂非全事务 |
| 更新兑换码状态 | `POST /api/ops/admin/v1/redeem-codes/{code_id}/status` | Admin Bearer | 启用、禁用、失效兑换码。 | 可直接用 |
| 兑换码核销记录 | `GET /api/ops/admin/v1/redeem-codes/{code_id}/redemptions` | Admin Bearer | 查看某兑换码的核销流水。 | 可直接用 |
| 调用记录 | `GET /api/ops/admin/v1/call-records` | Admin Bearer | 调用记录页。 | 可直接用；支持分页和多条件筛选 |
| 模型供应商列表/创建 | `GET/POST /api/ops/admin/v1/model-providers` | Admin Bearer | 模型供应商管理。 | 可直接用 |
| 模型供应商详情/更新/删除 | `GET/PUT/DELETE /api/ops/admin/v1/model-providers/{provider_code}` | Admin Bearer | 查看、编辑、删除供应商。 | 可直接用；有 route 引用时删除返回 409 |
| 模型路由列表/创建 | `GET/POST /api/ops/admin/v1/model-routes` | Admin Bearer | 抽象模型路由配置。 | 可直接用 |
| 模型路由详情/更新/删除 | `GET/PUT/DELETE /api/ops/admin/v1/model-routes/{route_id}` | Admin Bearer | 查看、编辑、删除路由。 | 可直接用 |

管理后台暂缺接口：

| 页面/能力 | 当前状态 |
|---|---|
| 用户管理增强 | 缺重置密码、并发/RPM 设置。 |
| 兑换码管理增强 | 缺批量导出、适用范围、活动规则。 |
| 价格策略完整管理 | 配置 Tab 有数据，但不是完整 pricing policy CRUD。 |
| 模型供应商与路由增强 | 缺 provider model 能力矩阵、成本字段、健康检查、AB 权重。 |
| 公开审核 | 未开始。 |
| 监控大盘/成本核算 | 未完成。 |

## 8. 前端对接注意事项

1. 所有 `httpx.WriteSuccess` 返回通常是统一成功包，前端要以实际响应结构为准，不要只看 OpenAPI schema 名称。
2. 用户登录后前端需要保存 access token；refresh token 是 HttpOnly Cookie，静默续期调用 refresh 接口即可。
3. 创建任务当前按异步模式处理，前端应提交后轮询 `GET /api/agent/image/v1/tasks/{task_id}`。
4. 生成结果目前主要在 task 的 `results` 字段中返回；本地持久化结果可使用 `GET /api/agent/image/v1/images/{image_id}` 下载，远程 URL 结果直接使用返回 URL。
5. 参考图上传可用于图生图/参考图任务，创建任务时传 `reference_asset_ids`。
6. API Key secret 只在创建和 reset 时返回一次，前端应做一次性展示和复制提示。
7. 用户兑换码兑换接口需要 `Idempotency-Key`；后台已有发码/批量发码接口。
8. 管理后台配置保存成功不代表 API runtime 已热加载新配置，前端文案应避免暗示“立即生效”。
9. 管理后台模型供应商/路由写操作与审计当前不是同一 DB 事务；审计失败可能导致业务写已成功但接口返回错误，前端重试前应重新拉取列表确认状态。

## 9. 后续后端优先级

| 优先级 | 建议补齐 | 原因 |
|---|---|---|
| P0-followup-1 | 支付/订单/套餐闭环 | 充值、套餐权益和支付入账仍是最大产品缺口。 |
| P0-followup-2 | Provider model 能力矩阵、成本字段、健康检查和 AB 权重 | 当前模型后台是 provider/route 最小闭环，尚未达到完整运营调度。 |
| P0-followup-3 | 配置中心 runtime 热生效 | billing/generation limits 等配置保存后还没有驱动运行时服务。 |
| P0-followup-4 | 监控大盘与成本/毛利聚合 | 调用记录已有明细，运营还需要聚合指标。 |
| P0-followup-5 | 后台写操作与审计事务化 | 避免业务写成功但审计失败导致接口返回错误和状态歧义。 |
| P1 | 公开审核/图片广场 | 数据字段已预留，可在主链路稳定后推进。 |

## 10. 本轮验证记录

| 命令 | 结果 |
|---|---|
| `go test ./internal/repository/entstore -run 'Model' -count=1` | 通过 |
| `go test ./internal/service/modeladmin ./internal/service/imagetask -run 'Model\|Route\|Provider\|Fallback' -count=1` | 通过 |
| `go test ./internal/http/router -run 'AdminModel' -count=1` | 通过 |
| `go test ./api/openapi -count=1` | 通过 |
| `go test ./...` | 通过 |
| `./scripts/workflow/verify.sh` | 通过 |
| `./scripts/workflow/api-smoke.sh` | 通过，`http://127.0.0.1:18080` |
