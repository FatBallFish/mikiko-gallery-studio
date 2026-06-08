# 管理后台安全配置与积分包配置开发方案

日期：2026-06-08

## 0. 实施进度

> 本区块用于记录当前实现进度，后续每完成一个事项同步更新，避免重复排查或重复实现。

| 状态 | 事项 | 当前结论 |
|---|---|---|
| 已完成 | `IMAGE_STORAGE_FAILED` 高优先级故障复核 | 2026-06-08 在 Docker dev 部署中用用户 `893721708@qq.com` 新建任务 `14a36cd7-c2ee-4f43-8ed9-96ebd01258de`，任务成功落盘并返回本地存储图片 `f2171359-3d19-44ae-9f30-3bf85608ec54`。18:31-18:51 的 `IMAGE_STORAGE_FAILED / failed to fetch generated image` 为修复前旧任务残留；当前 worker 已支持 provider 在 `url` 字段返回 `data:image/...;base64,...` 时按 base64 持久化。 |
| 已完成 | 支付渠道实例 write-only secret 后端语义 | 已新增 `ProviderInstanceWriteRequest`，支持 `config` 非敏感字段、`secrets` 旋转敏感字段、`clear_secrets` 清空敏感字段；更新非敏感字段时保留旧 secret；响应不回传 secret 明文。 |
| 已完成 | SMTP 安全配置后端底座 | 已新增 `secure_configs`、`secretcodec`、`secureconfig` service/store、SMTP Admin API、动态 SMTP resolver，并完成相关 Go 测试。 |
| 已完成 | 阶段 5：参考图预览与上传大小体验 | 已补 `ReferenceAsset.preview_url/download_url`、`referenceAssetPayload`、`IMAGE_REFERENCE_TOO_LARGE` 错误码及 details、capabilities 的 `reference_image_max_mb/reference_image_max_bytes`；用户端上传区展示单张最大大小，上传前拦截超限文件，缩略图使用 `preview_url || download_url` 并避免空 `src`。已验证 `go test ./internal/service/assets ./internal/service/capabilities ./internal/http/router -run 'TestUploadReturnsTooLargeErrorWithSizeDetails|TestCapabilitiesExposeConfiguredModelGroups|TestReferenceAssetDownloadAcceptsQueryToken' -count=1` 和 `npm --prefix web/user run typecheck`。 |
| 已完成 | 阶段 5：失败任务重试/删除 | 已新增 `POST /api/agent/image/v1/history/tasks/{task_id}/retry`，service 层 `RetryTask` 会复制失败任务核心参数创建新 queued 任务，原任务保持不变；用户端失败记录已增加“重试”“删除”按钮，删除复用已有 DELETE 历史任务接口。已验证 `go test ./internal/service/imagetask ./internal/http/router -run 'TestRetryTaskCreatesQueuedCopyFromFailedTask|TestAgentHistoryTaskRetryCreatesQueuedTask' -count=1` 和 `npm --prefix web/user run typecheck`。 |
| 已完成 | 阶段 5：调用记录错误明细和底层账号可观测 | 已扩展 `domainimagetask.Attempt`、Admin 调用记录响应中的 `account_model_id/model_account_id/upstream_model_code/error_detail/attempts`；真实执行链路会记录每次 provider 尝试的账号、上游模型、时间戳和 `UpstreamError` 结构化详情；管理后台调用记录支持展开查看 attempts 与错误明细。已验证 `go test ./internal/repository/entstore ./internal/service/imagetask ./internal/http/router -run 'TestAdminCallRecordStoreListsImageTasksWithFilters|TestExecuteFallsBackOnRetryableProviderError|TestAdminCallRecordsEndpointListsRealImageTasks' -count=1`、`npm --prefix web/admin run typecheck` 和 `npm --prefix web/admin run build`。 |
| 已完成 | 管理后台 SMTP 页面与支付实例 secret 表单 | 已新增管理后台 `/#/security-config` 安全配置页，接入 `GET/PUT /api/ops/admin/v1/security/smtp` 与 `POST /api/ops/admin/v1/security/smtp/test`；SMTP 密码按 write-only 语义处理，留空保留旧密码，勾选清空才发送 `clear_secrets`。收银台渠道实例弹窗已拆分“渠道配置 JSON”和“密钥 JSON/清空字段”，提交时自动把敏感 key 拆入 `secrets`，避免密钥混入可回显 config 或被脱敏响应覆盖。已验证 `npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build`、`go test ./internal/http/router ./internal/service/cashier ./internal/repository/entstore -run 'TestAdminSecuritySMTPConfigWriteOnlySecret|TestAdminCashierProviderInstance|ProviderInstance.*Secret|TestProviderInstancePayloadRedactsSecretConfig' -count=1`。 |
| 已完成 | OpenAPI、README 与最终验证 | 已补齐 OpenAPI 契约：`GET/PUT /api/ops/admin/v1/security/smtp`、`POST /api/ops/admin/v1/security/smtp/test`、支付渠道实例 `secrets/clear_secrets/credentials_status.secret_fields`、调用记录 `account_model_id/model_account_id/upstream_model_code/error_detail/attempts`；已更新 README、中文 README、部署 runbook、Compose env 示例和 dev/prod/e2e Compose 的 `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`。已修复 contract runner 下 `import.meta.env` 缺失导致的 shared HTTP client 初始化问题，并修正 API smoke readiness 断言。验证结果：`go test ./api/openapi -count=1`、`docker compose ... config` dev/prod/e2e、`./scripts/workflow/verify.sh`、`./scripts/workflow/api-smoke.sh` 均通过；用户已授权将 heavyweight approval 改为 approved，`./scripts/workflow/review-local.sh --scope committed` 返回 PASS，`./scripts/workflow/check-review-gate.sh` 返回 OK，`.review/gate.json` 为 committed scope PASS 且无 findings。 |

## 1. 背景与目标

当前项目已经具备收银台 MVP 能力：用户端从 `/api/agent/cashier/v1/options` 获取可购买积分包、自定义充值配置和可见支付方式；管理后台 `CashierPage` 已有充值套餐、支付方式、支付渠道实例、订单和回调事件页面；后端已有支付渠道实例表 `payment_provider_instances.config_encrypted`，运行时通过 `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY` 加密支付渠道配置。

这次改造要把以下能力闭环：

1. 支付配置在管理后台可配置，包括可见支付方式、支付渠道实例、商户号、密钥、回调相关参数、多账号调度策略。
2. SMTP 发信服务器配置在管理后台可配置，并被登录/注册/密码重置验证码发送链路实时使用。
3. 支付密钥、SMTP 密码、私钥、token 等敏感字段必须加密存储，禁止明文入库，禁止查询接口明文回传。
4. 固定积分包的种类、价格、积分数、上下架状态在管理后台可配置，并继续驱动用户端 `/api/agent/cashier/v1/options`。

本方案按用户已确认的安全口径 A 执行：

- 管理员可以在 HTTPS 管理后台创建/更新配置时一次性提交明文密钥。
- 后端收到请求后立即加密入库。
- 后续任何 GET/list/detail 接口不返回明文密钥，只返回 `has_secret`、`fingerprint`、`masked`、`updated_at` 等状态字段。
- 生产部署必须通过 HTTPS/反代 TLS 访问管理后台和 Admin API。

