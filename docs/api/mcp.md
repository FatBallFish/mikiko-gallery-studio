# MCP 连接与授权

## 接入地址与凭据

Mikiko Studio 的 MCP 地址为 `https://mikiko.studio/mcp`。客户端打开浏览器完成登录和授权，无需在配置中填写令牌。

- REST/兼容接口的 `MIKIKO_API_KEY` 是一次显示的 SK（`sk-`）；AK（`ak-`）只作标识。
- MCP 使用独立 OAuth 令牌，不能用 SK 或浏览器会话替代。
- 配置文件不保存 API Key、access token 或 refresh token。

## 客户端配置

### Mikiko / ZCode

合并至工作区 `.mikiko/config.json`（兼容名 `zcode.json`）或用户 `~/.mikiko/cli/config.json`：

```json
{
  "mcp": {
    "servers": {
      "studio": { "type": "http", "url": "https://mikiko.studio/mcp" }
    }
  }
}
```

1. 在设置 → MCP 中检查该 HTTP 服务，重启受影响会话。
2. 使用客户端提供的浏览器授权入口登录并同意权限。
3. 连接后检查工具列表。

用户同名服务覆盖工作区配置。不添加猜测的 `oauth` 字段。若客户端版本没有浏览器授权入口，先升级或确认版本支持。

### Codex

合并至 `~/.codex/config.toml`：

```toml
[mcp_servers.studio]
url = "https://mikiko.studio/mcp"
```

```bash
codex mcp login studio
codex mcp list
```

