# 错误与恢复

## 按协议读取错误

原生 REST、OpenAI 兼容接口和 OAuth 的错误封装不同，按对应接口定义解析。

- 保留请求 ID、任务 ID 和错误码。
- 参数、权限、账号或配额问题需要先修正。
- 暂态故障采用有上限的退避重试。

## 常见处理方式

| 状态 | 处理 |
| --- | --- |
| 401 | 检查 SK、过期时间和账号状态；MCP 重新授权 |
| 403 | 检查 scope 和资源归属 |
| 402 | 检查余额 |
| 409 | 刷新版本或重新预检；幂等冲突需核对业务输入 |
| 429 | 按实际 `Retry-After` 等待后退避 |
| 5xx | 有上限重试；创建请求保持幂等键 |

## 进一步定位

- [原生错误与调用点](developer-field-map.md)
- [OAuth 错误分类](mcp.md)
- [各操作完整定义](../../web/docs/public/openapi/openapi.yaml)

同步生成遇到 `504 TASK_WAIT_INTERRUPTED` 时，使用 `X-Task-ID` 或 `Location` 查询持久任务。