## 2. 当前实现基线

### 2.1 用户端积分包来源

文件：`internal/http/handlers/api.go`

- `HandleCashierOptions` 处理 `GET /api/agent/cashier/v1/options`。
- 当前逻辑调用 `a.billing.ListPlans(ctx)`，再用 `isPurchasableCashierPlan(plan)` 过滤可购买套餐。
- 响应包含：
  - `plans`
  - `custom_amount`
  - `visible_methods`
  - `order_timeout_seconds`

结论：固定积分包不需要新建用户端接口。管理后台只要维护 `subscription_plans`，用户端 options 会自然读取到最新可购买积分包。

### 2.2 管理后台积分包入口

文件：`web/admin/src/pages/CashierPage.tsx`

- 路由：`/#/cashier`
- Tab：`plans`，中文显示为“充值套餐”。
- 当前调用：
  - `adminApi.listCashierPlans()`
  - `adminApi.createCashierPlan(input)`
  - `adminApi.updateCashierPlan(plan_id, input)`
  - `adminApi.deleteCashierPlan(plan_id)`
- 后端接口：
  - `GET /api/ops/admin/v1/cashier/plans`
  - `POST /api/ops/admin/v1/cashier/plans`
  - `PUT /api/ops/admin/v1/cashier/plans/{plan_id}`
  - `DELETE /api/ops/admin/v1/cashier/plans/{plan_id}`

结论：固定积分包后台配置已经有基础能力，本次需要收敛字段语义、默认只开放 `points_package`、保留 `subscription` 定义但不开放购买。

### 2.3 支付渠道实例入口

文件：`web/admin/src/pages/CashierPage.tsx`

- 路由：`/#/cashier`
- Tab：`instances`，中文显示为“渠道实例”。
- 当前编辑弹窗使用 `config_text` JSON 保存商户号、密钥、网关地址等配置。
- 当前调用：
  - `adminApi.listPaymentProviderInstances()`
  - `adminApi.createPaymentProviderInstance(input)`
  - `adminApi.updatePaymentProviderInstance(instance_id, input)`
  - `adminApi.deletePaymentProviderInstance(instance_id)`
- 后端接口：
  - `GET /api/ops/admin/v1/cashier/provider-instances`
  - `POST /api/ops/admin/v1/cashier/provider-instances`
  - `GET /api/ops/admin/v1/cashier/provider-instances/{instance_id}`
  - `PUT /api/ops/admin/v1/cashier/provider-instances/{instance_id}`
  - `DELETE /api/ops/admin/v1/cashier/provider-instances/{instance_id}`

后端 `internal/repository/entstore/cashier_store.go` 已经在真实数据库 store 中使用 AES-GCM 加密 `config`，字段为 `config_encrypted`。

当前主要缺口：

- `ProviderInstancePayload` 虽已通过 `RedactProviderConfig` 移除 secret key，但前端编辑旧实例时拿到的是“脱敏后的 config”，直接保存会覆盖原有 secret。
- `PaymentProviderInstanceWriteRequest` 仍然把 `config` 当普通对象表达，没有明确 write-only secret 语义。
- 缺少“保留旧密钥、旋转密钥、清空密钥”的 PATCH/PUT 契约。

### 2.4 SMTP 配置来源

文件：

- `internal/config/config.go`
- `internal/config/load.go`
- `internal/service/auth/service.go`
- `internal/app/run.go`

当前 SMTP 配置来自 YAML/env：

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `SMTP_STARTTLS`
- `SMTP_INSECURE_SKIP_VERIFY`

`authservice.NewServiceWithStoreAndRedis` 初始化时，如果 `smtpConfigured(cfg.SMTP)` 为 true，就创建固定的 `SMTPEmailSender`。这意味着运行时不会读取后台新配置。

当前主要缺口：

- 管理后台无 SMTP 配置页。
- SMTP 密码只能通过 env/YAML 明文进入进程配置。
- Auth 服务启动后 SMTP sender 固定，不能动态读取后台配置。

### 2.5 配置中心现状

文件：

- `internal/service/adminconfig/service.go`
- `internal/repository/ent/schema/configitem.go`
- `web/admin/src/pages/ConfigPage.tsx`
- `web/admin/src/pages/configRows.ts`

系统配置表为 `system_configs`，字段 `config_value` 是普通 JSON，不适合直接保存 SMTP 密码等敏感信息。当前 ConfigPage 是通用配置编辑器，适合非敏感配置，不适合承载 write-only secret 字段。

结论：SMTP 安全配置不应简单塞进通用 ConfigPage 的普通 JSON 字段，应新增安全配置服务和专用 Admin API。

## 3. 总体设计

采用“业务配置与敏感配置分离”的方案。

1. 积分包继续使用现有 `subscription_plans` 和 Cashier plans 接口。
2. 支付渠道实例继续使用 `payment_provider_instances`，但补齐 write-only secret 更新契约，避免脱敏响应覆盖密钥。
3. SMTP 新增安全配置存储 `secure_configs`，由专用 `secureconfig` 服务加密存取。
4. Auth 邮件发送链路改为运行时读取 SMTP resolver，优先使用后台安全配置，未配置时回退 env/YAML。
5. 管理后台新增“邮件服务”配置页或 Cashier 外的系统设置入口；支付配置继续在“收银台”页面完成，避免混入通用 ConfigPage。

推荐页面结构：

- `/#/cashier`
  - `plans`：固定积分包和暂不开放的订阅套餐定义。
  - `methods`：用户端可见支付方式及调度策略。
  - `instances`：支付渠道账号实例和密钥配置。
- `/#/config`
  - 继续承载非敏感系统配置。
- 新增 `/#/security-config` 或在 `/#/config` 内新增专用 “邮件服务”区块，但不要复用通用 JSON 表单提交 secret。
  - 推荐新增 `SecurityConfigPage.tsx`，首期只放 SMTP。

## 4. 敏感配置安全契约

### 4.1 字段分类

非敏感字段可以查询回显：

- 支付：`provider_type`、`name`、`enabled`、`supported_methods`、`sort_order`、`scheduler_weight`、`limits`、`gateway_url`、`app_id`、`mch_id` 等。
- SMTP：`enabled`、`host`、`port`、`username`、`from`、`starttls`、`insecure_skip_verify`。

敏感字段只写不读：

- 通用：包含 `secret`、`private_key`、`token`、`api_key`、`password`、`mch_key`、`api_v3_key`、`pkey`、`cert` 等语义的字段。
- 支付：`private_key`、`alipay_public_key` 如按密钥管理、`mch_key`、`api_v3_key`、`key`、`secret`、`notify_secret`、`sign_key`。
- SMTP：`password`。

### 4.2 查询响应契约

所有 GET/list/detail 接口禁止返回敏感字段明文。

敏感状态统一返回：

```json
{
  "credentials_status": {
    "has_secret": true,
    "fingerprint": "sha256:4f2a9c1e7b0d3301",
    "updated_at": "2026-06-08T10:00:00Z",
    "secret_fields": ["password"]
  }
}
```

