# Pic Gallery 后端进度对照 PRD / 技术方案梳理（更新于 2026-05-21）

## 1. 梳理范围

本文只评估后端、OpenAPI 契约、持久化、Worker、Provider、部署底座、审计/后台 API 等服务端能力，不评估当前 `web/*` 前端页面实现。

参考资料：

- PRD：`docs/prd/pic-gallery-prd.md`
- 技术方案：`docs/tech/pic-gallery-tech-design.md`
- OpenAPI：`api/openapi/openapi.yaml`
- 后端入口：`cmd/api/main.go`、`cmd/worker/main.go`
- 当前工作分支：`codex/p0-backend-ready`

当前验证基准：

- `CGO_ENABLED=0 go build ./cmd/api ./cmd/worker` 通过
- `go test -count=1 ./...` 通过
- `go test -race -count=1 ./internal/service/billing ./internal/service/apikey ./internal/worker` 通过
- `docker build -f Dockerfile.api .` 通过
- `docker build -f Dockerfile.worker .` 通过
- 最终 BLOCK-only review 结论：`NO_BLOCK_FINDINGS`

## 2. 完成度定义

| 标识 | 含义 |
|---|---|
| ✅ 已准出完成 | 已有 runtime 路由、服务、持久化和测试；关键异常路径覆盖；与 OpenAPI 契约一致。 |
| 🟡 部分完成 | 主链路或 service 已有，但缺 endpoint、权限、动态配置、完整边界或产品级闭环。 |
| 🟠 架子完成 | 主要是 schema、配置、默认定义或少量 primitive 存在，业务闭环还没形成。 |
| ⚪ 未开始 | 没有可用 service/handler，或仅存在 PRD/技术方案描述。 |

## 3. 一句话结论

当前后端已经完成“图片生成平台最难的内核部分”：任务编排、Worker 抢占、计费结算、Provider 适配、OpenAPI / OpenAI Compat 最小闭环。

但完整 PRD 产品化能力还差“账号生产化、API Key 管理、历史图库图片落库、后台管理、审计、兑换/支付、部署交付”等外围闭环。前端准备推倒重做后，后端下一步最值得优先补的是：API Key 管理、Open API 查询能力、生成结果图库落库。

## 4. 已准出完成的后端能力

### 4.1 任务状态机、Worker lease 与抢占闭环

状态：✅ 已准出完成（核心后端内核）

代码位置：

- `internal/service/imagetask/service.go`
- `internal/repository/entstore/imagetask_store.go`
- `internal/worker/runner.go`
- `internal/domain/imagetask/types.go`

已完成能力：

- 创建任务并进入排队态。
- Worker 从 DB 抢占任务。
- 支持 `lease_owner`、`lease_expires_at`。
- 支持 heartbeat 续租。
- 支持过期 lease reclaim。
- stale worker 写入会被拒绝。
- provider 成功/失败后落终态。
- lease conflict / reclaim race 下避免重复结算和重复 provider 调用窗口。

测试覆盖：

- `internal/service/imagetask/service_test.go`
- `internal/repository/entstore/imagetask_store_test.go`
- `internal/worker/runner_test.go`

对照 PRD / 技术方案：

- 覆盖 PRD 5.2 图片任务状态机的核心后端执行模型。
- 覆盖技术方案 T13 “任务编排、状态机与 Worker 集群”的核心内容。
- 当前实际状态主要是 `queued/running/succeeded/failed/deleted`，PRD 中的 `pending_validation/rejected/partial_failed` 尚未完整显式落地。

### 4.2 计费核心：预估、预扣、结算、失败退款、5 位小数

状态：✅ 已准出完成（生成计费主链路）

代码位置：

- `internal/domain/billing/calculator.go`
- `internal/service/billing/service.go`
- `internal/repository/entstore/billing_store.go`
- `internal/domain/billing/types.go`

已完成能力：

