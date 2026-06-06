# Pic Gallery 产品缺陷清零技术方案与实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 清空当前体验报告中指出的产品交互缺陷和功能闭环缺陷，让 Pic Gallery 达到“新用户可完成首图、管理员可完成上线配置、用户可充值/消费/管理资产、公开广场可用于拉新和灵感复用、开发者可按文档接入 API”的可运营状态。

**Architecture:** 在现有 Go `net/http` + Ent + React/Vite 双前端架构上演进，不替换路由体系，不引入新的前端状态框架。后端新增 `cashier` 收银台域和体验额度授权逻辑，扩展 `billing` 钱包桶模型；前端新增用户收银台页和后台收银台管理页，并改造首页、生图、个人中心、公开广场、开发文档页。

**Tech Stack:** Go, Ent, SQLite/PostgreSQL-compatible schema, `net/http` ServeMux, React, TypeScript, Vite, Server-Sent Events, OpenAPI 3.1.

---

## 0. 已确认产品决策

| # | 决策 | 结论 |
|---|------|------|
| D1 | 新用户首图体验 | 注册送体验额度；后台可配置体验额度金额和有效期。产品侧保证宣发前完成真实模型账号配置；无模型时禁用生成并提示。 |
| D2 | 账务模型 | 体验额度、订阅额度、充值额度分桶记账。扣费默认按“最早过期体验额度 -> 订阅额度 -> 充值额度”。 |
| D3 | 公开广场社交边界 | 保留点赞和收藏，不做评论。广场用于展示平台效果和提示词灵感。 |
| D4 | 公开广场游客能力 | 游客可看公开广场列表；作品详情、完整提示词、复制/同款、点赞/收藏需要登录。 |
| D5 | 支付能力 | 建设内置收银台：产品侧发起支付、创建订单、回调、到账、流水由收银台包装。 |
| D6 | 支付渠道首版 | 实现支付宝官方、微信官方、易支付、JeePay、Mock 的最小真实链路；JeePay 支持 hosted URL、API 预下单、MD5 form 回调验签、金额校验、幂等到账、主动查单、真实退款和结构化 `channelExtra/wayCode` 配置。 |
| D7 | 充值商品 | 固定积分包 + 用户自定义金额充值。充值积分不过期，进入充值余额桶。订阅套餐类型和表结构保留，但本轮不开放订阅购买。 |
| D8 | 后台权限 | 先按 B 方案实现：本轮只开放 `super_admin` / `admin` 两级角色，但所有后台接口必须通过 permission facade 鉴权；后续向完整自定义权限系统演进时，只替换 permission resolver 和权限数据来源，不改业务 handler 契约。 |

---

## 1. 现状问题与目标验收

### 1.1 当前阻断点

1. 新账号余额为 `0.00000`，且无注册送体验额度。
2. `GET /api/agent/image/v1/capabilities` 在未配置模型时返回空，用户端只显示“暂无可用模型”，没有更完整的首图引导。
3. 生图页没有基于余额桶和额度过期提醒做决策，用户无法理解体验额度、充值额度、订阅额度。
4. 充值订单仍返回 `mock://checkout/...`，支付、回调、到账、流水不闭环。
5. 个人中心的“充值积分”误跳 API Key 页面。
6. 公开广场列表和详情边界不清，游客/登录态能力没有产品化。
7. 开发文档页显示 `0 / 0 endpoints`，示例区可能复制错误 HTML/bootstrap payload。
8. 后台模型、路由、价格、支付、用户、审核、调用记录之间没有上线检查和明确修复入口。
9. 后台权限只有粗粒度 `requireAdmin`。
10. 本地用户端默认代理 `18080`，项目 API 文档和实际服务是 `8080`。

### 1.2 最终验收标准

P0 验收：

- 全新用户邮箱验证码登录后，账户自动获得一笔体验额度，个人中心能看到体验额度金额、有效期、过期提醒。
- 后台可配置注册送体验额度金额、有效期、是否启用、过期提醒阈值。
- 若存在可见路由模型和价格，新用户可直接完成首图生成；若不存在，生图页禁用生成按钮并明确提示“平台模型配置中，请稍后再试”。
- 用户可从个人中心进入收银台，购买固定积分包或输入自定义金额；支付成功后充值额度到账，流水可查。
- 测试环境可用 Mock 支付完成完整链路；线上可配置支付宝/微信沙箱账号验证链路。
- 公开广场游客可看列表，登录后可看详情、完整提示词、点赞、收藏、同款生成。
- 开发文档页从 OpenAPI 正确渲染接口目录、错误码、示例代码。
- 后台首页能显示上线检查：模型账号、路由模型、价格、支付渠道、公开广场、文档 API 是否可用。

P1 验收：

- 后台收银台页可管理支付渠道实例、可见支付方式路由、固定积分包、自定义金额规则、订单与回调记录。
- 后台用户详情页能看到余额桶、流水、订单、任务、API Key、管理员积分调整记录。
- 调用记录能记录前置失败：无模型、无价格、余额不足、provider 不可用。
- 审核队列支持通过、拒绝、下架，并写审计。

P2 验收：

- 后台鉴权统一通过 permission facade；当前只区分 `super_admin` / `admin`，但 handler 不再直接散落裸 `requireAdmin`。
- 支付渠道 adapter 有统一接口；支付宝官方、微信官方、易支付、JeePay、Mock 均按同一收银台契约完成下单/回调/查单/退款或测试履约闭环，后续只继续增强更细渠道异常、争议、风控和行业参数产品化。
- E2E 覆盖“后台配置 -> 注册 -> 体验额度 -> 生图 -> 公开审核 -> 广场 -> 充值 -> API 文档”。

---

## 2. 总体架构

```mermaid
flowchart LR
  UserWeb["web/user React"] --> AgentAPI["Agent API /api/agent/*"]
  AdminWeb["web/admin React"] --> OpsAPI["Ops API /api/ops/admin/*"]
  Visitor["游客"] --> OpenAPI["Open API /api/open/*"]

  AgentAPI --> Auth["auth service"]
  Auth --> SignupGrant["signup experience grant"]
  SignupGrant --> Billing["billing wallet buckets"]

  AgentAPI --> ImageTask["image task service"]
  ImageTask --> Billing
  ImageTask --> ModelHub["model routing/model account"]

  AgentAPI --> Cashier["cashier service"]
  OpsAPI --> Cashier
  Cashier --> PaymentAdapters["payment adapters"]
  PaymentAdapters --> Alipay["Alipay Direct"]
  PaymentAdapters --> WxPay["WeChat Pay APIv3"]
  PaymentAdapters --> EasyPay["EasyPay"]
  PaymentAdapters --> MockPay["Mock"]

  PaymentAdapters --> Webhook["payment webhook"]
  Webhook --> Cashier
  Cashier --> Billing

  AgentAPI --> Gallery["gallery service"]
  OpenAPI --> Gallery
  OpsAPI --> Gallery
  Gallery --> Audit["audit logs"]

  OpsAPI --> Readiness["admin readiness"]
  Readiness --> ModelHub
  Readiness --> Cashier
  Readiness --> Billing
  Readiness --> Gallery
```

### 2.1 模块边界

- `auth`：负责登录、注册识别、会话刷新。登录/注册成功后触发注册送体验额度，必须幂等。
- `billing`：负责余额桶、预扣、结算、退款、流水、注册送体验额度、兑换码、管理员加减积分。
- `cashier`：负责充值商品、订单、支付方式路由、渠道实例、回调验签、到账履约。
- `modelhub` / `modeladmin`：继续负责模型账号、真实模型、路由模型、价格和用户分组可见性。
- `imagetask`：继续负责任务创建、执行、结果和调用记录。需在前置失败时写 call record。
- `gallery`：负责私有图库、公开申请、审核、游客列表、登录详情、点赞、收藏。
- `admindashboard`：负责上线检查和后台首页聚合。
- `docs`：负责 OpenAPI JSON/YAML、examples、errors 的稳定输出。

---

## 3. 前端路由与页面改造

### 3.1 用户端路由

现有路由定义位置：

- `web/user/src/types.ts`
- `web/user/src/App.tsx`
- `web/user/src/components.tsx`

新增/改造路由：

| 路由 | 页面文件 | 访问控制 | 目标 |
|------|----------|----------|------|
| `#/home` | `web/user/src/pages/HomePage.tsx` | 登录 | 首图入口、余额桶摘要、模型可用状态、公开广场精选。 |
| `#/genpic` | `web/user/src/pages/WorkspacePage.tsx` | 登录 | 生图工作台；无模型/余额不足时禁用生成。 |
| `#/gallery` | `web/user/src/pages/GalleryPage.tsx` | 登录 | 私有图库，申请公开、分组、删除、继续编辑。 |
| `#/public-gallery` | `web/user/src/pages/PublicGalleryPage.tsx` | 游客可访问 | 公开广场列表，游客只看列表卡片。 |
| `#/public-gallery?image_id={image_id}` | 同上 | 详情需登录 | 登录后打开详情弹层/详情区域，展示完整提示词和互动。 |
| `#/checkout` | 新增 `web/user/src/pages/CheckoutPage.tsx` | 登录 | 收银台：固定积分包、自定义金额、支付方式、订单状态。 |
| `#/profile` | `web/user/src/pages/ProfilePage.tsx` | 登录 | 余额桶、过期提醒、兑换码、订单入口、流水。 |
| `#/api-keys` | `web/user/src/pages/ApiKeysPage.tsx` | 登录 | API Key 生命周期和真实 Quickstart。 |
| `#/docs` | `web/user/src/pages/DocsPage.tsx` | 登录 | OpenAPI 目录、示例、错误码。后续可放开只读游客访问。 |

`RouteId` 必须改为：

```ts
export type RouteId =
  | 'landing'
  | 'login'
  | 'home'
  | 'genpic'
  | 'gallery'
  | 'public-gallery'
  | 'checkout'
  | 'api-keys'
  | 'profile'
  | 'docs'
```

`protectedRoutes` 调整：

- `public-gallery` 不再放入强制登录列表。
- `checkout` 加入强制登录列表。
- `docs` 首轮继续登录可见，避免公开 AK/SK 示例造成误用；后续可做匿名文档。

### 3.2 用户端页面具体改造

#### `HomePage.tsx`

位置：`web/user/src/pages/HomePage.tsx`

改造：

- 顶部增加 `AccountReadinessStrip`：
  - 可用余额总额。
  - 体验额度剩余和最近过期时间。
  - 模型可用状态。
  - 主按钮：`开始生成` / `充值积分` / `查看公开广场`。
- 若 `GET /api/agent/image/v1/capabilities` 返回空：
  - 显示 `平台模型配置中，暂不可生成`。
  - `开始生成` 按钮禁用或跳转到 `#/genpic` 后展示同样原因。
- 公开广场精选区读取 `GET /api/open/image/v1/gallery/images?sort=hot&page_size=8`。
- 游客落地页 CTA 指向 `#/public-gallery` 和 `#/login?returnTo=genpic`。

#### `WorkspacePage.tsx`

位置：`web/user/src/pages/WorkspacePage.tsx`

改造：

- 在加载 capability 后派生 `generationUnavailableReason`：
  - `NO_ROUTE_MODEL`：无可用模型。
  - `NO_PRICE`：模型存在但价格缺失。
  - `INSUFFICIENT_POINTS`：估算不足。
  - `REFERENCE_REQUIRED`：参考图模式未上传图。
- 生成按钮禁用规则：

```ts
const canGenerate =
  parametersReady &&
  prompt.trim().length > 0 &&
  !busy &&
  !generationUnavailableReason
```

- 无模型提示文案固定：
  - 标题：`平台模型配置中`
  - 详情：`当前没有可用生图模型，请稍后再试。`
  - 不展示后台配置细节给普通用户。
- 余额不足提示：
  - 展示分桶余额。
  - 主按钮跳 `#/checkout`。
  - 次按钮跳 `#/profile` 展示兑换码。
- 估算接口必须使用后端 `sufficient` 和 `balance` 扩展字段，不由前端硬算。
- 任务失败卡片必须显示 `error_code`、用户友好文案、`request_id` 或 `task_id`。

#### `ProfilePage.tsx`

位置：`web/user/src/pages/ProfilePage.tsx`

改造：

- 当前“充值积分”按钮从 `app.navigate('api-keys')` 改为 `app.navigate('checkout')`。
- 新增 `BalanceBucketCard`：
  - 体验额度：`trial_points`
  - 订阅额度：`subscription_points`
  - 充值额度：`recharge_points`
  - 冻结额度：`frozen_points`
  - 最近过期：`next_expiring_grant`
- 体验额度过期提醒：
  - `expires_in_days <= config.trial.expiry_reminder_days` 时显示 warning。
  - 已过期但未清理时显示“刷新后将更新余额”，后端必须在余额查询时懒清理过期 grant。
- 流水列表显示 `balance_bucket`、`ledger_type`、`source_type`、`expires_at`。
- 保留兑换码入口。

#### `CheckoutPage.tsx` 新增

位置：`web/user/src/pages/CheckoutPage.tsx`

布局：

- 左侧：固定积分包列表 + 自定义金额输入。
- 右侧：订单摘要、支付方式、支付二维码/跳转按钮、订单状态。
- 底部：订单列表最近 10 条。

状态机：

```ts
type CheckoutStep = 'select' | 'confirm' | 'paying' | 'success' | 'failed' | 'expired'
```

主要接口：

- `GET /api/agent/billing/v1/recharge-options`
- `POST /api/agent/cashier/v1/orders`
- `GET /api/agent/cashier/v1/orders/{order_id}`
- `POST /api/agent/cashier/v1/orders/{order_id}/cancel`
- `POST /api/agent/cashier/v1/orders/{order_id}/mock-pay` 仅测试环境可用

支付展示：

- `payment_display.type=qr_code`：展示二维码。
- `payment_display.type=redirect`：展示“打开支付页”按钮。
- `payment_display.type=form_html`：使用 sandboxed iframe 或新窗口提交，首版优先新窗口。
- `payment_display.type=jsapi`：后端已支持微信 JSAPI 预下单并返回 `client_token`；当前普通 Web 页面展示“不支持当前环境，请使用 H5/扫码”，避免在非微信内环境误导用户发起不可用支付。

轮询：

- 创建订单后每 2 秒查询订单，最多查到 `expires_at`。
- 状态 `completed` 后调用 `app.refreshAccount()` 并展示到账结果。

#### `PublicGalleryPage.tsx`

位置：`web/user/src/pages/PublicGalleryPage.tsx`

改造：

- 游客可访问列表接口，不传 token。
- 列表卡片只展示：
  - 图片缩略图。
  - 作者昵称或匿名。
  - 模型名称、比例、质量。
  - 点赞/收藏计数。
  - 提示词摘要，最多 40 字，服务端返回 `prompt_excerpt`。
- 游客点击卡片：
  - 打开登录弹层或跳 `#/login?returnTo=public-gallery&image_id={id}`。
  - 不展示完整提示词。
- 登录用户点击卡片：
  - 调 `GET /api/open/image/v1/gallery/images/{image_id}`，携带 Bearer token 可返回完整提示词和 viewer 互动状态。
- 点赞/收藏：
  - 未登录：提示登录。
  - 已登录：`POST /api/agent/gallery/v1/images/{image_id}/like` / `favorite`，body `{ "active": true|false }`。
- 同款生成：
  - 详情页按钮“用同款提示词生成”。
  - 将 `prompt`、`route_model_code`、`quality`、`aspect_ratio` 写入 `sessionStorage`，跳 `#/genpic`。

#### `DocsPage.tsx`

位置：`web/user/src/pages/DocsPage.tsx`

改造：

- `openApi.listEndpointDocs()` 必须能解析 `$ref` 后的 OpenAPI path summary/operationId/tags/security。
- 若 `/docs/openapi.json` 返回非 OpenAPI 结构，页面显示错误，不再渲染 `0 / 0 endpoints`。
- examples 接口固定返回：

```json
{
  "items": [
    {
      "id": "openapi-create-task-curl",
      "title": "Open API 创建生图任务",
      "language": "curl",
      "code": "curl ..."
    }
  ]
}
```

- errors 接口固定返回：

```json
{
  "items": [
    {
      "code": "BILLING_INSUFFICIENT_POINTS",
      "message": "积分余额不足",
      "suggestion": "充值或降低规格后重试"
    }
  ]
}
```

### 3.3 管理后台路由

现有路由定义位置：

- `web/admin/src/types.ts`
- `web/admin/src/App.tsx`
- `web/admin/src/components.tsx`

`AdminRouteId` 调整为：

```ts
export type AdminRouteId =
  | 'login'
  | 'overview'
  | 'config'
  | 'readiness'
  | 'cashier'
  | 'routing'
  | 'pricing'
  | 'reviews'
  | 'users'
  | 'user-groups'
  | 'redeem'
  | 'call-records'
  | 'provider-models'
  | 'audit'
  | 'health'
```

新增页面：

- `web/admin/src/pages/ReadinessPage.tsx`
- `web/admin/src/pages/CashierPage.tsx`

导航调整：

- 概览组：`overview`、`readiness`、`health`
- 业务管理：`users`、`user-groups`、`redeem`、`reviews`、`call-records`
- 模型与路由：`provider-models`、`routing`、`pricing`
- 商业化：`cashier`
- 系统：`audit`、`config`

#### `ReadinessPage.tsx`

接口：`GET /api/ops/admin/v1/readiness`

展示检查项：

- 模型账号：至少一个 enabled account。
- 真实模型：至少一个 enabled account model。
- 路由模型：至少一个 enabled public/groups route model。
- 候选模型：每个 enabled route model 至少一个 enabled candidate。
- 价格：每个 enabled route model 的常用 task/quality 至少一个 enabled price。
- 支付：至少一个 visible payment method 可用，或 payment disabled。
- 体验额度：注册送体验配置是否启用、金额是否大于 0、有效期是否大于 0。
- 公开广场：公开开关、审核队列、公开作品数量。
- 文档：OpenAPI JSON 是否可解析、examples/errors 是否正常。

每项返回：

```json
{
  "key": "route_models",
  "label": "路由模型",
  "status": "pass|warn|fail",
  "summary": "1 个已启用",
  "action_route": "routing",
  "action_label": "去配置"
}
```

#### `CashierPage.tsx`

页面分 Tab：

1. `概览`
   - 今日订单数、支付成功率、到账金额、失败回调数、Mock 是否启用。
2. `充值套餐`
   - 固定积分包 CRUD。
   - 自定义金额规则。
3. `支付方式`
   - 可见支付方式：支付宝、微信支付。
   - 每个可见方式选择来源：`alipay_direct` / `wxpay_direct` / `easypay_alipay` / `easypay_wxpay` / `mock`。
4. `渠道实例`
   - 多账号实例 CRUD。
   - provider type：`alipay_direct`、`wxpay_direct`、`easypay_alipay`、`easypay_wxpay`、`mock`、`jeepay_alipay`、`jeepay_wxpay`。
   - 调度策略：`round_robin`、`random`。
   - 限额：单笔最小/最大、每日金额上限。
5. `订单`
   - 查询订单、补单、关闭、查看 provider snapshot。
6. `回调事件`
   - 查看 webhook payload、验签状态、处理结果、重试。

---

## 4. 后端接口契约

所有接口继续使用统一 envelope：

```json
{
  "data": {},
  "meta": { "request_id": "..." }
}
```

错误：

```json
{
  "error": {
    "code": "BILLING_INSUFFICIENT_POINTS",
    "message": "insufficient points",
    "chargeable": false,
    "next_suggestion": "recharge or lower generation settings"
  },
  "meta": { "request_id": "..." }
}
```

### 4.1 认证与注册送体验额度

现有接口：

- `POST /api/agent/auth/v1/login/email-code`
- `POST /api/agent/auth/v1/login/password`

响应扩展：

```json
{
  "access_token": "...",
  "expires_in_seconds": 600,
  "user_id": 123,
  "profile": {},
  "signup_grant": {
    "granted": true,
    "grant_id": 10001,
    "grant_type": "trial",
    "points": "20.00000",
    "expires_at": "2026-06-12T00:00:00Z"
  }
}
```

后端逻辑：

- `auth.LoginWithEmailCode` 判断用户是否新建。
- 若是新用户，调用 `billing.EnsureSignupTrialGrant(ctx, userID, idempotencyKey)`。
- 幂等键固定为 `signup_trial:{user_id}`。
- 若体验额度配置关闭或金额为 0，则 `signup_grant.granted=false`。
- 体验额度 grant 写入 `wallet_grants`，`grant_type=trial`、`source_type=signup`。
- 流水写入 `point_ledgers`，`ledger_type=trial_grant`、`balance_bucket=trial`。

配置项：

```json
{
  "config_category": "billing_trial",
  "config_key": "signup_trial",
  "config_value": {
    "enabled": true,
    "points": "20.00000",
    "valid_days": 7,
    "expiry_reminder_days": 2,
    "grant_once_per_user": true
  },
  "scope": "global"
}
```

Ops 配置仍通过：

- `GET /api/ops/admin/v1/config-tabs`
- `PUT /api/ops/admin/v1/config-tabs/{tab_key}`

新增 tab：`trial_credits`，前端展示在 `ConfigPage.tsx` 或 `CashierPage.tsx` 的套餐配置区；最终以 `CashierPage` 为主入口，`ConfigPage` 只保留底层配置。

### 4.2 余额桶与流水

现有接口：

- `GET /api/agent/billing/v1/balance`
- `GET /api/agent/billing/v1/ledger`

`Balance` 响应替换/扩展：

```json
{
  "available_points": "120.00000",
  "frozen_points": "5.00000",
  "trial_points": "15.00000",
  "subscription_points": "0.00000",
  "recharge_points": "105.00000",
  "gift_points": "15.00000",
  "user_group_multiplier": "1.00000",
  "cny_per_point": "0.31250",
  "buckets": [
    {
      "bucket": "trial",
      "label": "体验额度",
      "available_points": "15.00000",
      "frozen_points": "0.00000",
      "expires_at": "2026-06-12T00:00:00Z",
      "expire_warning": true
    },
    {
      "bucket": "recharge",
      "label": "充值额度",
      "available_points": "105.00000",
      "frozen_points": "5.00000",
      "expires_at": null,
      "expire_warning": false
    }
  ],
  "next_expiring_grant": {
    "grant_id": 10001,
    "grant_type": "trial",
    "available_points": "15.00000",
    "expires_at": "2026-06-12T00:00:00Z"
  },
  "active_subscription": null
}
```

`LedgerEntry` 响应扩展：

```json
{
  "id": 1,
  "ledger_type": "trial_grant|recharge|reserve|consume|refund|expire|admin_adjust|redeem",
  "balance_bucket": "trial|subscription|recharge",
  "change_points": "20.00000",
  "balance_after": "20.00000",
  "bucket_balance_after": "20.00000",
  "reason": "signup trial grant",
  "source_type": "signup|payment_order|task|redeem_code|admin",
  "source_id": "10001",
  "expires_at": "2026-06-12T00:00:00Z",
  "created_at": "..."
}
```

扣费规则：

1. 查询用户 active grants：
   - `status=active`
   - `available_points > 0`
   - `expires_at is null or expires_at > now`
2. 排序：
   - trial：按 `expires_at asc`
   - subscription：按 `expires_at asc`
   - recharge：最后，`expires_at null`
3. 预扣时写 `wallet_reservation_allocations`，记录每个 grant 预扣数量。
4. 任务成功时按 allocation 消耗。
5. 任务失败/部分失败时按 allocation 返还。
6. 余额查询时懒清理过期 trial/subscription grant：
   - 将过期 grant `status=expired`。
   - 写 `point_ledgers ledger_type=expire`。

### 4.3 充值套餐与收银台用户接口

新增路径到 `web/shared/api-types.ts`：

```ts
cashierOptions: '/api/agent/cashier/v1/options',
cashierOrders: '/api/agent/cashier/v1/orders',
cashierOrderDetail: '/api/agent/cashier/v1/orders/{order_id}',
cashierOrderCancel: '/api/agent/cashier/v1/orders/{order_id}/cancel',
cashierOrderMockPay: '/api/agent/cashier/v1/orders/{order_id}/mock-pay',
```

#### `GET /api/agent/cashier/v1/options`

响应：

```json
{
  "plans": [
    {
      "plan_code": "points-100",
      "plan_name": "100 积分包",
      "status": "active",
      "price_cny": "19.90000",
      "points": "100.00000",
      "bonus_points": "0.00000",
      "currency": "CNY",
      "sort_order": 10
    }
  ],
  "custom_amount": {
    "enabled": true,
    "min_amount_cny": "1.00000",
    "max_amount_cny": "999.00000",
    "cny_per_point": "0.31250",
    "bonus_rule": null
  },
  "visible_methods": [
    {
      "method": "alipay",
      "label": "支付宝",
      "enabled": true,
      "display_order": 10
    },
    {
      "method": "wxpay",
      "label": "微信支付",
      "enabled": true,
      "display_order": 20
    }
  ],
  "order_timeout_seconds": 1800
}
```

#### `POST /api/agent/cashier/v1/orders`

请求：

固定积分包：

```json
{
  "purchase_type": "plan",
  "plan_code": "points-100",
  "visible_method": "alipay",
  "client_return_url": "https://example.com/#/checkout"
}
```

自定义金额：

```json
{
  "purchase_type": "custom_amount",
  "amount_cny": "25.00000",
  "visible_method": "wxpay",
  "client_return_url": "https://example.com/#/checkout"
}
```

响应：

```json
{
  "id": 1001,
  "order_no": "PGO202606050001",
  "status": "pending",
  "purchase_type": "plan",
  "visible_method": "alipay",
  "provider_type": "alipay_direct",
  "provider_instance_id": 12,
  "amount_cny": "19.90000",
  "points": "100.00000",
  "bonus_points": "0.00000",
  "currency": "CNY",
  "expires_at": "...",
  "payment_display": {
    "type": "qr_code",
    "qr_code": "https://qr.alipay.com/...",
    "payment_url": "https://openapi.alipay.com/..."
  }
}
```

幂等：

- 请求支持 `Idempotency-Key`。
- 同一用户同一幂等键返回同一订单。

校验：

- 未登录：401。
- 支付方式不可见：400 `PAYMENT_METHOD_UNAVAILABLE`。
- 金额低于/高于限制：400 `PAYMENT_AMOUNT_OUT_OF_RANGE`。
- 用户待支付订单数超过限制：409 `PAYMENT_TOO_MANY_PENDING_ORDERS`。
- 无可用渠道实例：409 `PAYMENT_PROVIDER_UNAVAILABLE`。

#### `GET /api/agent/cashier/v1/orders/{order_id}`

响应同订单详情，必须包含：

- `status`
- `paid_at`
- `completed_at`
- `failure_reason`
- `payment_display`
- `ledger_id` 若已到账

#### `POST /api/agent/cashier/v1/orders/{order_id}/mock-pay`

仅当：

- `APP_ENV != production`
- provider type 为 `mock`
- 当前用户为订单所有者，或 admin debug 调用

响应：订单进入 `completed`，充值额度到账。

### 4.4 收银台后台接口

新增路径到 `API_PATHS.ops`：