支付实例响应继续保留 `config`，但其中只能包含非敏感字段。SMTP 响应使用 `smtp` 对象加 `secret_status`。

### 4.3 写入请求契约

写入请求允许携带敏感字段，但只用于创建或旋转。

更新时采用三态语义：

- 未传 `secrets`：保留旧密钥。
- 传 `secrets.password = "new-value"`：更新/旋转密钥。
- 传 `clear_secrets = ["password"]`：清空指定密钥，仅在业务允许时支持。

禁止使用脱敏占位符更新密钥：

- 前端不得发送 `"******"`、`"********"` 之类占位符。
- 后端若收到常见 mask 值，应返回 `400 invalid_secret_placeholder`。

### 4.4 存储契约

统一使用服务端加密 envelope：

```json
{
  "ciphertext": "v1:<base64url(nonce+ciphertext)>",
  "fingerprint": "sha256:4f2a9c1e7b0d3301",
  "secret_fields": ["password"],
  "updated_at": "2026-06-08T10:00:00Z"
}
```

加密算法：

- AES-256-GCM。
- nonce 每次随机生成。
- 主密钥从环境变量读取。

主密钥配置：

- 复用并扩展现有 `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY` 的模式。
- 新增通用密钥：
  - `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`
  - 或兼容 `SECURE_CONFIG_ENCRYPTION_KEY`
- 生产环境如果未设置或仍为 local-dev 默认值，启动失败。

### 4.5 审计契约

审计日志只记录：

- 配置类型。
- 资源 ID。
- 更新人。
- 是否新增/旋转/清空 secret。
- fingerprint 前 16 hex。

审计日志不得记录明文 secret、完整密文、完整配置 JSON。

## 5. 后端设计

### 5.1 新增通用 SecretCodec

新增包建议：

- `internal/service/secretcodec`

职责：

- `EncryptJSON(value map[string]any) (EncryptedEnvelope, error)`
- `DecryptJSON(envelope map[string]any) (map[string]any, error)`
- `Fingerprint(value map[string]any, secretKeys []string) string`
- `Redact(value map[string]any, secretMatcher func(string) bool) map[string]any`

支付现有 `cashierConfigAEAD` 可迁移/复用到该包，避免 SMTP 再复制一份 AES-GCM。

兼容要求：

- 已存在的 `payment_provider_instances.config_encrypted` 中 `{"ciphertext":"v1:..."}` 必须继续可解。
- 空 key 的历史测试路径可以保留在非生产环境，但生产必须强制配置主密钥。

### 5.2 新增安全配置表

新增 Ent schema：

文件：`internal/repository/ent/schema/secureconfig.go`

表名：`secure_configs`

字段：

- `id`
- `config_category` string，例：`smtp`
- `config_key` string，例：`default`
- `public_value` JSON，保存非敏感配置。
- `secret_encrypted` JSON，保存加密 envelope。
- `secret_fingerprint` string。
- `secret_fields` JSON string array。
- `version` int64。
- `updated_by` int64。
- `created_at`
- `updated_at`

唯一索引：

- `(config_category, config_key)`

为什么不直接用 `system_configs`：

- `system_configs.config_value` 是普通 JSON，当前 ConfigPage 会完整读写，容易误回显/误覆盖 secret。
- 安全配置需要 write-only、fingerprint、secret 三态更新、独立权限和审计。

### 5.3 新增 secureconfig 服务

新增包建议：

- `internal/domain/secureconfig`
- `internal/service/secureconfig`
- `internal/repository/entstore/secure_config_store.go`

核心类型：

```go
type SecretStatus struct {
    HasSecret bool      `json:"has_secret"`
    Fingerprint string  `json:"fingerprint,omitempty"`
    UpdatedAt *time.Time `json:"updated_at,omitempty"`
    SecretFields []string `json:"secret_fields,omitempty"`
}

type SMTPConfigView struct {
    Enabled bool `json:"enabled"`
    Host string `json:"host"`
    Port int `json:"port"`
    Username string `json:"username"`
    From string `json:"from"`
    StartTLS bool `json:"starttls"`
    InsecureSkipVerify bool `json:"insecure_skip_verify"`
    SecretStatus SecretStatus `json:"secret_status"`
    Version int64 `json:"version"`
    UpdatedAt time.Time `json:"updated_at"`
}

type UpdateSMTPConfigRequest struct {
    Version int64 `json:"version"`
    Enabled bool `json:"enabled"`
    Host string `json:"host"`
    Port int `json:"port"`
    Username string `json:"username"`
    From string `json:"from"`
    StartTLS bool `json:"starttls"`
    InsecureSkipVerify bool `json:"insecure_skip_verify"`
    Secrets map[string]string `json:"secrets,omitempty"`
    ClearSecrets []string `json:"clear_secrets,omitempty"`
}
```

校验规则：

- `enabled=true` 时 `host`、`port`、`from` 必填。
- `port` 范围 1-65535。
- `from` 必须能解析出 envelope address。
- `username` 非空时，如果没有历史 password 且本次未传 `secrets.password`，返回 400。
- `insecure_skip_verify=true` 在生产环境允许保存但返回 warning；或者首期允许，由 readiness 提示风险。
- `secrets.password` 为空字符串时视为非法，清空必须用 `clear_secrets`。

### 5.4 新增 SMTP Admin API

路由注册文件：`internal/http/router/router.go`

新增：

- `GET /api/ops/admin/v1/security/smtp`
- `PUT /api/ops/admin/v1/security/smtp`
- `POST /api/ops/admin/v1/security/smtp/test`

权限：

- GET：`read:all` 或 `manage:dangerous_config`
- PUT：`manage:dangerous_config`
- TEST：`manage:dangerous_config`

响应示例：

`GET /api/ops/admin/v1/security/smtp`

```json
{
  "enabled": true,
  "host": "smtp.example.com",
  "port": 587,
  "username": "mailer@example.com",
  "from": "Pic Gallery <noreply@example.com>",
  "starttls": true,
  "insecure_skip_verify": false,
  "secret_status": {
    "has_secret": true,
    "fingerprint": "sha256:4f2a9c1e7b0d3301",
    "secret_fields": ["password"],
    "updated_at": "2026-06-08T10:00:00Z"
  },
  "version": 3,
  "updated_at": "2026-06-08T10:00:00Z"
}
```

`PUT /api/ops/admin/v1/security/smtp`

```json
{
  "version": 3,
  "enabled": true,
  "host": "smtp.example.com",
  "port": 587,
  "username": "mailer@example.com",
  "from": "Pic Gallery <noreply@example.com>",
  "starttls": true,
  "insecure_skip_verify": false,
  "secrets": {
    "password": "new-password"
  }
}
```

`POST /api/ops/admin/v1/security/smtp/test`

```json
{
  "email": "admin@example.com",
  "scene": "login"
}
```

测试接口行为：

- 使用当前已保存配置发送一封测试验证码邮件。
- 不修改登录验证码缓存。
- 响应只返回 `status` 和错误摘要，不返回 SMTP 密码。

```json
{
  "status": "sent",
  "recipient": "admin@example.com"
}
```

### 5.5 Auth 服务接入动态 SMTP 配置

