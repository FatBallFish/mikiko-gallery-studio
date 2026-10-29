# 快速开始

先发现模型，再预检、创建和查询任务。REST 和 OpenAI 兼容接口均使用 Bearer API Key。

## 接入地址 {#base-urls}

| 用途 | 地址 |
| --- | --- |
| 原生 REST | https://mikiko.studio/api/open/v1 |
| OpenAI SDK | https://mikiko.studio/v1 |
| MCP | https://mikiko.studio/mcp |

`MIKIKO_API_KEY` 填创建或轮换密钥时一次显示的 **SK（`sk-` 开头）**，由服务端环境变量注入。AK（`ak-` 开头）只是密钥标识，不能用于 Bearer 认证。

## 确认身份与模型 {#first-request}

```bash
curl https://mikiko.studio/api/open/v1/me \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
curl https://mikiko.studio/api/open/v1/image-models \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
curl https://mikiko.studio/api/open/v1/projects \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

## 创建第一项任务 {#next}

1. 从响应选择自己的项目和可用模型。
2. 按模型能力构建完整请求，调用 `POST https://mikiko.studio/api/open/v1/image-tasks/preflight`。
3. 保留相同业务输入，加入返回的 `quote_token` 和新的 `idempotency_key`，调用 `POST https://mikiko.studio/api/open/v1/image-tasks`。
4. 保存 `data.id`，查询任务详情，结果就绪后获取临时访问链接。

预检不扣费；创建会重新检查权限、能力、价格、余额和配额，生成任务可能产生费用。

继续阅读 [SDK 示例](sdk-examples.md)、[字段映射](developer-field-map.md) 或 [MCP 授权](mcp.md)。