```ts
cashierOverview: '/api/ops/admin/v1/cashier/overview',
cashierPlans: '/api/ops/admin/v1/cashier/plans',
cashierPlanDetail: '/api/ops/admin/v1/cashier/plans/{plan_id}',
cashierCustomAmountConfig: '/api/ops/admin/v1/cashier/custom-amount-config',
paymentVisibleMethods: '/api/ops/admin/v1/cashier/visible-methods',
paymentProviderInstances: '/api/ops/admin/v1/cashier/provider-instances',
paymentProviderInstanceDetail: '/api/ops/admin/v1/cashier/provider-instances/{instance_id}',
paymentOrders: '/api/ops/admin/v1/cashier/orders',
paymentOrderDetail: '/api/ops/admin/v1/cashier/orders/{order_id}',
paymentOrderComplete: '/api/ops/admin/v1/cashier/orders/{order_id}/complete',
paymentOrderRefund: '/api/ops/admin/v1/cashier/orders/{order_id}/refund',
paymentWebhookEvents: '/api/ops/admin/v1/cashier/webhook-events',
paymentWebhookEventRetry: '/api/ops/admin/v1/cashier/webhook-events/{event_id}/retry',
```

#### `GET /api/ops/admin/v1/cashier/overview`

响应：

```json
{
  "today_order_count": 10,
  "today_completed_count": 8,
  "today_amount_cny": "199.00000",
  "success_rate": "80.00%",
  "pending_count": 2,
  "failed_webhook_count": 0,
  "enabled_methods": ["alipay", "wxpay"],
  "enabled_provider_instances": 3,
  "mock_enabled": false
}
```

#### `GET/POST /api/ops/admin/v1/cashier/plans`

计划复用现有 `subscription_plans` 表，但新增语义字段：

- `plan_type`: `points_package|subscription`
- `purchase_enabled`: bool

首轮前端只展示 `plan_type=points_package` 且 `purchase_enabled=true`。

创建请求：

```json
{
  "plan_code": "points-100",
  "plan_name": "100 积分包",
  "plan_type": "points_package",
  "purchase_enabled": true,
  "price_cny": "19.90000",
  "points": "100.00000",
  "bonus_points": "0.00000",
  "currency": "CNY",
  "sort_order": 10,
  "description": "适合轻量体验"
}
```

#### `PUT /api/ops/admin/v1/cashier/custom-amount-config`

请求：

```json
{
  "enabled": true,
  "min_amount_cny": "1.00000",
  "max_amount_cny": "999.00000",
  "cny_per_point": "0.31250"
}
```

#### `GET/PUT /api/ops/admin/v1/cashier/visible-methods`

响应：

```json
{
  "items": [
    {
      "method": "alipay",
      "label": "支付宝",
      "enabled": true,
      "source_provider_type": "alipay_direct",
      "scheduler_strategy": "round_robin",
      "display_order": 10
    }
  ]
}
```

规则：

- `method=alipay` 可选 `source_provider_type`: `alipay_direct|easypay_alipay|jeepay_alipay|mock`
- `method=wxpay` 可选 `source_provider_type`: `wxpay_direct|easypay_wxpay|jeepay_wxpay|mock`
- `scheduler_strategy` 支持 `round_robin` / `random`，同一 provider type 下可配置多个账号实例参与调度。

#### `GET/POST /api/ops/admin/v1/cashier/provider-instances`

创建请求：

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
    "max_amount_cny": "500.00000",
    "daily_amount_limit_cny": "5000.00000"
  },
  "config": {
    "app_id": "...",
    "app_private_key": "...",
    "alipay_public_key": "...",
    "gateway_url": "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
  }
}
```

响应不返回明文密钥：

```json
{
  "id": 12,
  "provider_type": "alipay_direct",
  "name": "支付宝沙箱主账号",
  "enabled": true,
  "supported_methods": ["alipay"],
  "credentials_status": {
    "has_secret": true,
    "fingerprint": "sha256:abcd",
    "updated_at": "..."
  }
}
```

支持 provider type：

- `alipay_direct`
- `wxpay_direct`
- `easypay_alipay`
- `easypay_wxpay`
- `mock`
- `jeepay_alipay`
- `jeepay_wxpay`

#### `POST /api/ops/admin/v1/cashier/orders/{order_id}/complete`

后台人工补单/人工确认到账入口，用于支付渠道后台已确认付款成功，但自动回调未到达或到账失败的运营修复场景。

请求：

```json
{
  "provider": "manual_alipay",
  "trade_no": "MANUAL-TRADE-001",
  "reason": "已在渠道后台确认支付成功"
}
```

规则：

- 仅后台 `manage:cashier` 权限可调用。
- `trade_no` 必填，作为人工确认的渠道流水号。
- 若 `provider` 为空，后端按订单 `provider_type`、`provider`、`visible_method` 依次兜底，最后使用 `manual`。
- 仅 `pending` 订单可人工完成；已 `completed` 的同一订单重复调用幂等返回，不重复入账。
- 完成后进入与支付回调相同的充值履约事务：订单 `completed`、写 `recharge` 流水、充值余额桶到账、生成 webhook event 记录，并写审计 `cashier.order.manual_complete`。

#### `POST /api/ops/admin/v1/cashier/orders/{order_id}/refund`

后台退款入口。支持未消费充值余额的全额退款与部分退款闭环，并对已接入退款 adapter 的真实渠道先发起渠道退款。

请求：

```json
{
  "refund_trade_no": "REFUND-TRADE-001",
  "refund_amount_cny": "5.00000",
  "reason": "用户申请退款，余额未消费"
}
```

规则：

- 仅后台 `manage:cashier` 权限可调用。
- `refund_trade_no` 必填，作为退款渠道流水号或人工退款凭证号。
- `refund_amount_cny` 选填；不传时退款剩余可退金额，传入时按订单金额与积分比例折算本次退款积分。
- 仅 `completed` / `partially_refunded` 订单可退款；已 `refunded` 的同一订单重复调用幂等返回，不重复扣减。
- 同一 `refund_trade_no` 幂等返回，不重复调用本地扣减；不同 `refund_trade_no` 可继续退剩余可退金额。
- 对 `alipay_direct`、`wxpay_direct`、`easypay_alipay`、`easypay_wxpay`、`jeepay_alipay`、`jeepay_wxpay` 订单，后端先调用真实渠道退款接口，渠道受理失败时不执行本地余额扣减。
- 真实渠道 adapter 按本次退款金额发起退款：支付宝 `refund_amount`、微信 `amount.refund`、易支付 `money`、JeePay `refundAmount`；微信 `amount.total` 仍为订单总金额。
- `mock` / 人工补单订单维持本地退款闭环。
- 后端只扣回该支付订单生成的 `wallet_grants(source_type=payment_order, source_id=order_id, grant_type=recharge)`。
- 发起真实渠道退款前，后端先执行本地可退款预检；若该充值 grant 已被消费、被非退款任务冻结或可用余额不足，接口返回冲突且不调用渠道退款接口。
- 预检通过后，后端先将该订单本次应退的 recharge grant 积分从 `available_points` 移入 `frozen_points`，并在 `wallet_grants.metadata` 写入 `refund_freeze_trade_no` / `refund_freeze_points`；真实渠道退款失败时释放冻结，真实渠道退款成功后从冻结 grant 完成本地退款。
- 若真实渠道退款已成功/受理但本地 `RefundPaymentOrder` 最终落账失败，后端写入 `payment_webhook_events(event_type=refund.local_finalize_failed,status=failed)` 作为轻量补偿事件；后台 webhook event 重试入口会重新执行本地退款落账，成功后将事件置为 `processed`。
- 独立 worker 每轮会优先调用 `ProcessRefundFinalizeFailures` 自动扫描并处理一小批 `refund.local_finalize_failed` 失败事件；若业务条件仍不满足，事件保持 `failed`，继续进入后台失败事件列表和运营大盘失败回调计数。
- 未退完时订单进入 `partially_refunded`，全额退完时进入 `refunded` 并写 `refunded_at`；`refund_trade_no`、`refunded_amount_cny`、`refunded_points` 暂存于 `provider_payload` 并映射到响应字段。
- 成功后写 `payment_refund` 负向流水，绑定 `order_id`，更新充值余额桶，并写审计 `cashier.order.refund`。

### 4.5 支付回调接口

保留现有公开 webhook 前缀，但扩展渠道：

- `POST /api/open/image/v1/payments/webhooks/alipay`
- `POST /api/open/image/v1/payments/webhooks/wxpay`
- `POST /api/open/image/v1/payments/webhooks/easypay`
- `POST /api/open/image/v1/payments/webhooks/mock` 仅非生产
- `POST /api/open/image/v1/payments/webhooks/jeepay`
- `POST /api/open/image/v1/payments/webhooks/jeepay_alipay`
- `POST /api/open/image/v1/payments/webhooks/jeepay_wxpay`

处理规则：

1. 读取 raw body 和 headers。
2. 根据 path provider type 选择 adapter。
3. adapter 验签并解析：

```go
type PaymentNotification struct {
  ProviderType string
  ProviderInstanceHint string
  OrderNo string
  TradeNo string
  AmountCNY string
  PaidAt time.Time
  EventType string
  Raw map[string]any
}
```

4. 写 `payment_webhook_events`：
   - 唯一键：`provider_type + trade_no + event_type`。
   - 若重复回调，返回渠道要求的成功响应，不重复到账。
5. 查订单 `order_no`。
6. 校验金额、状态、provider snapshot。
7. 订单 `pending -> paid -> completed`。
8. 创建 `wallet_grants grant_type=recharge source_type=payment_order expires_at=null`。
9. 写 `point_ledgers ledger_type=recharge balance_bucket=recharge`。
10. 写审计 `payment.order.completed`。

渠道响应：

- Alipay/EasyPay：返回 `success`。
- WxPay：返回 `{ "code": "SUCCESS", "message": "成功" }`。
- Mock：返回标准 JSON envelope。

### 4.6 公开广场接口

#### `GET /api/open/image/v1/gallery/images`

游客可访问。

Query：

- `page`
- `page_size`
- `sort=latest|hot`
- `route_model_code`
- `task_type`

响应 item：

```json
{
  "id": "img_x",
  "image_url": "/api/open/image/v1/gallery/images/img_x/image",
  "prompt_excerpt": "A futuristic creative workspace...",
  "prompt": null,
  "route_model_code": "plus",
  "route_model_name": "Plus",
  "quality": "2K",
  "aspect_ratio": "16:9",
  "author_name": "user-1",
  "like_count": 12,
  "favorite_count": 3,
  "created_at": "..."
}
```

注意：游客列表不返回完整 `prompt`。

#### `GET /api/open/image/v1/gallery/images/{image_id}`

行为：

- 无 Bearer token：返回 401 `LOGIN_REQUIRED_FOR_GALLERY_DETAIL`。
- 有 token：返回完整详情。

响应：

```json
{
  "id": "img_x",
  "prompt": "完整提示词",
  "negative_prompt": "",
  "route_model_code": "plus",
  "quality": "2K",
  "aspect_ratio": "16:9",
  "reference_assets": [],
  "like_count": 12,
  "favorite_count": 3,
  "liked_by_viewer": true,
  "favorited_by_viewer": false,
  "can_remix": true
}
```

#### 点赞/收藏

现有接口保留：

- `POST /api/agent/gallery/v1/images/{image_id}/like`
- `POST /api/agent/gallery/v1/images/{image_id}/favorite`

请求：

```json
{ "active": true }
```

幂等：

- 多次设置同一 active 值返回当前状态，不重复计数。
- `public_image_interactions` 唯一键 `image_id + user_id`。
- `public_image_stats` 计数在同一事务内增减。

### 4.7 生图能力与估算接口

现有接口保留：

- `GET /api/agent/image/v1/capabilities`
- `GET /api/agent/billing/v1/estimate`
- `POST /api/agent/image/v1/tasks`

`capabilities` 扩展：

```json
{
  "model_groups": [],
  "unavailable_reason": {
    "code": "NO_ROUTE_MODEL",
    "message": "平台模型配置中，暂不可生成。"
  }
}
```

`estimate` 扩展：

```json
{
  "charged_points": "10.00000",
  "display_points": "10.00",
  "sufficient": false,
  "balance": {
    "available_points": "5.00000",
    "trial_points": "5.00000",
    "recharge_points": "0.00000"
  },
  "insufficient_points": "5.00000"
}
```

`create task` 前置失败必须写 call record：

- `MODEL_ROUTE_NOT_FOUND`
- `MODEL_ROUTE_NO_CANDIDATE`
- `ROUTE_MODEL_PRICE_MISSING`
- `BILLING_INSUFFICIENT_POINTS`

### 4.8 后台上线检查接口

新增：

- `GET /api/ops/admin/v1/readiness`

响应：

```json
{
  "overall_status": "pass|warn|fail",
  "items": [
    {
      "key": "model_accounts",
      "label": "模型接入账号",
      "status": "fail",
      "summary": "暂无启用账号",
      "action_route": "provider-models",
      "action_label": "去配置"
    }
  ]
}
```

后台首页 `OverviewPage.tsx` 应嵌入 fail/warn 前 5 项。

### 4.9 后台权限接口与 handler 契约

新增后端 helper：

位置：

- `internal/http/handlers/admin_permissions.go`
- `internal/domain/adminauth/permissions.go`

权限常量：

```go
const (
  PermissionReadOnly = "read:all"
  PermissionManageUsers = "manage:users"
  PermissionManageBilling = "manage:billing"
  PermissionManageCashier = "manage:cashier"
  PermissionManageModels = "manage:models"
  PermissionManageReviews = "manage:reviews"
  PermissionManageConfig = "manage:config"
  PermissionViewAudit = "view:audit"
)
```

首轮规则：

- `super_admin`：全部允许。
- `admin`：除管理员账号管理和高危系统配置外全部允许。
- 默认引导创建的后台账号角色为 `admin`；只有显式配置 `PIC_GALLERY_ADMIN_ROLE=super_admin` 时才创建超级管理员，未知角色值必须回退为 `admin`。
- 后台登录响应必须返回 `permissions: string[]`，前端菜单、按钮和危险操作入口按权限做可见性控制，但后端仍以 `requireAdminPermission` 为最终准入。
- 高危系统配置先包括 `auth_security`、`payments` 两个配置 Tab，必须要求 `manage:dangerous_config`，普通 `admin` 返回 403。
- 若某接口暂无法细分，使用 `requireAdminPermission(r, PermissionX)`，内部仍按两级判断。

本轮不做：

- 不做角色/权限自定义管理页。
- 不做权限组、数据范围、字段级权限和按钮级后端动态配置。
- 不做 C 端用户与后台管理员的组织/租户权限混合。

后续完整自定义 RBAC 时，只替换 permission resolver，并新增权限表/角色表/角色绑定表作为 resolver 的数据来源；业务 handler、OpenAPI 权限名、前端权限字符串不重命名。

---

## 5. 数据模型设计

现有 Ent schema 已具备：

- `wallet_grants`
- `wallet_reservation_allocations`
- `point_ledgers`
- `payment_orders`
- `payment_webhook_events`
- `subscription_plans`
- `user_subscriptions`
- `public_image_interactions`
- `public_image_stats`

本轮优先扩展，不重建。

### 5.1 `wallet_grants`

文件：`internal/repository/ent/schema/walletgrant.go`

现有字段足够支持余额桶。约定：

| 字段 | 约定 |
|------|------|
| `grant_type` | `trial`、`subscription`、`recharge`、`gift` |
| `source_type` | `signup`、`payment_order`、`redeem_code`、`admin_adjust`、`subscription` |
| `source_id` | 对应订单/兑换码/订阅 ID；signup 可为空 |
| `expires_at` | trial/subscription 可有；recharge 必须 null |
| `metadata` | 记录 `order_no`、`plan_code`、`admin_id`、`config_version` |

需要新增索引：

```go
index.Fields("user_id", "status", "grant_type", "expires_at")
```

### 5.2 `wallet_reservation_allocations`

文件：`internal/repository/ent/schema/walletreservationallocation.go`

用于记录任务预扣来自哪些 grant。

状态：

- `reserved`
- `consumed`
- `refunded`
- `cancelled`

新增唯一约束建议：

```go
index.Fields("wallet_grant_id", "task_id", "reservation_cycle").Unique()
```

### 5.3 `point_ledgers`

文件：`internal/repository/ent/schema/pointledger.go`

需要新增字段：

```go
field.String("balance_bucket").MaxLen(32).Default("recharge")
field.String("source_type").MaxLen(32).Default("")
field.Int64("source_id").Optional().Nillable()
field.String("bucket_balance_after").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000")
field.Time("expires_at").Optional().Nillable()
```

新增索引：

```go
index.Fields("user_id", "balance_bucket")
index.Fields("source_type", "source_id")
```

`idempotency_key` 当前唯一索引要允许空值在 SQLite/Postgres 下表现一致。若 Ent/SQLite 空值唯一有坑，实现层必须只在非空时写入，并测试多条空值 ledger。

### 5.4 `payment_orders`

文件：`internal/repository/ent/schema/paymentorder.go`

现有字段不足以支持收银台多渠道。新增：

```go
field.String("purchase_type").MaxLen(32).Default("plan") // plan|custom_amount
field.String("visible_method").MaxLen(32).Default("") // alipay|wxpay
field.String("provider_type").MaxLen(32).Default("") // alipay_direct|wxpay_direct|easypay|mock
field.Int64("provider_instance_id").Optional().Nillable()
field.JSON("provider_snapshot", map[string]any{}).Optional()
field.JSON("payment_display", map[string]any{}).Optional()
field.Time("completed_at").Optional().Nillable()
field.Int64("ledger_id").Optional().Nillable()
field.String("idempotency_key").MaxLen(128).Optional().Nillable()
```

`provider` 字段保留兼容，写入 `visible_method` 或 `provider_type` 的兼容值；前端新代码不再依赖它。

新增索引：

```go
index.Fields("user_id", "idempotency_key").Unique()
index.Fields("provider_type", "trade_no")
index.Fields("provider_instance_id")
index.Fields("completed_at")
```

状态流转：

```mermaid
stateDiagram-v2
  [*] --> pending
  pending --> paid
  paid --> completed
  pending --> expired
  pending --> canceled
  pending --> failed
  paid --> failed
  completed --> refund_requested
  refund_requested --> refunding
  refunding --> refunded
```

当前已实现后台 `/#/cashier` 对 `completed` 订单的最小退款 UI：录入退款交易号和原因，确认后调用 refund API 并刷新订单行。

### 5.5 `payment_provider_instances` 新表

新增文件：`internal/repository/ent/schema/paymentproviderinstance.go`

参考 Sub2API `payment_provider_instances`。

字段：

```go
field.String("provider_type").MaxLen(32).NotEmpty()
field.String("name").MaxLen(100).Default("")
field.JSON("config_encrypted", map[string]any{}).Optional()
field.String("credentials_fingerprint").MaxLen(128).Default("")
field.JSON("supported_methods", []string{}).Optional()
field.Bool("enabled").Default(true)
field.Int("sort_order").Default(0)
field.Int("scheduler_weight").Default(100)
field.JSON("limits", map[string]any{}).Optional()
field.Bool("refund_enabled").Default(false)
field.String("health_status").MaxLen(32).Default("unknown")
field.String("last_error").MaxLen(255).Default("")
field.Time("last_used_at").Optional().Nillable()
field.JSON("metadata", map[string]any{}).Optional()
```

索引：

```go
index.Fields("provider_type")
index.Fields("enabled")
index.Fields("provider_type", "enabled")
index.Fields("sort_order")
```

配置加密：

- 首轮复用项目现有密钥/配置加密方式；如没有统一 secret manager，则先使用 `config.Security.SecretKey` 派生 AES-GCM key。
- 响应永不返回明文 `config_encrypted`。

### 5.6 `payment_visible_methods` 新表或配置项

推荐使用配置项，减少表数量。

`config_items`：

```json
{
  "config_category": "cashier",
  "config_key": "visible_methods",
  "config_value": {
    "items": [
      {
        "method": "alipay",
        "label": "支付宝",
        "enabled": true,
        "source_provider_type": "alipay_direct",
        "scheduler_strategy": "round_robin",
        "display_order": 10
      }
    ]
  }
}
```

调度游标也用配置或 Redis 均可；首轮为数据库兼容，新增配置：

```json
{
  "config_category": "cashier",
  "config_key": "scheduler_state",
  "config_value": {
    "alipay:alipay_direct": { "last_instance_id": 12 }
  }
}
```

并发注意：round-robin 游标更新必须在事务内或允许轻微不均衡。支付调度不要求强一致公平。

### 5.7 `subscription_plans`

文件：`internal/repository/ent/schema/subscriptionplan.go`

新增：

```go
field.String("plan_type").MaxLen(32).Default("points_package")
field.Bool("purchase_enabled").Default(true)
```

现有订阅相关表和类型保留。若已有 seed 的 `basic-monthly`、`plus-monthly` 是订阅语义，迁移时设为：

- `plan_type=points_package` 如果当前产品把它们当一次性积分包使用。
- 或 `plan_type=subscription`、`purchase_enabled=false` 如果要保留为未来订阅。

本方案选择：现有 `basic-monthly`、`plus-monthly` 首轮作为 points package 使用，避免用户端无套餐。

### 5.8 公开广场数据

文件：

- `internal/repository/ent/schema/publicimageinteraction.go`
- `internal/repository/ent/schema/publicimagestat.go`
- `internal/repository/ent/schema/imageresult.go`

现有 interaction/stat 表可用。

需要确认 `imageresult` 保存完整 prompt/task snapshot。如果当前只在 `image_tasks` 有 prompt，则公开详情 service 需要 join task，列表只返回 excerpt。

---

## 6. 后端服务与文件规划

### 6.1 新增/修改包

#### Billing

文件：

- Modify: `internal/domain/billing/types.go`
- Modify: `internal/service/billing/service.go`
- Modify: `internal/service/billing/store.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Test: `internal/service/billing/service_test.go`
- Test: `internal/repository/entstore/billing_store_test.go`

新增能力：

- `EnsureSignupTrialGrant(ctx, req SignupTrialGrantRequest)`
- `ExpireGrants(ctx, userID int64, now time.Time)`
- `ReserveTask` 按 grant allocation 预扣。
- `FinalizeTask` 按 allocation 消耗/返还。
- `CompleteRechargeOrder` 创建 recharge grant。

核心类型：

```go
type BalanceBucket struct {
  Bucket string `json:"bucket"`
  Label string `json:"label"`
  AvailablePoints string `json:"available_points"`
  FrozenPoints string `json:"frozen_points"`
  ExpiresAt *time.Time `json:"expires_at,omitempty"`
  ExpireWarning bool `json:"expire_warning"`
}

type SignupTrialGrantRequest struct {
  UserID int64
  Points string
  ValidDays int
  ReminderDays int
  IdempotencyKey string
}
```

#### Cashier

新增目录：

- Create: `internal/domain/cashier/types.go`
- Create: `internal/service/cashier/service.go`
- Create: `internal/service/cashier/store.go`
- Create: `internal/service/cashier/provider.go`
- Create: `internal/service/cashier/scheduler.go`
- Create: `internal/service/cashier/mock_provider.go`
- Create: `internal/service/cashier/alipay_provider.go`
- Create: `internal/service/cashier/wxpay_provider.go`
- Create: `internal/service/cashier/easypay_provider.go`
- Create: `internal/service/cashier/jeepay_provider.go`
- Create: `internal/repository/entstore/cashier_store.go`

Provider interface：

```go
type Provider interface {
  Type() string
  CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error)
  ParseNotification(ctx context.Context, req NotificationRequest) (PaymentNotification, error)
  QueryPayment(ctx context.Context, req QueryPaymentRequest) (QueryPaymentResult, error)
}
```

Scheduler：

```go
type Scheduler interface {
  Pick(ctx context.Context, method VisibleMethod, amount decimal.Decimal) (PaymentProviderInstance, error)
}
```

首版策略：

- `round_robin`
- `random`

Sub2API 借鉴点：

- `backend/internal/payment/load_balancer.go`
- `backend/internal/payment/provider/alipay.go`
- `backend/internal/payment/provider/wxpay.go`
- `backend/internal/payment/provider/easypay.go`
- `backend/internal/service/payment_order_provider_snapshot.go`
- `backend/internal/handler/payment_webhook_handler.go`

注意：只借鉴设计和必要代码思路，不直接复制大量实现；Pic Gallery 仍遵守本仓库错误码、envelope、Ent schema 和测试风格。

#### HTTP handlers

文件：

- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/api.go`
- Create: `internal/http/handlers/cashier_api.go` 或拆分后从 `api.go` 调用
- Create: `internal/http/handlers/admin_permissions.go`

新增 handler：

- `HandleCashierOptions`
- `HandleCashierOrders`
- `HandleCashierOrderDetail`
- `HandleAdminCashierOverview`
- `HandleAdminCashierPlans`
- `HandleAdminCashierPlanDetail`
- `HandleAdminCashierCustomAmountConfig`
- `HandleAdminPaymentVisibleMethods`
- `HandleAdminPaymentProviderInstances`
- `HandleAdminPaymentProviderInstanceDetail`
- `HandleAdminCashierOrders`
- `HandleAdminCashierOrderDetail`
- `HandleAdminPaymentWebhookEvents`
- `HandleAdminPaymentWebhookEventRetry`
- `HandleAdminReadiness`

#### Gallery

文件：

- Modify: `internal/service/gallery/*` 若存在；否则相关逻辑在 `imagetask`/handlers 中需抽出。
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/gallery_api_test.go`

能力：

- 游客列表返回 excerpt。
- 登录详情返回完整 prompt。
- 点赞/收藏幂等。
- 公开详情登录态 Bearer 可选解析。

#### Docs

文件：

- Modify: `internal/http/handlers/api.go` docs handlers
- Modify: `web/shared/open-api.ts`
- Test: `internal/http/router/legacy_routes_test.go`
- Test: `api/openapi/openapi_test.go`

能力：

- `/docs/openapi.json` 必须返回完整 OpenAPI JSON，不得返回 health payload。
- `/docs/examples` 和 `/docs/errors` 返回固定结构。

### 6.2 配置

Modify:

- `internal/config/config.go`
- `.env.example` 或项目对应 env 示例
- `web/user/vite.config.ts`
- `web/admin/vite.config.ts`

新增配置：

```go
type CashierConfig struct {
  Enabled bool
  MockEnabled bool
  OrderTimeoutSeconds int
  MaxPendingOrdersPerUser int
  SiteBaseURL string
}
```

默认：

- 本地：`MockEnabled=true`
- 生产：`MockEnabled=false`
- `OrderTimeoutSeconds=1800`
- `MaxPendingOrdersPerUser=3`

Vite：

- `web/user/vite.config.ts` 默认 API target 改为 `http://127.0.0.1:8080`。
- `web/admin/vite.config.ts` 增加 `/api` 和 `/docs` proxy，默认 `http://127.0.0.1:8080`，同时仍支持 `VITE_API_BASE_URL`。

---

## 7. 支付渠道 Adapter 契约

### 7.1 支付宝官方 `alipay_direct`

配置：

```json
{
  "app_id": "202100...",
  "app_private_key": "-----BEGIN PRIVATE KEY-----...",
  "alipay_public_key": "-----BEGIN PUBLIC KEY-----...",
  "gateway_url": "https://openapi.alipay.com/gateway.do",
  "sign_type": "RSA2"
}
```

