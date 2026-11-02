# 技术设计：MCP OAuth localhost 回调端口豁免热修（Claude App 授权 400）

- 日期：2026-11-02
- 关联需求：`docs/plans/2026-10-28-mcp-client-compatibility-response-bounds.md`（MCP 客户端兼容性）
- 触发问题：用户在 Claude App 配置 MCP（`https://mikiko.studio/mcp`）授权时报
  `{"error":"invalid_request"}`

## 问题

Claude Code 的 client-id metadata 文档
（`https://claude.ai/oauth/claude-code-client-metadata`）注册的回调是
**无端口 localhost 主机名**形式：

```json
{"client_id":"https://claude.ai/oauth/claude-code-client-metadata",
 "redirect_uris":["http://localhost/callback","http://127.0.0.1/callback"], ...}
```

实际授权请求携带随机端口（如 `http://localhost:61228/callback`）。现有
`redirectURIMatches` 的 RFC 8252 端口例外仅接受 loopback **IP 字面量**
（`netip.ParseAddr`），`localhost` 主机名解析失败 → 精确匹配失败 →
`Begin` 返回 `invalid_request`（400）。Codex 使用 `127.0.0.1` 字面量，
恰好被既有实现覆盖，故此前未暴露。

线上验证：同一请求仅替换 redirect_uri，`127.0.0.1:61228` 形式返回 303
（进入授权确认），`localhost:61228` 形式返回 400，确认差异仅在回调匹配。

## 修复

`internal/service/oauthserver/clients.go`：端口豁免资格从「IP 字面量且
IsLoopback」扩展为「IP 字面量且 IsLoopback，或主机名恰好为 `localhost`」。
不变量保持：

- 注册与请求的 host 必须**完全一致**（`localhost` 不与 `127.0.0.1`/`[::1]`
  互换，`localhostx.example` 等前缀伪造不匹配）；
- 仅 `http` scheme；
- 除端口外其余部分逐字节一致（路径转义、query 顺序、空 query 标记、
  fragment/userinfo/无效端口仍全部拒绝）；
- 授权保存实际请求的 redirect_uri，code/token 兑换仍完整匹配实际值。

安全性：localhost 端口豁免不弱于 127.0.0.1 豁免（攻击者仍需既是已注册
client 又能在受害者 loopback 监听）；`parseRedirectURI` 继续只放行
http + loopback 主机，无新增 SSRF 面。与 GitHub 等主流 MCP 授权实现及
RFC 8252「请求时任意端口」意图一致。

## 测试

- `TestOAuthRedirectPortExceptionBoundaries`：localhost 无端口→带端口由
  `false` 改为 `true`；新增 host 不一致、https scheme 等负例。
- 新增 `TestOAuthClaudeLocalhostMetadataAuthorization`：以 Claude 真实
  metadata 形状（双回调注册）走 `Begin` 回归，含路径不一致负例。

## 交付门禁

`verify.sh`、`review-local.sh --scope committed`、`check-review-gate.sh`、
`api-smoke.sh`（后端变更）；打 tag 后远程升级部署并线上复测 303。
