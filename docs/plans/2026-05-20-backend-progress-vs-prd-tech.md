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
| ✅ 已准出完成 | 已有 runtime 路由、服务、持久化和测试；关键异常路径覆盖；当前范围可交付。 |
| 🟡 部分完成 | 主链路或 service 已有，但仍缺完整产品视图、后台 CRUD、动态配置、E2E 或生产增强。 |
| 🟠 架子完成 | 主要是 schema、配置、默认定义或 stub 存在，业务闭环还没形成。 |
| ⚪ 未开始 | 没有可用 service/handler，或仅存在 PRD/技术方案描述。 |

## 3. 一句话结论

本轮已经把原进度文档中 P0 后端主缺口大幅补齐：生产级认证保护、用户资料与偏好、API Key 生命周期、Canonical HMAC、Key quota/RPM、兑换码核销、Open API 查询能力、结果图片基础落库、Admin 独立 Token、审计 service、部署资产和安全启动校验都已进入可准出状态。

仍未完成的主要集中在：真实支付/订阅、完整后台运营 CRUD、Provider Model/Route/Error Policy 的 DB 驱动动态生效、公开广场/审核完整链路、OpenAPI 文档全量对齐、E2E/联调报告，以及更强的生产运维能力。

## 4. 本轮已补齐并可准出的后端能力

### 4.1 生产级认证、会话与用户资料

状态：✅ 已准出完成（当前后端范围）

已完成：

- 邮箱验证码改为随机 6 位码；生产禁用固定码/开发码/`issuer=test`。
- 增加邮件发送配置与 SMTP/dev-log 支持。
- Access token 校验用户存在、禁用状态与 `token_version`。
- 增加 logout、修改密码、重置密码。
- 增加 profile、preferences、avatar 更新接口。
- 用户禁用后阻断登录、refresh、JWT 访问、API Key 鉴权和任务创建。

主要代码：

- `internal/service/auth/service.go`
- `internal/repository/entstore/auth_store.go`
- `internal/http/handlers/api.go`
- `internal/http/handlers/api_extra.go`
- `internal/app/run.go`

### 4.2 API Key 生命周期、Canonical HMAC、quota/RPM

状态：✅ 已准出完成（当前后端范围）

已完成：

- 用户侧 API Key 列表、创建、更新、重置 secret、软删除。
- Secret 只在创建/重置返回；数据库保存 hash 与加密 signing material。
- 原生 Open API 强制 `X-Timestamp` + canonical HMAC：`HMAC-SHA256(secret, method + path + timestamp + body_sha256)`。
- OpenAI Compat 保留 Bearer `sk-*` 鉴权。
- API Key total quota、daily quota 在 billing reserve 同一事务中原子校验。
- API Key RPM 增加限流预检。
- 普通用户不能通过 `group_code` 越权改变计费/权限分组。
- 生产环境拒绝弱 API Key signing encryption key。

主要代码：

- `internal/domain/apikey/types.go`
- `internal/service/apikey/service.go`
- `internal/repository/entstore/apikey_store.go`
- `internal/repository/entstore/billing_store.go`
- `internal/http/router/api_keys_test.go`

### 4.3 计费、预扣、结算与兑换码

状态：✅ 已准出完成（不含支付/订阅）

已完成：

- 预估、预扣、结算、失败退款、部分成功按成功图数扣费。
- 账务 5 位小数。
- reserve/finalize/refund 幂等。
- wrong-user protection。
- API Key quota 使用已消费 + 活跃预扣视图，避免 finalize 双计。
- 兑换码核销：不存在、未生效、过期、禁用、次数超限、幂等重放校验。
- 兑换成功写入 `point_ledgers(redeem)`，关联 `redeem_code_id`。

主要代码：

- `internal/domain/billing/calculator.go`
- `internal/service/billing/service.go`
- `internal/service/billing/store.go`
- `internal/repository/entstore/billing_store.go`
- `internal/repository/entstore/redeem_store_test.go`

### 4.4 图片任务、Worker 与结果图库基础

状态：✅ 核心完成；🟡 图片级产品视图仍需增强

已完成：