创建支付：

- 桌面优先 `alipay.trade.precreate`，返回 QR。
- 若预创建不可用，可回退电脑网站支付 URL。
- `notify_url` 自动生成：`{site_base_url}/api/open/image/v1/payments/webhooks/alipay`
- `return_url` 使用请求 `client_return_url` 或 `{site_base_url}/#/checkout`

验签：

- 使用支付宝公钥验签。
- 校验 `out_trade_no`、`trade_no`、`total_amount`、`trade_status in TRADE_SUCCESS|TRADE_FINISHED`。

### 7.2 微信官方 `wxpay_direct`

配置：

```json
{
  "app_id": "...",
  "mch_id": "...",
  "api_v3_key": "...",
  "merchant_private_key": "-----BEGIN PRIVATE KEY-----...",
  "merchant_certificate_serial": "...",
  "wechat_pay_public_key": "-----BEGIN PUBLIC KEY-----...",
  "wechat_pay_public_key_id": "..."
}
```

创建支付：

- 支持 Native、H5、JSAPI 三种预下单：
  - Native：`POST /v3/pay/transactions/native`，返回 `code_url`，映射为 `payment_display.type=qr_code`。
  - H5：`POST /v3/pay/transactions/h5`，返回 `h5_url`，映射为 `payment_display.type=redirect`。
  - JSAPI：`POST /v3/pay/transactions/jsapi`，返回 `prepay_id` 后生成 `appId/timeStamp/nonceStr/package/signType/paySign`，序列化为 `payment_display.client_token`。
- 用户端普通 Web 环境暂不直接唤起微信 JSAPI；收到 `payment_display.type=jsapi` 时提示改用 H5 或扫码，避免非微信内环境支付失败。

验签：

- 验证 `Wechatpay-Signature`、`Wechatpay-Timestamp`、`Wechatpay-Nonce`、`Wechatpay-Serial`。
- 解密 resource。
- 校验 `out_trade_no`、`transaction_id`、`amount.total`。

### 7.3 易支付 `easypay`

配置：

```json
{
  "pid": "...",
  "pkey": "...",
  "api_base_url": "https://z-pay.cn/",
  "alipay_channel_id": "",
  "wxpay_channel_id": ""
}
```

创建支付：

- 根据 visible method 转为 EasyPay `type=alipay|wxpay`。
- 返回跳转 URL 或 QR URL。

验签：

- 按 EasyPay 参数排序和 pkey 计算签名。
- 校验 `out_trade_no`、`trade_no`、`money`、`trade_status=TRADE_SUCCESS` 或兼容成功状态。

### 7.4 Mock `mock`

配置：

```json
{
  "auto_success": false,
  "success_delay_seconds": 0
}
```

创建支付：

- 返回 `payment_display.type=mock`。
- 前端测试环境展示“模拟支付成功”按钮。

生产保护：

- production 环境不允许启用 Mock visible method。
- production 环境即使 DB 有 mock instance，`GET options` 不返回。

### 7.5 JeePay adapter

配置 schema：

```json
{
  "gateway_url": "",
  "app_id": "",
  "mch_no": "",
  "key": "",
  "notify_url": "",
  "return_url": "",
  "way_code": "ALI_PC"
}
```

首轮行为：

- 后台可创建 `jeepay_alipay` / `jeepay_wxpay` 渠道实例，并通过可见支付方式参与 `round_robin` / `random` 调度。
- 创建订单时按 JeePay `/api/pay/unifiedOrder` 契约构造支付 URL，参数包含 `mchNo`、`appId`、`mchOrderNo`、`wayCode`、`amount`、`notifyUrl`、`returnUrl`、`signType=MD5`、`sign`。
- 当渠道实例配置 `payment_mode=api/qrcode/qr_code` 时，后端向 JeePay `/api/pay/unifiedOrder` 发起 `application/x-www-form-urlencoded` POST 预下单，并将 `payUrl/codeUrl/qrCode/payData` 映射为统一 `payment_display`。
- `channel_extra` / `channelExtra` / `channel_extra_json` / `channelExtraJSON` 支持对象或字符串配置；对象会序列化为紧凑 `channelExtra` 并纳入 MD5 签名，覆盖微信 JSAPI、小程序、H5、服务商、分账、支付宝 JSAPI/服务商等 JeePay 原生扩展参数。
- 回调入口接收 JeePay form payload，按 `mchNo/appId` 匹配渠道实例，排除 `sign/signType` 和空值后按 key 排序拼接并追加 `key` 做 MD5 验签。
- 回调金额以分为单位，需与订单人民币金额完全一致；成功回调进入通用 `MarkOrderPaid` 履约事务，保证重复回调不重复入账。
- 主动查单通过 `POST {gateway}/api/pay/query`，按 `mchNo/appId/mchOrderNo/signType/sign` 查询，`state=2` 归一化为平台 `paid`，金额分转元后参与金额一致性校验。
- 退款通过 `POST {gateway}/api/refund/refundOrder`，请求 `mchNo/appId/mchOrderNo/payOrderId/mchRefundNo/refundAmount/currency/refundReason/reqTime/version/signType/sign`；`state=2` 视为成功，`state=0/1` 视为渠道已受理。

---

## 8. 开发实施计划

### Phase 0: 契约与测试基线

#### Task 0.1: 固化 API path 和 TS 类型

Files:

- Modify: `web/shared/api-types.ts`
- Modify: `web/shared/user-api.ts`
- Modify: `web/shared/admin-api.ts`

Steps:

1. 添加 `checkout` / cashier / readiness 相关 API_PATHS。
2. 添加 `BalanceBucket`、`CashierOptions`、`CashierOrder`、`PaymentProviderInstance`、`PaymentVisibleMethod` 类型。
3. 保持旧 `PaymentOrder` 兼容字段，但新增字段必须可选。
4. 运行：

```bash
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
```

Expected: 类型错误只来自尚未实现页面引用时，完成本 task 后 shared 层通过。

#### Task 0.2: 修复本地开发代理

Files:

- Modify: `web/user/vite.config.ts`
- Modify: `web/admin/vite.config.ts`
- Modify: `README.md` 或 `docs/runbooks/*`

Steps:

1. 用户端默认 proxy target 改为 `http://127.0.0.1:8080`。
2. 管理端增加 `/api` 和 `/docs` proxy。
3. 文档写明 `VITE_API_PROXY_TARGET` 和 `VITE_API_BASE_URL`。
4. 验证：

```bash
npm --prefix web/user run build
npm --prefix web/admin run build
```

### Phase 1: 余额桶与注册送体验额度

#### Task 1.1: 扩展 Ent schema

Files:

- Modify: `internal/repository/ent/schema/pointledger.go`
- Modify: `internal/repository/ent/schema/paymentorder.go`
- Modify: `internal/repository/ent/schema/subscriptionplan.go`
- Modify: `internal/repository/ent/schema/walletgrant.go`
- Modify: `internal/repository/ent/schema/walletreservationallocation.go`
- Create: `internal/repository/ent/schema/paymentproviderinstance.go`

Steps:

1. 按 §5 增加字段和索引。
2. 运行 Ent generate：

```bash
go generate ./internal/repository/ent
```

3. 运行 schema tests：

```bash
go test ./internal/repository/ent/schema ./internal/repository/entstore
```

#### Task 1.2: Billing store 支持 grant 分桶

Files:

- Modify: `internal/service/billing/store.go`
- Modify: `internal/repository/entstore/billing_store.go`
- Test: `internal/repository/entstore/billing_store_test.go`

Test cases:

- 创建 trial grant 后 balance 返回 trial bucket。
- 过期 trial grant 在 GetBalance 时被标记 expired，并写 expire ledger。
- recharge grant 不过期。
- ReserveTask 优先预扣最早过期 trial。
- FinalizeTask 部分成功时按 allocation 消耗和返还。

#### Task 1.3: 注册送体验额度

Files:

- Modify: `internal/service/auth/service.go`
- Modify: `internal/service/billing/service.go`
- Modify: `internal/http/handlers/api.go`
- Test: `internal/http/router/auth_api_test.go` 或对应 auth tests

Steps:

1. 增加 `billing.EnsureSignupTrialGrant`。
2. 登录/注册接口返回 `signup_grant`。
3. 重复登录不重复发放。
4. 配置关闭时不发放。

### Phase 2: 收银台后端

#### Task 2.1: Cashier domain 和 store

Files:

- Create: `internal/domain/cashier/types.go`
- Create: `internal/service/cashier/store.go`
- Create: `internal/repository/entstore/cashier_store.go`
- Test: `internal/repository/entstore/cashier_store_test.go`

Test cases:

- 创建 provider instance 不返回密钥明文。
- visible method 配置可读写。
- round-robin/random 可选择 enabled 且未超限实例。
- 创建订单写 provider snapshot。

#### Task 2.2: Cashier service

Files:

- Create: `internal/service/cashier/service.go`
- Create: `internal/service/cashier/scheduler.go`
- Create: `internal/service/cashier/provider.go`
- Test: `internal/service/cashier/service_test.go`

Test cases:

- 固定积分包下单。
- 自定义金额按 `cny_per_point` 换算 points。
- 金额越界失败。
- 待支付订单超限失败。
- 无可用渠道失败。
- 回调幂等到账一次。

#### Task 2.3: Payment adapters

Files:

- Create: `internal/service/cashier/mock_provider.go`
- Create: `internal/service/cashier/alipay_provider.go`
- Create: `internal/service/cashier/wxpay_provider.go`
- Create: `internal/service/cashier/easypay_provider.go`
- Create: `internal/service/cashier/jeepay_provider.go`
- Test: adapter tests

Test cases:

- Mock create + mock pay。
- Alipay 签名参数构造和 notify 验签。
- WxPay 回调验签/解密用 fixture。
- EasyPay sign 校验。
- JeePay 下单 URL 签名参数构造、notify 坏签名、金额不一致、成功到账和重复回调幂等。

Sub2API reference:

- `/Users/fatballfish/Documents/Projects/GoProjects/Public/sub2api/backend/internal/payment/provider/alipay.go`
- `/Users/fatballfish/Documents/Projects/GoProjects/Public/sub2api/backend/internal/payment/provider/wxpay.go`
- `/Users/fatballfish/Documents/Projects/GoProjects/Public/sub2api/backend/internal/payment/provider/easypay.go`

#### Task 2.4: Cashier HTTP API

Files:

- Modify: `internal/http/router/router.go`
- Create/Modify: `internal/http/handlers/cashier_api.go`
- Test: `internal/http/router/cashier_api_test.go`

Test cases:

- 用户读取 options。
- 用户创建 plan/custom amount 订单。
- 用户查询/取消订单。
- Mock pay 非生产成功，生产禁用。
- Admin CRUD provider instance。
- Admin CRUD plans/custom amount config。
- Webhook 重复回调只到账一次。

### Phase 3: 用户端闭环

#### Task 3.1: App 路由和 CheckoutPage

Files:

- Modify: `web/user/src/types.ts`
- Modify: `web/user/src/App.tsx`
- Modify: `web/user/src/components.tsx`
- Create: `web/user/src/pages/CheckoutPage.tsx`

Acceptance:

- `#/checkout` 登录后可访问。
- 未登录跳 login 并回跳。
- 固定套餐和自定义金额都能创建订单。
- Mock 支付成功后余额刷新。

#### Task 3.2: ProfilePage 余额桶

Files:

- Modify: `web/user/src/pages/ProfilePage.tsx`

Acceptance:

- 显示体验额度、充值额度、订阅额度。
- 体验额度临期 warning。
- 充值按钮跳 `checkout`。
- 流水显示 bucket 和 source。

#### Task 3.3: WorkspacePage 首图和失败引导

Files:

- Modify: `web/user/src/pages/WorkspacePage.tsx`

Acceptance:

- 无模型时禁用生成，不暴露后台术语。
- 余额不足时显示充值/兑换入口。
- 估算展示来自后端的 `sufficient`、`insufficient_points`。
- 任务失败卡片显示用户可理解原因。

#### Task 3.4: HomePage 首图入口

Files:

- Modify: `web/user/src/pages/HomePage.tsx`

Acceptance:

- 新用户看到体验额度和“开始生成”。
- 无模型时看到平台配置提示。
- 公开广场精选能匿名展示。

### Phase 4: 公开广场

#### Task 4.1: 后端游客列表/登录详情

Files:

- Modify: `internal/http/handlers/api.go`
- Modify: gallery store/service files
- Test: `internal/http/router/gallery_api_test.go`

Acceptance:

- 游客列表不返回完整 prompt。
- 游客详情返回 401。
- 登录详情返回完整 prompt。
- like/favorite 幂等计数正确。

#### Task 4.2: PublicGalleryPage 改造

Files:

- Modify: `web/user/src/pages/PublicGalleryPage.tsx`
- Modify: `web/user/src/components.tsx`

Acceptance:

- 未登录可看列表。
- 未登录点详情/点赞/收藏提示登录。
- 登录后详情展示完整提示词。
- 同款生成跳工作台并填入 prompt。

### Phase 5: 后台运营闭环

#### Task 5.1: Readiness API 和页面

Files:

