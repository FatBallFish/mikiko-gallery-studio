# Pic Gallery 后端能力收口与前端/API 对接状态

更新日期：2026-05-23

## 1. 结论

基于以下资料完成对齐与实现：

- PRD：`docs/prd/pic-gallery-prd.md`
- 技术方案：`docs/tech/pic-gallery-tech-design.md`
- OpenAPI：`api/openapi/openapi.yaml`

截至本次更新，原文档中列出的后端 P0、P0-followup 与本轮要求一并收口的 P1 公开链路，均已落为可运行实现。当前仓库不再存在“只搭架子、尚未落地接口、主要链路未闭环”的后端缺口项；此前差距分析文档中的未完成结论，自本文件起全部作废，以本文件为准。

## 2. 已收口范围

### 2.1 账户、用户运营与 Redis 热路径

- 邮箱验证码登录、密码登录、刷新、退出、修改密码、忘记密码请求与确认、账号关闭已完成。
- 管理后台用户启停、重置密码、RPM/并发设置、用户组分配与用户组 CRUD 已完成。
- Redis 运行时已接入 API/Worker 装配：
  - 本地环境允许降级。
  - 非本地环境要求 Redis 可用。
  - 邮箱验证码 cooldown、验证码存储、refresh token session 热路径、refresh family replay block 已落 Redis runtime。

### 2.2 计费、订单、套餐与支付

- 钱包批次模型已落地：`wallet_grants`、`wallet_reservation_allocations`、`point_ledgers` 审计流水协同工作。
- 套餐、订阅、订单、webhook 事件已落地：`subscription_plans`、`user_subscriptions`、`payment_orders`、`payment_webhook_events`。
- 用户接口已完成：
  - `GET /api/agent/billing/v1/plans`
  - `GET /api/agent/billing/v1/subscription`
  - `GET/POST /api/agent/billing/v1/orders`
  - `GET /api/agent/billing/v1/orders/{order_id}`
  - `POST /api/agent/billing/v1/orders/{order_id}/cancel`
- 支付 webhook 已完成：
  - `POST /api/open/image/v1/payments/webhooks/alipay`
  - `POST /api/open/image/v1/payments/webhooks/wxpay`
- 余额视图已扩展为前端可直接展示的结构，包含订阅积分、赠送积分、充值积分、冻结积分、当前订阅、最近过期 grant。

### 2.3 模型接入、路由与成本字段

- `provider_models` 已落地 schema、store、service、后台 CRUD 与 OpenAPI。
- runtime resolver 已支持：
  - capability 过滤
  - health 状态过滤
  - 同 priority bucket 稳定哈希权重选择
  - fallback 顺序控制
- 任务与调用记录已补齐：
  - `provider_model_id`
  - `provider_cost`
  - `gross_margin`
  - `fallback_count`
  - `route_snapshot_version`

### 2.4 存储、远程结果镜像、公开审核与广场

- 存储驱动已升级为 `local + s3-compatible` 双后端。
- API/Worker 统一通过对象存储抽象读写，不再直接耦合本地文件系统。
- provider 返回远程 URL 结果时，后端会先拉取并镜像到平台存储，再对外暴露平台图片下载链路；不再维持“远程 URL 只落库不入存储”的旧行为。
- 公开链路已完成：
  - `POST /api/agent/gallery/v1/images/{image_id}/publish`
  - `GET /api/open/image/v1/gallery/images`
  - `GET /api/open/image/v1/gallery/images/{image_id}`
  - `GET /api/ops/admin/v1/image-reviews`
  - `POST /api/ops/admin/v1/image-reviews/{image_id}:approve`
  - `POST /api/ops/admin/v1/image-reviews/{image_id}:reject`
  - `POST /api/ops/admin/v1/image-reviews/{image_id}:unpublish`
- 机审 + 人工审核链路已生效；公开图片仅在审核通过且广场开关开启时可匿名读取。

### 2.5 审计、后台大盘与文档契约

- 审计日志已支持分页与筛选查询，不再是固定最近 200 条视图。
- 后台大盘接口已完成：`GET /api/ops/admin/v1/metrics/dashboard`
- OpenAPI 已同步新增账户密码流、用户组、billing 订单/套餐/订阅/支付 webhook、provider-model、gallery 审核/公开与 dashboard 契约。
- 历史任务/图片结果契约已补充 `provider_model_id`、`download_url`、`provider_cost`、`visibility_status`、`review_reason`、`published_at`、`storage_driver` 等关键字段。

## 3. 当前前端/API 可对接结论

用户侧、Open API、OpenAI 兼容接口、后台管理接口均可直接按当前 OpenAPI 与 Router 实现对接。此次收口后，不再存在需要前端绕开的“后端占位接口”或“文档有、实现没有”的主路径问题。

唯一需要注意的是：

- 本地开发若未启动 Redis，会按本地容错策略降级，不影响开发联调。
- 非本地环境必须提供可用 Redis 与合法存储配置。
- 若切换到 `storage.driver=s3`，需同步提供 `storage.s3.*` 配置。

## 4. 验证结果

本轮完成后已通过：

- `go test ./...`
- `./scripts/workflow/verify.sh`
- `./scripts/workflow/api-smoke.sh`

## 5. 状态声明

本文件将 2026-05-22 版本中的“后端功能缺口盘点”正式收口为“后端已完成，对接可用”状态。后续若再出现新增需求或范围扩展，应新开变更文档，不再沿用“剩余后端 gap”口径描述当前仓库。
