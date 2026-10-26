# MCP、可信端点与浏览器 OAuth

先从密钥页读取 `GET /api/v1/public/developer-endpoints`（正常浏览器 API envelope 的 `data`）。所有绝对 URL 都由可信运行配置投影；不得由 Host、`window.location` 或内部 API listener 推导 issuer。

| 字段 | 用法 |
| --- | --- |
| api_origin | 公共 origin，无 API 路径 |
| rest_base | 原生 REST BaseURL，包含 `/api/open/v1` |
| openai_base | OpenAI SDK BaseURL，包含 `/v1` |
| mcp_url | Streamable HTTP 端点，包含 `/mcp` |
| web_url | 浏览器登录/consent 所在公共 Web 地址 |
| authorization_endpoint, token_endpoint, registration_endpoint | OAuth 对应阶段；以 discovery 校验，不自行改域名 |
| protected_resource_metadata_url | protected resource metadata |
| canonical_configured, disabled_reason | canonical 未配置时 false、`CANONICAL_URL_UNCONFIGURED`，所有 URL 为空 |

空 canonical 时站内相对路径 REST 仍可能可用，但无法发布可信绝对 BaseURL 或开启 OAuth。联系管理员配置公共 HTTPS canonical（loopback 开发例外），不能把 `https://api.example.com`、当前浏览器地址或某个历史 localhost 端口冒充实际部署地址。API-only 域名还必须能承载浏览器登录/consent，代理的 `/oauth`、`/.well-known`、`/mcp` 必须返回协议响应而非 SPA HTML。当前实现不支持 canonical 路径前缀；不要自行在反向代理增加前缀或替换 localhost/127.0.0.1。

## 客户端配置（无 token）

以下 `https://studio.example.com/mcp` 仅为占位，使用密钥页实际复制的 MCP URL。配置中不包含 API Key、浏览器令牌、access token 或 refresh token。

### Mikiko / ZCode

合并至工作区 `.mikiko/config.json`（兼容名 `zcode.json`）或用户 `~/.mikiko/cli/config.json`：

```json
{
  "mcp": {
    "servers": {
      "studio": { "type": "http", "url": "https://studio.example.com/mcp" }
    }
  }
}
```

在设置 → MCP 添加/检查该 HTTP 服务，重启受影响会话。使用客户端提供的浏览器授权入口登录并同意权限，再检查 tools。已安装官方配置插件确认此 JSON 格式与严格字段规则；不能添加猜测的 `oauth` 配置键，不能把插件专用 `${...}` 插值复制进普通配置。用户同名服务覆盖工作区服务。若当前版本没有浏览器 OAuth 操作入口，升级或确认该版本支持，不能以手工 token 模板假装自动授权已验证。

### Codex

合并至 `~/.codex/config.toml`，这是 TOML：

```toml
[mcp_servers.studio]
url = "https://studio.example.com/mcp"
```

