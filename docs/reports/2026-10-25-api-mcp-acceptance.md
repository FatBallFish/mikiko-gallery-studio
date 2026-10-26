# API Key 与 MCP 功能实测报告

日期：2026-10-25。真实 API：`http://127.0.0.1:8091`。结论：多数查询、权限和资源生命周期能力正常，但当前本地部署不能判定为全部正常。

## 测试结果

| 能力 | 实际结果 |
|---|---|
| 账号与余额 | Bearer API Key 和 OAuth MCP 真实查询通过。 |
| API Key 生命周期 | 创建、scope 限制、无效密钥、轮换、停用/启用、过期、删除后失效均通过；本次完整/受限 Key 已删除并确认 401。 |
| OpenAI 兼容 | 真实 `/v1/models` 和错误参数拒绝通过。同步、SSE、编辑含 mask 的自动化测试通过。真实成功生成未执行：当前无 Worker，避免留下额外不可取消的付费图片任务。 |
| 图片任务 | 真实合法预校验通过，最低测试报价 8 积分；历史列表、详情、事件、结果访问和下载通过。真实新任务创建到完成受 Worker 缺失阻塞。隔离假上游创建、Worker 成功及 SHA256 下载校验通过。 |
| 视频任务 | 真实 5 秒 480p 任务创建成功；列表、详情、事件与已有成功结果下载通过。新任务没有 Worker 执行。取消请求成功，但仍 cancel_requested、258 积分冻结，未完成取消结算。 |
| 历史与资产 | 真实图片/视频列表、详情、访问链接、完整下载和 Range 通过；下载校验了实际媒体签名及字节数，不以 HTTP 200 作为唯一依据。 |
| 项目与分组 | 仅本次测试数据的 CRUD 通过。 |
| 上传与资产写入 | REST 初始化、查询、代理分片 PUT、完成、终止和新资产更新/删除通过。获取分片签名目标失败，本地存储返回 500。 |
| Mikiko 实际安装 | 已通过设置界面创建 HTTP MCP 配置，但实际连接/授权失败，resource 地址不匹配。不能视作安装可用。 |
| MCP 直接协议 | 35/35 工具实际调用：31 通过、1 失败、3 成功路径受阻。5 个协议版本和官方 Go SDK Connect/ListTools/get_balance 通过。 |
| OAuth 生命周期 | scope 不足拒绝、refresh 轮换、旧 refresh 重放拒绝、family 撤销通过；本次授权已撤销，access 401、refresh 400。 |

REST 业务操作矩阵为 35 项：33 通过、1 失败、1 阻塞；另有 3 项 REST transport/events 扩展通过。请求级检查含正常响应和预期拒绝，最终统计以 JSON 为准。操作被调用不代表所有成功路径已完成。

## 发现的问题与阻塞

1. **MCP 规范公开地址与实际路由不一致。** API 8091 的 OAuth discovery 声明 issuer `http://localhost:5273`、resource `http://localhost:5273/mcp`。Mikiko 连接 `http://127.0.0.1:8091/mcp` 后显示：`Protected resource http://localhost:5273/mcp does not match expected http://127.0.0.1:8091/mcp (or origin)`。5273 对 discovery/MCP 返回前端 HTML。直接协议测试显式绕开此发现错误，不证明 Mikiko 客户端授权成功。
2. **本地存储分片签名目标失败。** REST `/uploads/{id}/parts/{part}/url` 返回 500；MCP `get_upload_part_url` 返回工具错误。REST 代理 PUT 可用；用网页会话代理补传后 MCP complete 成功，不构成纯 MCP 上传闭环。
3. **当前没有 Worker。** 新图片、视频、兼容生成的真实上游执行与结算无法闭环。本次未启动共享 Worker，避免处理其他历史排队任务并产生费用。
4. **本次视频取消仍待结算，258 积分冻结。** 任务 `86764a67-e2da-4367-ba8e-e877e7849753`；取消请求 200，状态 cancel_requested，当前 available 270 / frozen 258，初始 available 528。不能称退款成功。关联测试项目保留，未手工改账。
5. **视频能力展示与完整预校验存在差异。** 列表展示的 720p 组合在预校验时被候选能力拒绝；480p 5 秒实际报价 258，展示最低价 10 不能作为该完整请求报价。

## 验证层次

- 真实 REST：97 项请求检查，82 pass、13 expected_rejection、1 blocked、1 fail。
- 真实直接 MCP：35 工具，31 pass、1 fail、3 blocked；成功创建图片/视频、成功取消结算未测。
- 现有隔离 API smoke：exit 0，假 provider、独立 API/Worker/PostgreSQL/Redis，结束后已清理；未代替真实部署结果。
- 5 个服务包回归及 15 项 OpenAI/共享应用 HTTP 测试通过。
- 不包含真实上游图片/视频生成成功验收、所有模型/参数排列、跨用户/额度耗尽/速率耗尽的完整实测。

## 清理与残留

本次测试 Key 和 OAuth grants 均已失效；新建分组、额外空项目、上传资产已清理，未操作既有数据。保留取消待结算的视频及其项目，以及 Mikiko 中失败安装项 `mikiko-studio-local-acceptance`。本次只测试和输出报告，没有修复业务代码、变更部署配置、重启服务或充值。

## 详细证据

- [真实 API Key 请求与操作矩阵](2026-10-25-api-mcp-api-key-results.json)
- [真实 MCP 协议与逐工具记录](2026-10-25-api-mcp-protocol-results.json)
- [Mikiko 安装现场](2026-10-25-api-mcp-mikiko-install.md)
- [隔离假上游冒烟](2026-10-25-api-mcp-isolated-smoke-results.json)
- [自动化回归补充](2026-10-25-api-mcp-automated-tests.md)

公开报告已去除密钥、访问令牌和签名下载能力；私有临时证据不应提交。
