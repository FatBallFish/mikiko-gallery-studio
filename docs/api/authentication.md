# 认证与密钥

## 使用 SK 认证

在密钥页创建或轮换后，保存一次显示的 **SK（`sk-` 开头）**。它就是实际的 Bearer API Key，应放入服务端的 `MIKIKO_API_KEY` 环境变量。

- AK（`ak-` 开头）只用于标识密钥，不能放入 Bearer 请求头。
- 原生 REST 和 OpenAI 兼容接口使用同一个 SK，无需额外签名。
- 不把 SK 放进网页代码、URL、日志或公开仓库；遗失后轮换。

```bash
curl https://mikiko.studio/api/open/v1/me \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

## 权限与额度 {#scopes}

- 按操作授予最小 scope，例如模型发现需要 `models:read`。
- 图片密钥不会自动获得视频、删除或项目管理权限。
- 每次请求检查账号、密钥状态、过期时间和 scope。
- RPM、总额度和日额度均适用；预检不扣费。

## 区分其他凭据

MCP 使用[浏览器 OAuth](mcp.md)获取令牌。浏览器会话、管理员会话、支付回调和供应商回调各有独立认证方式，不能互换。