当前 `auth.Service` 持有固定 `emailSender`。改造为可注入 resolver：

```go
type EmailSenderResolver interface {
    EmailSender(ctx context.Context) (EmailSender, error)
}
```

改造点：

- `auth.Service.SendEmailCode` 调用发送前，从 resolver 获取 sender。
- 如果后台 SMTP 未启用，回退启动配置 `cfg.Auth.SMTP`。
- 如果两者都未配置，保持当前 fail-closed 行为：返回 “email verification SMTP delivery is not configured”。

运行时 wiring：

文件：`internal/app/run.go`

- 创建 `secureConfigStore := entstore.NewSecureConfigStore(client)`
- 创建 `secureConfigSvc := secureconfig.NewService(secureConfigStore, secureConfigEncryptionKey, cfg.Auth.SMTP, cfg.App.Env)`
- `authSvc.SetEmailSenderResolver(secureConfigSvc)`
- API 注入 `secureConfigSvc`，用于 Admin SMTP API。

缓存策略：

- 首期可以每次发送验证码读取 DB，简单可靠。
- 若担心频繁读库，可在 `secureconfig.Service` 内做 30 秒内存缓存；PUT 成功后清缓存。

### 5.6 支付渠道实例安全更新契约

现有接口保留：

- `GET /api/ops/admin/v1/cashier/provider-instances`
- `POST /api/ops/admin/v1/cashier/provider-instances`
- `GET /api/ops/admin/v1/cashier/provider-instances/{instance_id}`
- `PUT /api/ops/admin/v1/cashier/provider-instances/{instance_id}`
- `DELETE /api/ops/admin/v1/cashier/provider-instances/{instance_id}`

但请求结构需要收紧：

```json
{
  "provider_type": "alipay_direct",
  "name": "支付宝沙箱主账号",
  "enabled": true,
  "supported_methods": ["alipay"],
  "sort_order": 10,
  "scheduler_weight": 100,
  "limits": {
    "min_amount_cny": "1.00000",
    "max_amount_cny": "999.00000",
    "daily_amount_limit_cny": "5000.00000"
  },
  "config": {
    "gateway_url": "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
    "app_id": "2021000000000000"
  },
  "secrets": {
    "private_key": "-----BEGIN PRIVATE KEY-----..."
  }
}
```

响应结构：

```json
{
  "id": 12,
  "provider_type": "alipay_direct",
  "name": "支付宝沙箱主账号",
  "enabled": true,
  "supported_methods": ["alipay"],
  "sort_order": 10,
  "scheduler_weight": 100,
  "limits": {
    "min_amount_cny": "1.00000",
    "max_amount_cny": "999.00000",
    "daily_amount_limit_cny": "5000.00000"
  },
  "config": {
    "gateway_url": "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
    "app_id": "2021000000000000"
  },
  "config_status": "configured",
  "credentials_status": {
    "has_secret": true,
    "fingerprint": "sha256:4f2a9c1e7b0d3301",
    "secret_fields": ["private_key"],
    "updated_at": "2026-06-08T10:00:00Z"
  }
}
```

服务层改造：

- `domaincashier.ProviderInstance` 保留运行时完整 `Config`，新增 write request 类型：
  - `ProviderInstanceWriteRequest`
  - `Config map[string]any`
  - `Secrets map[string]any`
  - `ClearSecrets []string`
- `entstore.CashierStore.UpdateProviderInstance` 更新时先读取旧配置并解密。
- 合并规则：
  - 新 `config` 覆盖非敏感字段。
  - `secrets` 覆盖敏感字段。
  - 未传的旧敏感字段保留。
  - `clear_secrets` 删除指定敏感字段。
- 运行时支付适配器仍通过 `ProviderInstance.Config` 拿完整配置。
- Admin payload 仍只回传 `RedactProviderConfig(config)`。

### 5.7 积分包配置契约

继续使用现有接口：

- `GET /api/ops/admin/v1/cashier/plans`
- `POST /api/ops/admin/v1/cashier/plans`
- `PUT /api/ops/admin/v1/cashier/plans/{plan_id}`
- `DELETE /api/ops/admin/v1/cashier/plans/{plan_id}`

字段：

```json
{
  "plan_code": "points-100",
  "plan_name": "100 积分包",
  "plan_type": "points_package",
  "purchase_enabled": true,
  "status": "active",
  "price_cny": "19.90000",
  "points": "100.00000",
  "bonus_points": "0.00000",
  "duration_days": 30,
  "currency": "CNY",
  "sort_order": 10,
  "description": "适合轻量体验"
}
```

产品规则：

- 首期开放 `points_package`。
- `subscription` 类型保留在后端、类型定义和数据库中，但管理后台创建/编辑时默认隐藏或禁用购买。
- 固定积分包充值成功后进入充值余额桶，不过期。

当前后端 `CompletePaymentOrder` 对 `points_package` 是否创建不过期 recharge bucket 需要复核：

- 若现在按 `DurationDays` 创建过期 grant，需要改为：
  - `plan_type=points_package`：grant_type=`recharge`，`expires_at=nil`。
  - `plan_type=subscription`：暂不开放购买，保留未来订阅逻辑。
- `duration_days` 对 `points_package` 可在 UI 上隐藏或显示为“充值积分不过期”，保存时后端忽略或允许填默认值。

用户端过滤规则：

`isPurchasableCashierPlan(plan)` 应确保只有以下计划出现在 `/api/agent/cashier/v1/options`：

- `status=active`
- `purchase_enabled=true`
- `plan_type=points_package`

订阅套餐即使存在也不应在用户端展示。

## 6. 前端设计

### 6.1 共享 API 类型

文件：`web/shared/api-types.ts`

新增/调整：

```ts
export type SecretStatus = {
  has_secret: boolean
  fingerprint?: string
  updated_at?: string | null
  secret_fields?: string[]
}

export type PaymentProviderInstance = {
  ...
  config?: Record<string, unknown>
  credentials_status?: SecretStatus
}

export type PaymentProviderInstanceWriteRequest = {
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string[]
  sort_order?: number
  scheduler_weight?: number
  limits?: Record<string, unknown>
  config?: Record<string, unknown>
  secrets?: Record<string, string>
  clear_secrets?: string[]
}

export type SMTPConfig = {
  enabled: boolean
  host: string
  port: number
  username: string
  from: string
  starttls: boolean
  insecure_skip_verify: boolean
  secret_status: SecretStatus
  version: number
  updated_at?: string
}

export type UpdateSMTPConfigRequest = Omit<SMTPConfig, 'secret_status' | 'updated_at'> & {
  secrets?: { password?: string }
  clear_secrets?: string[]
}
```

### 6.2 Admin API client

文件：`web/shared/admin-api.ts`

新增：

- `getSMTPConfig()`
- `updateSMTPConfig(input)`
- `testSMTPConfig(input)`

路径新增到 `API_PATHS.ops`：

- `securitySMTP: '/api/ops/admin/v1/security/smtp'`
- `securitySMTPTest: '/api/ops/admin/v1/security/smtp/test'`

支付实例 API 保留方法名，但 payload 改用 `secrets`。

### 6.3 支付渠道实例编辑 UX