- 任务创建、排队、Worker 抢占、lease、heartbeat、过期回收。
- Provider 成功/失败后落终态。
- 任务创建接入 `negative_prompt`、比例、seed、reference_strength、save policy 等字段。
- sync 模式基础执行/等待策略。
- Provider URL/b64 结果转存后端 storage。
- `task_images` 保存 storage/object/mime/size/width/height/sha256 等真实图库字段。
- 任务详情返回结果图对象。
- 图片下载接口具备路由与归属校验基础。

主要代码：

- `internal/service/imagetask/service.go`
- `internal/repository/entstore/imagetask_store.go`
- `internal/worker/runner.go`
- `internal/domain/imagetask/types.go`

仍需增强：

- 下载接口的真实流式返回/签名 URL 需要按 storage driver 完整落地。
- 图片筛选、分页、公开状态、审核状态仍不是完整产品视图。
- `partial_failed` 已有状态建模，但业务展示与 OpenAPI 示例仍需继续完善。

### 4.5 Open API / OpenAI Compat

状态：✅ 当前 P0 后端闭环完成；🟡 契约文档仍需补全

已完成路由：

- `POST /api/open/image/v1/reference-assets/uploads`
- `POST /api/open/image/v1/reference-assets`
- `GET /api/open/image/v1/reference-assets/{asset_id}`
- `POST /api/open/image/v1/tasks`
- `GET /api/open/image/v1/tasks/{task_id}`
- `GET /api/open/image/v1/balance`
- `GET /api/open/image/v1/capabilities`
- `GET /api/open/image/v1/estimate`
- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `GET /v1/models`

已完成能力：

- 原生 Open API 与 OpenAI Compat 均进入统一 task/billing/provider 链路。
- OpenAI Compat edit 的 multipart 图片进入参考资产/任务链路。
- API Key quota/RPM 对 Open API 与 Compat 生效。
- Canonical HMAC body/timestamp mismatch 测试覆盖。

仍需增强：

- OpenAPI YAML 尚未完整覆盖本轮新增所有 Agent/Ops/Docs 路由和 schema 示例。
- HMAC canonical 当前未包含 query string；若后续 query 参与敏感语义，建议纳入签名。

### 4.6 Admin auth、配置中心、审计与调用记录基础

状态：🟡 部分完成

已完成：

- 新增独立 admin login/logout。
- Ops config API 切换到 admin token。
- Admin JWT 生产环境要求显式强 secret，拒绝弱值和回退普通用户 token secret。
- `requireAdmin` 校验 issuer、subject、role、email、可选 admin id/email。
- 配置更新写审计基础。
- 新增 audit service/store skeleton。
- 新增 audit/call-records 查询 stub，确保路由与鉴权基础存在。

主要代码：

- `internal/http/handlers/api_extra.go`
- `internal/service/audit/service.go`
- `internal/repository/entstore/audit_store.go`
- `internal/service/adminconfig/service.go`

仍需增强：

- Admin users 表驱动登录、session/refresh、密码 hash 与后台账号管理尚未完整产品化。
- 用户管理、模型供应商、Provider Model、路由、错误策略、运营大盘等后台 CRUD 尚未补齐。
- audit/call-records 当前是基础/stub 查询，还不是完整运营检索。

### 4.7 部署资产与生产启动保护

状态：✅ P0 部署骨架完成；🟡 集群运维仍需增强

已完成：

- `Dockerfile.api`
- `Dockerfile.worker`
- `.dockerignore`
- `deployments/docker-compose/docker-compose.prod.yml`
- `deployments/docker-compose/.env.example`
- `deployments/nginx/pic-gallery.conf`
- `docs/deploy/backend-runbook.md`
- `CGO_ENABLED=0` API/Worker 构建通过。
- Docker API/Worker 镜像构建通过。
- 生产启动路径拒绝弱 API Key signing encryption key、弱 admin token secret、固定验证码/开发验证码。

仍需增强：

- Compose 冒烟、MinIO/S3 生产配置、Nginx TLS、监控告警和值班说明还需补齐。
- local SQLite 在 CGO disabled 下显式不可用，生产应使用 Postgres。

## 5. PRD A1-A26 当前后端状态

