# API Key / MCP 自动化测试补充证据

日期：2026-10-25。此文件只记录自动化测试，不能替代真实部署或 Mikiko 客户端验收。

## 服务级回归

命令：

```bash
GOMAXPROCS=2 go test -p=2 ./internal/mcpserver ./internal/service/oauthserver ./internal/service/openplatform ./internal/service/compat ./internal/service/apikey -count=1 -timeout=180s
```

结果：全部 5 包通过，exit 0。

## OpenAI 兼容与共享应用 HTTP 集成

命令：

```bash
GOMAXPROCS=2 go test -p=2 ./internal/http/handlers ./internal/app ./internal/service/compat -run 'TestCompatLifecycle|TestConfigureDeveloperServicesHTTPIntegration|TestOpenAICompat' -count=1 -v
```

结果：15 项测试通过，exit 0。覆盖 SSE 最终事件结构、同步 OpenAI 响应结构、断线不取消任务、配置开发者服务的 HTTP OAuth/SDK 集成、图片参数/模型路由、图片编辑含 mask、余额配额与无效 Bearer 拒绝。

日志：`/tmp/api-mcp-focused-20261025.log`、`/tmp/api-mcp-compat-http-20261025.log`。

## 限制

测试使用内部 fake/stub provider 或隔离测试环境，不证明 8091 的真实上游图片/视频执行成功。真实环境没有 Worker，Mikiko OAuth 发现地址错配，分别见真实 API 和 Mikiko 安装记录。