文件：`web/admin/src/pages/CashierPage.tsx`

位置：

- `CashierTabId = 'instances'`
- `InstanceDraft`
- `saveInstance`
- `editInstanceDraft`
- instance modal 的 `渠道配置 JSON`

改造：

1. `InstanceDraft` 拆成：
   - `config_text`：非敏感 JSON。
   - `secrets_text` 或结构化 secret fields：只用于本次保存。
   - `clear_secret_fields`：可选。
2. 编辑已有实例时：
   - `config_text` 填入后端返回的非敏感 config。
   - secret 输入框为空。
   - 显示状态：“已配置密钥，指纹 sha256:xxxx，留空则保留原密钥”。
3. 保存时：
   - `config` 只提交非敏感配置。
   - 如果 secret 输入框有值，提交 `secrets`。
   - 如果 secret 输入框为空，不提交 `secrets`。
4. 防误操作：
   - 如果用户在 secret 输入框输入 `******`、`********`，前端直接提示不能使用脱敏占位符。
   - 删除/清空 secret 需要二次确认，并提交 `clear_secrets`。

首期可以继续保留 JSON 高级模式，但文案必须明确：

- “配置 JSON 只保存非敏感字段。”
- “密钥字段请填在下方密钥区，保存后不会回显。”

### 6.4 SMTP 配置页面

推荐新增文件：

- `web/admin/src/pages/SecurityConfigPage.tsx`

路由：

- 修改 `web/admin/src/App.tsx`
- 新增 route：`security-config`
- 左侧导航显示：“安全配置”或“邮件服务”

页面结构：

- SMTP 服务状态卡：
  - 是否启用。
  - 当前 host/port/from。
  - 密码状态和 fingerprint。
  - 最近更新时间。
- 配置表单：
  - 启用发信。
  - SMTP Host。
  - SMTP Port。
  - Username。
  - Password，新值留空则保留。
  - From。
  - StartTLS。
  - InsecureSkipVerify。
- 测试发送：
  - 测试收件邮箱。
  - “发送测试邮件”按钮。

权限：

- 无 `manage:dangerous_config` 时只读。
- Secret 输入框不可见或禁用。

### 6.5 ConfigPage 处理

文件：

- `web/admin/src/pages/ConfigPage.tsx`
- `web/admin/src/pages/configRows.ts`

调整：

- 不在通用 ConfigPage 新增 SMTP 密码字段。
- `payments.provider_instances` 在 ConfigPage 中标为只读或隐藏，避免绕过 CashierPage 的安全 secret 契约。
- `payments.visible_methods`、`custom_amount_*` 可保留，但建议文案引导到 CashierPage。

推荐：

- 在 `configRows.ts` 将 `provider_instances` 标记为危险/只读，ConfigPage 渲染时不可编辑。
- 后端 `adminconfig.UpdateTab` 对 `payments.provider_instances` 可逐步禁止通用更新，避免密钥明文路径继续存在。

## 7. OpenAPI 与文档

需要更新：

- `api/openapi/openapi.yaml`
- `api/openapi/components/schemas/admin.yaml`
- `web/shared/mock-data.ts` 如有接口示例。
- README/中文 README 的配置章节。

重点写清：

- `config` 不包含 secret。
- `secrets` 只写不读。
- GET 响应仅返回 `secret_status`。
- 生产必须配置 `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY` 和 `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`。

## 8. 用户端生成体验与调用记录补强

本节补充用户端生图页面、参考图上传、失败记录重试/删除，以及管理后台调用记录可观测性问题。它们与支付/SMTP 配置同属“产品缺陷闭环”范围，后续实现应与前述安全配置分阶段推进。

### 8.1 当前实现基线

用户端生图页面：

- 文件：`web/user/src/pages/WorkspacePage.tsx`
- 路由：用户端工作台，当前由 App 内部导航到 `workspace`。
- 参考图上传入口：
  - 参考生图模式：`uploadReference(event, 'reference')`
  - 图片编辑来源：`uploadReference(event, 'edit')`
- 上传 API：
  - `userApi.uploadReferenceAsset(file)`
  - `POST /api/agent/image/v1/reference-assets`
- 上传成功后页面用 `asset.preview_url` 渲染 `<img>`。

后端参考图上传：

- 文件：`internal/http/handlers/api.go`
- Handler：`HandleReferenceAssetUpload`
- 服务：`internal/service/assets/service.go`
- 当前大小限制：`limits.ReferenceImageMaxMB * 1024 * 1024`
- 超限错误：`errs.CodeImageReferenceExceeded`，message 为 `reference asset too large`。

能力配置：

- 用户端已调用 `GET /api/agent/image/v1/capabilities`。
- 当前 `Capability` 类型包含 `max_image_count`、模型级 `max_reference_image_count`，但没有明确暴露 `reference_image_max_mb`。

历史任务：

- 用户端 API 已有：
  - `GET /api/agent/image/v1/history/tasks`
  - `GET /api/agent/image/v1/history/tasks/{task_id}`
  - `DELETE /api/agent/image/v1/history/tasks/{task_id}`
- 后端 `HandleAgentHistoryTaskDetail` 已支持 DELETE。
- `WorkspacePage` 当前未在失败任务卡片上接入重试和删除按钮。

调用记录：

- 文件：
  - `internal/domain/admincallrecord/types.go`
  - `internal/repository/entstore/admin_call_record_store.go`
  - `web/admin/src/pages/CallRecordsPage.tsx`
- 管理后台路由：`/#/call-records`
- 后端接口：`GET /api/ops/admin/v1/call-records`
- 当前调用记录从 `image_tasks` 映射，不是独立 call_records 表。
- `image_tasks` 已持久化：
  - `account_model_id`
  - `model_account_id`
  - `provider_trace.attempts`
  - `error_code`
  - `error_message`
- 当前 `AdminCallRecord` 响应只暴露简短错误字段和 `attempt_count`，没有暴露底层账号 ID、模型账号名、attempt 明细和错误详情。

### 8.2 本地上传图片展示异常

问题判断：

- `ReferenceAsset` 后端类型当前不包含 `preview_url`。
- `web/shared/user-api.ts` 的 `toReferenceAsset` 用 `raw.preview_url ?? raw.download_url ?? ''` 生成预览地址。
- `POST /api/agent/image/v1/reference-assets` 返回的 asset 如果没有 `preview_url/download_url`，页面 `<img src="">` 就会展示异常。

后端契约调整：

所有参考图资产响应都应包含可预览 URL：

```json
{
  "id": "asset-id",
  "status": "ready",
  "mime_type": "image/png",
  "file_size_bytes": 102400,
  "width": 1024,
  "height": 1024,
  "preview_url": "/api/agent/image/v1/reference-assets/{asset_id}/download",
  "download_url": "/api/agent/image/v1/reference-assets/{asset_id}/download",
  "created_at": "2026-06-08T10:00:00Z"
}
```

落点：

- `internal/domain/assets/types.go`
  - `ReferenceAsset` 增加 `PreviewURL string json:"preview_url,omitempty"` 和 `DownloadURL string json:"download_url,omitempty"`。
