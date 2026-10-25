# API Key / MCP 验收问题修复结果

## 已修复

- Vite 公开入口转发 MCP、OAuth discovery、authorize/token/register/revoke，保留规范 issuer/resource 校验。`http://localhost:5273/mcp` 为本地 MCP 客户端地址，8091 为内部 API。
- OIDC 返回恢复同源完整 URL，保留 `oauth_request`，避免只恢复 hash 后丢失授权请求。
- local storage 获取上传目标改为短期 HMAC capability；绑定主体/凭据代际或 grant、会话、分片、方法、路径、大小、checksum 和过期。目标上传不需要原 API Key/OAuth token，S3 仍直签。撤权后旧目标拒绝。
- 完成期间上传会话保持不可写，查询/旧分片记录不能把 completing 回退为 uploading；错误完成信息可用正确分片重试。
- 视频公开组合与 preflight 共用正向分辨率映射，移除映射后不可路由的组合。
- 无 attempt 且无有效 worker lease 的 queued/cancel_requested 视频同步取消，在同一事务内释放钱包与 API Key quota；保留已提交/有有效 lease 的异步取消。
- REST/MCP 共享层将项目非空、版本冲突、默认项目保护等业务错误映射为明确 4xx，不再 generic 500。

## 真实复验

详见 [去敏请求记录](2026-10-25-api-mcp-remediation-retest.json)。

- 旧视频任务 `86764a67-e2da-4367-ba8e-e877e7849753` 正常 cancel API 返回 202，task cancelled、settlement finalized、actual 0；余额 available 270/frozen 258 → available 528/frozen 0。重复取消余额不变。
- API Key 与 OAuth MCP 均完成 create upload → target → 无原凭据 PUT → complete → 下载；69 字节 PNG 字节及 SHA256 相同。
- canonical discovery、PKCE/consent/token、35 MCP tools、get_balance 通过；OAuth revoke 后旧目标 403/MCP401，Key 删除后旧目标被拒绝。
- discovery 不再展示错误720p；480p preflight200，1080p返回余额不足而非分辨率不支持。
- 临时项目不带target删除返回409 `project_not_empty`（tasks0/assets1）；指定已确认默认项目后200，源项目删除、再GET404。未知pending session正常转移，未执行abort，不称已清理该session。
- 本次测试Key已删除，OAuth授权已撤销，最终 available528/frozen0；未发起额外收费生成，未启动共享Worker，未手改数据库账务。

## 验证

- 仓库 `verify.sh` 通过，含Go tests/vet、前端契约/typecheck/build及原生包契约。
- 隔离 `api-smoke.sh` 通过，exit0。
- 全量后新增项目错误映射，完整 `internal/service/openplatform` 和 `internal/http/router` 两包再次测试通过。
- 视频取消race、claim竞争与账务幂等、上传安全边界、真实Vite流式转发聚焦测试通过；独立只读复核无剩余阻塞。
- `git diff --check` 通过。

## 限制与运行状态

本地API8091已加载 `/tmp/e2e-api-mcp-remediated-final`，使用原 `/tmp/e2e-config/runtime.env`，公开入口仍为localhost5273。没有生产部署、commit或push。

Mikiko桌面会话在验收过程中锁定，Computer Use先返回 `requires an unlocked session`，随后 `disposed`。因此本轮没有完成在Mikiko设置界面把旧 `http://127.0.0.1:8091/mcp` 改为 `http://localhost:5273/mcp` 并重新授权的桌面验收；真实canonical OAuth/官方协议调用已验证，不能冒称Mikiko客户端已连接成功。

本地流程门禁仍为BLOCK：heavyweight任务 `approval.status` pending，限制push/PR；已完成修复、测试与独立代码复核，未绕过审批标记。
