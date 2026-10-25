# API Key 与 MCP 实测：Mikiko 客户端安装记录

- 日期：2026-10-25
- 环境：本机 Mikiko 桌面应用 `dev.mikiko.app`，项目 API `http://127.0.0.1:8091`
- 测试方式：实际操作 Mikiko 设置 → MCP 服务器 → 新建 → HTTP；未通过修改配置文件代替安装。
- 安装名称：`mikiko-studio-local-acceptance`
- URL：`http://127.0.0.1:8091/mcp`
- 安装结果：保存成功，Mikiko 列表显示已安装 1 个自定义服务器，开关开启。
- 连接结果：失败，界面显示“ MCP 授权未完成或已超时，请重新授权。”
- 错误详情：`Protected resource http://localhost:5273/mcp does not match expected http://127.0.0.1:8091/mcp (or origin)`。

## 环境证据

API 8091 的健康检查、就绪检查以及 OAuth discovery 返回正常 JSON，未认证 MCP 返回 401 challenge。但 discovery 中声明的 issuer/resource 是 `http://localhost:5273` / `http://localhost:5273/mcp`；该端口为 Vite 前端，其 discovery 与 `/mcp` 返回 SPA HTML。

这是本机部署的规范公开地址与实际路由不一致。不能把保存配置成功视为 OAuth 成功，也不能把直接协议调用结果视为 Mikiko 客户端调用成功。本次不修改配置、不重启服务；保留失败安装项供排查。

API Key 及直接 MCP OAuth/工具测试分别记录在相邻 JSON 报告，凭据不写入报告。
