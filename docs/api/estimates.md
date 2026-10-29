# 完整请求预检

## 发送完整业务输入

预检与创建使用相同项目、参考资产、提示词变量、绑定和模型参数。

| 用途 | 地址 |
| --- | --- |
| 图片预检 | `POST https://mikiko.studio/api/open/v1/image-tasks/preflight` |
| 视频预检 | `POST https://mikiko.studio/api/open/v1/video-tasks/preflight` |
| 查询余额 | `GET https://mikiko.studio/api/open/v1/me/balance` |

## 使用报价创建任务

1. 预检检查归属、能力、余额和密钥配额，不扣费。
2. 保存返回的 `quote_token`、有效期和能力版本。
3. 使用同一业务输入、报价和幂等键创建任务。
4. 参数或报价变化后重新预检。

原生图片报价有效期为 120 秒，过期返回 `QUOTE_STALE`。创建时仍会重新校验并原子预留费用。

## 处理积分

积分使用五位小数字符串。避免用浮点运算计算费用；CNY 费率与积分是不同单位。