执行 `codex mcp login studio`，完成浏览器登录与授权；`codex mcp list` 检查配置。依据 [OpenAI 官方 MCP 文档](https://developers.openai.com/codex/mcp)（当前重定向至 [官方学习站](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)），HTTP OAuth 不需要预填 bearer token。不要把 JSON 当 TOML，也不要把 MCP URL 填到模型 provider BaseURL。

### Claude Code

终端执行：

```sh
claude mcp add --transport http studio https://studio.example.com/mcp
```

在 Claude Code 输入 `/mcp`，选择 studio，按 Authenticate 提示完成浏览器授权；`claude mcp list` 检查配置。依据 [Claude Code 官方文档](https://code.claude.com/docs/en/mcp)。Claude 桌面产品的连接器与 Claude Code CLI 配置入口不同，本教程只声明 Claude Code 格式。

## OAuth 逐阶段流程与字段

1. 未认证 MCP POST 收到401与 `WWW-Authenticate`，读取 challenge 的 resource metadata URL。受保护元数据的 `resource` 是精确 canonical `/mcp`，`authorization_servers` 给出 issuer；还有 `scopes_supported`、`bearer_methods_supported:["header"]`。
2. 获取 `/.well-known/oauth-authorization-server`。返回 issuer、authorization_endpoint、token_endpoint、revocation_endpoint、registration_endpoint、response_types_supported、grant_types_supported、token_endpoint_auth_methods_supported、code_challenge_methods_supported、scopes_supported、client_id_metadata_document_supported。检查 JSON/issuer；HTTP200 HTML是代理故障。
3. 根据部署策略注册公共客户端：POST registration_endpoint JSON `{client_name,redirect_uris,token_endpoint_auth_method:"none",grant_types:["authorization_code","refresh_token"],response_types:["code"]}`。201返回客户端元数据并增加 client_id；不发 client_secret。支持已配置客户端与经过校验的 Client ID Metadata Document；动态注册可能被关闭、限速或受活动客户端容量限制。
4. 客户端生成随机 state、PKCE verifier及其S256 challenge。浏览器 GET authorization_endpoint，query为 `client_id,redirect_uri,resource,response_type=code,scope,code_challenge,code_challenge_method=S256,state`。redirect_uri须精确注册，resource须精确匹配，scope用空格分隔。
5. 服务器303到 `/?oauth_request=...#/oauth-consent`。使用 Studio 本地登录或 OIDC，不向 MCP 发送密码。浏览器会话调用 GET `/api/v1/oauth/consent?request_id=...`，返回 `{request_id,client:{client_id,client_name,redirect_uris,...},resource,scopes,csrf_token}`。POST同路径 JSON `{request_id,csrf_token,allow}` 返回 `{redirect_uri}`。请求过期须从客户端重来；不要修改请求ID/回调。浏览器登录保留 oauth_request。
6. 客户端校验回调 state，接受 code；拒绝授权返回 OAuth error。向 token_endpoint POST `application/x-www-form-urlencoded`：`grant_type=authorization_code,client_id,resource,code,redirect_uri,code_verifier`。不带浏览器 Authorization 或 client secret。返回 `{access_token,token_type:"Bearer",expires_in,refresh_token,scope}`；当前访问令牌600秒，不能依赖永久有效。
7. 客户端以 `Authorization: Bearer` 使用令牌调用 MCP initialize，后续发送协商的 MCP-Protocol-Version并遵循SDK初始化通知，再 tools/list/tools/call。任务创建返回持久ID，断连后继续按ID查询。工具结果 `structuredContent:{data:...}`，文本JSON同包装；操作字段见[字段映射](developer-field-map.md)和文档站 `#/reference`。
8. 刷新使用 form POST token_endpoint：`grant_type=refresh_token,client_id,resource,refresh_token`，可请求已获批权限子集scope。刷新令牌轮换，安全替换整组新令牌；并行刷新/旧token重用可能撤销授权族。POST revocation_endpoint form `token,client_id` 撤销当前grant。浏览器已连接应用 GET `/api/v1/oauth/connected-apps` 返回 `{items:[{id,client_id,client_name,scopes,created_at}]}`；DELETE query `grant_id` 返回 `{revoked:true}`。

## 错误、重试与安全边界

OAuth端点错误是 `{error:string}`，与原生REST `{error:{code,message,chargeable,next_suggestion?,details?},meta:...}` 不同。实际OAuth枚举包括 `invalid_request`、`invalid_client_metadata`、`invalid_client`、`invalid_grant`、`invalid_scope`、`invalid_target`、`unsupported_grant_type`、`access_denied`、`login_required`、`invalid_session`、`authorization_request_expired`、`CANONICAL_URL_UNCONFIGURED`、`temporarily_unavailable`；动态注册每分钟最多10次，429 `temporarily_unavailable`按Retry-After等待当前固定分钟窗口恢复后退避重试，403关闭注册应联系管理员。token400先检查grant/resource/PKCE，不无限重试；503是暂时服务故障，不能一律说未配置。无凭据401 `login_required`先登录，有无效凭据401 `invalid_session`重新登录；只有consent400 `authorization_request_expired`表示过期；其他400表示无效请求，不得笼统归类过期，返回客户端修正后重新连接；网络错误保留重试。

未配置以可信public config为准。consent/已连接应用只凭503无法区分暂态故障与canonical未配，页面先读取配置再请求业务接口。配置请求本身失败不能猜成未配置。refresh/revoke不会登出所有浏览器会话，账号禁用和grant撤销仍逐次检查。scope不足返回tool error及 `_meta["mcp/www_authenticate"]`。API Key不替代MCP OAuth token，OAuth token不替代浏览器登录，也不转发给任意下游资源。

MCP stateless POST，独立GET/DELETE返回405；不依赖session ID或历史事件恢复。读取 initialize.protocolVersion，不猜测协议兼容。文件用上传目标传输，不把大型base64塞进tool，不宣称支持任意远程URL导入。平台生命周期SSE不是partial images。

本轮仅复核官方配置格式和本地协议/界面逻辑，尚未在 Mikiko/ZCode、Codex、Claude Code 完成真实浏览器授权、刷新、撤权与工具调用验证；不能将模板生成或HTTP200当作客户端接入成功。
