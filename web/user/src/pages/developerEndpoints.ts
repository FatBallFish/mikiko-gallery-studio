import type { DeveloperEndpoints } from '../../../shared/api-types'

export function developerEndpointRows(config: DeveloperEndpoints) {
  return [
    ['Origin（无 API 路径）', config.api_origin],
    ['原生 REST BaseURL', config.rest_base],
    ['OpenAI BaseURL', config.openai_base],
    ['MCP Streamable HTTP', config.mcp_url],
  ] as const
}

export function endpointDisabledReason(config: DeveloperEndpoints) {
  if (config.canonical_configured) return ''
  if (config.disabled_reason === 'CANONICAL_URL_UNCONFIGURED') return '部署尚未配置 canonical URL，无法提供可信的绝对 BaseURL 与 OAuth 发现地址。站内 REST 可能仍可用；请管理员配置公共 HTTPS 地址后重试。'
  return `开发者端点不可用：${config.disabled_reason || '公共地址配置无效'}。请联系管理员检查 canonical 配置。`
}

export function developerClientTemplates(config: DeveloperEndpoints) {
  if (!config.canonical_configured || !config.mcp_url) return []
  const url = config.mcp_url
  // JSON string quoting is also valid for TOML basic strings used here.
  return [
    { name: 'Mikiko / ZCode', format: 'JSON · .mikiko/config.json', code: JSON.stringify({ mcp: { servers: { studio: { type: 'http', url } } } }, null, 2), instructions: '在设置 → MCP 添加 HTTP 服务，或合并到工作区配置。重启会话，在 MCP 状态中完成客户端提供的浏览器授权；若版本未提供 OAuth 登录入口，请升级客户端。配置中不填 API Key 或手工 token。' },
    { name: 'Codex', format: 'TOML · ~/.codex/config.toml', code: `[mcp_servers.studio]\nurl = ${JSON.stringify(url)}`, instructions: '合并 TOML 后执行 codex mcp login studio，完成浏览器登录与同意授权；使用 codex mcp list 检查配置。' },
    { name: 'Claude Code', format: 'CLI · 终端执行', code: `claude mcp add --transport http studio '${url.replace(/'/g, "'\\''")}'`, instructions: '在 Claude Code 输入 /mcp，选择 studio，按提示 Authenticate 完成浏览器 OAuth。可通过 claude mcp list 检查配置。' },
  ]
}