- `internal/http/handlers/api.go`
  - 新增 `referenceAssetPayload(asset)`，在所有 reference asset response 中补 URL。
  - `HandleReferenceAssetUpload` 返回 payload，而不是直接返回 domain asset。
  - `HandleReferenceAssetGet` 返回 payload。
  - Open API 的参考图上传响应也补 URL。
- `web/shared/user-api.ts`
  - `toReferenceAsset` 继续兼容 `preview_url/download_url`。
  - `imageAssetUrl` 对相对 URL 附加 token 用于下载预览。
- `web/user/src/pages/WorkspacePage.tsx`
  - 渲染上传列表时统一使用 `userApi.imageAssetUrl(asset.preview_url || asset.download_url || '', token)`。
  - 如果 URL 为空，展示占位态“预览生成中/无法预览”，不要渲染空 src。

验收：

- 手动从本地上传图片后，参考生图和图片编辑来源都能立即显示缩略图。
- 刷新页面或 SSE 历史记录带回 reference_assets 时，缩略图仍可展示。

### 8.3 上传大小超限提示与最大大小展示

当前问题：

- 上传文件超过后台配置大小时，用户端看到的是“参考图数量超过当前模型上限，请减少后重试”，语义不对。
- 页面没有展示允许上传的最大文件大小。

后端错误契约：

新增或细分错误码：

- 推荐新增：`image_reference_too_large`
- 保留旧 `image_reference_exceeded` 用于数量超限。

响应示例：

```json
{
  "error": {
    "code": "image_reference_too_large",
    "message": "参考图文件超过 10 MB，请压缩后重新上传。",
    "status_code": 400,
    "details": {
      "max_size_bytes": 10485760,
      "max_size_mb": 10,
      "actual_size_bytes": 15728640
    }
  }
}
```

落点：

- `pkg/errs`：
  - 新增 `CodeImageReferenceTooLarge`。
- `internal/service/assets/service.go`
  - 超限时返回 `CodeImageReferenceTooLarge`。
  - message 使用中文业务可读文案或由前端本地化。
  - err details 包含 `max_size_bytes`、`max_size_mb`、`actual_size_bytes`。
- `internal/domain/modelhub/resolver.go`
  - 数量超限继续使用 `CodeImageReferenceExceeded`。
  - 文案保持“参考图数量超过当前模型上限”。
- `internal/http/handlers/api.go`
  - `HandleReferenceAssetUpload` 不要强转 `svcErr.(*errs.Error)`，统一 `normalizeAppError(svcErr)`，避免非标准错误 panic。

能力接口调整：

`GET /api/agent/image/v1/capabilities` 增加：

```json
{
  "reference_image_max_mb": 10,
  "reference_image_max_bytes": 10485760
}
```

前端类型：

- `web/shared/api-types.ts`
  - `Capability.reference_image_max_mb?: number`
  - `Capability.reference_image_max_bytes?: number`

用户端页面：

- `web/user/src/pages/WorkspacePage.tsx`
  - 在参考生图上传区和图片编辑来源上传区展示“单张最大 X MB”。
  - 上传前做 client-side 校验：
    - 如果 `file.size > capability.reference_image_max_bytes`，不发请求，直接提示“单张参考图最大 X MB，当前文件 Y MB”。
  - 多文件上传时逐个校验，并提示被跳过的文件。
  - 捕获后端 `image_reference_too_large` 时优先展示 details 中的最大大小。

验收：

- 页面明确展示最大上传大小。
- 超限错误不再显示“数量超过”。
- 文件超限不会创建 reference asset 记录。

### 8.4 生图错误明细与底层账号可观测

目标：

- 生图失败后记录完整错误明细。
- 管理后台调用记录可点击查看错误详情。
- 管理后台调用记录能看到此次调用使用的底层账号。

后端数据模型：

优先复用 `image_tasks` 现有字段：

- `error_code`
- `error_message`
- `provider_trace`
- `account_model_id`
- `model_account_id`

增强 `provider_trace` 内容：

```json
{
  "provider": "openai",
  "provider_model_id": 12,
  "account_model_id": 34,
  "model_account_id": 56,
  "upstream_model_code": "gpt-image-1",
  "route_snapshot_version": "2026-06-08T10:00:00Z",
  "attempts": [
    {
      "provider": "openai",
      "adapter_type": "openai_compatible",
      "account_model_id": 34,
      "model_account_id": 56,
      "model_code": "gpt-image-1",
      "status": "failed",
      "error_code": "upstream_unavailable",
      "error_message": "upstream returned 429",
      "error_detail": {
        "http_status": 429,
        "upstream_code": "rate_limit_exceeded",
        "upstream_request_id": "req_xxx",
        "retryable": true,
        "action": "retry"
      },
      "started_at": "2026-06-08T10:00:00Z",
      "finished_at": "2026-06-08T10:00:02Z"
    }
  ]
}
```

如当前 `domainimagetask.Attempt` 只有 `Provider/Status/Error`，需要扩展：

```go
type Attempt struct {
    Provider string `json:"provider"`
    AdapterType string `json:"adapter_type,omitempty"`
    AccountModelID int64 `json:"account_model_id,omitempty"`
    ModelAccountID int64 `json:"model_account_id,omitempty"`
    ModelCode string `json:"model_code,omitempty"`
    Status string `json:"status"`
    Error string `json:"error,omitempty"`
    ErrorCode string `json:"error_code,omitempty"`
    ErrorMessage string `json:"error_message,omitempty"`
    ErrorDetail map[string]any `json:"error_detail,omitempty"`
    StartedAt *time.Time `json:"started_at,omitempty"`
    FinishedAt *time.Time `json:"finished_at,omitempty"`
}
```

服务落点：

- `internal/service/imagetask/service.go`
  - provider 调用失败时，把 provider candidate 信息写入 attempt。
  - 捕获 `provider.UpstreamError` 时提取 `HTTPStatus`、`ProviderCode`、`RequestID`、`Action` 等信息进入 `ErrorDetail`。
  - 最终 `failOwnedTask` 保存 `ErrorCode/ErrorMessage/Attempts`。
- `internal/repository/entstore/imagetask_store.go`
  - `provider_trace` JSON 继续保存 attempts；如 schema 已足够无需新增列。
  - 确认 `account_model_id/model_account_id` 在失败时也能保存。当前成功时会设置，失败时也应在每次 attempt 里至少记录 candidate 的账号 ID。

Admin Call Records API 调整：

当前接口保留：

- `GET /api/ops/admin/v1/call-records`

列表响应增加字段：

```ts
export type CallRecord = {
  ...
  account_model_id?: number | null
  model_account_id?: number | null
  model_account_name?: string | null
  account_model_code?: string | null
  upstream_model_code?: string | null
  error_detail?: Record<string, unknown> | null
  attempts?: CallRecordAttempt[]
}
```

可选新增详情接口：

- `GET /api/ops/admin/v1/call-records/{task_id}`

推荐新增详情接口，原因：

- 列表保持轻量。
- 错误详情和 attempts JSON 可能较大。
- 前端点击弹窗时再取详情。

详情响应：