- 按抽象模型、质量档位、任务类型、输出数量、参考图数量、用户分组倍率计算预估积分。
- 支持 `auto` 质量档位解析。
- 支持 reserve / finalize / refund。
- 失败不扣费。
- 部分成功可按成功图片数计算实际点数。
- 积分字段统一 5 位小数。
- wrong-user protection。
- finalize 幂等和并发场景测试。

测试覆盖：

- `internal/domain/billing/calculator_test.go`
- `internal/service/billing/service_test.go`
- `internal/repository/entstore/billing_store_test.go`

对照 PRD / 技术方案：

- 覆盖 PRD A8、A21、A22 的核心后端计算逻辑。
- 覆盖技术方案 2.5.2 计费结算伪代码的核心路径。
- 不包含充值、兑换码、套餐、积分过期、支付订单相关流水。

### 4.3 Agent 侧余额与流水查询

状态：✅ 已准出完成（当前账本查询范围）

路由：

- `GET /api/agent/billing/v1/balance`
- `GET /api/agent/billing/v1/ledger`

代码位置：

- `internal/http/handlers/api.go`
- `internal/http/router/tasks_api_test.go`

已完成能力：

- 用户 JWT 鉴权。
- 查询可用积分、冻结积分、用户倍率、折算金额。
- 查询积分流水分页。
- 与 task service 共享同一个 billing backend，避免账本分裂。

对照 PRD / 技术方案：

- 覆盖 PRD A4 中余额与流水的基础后端能力。
- 不包含套餐、即将过期积分、充值余额来源拆分、筛选条件等完整产品视图。

### 4.4 Open Image API 三个 Task 4 P0 接口

状态：✅ 已准出完成（Task 4 最小闭环）

路由：

- `POST /api/open/image/v1/reference-assets/uploads`
- `POST /api/open/image/v1/tasks`
- `GET /api/open/image/v1/estimate`

代码位置：

- `internal/http/handlers/api.go`
- `internal/http/router/router.go`
- `internal/http/router/open_api_test.go`
- `api/openapi/openapi.yaml`
- `api/openapi/openapi_test.go`

已完成能力：

- AK/SK 最小鉴权。
- OpenAPI 原生估价复用 billing。
- OpenAPI 创建任务复用 task / billing 主链路。
- OpenAPI 参考图上传采用 inline base64 local storage 模式。
- 任务落 `api_key_id` / `source_channel=openapi`。
- 参考图落 `api_key_id` / `upload_source=openapi`。
- `Idempotency-Key` 创建任务重试可复用相同任务 ID。

对照 PRD / 技术方案：

- 技术方案中当前 P0 runtime 缺口已闭环。
- 完整 Open API 中的任务查询、余额、capabilities、multipart 上传、资产查询、支付 webhook 仍未完成。

### 4.5 OpenAI 兼容接口最小闭环

状态：✅ 已准出完成（兼容接口最小可用）

路由：

- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `GET /v1/models`

代码位置：

- `internal/service/compat/service.go`
- `internal/service/compat/service_test.go`
- `internal/http/handlers/api.go`

已完成能力：

- `Authorization: Bearer sk-*` API Key 鉴权。
- 兼容 generate / edit / models。
- 支持 OpenAI / OpenRouter provider 路由。
- OpenAI / OpenRouter 上游错误归一化。
- OpenAI compat 请求进入现有 task / billing / provider 主链路。

对照 PRD / 技术方案：

- 覆盖 PRD A23、A24 的最小后端能力。
- 还缺 `Idempotency-Key`、限流、Key 额度、完整 OpenAI SDK 兼容细节、edit 图片转存 reference_assets 等增强项。

### 4.6 Provider 抽象层：OpenAI + OpenRouter

状态：✅ 已准出完成（当前两个 provider）

代码位置：

- `internal/provider/contracts.go`
- `internal/provider/openai/client.go`
- `internal/provider/openrouter/client.go`
- `internal/provider/errors.go`

已完成能力：

- OpenAI Images API generate / edit。
- OpenRouter chat completions 多模态图片生成适配。
- data URL / url 响应归一。
- 上游错误分类。