| PRD 验收项 | 后端状态 | 当前说明 |
|---|---:|---|
| A1 邮箱注册/登录 | ✅ | 随机验证码、邮件发送接口、生产固定码熔断、登录注册主链路完成；Redis 分布式验证码/冷却仍可增强。 |
| A2 双 Token 会话 | ✅ | Access/Refresh、刷新轮换、logout、token_version、禁用阻断已完成。 |
| A3 用户资料 | ✅ | Profile、Preferences、Avatar 后端接口已完成。 |
| A4 额度展示 | 🟡 | balance/ledger 可用；套餐、过期积分、充值来源拆分未做。 |
| A5 兑换码 | 🟡 | 用户核销与账务关联完成；后台批量生成/导出/失效未做。 |
| A6 API Key | ✅ | 生命周期、secret 一次展示、HMAC、quota、daily quota、RPM、禁用/删除完成；后台 Key 管理仍未做。 |
| A7 图片生成成功 | ✅ | 文生图/图生图/参考图任务主链路、结果落 storage/task_images 完成。 |
| A8 计费准确 | ✅ | 预估、预扣、结算、失败退款、5 位小数、并发幂等已覆盖。 |
| A9 参数边界 | 🟡 | 余额、模型、数量、参考图、Key quota/RPM、禁用用户等核心边界完成；平台级/用户级 Redis 限流仍需增强。 |
| A10 数量上限 | 🟡 | config resolver 校验已有；后台调低后 runtime 动态生效仍需完整化。 |
| A11 同步/异步 | 🟡 | 异步 Worker 完成，sync 模式基础完成；HTTP 长轮询与超时体验仍需打磨。 |
| A12 历史图库 | 🟡 | task list/detail/delete 与 task_images 基础完成；图片级下载、筛选分页、公开状态仍需增强。 |
| A13 后台模型配置 | 🟠 | config-driven routing 有；Provider Model/Route/Error Policy CRUD 与 DB 动态生效未完成。 |
| A14 后台积分策略 | 🟡 | 计费配置和 config tab 有；完整后台策略管理、审计查询和动态快照仍需增强。 |
| A15 系统配置中心 | 🟡 | config-tabs GET/PUT + admin auth + 审计基础完成；运行时快照/回滚/发布仍需增强。 |
| A16 用户管理 | ⚪ | 无完整 admin 用户管理 API。 |
| A17 调用记录 | 🟠 | call-records stub 有；筛选、成本、耗时、大盘未完成。 |
| A18 审计 | 🟡 | audit service/store 与部分写入基础完成；全量动作覆盖和查询筛选未完成。 |
| A19 权限隔离 | ✅ | 用户资源隔离、admin token 隔离、API Key group_code 越权修复完成。 |
| A20 容器化 | ✅ | API/Worker Dockerfile、prod compose、nginx 示例、runbook 完成。 |
| A21 auto 分辨率解析 | ✅ | `auto` 和 size 到 1k/2k/4k 的后端解析已做。 |
| A22 输出/参考图片数量识别 | ✅ | 计费和任务字段已区分输出图数与参考图数。 |
| A23 OpenAI 兼容接口 | ✅ | generate/edit/models、Bearer sk、统一任务链路、Key quota/RPM 完成。 |
| A24 多平台路由 | 🟡 | OpenAI/OpenRouter provider 支持已做；后台动态路由/健康检查/成本字段未完成。 |
| A25 开发文档页面 | 🟡 | `/docs/openapi.*`、examples/errors 基础路由完成；正式文档门户和完整示例未完成。 |
| A26 错误归一化 | 🟡 | `errs/httpx`、provider mapping 有；i18n、扣费建议、error policy 动态生效未完整。 |

## 6. 技术方案 T01-T26 当前后端状态（忽略前端 UI）

