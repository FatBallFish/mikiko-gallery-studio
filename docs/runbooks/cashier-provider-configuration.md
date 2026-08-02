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

异步通知示例：

```text
https://gallery.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay
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

## 验收

1. 保存实例后重新打开编辑窗口，确认普通字段保留、密钥不回显且配置状态为已配置。
2. 创建最小金额测试订单，确认用户端出现对应支付方式。
3. 完成支付并确认订单只入账一次；重复 webhook 不得重复增加积分。
4. 在管理后台执行查单，确认渠道交易号与金额一致。
5. 执行部分退款，确认渠道退款号、审计记录、订单退款金额和扣回积分一致。