对照 PRD / 技术方案：

- 覆盖技术方案 2.4.4 和 2.5.3 的核心 provider 转换策略。
- provider 健康检查、成本字段、DB 驱动模型能力尚未完成。

### 4.7 OpenAPI 契约基线与当前已接路由一致性

状态：✅ 当前范围完成

代码位置：

- `api/openapi/openapi.yaml`
- `api/openapi/openapi_test.go`

已完成能力：

- P0 Open Image 三路由已在 spec 和 runtime 对齐。
- OpenAI compat security 已改为 `compatBearerAuth`。
- native Open API security 使用 `X-Access-Key` + `X-Signature` 同时要求。
- inline upload schema 已补齐。

对照 PRD / 技术方案：

- 开放接口契约基础完成。
- 不是完整开发文档页面。

## 5. 部分完成的后端能力

### 5.1 用户认证与会话

状态：🟡 部分完成

代码位置：

- `internal/service/auth/service.go`
- `internal/repository/entstore/auth_store.go`
- `internal/http/handlers/api.go`

已完成：

- 邮箱验证码登录/注册主流程。
- 登录后签发 Access Token + Refresh Token。
- Refresh Token hash 持久化。
- refresh rotation。
- refresh replay 检测并阻断 session family。
- `users` / `refresh_sessions` 持久化。

缺口：

- 验证码目前固定为 `123456`，没有真实邮件发送。
- 没有 60 秒重发冷却。
- 没有连续错误 5 次冷却 15 分钟。
- 没有 logout。
- 没有修改密码、忘记密码。
- `token_version` 没有形成完整吊销校验闭环。
- 用户禁用后现有 token / API Key 的联动阻断不完整。

对照 PRD：

- A1 / A2 后端主干有了，但不能算完整准出。

### 5.2 用户资料与偏好

状态：🟠 架子完成

已完成：

- `GET /api/agent/user/v1/profile` 可返回用户基础资料。

缺口：

- `PUT /api/agent/user/v1/profile` OpenAPI 声明了，但 handler 实际没有更新逻辑。
- 没有头像上传。
- 没有 bio / signature 更新。
- 没有 theme / default_locale / preferences 更新和持久化 API。

对照 PRD：

- A3 基本未完成，只能算资料读取架子。

### 5.3 API Key 鉴权服务

状态：🟡 部分完成

代码位置：

- `internal/domain/apikey/types.go`
- `internal/service/apikey/service.go`
- `internal/repository/entstore/apikey_store.go`

已完成：

- 创建 key 的 service primitive。
- secret 只存 hash。
- native `X-Access-Key` + `X-Signature` 校验。
- compat Bearer sk 校验。
- disabled / expired key 拒绝。
- `last_used_at` 更新。

缺口：

- 没有用户侧 API Key 创建 / 列表 / 重置 / 删除 / 启停接口。
- 没有管理员侧 Key 管理。
- 没有 total quota / daily quota 实际扣减限制。
- 没有 RPM 限速。
- 没有完整 HMAC canonical signing。
- 没有 `X-Timestamp` 漂移校验。
- 没有 API Key 操作审计。

对照 PRD：

- A6 的“可调用鉴权”部分完成；“密钥管理产品能力”还没完成。

### 5.4 Agent 图片任务 API

状态：🟡 部分完成

路由：

- `POST /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks/{task_id}`
- `GET /api/agent/image/v1/history/tasks`
- `GET /api/agent/image/v1/history/tasks/{task_id}`
- `DELETE /api/agent/image/v1/history/tasks/{task_id}`

已完成：

- 用户创建任务。
- 幂等任务 ID。
- 查询自己的任务。
- 列表自己的任务。
- 删除历史任务视角。
- 任务创建会进行估价和 reserve。

缺口：

- `response_mode=sync` 目前没有 HTTP 同步等待结果闭环，创建任务返回 queued。
- 任务详情没有完整结果图片对象、下载地址、预览 URL。
- 缺少筛选、分页、时间范围。
- `negative_prompt`、`aspect_ratio`、`reference_strength`、`seed`、`save_policy` 没有完整接入 handler。
- `partial_failed` 状态未完整体现。