| 任务 | 后端状态 | 当前说明 |
|---|---:|---|
| T01 仓库骨架与工程约定 | ✅ | Go module、cmd、internal/pkg 结构已成型。 |
| T02 本地开发依赖与容器编排 | ✅ | dev compose 与 prod compose 均已有基础。 |
| T03 配置体系与环境变量模型 | ✅ | YAML + env override + 测试；生产关键 secret fail-closed。 |
| T04 通用协议、错误码与响应封装 | 🟡 | `errs/httpx` 有；少量静态文件/legacy 路径仍可继续统一。 |
| T05 OpenAPI 契约基线 | 🟡 | runtime 路由丰富，但 YAML 未完整覆盖本轮全部新增路由。 |
| T06 可观测性底座 | 🟡 | metrics/health/logger/recovery 有；业务指标/大盘无。 |
| T07 数据模型与 Ent Schema | 🟡 | 核心 schema 多数有；provider_models、orders、subscriptions 等缺。 |
| T08 认证与会话域 | ✅ | 当前 P0 后端准出完成。 |
| T09 计费、余额与用户分组倍率域 | ✅ | 不含支付/订阅/过期积分。 |
| T10 参考图资产与对象存储域 | 🟡 | local upload/get/dedupe 有；S3/presigned/delete/expiry 仍需增强。 |
| T11 模型能力矩阵、路由与配置域 | 🟡 | config resolver 有；DB/后台/AB/健康检查缺。 |
| T12 Provider 抽象层 | ✅ | OpenAI + OpenRouter client 和错误映射有。 |
| T13 任务编排、状态机与 Worker 集群 | ✅ | lease/heartbeat/reclaim/settlement 已覆盖。 |
| T14 Open API 与 OpenAI Compat 接入层 | ✅ | P0 后端 runtime 完成；OpenAPI YAML 仍需补齐。 |
| T15 管理后台后端能力 | 🟡 | admin auth/config/audit/call-record stub 有；用户/模型/运营 CRUD 缺。 |
| T23 集群部署与发布资产 | ✅ | API/Worker Docker、prod compose、nginx、runbook 已补。 |
| T24 测试资产与 Provider Mock | 🟡 | 单测和 mock provider 丰富；E2E/mock server 未系统化。 |
| T25 联调与验收回归 | 🟠 | 自动化测试可用；无完整联调记录和验收报告。 |
| T26 上线准备与交付文档 | 🟡 | backend runbook 有；容量评估、告警值班、回滚演练仍需补。 |

## 7. 目前仍未完成 / 后续待办清单

### P0 后端准出后的必要收尾

1. **OpenAPI YAML 全量对齐**
   - 补齐本轮新增 Agent/Open/Ops/Docs 路由、schema、security、错误码和示例。
   - 增加更严格的 spec paths 与 runtime router 对齐测试。

2. **图片下载与图库产品视图增强**
   - 下载接口按 local/S3 storage driver 做真实流式返回或签名 URL。
   - task/image 列表增加状态、任务类型、模型、时间范围筛选和分页。
   - 历史删除、图片可见性、公开状态与审计联动继续补齐。

3. **配置运行时快照与动态生效**
   - 建立 `runtimeconfig` snapshot/cache。
   - Billing calculator、capabilities、model resolver、provider registry 从 runtime snapshot 读取。
   - 配置保存后 1 分钟内影响新任务，历史任务快照不回算。

4. **审计覆盖率补齐**
   - 登录、刷新、登出、改密、API Key、任务、计费、兑换码、后台配置、模型、用户管理写操作全覆盖。
   - 审计查询支持 actor/action/target/result/time 分页筛选。

### 后台能力待补

5. **管理员账号体系产品化**
   - 从环境变量 bootstrap 迁移到 `admin_users` 表驱动账号。
   - 增加 admin password hash、session/refresh、禁用、last_login、token_version。
   - 后台登录限流与审计。

6. **用户管理 API**
   - 用户列表筛选。
   - 禁用/启用、重置密码、设置用户组、RPM、并发。
   - 管理员手动调账 endpoint 与原因必填。

7. **模型供应商 / Provider Model / Route / Error Policy CRUD**
   - Provider 密钥加密/脱敏、健康检查、禁用跳过。
   - Provider Model 维护任务类型、质量、比例、数量、参考图、mask、成本字段。
   - Route 权重、AB、fallback order 校验。
   - Error Policy 控制 retry/wrap/block/sanitize。

8. **调用记录与运营大盘**
   - 基于 `image_tasks + task_images + point_ledgers` 聚合调用记录。
   - 支持用户、模型、状态、时间、API Key 筛选。
   - 成功率、失败率、耗时、成本/毛利大盘。

### 商业化 / P1 能力待补

9. **支付、订单、订阅套餐**
   - 订单与支付 schema。
   - 支付 webhook。
   - 套餐权益、月额度、过期积分。
   - 当前仍以兑换码和管理员调账作为 MVP 资金/额度闭环。