```json
{
  "task_id": "task-id",
  "status": "failed",
  "provider": "openai",
  "model_account_id": 56,
  "model_account_name": "OpenAI 主账号",
  "account_model_id": 34,
  "account_model_code": "gpt-image-1",
  "error_code": "upstream_unavailable",
  "error_message": "上游模型服务暂不可用",
  "error_detail": {
    "http_status": 429,
    "upstream_code": "rate_limit_exceeded",
    "upstream_request_id": "req_xxx"
  },
  "attempts": []
}
```

后台页面：

- 文件：`web/admin/src/pages/CallRecordsPage.tsx`
- 行上增加“详情”按钮。
- 点击打开 modal：
  - 基础信息：任务 ID、用户、入口、路由模型、provider。
  - 底层账号：`model_account_id/model_account_name/account_model_id/account_model_code`。
  - 错误摘要：`error_code/error_message`。
  - 错误详情：格式化 JSON。
  - 尝试链路：按 attempts 展示每次 provider、账号、模型、状态、错误、耗时。
- 列表 Provider 列显示：
  - provider 名。
  - 底层账号：`账号 #56 / 模型 #34`；如果能 join 名称则显示名称。

Store 落点：

- `internal/domain/admincallrecord/types.go`
  - `Record` 增加账号字段、`Attempts`、`ErrorDetail`。
  - 新增 `Attempt` 类型或复用 `domainimagetask.Attempt`。
- `internal/repository/entstore/admin_call_record_store.go`
  - `mapAdminCallRecord` 从 entity 字段和 `ProviderTrace` 映射。
  - 如需名称，查询 `model_accounts` 和 `model_account_models`，或先只返回 ID，名称后续增强。
- `web/admin/src/pages/callRecordRows.ts`
  - 增加 provider detail 文案。
  - 增加 detail modal 的展示行模型。

验收：

- 上游失败后，调用记录详情能看到错误明细 JSON 和每次 attempt。
- 调用记录列表/详情能看到底层模型账号 ID 和账号模型 ID。
- 不暴露模型账号密钥。

### 8.5 用户端失败记录重试

目标：

- 用户端生图失败后，在失败记录卡片上增加“重试”按钮。
- 重试应使用原任务参数快速创建新任务。

后端推荐新增接口：

- `POST /api/agent/image/v1/history/tasks/{task_id}/retry`

请求：

```json
{
  "idempotency_key": "uuid"
}
```

响应：

```json
{
  "id": "new-task-id",
  "status": "queued",
  "prompt": "...",
  "reference_asset_ids": ["..."]
}
```

服务逻辑：

- 校验任务属于当前用户。
- 仅允许重试状态：
  - `failed`
  - `partial_failed`
  - `cancelled`
  - `rejected` 可选，若内容安全拒绝不允许重试，返回 400。
- 读取原任务参数：
  - `task_type`
  - `prompt`
  - `negative_prompt`
  - `route_model_code`
  - `requested_quality`
  - `aspect_ratio/requested_size`
  - `requested_output_image_count`
  - `reference_asset_ids`
  - `reference_strength`
  - `save_policy`
- 新建任务 ID，重新走 `CreateTask`，重新估算和预扣积分。
- 不复用原 task ID。
- 新任务记录 `retry_of_task_id`。

是否新增字段：

- 推荐在 `image_tasks` 增加 `retry_of_task_id` nullable uuid/string。
- 如暂不加列，可在 `provider_trace` 或 metadata 中记录，但后续追踪不如独立列清晰。

前端：

- 文件：`web/shared/api-types.ts`
  - `ImageTask.retry_of_task_id?: string`
- 文件：`web/shared/user-api.ts`
  - 新增 `retryTask(task_id, idempotencyKey?)`。
- 文件：`web/user/src/pages/WorkspacePage.tsx`
  - 在 `TaskRecord` 或失败 pending 区域显示“重试”按钮。
  - 点击后调用 `userApi.retryTask(task.id)`。
  - 成功后把新任务插入 records，并提示“已重新提交”。
  - 重试按钮只在失败/部分失败/取消且用户余额足够或可重新估算时展示；余额不足时仍允许点击，由后端返回积分不足并引导充值。

验收：

- 失败任务点击重试后创建一个新 queued task。
- 新任务保留原提示词和参考图。
- 原失败记录不被覆盖。

### 8.6 用户端失败记录删除

当前后端已支持：

- `DELETE /api/agent/image/v1/history/tasks/{task_id}`
- `userApi.deleteTask(task_id)` 已存在。

需要补前端：

- 文件：`web/user/src/pages/WorkspacePage.tsx`
- 在失败记录卡片增加“删除”按钮。
- 删除前弹出确认：
  - “删除后会从当前历史记录移除；已生成图片文件也会按任务删除逻辑清理。”
- 成功后：
  - 从 `records` state 移除。
  - 如果删除的是当前正在显示详情/预览相关任务，关闭预览。

后端需复核：

- `imagetask.Service.DeleteByID` 当前调用 store 删除任务。
- 对已有结果的任务删除是否清理结果文件，当前 `DeleteImageResult` 会清文件，但 `DeleteByID` 是否清文件需看 store 实现。
- 若当前只是软删任务，不清理结果文件，需要明确产品行为：
  - 历史任务删除：软删任务和结果，不一定物理删除文件。
  - 图库图片删除：清理图片文件。
- 本次目标是“删除此次记录”，因此首期可以软删任务记录；不要误删仍在图库中展示的图片。

验收：

- 失败记录可删除，并从工作台历史列表消失。
- 删除接口只能删除自己的任务。
- 删除成功后 SSE/history reload 不再返回该任务。

## 9. 迁移与兼容

### 9.1 支付实例历史数据

现有数据可能有两类：

1. `config_encrypted` 已是 `{"ciphertext":"v1:..."}`。
2. 旧测试/开发数据可能直接保存明文 JSON。

读取策略：

- 有 `ciphertext` 且可解密：正常使用。
- 无 `ciphertext`：视为历史明文配置，在非生产可读；生产启动/ready 检查提示需要迁移。

迁移策略：

- 增加一次性迁移函数或管理命令：读取无 ciphertext 的支付实例，用主密钥重新加密写回。
- 首期也可在首次更新实例时自动加密。

### 9.2 SMTP env/YAML 兼容

优先级：

1. 后台 `secure_configs.smtp/default` 且 `enabled=true`。
2. env/YAML `cfg.Auth.SMTP`。
3. 未配置则发送验证码失败。

迁移策略：

- 不自动把 env/YAML 密码写入 DB，避免隐式持久化 secret。
- 管理后台首次打开 SMTP 页面时，若 DB 未配置但 env/YAML 已配置，显示“当前使用环境变量配置，保存后将改用后台配置”。

## 10. 上架检查与运维可观测

Admin readiness 新增检查：

- `smtp_configured`
  - OK：后台 SMTP 或 env SMTP 可用。
  - WARN：未配置 SMTP，验证码登录/注册不可用。
  - WARN：生产环境 `insecure_skip_verify=true`。
- `secure_config_key`
  - OK：生产设置了非默认 `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`。
  - FAIL：生产缺少主密钥。
- `cashier_provider_secret_key`
  - OK：生产设置了非默认 `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`。
  - FAIL：生产缺少主密钥。