对照 PRD：

- A7 / A11 / A12 的主干有，但产品验收还不完整。

### 5.5 参考图资产

状态：🟡 部分完成

代码位置：

- `internal/service/assets/service.go`
- `internal/repository/entstore/assets_store.go`
- `internal/domain/assets/types.go`

已完成：

- Agent multipart 上传。
- OpenAPI inline base64 上传。
- 图片 decode 校验。
- 大小限制。
- 按 hash 去重。
- local storage 保存。
- Ent 持久化。
- 用户资源隔离查询。

缺口：

- 没有删除接口。
- 没有预签名 URL / S3 模式。
- 没有上传 session complete。
- 没有 preview / download。
- 没有过期清理。
- OpenAI compat edit 目前直接读 multipart 图片传 provider，没有按技术方案转存到 reference_assets。

对照 PRD：

- “图生图/参考图输入资产”后端基础完成；完整图床/资产生命周期未完成。

### 5.6 模型能力矩阵与路由

状态：🟡 部分完成

代码位置：

- `internal/domain/modelhub/resolver.go`
- `internal/service/capabilities/service.go`
- `internal/config/config.go`

已完成：

- 从 YAML config 读取 provider capabilities。
- 按 abstract model、task type、quality、输出数量、参考图数量、mask 能力筛 provider。
- 按 priority 排序。
- capabilities endpoint 返回模型能力。

缺口：

- 没有 DB 驱动的 `provider_models` 表；技术方案里有 `provider_models`，当前 Ent 里没有对应 schema。
- 没有后台供应商 CRUD。
- 没有 AB weight 真实分流。
- 没有 provider 健康状态动态跳过。
- 没有成本字段和毛利核算。
- 没有 model route 动态发布生效机制。

对照 PRD：

- A13 / A24 的运行时路由基础有了；后台模型配置不是完整闭环。

### 5.7 系统配置中心

状态：🟡 部分完成

路由：

- `GET /api/ops/admin/v1/config-tabs`
- `PUT /api/ops/admin/v1/config-tabs/{tab_key}`

代码位置：

- `internal/service/adminconfig/service.go`
- `internal/repository/entstore/config_store.go`
- `internal/domain/adminconfig/types.go`

已完成：

- 默认配置 Tab 定义。
- Ent 持久化 override。
- version 乐观锁。
- 配置项更新测试。

缺口：

- 当前 Ops API 使用普通用户 JWT，不是独立 admin auth。
- 配置更新后没有完整“1 分钟内影响新任务”的 runtime reload / 缓存失效机制。
- 没有配置变更审计。
- 只覆盖配置 Tab，不覆盖用户管理、模型管理、运营管理。

对照 PRD：

- A15 有架子和部分可用逻辑，不能算完整准出。

### 5.8 错误归一化

状态：🟡 部分完成

已完成：

- 平台错误类型 `pkg/errs`。
- provider upstream error classification。
- compat error shape。
- 部分 HTTP handler 标准响应。

缺口：

- 技术方案里的 `provider_error_policies` 只是 schema，没有 service / 后台配置生效。
- 部分 handler 仍直接 `http.Error`，没有完全统一 error envelope。
- 用户可见“是否扣费 / 下一步建议”还没有系统化文案。

对照 PRD：

- A26 部分完成。

### 5.9 本地开发与部署

状态：🟡 部分完成

已完成：

- `cmd/api`。
- `cmd/worker`。
- config loader。
- local docker-compose 依赖：Postgres / Redis / MinIO / Mailpit。
- metrics / health / ready。
- storage topology validation。

缺口：

- 没有后端 Dockerfile。
- 没有 worker Dockerfile。
- 没有完整 nginx 配置。
- 没有完整集群部署资产。

对照 PRD：

- A20 只完成本地开发依赖层，不是完整交付。