- Create: `internal/service/readiness/service.go`
- Modify: `internal/http/router/router.go`
- Modify: `internal/http/handlers/api.go`
- Create: `web/admin/src/pages/ReadinessPage.tsx`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/types.ts`

Acceptance:

- 后台可见 pass/warn/fail 检查。
- 每项提供 action route。
- Overview 嵌入失败项。

#### Task 5.2: CashierPage

Files:

- Create: `web/admin/src/pages/CashierPage.tsx`
- Modify: `web/admin/src/pages/index.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/components.tsx`
- Modify: `web/shared/admin-api.ts`

Acceptance:

- 可配置套餐、自定义金额、可见方式、渠道实例。
- 可查看订单和回调事件。
- 密钥字段保存后不回显。
- production 不允许启用 Mock。

#### Task 5.3: UsersPage 用户详情增强

Files:

- Modify: `web/admin/src/pages/UsersPage.tsx`
- Modify: `web/shared/admin-api.ts`
- Modify: `internal/http/handlers/api.go`

Acceptance:

- 用户详情显示余额桶、最近流水、订单、任务、API Key。
- 管理员加减积分必须填原因和幂等键。

### Phase 6: 文档、权限、调用记录

#### Task 6.1: Docs API 修复

Files:

- Modify: `internal/http/handlers/api.go`
- Modify: `web/shared/open-api.ts`
- Modify: `web/user/src/pages/DocsPage.tsx`
- Test: docs route tests

Acceptance:

- 文档页 endpoint 数 > 0。
- examples/errors 结构稳定。
- 错误时页面显示“文档接口不可用”而不是复制 HTML。

#### Task 6.2: Permission facade

Files:

- Create: `internal/domain/adminauth/permissions.go`
- Create: `internal/http/handlers/admin_permissions.go`
- Modify: admin handlers in `internal/http/handlers/api.go`
- Test: admin auth tests

Acceptance:

- 新增接口均使用 `requireAdminPermission`。
- `admin` 不能管理管理员账号和高危配置。
- `super_admin` 全部允许。

#### Task 6.3: Call records 前置失败

Files:

- Modify: `internal/service/imagetask/service.go`
- Modify: `internal/repository/entstore/*call*`
- Modify: `web/admin/src/pages/CallRecordsPage.tsx`

Acceptance:

- 无模型、无价格、余额不足都有 call record。
- 后台可按 status/error_code 过滤。

### Phase 7: 验证与 E2E

#### Task 7.1: API smoke

Add/modify:

- `scripts/workflow/api-smoke.sh`
- `scripts/test/*` if present

Must cover:

1. admin login。
2. 配置 Mock 支付。
3. 配置 route model/price 前 readiness fail。
4. 用户注册获得 trial grant。
5. balance buckets 正确。
6. Mock 充值到账。
7. docs endpoints 可解析。

#### Task 7.2: Frontend build/typecheck

Run:

```bash
./scripts/workflow/verify.sh
```

Expected:

- Go tests pass。
- Go vet pass。
- user/admin typecheck pass。
- user/admin build pass。

#### Task 7.3: Browser E2E 手动验收脚本

用本地浏览器验证：

1. 打开 `/#/public-gallery` 未登录可看列表。
2. 登录新账号，看到注册送体验额度。
3. 若模型已配置，可生成首图；若未配置，按钮禁用且提示。
4. 打开 `/#/checkout`，用 Mock 支付充值。
5. 回到 `/#/profile`，充值额度增加且不过期。
6. 后台 `/#/cashier` 可看到订单和回调。
7. 后台 `/#/readiness` 状态符合当前配置。

---

## 9. 错误码清单

新增错误码：

| Code | HTTP | 场景 | 用户文案 |
|------|------|------|----------|
| `SIGNUP_TRIAL_CONFIG_INVALID` | 500 | 体验额度配置非法 | 新人额度配置异常，请稍后再试。 |
| `PAYMENT_METHOD_UNAVAILABLE` | 400 | 支付方式不可用 | 当前支付方式暂不可用，请选择其他方式。 |
| `PAYMENT_AMOUNT_OUT_OF_RANGE` | 400 | 金额越界 | 充值金额不在允许范围内。 |
| `PAYMENT_TOO_MANY_PENDING_ORDERS` | 409 | 待支付订单过多 | 请先完成或取消已有订单。 |
| `PAYMENT_PROVIDER_UNAVAILABLE` | 409 | 无可用渠道实例 | 支付渠道暂不可用，请稍后再试。 |
| `PAYMENT_PROVIDER_NOT_IMPLEMENTED` | 501 | 渠道能力未实现 | 当前支付渠道暂未开放。 |
| `PAYMENT_SIGNATURE_INVALID` | 400 | 回调验签失败 | 支付回调校验失败。 |
| `PAYMENT_AMOUNT_MISMATCH` | 409 | 回调金额不一致 | 支付金额异常，已进入人工核查。 |
| `LOGIN_REQUIRED_FOR_GALLERY_DETAIL` | 401 | 游客访问详情 | 登录后可查看完整提示词。 |
| `ROUTE_MODEL_PRICE_MISSING` | 409 | 模型价格缺失 | 平台模型价格配置中，请稍后再试。 |

---

## 10. 监控与审计

### 10.1 审计 action

新增：

- `billing.signup_trial_grant`
- `billing.grant_expire`
- `cashier.plan.create`
- `cashier.plan.update`
- `cashier.provider.create`
- `cashier.provider.update`
- `cashier.provider.disable`
- `cashier.visible_method.update`
- `payment.order.create`
- `payment.order.cancel`
- `payment.order.paid`
- `payment.order.completed`
- `payment.webhook.received`
- `payment.webhook.failed`
- `gallery.image.like`
- `gallery.image.favorite`
- `readiness.check`

### 10.2 指标

后台 dashboard/readiness 至少展示：

- 今日订单数。
- 支付成功率。
- 支付回调失败数。
- Mock 是否启用。
- 新用户体验额度发放数。
- 体验额度临期用户数。
- 生图前置失败数，按 error_code。
- 公开广场列表 PV、详情登录拦截次数。

告警建议：

- 5 分钟内支付回调验签失败 > 5：P1。
- 10 分钟内支付成功但到账失败 > 0：P0。
- 模型 readiness 从 pass 变 fail：P0。
- OpenAPI docs 解析失败：P2。

---

## 11. 兼容与迁移

1. 旧 `GET /api/agent/billing/v1/plans` 保留，内部返回 points package。
2. 旧 `POST /api/agent/billing/v1/orders` 保留一版兼容：
   - 内部转调 cashier create order。
   - 响应保持 `PaymentOrder` 旧字段，同时包含新字段。
3. 旧 `payment_url=mock://checkout` 禁止再生成。
4. `subscription_plans` 不删除订阅相关字段；本轮只是新增 `plan_type` 和 `purchase_enabled`。
5. `gift_points` 字段前端兼容旧语义，但新接口明确 `trial_points`。
6. 公开广场旧列表字段 `prompt` 对游客改为 null，新增 `prompt_excerpt`；登录详情不受影响。

---

## 12. 风险与应对

| 风险 | 概率 | 影响 | 应对 |
|------|------|------|------|
| 支付渠道验签差异导致回调失败 | 中 | 高 | Adapter 单测使用 fixture；Mock 链路先跑通；后台保留回调重试和人工补单。 |
| 余额桶预扣/返还出错造成错扣 | 中 | 高 | allocation 表记录每次预扣来源；所有任务结算走幂等；增加账务一致性测试。 |
| 游客列表泄露完整提示词 | 中 | 中 | Open API 列表 service 强制只返回 `prompt_excerpt`；详情接口强制登录。 |
| Mock 支付误上线 | 低 | 高 | production 配置和接口双重禁止；readiness 标红；后台启用 Mock 需 super_admin。 |
| 后台权限未来迁移成本高 | 中 | 中 | 本轮新增 permission facade，handler 不直接依赖 role 字符串。 |

---

## 13. 最终交付定义

代码交付前必须完成：

```bash
./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
BASE_URL=http://localhost:8080 ./scripts/workflow/api-smoke.sh
```

若 Docker 不可用，必须至少完成 SQLite 本地 API smoke，并在交付说明中标注 Docker blocker。

人工验收截图建议保存到：

- `.codex/acceptance-home.png`
- `.codex/acceptance-workspace.png`
- `.codex/acceptance-checkout.png`
- `.codex/acceptance-profile-buckets.png`
- `.codex/acceptance-public-gallery-guest.png`
- `.codex/acceptance-admin-cashier.png`
- `.codex/acceptance-admin-readiness.png`

---

## 14. 待人工确认项

当前已确认大方向，无阻塞项。后续实现前仍建议确认：

| # | 内容 | 影响 |
|---|------|------|
| 1 |注册送体验额度默认点数和默认有效期，例如 `20 点 / 7 天` 是否合适。| 影响 seed 配置和默认文案。 |
| 2 | 自定义金额充值默认汇率，例如沿用 `1 点 = 0.3125 元`。| 影响收银台金额换算。 |
| 3 | 支付宝/微信沙箱是否需要首轮在仓库提供配置示例模板。| 影响 docs 和后台表单提示。 |
| 4 | 公开广场列表是否展示提示词摘要，还是只展示“风格标签”。| 影响游客转化策略和提示词保护。 |

---

## 15. 推荐实施顺序

严格按以下顺序：

1. 代理和 docs 修复，保证本地体验不再误导。
2. Ent schema + billing bucket，先让账务模型稳。
3. 注册送体验额度。
4. Cashier 后端 + Mock。
5. 用户端 checkout/profile/workspace/home。
6. Cashier 支付宝/微信/易支付 adapter。
7. 后台 cashier/readiness。
8. 公开广场游客/登录详情/点赞收藏。
9. DocsPage、API Key quickstart、调用记录、权限 facade。
10. E2E、review gate、smoke。

这份方案是后续实现的契约来源。实现过程中若发现需要改接口路径、字段名、状态机或页面路由，必须先更新本文档并说明原因，再改代码。

---

## 16. 实施进度记录

### 2026-06-05

- 已补齐 `POST /api/agent/cashier/v1/orders` 在新收银台路径下的非 Mock 支付展示契约：
  - `alipay_direct` 下单返回并持久化 `payment_display.payment_url`，URL 包含 `app_id`、`out_trade_no`、`total_amount`、`notify_url`、`return_url`、`biz_content` 等标准参数，并使用渠道实例 `app_private_key` / `private_key` 生成 RSA2 `sign`，`payment_display.signed=true`。
  - `easypay_alipay` / `easypay_wxpay` 生成并持久化 `submit.php` 跳转 URL，按易支付规则生成 `sign` 和 `sign_type=MD5`。
  - `mock` 仅返回 `payment_display.type=mock` 并通过 `/api/agent/cashier/v1/orders/{order_id}/mock-pay` 完成测试履约，不再生成顶层 `payment_url=mock://checkout/{order_no}`。
  - `wxpay_direct` 已完成 Native/H5/JSAPI 预下单：Native `code_url` 映射 `qr_code`，H5 `h5_url` 映射 `payment_url`，JSAPI `prepay_id` 生成 `client_token` 支付参数；普通 Web 页面收到 JSAPI 展示时提示改用 H5/扫码。
- 已扩展 `domainbilling.CreateOrderRequest` / `CreateCustomAmountOrderRequest`，允许 handler 提前生成 `order_no` 并传入 `payment_url`、`qr_code`、`client_token`，Ent store 与 Memory store 均按传入值落库。
- 已补齐 `POST /api/agent/cashier/v1/orders` 的 `Idempotency-Key` 契约：
  - 新收银台入口和旧 `POST /api/agent/billing/v1/orders` 兼容入口均读取 `Idempotency-Key` 请求头。
  - `domainbilling.CreateOrderRequest` / `CreateCustomAmountOrderRequest` 增加 `IdempotencyKey`，Memory store 与 Ent store 均写入 `payment_orders.idempotency_key`。
  - 创建流程最开始按 `(user_id, idempotency_key)` 查询既有订单，若存在则直接复用原订单，避免重复点击创建多笔 pending 订单。
  - 幂等复用发生在待支付订单上限、渠道调度、支付展示构建之前；即使当前 pending 订单数已达上限，同一 `Idempotency-Key` 重放也会返回原订单，不会误报 `PAYMENT_TOO_MANY_PENDING_ORDERS`。
- 已补齐 `/docs/examples` 固定示例结构：
  - 后端 `GET /docs/examples` 返回 `items[]`，每项包含 `id`、`title`、`language`、`code`，不再只返回旧的 `name/method/path/request/response` 结构。
  - 示例内容提供可直接复制的 curl 片段，避免文档页复制 HTML/bootstrap payload 或 JSON 包装对象。
  - `web/shared/open-api.ts` 优先解析新结构，并保留对旧结构的兼容 fallback。
- 已补齐 `CreateTask` 路由模型前置失败的调用记录错误码契约：
  - `MODEL_ROUTE_NOT_FOUND`、`MODEL_ROUTE_NO_CANDIDATE`、`ROUTE_MODEL_PRICE_MISSING`、`MODEL_ROUTE_NOT_VISIBLE` 已集中定义在 `pkg/errs`。
  - 路由模型不存在/不可见/无候选/价格缺失不再落通用 `NOT_FOUND`、`FORBIDDEN`、`CONFLICT`，便于后台按 `error_code` 筛选和后续告警聚合。
  - 已增加 Ent 真实存储集成测试，验证 `imagetask.CreateTask` 自动落库的前置失败能被 `AdminCallRecordStore` 按 `status=failed` + 专用 `error_code` 查询。
  - `web/user/src/pages/DocsPage.tsx` 侧边栏复制内容改为真实代码片段拼接，不再复制 examples JSON 数组。
- 已补齐后台用户积分调整的幂等键交互契约：
  - `web/admin/src/pages/UsersPage.tsx` 的积分调整弹窗显式展示并要求填写 `Idempotency-Key`，不再由页面静默生成后直接提交。
  - 弹窗保留“生成”按钮，用 `admin-user-{user_id}-points-{uuid}` 作为默认幂等键格式，便于运营复制、追溯和重试同一次调整。
  - 积分调整保存条件改为 `change_points`、`reason`、`idempotency_key` 三者均必填，和后端 `Idempotency-Key is required` 约束保持一致。
  - 前端保存规则已抽到 `web/admin/src/pages/userPointAdjustment.ts`，contract `web/admin/src/pages/userPointAdjustment.contract.ts` 锁定三字段必填和 `admin-user-{user_id}-points-{uuid}` 幂等键前缀，避免后续页面改动绕过运营审计护栏。
- 已补齐支付错误码契约：
  - `PAYMENT_METHOD_UNAVAILABLE`
  - `PAYMENT_PROVIDER_UNAVAILABLE`
  - `PAYMENT_PROVIDER_NOT_IMPLEMENTED`
  - `PAYMENT_TOO_MANY_PENDING_ORDERS`
  - `PAYMENT_SIGNATURE_INVALID`
  - `PAYMENT_AMOUNT_MISMATCH`
- 已将旧 `POST /api/agent/billing/v1/orders` 改为兼容壳，内部复用 cashier 下单 helper：
  - 旧 `provider=mock` / 空 provider 映射为 `visible_method=mock`。
  - 旧 `provider=alipay` / `alipay_direct` / `easypay_alipay` 映射为 `visible_method=alipay`。
  - 旧 `provider=wxpay` / `wechatpay` / `wxpay_direct` / `easypay_wxpay` 映射为 `visible_method=wxpay`。
  - 旧入口创建的 points package 订单也进入 cashier 充值链路，支付成功后进入 `recharge_points`，不再走订阅额度到账语义。
- 已补齐易支付 webhook 的最小真实回调闭环：
  - `POST /api/open/image/v1/payments/webhooks/easypay` 支持 `application/x-www-form-urlencoded` 回调体。
  - 根据回调 `pid` 匹配后台启用且已配置的 `easypay_alipay` / `easypay_wxpay` 渠道实例。
  - 使用渠道实例配置中的 `key` / `pkey` 按易支付 MD5 规则验签，签名失败返回 `PAYMENT_SIGNATURE_INVALID`。
  - 回调金额 `money` 会在 `billing.MarkOrderPaid` 的 MemoryStore 与 EntStore 两条路径中统一校验，不一致返回 `PAYMENT_AMOUNT_MISMATCH`。
  - 合法回调完成充值订单，重复回调幂等返回，不重复到账。
- 已补齐易支付 `easypay_alipay` / `easypay_wxpay` API 预下单最小真实闭环：
  - 当后台渠道实例配置 `payment_mode=api` / `payment_mode=qrcode` / `prepay_mode=api` / `trade_type=api` 时，`POST /api/agent/cashier/v1/orders` 会调用易支付网关 `/mapi.php`，不再只返回 hosted `submit.php` 跳转地址。
  - 请求以 `application/x-www-form-urlencoded` POST，字段包含 `pid`、`type=alipay|wxpay`、`out_trade_no`、`notify_url`、`return_url`、`name`、`money`、`clientip`，可选 `cid` / `device`。
  - 请求按易支付 MD5 规则生成 `sign`，并携带 `sign_type=MD5`；`clientip` 参与 API 模式签名。
  - 易支付响应 `code=1` 时，将 `payurl` / `payurl2` 写入订单 `payment_url` 和 `payment_display.payment_url`，将 `qrcode` 写入订单 `qr_code` 和 `payment_display.qr_code`。
  - 若返回二维码，`payment_display.type=qr_code`；否则保留 `payment_display.type=redirect`。`payment_display.prepay_mode=api` 用于前端区分 API 预下单和 popup hosted 跳转。
  - 非 2xx、非法 JSON、`code!=1` 或响应中同时缺少 `payurl/payurl2/qrcode` 时，统一返回 `PAYMENT_PROVIDER_UNAVAILABLE`。
- 已补齐支付宝官方 `alipay_direct` notify 回调的最小真实闭环：
  - `POST /api/open/image/v1/payments/webhooks/alipay_direct` / `.../alipay` 支持 `application/x-www-form-urlencoded` 回调体。
  - 根据回调 `app_id` 匹配后台启用且已配置的 `alipay_direct` 渠道实例。
  - 使用渠道实例配置中的 `alipay_public_key` / `public_key` 进行 RSA2 验签，签名失败返回 `PAYMENT_SIGNATURE_INVALID`。
  - 回调金额 `total_amount` 会在 `billing.MarkOrderPaid` 的 MemoryStore 与 EntStore 两条路径中统一校验，不一致返回 `PAYMENT_AMOUNT_MISMATCH`。
  - 合法回调完成充值订单，重复回调幂等返回，不重复到账。
- 已补齐微信支付官方 `wxpay_direct` notify 回调的最小真实闭环：
  - `POST /api/open/image/v1/payments/webhooks/wxpay_direct` / `.../wxpay` / `.../wechatpay` 支持微信支付 v3 JSON 回调体。
  - 根据回调头 `Wechatpay-Serial` 匹配后台启用且已配置的 `wxpay_direct` 渠道实例。
  - 使用渠道实例配置中的 `wechat_pay_public_key` / `public_key` 验证 `Wechatpay-Signature`，签名失败返回 `PAYMENT_SIGNATURE_INVALID`。
  - 使用渠道实例配置中的 `api_v3_key` 对 `resource` 做 `AEAD_AES_256_GCM` 解密。
  - 回调金额 `amount.total` 按分转元后进入 `billing.MarkOrderPaid` 的 MemoryStore 与 EntStore 统一校验，不一致返回 `PAYMENT_AMOUNT_MISMATCH`。
  - 合法回调完成充值订单，重复回调幂等返回，不重复到账。
- 已补齐微信支付官方 `wxpay_direct` Native 预下单的最小真实闭环：
  - 当后台渠道实例未配置静态 `qr_code` / `code_url` 时，`POST /api/agent/cashier/v1/orders` 会调用微信支付 v3 Native 下单接口 `/v3/pay/transactions/native`。
  - 请求体包含 `appid`、`mchid`、`description`、`out_trade_no`、`notify_url`、`amount.total`、`amount.currency=CNY`。
  - 请求头使用 `WECHATPAY2-SHA256-RSA2048`，通过渠道实例配置的 `merchant_private_key` 与 `merchant_certificate_serial` 生成签名。
  - 微信返回的 `code_url` 写入订单 `qr_code` 和 `payment_display.qr_code`，`payment_display.prepay_mode=native`。
- 已补齐微信支付官方 `wxpay_direct` H5 预下单的最小真实闭环：
  - 当后台渠道实例配置 `payment_mode=h5` / `prepay_mode=h5` / `trade_type=h5`，且未配置静态 `payment_url` / `h5_url` 时，`POST /api/agent/cashier/v1/orders` 会调用微信支付 v3 H5 下单接口 `/v3/pay/transactions/h5`。
  - 请求体包含 `appid`、`mchid`、`description`、`out_trade_no`、`notify_url`、`amount.total`、`amount.currency=CNY`，以及 `scene_info.payer_client_ip`、`scene_info.h5_info.type`。
  - 请求头同样使用 `WECHATPAY2-SHA256-RSA2048`，通过渠道实例配置的 `merchant_private_key` 与 `merchant_certificate_serial` 生成签名。
  - 微信返回的 `h5_url` 写入订单 `payment_url` 和 `payment_display.payment_url`，`payment_display.type=redirect`，`payment_display.prepay_mode=h5`。
- 已补齐微信支付官方 `wxpay_direct` JSAPI 预下单的最小真实闭环：
  - 当后台渠道实例配置 `payment_mode=jsapi` / `prepay_mode=jsapi` / `trade_type=jsapi`，且未配置静态 `client_token` 时，`POST /api/agent/cashier/v1/orders` 会调用微信支付 v3 JSAPI 下单接口 `/v3/pay/transactions/jsapi`。
  - 请求体包含 `appid`、`mchid`、`description`、`out_trade_no`、`notify_url`、`amount.total`、`amount.currency=CNY`，以及 `payer.openid`。
  - 请求头同样使用 `WECHATPAY2-SHA256-RSA2048`，通过渠道实例配置的 `merchant_private_key` 与 `merchant_certificate_serial` 生成签名。
  - 微信返回的 `prepay_id` 会按 JSAPI 前端支付参数规则生成 `appId`、`timeStamp`、`nonceStr`、`package=prepay_id=...`、`signType=RSA`、`paySign`，序列化后写入订单 `client_token` 和 `payment_display.client_token`，`payment_display.type=jsapi`，`payment_display.prepay_mode=jsapi`。
- 已补齐支付渠道配置密钥遮蔽：
  - 后台 provider instance 响应不返回 `app_private_key`、`merchant_private_key`、`api_v3_key`、`key`、`pkey` 等密钥明文。
  - `credentials_status` 保留 `has_secret` 与指纹，便于后台判断配置是否完整且不暴露密钥。
- 已补齐后台人工补单/人工确认到账最小闭环：
  - 新增 `POST /api/ops/admin/v1/cashier/orders/{order_id}/complete`，仅后台 `manage:cashier` 权限可调用。
  - 请求要求填写 `trade_no`，可选 `provider` 与 `reason`；`provider` 为空时按订单已有 `provider_type`、`provider`、`visible_method` 兜底。
  - 后端复用 `billing.CompleteRechargeOrder` 事务完成订单，确保订单 `pending -> completed`、充值余额桶到账、`recharge` 流水生成、webhook event 记录生成且重复调用不重复入账。
  - 后台写审计 `cashier.order.manual_complete`，metadata 包含 `order_no`、`provider`、`trade_no`、`reason`。
  - 管理后台 `/#/cashier` 最近订单列表对 `pending` 订单提供“补单”操作，弹窗要求录入渠道交易号和原因，完成后刷新当前订单状态。
- 已补齐后台未消费充值余额全额退款最小闭环：
  - 新增 `POST /api/ops/admin/v1/cashier/orders/{order_id}/refund`，仅后台 `manage:cashier` 权限可调用。
  - 请求要求填写 `refund_trade_no`，可选 `reason`；退款交易号写入 `provider_payload.refund_trade_no` 并映射到 `PaymentOrder.refund_trade_no` 响应字段。
  - 后端新增 `billing.RefundPaymentOrder`，允许 `completed/partially_refunded -> partially_refunded/refunded`；已 `refunded` 或同一 `refund_trade_no` 重复调用幂等返回。
  - 已接入 `alipay_direct`、`wxpay_direct`、`easypay_alipay`、`easypay_wxpay`、`jeepay_alipay`、`jeepay_wxpay` 真实渠道退款最小 adapter：后台退款时先请求渠道退款，渠道返回失败或不可用时不执行本地余额扣减。
  - 支付宝官方退款调用 `alipay.trade.refund`，请求包含 `out_trade_no`、`refund_amount`、`refund_reason`、`out_request_no` 并按 RSA2 签名。
  - 微信官方退款调用 `/v3/refund/domestic/refunds`，请求包含 `out_trade_no`、`out_refund_no`、`reason`、`amount.refund/total/currency` 并复用微信支付 v3 Authorization 签名。
  - 易支付退款调用 `api.php?act=refund` 表单接口，按 `out_trade_no` 退款，必要时可按渠道 `trade_no` 兜底重试。
  - JeePay 退款调用 `/api/refund/refundOrder` 表单接口，请求包含 `mchNo`、`appId`、`mchOrderNo/payOrderId`、`mchRefundNo`、`refundAmount`、`currency`、`refundReason`、`reqTime`、`version`、`signType`、`sign`；`state=2` 视为成功，`state=0/1` 视为渠道已受理。
  - 后端新增 `CheckRefundPaymentOrder` 只读预检，在真实渠道退款前确认订单本次退款金额仍可本地扣回；若充值 grant 已消费、被非退款任务冻结或可用余额不足，返回冲突且不会调用上游退款接口。
  - 后端新增 `FreezeRefundPaymentOrder` / `ReleaseRefundPaymentOrder`，渠道退款前先将目标订单本次退款积分从 recharge grant 的 `available_points` 移到 `frozen_points` 并记录退款冻结 metadata；渠道失败时释放冻结，渠道成功后 `RefundPaymentOrder` 从冻结 grant 完成本地退款，避免 provider 调用期间被生图任务并发消费。
  - 后端新增 `RecordRefundFinalizeFailure` 补偿事件：真实渠道退款已成功/受理但本地 `RefundPaymentOrder` 失败时，写入 `payment_webhook_events(event_type=refund.local_finalize_failed,status=failed)`，记录 `refund_trade_no`、`operator_admin_id` 与失败原因；后台 `webhook-events/{event_id}/retry` 对该事件会重新执行本地退款落账，成功后事件置为 `processed`。
  - 后端新增 `ProcessRefundFinalizeFailures` 批处理入口，独立 worker 每轮会优先扫描 `refund.local_finalize_failed + failed` 事件并自动重试本地退款落账；自动重试仍失败时不吞掉事件，继续保留 `failed` 状态供后台列表、运营大盘和人工重试处理。
  - 退款只扣回该支付订单生成且仍未消费的 recharge grant；渠道侧拒付、争议扣款等无法走原订单退款语义的场景，已通过独立订单级人工追扣动作处理。
  - 成功退款会按累计退款金额将订单标记为 `partially_refunded` 或 `refunded`，全额退款时写 `refunded_at` 并将 recharge grant 置为 `refunded`，写绑定 `order_id` 的 `payment_refund` 负向流水，并写审计 `cashier.order.refund`。
  - 管理后台 `/#/cashier` 最近订单列表对 `completed` / `partially_refunded` 订单提供“退款”操作，弹窗要求录入退款交易号，退款金额可选，完成后刷新当前订单状态。
- 已补齐后台收银台订单级人工追扣闭环：
  - 新增 `POST /api/ops/admin/v1/cashier/orders/{order_id}/chargeback`，要求 `Idempotency-Key` 请求头，Body 为 `charge_points` 和 `reason`。
  - 后端按订单 `user_id` 调用管理员积分调整能力写入负向 `admin_adjust` 流水，原因固定带上订单号，避免追扣流水脱离订单上下文。
  - 幂等键复用 `billing.AdminAdjust` 的冲突校验：同一键同一参数重复调用返回当前余额，不重复扣减；同一键不同用户、积分或原因返回冲突。
  - 本地内存账本负向管理员调整已对齐 Ent 持久化账本的扣减优先级，按 `recharge -> gift -> subscription -> trial` 扣减余额桶，避免测试环境出现总余额和分桶余额不一致。
  - 接口响应返回 `{ order, balance }`，便于后台立即展示扣减后的用户余额；审计动作写为 `cashier.order.chargeback`。
  - OpenAPI、`web/shared/api-types.ts`、`web/shared/admin-api.ts` 已补齐追扣契约；管理后台 `/#/cashier` 最近订单对 `completed` / `partially_refunded` / `refunded` 订单提供“追扣”操作，弹窗要求填写追扣积分、原因和幂等键。
- 已按用户确认的 B 方案补齐后台权限门面：
  - 当前保留 `super_admin` / `admin` 两级角色，`super_admin` 全部允许。
  - `admin` 可执行普通运营、用户、计费、收银台、模型、审核和审计查看动作，但不能管理管理员账号，也不能更新 `auth_security` / `payments` 等高危配置。
  - 后端新增 `domainadminauth.PermissionResolver` 接口，`API` 默认使用 `RolePermissionResolver`，并通过 `SetAdminPermissionResolver` 预留未来完整自定义 RBAC 接入点。
  - `PermissionResolver` 已从单纯 `role string` 升级为接收 `AdminPrincipal{ID, Email, Role, Status}`，当前两级角色行为不变，但后续可在 resolver 内按管理员、角色组、组织、策略或权限表做动态决策。
  - 后台登录返回的 `permissions` 与 handler 鉴权均走同一个 resolver，避免前端展示权限和后端执行权限漂移。
  - 管理后台前端新增页面级权限映射，侧边栏只展示当前 session 可访问的页面；直接输入 hash 访问无权限页面时会跳转到第一个可访问页面并提示。
  - 前端未知角色默认无权限，只有后端明确下发 `permissions` 时才开放对应页面，为后续自定义角色/RBAC 做好兼容。
  - 前端权限解析已调整为“存在 `permissions` 字段则完全以该字段为准”，即使后端显式返回空权限数组，也不会回退到内置 `super_admin` / `admin` 权限映射；仅历史会话缺失该字段时才走两级角色兼容回退。
  - 管理员创建链路的默认角色已统一为 `admin`：内存管理员仓库、Ent 管理员认证仓库、Ent schema、初始 SQL migration 和技术设计表均不再默认写入旧 `ops_admin`，避免未显式传角色的新管理员账号登录后没有任何后台权限。
  - OpenAPI 已补齐后台鉴权契约：新增 `Admin Auth` tag、`AdminPermission` 枚举、`AdminRole`、`AdminSession`、`AdminLoginRequest`、`AdminLoginResponse`，并将 `/api/ops/admin/v1/auth/login` 响应从泛型成功包收敛为带 `permissions` 的明确 session 契约。
- 已补齐收银台与上线检查 OpenAPI 契约：
  - 新增 `Agent Cashier`、`Admin Readiness`、`Admin Cashier` tags，并将用户端 `/api/agent/cashier/v1/options`、`/api/agent/cashier/v1/orders`、`/api/agent/cashier/v1/orders/{order_id}`、`/api/agent/cashier/v1/orders/{order_id}/cancel`、`/api/agent/cashier/v1/orders/{order_id}/mock-pay` 写入 OpenAPI。
  - 新增后台 `/api/ops/admin/v1/readiness`、`/api/ops/admin/v1/cashier/overview`、`plans`、`custom-amount-config`、`visible-methods`、`provider-instances`、`orders`、`orders/{order_id}/complete`、`orders/{order_id}/refund`、`webhook-events`、`webhook-events/{event_id}/retry` 契约。
  - `agent.yaml` 已补齐 `CashierOptionsResponse`、`CashierOrderCreateRequest`、`CashierOrderResponse`、`PaymentDisplay`，并扩展 `PaymentOrder` 的 `purchase_type`、`visible_method`、`provider_type`、`provider_instance_id`、`payment_display`、`ledger_id`、`completed_at`、`refund_trade_no` 等收银台字段。
  - `admin.yaml` 已补齐 `ReadinessReport`、`ReadinessCheck`、`AdminCashierOverview`、`AdminCashierPlan*`、`AdminCashierCustomAmountConfig`、`AdminCashierProviderInstance*`、`AdminCashierManualCompleteRequest`、`AdminCashierRefundRequest`、`AdminPaymentWebhookEvent*`。
- 已增加/更新测试覆盖：
  - 支付宝配置渠道下单返回带 RSA2 `sign` 的支付 URL，并标记 `payment_display.signed=true`。
  - 易支付 popup 下单返回带 MD5 签名的 `submit.php` URL，并可从订单详情读回。
  - 易支付 API 模式调用 `/mapi.php`，校验 POST 参数和 `payurl/qrcode -> payment_url/qr_code` 展示映射。
  - 易支付回调坏签名、金额不一致、成功到账和重复回调幂等。
  - 支付宝 notify 回调坏签名、金额不一致、成功到账和重复回调幂等。
  - 微信支付 v3 notify 回调坏签名、金额不一致、成功到账和重复回调幂等。
  - 微信支付 v3 Native 预下单请求路径、请求体、Authorization 头和 `code_url -> qr_code` 展示映射。
  - 微信支付 v3 H5 预下单请求路径、请求体、Authorization 头、`scene_info` 和 `h5_url -> payment_url` 展示映射。
  - 微信支付 v3 JSAPI 预下单请求路径、请求体、Authorization 头、`payer.openid` 和 `prepay_id -> client_token` 展示映射。
  - 后台 provider instance 对 `api_v3_key` / `merchant_private_key` 等支付密钥的遮蔽。
  - 后台人工补单完成 pending 收银台订单、充值余额到账、重复调用不重复入账。
  - 后台退款完成 completed 收银台订单、扣回未消费充值余额、写负向流水、重复调用不重复扣减。
  - Ent store 持久化退款扣减 recharge grant、订单 `refund_trade_no` 映射、`payment_refund` 流水绑定 `order_id`。
  - 后台权限两级角色解析、登录权限列表、高危配置限制和未来 RBAC resolver 替换点。
  - 后台权限路由级覆盖：注入只读 `PermissionResolver` 后，`/api/ops/admin/v1/readiness` 可访问而 `/api/ops/admin/v1/users` 返回 403，证明已挂载业务路由由 permission facade 控制，不依赖写死角色判断。
  - 后台登出接口也已从裸 `requireAdmin` 收敛为 `requireAdminPermission(..., read:all)`，并新增 `TestAdminLogoutUsesPermissionFacade`，证明后续自定义 RBAC resolver 可拦截基础后台会话接口。
  - 管理员默认角色回归：`adminauth.MemoryStore` 和 Ent `AdminAuthStore` 在创建管理员时未传 `role` 会默认写入内置 `admin`，不再写入旧 `ops_admin`。
  - 管理后台权限 contract 覆盖 `admin` 保留运营入口、禁止管理员账号管理权限、未知角色默认无权限、后端显式空 `permissions` 不回退内置角色、自定义角色按 session `permissions` 控制页面和菜单。
  - OpenAPI 权限契约 contract 覆盖后台登录 tag、权限枚举完整性、`AdminSession.permissions` 引用 `AdminPermission`、`AdminSession.role` 引用 `AdminRole`。
  - OpenAPI 收银台与上线检查契约 contract 覆盖用户端收银台路径、后台上线检查路径、后台收银台路径、Bearer 鉴权、订单创建幂等头、关键 request body 必填字段，以及 agent/admin 侧核心 schema。
  - JeePay 最小真实链路：`jeepay_alipay` / `jeepay_wxpay` 可参与可见支付方式调度，下单返回带 MD5 签名的 JeePay `/api/pay/unifiedOrder` URL，`jeepay`、`jeepay_alipay`、`jeepay_wxpay` 公开 form 回调入口完成实例匹配、MD5 验签、金额校验、成功到账和重复回调幂等。
  - JeePay API 预下单模式：当渠道实例配置 `payment_mode=api` / `qrcode` / `qr_code` 时，创建订单会向 JeePay `/api/pay/unifiedOrder` 发起 `application/x-www-form-urlencoded` POST，不再只构造 hosted URL；请求参数复用 hosted URL 的 `mchNo/appId/mchOrderNo/wayCode/amount/notifyUrl/returnUrl/clientIp/signType/sign` 规则。
  - JeePay API 响应 `code=0/1/success` 且返回 `data.payUrl` / `data.codeUrl` / `data.qrCode` / `data.payData` 任一支付展示字段时，后端会映射为 `PaymentOrder.payment_url`、`qr_code` 与 `payment_display`，并保留 `prepay_mode=api`、`way_code`、`channel_trade_no`，便于前端按统一支付展示模型渲染。
  - JeePay `wayCode` 差异化参数首版已补齐结构化 `channel_extra` / `channelExtra` / `channel_extra_json` / `channelExtraJSON` 配置：后台可直接保存 JSON 对象，创建订单时后端序列化为紧凑 `channelExtra` 并纳入 MD5 签名，支持 `WX_JSAPI` 的 `openid`、子商户/子应用字段、H5 场景字段、小程序字段等 JeePay 原生扩展参数；字符串配置仍保持原样透传。
  - 管理后台收银台渠道配置入口已同步 JeePay 真实 adapter 口径，`jeepay_alipay` / `jeepay_wxpay` 展示为可配置渠道，不再标记为“占位”；新增编译期 contract 防止 JeePay 文案回退到 placeholder。
  - 收银台创建订单 `Idempotency-Key` 重放复用原订单，并覆盖 pending 上限已满时的重放场景。
  - Ent store 持久化并复用 `payment_orders.idempotency_key`。
  - Docs examples 固定返回 `id/title/language/code`，用户端 normalizer 与页面复制内容对齐新契约。
  - Docs OpenAPI/examples/errors 归一化已抽到纯模块 `web/shared/open-api-docs.ts`，contract `web/shared/open-api.contract.ts` 锁定有效 OpenAPI 可解析为 endpoint、非 OpenAPI payload 直接报“文档接口不可用”而不是渲染 `0 / 0 endpoints`、复制示例过滤 HTML/bootstrap payload；`DocsPage.tsx` 复用同一 `docsCopyableExamplesText`，避免页面侧重新拼接导致复制错误内容。
  - Docs OpenAPI 端点请求示例已从 `curl /path` 升级为可复制请求骨架：`docsFromOpenApi` 会按 OpenAPI method 生成 `curl -X METHOD`，按 `requestBody.content.application/json.example/examples` 生成 JSON `-d`，并根据 security scheme 区分鉴权头；用户端/后台 bearer 接口展示 `Authorization: Bearer <your_access_token>`，OpenAI 兼容接口展示 `Authorization: Bearer <your_sk_key>`，平台原生 Open API 展示 `X-Access-Key / X-Timestamp / X-Body-SHA256 / X-Signature`，避免开发者把 AK/SK 签名接口误按 Bearer 调用。
  - `web/shared/open-api.contract.ts` 已锁定 OpenAPI 请求示例必须包含请求方法、JSON Content-Type、请求体字段，以及 Open API AK/SK 签名头且不得误用 Bearer；`npm exec tsx shared/open-api.contract.ts`、`npm exec tsx src/pages/docsPageModel.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐端点请求示例后通过。
  - 用户端开发文档页展示模型已抽到 `web/user/src/pages/docsPageModel.ts`，contract `web/user/src/pages/docsPageModel.contract.ts` 可独立运行，覆盖接口分组、鉴权方式、搜索占位、端点计数、分组标签和多关键词检索；`DocsPage.tsx` 下拉筛选继续使用 `All/Agent API/Open API/OpenAI Compat/Ops API` 原始值，但可见文案改为“全部接口/用户端 API/开放 API/OpenAI 兼容接口/管理后台 API”，页面不再直出 `Auth:`、`endpoints`、`DEVELOPER PORTAL` 等开发态英文标签。
  - 开发文档页错误码区已继续补齐排障指导：`docsErrorRows` 将 `/docs/errors` 的 `code/http_status/message/retryable` 归一为“HTTP 状态 / 可重试性 / 处理建议”，并兼容旧二元组 fallback；`DocsPage.tsx` 不再只展示 code/message，开发者可直接看到模型路由、价格缺失、支付渠道不可用、签名错误、余额不足、限速等错误的下一步处理方向。
  - `web/user/src/pages/docsPageModel.contract.ts` 已锁定错误码行模型、旧格式 fallback、支付与模型错误处理建议；`npm exec tsx src/pages/docsPageModel.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐开发文档错误码指导后通过。
  - 用户端 API Key 页展示模型已抽到 `web/user/src/pages/apiKeyRows.ts`，contract `web/user/src/pages/apiKeyRows.contract.ts` 可独立运行，覆盖密钥状态、scope 中文标签、Quickstart 示例、表头、删除确认和 Secret 重置提示；`ApiKeysPage.tsx` 不再直出 `DEVELOPER PORTAL` / `Example Request (cURL)` / `copy` 等开发态标签，并补齐重置 Secret 与删除前二次确认，创建或重置返回的一次性 Secret 会在本地保留到用户查看/复制。
  - Docs 运行时 smoke 已加固：`/docs/openapi.json` 必须包含用户收银台、公开广场、后台上线检查和后台收银台退款等关键路径；`/docs/examples` 必须返回可复制代码且不能是 HTML/bootstrap；`/docs/errors` 必须包含模型路由与支付链路关键错误码（如 `MODEL_ROUTE_NOT_FOUND`、`MODEL_ROUTE_NO_CANDIDATE`、`ROUTE_MODEL_PRICE_MISSING`、`PAYMENT_PROVIDER_UNAVAILABLE`、`PAYMENT_SIGNATURE_INVALID`、`PAYMENT_AMOUNT_MISMATCH`）。
  - Ent store 持久化 `payment_url`、`qr_code`、`client_token`。
  - 支付方式不可用/渠道实例不可用返回固定 `PAYMENT_*` 错误码。
  - 旧 billing 下单入口复用 cashier 链路，订单带 `visible_method` / `provider_type` / `payment_display`，回调后充值余额到账。
  - 公开广场同款生成上下文统一为 `galleryEditContext` 契约，写入 `prompt`、`route_model_code`、`quality`、`aspect_ratio` 后跳转工作台。
  - 工作台恢复 `galleryEditContext` 时保留 prompt，并在能力配置加载完成后一次性恢复模型、质量和比例，避免初始化 reset effect 清空同款生成内容。
  - 用户端私有图库筛选展示模型已抽到 `web/user/src/pages/galleryRows.ts`，contract `web/user/src/pages/galleryRows.contract.ts` 可独立运行，覆盖 `public/approved` 统一归入“已公开”、`reviewing/pending_review` 统一归入“审核中”、未知状态原样保留、多关键词搜索和分组/类型/任务状态/公开状态组合筛选；`GalleryPage.tsx` 继续保留后端原始 `visibility_status`，但“已公开/审核中”筛选不再漏掉历史兼容状态。
  - 公开广场游客列表新增 `prompt_excerpt` 脱敏摘要，前端列表卡片优先展示摘要，并补齐模型、质量、比例的一眼可见信息。
  - 公开广场 `prompt_excerpt` 对短提示词也不返回完整 prompt，确保游客只能看到灵感预览，完整提示词仍需登录详情查看。
  - 个人中心余额桶规范化固定展示体验额度、订阅额度、充值额度三类，即使某类余额为 0 也保留展示位；服务端返回的 bucket 明细会覆盖默认值。
  - 首页公开广场精选卡片优先展示 `prompt_excerpt`，匿名用户不再看到图片 ID 占位，首屏灵感区可读性对齐公开广场列表。
  - 首页公开广场精选点击边界对齐公开广场：游客点击作品提示登录并回跳首页；登录用户点击后拉取公开详情再展示完整 prompt 和互动能力。
  - 用户收银台订单状态区新增统一支付展示模型，支持 `qr_code`、`redirect`、`form/form_html`、`jsapi` 和无展示信息的明确状态。
  - `payment_display.type=jsapi` 首版不再误导用户直接支付，页面明确提示当前环境暂不支持微信 JSAPI，并引导改用 H5 或扫码支付。
  - 用户收银台创建订单后按 `expires_at` 执行 2 秒轮询；`pending` 未过期继续查单，支付成功后自动刷新账户余额，过期或失败后停止轮询并展示明确状态。
  - 用户收银台补齐 pending 订单取消动作：订单状态区对未过期 `pending` 订单展示“取消订单”，调用 `/api/agent/cashier/v1/orders/{order_id}/cancel` 后刷新当前订单和最近订单；取消后隐藏 Mock 支付按钮，避免已取消订单继续支付。
  - 用户收银台底部新增最近 10 条订单列表，读取真实订单接口；每行展示套餐/自定义充值标题、金额、到账积分、状态、支付方式和创建时间，点击后接管为当前订单继续刷新或查看支付信息。
  - 个人中心流水新增 `profileLedgerRows` 展示模型，统一兼容后端方案字段 `balance_bucket` 与旧字段 `bucket_type`，并固定展示额度桶、流水类型、来源类型、有效期和金额方向。
  - `LedgerEntry` 前端共享类型对齐真实账务接口：`balance_bucket`、`bucket_balance_after`、`expires_at` 等字段可直接透传，旧 mock 展示字段改为可选，由页面层补齐展示文案。
  - OpenAPI 已补齐注册送体验额度、余额桶与积分流水分桶字段契约：登录 `SessionResponse.data.signup_grant` 明确引用 `SignupGrant`；`BalanceSummary` 增加 `trial_points` 与 `buckets`；新增 `BalanceBucket`；`PointLedgerEntry` 增加 `balance_bucket`、`bucket_type`、`source_type`、`source_id`、`bucket_balance_after`、`expires_at` 等账务追踪字段；`GrantExpirySummary` 对齐运行态 `grant_type` 与 `available_points`。
  - 工作台新增 `workspaceGenerateReadiness` 生成前置状态模型，统一收敛无可用模型、参数未完整、提示词过短、估价中、余额不足和提交中状态，禁用按钮时展示用户可理解原因。
  - 工作台无模型提示避免暴露后台、账号、route/provider 等内部术语；余额不足时保留充值/兑换入口并复用后端 estimate 的 `sufficient` / `insufficient_points`。
  - 后台上线检查页新增 `readinessRows` 行模型，将每个检查项的 `fix_route/action_route` 统一转成可点击修复入口，不再只展示 route 文本。
  - 后台上线检查页将 blocking 状态改为中文运营文案“阻塞上线/非阻塞”，保留 pass/warn/fail 状态，降低运营理解成本。
  - 后台上线检查页状态文案已进一步中文化：顶部整体状态 `pass/warn/fail` 展示为“全部通过/存在警告/存在阻塞”，检查项状态展示为“通过/警告/阻塞”；行模型保留 `rawStatus`，避免页面继续直出内部检查枚举但仍可排障。
  - 后台用户详情最近流水新增 `userDetailLedgerRows` 展示模型，显示额度桶、来源、有效期、桶内余额和金额方向，方便运营排查体验额度/充值余额问题。
  - 后台用户详情流水表扩展为 6 列布局，避免新增账务维度挤压原有类型、余额和时间信息。
  - 后台调用记录页从薄表格升级为运维排障视图：
    - 页面位置：`web/admin/src/pages/CallRecordsPage.tsx`，路由保持 `/#/call-records`。
    - 筛选区固定对接 `GET /api/ops/admin/v1/call-records` 的 `status`、`error_code`、`source_channel`、`provider`、`user_id`、`task_id` 查询参数，`status` 改为结构化下拉，`error_code` 提供常见前置失败码候选。
    - 表格展示任务现场、用户/API Key、入口渠道、抽象模型、质量、Provider、尝试次数、状态、错误码/错误提示、预估/实扣积分、Provider 成本、毛利和生命周期时间，方便运营直接定位无模型、无价格、余额不足、Provider 异常等失败。
    - 新增 `callRecordRows` 行模型和 `web/admin/src/pages/callRecordRows.contract.ts`，锁定失败 tone、错误码提示、API Key 展示、积分/成本格式和成功记录静默展示规则。
  - 余额查询已补齐 trial/subscription grant 懒清理：`GetBalance` 事务内将已过期且仍 active 的体验额度/订阅额度 grant 标记为 `expired`，充值额度 grant 不参与过期清理。
  - 过期清理会按 grant 写入幂等 `expire` 流水，固定幂等键 `expire:wallet_grant:{grant_id}`，并持久化 `balance_bucket`、`source_type`、`source_id`、`bucket_balance_after`、`expires_at`。
  - `expire` 流水领域展示已补齐，返回标题“额度过期”，金额为负向 debit，详情可区分体验额度/订阅额度和系统来源，用户端/后台可继续复用通用流水展示模型。
  - Docker E2E 主链路已从旧 `billing/v1/orders` 兼容下单升级为新收银台主路径：注册后校验 `signup_grant`、trial bucket 与 `trial_grant` 流水，读取 `cashier/v1/options`，创建 `cashier/v1/orders` Mock 订单，调用 `mock-pay`，再校验充值余额桶增量和 `recharge/payment_order` 流水 metadata。
  - Docker E2E 前端 app shell 覆盖新增用户端 `public-gallery`、`checkout` 路由，以及后台 `readiness`、`user-groups`、`cashier`、`call-records` 路由，避免验收脚本继续绕开新增页面。
  - Docker E2E 后台管理主链路新增调用记录前置失败验收：用唯一 `route_model_code` 触发 `MODEL_ROUTE_NOT_FOUND`，再通过 `GET /api/ops/admin/v1/call-records?status=failed&error_code=MODEL_ROUTE_NOT_FOUND&user_id=...&source_channel=web` 验证运营后台可按状态和错误码筛出对应失败记录。
  - 后台收银台回调事件操作区收紧重试边界：只有 `failed` 事件显示可点击“重试”，`processed` 事件展示“已处理”，`received/verified` 等中间态展示“等待处理”，避免运营误以为已处理事件会被重新履约。
  - OpenAPI 进一步锁定后台调用记录排障契约：`GET /api/ops/admin/v1/call-records` 文档补齐 `error_code` 查询参数，并通过 contract 覆盖 `status/error_code/source_channel/provider/user_id/task_id/created_from/created_to` 等筛选项，以及 `provider_cost`、`gross_margin`、`attempt_count`、错误字段和生命周期时间字段。
  - OpenAPI 进一步锁定后台收银台回调事件契约：`POST /api/ops/admin/v1/cashier/webhook-events/{event_id}/retry` 覆盖 Admin Cashier tag、Bearer 鉴权、`event_id` path 参数和 200 响应；`AdminPaymentWebhookEvent.status` 固定包含 `received/verified/processed/failed`，响应 schema 明确引用事件对象。
  - API smoke 已补强后台调用记录前置失败验收：用登录用户触发一次 `route_model_code` 不存在的 `MODEL_ROUTE_NOT_FOUND` 生图前置失败，再用后台 `GET /api/ops/admin/v1/call-records?status=failed&error_code=MODEL_ROUTE_NOT_FOUND&user_id=...&source_channel=web` 验证运营后台可查到该失败记录。
  - 修复前置失败记录的真实边界问题：当 `route_model_code` 长度超过 32 字符时，失败任务的 `abstract_model` 默认回填 route code，但 `image_tasks.abstract_model` 旧上限为 32，导致 preflight failed record 保存失败且被 best-effort 吞掉；现已将 Ent schema、runtime migrate schema 和初始 SQL migration 的 `abstract_model` 上限统一到 64，与 `route_model_code` 对齐。
  - `AdminCallRecordStore` 前置失败回归测试已覆盖超过 32 字符的 missing route code，防止后续再次出现“接口返回错误但后台调用记录丢失”的运营排障断点。
  - 公开广场列表态提示词保护进一步收紧：
    - 页面位置：`web/user/src/pages/PublicGalleryPage.tsx`，路由保持 `/#/public-gallery`。
    - 列表卡片展示统一由 `publicGalleryCardView` 生成，标题只使用 `prompt_excerpt`，无摘要时展示“登录后查看完整提示词”，即使列表对象意外带有完整 `prompt` 也不会渲染。
    - 列表搜索统一由 `publicGallerySearchText` 生成，只索引 `image_id`、`prompt_excerpt`、模型和作者，不再通过搜索框泄露完整 `prompt`。
    - 首页精选公开作品位置：`web/user/src/pages/HomePage.tsx`，路由保持 `/#/home`；`homeGalleryCardView` 只使用 `prompt_excerpt` 或“公开作品”，不在匿名精选卡片展示完整 `prompt`。
    - 登录态详情、复制提示词和同款生成仍保留完整提示词能力，边界保持为“列表摘要、详情完整提示词”。
  - 前端契约新增 `web/user/src/pages/publicGalleryCard.contract.ts` 与 `web/user/src/pages/homeGalleryCard.contract.ts`，锁定公开广场列表、列表搜索和首页精选卡片均不能展示或索引完整 `prompt`。
  - OpenAPI 与运行态进一步锁定公开广场列表/详情边界：
    - `GET /api/open/image/v1/gallery/images` 不再复用后台审核列表 schema，改为独立 `PublicGalleryListResponse` / `PublicGalleryListImage`。
    - 公开广场列表响应固定 `prompt: null`，只返回 `prompt_excerpt`、模型、质量、比例、作者、互动计数和登录态 viewer 互动状态；完整提示词只由登录详情接口返回。
    - `GET /api/open/image/v1/gallery/images/{image_id}` 改为 `PublicGalleryDetailResponse`，并在 OpenAPI 中明确 401 登录要求。
    - 公开列表接口补齐并接通 `sort`、`route_model_code`、`task_type`、`liked`、`favorited` 查询参数；`route_model_code` 筛选对历史数据兼容 `abstract_model` 回退。
    - 后端列表响应新增专用 DTO `publicGalleryListImage`，无论游客还是登录用户，只要走列表接口都不会返回完整 `prompt`；登录用户仍可在列表保留点赞/收藏状态。
  - 新增 OpenAPI contract 覆盖公开广场提示词边界：列表 schema 必须引用 `PublicGalleryListResponse`，列表 item 的 `prompt` 必须为 null，必须提供 `prompt_excerpt`，详情 schema 必须引用 `PublicGalleryDetailResponse` 并文档化 401。
  - 新增/扩展运行态测试：
    - `TestGalleryPublishReviewAndPublicListFlow` 覆盖公开列表 `route_model_code` / `task_type` 命中和未命中筛选。
    - `TestImageTaskStoreListsApprovedPublicImagesWithoutPublishedAt` 覆盖 Ent store 公开列表筛选，并锁定历史数据缺失 `route_model_code` 时回退 `abstract_model` 的兼容行为。
  - 后台运营总览 dashboard 指标闭环进一步收紧：
    - 页面位置：`web/admin/src/pages/OverviewPage.tsx`，路由保持 `/#/overview`，接口保持 `GET /api/ops/admin/v1/metrics/dashboard`。
    - 后端 `HandleAdminDashboard` 新增 `operations` 聚合对象，固定返回今日订单数、支付成功率、失败回调数、Mock 支付是否可见、注册送体验额度用户数、体验额度临期用户数、前置失败数及按错误码分布、公开广场列表 PV、公开详情游客登录拦截数、当前可见支付方式和生成时间。
    - 后台指标卡新增/固定 `payment_success_rate`、`failed_webhook_count`、`signup_trial_users`、`preflight_failures`、`public_gallery_views`、`mock_payment`，旧生图成功率、平均耗时、积分消耗和活跃用户指标补齐 `trend` 字段，避免前端卡片信息缺失。
    - 公开广场列表成功访问会递增 `pic_gallery_public_gallery_list_views_total`，游客访问公开详情被登录要求拦截会递增 `pic_gallery_public_gallery_detail_login_blocks_total`，并同时进入后台 dashboard `operations`。
    - OpenAPI 将 `AdminDashboardResponse` 从 `additionalProperties: true` 收紧为显式 `AdminDashboard` / `AdminDashboardOperations` / `AdminMetric` / `AdminProviderHealth` / `AdminDashboardQueueItem` schema，前端共享类型 `AdminDashboard` 同步新增 `operations`。
    - 后台首页上线检查风险面板已抽出可验收模型 `web/admin/src/pages/overviewReadinessRows.ts`，`OverviewPage.tsx` 只消费归一化行数据；contract `web/admin/src/pages/overviewReadinessRows.contract.ts` 锁定仅展示 fail/warn、保留原始顺序、最多展示前 5 项、fail/warn 状态色、`detail/summary/-` 说明兜底，以及 `fix_route/action_route/readiness` 和 `fix_action/action_label/处理` 操作入口兜底。
    - 后台首页上线检查风险面板的状态文案已中文化：`fail` 展示为“阻塞”、`warn` 展示为“警告”，同时在 row 模型保留 `rawStatus` 供排障和后续扩展使用，避免总览页继续直出内部检查枚举。
  - 后台收银台主动查单同步闭环已补齐：
    - 页面位置：`web/admin/src/pages/CashierPage.tsx`，路由保持 `/#/cashier`。
    - 后端接口：`POST /api/ops/admin/v1/cashier/orders/{order_id}/sync`，OpenAPI `operationId=postAdminCashierOrderSync`，响应固定为 `AdminCashierOrderSyncResponse`，其中 `data.order` 引用 `PaymentOrder`，`data.sync` 引用 `AdminCashierOrderSyncResult`。
    - 管理后台待支付订单新增“查单”入口，调用 `adminApi.syncPaymentOrder(order_id)` 后用返回订单刷新订单列表和详情；已完成时提示“查单已确认到账”，未完成时展示渠道查询状态。
    - 查询 adapter 保留配置驱动骨架：渠道实例 `config` 可声明 `query_status`、`query_trade_no`、`query_amount_cny`，用于 Mock/测试环境验证主动查单契约；支付宝官方、微信支付官方、易支付、JeePay 真实 HTTP 查单均已接入同一 `queryCashierOrderStatus` 契约，不改变前后端接口。
    - 易支付与 JeePay 真实 HTTP 主动查单已接入同一 `/sync` 契约：
      - 易支付读取渠道实例 `gateway_url`、`pid`、`key`，向 `POST {gateway}/api.php` 发起 `act=order`、`out_trade_no={order_no}` 查询；`status=1` 视为已支付，`money` 映射 `amount_cny`，`trade_no` 映射渠道流水号。
      - JeePay 读取渠道实例 `gateway_url`、`mch_no`、`app_id`、`key`，向 `POST {gateway}/api/pay/query` 发起 `mchNo/appId/mchOrderNo/signType/sign` 查询；默认 `query_path` 可通过配置覆盖以兼容不同 JeePay 部署，`state=2` 视为已支付，`amount` 分转元后映射 `amount_cny`，`payOrderId/channelOrderNo` 映射渠道流水号。
      - 易支付/JeePay 查询成功后的入账、金额不一致阻断和幂等逻辑仍复用 `CompleteRechargeOrder`，不新增旁路账务实现。
    - 支付宝官方与微信支付官方真实 HTTP 主动查单已接入同一 `/sync` 契约：
      - 支付宝官方读取渠道实例 `gateway_url`、`app_id`、`app_private_key`，向支付宝网关发起 `alipay.trade.query` 查询，请求使用 RSA2 签名；`TRADE_SUCCESS` / `TRADE_FINISHED` 视为已支付，`total_amount` 映射 `amount_cny`，`trade_no` 映射渠道流水号。
      - 微信支付官方读取渠道实例 `gateway_url`、`mch_id`、`merchant_certificate_serial`、`merchant_private_key`，向 `GET /v3/pay/transactions/out-trade-no/{order_no}?mchid={mch_id}` 发起查询，请求复用微信支付 v3 RSA Authorization 签名；`SUCCESS` 视为已支付，`amount.total` 分转元后映射 `amount_cny`，`transaction_id` 映射渠道流水号。
      - 支付宝/微信查询成功后的入账、金额不一致阻断和幂等逻辑仍复用 `CompleteRechargeOrder`，不新增旁路账务实现。
    - 当渠道查询结果为已支付且本地订单仍为 `pending` 时，复用既有 `CompleteRechargeOrder` 履约链路，确保充值余额桶到账、`recharge/payment_order` 流水、webhook event、幂等语义和人工补单一致；已完成订单重复查单不重复入账。
    - 当渠道查询金额与订单金额不一致时，返回 `PAYMENT_AMOUNT_MISMATCH`，不改订单、不入账，保留人工核查空间。
    - 主动查单 `sync.query_status` 已统一收敛为平台态 `pending/paid/closed/failed/refunded`；支付宝 `TRADE_SUCCESS/TRADE_CLOSED`、微信 `SUCCESS/REVOKED/REFUND/PAYERROR`、易支付 `0/1`、JeePay `2/3/4` 等渠道差异状态先在后端归一化，后台 UI 读取稳定状态和中文 `message`，原始渠道响应仍保留在 `sync.raw` 里供排障。
  - 后台收银台 JeePay `wayCode` 配置入口已产品化：
    - 页面位置：`web/admin/src/pages/CashierPage.tsx`，路由保持 `/#/cashier`，入口仍在“支付渠道实例”新增/编辑弹窗。
    - `jeepay_alipay` / `jeepay_wxpay` 渠道实例弹窗新增 `JeePay wayCode 模板` 辅助区；模板逻辑已从页面抽到纯模块 `web/admin/src/pages/cashierJeePayWayCodeTemplates.ts`，便于独立 contract 验证。
    - 当前模板覆盖 `ALI_PC`、`ALI_JSAPI`、`ALI_PC_SUB_MCH`、`WX_NATIVE`、`WX_JSAPI`、`WX_H5`、`WX_LITE`、`WX_NATIVE_SUB_MCH`、`WX_NATIVE_PROFIT_SHARING`，覆盖支付宝 JSAPI/服务商、微信小程序/服务商/分账等高频 JeePay 扩展参数配置场景。
    - 套用模板时会保留既有 `gateway_url`、`mch_no`、`app_id`、`key`、`client_ip` 等商户配置，只合并 `payment_mode=api`、`way_code` 和对应 `channel_extra` 示例，降低运营手写 JSON 出错概率。
    - `WX_JSAPI` 模板内置 `channel_extra.openid` 示例值；`WX_H5` 模板内置 `channel_extra.sceneInfo` 示例配置；`WX_LITE` 模板内置小程序 `appId/openId` 示例值；服务商模板内置子商户字段示例；分账模板内置 `profitSharingReceivers` 接收方数组示例；高级参数仍可继续在 JSON 文本区直接编辑。
    - JeePay 模板按钮和 tooltip 的可见文案已统一为“可配置示例”口径，禁止回退到“占位/placeholder”描述；JSON 模板内仍保留 `<...>` 示例值，方便运营替换真实商户参数。
- 验证结果：
  - `npm exec tsx src/pages/cashierJeePayWayCodeTemplates.contract.ts` 在 `web/admin` 目录下通过，覆盖 JeePay 模板纯模块可独立执行、基础模板、微信小程序、微信服务商、微信分账、支付宝 JSAPI、支付宝服务商、模板可见文案不含占位/placeholder 以及 `channel_extra` 合并不覆盖既有字段。
  - `npm --prefix web/admin run typecheck` 在新增 JeePay wayCode 模板 contract 与后台弹窗模板入口后通过。
  - `npm --prefix web/admin run build` 在新增 JeePay wayCode 模板 UI 后通过。
  - `./scripts/workflow/verify.sh` 在补齐 JeePay 更多模板并抽离纯模板模块后通过。
  - `./scripts/workflow/api-smoke.sh` 在补齐 JeePay 更多模板并抽离纯模板模块后通过。
  - `./scripts/workflow/review-local.sh --scope working` 在补齐 JeePay 更多模板并抽离纯模板模块后通过。
  - `go test ./internal/http/handlers -run TestNormalizeCashierQueryStatusMapsProviderTerminalStates -count=1` 在补齐主动查单状态归一化后通过。
  - `go test ./internal/http/router -run 'TestAdminCashierOrderSync(CompletesPaidProviderOrder|RejectsPaidAmountMismatch|QueriesAlipayDirectProvider|QueriesWxPayDirectProvider|QueriesEasyPayProvider|QueriesJeePayProvider)' -count=1` 在收敛查单 `query_status=pending/paid/closed/failed/refunded` 后通过。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsCashierAndReadinessContracts -count=1` 在补齐 `AdminCashierOrderSyncResult.query_status` 枚举契约后通过。
  - `npm --prefix web/admin run typecheck` 在前端共享类型新增 `PaymentOrderSyncStatus` 后通过。
  - `go test ./internal/http/router -run 'TestAdminCashierOrderSync(QueriesAlipayDirectProvider|QueriesWxPayDirectProvider|QueriesEasyPayProvider|QueriesJeePayProvider)' -count=1` 在补齐支付宝/微信官方真实 HTTP 主动查单后通过。
  - `go test ./internal/http/handlers ./internal/http/router -run TestNonExistent -count=0` 在补齐支付宝/微信官方真实 HTTP 主动查单后通过。
  - `go test ./internal/http/router -run 'TestAdminCashierOrderSync(CompletesPaidProviderOrder|RejectsPaidAmountMismatch|QueriesEasyPayProvider|QueriesJeePayProvider)' -count=1` 在补齐易支付/JeePay 真实 HTTP 主动查单后通过。
  - `go test ./internal/http/handlers ./internal/http/router -run TestNonExistent -count=0` 在补齐易支付/JeePay 真实 HTTP 主动查单后通过。
  - `go test ./internal/http/router -run 'TestAdminCashierOrderSync(CompletesPaidProviderOrder|RejectsPaidAmountMismatch)' -count=1` 在补齐后台收银台主动查单同步后通过。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsCashierAndReadinessContracts -count=1` 在锁定主动查单 OpenAPI 路径、operationId、响应 schema 和 sync result 必填字段后通过。
  - `npm --prefix web/admin run typecheck` 在接入后台收银台“查单”按钮和共享 API 类型后通过。
  - `go test ./internal/http/router -run 'TestRouterExposesMetricsEndpoint|TestGalleryPublishReviewAndPublicListFlow' -count=1` 在补齐后台运营总览指标和公开广场观测计数后通过。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsAdminDashboardOperationsContract -count=1` 在收紧后台 dashboard OpenAPI 契约后通过。
  - `npm --prefix web/admin run typecheck` 在同步 `AdminDashboardOperations` 前端共享类型后通过。
  - `node --check scripts/e2e/docker-e2e.mjs` 通过。
  - `node --check scripts/e2e/docker-e2e.mjs` 在新增调用记录前置失败 E2E 验收后通过。
  - `go test ./internal/domain/billing -run TestPopulateLedgerDisplayFieldsFormatsExpireLedger -count=1` 通过。
  - `go test ./internal/repository/entstore -run TestBillingStoreGetBalanceExpiresOldTrialAndSubscriptionGrants -count=1` 通过。
  - `go test ./internal/domain/billing ./internal/service/billing ./internal/repository/entstore -count=1` 通过。
  - `go test ./internal/http/router -run TestCashierWxPayDirectOrderUsesNativePrepayCodeURL` 通过。
  - `go test ./internal/http/router -run TestCashierWxPayDirectOrderUsesH5PrepayURL` 通过。
  - `go test ./internal/http/router -run TestCashierWxPayDirectOrderUsesJSAPIPrepayToken` 通过。
  - `go test ./internal/http/router -run TestCashierWxPayWebhook` 通过。
  - `go test ./internal/http/router -run TestAdminCashierProviderInstanceCreateAndUpdate` 通过。
  - `go test ./internal/http/router -run TestCashierAlipayWebhook` 通过。
  - `go test ./internal/http/router -run TestCashierEasyPayAPIModeUsesMAPIAndPersistsDisplay` 通过。
  - `go test ./internal/http/router -run 'TestCashierEasyPay(APIModeUsesMAPIAndPersistsDisplay|PopupDisplayIsSignedAndPersisted)'` 通过。
  - `go test ./internal/http/router -run TestCashierEasyPayWebhook` 通过。
  - `go test ./internal/http/router -run TestAdminCashierOrderCompleteManuallyCreditsRechargeBalance` 通过。
  - `go test ./internal/http/router -run TestAdminCashierOrderRefundDeductsUnusedRechargeBalance` 通过。
  - `go test ./internal/http/router -run 'TestAdminCashierOrder(CompleteManuallyCreditsRechargeBalance|RefundDeductsUnusedRechargeBalance)'` 通过。
  - `go test ./internal/http/router -run 'TestCashierJeePay' -count=1` 通过。
  - `go test ./internal/http/router -run 'TestCashier(JeePay|WebhookCompletesRechargeOrderIdempotently|EasyPayWebhook|AlipayWebhook|WxPayWebhook)' -count=1` 通过。
  - `go test ./internal/http/router -run TestCashierCreateOrderReusesIdempotencyKey -count=1` 通过。
  - `go test ./internal/http/router -run TestCashierCancelPendingOrderUsesCanceledStatusAndBlocksMockPay -count=1` 通过。
  - `go test ./internal/http/router -run TestDocsEndpointsReturnStructuredContract -count=1` 通过。
  - `go test ./internal/http/handlers -run TestPromptExcerptNeverReturnsFullPrompt -count=1` 通过。
  - `go test ./internal/http/router -run TestGalleryPublishReviewAndPublicListFlow -count=1` 通过。
  - `go test ./internal/http/router -run TestCashier` 通过。
  - `go test ./internal/http/router -run 'TestCashier|TestAdminCashier'` 通过。
  - `go test ./internal/repository/entstore -run TestBillingStoreCreateOrderReusesIdempotencyKey -count=1` 通过。
  - `go test ./internal/repository/entstore -run 'TestBillingStore(CreateOrderReusesIdempotencyKey|CompleteRechargeOrderCompletesAndIsIdempotent|RefundPaymentOrderDeductsRechargeGrantAndIsIdempotent|PersistsPaymentOrderChannelFields)' -count=1` 通过。
  - `go test ./internal/repository/entstore -run 'TestBillingStore(CompleteRechargeOrderCompletesAndIsIdempotent|RefundPaymentOrderDeductsRechargeGrantAndIsIdempotent)'` 通过。
  - `go test ./internal/repository/entstore -run 'TestBillingStoreMarkOrderPaidCompletesCashierRechargeOrderIdempotently|TestBillingStorePersistsPaymentOrderChannelFields'` 通过。
  - `npm --prefix web/admin run typecheck` 通过。
  - `npm --prefix web/admin run build` 通过。
  - `npm --prefix web/user run typecheck` 通过。
  - `npm --prefix web/user run build` 通过。
  - 用户收银台支付展示 contract 覆盖二维码、跳转、JSAPI 不支持和空状态；随 `npm --prefix web/user run typecheck` 执行通过。
  - 用户收银台支付展示模型已抽到 `web/user/src/pages/checkoutPaymentDisplay.ts`，JSAPI 不支持态不再展示“后续版本”路线图文案，而是给出“改选 H5 或扫码支付后重新创建订单”的当前可执行指引；缺少或无法展示二维码/支付链接时也改为“刷新订单，仍无法展示则改选其他渠道后重新创建订单”，contract `web/user/src/pages/checkoutPaymentDisplay.contract.ts` 锁定支付链路文案不得出现 `后续/版本/暂未/即将` 等弱提示。
  - 收银台支付展示契约已对齐 OpenAPI、共享类型和用户端模型：`PaymentDisplay.type` 明确覆盖 `qr_code`、`redirect`、`form_html`、兼容旧 `form`、`jsapi`、`mock`、`none`；`form_html` 保留渠道返回表单 HTML 由页面新窗口承载，Mock 渠道只展示“模拟支付成功”页面内动作，不再暴露 `mock://` 跳转链接。
  - Mock 渠道订单后端契约已同步清理：`cashierPaymentDisplay`、Memory store、Ent store 均不再为 Mock 自动生成 `payment_url=mock://checkout/{order_no}`，新收银台与旧 billing 兼容入口的 Mock 订单只保留 `payment_display.type=mock`，Docker E2E 也改为验收“无旧 payment_url + 通过 mock-pay 履约”。
  - 用户收银台订单状态模型已抽到 `web/user/src/pages/checkoutOrderState.ts`，订单轮询、订单动作和最近订单 contract 不再 import 整页 `CheckoutPage`，可独立运行；Mock 支付动作兼容 `provider_type`、`visible_method`、`provider` 任一字段为 `mock` 的订单，避免旧 billing 兼容订单或测试订单无法继续模拟支付。
  - 用户收银台订单轮询 contract 覆盖 pending 未过期继续轮询、过期停止轮询、completed 成功终态、failed 失败终态；随 `npm --prefix web/user run typecheck` 执行通过。
  - 用户收银台订单动作 contract 覆盖未过期 pending 订单可取消且可 Mock 支付，过期/completed/canceled 订单不可取消也不可 Mock 支付；随 `npm --prefix web/user run typecheck` 执行通过。
  - 用户收银台最近订单 contract 覆盖最多 10 条、新订单优先排序、自定义金额充值展示；随 `npm --prefix web/user run typecheck` 执行通过。
  - 用户收银台最近订单展示模型新增订单状态与支付方式中文化：`pending/paid/completed/canceled/closed/failed/expired/refunded/partially_refunded` 不再直接露出原始枚举，`mock/alipay/wxpay/easypay_*/jeepay_*` 显示为用户可理解的支付方式；未知枚举仍保留原值便于排障，原始订单对象继续保存在 row 上供点击接管订单使用。
  - 用户收银台配置缺失空态已抽到 `web/user/src/pages/checkoutEmptyState.ts`，contract `web/user/src/pages/checkoutEmptyState.contract.ts` 可独立运行，锁定页面不得再显示“收银台暂不可用 / 后续版本 / 管理员 / Mock”等弱提示或内部术语；当配置读取失败、后端未返回可购买固定积分包或未返回可选支付方式时，页面分别展示“充值配置待完善 / 支付方式待开启”，并给出“刷新配置 / 查看余额”两个当前可执行动作，引导用户刷新、稍后再试或先查看余额。
  - 用户收银台支付方式按钮新增 `checkoutPaymentMethodOptionModel`：按钮提交值继续保留 `visible_method` 原始值，但可见说明改为“支付宝渠道 / 微信支付渠道 / 测试环境模拟支付”等用户可理解文案，不再把 `alipay/wxpay/mock` 作为辅助说明直出。
  - 用户收银台可购买套餐模型已抽到 `web/user/src/pages/checkoutPlans.ts`，contract `web/user/src/pages/checkoutPlans.contract.ts` 锁定用户端只展示 `points_package + purchase_enabled + 非 disabled/archived` 的固定积分包，即使后端或缓存误返回 active subscription，也不会在收银台开放购买；随 `npm --prefix web/user run typecheck` 执行通过。
  - 用户登录页入口文案已抽到 `web/user/src/pages/loginCopy.ts`，contract `web/user/src/pages/loginCopy.contract.ts` 可独立运行，锁定 MVP 默认中文、账号密码/验证码/重置密码失败标题中文化、第三方登录预留提示必须引导用户使用邮箱验证码或账号密码登录；`LoginPage.tsx` 不再因为浏览器英文环境展示 `Password sign-in failed` 等半英文错误，也不再用“暂不可用/后续版本”弱提示解释第三方入口。
  - 用户端首页账户 readiness 展示已继续收敛到 `web/user/src/pages/homeGalleryModel.ts`：新增 `homeAccountReadinessView` 统一可用积分、体验额度、体验额度过期提示和二级行动按钮；`HomePage.tsx` 不再在页面内自行 `toLocaleDateString` 格式化体验额度过期时间，contract `web/user/src/pages/homeReadinessModel.contract.ts` 锁定首页体验额度日期为稳定 `YYYY/MM/DD`、即将过期提示、无体验额度引导和非法日期排障兜底。
  - 用户端顶栏状态已抽到 `web/user/src/topbarStatus.ts`，contract `web/user/src/topbarStatus.contract.ts` 可独立运行，锁定顶栏不再展示静态假数据“消息 3 / 活动 2”，而是基于真实 `Balance` 展示体验额度、充值余额、临期提醒或账户读取中状态；体验额度过期时间统一为稳定 `YYYY/MM/DD`，非法日期保留原值用于排障；`Shell` 继续保留总可用积分快捷充值入口。
  - 用户端头像菜单已抽到 `web/user/src/avatarMenu.ts`，contract `web/user/src/avatarMenu.contract.ts` 可独立运行，锁定个人中心、积分充值、API 密钥、开发文档均跳转真实路由，不再通过“该功能暂不可用”形成死入口；菜单项保留 `permission` 锚点，后续可接入完整自定义权限系统进行过滤。
  - 个人中心余额/流水展示模型已抽到 `web/user/src/pages/profileBalanceModel.ts`，contract `web/user/src/pages/profileBalanceBuckets.contract.ts` 与 `web/user/src/pages/profileLedgerRows.contract.ts` 可独立运行，锁定体验额度/订阅额度/充值额度三桶展示、服务端分桶兼容、`balance_bucket` 标签、`source_type` 标签、`expires_at` 格式化、充值额度长期有效、金额方向和流水发生时间 `YYYY/MM/DD HH:mm` 展示，避免用户侧流水继续直出 ISO 技术时间。
  - 工作台生成前置状态模型已抽到 `web/user/src/pages/workspaceGenerateReadiness.ts`，`WorkspacePage.tsx` 只消费纯模型；contract `web/user/src/pages/workspaceGenerateReadiness.contract.ts` 可独立运行，覆盖无模型用户文案、内部术语屏蔽、余额不足充值引导、提示词长度和正常可生成状态。
  - 工作台任务卡片展示继续收敛到 `workspaceTaskFailure.ts` 侧的纯模型：`workspaceTaskCardView` 统一输出任务类型、`YYYY/MM/DD HH:mm` 创建时间、状态中文文案和 tone，`workspaceTaskPendingView` 统一输出排队/生成/等待结果文案；`WorkspacePage.tsx` 不再直接 `slice` ISO 时间、调用共享 `StatusPill` 或用页面三元表达式拼等待态。
  - 工作台生成结果图片操作提示已抽到 `web/user/src/pages/workspaceImageActions.ts`，contract `web/user/src/pages/workspaceImageActions.contract.ts` 可独立运行，锁定“标记 / 更多”这类未完全产品化入口不得再提示“暂不可用 / 后续版本”，而是引导用户使用当前可执行的下载、继续编辑、提交公开审核和前往图库管理路径。
  - 工作台生成结果图片操作提示进一步收敛：`标记` 提示标题改为“前往图库管理”，`更多` 提示标题改为“使用当前可用操作”，contract 禁止回退到“整理中 / 暂不可用 / 后续版本”这类半成品文案，确保按钮反馈直接指向可执行路径。
  - 首页精选公开作品模型已抽到 `web/user/src/pages/homeGalleryModel.ts`，contract `web/user/src/pages/homeGalleryCard.contract.ts` 与 `web/user/src/pages/homeGalleryAccess.contract.ts` 可独立运行，锁定匿名首页卡片只展示 `prompt_excerpt`、游客点击详情必须登录、登录用户才请求详情。
  - 首页精选公开作品卡片元信息继续收敛：`homeGalleryCardView` 将创建时间格式化为 `YYYY/MM/DD HH:mm`，不再把 `2026-06-05T00:00` 这类 ISO 片段展示给新用户；非法时间保留原值用于排障。
  - 首页模型可用性展示已抽到 `homeModelReadinessView`，contract `web/user/src/pages/homeReadinessModel.contract.ts` 可独立运行，锁定 loading/ready/unavailable 三态文案；当没有可用模型时页面展示“平台模型配置中 / 当前没有可用生图模型，请稍后再试。”并进入 warning 状态，不再只给用户“暂不可生成”式弱提示。
  - 公开广场列表模型已抽到 `web/user/src/pages/publicGalleryModel.ts`，contract `web/user/src/pages/publicGalleryCard.contract.ts` 可独立运行，锁定列表卡片和搜索索引只使用 `prompt_excerpt`、模型和作者，不通过列表对象泄露完整 `prompt`。
  - 公开广场列表卡片元信息继续收敛：`publicGalleryCardView` 输出本地化任务类型和 `YYYY/MM/DD` 创建日期，`PublicGalleryPage.tsx` 不再自行拼 `task_type` 标签或展示 `2026-06-05` 这类技术日期格式；contract 锁定日期不得出现 `T/Z`。
  - 工作台任务失败卡片已抽出 `workspaceTaskFailure` 展示模型，失败终态固定展示用户友好原因、`error_code` 和追踪 ID（优先 `request_id`，无则回退 `task_id`），同时避免把 route/provider 等内部细节直接暴露给用户；`toTask` 已透传 `error_code`、`error_message` 与 `request_id`，contract `web/user/src/pages/workspaceTaskFailure.contract.ts` 锁定上述展示边界。
  - `workspaceTaskFailure.contract.ts` 已扩展覆盖任务卡片元信息和等待态：`partial_failed` 展示“部分完成”、非法时间保留原值、未知等待态回落“等待结果”；`npm exec tsx src/pages/workspaceTaskFailure.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐工作台任务卡片展示模型后通过。
  - 后台上线检查行模型已抽到 `web/admin/src/pages/readinessRows.ts`，contract `web/admin/src/pages/readinessRows.contract.ts` 可独立运行，覆盖 fail 检查项可点击跳转、阻塞状态文案、状态 tone 和无修复入口时回落到 `#/readiness`。
  - 后台用户管理页状态展示模型已抽到 `web/admin/src/pages/userRows.ts`，contract `web/admin/src/pages/userRows.contract.ts` 可独立运行，覆盖 `active/pending/disabled/closed` 到“正常/待验证/禁用/已关闭”的中文文案、badge tone、筛选/弹窗状态选项和列表统计口径；`UsersPage.tsx` 列表、筛选、弹窗和反馈文案复用同一模型，页面仍向后端提交原始状态值，启用/禁用操作 eligibility 逻辑不变。
  - 后台用户列表行展示继续收敛到 `adminUserRowView`：统一输出展示名兜底、邮箱/ID 副标题、状态文案/tone、分组展示、余额、最后活跃时间和创建时间；`UsersPage.tsx` 列表不再自行 `new Date(...).toLocaleString` 格式化两列时间，也不再保留重复的 `UserStatusBadge` 小组件。contract 锁定列表时间为 `YYYY/MM/DD HH:mm`、缺失时间显示 `-`、非法时间保留原值用于排障。
  - 后台用户详情流水模型已抽到 `web/admin/src/pages/userDetailLedgerRows.ts`，contract `web/admin/src/pages/userDetailLedgerRows.contract.ts` 可独立运行，覆盖 `balance_bucket` / `bucket_type` 兼容、来源标签、有效期格式化、充值长期有效、负向金额 tone 和流水发生时间 `YYYY/MM/DD HH:mm` 展示，避免运营详情弹窗继续受浏览器本地时区/语言环境影响或直出非补零时间。
  - 后台用户详情子资源展示模型已抽到 `web/admin/src/pages/userDetailResourceRows.ts`，contract `web/admin/src/pages/userDetailResourceRows.contract.ts` 可独立运行，覆盖余额桶、订单、任务、API Key 状态中文文案、badge tone、任务类型中文文案、稳定 `YYYY/MM/DD HH:mm` 时间展示和未知枚举回退；`UsersPage.tsx` 详情弹窗的余额桶过期时间、订单创建时间、API Key 最近使用时间、任务短 ID/模型/积分均复用该模型，避免详情弹窗继续直接暴露 `completed/queued/active/text_to_image` 或 ISO 时间等原始值。
  - 后台任务类型展示模型已抽到 `web/admin/src/pages/adminTaskTypes.ts`，contract `web/admin/src/pages/adminTaskTypes.contract.ts` 可独立运行，统一锁定 `text_to_image/reference_to_image/image_edit` 等原始接口值到“文生图/参考生图/图片编辑”的运营可读文案；价格配置、模型接入、审核队列、调用记录和用户详情均复用该模型，接口 payload 继续保留原始枚举。
  - 后台价格配置展示模型已抽到 `web/admin/src/pages/pricingRows.ts`，contract `web/admin/src/pages/pricingRows.contract.ts` 可独立运行，覆盖价格启停状态、质量档位、路由模型展示名和价格页统计口径；`PricingPage.tsx` 继续向后端提交 `text_to_image/reference_to_image/image_edit`、`1K/2K/4K`、`enabled/disabled` 等原始值，但页面展示为“文生图/参考生图/图片编辑”“1K 标准/2K 高清/4K 超清”“启用/停用”，且“缺价格路由”只统计启用路由模型，避免停用路由误报上线风险。
  - 后台价格配置字段说明已集中到 `pricingFieldHints`：弹窗说明明确价格项会参与计费预估和生成扣费、这里配置的是用户积分而不是 Provider 成本；“基础积分”说明为单张图片基础积分且支持 5 位小数；“参考图倍率”说明带参考图时会在基础积分上放大，无参考图按 `1.00000` 计算。
  - `PricingPage.tsx` 已将 `pricingFieldHints.dialogDetail/basePoints/referenceMultiplier` 接入价格配置弹窗，避免管理员误把基础积分理解为人民币价格或供应商成本字段；contract 禁止说明文案回退到 `provider_cost/base_points/reference_multiplier/raw` 等内部字段名。
  - `npm exec tsx src/pages/pricingRows.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐后台价格配置字段说明后通过。
  - 后台路由模型页展示模型已抽到 `web/admin/src/pages/routingRows.ts`，contract `web/admin/src/pages/routingRows.contract.ts` 可独立运行，覆盖 `public/groups/hidden` 到“全员可见/按分组可见/隐藏”的中文展示、路由与候选启停 badge、候选总数与启用数摘要、可见分组名称解析和未知分组 ID 回退；`RoutingPage.tsx` 弹窗下拉和列表继续提交 `public/groups/hidden`、`enabled/disabled` 原始接口值，但页面不再要求运营记忆英文可见性枚举。
  - 后台路由候选字段标签和说明已集中到 `routingFieldLabels` / `routingFieldHints`：候选列表与弹窗展示“路由代码 / 优先级 / 权重 / 兜底顺序”等运营可读文案，提示语不再暴露 `Code / Priority / Weight / Fallback` 等内部字段名；提交 payload 仍保留 `code/priority/weight/fallback_order` 原始契约，便于后端兼容和排障。
  - 后台模型接入页展示模型已抽到 `web/admin/src/pages/providerModelRows.ts`，contract `web/admin/src/pages/providerModelRows.contract.ts` 可独立运行，覆盖接入方式、鉴权方式、账号状态、密钥配置状态、真实模型启停和能力摘要；`ProviderModelsPage.tsx` 列表/弹窗继续提交 `adapter_type/auth_type/status/task_types` 原始接口值，但页面可见文案统一为“OpenAI 兼容 / API Key / 启用 / 异常 / 文生图”等运营可读口径。
  - 后台模型接入账号弹窗提示已收敛到 `providerAccountDialogDetail()`：明确管理员填写账号名称、Base URL 和 API Key 即可启用模型账号，编辑账号时 API Key 留空保持原密钥；contract 禁止弹窗继续出现“后续/暂未/即将/版本/预留”等路线图弱文案。
  - 后台健康状态展示模型已抽到 `web/admin/src/healthRows.ts`，contract `web/admin/src/healthRows.contract.ts` 可独立运行，覆盖 `healthy/degraded/down/pass` 到“健康/降级/不可用/通过”的中文文案、badge tone、顶栏 Provider 状态值、任务队列兜底和巡检策略文案；`HealthPage.tsx` 与全局 `AdminLayout` 复用同一模型，避免系统健康页继续直接展示 `healthy/pass/30s interval` 等内部值。
  - 后台系统健康页已移除硬编码的 `API Gateway / Postgres / Redis Queue / Object Storage` 伪探针，页面改为并发读取 `GET /api/ops/admin/v1/metrics/dashboard` 的 Provider 探针和 `GET /api/ops/admin/v1/readiness` 的真实运行检查；刷新时间使用后端 `generated_at` 稳定格式化，运行检查行展示阻塞上线/修复入口，避免管理员被静态健康数据误导。
  - 后台收银台 provider options 模型已抽到 `web/admin/src/pages/cashierProviderOptions.ts`，contract `web/admin/src/pages/cashierProviderOptions.contract.ts` 可独立运行，覆盖 `jeepay_alipay` / `jeepay_wxpay` 可选，并通过类型级断言禁止 JeePay 标签包含“占位”或 placeholder。
  - 后台收银台支付渠道实例弹窗新增渠道配置引导：`cashierProviderConfigGuide` 覆盖支付宝直连、微信直连、易支付、JeePay、Mock 的必填字段、可选字段、沙箱/回调/模板说明和密钥不回显提示；`CashierPage.tsx` 在实例弹窗内展示配置清单，降低运营只面对 JSON 文本框时的配置门槛，contract 禁止引导文案回退到占位/路线图弱提示；同时修正支付渠道实例表格为 6 列布局，避免“操作”列与表头错位。
  - 后台收银台套餐开放规则已抽到 `web/admin/src/pages/cashierPlanPurchase.ts`，contract `web/admin/src/pages/cashierPlanPurchase.contract.ts` 锁定 `subscription` 订阅占位即使被误勾“允许用户购买”，保存 payload 也会强制 `purchase_enabled=false`，列表购买状态也显示 `hidden`；`points_package` 积分包仍保留可购买、排序和有效天数等字段。
  - 后台收银台固定积分包空态文案已改为当前可执行指引：提示点击“新增套餐”创建 `points_package` 固定积分包并启用购买，不再显示“后续在这里创建”一类弱路线图文案；contract 同步锁定空态标题、操作指引和禁止 `后续/暂未/即将/版本` 回退。
  - 后台收银台固定积分包空态进一步产品化：空态可见文案改为“创建固定积分包”，不再把 `points_package` 接口枚举直出给运营；contract `web/admin/src/pages/cashierPlanPurchase.contract.ts` 增加禁止空态展示 `points_package/subscription` 的约束。
  - 后台收银台 webhook event action 模型已抽到 `web/admin/src/pages/cashierWebhookEventActions.ts`，contract `web/admin/src/pages/cashierWebhookEventActions.contract.ts` 可独立运行，覆盖失败事件可重试、已处理事件不可重试、中间态等待处理。
  - 后台收银台订单/回调支付展示已抽到 `web/admin/src/pages/cashierPaymentDisplay.ts`，contract `web/admin/src/pages/cashierPaymentDisplay.contract.ts` 可独立运行，锁定订单列表“方式”、订单详情“支付渠道”、订单详情创建/支付/退款/关闭时间、支付渠道实例“支持方式 / 配置状态”和回调事件“渠道”不再直接展示 `mock/alipay_direct/wxpay_direct/jeepay_*`、`alipay/wxpay/mock`、`configured/missing/invalid` 或本地化漂移时间等接口枚举/技术格式，而是展示“Mock 测试 / 支付宝 / 微信支付 / JeePay 微信 / 配置已完成 / 缺少配置 / 配置异常”和稳定 `YYYY/MM/DD HH:mm` 等运营可读文案；订单列表购买类型也从 `plan/custom_amount/subscription` 归一为“积分包购买 / 自定义金额充值 / 订阅套餐”，回调事件类型从 `payment.retryable_failed/refund.local_finalize_failed` 归一为“支付回调失败 / 退款本地落账失败”；未知渠道、未知类型和非法时间仍保留原值用于排障。
  - 后台调用记录展示模型已抽到 `web/admin/src/pages/callRecordRows.ts`，contract `web/admin/src/pages/callRecordRows.contract.ts` 可独立运行，覆盖前置失败错误码展示、价格配置排障提示、API Key 展示、预估/实扣积分、成功记录无错误和 Provider 成本/毛利展示。
  - 后台调用记录筛选区已补齐运营友好 copy：页面仍固定对接 `status/error_code/source_channel/provider/user_id/task_id` 查询参数，但可见筛选文案改为“错误码 / 入口 / Provider / 用户 ID / 任务 ID”，不再把 `error_code`、`source_channel`、`user_id`、`task_id` 作为输入框占位符直出；列表中的入口展示也从 `open_api` 归一为 `Open API` 等可读标签。
  - 后台调用记录状态文案已本地化：筛选下拉仍提交 `queued/running/succeeded/failed/canceled` 等后端查询值，但可见选项展示为“排队中 / 执行中 / 成功 / 失败 / 已取消”；表格行保留 `status` 原始值用于调试与后续契约扩展，同时新增 `statusLabel` 统一展示中文状态，contract 锁定 raw value 与 visible label 不漂移。
  - 后台调用记录入口筛选已从自由输入升级为结构化下拉：可见选项为“全部入口 / 用户 Web / Open API / OpenAI 兼容”，但提交给 `GET /api/ops/admin/v1/call-records` 的 `source_channel` 仍保持 `web/open_api/openai_compatible` 原始查询值；contract 锁定入口选项 raw value 与运营可读 label 不漂移，页面不再要求运营记忆内部枚举。
  - 后台调用记录时间线展示继续收敛：`callRecordRows` 的 `createdAt` 与 `lifecycleLabel` 改为稳定 `YYYY/MM/DD HH:mm` 格式，不再受浏览器时区/locale 影响显示成 `06/05 08:00` 等漂移结果；缺失时间显示 `-`，非法时间保留原值用于排障，contract 锁定创建/开始/结束时间不得出现 `T/Z`。
  - 后台审核队列行展示已收敛到 `web/admin/src/pages/reviewRows.ts` 的 `reviewRowView`：列表行统一输出图片 ID、标题、用户、审核上下文、任务类型、`YYYY/MM/DD HH:mm` 创建时间、状态文案/tone、可执行动作和终态文案；`ReviewPage.tsx` 不再直接展示 `created_at` 或自行重复计算任务类型/状态动作。contract 锁定待审核只允许“通过/驳回”、已通过只允许“下架”、已驳回/已下架只展示终态文案，非法时间保留原值用于排障。
  - 后台兑换码页展示模型已抽到 `web/admin/src/pages/redeemRows.ts`，contract `web/admin/src/pages/redeemRows.contract.ts` 可独立运行，覆盖 `inactive/available/redeemed/expired/disabled` 到“未生效/可用/已核销/已过期/已停用”的中文文案、badge tone、积分奖励文案、核销次数、有效期 `YYYY/MM/DD HH:mm` 稳定格式化、非法有效期排障兜底和合法状态快捷操作；`RedeemPage.tsx` 继续向后台提交 `available/disabled` 原始状态值，弹窗选项与反馈文案改为运营可读标签，已核销/已过期兑换码不再展示可疑的重新启用入口。
  - 后台兑换码页补齐批量生成与本批次导出闭环：`RedeemPage.tsx` 顶部新增“批量生成”入口，弹窗要求填写生成数量、奖励积分、有效天数和每码可核销次数，保存时复用既有 `POST /api/ops/admin/v1/redeem-codes:batch-create` 后端接口；前端 payload 固定传 `batch_id=0` 让后端生成唯一批次号，成功后立即下载本批次 CSV，文件名为 `redeem-codes-{batch_id}.csv`，并刷新列表。`redeemRows.contract.ts` 锁定批量参数校验、payload 契约、CSV 表头、中文状态/奖励、逗号/引号转义、非法时间保留和空导出表头。
  - 后台兑换码“导出”已升级为可审计后端动作：新增 `POST /api/ops/admin/v1/redeem-codes:export`，请求支持 `status/code/batch_id` 筛选，响应返回导出的 `items/count/filters`；后端复用兑换码列表过滤并写审计 `redeem.export`，metadata 包含导出数量和筛选条件。`RedeemPage.tsx` 顶部“导出兑换码”和批量生成后的自动下载都会先调用该接口，再用返回 items 生成 CSV，避免纯前端下载绕过审计；OpenAPI、`web/shared/api-types.ts`、`web/shared/admin-api.ts` 和 `redeemRows.contract.ts` 已同步锁定导出路径与响应契约。
  - 后台兑换码页补齐“查看核销记录”入口：每行兑换码新增“核销记录”按钮，弹窗复用既有 `GET /api/ops/admin/v1/redeem-codes/{code_id}/redemptions` 接口展示最近 20 条核销流水，包含用户、流水类型、到账积分、余额、来源和发生时间；无核销时展示空态，接口失败时可在弹窗内重试。`redeemRedemptionRows` 将 `LedgerEntry` 转成运营可读模型，contract 锁定 `redeem` 类型本地化、用户 ID、正负金额 tone、来源、详情、稳定时间格式，以及未知类型和非法时间保留用于排障。
  - 后台用户分组页展示模型已抽到 `web/admin/src/pages/userGroupRows.ts`，contract `web/admin/src/pages/userGroupRows.contract.ts` 可独立运行，覆盖 `enabled/active/disabled` 状态到“启用/停用”的运营文案、badge tone、默认分组标签、启用统计、默认分组名称和最高倍率；`UserGroupsPage.tsx` 列表不再直出 `enabled/active/disabled`，但表单仍向后端提交 `enabled/disabled` 原始状态值，并兼容历史 `active` 状态。
  - 后台收银台状态展示模型已抽到 `web/admin/src/pages/cashierStatusRows.ts`，contract `web/admin/src/pages/cashierStatusRows.contract.ts` 可独立运行，覆盖计划状态、订单状态、回调状态、启停状态、Mock 可见性、套餐类型和查单反馈的中文展示；`CashierPage.tsx` 列表、详情、弹窗选项和操作反馈复用该模型，页面仍向后端提交 `active/disabled/archived/pending/completed` 等原始接口值，订单操作 eligibility 逻辑不变。
  - 后台审计日志展示模型已抽到 `web/admin/src/pages/auditRows.ts`，contract `web/admin/src/pages/auditRows.contract.ts` 可独立运行，覆盖审计 action、result、操作人、对象、详情摘要和搜索占位文案的运营可读展示；`AuditPage.tsx` 筛选值和搜索索引继续保留 `admin.login/user.points_adjust/success/failure` 等原始接口值，未知 action/result 原样展示用于排障，页面不再直出 `actor=` / `target=` 一类调试式详情。
  - 后台审计时间线行展示继续收敛到 `auditTimelineRow`：行模型统一输出动作标签、操作人、对象、操作人 tone、`YYYY/MM/DD HH:mm` 创建时间、详情摘要和 result badge，`AuditPage.tsx` 不再直接展示原始 `created_at`；contract 锁定系统操作使用 neutral tone、非法时间保留原值、可见时间不得出现 `T/Z`。
  - 后台审计“导出日志”已从假反馈改为真实下载当前筛选结果 CSV：`auditRowsCSV` 统一输出“时间/动作/操作人/对象/结果/详情/审计ID”运营表头，复用页面本地化 action、subject、result、detail 和稳定时间格式，正确转义逗号、引号与换行；`AuditPage.tsx` 在无匹配行时禁用导出，有匹配行时生成带 BOM 的 `audit-logs-YYYYMMDD-HHmm.csv`，并反馈已下载行数。contract 锁定空导出表头、未知 action 原值保留、CSV 转义和文件名格式。
  - 新增 `scripts/workflow/verify-contracts.sh`，统一执行 `web/admin/src`、`web/user/src`、`web/shared` 下所有 `.contract.ts`；`scripts/workflow/verify.sh` 已接入该脚本，后续仓库统一验证会自动覆盖前端/shared 产品契约，避免 contract 只靠人工巡检。
  - `scripts/test/api_contract_smoke.sh` 已补齐公开广场真实 API smoke 边界：在临时 SQLite 环境内种入一张 `approved` 公开图，游客访问 `GET /api/open/image/v1/gallery/images` 必须返回 `prompt: null` 与脱敏 `prompt_excerpt`，响应体不得包含完整 prompt；游客访问 `GET /api/open/image/v1/gallery/images/{image_id}` 必须返回 401；登录用户访问同一详情接口必须返回完整 prompt 与 `visibility_status=approved`。
  - `scripts/test/api_contract_smoke.sh` 已将公开广场 seed 流程升级为审核闭环：临时 SQLite 先写入 `pending_review` 图片，后台 `GET /api/ops/admin/v1/image-reviews?status=pending_review` 必须能看到待审项；审核前公开列表必须排除该图；后台调用 `POST /api/ops/admin/v1/image-reviews/{image_id}:approve` 后才进入 `approved` 状态，并继续复用游客列表、游客详情登录拦截、登录详情完整 prompt、点赞/收藏互动 smoke。
  - 后台审核队列动作规则已抽到 `web/admin/src/pages/reviewRows.ts`：`pending_review/pending` 只展示“通过/驳回”，`approved/public` 只展示“下架”，`rejected/unpublished` 只展示终态文案，不再提供无效写操作按钮；`ReviewPage.tsx` 只消费该模型，contract `web/admin/src/pages/reviewRows.contract.ts` 锁定状态标签、状态 tone、默认原因和合法动作矩阵。
  - `scripts/test/api_contract_smoke.sh` 已补齐公开广场点赞/收藏真实 API smoke 边界：登录用户对同一张 `approved` 公开图调用 `POST /api/agent/gallery/v1/images/{image_id}/like` 与 `/favorite` 后，响应必须分别返回 `like_count=1` / `favorite_count=1`，并携带 `liked_by_viewer=true` / `favorited_by_viewer=true`；随后通过 `GET /api/open/image/v1/gallery/images?liked=true&access_token=...` 与 `favorited=true&access_token=...` 验证登录态筛选结果包含该图且 viewer 状态正确。
  - `scripts/test/api_contract_smoke.sh` 已补齐用户收银台真实 API smoke 边界：后台先通过 `PUT /api/ops/admin/v1/cashier/custom-amount-config` 配置自定义金额规则，用户创建 `custom_amount` 订单后按配置将 `10.00000 CNY` 换算为 `20.00000` 积分并通过 Mock 支付到账；另创建一笔待支付套餐订单，调用 `POST /api/agent/cashier/v1/orders/{order_id}/cancel` 后必须返回 `canceled`、写入 `closed_at`、不产生 `ledger_id`，且再次调用 `mock-pay` 必须返回 409 `CONFLICT`，订单详情保持未到账。
  - `go test ./api/openapi -count=1` 在补齐后台调用记录与回调重试 OpenAPI 契约后通过。
  - `./scripts/workflow/verify.sh` 在补齐后台调用记录与回调重试 OpenAPI 契约后通过。
  - `./scripts/workflow/api-smoke.sh` 在补齐后台调用记录与回调重试 OpenAPI 契约后通过。
  - `./scripts/workflow/review-local.sh --scope working` 在补齐后台调用记录与回调重试 OpenAPI 契约后仍为流程性 `BLOCK`，唯一原因是 heavyweight 任务的 `.coding-context.json` 中 `approval.status=pending`，需审批通过后才能进入 push/PR ready。
  - `go test ./internal/repository/entstore -run TestAdminCallRecordStoreListsCreateTaskRoutePreflightFailures -count=1` 在补齐超过 32 字符 route code 回归后通过。
  - `./scripts/workflow/api-smoke.sh` 在新增后台调用记录前置失败验收、并修复 `abstract_model` 长度边界后通过。
  - `./scripts/workflow/verify.sh` 在新增后台调用记录 smoke 验收、并修复 `abstract_model` 长度边界后通过。
  - `./scripts/workflow/review-local.sh --scope working` 仍为流程性 `BLOCK`，唯一原因是 heavyweight 任务的 `.coding-context.json` 中 `approval.status=pending`。
  - Docker E2E 当前仍受本机 Docker daemon 未运行阻塞：`Cannot connect to the Docker daemon at unix:///Users/fatballfish/.docker/run/docker.sock`。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsTrialBalanceBucketAndLedgerContracts -count=1` 通过。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsCashierAndReadinessContracts -count=1` 通过。
  - `go test ./api/openapi -count=1` 通过。
  - `go test ./internal/http/router -run TestDocsEndpointsReturnStructuredContract -count=1` 通过。
  - `go test ./api/openapi ./internal/domain/adminauth ./internal/http/handlers -run 'TestOpenAPISpecDocumentsAdminPermissionContract|TestRolePermissionResolver|TestRequireAdminPermissionUsesRoleFacade|TestAdminPermissionResolverCanBeReplacedForFutureRBAC' -count=1` 通过。
  - `go test ./internal/domain/adminauth ./internal/http/handlers -run 'TestRolePermissionResolver|TestRequireAdminPermissionUsesRoleFacade|TestAdminPermissionResolverCanBeReplacedForFutureRBAC'` 通过。
  - `go test ./internal/repository/entstore -run TestBillingStorePersistsPaymentOrderChannelFields` 通过。
  - `./scripts/workflow/verify.sh` 通过。
  - `./scripts/workflow/api-smoke.sh` 通过。
  - `./scripts/workflow/review-local.sh --scope working` 当前被流程阻塞，`.review/gate.json` 为 `BLOCK`；阻塞原因是 `.coding-context.json` 的 heavyweight `approval.status=pending`，需审批通过后才能进入 push/PR ready 状态。
  - `npm --prefix web/user run typecheck` 在补齐公开广场/首页列表提示词保护契约后通过。
  - `npm --prefix web/user run build` 在补齐公开广场/首页列表提示词保护契约后通过。
  - `./scripts/workflow/verify.sh` 在补齐公开广场/首页列表提示词保护契约后通过。
  - `./scripts/workflow/api-smoke.sh` 在补齐公开广场/首页列表提示词保护契约后通过。
  - `./scripts/workflow/review-local.sh --scope working` 在补齐公开广场/首页列表提示词保护契约后仍为流程性 `BLOCK`，唯一原因是 heavyweight 任务的 `.coding-context.json` 中 `approval.status=pending`，需审批通过后才能进入 push/PR ready。
  - `go test ./api/openapi -run TestOpenAPISpecDocumentsPublicGalleryPromptBoundaryContract -count=1` 通过。
  - `go test ./internal/http/router -run TestGalleryPublishReviewAndPublicListFlow -count=1` 在补齐公开列表筛选和列表 prompt 固定 null 后通过。
  - `go test ./internal/repository/entstore -run TestImageTaskStoreListsApprovedPublicImagesWithoutPublishedAt -count=1` 在补齐 Ent store 公开列表筛选后通过。
  - `go test ./api/openapi -count=1` 在补齐公开广场列表/详情 OpenAPI 边界后通过。
  - `./scripts/workflow/verify.sh` 在补齐公开广场列表/详情 OpenAPI 边界、运行态筛选和列表 DTO 后通过。
  - `./scripts/workflow/api-smoke.sh` 在补齐公开广场列表/详情 OpenAPI 边界、运行态筛选和列表 DTO 后通过。
  - `./scripts/workflow/review-local.sh --scope working` 仍为流程性 `BLOCK`，唯一原因是 heavyweight 任务的 `.coding-context.json` 中 `approval.status=pending`。
  - `./scripts/workflow/api-smoke.sh` 在新增公开广场游客列表/游客详情拦截/登录详情完整 prompt 的真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增公开广场 API smoke 验收后通过。
  - `./scripts/workflow/api-smoke.sh` 在新增公开广场点赞/收藏互动与登录态 liked/favorited 筛选真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增公开广场互动 API smoke 验收后通过。
  - `./scripts/workflow/api-smoke.sh` 在新增公开广场待审队列、审核前不可见、后台审核通过后进入公开列表的真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增公开审核 API smoke 验收后通过。
  - `./scripts/workflow/api-smoke.sh` 在新增收银台自定义金额订单、Mock 到账、取消订单和取消后禁止支付的真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增收银台 API smoke 验收后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐收银台待支付订单上限真实 API smoke 边界：用户先用 `Idempotency-Key` 创建一笔 pending 订单并重放，必须返回同一 `id/order_no`，证明幂等复用发生在 pending 上限校验之前；随后继续创建到默认 3 笔 pending 订单，第 4 笔新订单必须返回 409 `PAYMENT_TOO_MANY_PENDING_ORDERS`。
  - `./scripts/workflow/api-smoke.sh` 在新增收银台 pending 上限与幂等复用真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增收银台 pending 上限与幂等复用真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台用户详情真实 API smoke 边界：用户完成注册送体验额度、Mock 充值、Open API 创建任务和 API Key 创建后，后台 `GET /api/ops/admin/v1/users/{user_id}` 必须返回正确用户、完整余额结构、`trial/recharge` 余额桶、`trial_grant/signup` 与 `recharge/payment_order` 流水、已完成订单、排队任务和 active API Key。
  - 后台用户详情余额缺口已修复：`internal/repository/entstore/admin_user_store.go` 的 `GetUserDetail` 不再只返回 `available_points/frozen_points`，同步透出 `trial_points/subscription_points/gift_points/recharge_points/buckets/active_subscription/next_expiring_grant`，支撑后台用户详情页区分体验额度、订阅额度、赠送额度和充值余额。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台积分调整真实 API smoke 边界：后台 `POST /api/ops/admin/v1/users/{user_id}/points-adjustments` 使用 `Idempotency-Key` 增加 `7.00000` 积分后余额从 `118.00000` 变为 `125.00000`；同 key 同 body 重放必须保持幂等；同 key 不同金额重放必须返回 409 `CONFLICT`；再次查询后台用户详情必须能看到 `admin_adjust/recharge/admin` 流水。
  - `go test ./internal/http/router -run TestAdminUserManagementEndpoints -count=1` 在补齐后台用户详情余额分桶和运营子资源断言后通过。
  - `./scripts/workflow/api-smoke.sh` 在新增后台用户详情聚合、余额桶、订单/任务/API Key、积分调整幂等与冲突的真实 API smoke 后通过。
  - `./scripts/workflow/verify.sh` 在新增后台用户详情运营子资源 smoke 与后端余额透传修复后通过。
  - `./scripts/workflow/review-local.sh --scope working` 在新增后台用户详情运营子资源 smoke 与后端余额透传修复后通过。
  - 已按用户最新确认继续收紧 B 方案权限契约：`PIC_GALLERY_ADMIN_EMAIL/PASSWORD` 引导创建的默认后台账号不再硬编码为 `super_admin`，默认角色改为普通 `admin`；如确需引导超级管理员，可显式设置 `PIC_GALLERY_ADMIN_ROLE=super_admin`。未知角色值会回退到 `admin`，避免误配产生超管。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台权限真实 API smoke 边界：默认 `admin` 登录必须返回 `role=admin`，权限列表包含 `manage:cashier`，但不包含 `manage:admins` / `manage:dangerous_config`；普通 `admin` 更新 `payments` 高危配置必须返回 403 `FORBIDDEN`；临时种入的 `super_admin` 登录后必须包含上述高危权限，并可更新同一 `payments` 配置。
  - 新增 `TestAdminHandlersUsePermissionFacadeContract` 静态契约：除 `HandleAdminLogin` 外，所有 `HandleAdmin*` 后台 handler 禁止直接调用裸 `requireAdmin` / `requireAdminWithQueryToken`，必须通过 `requireAdminPermission` / `requireAdminPermissionWithQueryToken` 收口，防止后续新增后台接口绕过 B 方案 permission facade。
  - `go test ./internal/app ./internal/domain/adminauth ./internal/http/handlers ./internal/http/router -run 'TestDefaultAdminSeedRole|TestRolePermissionResolver|TestRequireAdminPermission|TestAdminPermissionResolver|TestAdminConfigDangerousTabsRequireSuperAdmin|TestAdminLoginReturnsPermissions'` 通过。
  - `./scripts/workflow/api-smoke.sh` 在补齐默认后台账号角色与高危配置权限真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐订阅套餐保留但不开放购买的真实 API smoke 边界：先触发默认积分包 seed，再在临时 SQLite 中插入 `active + plan_type=subscription + purchase_enabled=true` 的未来订阅占位套餐；用户端 `GET /api/agent/cashier/v1/options` 必须仍只返回 `points_package + purchase_enabled=true + active` 的积分包，且不得包含该订阅 `plan_code`；用户尝试用订阅 `plan_code` 创建收银台订单必须返回 404 `NOT_FOUND`。
  - `./scripts/workflow/api-smoke.sh` 在补齐订阅套餐隐藏与拒绝下单真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台支付渠道实例密钥遮蔽真实 API smoke 边界：后台创建带 `app_private_key` 的支付宝实例后，创建响应、列表响应和详情响应都不得包含私钥明文，`config` 不得返回敏感 key，且 `credentials_status.has_secret=true` 并只返回 `sha256:` 指纹；同时创建带 `api_v3_key` / `merchant_private_key` 的微信支付实例，响应同样不得泄露密钥明文或敏感 key。
  - `./scripts/workflow/api-smoke.sh` 在补齐支付渠道实例密钥遮蔽真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台收银台退款真实 API smoke 边界：用户创建自定义金额订单并 Mock 支付后，后台调用 `POST /api/ops/admin/v1/cashier/orders/{order_id}/refund` 使用 `refund_trade_no` 全额退款；响应必须进入 `refunded`，返回 `refunded_amount_cny=10.00000`、`refunded_points=20.00000` 和 `refunded_at`；用户充值余额从 `120.00000` 扣回 `100.00000`，体验额度 `18.00000` 与冻结额度 `2.00000` 不变；同一 `refund_trade_no` 重放必须幂等且不重复扣减；用户账本必须出现 `payment_refund/recharge/payment_order` 负向流水；后台收银台 overview 的今日完成订单数按退款后状态回落为 `1`。
  - `./scripts/workflow/api-smoke.sh` 在补齐后台收银台退款、余额扣回、幂等重放与负向流水真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台收银台部分退款真实 API smoke 边界：用户创建第二笔自定义金额订单并 Mock 支付后，后台传入 `refund_amount_cny=5.00000` 发起部分退款，响应必须进入 `partially_refunded`，累计 `refunded_amount_cny=5.00000`、`refunded_points=10.00000`，且不会写全额退款才有的 `refunded_at`；用户充值余额只从 `140.00000` 扣到 `130.00000`，体验额度与冻结额度保持不变；随后使用新的 `refund_trade_no` 不传退款金额退剩余可退金额，订单进入 `refunded`，累计退款金额/积分变为 `10.00000/20.00000` 并写入 `refunded_at`。
  - `./scripts/workflow/api-smoke.sh` 在补齐部分退款中间态、累计退款金额、余额按比例扣回和最终全额退款真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台收银台订单级人工追扣真实 API smoke 边界：在用户完成充值、后台积分调整、退款链路后，对首笔已完成收银台订单调用 `POST /api/ops/admin/v1/cashier/orders/{order_id}/chargeback`，请求必须携带 `Idempotency-Key`，Body 为 `charge_points=5.00000` 与原因；响应必须返回 `{ order, balance }`，充值余额桶从 `100.00000` 扣到 `95.00000`，体验额度 `18.00000`、冻结额度 `2.00000` 不变，总可用余额按 `trial 18 + gift 7 + recharge 95` 汇总为 `120.00000`；同一幂等键同一请求重放必须不重复扣减；同一幂等键不同追扣金额必须返回 409 `CONFLICT`；缺失幂等键必须返回 400 `BAD_REQUEST`；用户账本必须出现 `admin_adjust/recharge/admin` 负向流水。
  - `./scripts/workflow/api-smoke.sh` 在补齐订单级追扣、分桶余额扣减、幂等重放、冲突保护和缺失幂等键真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台人工补单/人工确认到账真实 API smoke 边界：用户创建一笔 `custom_amount` 待支付订单后，后台调用 `POST /api/ops/admin/v1/cashier/orders/{order_id}/complete`，请求携带 `provider=manual_alipay`、人工 `trade_no` 和原因；响应必须返回同一订单进入 `completed`，写入 `provider`、`trade_no`、`ledger_id` 和 `completed_at`；用户充值余额桶临时从 `120.00000` 增至 `140.00000`，体验额度 `18.00000` 与冻结额度 `2.00000` 不变；用户账本必须出现绑定该订单 ID 的 `recharge/recharge/payment_order` 流水；同一订单重复人工完成必须幂等返回且不重复入账；随后通过后台退款将该人工补单订单回滚为 `refunded`，让后续 smoke 余额基线保持稳定。
  - `./scripts/workflow/api-smoke.sh` 在补齐后台人工补单、订单级充值流水、幂等重放和退款回滚真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台主动查单同步真实 API smoke 边界：后台先创建 `sort_order=0` 的专用 Mock 查单渠道实例，配置 `query_status=paid`、`query_trade_no` 和 `query_amount_cny=10.00000`；用户创建 `custom_amount` 待支付订单时必须绑定该 `provider_instance_id`；后台调用 `POST /api/ops/admin/v1/cashier/orders/{order_id}/sync` 后，响应 `data.order` 必须进入 `completed` 并写入渠道流水、`ledger_id`，`data.sync` 必须返回 `query_status=paid`、`paid=true`、`completed=true`、查单金额与渠道流水；用户充值余额桶临时增至 `140.00000`，账本必须出现绑定该订单 ID 的 `recharge/recharge/payment_order` 流水；同一订单重复 `/sync` 必须仍返回已支付订单，但 `sync.completed=false`，证明本次没有重复履约入账；最后通过后台退款将该查单完成订单回滚为 `refunded`，保持后续 smoke 余额基线稳定。
  - `./scripts/workflow/api-smoke.sh` 在补齐后台主动查单、Mock 查询配置、订单绑定渠道实例、首次查单入账、重复查单不重复履约和退款回滚真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐支付渠道实例 `round_robin` 调度真实 API smoke 边界：在已有 Mock 查单实例基础上，后台继续创建两个同为 `mock`、支持 `mock` 支付方式且 `sort_order=1/2` 的渠道实例；用户连续创建两笔 `custom_amount` 待支付订单时，第一笔必须绑定新实例 A 的 `provider_instance_id`，第二笔必须绑定新实例 B 的 `provider_instance_id`，证明可见支付方式的轮询调度会在同渠道多账号之间按状态游标切换；随后取消两笔订单，避免影响待支付订单上限验收。
  - `bash -n scripts/test/api_contract_smoke.sh`、`./scripts/workflow/api-smoke.sh`、`./scripts/workflow/verify.sh` 在补齐渠道实例 round-robin 真实 API smoke 后通过。
  - 支付渠道实例 `random` 调度已从时间戳取模切换为 `crypto/rand` 选择候选，并保留随机源失败时回退第一个候选的安全兜底；新增 `TestRandomCashierProviderInstanceWithReader` 覆盖空候选、单候选、固定随机源选中候选和随机源失败兜底，避免随机调度只能靠不可重复的概率 smoke 验收。
  - `scripts/test/api_contract_smoke.sh` 已补齐支付渠道实例金额限额过滤真实 API smoke 边界：先将早先用于密钥遮蔽验收的微信支付实例禁用，避免非限额实例干扰；后台新增 `wxpay_direct` 限额实例，配置 `min_amount_cny=5.00000`、`max_amount_cny=500.00000` 和静态 `qr_code`；用户以 `amount_cny=4.00000` 创建 `wxpay` 自定义金额订单必须返回 409 `PAYMENT_PROVIDER_UNAVAILABLE`，证明可用实例会被限额过滤；用户以 `amount_cny=10.00000` 创建订单必须成功进入 `pending`，绑定该限额实例的 `provider_instance_id`，并返回 `payment_display.type=qr_code`；最后取消订单保持后续 pending 上限验收稳定。
  - `bash -n scripts/test/api_contract_smoke.sh`、`./scripts/workflow/api-smoke.sh`、`./scripts/workflow/verify.sh` 在补齐渠道实例金额限额真实 API smoke 后通过。
  - `scripts/test/api_contract_smoke.sh` 已补齐后台收银台 webhook event 列表与重试真实 API smoke 边界：在临时 SQLite 中种入一条绑定已完成订单的普通 `failed` 回调事件，`event_type=payment.retryable_failed` 且 payload 带 `failure_reason`；后台 `GET /api/ops/admin/v1/cashier/webhook-events` 必须能看到该事件、订单号、失败状态和失败原因；后台调用 `POST /api/ops/admin/v1/cashier/webhook-events/{event_id}/retry` 后必须返回同一事件进入 `processed`，写入 `processed_at`，并继续携带订单号；再次列表查询必须显示该事件已处理，锁住运营侧“失败事件可重试、已处理事件不可重复误导”的接口基础。
  - `./scripts/workflow/api-smoke.sh` 在补齐后台回调事件列表、失败原因展示、retry 处理状态和 `processed_at` 真实 API smoke 后通过。
  - 用户端公开互动边界已按 D3 收敛为“点赞 + 收藏”：新增 `web/user/src/publicEngagementModel.ts` 统一公开互动统计与热度分计算，`PublicImageDetail` 不再展示“评论 0”，首页灵感瀑布流热度分不再引用历史兼容字段 `comment_count`。
  - `npm exec tsx src/publicEngagementModel.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐公开互动“不做评论”前端 contract 后通过。
  - 公开广场 API/OpenAPI 契约已继续按 D3 收敛：`GET /api/open/image/v1/gallery/images` 列表 DTO 与登录态详情 DTO 均不再输出 `comment_count`；OpenAPI 新增 `PublicGalleryDetailImage`，公开列表/详情 schema 均不声明评论字段；前端 `ImageResult` / `GalleryImage` shared 类型和 normalizer 不再把 `comment_count` 作为正式产品字段。
  - `go test ./internal/http/router -run TestGalleryPublishReviewAndPublicListFlow -count=1`、`go test ./api/openapi -run TestOpenAPISpecDocumentsPublicGalleryPromptBoundaryContract -count=1`、`npm exec tsx src/publicEngagementModel.contract.ts` 在补齐公开广场 API/OpenAPI 评论字段不外露契约后通过。
  - 公开广场后端推荐排序已继续按 D3 收敛为“点赞 + 收藏”：`MemoryStore` 与 Ent `ImageTaskStore` 的 `sort=hot` 热度分统一为 `like_count * 2 + favorite_count * 3`，不再读取历史 `public_image_stats.comment_count`；即使旧数据库中残留非零 `comment_count`，也不会把无点赞/收藏的图片顶到热门前列。
  - 领域 `GalleryImage` 已移除 `comment_count` JSON 字段，Agent 点赞/收藏响应也不会外露评论字段；旧 `public_image_stats.comment_count` 列仅作为 schema 兼容残留，不再进入公开广场运行态模型。新增 `TestMemoryStorePublicGalleryHotSortIgnoresLegacyCommentCount`、`TestImageTaskStorePublicGalleryHotSortIgnoresLegacyCommentCount`，并扩展 `TestGalleryPublishReviewAndPublicListFlow` 覆盖公开列表、登录详情、点赞响应、收藏响应均不输出 `comment_count`。
  - `go test ./internal/service/imagetask ./internal/repository/entstore ./internal/http/router -run 'TestMemoryStorePublicGalleryHotSortIgnoresLegacyCommentCount|TestImageTaskStorePublicGalleryHotSortIgnoresLegacyCommentCount|TestGalleryPublishReviewAndPublicListFlow' -count=1` 在补齐公开广场后端评论残留收敛后通过。
  - 后台收银台“可见支付方式”展示模型已抽到 `web/admin/src/pages/cashierVisibleMethodRows.ts`，contract `web/admin/src/pages/cashierVisibleMethodRows.contract.ts` 可独立运行，锁定 `alipay/wxpay/mock` 展示为“支付宝入口 / 微信支付入口 / 测试支付入口”，渠道类型与调度策略中文化，同时保留 `rawMethod/rawProviderType` 作为提交和排障契约。
  - `cashierVisibleMethodRow` 已预留 `permission='cashier.visible_methods.write'`，后续完整自定义权限系统可直接基于该锚点过滤可见支付方式配置入口；当前页面保存 payload 仍提交 `method/source_provider_type/scheduler_strategy/display_order/enabled` 原始接口值。
  - `npm exec tsx src/pages/cashierVisibleMethodRows.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 和 `./scripts/workflow/verify.sh` 在补齐后台收银台可见支付方式展示模型后通过。
  - 后台收银台支付渠道实例“支持方式”已从自由输入 `alipay/wxpay/mock` 改为中文复选项：`cashierProviderSupportedMethodOptions` 根据渠道类型只展示适用支付方式，“支付宝 / 微信支付 / Mock 测试”用于运营选择，保存 payload 仍按 `supported_methods: string[]` 提交原始值。
  - `cashierToggleSupportedMethod` 锁定支持方式勾选/取消的原始值拼接契约，未知支持方式仍保留 raw value 供排障；contract `web/admin/src/pages/cashierProviderOptions.contract.ts` 防止支持方式标签回退到 raw code。
  - `npm exec tsx src/pages/cashierProviderOptions.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐支付渠道实例支持方式复选项后通过。
  - 后台收银台支付渠道实例字段说明已集中到 `cashierProviderInstanceFieldHints`：实例排序说明“同一支付方式下排序越小越优先”、调度权重说明多实例随机/轮询调度影响、金额上下限说明订单金额过滤规则、日限额说明为空不限制、渠道 JSON 说明商户号/网关/支付模式/密钥不回显和 JeePay 模板用途。
  - 支付渠道实例弹窗已接入上述 hints，避免运营配置多账号调度、金额限制或密钥 JSON 时只能理解 `sort_order/scheduler_weight/daily_amount_limit/config_text` 等内部字段；contract 禁止说明文案回退到 raw 字段名。
  - `npm exec tsx src/pages/cashierProviderOptions.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐支付渠道实例字段说明后通过。
  - 后台收银台人工确认到账渠道已产品化：`cashierManualCompletionProviderOptions` 固定提供“人工确认 · 支付宝 / 微信支付 / 银行转账”，弹窗从自由输入 `manual_alipay` 改为下拉选择，提交给后端的 provider 值仍保持 `manual_alipay/manual_wxpay/manual_bank` 原始契约。
  - 订单支付方式展示新增人工确认渠道中文化，已完成/退款后的人工补单订单不再在后台列表或详情中直出 `manual_alipay`；未知 provider 仍保留原值用于排障。
  - `npm exec tsx src/pages/cashierPaymentDisplay.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐人工确认到账渠道选项和展示文案后通过。
  - 后台收银台订阅套餐展示 copy 已从“订阅占位”收口为“订阅套餐”：`cashierPlanTypeLabel('subscription')`、套餐类型下拉、固定积分包说明和编辑弹窗说明均使用运营可读文案，同时继续明确“仅后台保留定义，不在用户端开放购买”。
  - `cashierPlanSectionCopy` 已集中固定积分包/订阅套餐说明文案，contract `web/admin/src/pages/cashierPlanPurchase.contract.ts` 禁止后台收银台主流程回退到 `占位/placeholder` 文案；`cashierPlanSavePayload` 和 `cashierPlanPurchaseBadge` 仍锁住 subscription 即使误勾购买也保存/展示为未开放。
  - `npm exec tsx src/pages/cashierStatusRows.contract.ts`、`npm exec tsx src/pages/cashierPlanPurchase.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐订阅套餐后台 copy 后通过。
  - 后台系统设置页展示模型已抽到 `web/admin/src/pages/configRows.ts`，contract `web/admin/src/pages/configRows.contract.ts` 可独立运行，覆盖 `auth_security/payments` 高危配置必须要求 `manage:dangerous_config`、普通配置要求 `manage:config`、高危锁定文案必须提示支付/认证/密钥和超级管理员、未知 tab/key 保留原始值用于排障。
  - `ConfigPage.tsx` 已复用配置页展示模型：`payments` tab 明确定位为底层支付配置并提示日常运营优先走收银台页；`signup_trial`、`visible_methods`、`provider_instances`、自定义金额边界等字段补齐运营可读说明；结构化列表以 JSON 文本框承载，保存 payload 仍提交后端原始 `config_category/config_key/config_value/scope` 契约。
  - `npm exec tsx src/pages/configRows.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐后台系统设置页字段元数据、高危权限说明和结构化列表编辑后通过。
  - 后台运营总览页展示模型已抽到 `web/admin/src/pages/overviewRows.ts`，contract `web/admin/src/pages/overviewRows.contract.ts` 可独立运行，覆盖最新注册用户按注册时间倒序取 5 条、状态文案复用用户管理页口径、`closed/unknown` 状态回退、空昵称回退邮箱，以及 `permission='users.manage'` 管理入口锚点。
  - `OverviewPage.tsx` 已复用总览展示模型：最新用户不再在页面内维护独立状态映射，避免 `disabled/closed` 文案漂移；总览指标新增本地化模型，`payment_success_rate`、`refund_compensation_failed_count` 等关键指标从 raw key 转为运营可读标题与处理说明，未知指标仍保留原始 key 便于排障。
  - `npm exec tsx src/pages/overviewRows.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐后台运营总览用户与指标展示模型后通过。
  - 后台登录页文案模型已抽到 `web/admin/src/pages/adminLoginCopy.ts`，contract `web/admin/src/pages/adminLoginCopy.contract.ts` 可独立运行，锁定默认不再预填 `admin123` 等演示弱口令，只有显式 `VITE_DEFAULT_ADMIN_EMAIL/PASSWORD` 才会填充；登录页可见文案统一为中文运营后台口径，不再展示 `Soft Grid Ops / Admin Access / Route guard` 等混合英文和内部实现术语。
  - `LoginPage.tsx` 已复用后台登录文案模型：登录失败会将 401/403 映射为“管理员邮箱或密码不正确”，网络失败映射为部署/连接检查提示，未知后端错误仍保留原值用于排障；表单校验保持中文提示。
  - `npm exec tsx src/pages/adminLoginCopy.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 在补齐后台登录页产品化 copy 和默认表单行为后通过。
  - 用户端收银台最近订单与支付方式展示继续补齐人工确认渠道中文化：`checkoutPaymentMethodLabel` 新增 `manual_alipay/manual_wxpay/manual_bank` 到“人工确认 · 支付宝 / 微信支付 / 银行转账”的展示映射，避免用户最近订单中直出后台人工补单 provider 枚举。
  - `checkoutPaymentMethodOptionModel` 在可见支付方式使用自定义 `method` 但底层渠道为 `manual_bank` 时，按钮说明展示“人工确认 · 银行转账 渠道”，同时保留自定义入口 raw method 作为提交契约。
  - `npm exec tsx src/pages/checkoutRecentOrders.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐用户端人工确认渠道展示后通过。
  - 用户端收银台订单时间展示已收敛到 `checkoutDateTime`：订单详情“过期时间”和最近订单创建时间不再由页面直接 `slice` ISO 字符串，统一展示为 `YYYY/MM/DD HH:mm`，避免用户看到 `T/Z` 等技术格式；非法时间保留原值用于排障，空值显示 `-`。
  - `checkoutRecentOrderRows` 新增 `createdAtLabel`，最近订单列表继续保留原始 `createdAt` 作为 `<time dateTime>` 契约，同时可见文本使用格式化后的用户友好时间。
  - `npm exec tsx src/pages/checkoutRecentOrders.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐收银台订单时间展示后通过。
  - 用户端 API Key 页过期时间与生命周期展示已收敛到 `apiKeyRows`：`apiKeyExpiryText` 将 ISO 过期时间格式化为 `YYYY/MM/DD`，空值明确展示“永不过期”，并通过 `apiKeyExpiryHint` 对永不过期生产密钥提示“建议为生产密钥设置有效期”；非法时间保留原值用于排障，避免页面直出 `T/Z` 技术格式。
  - `apiKeyRow` 新增 `accessKeyMasked`、`expiresAtLabel`、`createdAtLabel`、`lastUsedAtLabel`、`expiryHint`，列表页不再自行拼接 AK 遮罩或直接读取 `expires_at`；状态、权限范围和过期风险提示统一由页面模型输出，后续扩展密钥分组/额度/完整权限系统时可继续沿该模型补字段。
  - `npm exec tsx src/pages/apiKeyRows.contract.ts`、`npm --prefix web/user run typecheck` 在补齐 API Key 过期时间和状态展示模型后通过。
  - 用户端 API Key 列表已继续补齐生命周期列：表头新增“创建时间 / 最近调用”，`ApiKeysPage.tsx` 直接展示 `apiKeyRow.createdAtLabel` 与 `lastUsedAtLabel`；最近调用为空显示“未调用”，有效时间统一为 `YYYY/MM/DD`，避免开发者排查密钥活跃度时看到原始 ISO 时间或无法判断密钥是否仍在使用。
  - `apiKeyRows.contract.ts` 已锁定生命周期列表头和行模型日期展示，防止后续页面只计算不展示、或重新退回 `T/Z` 技术格式。
  - 用户端 API Key 额度配置已补齐到页面闭环：列表新增“额度”列，展示“总额度 已用 / 上限”和“日额度 已用 / 上限”，未设置上限时展示“不限额”；创建密钥弹窗新增“总额度 / 日额度”两个可选输入，不填即提交 `null` 表示不限额。
  - `apiKeyCreatePayload` 统一生成创建密钥 payload，提交 `name/scopes/rpm_limit/expires_at/total_quota_points/daily_quota_points`，`web/shared/user-api.ts` 透传后端既有字段，开发态 mock API 与初始 mock key 也同步支持额度字段，避免本地体验和真实 API 漂移。
  - `apiKeyRows.contract.ts` 已锁定额度展示、无限额兜底和创建 payload 字段，覆盖 PRD 中“密钥可设置总额度和日额度”的用户端交互缺口。
  - 用户端 API Key 已补齐已创建密钥的维护入口：列表操作新增“编辑”，弹窗支持修改密钥名称、RPM、总额度、日额度和过期时间；当前分组仅只读展示，保存 payload 明确不提交 `group_code`，对齐后端禁止用户侧改分组的契约。
  - `apiKeyEditForm` / `apiKeyUpdatePayload` 统一处理编辑表单初始值和提交值：ISO 过期时间转为 date input，空额度/空过期时间提交为 `null`；`web/shared/user-api.ts` 在更新密钥时将日期转 ISO 后再发给 Go `time.Time` 解码，避免浏览器表单值导致接口失败。
  - `apiKeyRows.contract.ts` 已锁定编辑表单、更新 payload、不提交 `group_code` 和空值归一规则；`npm exec tsx src/pages/apiKeyRows.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐 API Key 编辑维护入口后通过。
  - 用户端 API Key 编辑弹窗的分组说明已产品化为 `apiKeyGroupReadOnlyHint`：页面明确“密钥分组由账号分组统一决定，管理员调整账号分组后会同步影响密钥可用范围”，不再出现“当前版本不支持”等半成品话术；contract 同时锁定该说明不得回退到路线图/版本文案，为后续完整自定义权限系统保留统一模型出口。
  - 用户端 API Key 快速接入示例已修正 OpenAI 兼容鉴权口径：`/#/api-keys` 的 `/v1/images/generations` cURL 示例优先使用创建/重置后仅本地保留的一次性 `secret_preview`/`secret` 作为 `Authorization: Bearer sk-*`，无可用 Secret 时使用 `sk_live_xxx` 占位，不再误把 `pk_*` Access Key 当 Bearer token；`ApiKeysPage.tsx` 会将本地 `secretPreviews` 合并到当前选中密钥后生成 Quickstart，避免刚创建或重置后示例仍显示错误凭据。
  - `apiKeyRows.contract.ts` 已锁定 Quickstart 必须使用 `sk_*` Secret、不得出现 `Bearer pk_*`，并覆盖空状态占位符；`npm exec tsx src/pages/apiKeyRows.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在修正 API Key Quickstart Bearer 示例后通过。
  - 用户端图库卡片展示模型已收敛到 `web/user/src/pages/galleryRows.ts` 的 `galleryImageCard`：卡片标题、模型行、分组、公开状态、创建时间、资源地址、下载/公开动作可用性均由纯模型输出，`GalleryPage.tsx` 不再直接 `slice` ISO 时间或只按 `url` 判断下载能力。
  - `galleryRows.contract.ts` 已锁定图库卡片创建时间展示为 `YYYY/MM/DD HH:mm`、非法时间保留原值、`download_url` 可触发下载、已公开/审核中图片不可重复申请公开、无图片文件时同时禁用下载和公开；`npm exec tsx src/pages/galleryRows.contract.ts`、`npm --prefix web/user run typecheck`、`npm --prefix web/user run build` 在补齐图库资产卡片模型后通过。
  - 用户端公开图片详情共享组件已补齐 `web/user/src/components.contract.ts`，锁定 `PublicImageDetail` 依赖的 `formatDate()` 统一展示 `YYYY/MM/DD HH:mm`，空值显示 `-`，非法时间保留原值；首页灵感弹窗、图库详情弹窗、公开广场详情弹窗不再通过共享组件暴露 `2026-06-05T13:45` 这类 ISO 片段。
  - 用户端收银台 JSAPI 支付展示文案已从“当前环境暂不支持微信 JSAPI”收口为支付环境限制说明：“微信内支付需在微信环境打开”，并提示当前浏览器改选 H5 或扫码支付；`checkoutPaymentDisplay.contract.ts` 已锁定 JSAPI 空间不得回退到“当前版本/暂不支持/后续/即将”等半成品话术。
  - 后台收银台订单运营风险展示已抽到 `web/admin/src/pages/cashierRiskRows.ts`，订单详情弹窗顶部新增风险卡片，将待支付查单/人工补单、支付失败、可退余额、部分退款、全额退款、最近退款单号和渠道交易号归纳为运营可读处理建议；回调事件列表对 `payment.retryable_failed` 和 `refund.local_finalize_failed` 展示重试/补偿导向，不再只暴露失败原因字符串。
  - `cashierRiskRows.contract.ts` 锁定部分退款金额与剩余可退金额、退款本地落账失败补偿提示、支付回调失败对账提示，以及不得在可见文案中直出 `partially_refunded` / `refund.local_finalize_failed` / `payment.retryable_failed` 等内部枚举；`npm exec tsx src/pages/cashierRiskRows.contract.ts`、`npm --prefix web/admin run typecheck` 在补齐后台收银台运营风险展示后通过。
  - 后台收银台回调事件已补齐 payload/验签/处理结果排障闭环：`PaymentWebhookEvent` admin DTO 新增 `signature_status`、`result_summary`、`payload_preview`，Ent store 从真实 webhook payload/signature/status 派生字段，Memory store 同步补齐本地/测试事件字段，OpenAPI `AdminPaymentWebhookEvent` 同步文档化。
  - `web/admin/src/pages/cashierWebhookRows.ts` 将回调事件行归一为事件标题、订单、渠道、状态、验签状态、处理摘要、接收/处理时间和 payload 预览；`CashierPage.tsx` 在回调事件列表内直接展示 inspector 区域，满足“查看 webhook payload、验签状态、处理结果、重试”。`cashierWebhookRows.contract.ts`、`go test ./internal/http/router -run TestAdminCashierOrderRefundRecordsCompensationWhenLocalFinalizeFailsAfterProviderSuccess -count=1`、`go test ./internal/repository/entstore -run TestBillingStoreRefundFinalizeFailureEventCanRetryLocalRefund -count=1`、`go test ./api/openapi -count=1`、`npm --prefix web/admin run typecheck && npm --prefix web/admin run build` 已通过。
  - 后台收银台订单级争议/拒付追扣已补齐订单摘要闭环：`PaymentOrder` 新增 `chargeback_points`、`chargeback_reason`、`chargeback_at`、`chargeback_idempotency_key` 响应字段；`/api/ops/admin/v1/cashier/orders/{order_id}/chargeback` 在管理员追扣成功后将摘要写入订单 `provider_payload` 并返回更新后的订单，后台刷新订单详情仍可看到最近一次争议处理结果。
  - `cashierRiskRows` 新增“争议追扣”风险卡片，展示已追扣积分和运营原因，不再只依赖审计日志追溯渠道拒付；`scripts/test/api_contract_smoke.sh` 也补齐真实 HTTP 链路断言，要求追扣响应和后台订单详情均包含摘要字段。`go test ./internal/http/router -run TestAdminCashierOrderChargebackDeductsBalanceAndIsIdempotent -count=1`、`go test ./internal/repository/entstore -run TestBillingStoreRecordChargebackSummaryPersistsOnOrder -count=1`、`go test ./api/openapi -count=1`、`npm exec tsx src/pages/cashierRiskRows.contract.ts`、`npm --prefix web/admin run typecheck` 已通过。
  - 后台主动查单同步已补齐渠道异常运营分类：`AdminCashierOrderSyncResult` 新增 `risk_category` 与 `action_hint`，服务端将渠道状态归一为 `pending/paid/closed/failed/refunded`，并进一步区分 `risk_control`、`channel_limited`、`signature_error`、`amount_mismatch`、`account_abnormal`、`channel_timeout`、`channel_error` 等运营风险类别，返回可执行处理建议。
  - 后台收银台页面已接入 `cashierSyncRows` 展示模型，主动查单反馈不再只显示 raw `query_status`，而是展示“渠道已支付 / 渠道已关闭 / 风控拦截 / 渠道限额 / 签名配置异常 / 金额不一致 / 商户账号异常 / 查单超时 / 渠道处理异常”等中文分类与下一步处理建议；contract 禁止可见文案回退到 `risk_control`、`channel_limited`、`signature_error`、`amount_mismatch`、`account_abnormal`、`channel_timeout`、`TRADE_CLOSED`、`SUCCESS` 等渠道原始枚举。
  - `scripts/test/api_contract_smoke.sh` 已补齐 Mock 风控查单真实 API smoke：后台先禁用已支付查单和轮询测试的临时 Mock provider，再创建专用 Mock 查单渠道实例并配置 `query_status=risk_control`，用户创建待支付订单后必须绑定该实例；后台 `/sync` 必须保持订单 `pending`、返回 `query_status=failed`、`risk_category=risk_control` 且 `action_hint` 包含更换支付渠道建议；随后取消订单并禁用临时 provider，避免影响后续 pending-limit smoke。`go test ./internal/http/router -run 'TestAdminCashierOrderSync(ClassifiesRiskControlStatus|CompletesPaidProviderOrder|RejectsPaidAmountMismatch)' -count=1`、`go test ./api/openapi -count=1`、`npm exec tsx src/pages/cashierSyncRows.contract.ts`、`npm --prefix web/admin run typecheck && npm --prefix web/admin run build` 已通过。
  - 后台收银台 JeePay 渠道实例已补齐字段级配置表单：`cashierJeePayConfigFields` 固定输出“网关地址 / 商户号 / 应用 ID / 商户密钥 / 支付模式 / wayCode / 客户端 IP / 渠道参数”，`cashierJeePayStructuredConfig` 可从原始 JSON 提取常用字段并保留未知字段，`updateCashierJeePayStructuredConfig` 将表单改动回写到原 `config_text`，保存接口仍提交既有 `config` JSON 契约。
  - `CashierPage.tsx` 的支付渠道实例弹窗在 JeePay provider 下新增“JeePay 常用字段”区域，运营可直接填写 wayCode、服务商/JSAPI/分账等 `channel_extra` JSON；原始 JSON 文本框仍保留用于高级排障和未知字段。`cashierProviderOptions.contract.ts` 已锁定字段中文文案、未知配置保留、非法渠道参数错误提示和非 JeePay provider 不展示结构化表单；`npm exec tsx src/pages/cashierProviderOptions.contract.ts`、`npm --prefix web/admin run typecheck`、`npm --prefix web/admin run build` 已通过。
  - 后台主动查单同步已继续补齐更细渠道错误码分类规则：`normalizeCashierQueryStatus` 将限额、签名/验签、金额不一致、商户账号异常、查单/网关超时分别归入独立 `risk_category`，OpenAPI `AdminCashierOrderSyncResult.risk_category` 和 `web/shared/api-types.ts` 同步枚举；`cashierSyncRows.contract.ts` 锁定五类新风险的中文标签与处理建议。
  - 后台 JeePay 模板已从纯 wayCode 按钮升级为“场景模板”：模板模型新增中文 `category`，后台弹窗按基础支付、网页支付、移动支付、小程序、服务商、分账和行业参数展示中文名称、分类与 wayCode；保存仍回写既有 `config` JSON。
  - JeePay 行业参数模板首批补齐餐饮外卖 H5、停车缴费扫码、酒店预授权、校园缴费四类非常规场景，分别沉淀 `storeInfo/terminalInfo`、`parkingInfo`、`industryScenario/hotelOrderNo`、`schoolInfo` 等 `channel_extra` 示例；`cashierJeePayWayCodeTemplates.contract.ts` 锁定模板分类、合并已有配置、支付宝/微信双侧行业覆盖和可见文案不得回退到占位/路线图话术。
  - `scripts/test/api_contract_smoke.sh` 已从“Open API 任务只验证 queued”升级为真实 worker 端到端链路：smoke 启动临时 OpenRouter 兼容 fake provider，通过后台模型账号 API 创建 `openrouter/api_key/base_url` 账号和启用账号模型，用户用 Open API 创建文生图任务后启动 `cmd/worker` 处理队列，必须轮询到任务 `succeeded`、结果图进入私有图库、账本出现 `consume/usage/task` 扣费流水、后台用户详情展示该任务为 `succeeded`。同时修复 `modelhub.Resolver` 将后台模型账号状态 `enabled` 误判为不可用的问题，新增 `TestResolveAllowsEnabledModelAccountCandidates`，证明后台启用账号可驱动默认抽象模型生成。
- 浏览器手动验收补证：
  - 已用临时 SQLite 验收环境启动后端 `http://127.0.0.1:8080`、用户端 `http://127.0.0.1:5173`、后台端 `http://127.0.0.1:5175`。
  - 游客访问 `/#/public-gallery` 可正常打开空广场态，控制台无 warning/error，截图：`.codex/acceptance-public-gallery-guest.png`。
  - 新用户通过邮箱验证码登录，首页展示注册送体验额度 `20.00`、有效期至 `2026-06-12`，无模型时“开始生成”禁用且提示平台能力配置中，截图：`.codex/acceptance-home.png`。
  - 工作台 `/#/genpic` 在无可用模型时禁用“开始创作”，提示“平台生图能力正在配置中，请稍后再试”，未暴露 route/provider/account 等后台术语，截图：`.codex/acceptance-workspace.png`。
  - 用户收银台 `/#/checkout` 读取固定积分包与 Mock 支付方式，创建 `Basic Monthly` 订单后点击“模拟支付成功”，订单变为 `completed`，余额从 `20.00000` 刷新为 `120.00000`，最近订单可见，截图：`.codex/acceptance-checkout.png`。
  - 个人中心 `/#/profile` 固定展示体验额度、订阅额度、充值额度三类余额桶；体验额度 `20.00000` 有效期至 `2026/06/12`，充值额度 `100.00000` 标记长期有效，流水可区分注册赠送和支付订单，截图：`.codex/acceptance-profile-buckets.png`。
  - 后台 `/#/readiness` 展示 pass/warn/fail 检查、阻塞上线/非阻塞文案和修复入口；当前测试环境因未配置模型账号/路由/价格而 fail，支付与注册送体验额度为 pass，控制台无 warning/error，截图：`.codex/acceptance-admin-readiness.png`。
  - 后台 `/#/cashier` 展示自定义金额、固定积分包、可见支付方式、Mock 渠道实例、刚才用户端完成的 `completed` 订单和 `processed` 回调事件，控制台无 warning/error，截图：`.codex/acceptance-admin-cashier.png`。
  - 后台 `/#/users` 用户详情弹窗已用临时 SQLite 环境补充验收：验收用户经注册送体验额度、Mock 充值、Open API 创建任务、API Key 创建和后台积分调整后，用户列表显示余额 `125.00000`；详情弹窗展示可用积分 `125.00000`、体验额度 `18.00000`、充值余额 `100.00000`、`trial/gift/recharge` 余额桶、后台调整/生成预冻结/充值到账/体验额度发放流水、`completed` 订单、`queued` 任务和 `ui-detail-key` API Key；控制台无 warning/error，截图：`.codex/acceptance-admin-user-detail.png`。