Dashboard/审计：

- 安全配置更新计入 audit。
- 支付渠道实例 secret 旋转计入 audit。
- 不记录明文。

## 11. 测试方案

### 11.1 Go 单元测试

新增/调整：

- `internal/service/secretcodec/*_test.go`
  - AES-GCM 加解密成功。
  - 相同明文多次加密 ciphertext 不同。
  - 错误 key 解密失败。
  - fingerprint 稳定。
- `internal/repository/entstore/secure_config_store_test.go`
  - SMTP 配置保存后 DB 不含明文 password。
  - GET view 不返回 password。
  - 未传 secrets 更新时保留旧 password。
  - clear password 生效。
- `internal/repository/entstore/cashier_store_test.go`
  - 支付实例 update 未传 secrets 保留旧密钥。
  - 传新 secret 后 fingerprint 变化。
  - payload 不返回密钥字段。
- `internal/service/auth/service_test.go`
  - SendEmailCode 使用后台 SMTP resolver。
  - 后台未配置时回退 env/YAML。
  - 均未配置时 fail-closed。

### 11.2 Handler/API 测试

新增/调整：

- `internal/http/router/admin_security_smtp_api_test.go`
  - GET SMTP 配置不返回 password。
  - PUT SMTP 配置需要 `manage:dangerous_config`。
  - PUT 后审计记录不含明文。
  - TEST 接口使用保存配置。
- `internal/http/router/admin_cashier_provider_instance_api_test.go`
  - 创建实例响应不含 secret。
  - 更新非敏感字段不清空旧 secret。
  - mask 占位符被拒绝。
- `api/openapi/openapi_test.go`
  - 新 SMTP 路径存在。
  - payment provider schema 有 `secrets`，响应 schema 无明文 secret。

### 11.3 前端测试/验证

命令：

- `npm --prefix web/admin run typecheck`
- `npm --prefix web/admin run build`
- `npm --prefix web/user run typecheck`
- `npm --prefix web/user run build`

手工验证：

1. 管理后台新增/编辑 SMTP，保存后刷新页面，密码不回显，只显示已配置。
2. 留空密码更新 host，测试发送仍可用。
3. 新增支付渠道实例，保存后刷新，密钥不回显。
4. 编辑支付渠道非敏感字段，旧密钥仍可用于创建订单。
5. 管理后台新增积分包后，用户端充值页立即显示。
6. 订阅套餐存在时，用户端不展示。

### 11.4 全量验证

最终交付前执行：

```bash
./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope all
```

若触及 Docker/部署配置，再执行：

```bash
./scripts/workflow/api-smoke.sh
```

## 12. 实施顺序

### 阶段 1：安全底座

1. 新增 `secretcodec`。
2. 新增 `secure_configs` schema 和 ent 生成。
3. 新增 `secureconfig` domain/service/store。
4. 增加生产主密钥校验。

验收：

- 单元测试证明 DB 不存 SMTP 明文 password。

### 阶段 2：SMTP 后台配置

1. 新增 Admin SMTP API。
2. Auth 服务接入动态 SMTP resolver。
3. 新增 `SecurityConfigPage.tsx` 和导航。
4. 增加测试发送能力。

验收：

- 管理后台能保存 SMTP 配置。
- 验证码邮件链路使用后台配置。
- GET 不返回 password。

### 阶段 3：支付实例 write-only secret

1. 定义 `ProviderInstanceWriteRequest`。
2. 后端更新逻辑改为合并旧 secret。
3. 响应补齐 `credentials_status.secret_fields`。
4. 前端实例弹窗拆分非敏感 config 和 secret 输入。
5. 禁止 ConfigPage 通用编辑 `provider_instances`。

验收：

- 编辑旧实例非敏感字段不会导致密钥丢失。
- 支付下单和回调仍能读取完整配置。

### 阶段 4：积分包收口

1. Cashier plans tab 文案明确“积分包充值后不过期”。
2. 用户端 options 只返回 `points_package`。
3. 支付完成入账逻辑确保 points package 进入 `recharge` bucket 且不过期。
4. 保留 subscription 类型定义，但后台默认禁用购买，用户端不展示。

验收：

- 后台新增/修改/归档积分包能反映到用户充值页。
- 完成支付后进入充值余额桶且无过期时间。

### 阶段 5：用户端生成体验与调用记录补强

1. 参考图 asset 响应补 `preview_url/download_url`，修复本地上传后缩略图异常。
2. 能力接口补 `reference_image_max_mb/reference_image_max_bytes`，用户端上传区展示最大大小并做前置校验。
3. 新增 `image_reference_too_large` 错误码，区分文件大小超限和参考图数量超限。
4. 扩展任务 attempts/error detail 记录，调用记录详情页展示错误明细和底层模型账号。
5. 新增历史任务 retry API，用户端失败记录增加“重试”按钮。
6. 接入已有历史任务 DELETE API，用户端失败记录增加“删除”按钮。

验收：

- 本地上传图片能立即预览。
- 大文件上传提示正确，页面展示最大大小。
- 管理后台调用记录详情能看到错误明细和底层账号。
- 用户端失败记录可重试、可删除。

## 13. 风险与边界

1. “禁止明文传输”按已确认口径 A：写入请求中会出现一次明文 secret，因此生产必须通过 HTTPS；后端不在响应、DB、审计日志中保留明文。
2. 当前模型账号等其他模块也有 `credentials_encrypted` 命名但不一定真加密；本方案只覆盖支付和 SMTP，后续可复用 `secretcodec` 扩展到模型账号。
3. 若已有支付实例使用旧通用 ConfigPage 保存过明文配置，需要提供迁移或首次更新重加密策略。
4. SMTP 测试接口可能触发真实邮件发送，需要管理后台明确展示测试收件人，避免误发。
5. 用户端失败任务重试会重新预扣积分，不能复用原任务已退回/已结算的账务流水。
6. 失败记录删除首期按“删除历史记录/软删任务”处理，不承诺物理删除已入库图片文件，避免误删图库图片。

## 14. Definition of Done

1. 管理后台可以配置 SMTP，密码保存后不回显，DB 不存在明文 password。
2. 登录/注册/密码重置验证码发送使用后台 SMTP 配置。
3. 管理后台可以配置支付渠道实例，多账号调度策略保留，密钥保存后不回显。
4. 编辑支付渠道实例非敏感字段不会清空旧密钥。
5. 管理后台可以配置固定积分包种类、售价、积分数、状态和排序。
6. 用户端 `/api/agent/cashier/v1/options` 展示后台配置的可购买积分包。
7. 积分包充值到账进入充值余额桶且不过期。
8. 订阅套餐定义保留但用户端不开放购买。
9. OpenAPI、README 或部署文档说明新增安全配置主密钥。
10. 本地上传参考图能正常显示缩略图。
11. 上传大小超限提示准确，且上传区展示后台配置的最大文件大小。
12. 管理后台调用记录可查看错误详情、attempt 链路和底层模型账号。
13. 用户端失败生成记录可以一键重试。
14. 用户端失败生成记录可以删除，并从历史列表消失。
15. `./scripts/workflow/verify.sh` 通过。