## 6. 只有架子的后端能力

### 6.1 审计日志

状态：🟠 架子完成

已有：

- Ent schema：`audit_logs`
- 代码位置：`internal/repository/ent/schema/auditlog.go`

缺口：

- 没有 audit service。
- 没有登录、续期、API Key、任务、计费、后台配置写审计。
- 没有审计查询。

对照 PRD：

- A18 未形成可用能力。

### 6.2 管理员账号体系

状态：🟠 架子完成

已有：

- Ent schema：`admin_users`
- 代码位置：`internal/repository/ent/schema/adminuser.go`

缺口：

- 没有 admin login。
- 没有 admin token / session。
- Ops API 没有 admin role 权限。
- 管理员与 C 端用户体系未隔离。

对照 PRD：

- A16 / A19 未完成。

### 6.3 兑换码

状态：🟠 架子完成

已有：

- Ent schema：`redeem_codes`
- 代码位置：`internal/repository/ent/schema/redeemcode.go`

缺口：

- 没有 redeem service。
- 没有核销 API。
- 没有并发幂等核销。
- 没有后台批量生成 / 导出 / 失效。

对照 PRD：

- A5 未完成。

### 6.4 图片结果与图库公开状态

状态：🟠 架子完成

已有：

- Ent schema：`image_results`
- 字段包含 `visibility_status`、`review_reason`、`published_at`
- 代码位置：`internal/repository/ent/schema/imageresult.go`

缺口：

- provider 返回结果没有落 `image_results` 形成图床记录。
- 没有图片下载接口。
- 没有公开申请 / 审核 service。
- 没有公开图片列表。

对照 PRD：

- A12 的“任务历史”部分有，图片级图库没完成。
- P1 图片广场 / 审核未开始。

### 6.5 模型供应商和路由后台

状态：🟠 架子完成

已有：

- Ent schema：`model_providers`、`model_routes`、`provider_error_policies`
- config-driven resolver。

缺口：

- 没有 provider CRUD。
- 没有 route CRUD。
- 没有 error policy CRUD。
- 没有 DB 配置驱动 provider client 初始化。
- 没有 health check 和禁用生效。

对照 PRD：

- A13 / A17 只完成运行时 config 路由的一小部分。

## 7. 基本未开始的后端能力

### 7.1 支付 / 订单 / 订阅套餐

状态：⚪ 未开始

PRD 范围：

- 充值。
- 订阅套餐。
- 支付订单状态闭环。

当前情况：

- 没有 order / payment / subscription schema。
- 没有支付 webhook。
- 没有订单状态机。
- 没有套餐权益。

对照 PRD：

- 8.3 支付订单、订阅套餐未开始。

### 7.2 用户管理后台

状态：⚪ 未开始

PRD 范围：

- 禁用用户。
- 重置密码。
- 调整积分。
- 设置并发 / RPM。
- 配置用户分组倍率。

当前情况：

- 只有 `users`、`user_groups` schema 和 auth store 创建默认 basic 用户。
- 没有管理 API。
- `billing.AdminAdjust` service 有 primitive，但没有 admin endpoint 和审计。

对照 PRD：

- A16 未开始。

### 7.3 API Key 管理页面对应后端 API

状态：⚪ 未开始（鉴权 service 已有，管理 API 未开始）

PRD 范围：

- 创建、删除、重置、启用 / 禁用、额度上限、分组、过期时间、RPM。

当前情况：

- 鉴权 service / store 完成。
- 没有用户侧 `/api/agent/.../api-keys` 管理接口。
- 没有 reset / delete / quota / rpm 限制。

对照 PRD：

- A6 的管理能力未开始。

### 7.4 限流与额度配额

状态：⚪ 未开始

PRD / 技术方案范围：

- 用户、API Key、套餐、平台多层限流。
- Redis `rate:user:{id}` / `rate:api_key:{id}`。

当前情况：

- Config / schema 中有 `rpm_limit` 字段。
- 没有 Redis rate limiter。
- 没有 quota enforcement。

