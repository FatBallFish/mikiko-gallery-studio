# 支付渠道配置指南

支付渠道实例在管理后台的“支付配置”中维护。普通配置与密钥分开保存；密钥、私钥和 webhook signing secret 保存后不会明文回显。编辑现有实例时，密钥字段留空会保留原值，只有显式轮换时才重新填写。

## 通用配置

- 带“必填”标识的字段必须填写；最低/最高金额、日限额等字段为可选限制。
- “异步回调基础域名”和“同步返回基础域名”只填写 origin，例如 `https://gallery.example.com`。后台保存时自动拼接项目路由。
- 异步通知路由为 `/api/open/image/v1/payments/webhooks/<provider_type>`，同步返回路由为 `/#/checkout`。
- 默认值来自当前管理后台 origin。生产环境必须改为支付平台可公网访问的 HTTPS 域名。
- 保存后先使用测试环境完成下单、回调、到账、查单和退款，再启用正式实例。

## JeePay

JeePay 支付宝和微信实例分别使用 `jeepay_alipay`、`jeepay_wxpay`。必填字段：

| 字段 | 说明 |
| --- | --- |
| 网关地址 | JeePay 服务地址，例如 `https://pay.example.com` |
| 商户号 | JeePay `mchNo` |
| 应用 ID | JeePay `appId` |
| 商户密钥 | 签名 key，仅写入密钥存储 |
| `wayCode` | 实际支付通道编码 |

支付模式、客户端 IP 和渠道参数为可选项。微信 JSAPI/小程序、服务商子商户、分账等场景通常还需要渠道参数；使用管理后台内置模板生成结构后，再按 JeePay 和下游渠道文档填写。商户号与应用 ID 属于普通配置，编辑保存后应继续可见；商户密钥不会回显，留空保存会保留已有值。

MGS 通过服务端 `POST application/json` 调用 JeePay 的 `/api/pay/unifiedOrder`，浏览器只接收 JeePay 返回的支付 URL、二维码或表单。不要把 `gateway_url` 配成带签名参数的统一下单 URL，也不要让用户端直接访问 `/api/pay/unifiedOrder`。查单 `/api/pay/query` 和退款 `/api/refund/refundOrder` 同样使用 JSON；`amount`、`refundAmount`、`reqTime` 按 JSON 数字发送。

异步通知示例：

```text
https://gallery.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay
```

JeePay 回调必须同时携带与实例匹配的 `mchNo` 和 `appId`，签名覆盖除 `sign` 外的所有非空字段（包括 `signType`）。服务端只接受明确的 `state=2` 成功状态并返回精确文本 `success`；缺少状态也会被拒绝。

## 易支付

易支付托管页使用 `submit.php`，API/二维码模式使用 `mapi.php`，查单和退款使用 `api.php`。实际回调地址按实例类型区分：

```text
https://gallery.example.com/api/open/image/v1/payments/webhooks/easypay_alipay
https://gallery.example.com/api/open/image/v1/payments/webhooks/easypay_wxpay
```

回调路径、`pid`、签名、明确的 `trade_status=TRADE_SUCCESS`（或兼容值 `1`）和非空且匹配的订单金额必须同时满足。托管页 URL 必须包含易支付要求的临时签名参数，但 API 响应不会在 `payment_display` 中重复暴露独立的 `sign` 字段。

## 支付宝原生

支付宝原生使用 `alipay.trade.page.pay` 与 RSA2。回调按 `app_id` 选择实例，使用支付宝公钥验签，只接受明确的 `TRADE_SUCCESS` 或 `TRADE_FINISHED`，且金额必须存在并匹配，然后返回精确文本 `success`。查单和退款业务响应的 `code` 必须明确为 `10000`；缺少成功码或返回业务错误都不会被误判为待支付或退款已受理。

## 微信支付原生