按浏览器提示完成授权。配置说明见 [Codex 官方文档](https://developers.openai.com/codex/mcp)。MCP 地址不要填到模型 provider 的 BaseURL。

### Claude Code

```bash
claude mcp add --transport http studio https://mikiko.studio/mcp
claude mcp list
```

在 Claude Code 输入 `/mcp`，选择 studio，再按 Authenticate 提示授权。此处针对 Claude Code CLI；桌面连接器入口不同。见 [Claude Code 官方文档](https://code.claude.com/docs/en/mcp)。

## OAuth 协议流程

### 发现与注册

1. 未认证的 MCP POST 返回 401 和 `WWW-Authenticate`，客户端读取其中的 resource metadata URL。
2. 受保护元数据给出精确资源 `https://mikiko.studio/mcp`、`authorization_servers`、`scopes_supported` 和 `bearer_methods_supported:["header"]`。
3. 获取 `https://mikiko.studio/.well-known/oauth-authorization-server`，校验 issuer 和各端点。HTTP 200 却返回 HTML 通常是代理路由错误。
4. 根据策略使用预注册客户端、经过校验的 Client ID Metadata Document，或动态注册。

动态注册请求发送到 discovery 返回的 `registration_endpoint`：

```json
{
  "client_name": "你的客户端名称",
  "redirect_uris": ["REPLACE_WITH_CLIENT_CALLBACK"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"]
}
```

201 响应包含 `client_id`，不发 `client_secret`。动态注册可能被关闭、限速或受活动客户端容量限制。

Discovery 还声明 `revocation_endpoint`、支持的 response/grant/token-auth 类型、`code_challenge_methods_supported` 和 `client_id_metadata_document_supported`。

### 浏览器授权

客户端生成随机 `state`、PKCE verifier 和 S256 challenge，打开 `authorization_endpoint`，携带：

```text
client_id, redirect_uri, resource, response_type=code,
scope, code_challenge, code_challenge_method=S256, state
```

- `redirect_uri` 须与注册值精确匹配；`resource` 须精确匹配 MCP 资源。
- scope 使用空格分隔。
- 服务器 303 跳转至 `/?oauth_request=...#/oauth-consent`，使用 Studio 本地登录或 OIDC。
- 浏览器登录保留 `oauth_request`，不要修改请求 ID 或回调。

浏览器会话读取 `GET https://mikiko.studio/api/v1/oauth/consent?request_id=...`，得到请求、客户端、资源、权限和 `csrf_token`。POST 同路径提交：

```json
{ "request_id": "REQUEST_ID", "csrf_token": "CSRF_TOKEN", "allow": true }
```

响应包含 `redirect_uri`。用户拒绝时回调返回 OAuth error；请求过期后从客户端重新发起。

### 换取令牌与调用工具

客户端校验回调 `state`，向 `token_endpoint` 发送 form：

```text
grant_type=authorization_code
client_id, resource, code, redirect_uri, code_verifier
```

不携带浏览器 Authorization 或 client secret。响应字段为 `access_token`、`token_type:"Bearer"`、`expires_in`、`refresh_token`、`scope`，当前访问令牌有效期为 600 秒。

1. 使用 OAuth Bearer 令牌调用 MCP initialize。
2. 读取协商的 `protocolVersion`，后续携带 `MCP-Protocol-Version`，按 SDK 要求发送初始化通知。
3. 调用 `tools/list`、`tools/call`。工具结果为 `structuredContent:{data:...}`，文本 JSON 使用同样包装。
4. 创建任务后保存持久 ID，断线后继续查询。字段见[操作映射](developer-field-map.md)。

### 刷新与撤销

刷新向 `token_endpoint` 发送 form：

```text
grant_type=refresh_token
client_id, resource, refresh_token
```

可请求已获批 scope 的子集。刷新令牌会轮换，应原子替换整组令牌；并发刷新或旧令牌重用可能撤销授权族。

- 向 `revocation_endpoint` 发送 form `token,client_id` 撤销当前 grant。
- 浏览器 `GET https://mikiko.studio/api/v1/oauth/connected-apps` 返回已连接应用 `items`。
- 同路径 DELETE 使用 query `grant_id`，返回 `{revoked:true}`。
- 刷新或撤销不会登出所有浏览器会话；账号禁用和 grant 撤销逐次检查。

## 错误与恢复

OAuth 使用 `{error:string}`，不同于原生 REST 的嵌套 error 对象。

| 错误或状态 | 处理方式 |
| --- | --- |
| 401 `login_required` | 先登录 |
| 401 `invalid_session` | 重新登录 |
| consent 400 `authorization_request_expired` | 返回客户端重新连接 |
| `invalid_request`、`invalid_client_metadata`、`invalid_client` | 核对请求和注册信息 |
| `invalid_grant`、`invalid_scope`、`invalid_target`、`unsupported_grant_type` | 核对 grant、resource、scope、PKCE 和流程 |
| `access_denied` | 尊重拒绝结果，需要时重新请求授权 |
| `CANONICAL_URL_UNCONFIGURED` | 联系管理员检查可信公共端点配置 |
| 429 `temporarily_unavailable` | 按 `Retry-After` 等待当前分钟窗口后退避 |
| 动态注册 403 | 联系管理员确认是否允许注册 |
| 503 或网络故障 | 保留重试；不能仅凭 503 判断未配置 |

动态注册每分钟最多 10 次。其他 consent 400 不应笼统归为过期；token 400 应先修正请求，不无限重试。scope 不足会返回 tool error 和 `_meta["mcp/www_authenticate"]`。

## 部署与传输边界

可信端点可从 `GET https://mikiko.studio/api/v1/public/developer-endpoints` 的 `data` 读取：

| 字段 | 含义 |
| --- | --- |
| `api_origin` | 公共 origin |
| `rest_base` / `openai_base` | 原生 REST / OpenAI 地址 |
| `mcp_url` / `web_url` | MCP / 登录与授权页面 |
| OAuth 各 endpoint | 与 discovery 校验一致 |
| `protected_resource_metadata_url` | 受保护资源元数据地址 |
| `canonical_configured` / `disabled_reason` | 配置状态；未配时 URL 为空 |

- 自建部署使用管理员配置的可信 HTTPS canonical（loopback 开发例外），不从 Host、浏览器地址或内部 listener 推导 issuer。
- canonical 未配置时 OAuth 禁用；配置请求失败不能猜成未配置。
- `/oauth`、`/.well-known`、`/mcp` 必须返回协议响应，登录/授权页必须可达。
- 当前不支持 canonical 路径前缀，不任意替换主机名或 localhost/127.0.0.1。
- MCP 为 stateless POST，独立 GET/DELETE 返回 405，不依赖 session ID 或历史重放。
- 文件使用上传目标传输；平台生命周期 SSE 不是部分图片协议。

配置格式检查与真实客户端授权是不同验收项；完成连接后还需验证工具发现、刷新和撤销。