对照 PRD：

- A9 中“限速”未完成。

### 7.5 完整开发文档页面

状态：⚪ 未开始

当前情况：

- OpenAPI spec 有。
- 没有后端 docs 页面服务或静态文档页面。
- 没有示例代码页面。

对照 PRD：

- A25 未完成；只有契约文件基础。

### 7.6 Inner / Debug API

状态：⚪ 未开始

技术方案范围：

- upload complete。
- provider callback。
- cluster heartbeat。
- debug retry。
- mock result。

当前情况：

- 无对应路由。
- Worker heartbeat 是内部 Go service 调用，不是 Inner API。

对照技术方案：

- 2.4.6 未开始。

### 7.7 后台调用记录、监控大盘、运营大盘

状态：⚪ 未开始

当前情况：

- metrics endpoint 有。
- `image_tasks` schema 可以作为调用记录来源。
- 没有 Ops 查询 API。
- 没有成功率、失败率、耗时、成本、毛利大盘。

对照 PRD：

- A17 未完成。

### 7.8 多语言后端资源结构

状态：⚪ 未开始

当前情况：

- 用户和错误文案多为硬编码英文或配置内文本。
- 没有 i18n message catalog。

对照 PRD：

- Q7 后端支撑未开始。

## 8. 当前后端可真实使用的接口清单

### 8.1 基础接口

- `GET /`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### 8.2 Agent/Auth

- `POST /api/agent/auth/v1/email/send-code`
- `POST /api/agent/auth/v1/login/email-code`
- `POST /api/agent/auth/v1/session/refresh`

注意：验证码固定为 `123456`，适合开发和测试，不是生产闭环。

### 8.3 Agent/User

- `GET /api/agent/user/v1/profile`

注意：`PUT /api/agent/user/v1/profile` 在 OpenAPI 中有声明，但 runtime 尚未实现真实更新。

### 8.4 Agent/Billing

- `GET /api/agent/billing/v1/balance`
- `GET /api/agent/billing/v1/ledger`
- `GET /api/agent/billing/v1/estimate`

### 8.5 Agent/Image

- `GET /api/agent/image/v1/capabilities`
- `POST /api/agent/image/v1/reference-assets`
- `GET /api/agent/image/v1/reference-assets/{asset_id}`
- `POST /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks/{task_id}`
- `GET /api/agent/image/v1/history/tasks`
- `GET /api/agent/image/v1/history/tasks/{task_id}`
- `DELETE /api/agent/image/v1/history/tasks/{task_id}`

### 8.6 Open Image API

- `POST /api/open/image/v1/reference-assets/uploads`
- `POST /api/open/image/v1/tasks`
- `GET /api/open/image/v1/estimate`

### 8.7 OpenAI Compat

- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `GET /v1/models`

### 8.8 Ops/Admin

- `GET /api/ops/admin/v1/config-tabs`
- `PUT /api/ops/admin/v1/config-tabs/{tab_key}`

注意：当前不是独立 admin auth，而是复用普通用户 JWT。

## 9. PRD 必须验收项 A1-A26 对照