- 剩余收银台缺口：
  - `wxpay_direct` Native/H5/JSAPI 预下单、notify 签名验签与解密已完成最小闭环。
  - 易支付 popup/API 预下单、notify 签名验签已完成最小闭环。
  - 后台人工补单/人工确认到账已完成最小闭环。
  - 后台主动查单同步已完成配置驱动骨架闭环，且支付宝/微信官方、易支付、JeePay HTTP 查单均已接入；渠道差异状态已归一化为 `pending/paid/closed/failed/refunded` 并保留 raw 原始响应，常见风控、限额、签名、金额不一致、商户账号异常、超时和普通渠道异常已归入独立运营分类并返回处理建议。
  - 后台未消费充值余额全额退款和部分退款已完成最小闭环；支付宝官方、微信官方、易支付、JeePay 真实渠道退款最小 adapter 已接入并按本次退款金额请求渠道；渠道退款前本地可退款预检与 recharge grant 冻结已完成，可避免余额已消费/冻结/不足时仍先打渠道退款，也可避免真实渠道退款调用期间被生图任务并发消费；真实渠道已退款但本地最终落账失败时已写入 `refund.local_finalize_failed` 失败事件，并复用后台 webhook event 重试入口和独立 worker 自动扫描做补偿；后台 dashboard 已新增 `refund_compensation_failed_count` / `refund_compensation_oldest_failed_at` 和 `refund_compensation_failures` 危险指标卡，上线检查新增 `refund_compensation` 阻塞检查，失败补偿不会只藏在回调列表里；订单级人工追扣已通过 `/api/ops/admin/v1/cashier/orders/{order_id}/chargeback`、审计 `cashier.order.chargeback` 和后台“追扣”弹窗完成最小闭环。
  - JeePay 下单/回调/主动查单/真实退款最小闭环已完成；JeePay `payment_mode=api/qrcode/qr_code` 预下单 POST 模式已完成；`wayCode` 差异化参数已通过结构化 `channelExtra` 配置打通，后台收银台已新增支付宝 PC/JSAPI/服务商、微信扫码/JSAPI/H5/小程序/服务商/分账模板辅助配置，并补齐常用字段级配置表单；非常规行业参数已先沉淀餐饮外卖、停车缴费、酒店预授权、校园缴费模板。后续只需按真实商户行业继续追加模板数据，不再需要改收银台配置契约。