微信支付原生支持 Native、H5 和 JSAPI，使用 API v3 请求签名、平台公钥验签和 AES-GCM 回调解密。回调除平台签名外，还必须满足事件类型 `TRANSACTION.SUCCESS`、解密后的 `trade_state=SUCCESS`，且 `appid`、`mchid` 与实例一致。成功响应为微信要求的 JSON：

```json
{"code":"SUCCESS","message":"成功"}
```

## Stripe

Stripe 当前使用 PaymentIntent 与前端 Payment Element，收银台仅按 CNY 创建订单。必填字段：

| 字段 | 说明 |
| --- | --- |
| Publishable key | 前端初始化 Payment Element 使用，通常以 `pk_test_` 或 `pk_live_` 开头 |
| Secret key | 后端创建、查询 PaymentIntent 和退款使用，通常以 `sk_test_` 或 `sk_live_` 开头 |
| Webhook signing secret | Stripe webhook endpoint 的签名密钥，通常以 `whsec_` 开头 |

在 Stripe Dashboard 中创建 webhook endpoint：

```text
https://gallery.example.com/api/open/image/v1/payments/webhooks/stripe
```

至少订阅 `payment_intent.succeeded` 和 `payment_intent.payment_failed`。测试模式的 publishable key、secret key、PaymentIntent 和 webhook secret 必须成套使用；不要把 test 与 live 凭据混用。生产切换 live mode 前，应重新创建 live webhook endpoint，并在管理后台一次性轮换三项凭据。

用户端只接收 Payment Element 所需的 publishable key、client secret 和显示类型。Stripe secret key 与 webhook signing secret 不会进入 API 响应或页面 DOM。

## 协议审计矩阵

| 渠道 | 创建 | 回调 | 查单 | 退款 | 幂等与超时 |
| --- | --- | --- | --- | --- | --- |
| JeePay | JSON 统一下单，整数分/毫秒时间，MD5 含 `signType` | 商户+应用+订单实例绑定，表单验签，明确 `state=2` | JSON、同一签名规则、明确 `code=0` | JSON、整数分、同一签名规则、明确 `code=0` | 先存本地订单；结果未知保持 `pending` |
| 易支付 | `submit.php` 或表单 POST `mapi.php`，MD5 | 类型路径+`pid`+订单实例绑定，明确成功状态，金额校验 | `api.php` | `api.php` | API 模式 15 秒超时；本地幂等键防重复 |
| Stripe | PaymentIntent，整数分，订单号作为 Stripe 幂等键 | 原始正文验签、渠道实例/CNY/金额/metadata 校验 | PaymentIntent 状态映射 | 退款号作为幂等键 | SDK 上下文取消；结果未知保持 `pending` |
| 支付宝原生 | RSA2 页面支付，金额精确到分 | `app_id`+订单实例绑定、公钥验签、明确成功状态、金额校验 | `code=10000` 后映射状态 | RSA2 `alipay.trade.refund` | 本地订单号和退款号保持稳定 |
| 微信支付原生 | API v3 Native/H5/JSAPI，整数分 | 平台签名、AES-GCM、订单实例/商户身份/金额校验 | API v3 商户订单号查询 | API v3 国内退款 | 15 秒超时；结果未知保持 `pending` |

所有支付上游响应和 webhook 正文限制为 1 MiB。商户密钥、私钥、webhook secret、Authorization 和原始请求签名不得写入日志或独立返回字段。

所有真实支付渠道的成功回调都必须与订单创建时记录的 `provider_instance_id` 精确匹配。即使另一个实例使用同类渠道且签名合法，也不能完成该订单；历史上没有实例 ID 的旧订单仅保留渠道类型校验以便平滑处理存量回调。

## 验收

1. 保存实例后重新打开编辑窗口，确认普通字段保留、密钥不回显且配置状态为已配置。
2. 创建最小金额测试订单，确认用户端出现对应支付方式。
3. 完成支付并确认订单只入账一次；重复 webhook 不得重复增加积分。
4. 在管理后台执行查单，确认渠道交易号与金额一致。
5. 执行部分退款，确认渠道退款号、审计记录、订单退款金额和扣回积分一致。