| PRD 验收项 | 后端状态 | 说明 |
|---|---:|---|
| A1 邮箱注册/登录 | 🟡 部分完成 | 注册/登录主链路可用；验证码固定、无真实邮件、无冷却/错误次数限制。 |
| A2 双 Token 会话 | 🟡 部分完成 | Access/Refresh、刷新轮换、重放阻断已做；logout、token_version 吊销、前端回跳不在后端闭环。 |
| A3 用户资料 | 🟠 架子 | profile GET 有；编辑昵称/签名/头像/偏好未做。 |
| A4 额度展示 | 🟡 部分完成 | balance/ledger 后端可用；套餐、过期积分、充值来源拆分未做。 |
| A5 兑换码 | ⚪ 未开始 | 只有 schema，无核销 service/API。 |
| A6 API Key | 🟡 部分完成 | 鉴权和 secret hash 完成；管理 CRUD、重置、删除、额度、RPM 未做。 |
| A7 图片生成成功 | 🟡 部分完成 | 文生图/图生图/参考图任务主链路可跑；结果图床/历史图库展示/download 不完整。 |
| A8 计费准确 | ✅ 核心完成 | 预估、预扣、结算、失败退款、5 位小数、并发幂等已覆盖；充值/兑换流水除外。 |
| A9 参数边界 | 🟡 部分完成 | prompt、模型、数量、参考图、余额等部分有；限速、禁用用户、完整参数边界不足。 |
| A10 数量上限 | 🟡 部分完成 | config 中 max count 会被 resolver 校验；后台调低后 runtime 生效闭环不足。 |
| A11 同步/异步 | 🟡 部分完成 | 异步队列/查询有；Web 同步等待未做；Open API 任务查询未做。 |
| A12 历史图库 | 🟡 部分完成 | task list/detail/delete 有；图片级记录、下载、筛选分页不完整。 |
| A13 后台模型配置 | 🟠 架子 | config-driven routing 有；后台供应商/模型/AB/降级 CRUD 未做。 |
| A14 后台积分策略 | 🟡 部分完成 | 计费配置和 config tab 有；动态生效、审计、完整后台管理不足。 |
| A15 系统配置中心 | 🟡 部分完成 | config-tabs GET/PUT + version 有；admin auth、审计、动态应用不完整。 |
| A16 用户管理 | ⚪ 未开始 | 无 admin 用户管理 API。 |
| A17 调用记录 | ⚪ 未开始 | task 数据可作为基础；无后台筛选查询/成本/大盘。 |
| A18 审计 | 🟠 架子 | audit_logs schema 有；无写入逻辑。 |
| A19 权限隔离 | 🟡 部分完成 | 用户资源查询有 userID 隔离；admin/C 端账号体系未隔离。 |
| A20 容器化 | 🟡 部分完成 | 本地依赖 compose 有；应用 Docker/集群/nginx 不完整。 |
| A21 auto 分辨率解析 | ✅ 核心完成 | `auto` 和 size 到 1k/2k/4k 的后端解析已做。 |
| A22 输出/参考图片数量识别 | ✅ 核心完成 | 计费和任务字段已区分输出图数与参考图数。 |
| A23 OpenAI 兼容接口 | ✅ 最小完成 | generate/edit/models 可用，Bearer sk 鉴权。 |
| A24 多平台路由 | ✅ 最小完成 | OpenAI/OpenRouter provider 支持已做；后台动态配置不足。 |
| A25 开发文档页面 | 🟠 架子 | OpenAPI spec 有；页面和示例代码未做。 |
| A26 错误归一化 | 🟡 部分完成 | platform error/provider mapping 有；错误文案/策略配置/全 handler 统一还不完整。 |

## 10. 技术方案任务 T01-T26 对照（忽略前端 UI）