10. **公开广场与审核完整链路**
    - 用户公开申请。
    - 管理员审核、下架。
    - 公开图片列表/瀑布流 API。
    - 内容安全与审核审计。

11. **开发者文档门户**
    - 正式 API 文档页面。
    - curl / Python / TypeScript 可执行示例。
    - 错误码、签名说明、SDK 兼容说明。

### 测试与交付待补

12. **E2E / 联调回归**
    - Mock provider 冒烟：登录、兑换、创建任务、worker 产图、查询、下载。
    - OpenAI SDK 兼容脚本。
    - 后台配置保存后动态生效测试。

13. **生产运维补强**
    - `AUTH_ACCESS_TOKEN_SECRET` 启动期弱值熔断。
    - Postgres/Redis/对象存储高可用说明。
    - 告警阈值、值班手册、容量评估、回滚演练。

## 8. 当前可真实使用的后端接口清单

### 基础接口

- `GET /`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### Agent/Auth

- `POST /api/agent/auth/v1/email/send-code`
- `POST /api/agent/auth/v1/login/email-code`
- `POST /api/agent/auth/v1/session/refresh`
- `POST /api/agent/auth/v1/logout`
- `POST /api/agent/auth/v1/password/change`
- `POST /api/agent/auth/v1/password/reset`

### Agent/User

- `GET /api/agent/user/v1/profile`
- `PUT /api/agent/user/v1/profile`
- `PUT /api/agent/user/v1/preferences`
- `POST /api/agent/user/v1/avatar`

### Agent/Billing

- `GET /api/agent/billing/v1/balance`
- `GET /api/agent/billing/v1/ledger`
- `GET /api/agent/billing/v1/estimate`
- `POST /api/agent/billing/v1/redeem-codes/redeem`

### Agent/Developer

- `GET /api/agent/developer/v1/api-keys`
- `POST /api/agent/developer/v1/api-keys`
- `PATCH /api/agent/developer/v1/api-keys/{key_id}`
- `POST /api/agent/developer/v1/api-keys/{key_id}/reset-secret`
- `DELETE /api/agent/developer/v1/api-keys/{key_id}`

### Agent/Image

- `GET /api/agent/image/v1/capabilities`
- `POST /api/agent/image/v1/reference-assets`
- `GET /api/agent/image/v1/reference-assets/{asset_id}`
- `GET /api/agent/image/v1/images/{image_id}/download`
- `POST /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks`
- `GET /api/agent/image/v1/tasks/{task_id}`
- `GET /api/agent/image/v1/history/tasks`
- `GET /api/agent/image/v1/history/tasks/{task_id}`
- `DELETE /api/agent/image/v1/history/tasks/{task_id}`

### Open Image API

- `POST /api/open/image/v1/reference-assets/uploads`
- `POST /api/open/image/v1/reference-assets`
- `GET /api/open/image/v1/reference-assets/{asset_id}`
- `POST /api/open/image/v1/tasks`
- `GET /api/open/image/v1/tasks/{task_id}`
- `GET /api/open/image/v1/balance`
- `GET /api/open/image/v1/capabilities`
- `GET /api/open/image/v1/estimate`

### OpenAI Compat

- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `GET /v1/models`

### Ops/Admin

- `POST /api/ops/admin/v1/auth/login`
- `POST /api/ops/admin/v1/auth/logout`
- `GET /api/ops/admin/v1/config-tabs`
- `PUT /api/ops/admin/v1/config-tabs/{tab_key}`
- `GET /api/ops/admin/v1/audit-logs`
- `GET /api/ops/admin/v1/call-records`

### Docs

- `GET /docs/openapi.yaml`
- `GET /docs/openapi.json`
- `GET /docs/examples`
- `GET /docs/errors`

## 9. 结论

截至 2026-05-21，本分支后端已达到“P0 后端准出”的主要目标：生成、计费、鉴权、Key 管理、Open API、Compat API、基础后台、部署骨架与安全启动保护均已形成可验证闭环。

后续不应再把主要精力放在后端主链路补洞，而应转向三类收尾：

1. OpenAPI/文档/E2E 交付资产对齐。
2. 后台运营能力和动态配置产品化。
3. 支付订阅、公开广场等商业化/P1 能力。