| 任务 | 后端状态 | 说明 |
|---|---:|---|
| T01 仓库骨架与工程约定 | ✅ 完成 | Go module、cmd、internal/pkg 结构已成型。 |
| T02 本地开发依赖与容器编排 | 🟡 部分 | 依赖 compose 有；应用容器和完整部署未做。 |
| T03 配置体系与环境变量模型 | ✅ 基础完成 | YAML + env override + tests；动态 reload 未做。 |
| T04 通用协议、错误码与响应封装 | 🟡 部分 | `errs/httpx` 有；部分 handler 仍未完全统一。 |
| T05 OpenAPI 契约基线 | ✅ 当前范围完成 | 当前 runtime 接口有 spec 测试；完整 PRD API 未全覆盖。 |
| T06 可观测性底座 | 🟡 部分 | metrics/health/logger/recovery 有；业务指标/大盘无。 |
| T07 数据模型与 Ent Schema | 🟡 部分 | 核心 schema 多数有；provider_models、orders、subscriptions 等缺。 |
| T08 认证与会话域 | 🟡 部分 | 主链路有，生产级验证码/冷却/password/logout 缺。 |
| T09 计费、余额与用户分组倍率域 | ✅ 核心完成 | 不含充值/兑换/套餐/过期积分。 |
| T10 参考图资产与对象存储域 | 🟡 部分 | local upload/get/dedupe 有；S3/presigned/delete/expiry 缺。 |
| T11 模型能力矩阵、路由与配置域 | 🟡 部分 | config resolver 有；DB/后台/AB/健康检查缺。 |
| T12 Provider 抽象层 | ✅ 当前完成 | OpenAI + OpenRouter client 和错误映射有。 |
| T13 任务编排、状态机与 Worker 集群 | ✅ 核心完成 | lease/heartbeat/reclaim/settlement 已重点覆盖。 |
| T14 Open API 与 OpenAI Compat 接入层 | ✅ 最小完成 | Task 4 P0 + compat 完成；完整 Open API 仍缺。 |
| T15 管理后台后端能力 | 🟠 架子 | 只有 config-tabs；admin auth/users/model/tasks/dashboards 缺。 |
| T23 集群部署与发布资产 | 🟠 架子 | 本地依赖有；发布资产不足。 |
| T24 测试资产与 Provider Mock | 🟡 部分 | 单测/mock provider 丰富；E2E/mock server 未系统化。 |
| T25 联调与验收回归 | 🟠 架子 | `go test ./...` 可用；无真实联调/E2E 验收。 |
| T26 上线准备与交付文档 | ⚪ 未开始 | 无上线 runbook/交付文档/运维手册。 |

## 11. 推荐下一阶段后端优先级

### P0-1：API Key 管理闭环

目标：让 Open API 从“测试用 service primitive”变成真实用户可管理能力。

建议补齐：

- 用户侧创建 / 列表 / 重置 / 禁用 / 删除 API Key。
- Secret 只展示一次。
- quota / daily quota / RPM 字段真正生效。
- API Key 操作审计。

### P0-2：完整 Open API 查询能力

目标：让开发者链路从“能提交任务”变成“能查询完整结果”。

建议补齐：

- `GET /api/open/image/v1/tasks/{task_id}`
- `GET /api/open/image/v1/balance`
- `GET /api/open/image/v1/capabilities`
- `GET /api/open/image/v1/reference-assets/{asset_id}`

### P0-3：生成结果落库与历史图库后端

目标：让生成结果真正成为图库资产，而不是只停留在 provider response / task results。

建议补齐：

- provider 结果写入 `image_results`。
- 生成图片本地 / S3 存储。
- 下载接口。
- task detail 返回图片结果。
- 删除只影响图库展示，不影响流水。

### P0-4：生产级认证补齐

建议补齐：

- 真实邮件验证码发送。
- 发送冷却。
- 错误次数冷却。
- logout。
- token_version 吊销校验。
- 用户 disabled 状态联动 JWT 和 API Key。

### P0-5：管理员账号体系

建议补齐：

- admin login。
- admin token。
- Ops API 切换到 admin auth。
- C 端用户和管理员彻底隔离。

### P0-6：配置中心真实生效

建议补齐：

- 配置更新后影响新任务。
- billing / routing / generation limits 从配置 store 或缓存读取。
- 变更审计。
- 配置回滚 / 版本冲突完善。

### P0-7：兑换码与管理员调积分

建议补齐：

- redeem service + endpoint。
- 并发核销幂等。
- admin points adjustment endpoint。
- 审计日志。

### P0-8：模型接入后台

建议补齐：

- provider CRUD。
- route CRUD。
- AB weight。
- fallback chain。
- provider error policy 生效。
- provider health check。

### P0-9：部署交付

建议补齐：

- backend API Dockerfile。
- worker Dockerfile。
- nginx 配置。
- 单机部署文档。
- 集群部署约束文档。

### P1：支付 / 订阅 / 公开广场

支付和公开广场都不是当前后端主链路的阻塞项，可在生成、账户、Key、后台稳定后再做。
