# 鹈鹕阳台单图 MCP 实测（2026-10-25）

结果：独立协议客户端经正常 PKCE S256 OAuth 授权，通过 `http://localhost:5273/mcp` 的 Streamable HTTP MCP 成功生成且下载恰好 1 张图片。未读取 Mikiko 私有凭据；未修改源代码、coding context 或直接写数据库；本执行代理未启动/停止服务。

## 授权与调用边界

- GUI 安装连接验证：据主会话现场验证，Mikiko GUI 已完成真实 OAuth 授权，MCP 列表显示绿点。
- 本次生成验证：使用独立新建 OAuth client/grant，发现授权服务、动态注册、登录、授权同意、authorization code + PKCE S256 换取令牌，再 initialize/initialized 和调用 MCP 工具。与 GUI 授权凭据独立。
- 当前宿主不能热加载此 MCP；GUI 原生输入框 AX 写入未生效，未从 GUI 提交任务。因此本记录不能证明 GUI agent 已调用生成工具成功。
- 实际调用：`get_balance`、`list_image_models`、`list_projects`、`preflight_image_task`、`create_image_task`、持续 `get_image_task`、`get_image_result_access`、最终 `get_balance`。
- 凭据及签名访问链接仅存本次独立临时私有目录，状态文件权限 0600，不写入本报告。完成后仅撤销本次新建 OAuth grant；撤销后 MCP `get_balance` 返回 HTTP 401，未撤销 GUI grant。

## 唯一生成任务

| 字段 | 实测值 |
| --- | --- |
| task_id | `6eda934c-0ef4-5f91-93e2-eee2d7b61c42` |
| idempotency_key | `pelican-balcony-mcp-20261025-single-01` |
| route_model_code | `gpt-image-salf` |
| project | 既有默认项目 `9714b638-1b56-4bbc-bd81-3f3a02a9ad21` |
| 单图参数 | 图像工具实际 schema 为 `output_image_count: 1`，响应 `requested_output_image_count: 1` |
| 请求 | text_to_image，pixel，1024x768，PNG，quality/background/moderation auto |
| 终态 | `succeeded` / `completed`，`results` 恰好 1 项 |
| 预估 / 实际费用 | `8.00000` / `8.00000` 积分 |
| 余额 | 生成前 `528.00000`，生成后 `520.00000`，最终冻结 `0.00000` |

提示词：“一只鹈鹕在住宅阳台安静睡觉，清楚羽毛与长喙，无文字。写实摄影，自然日光，真实羽毛细节，安静温暖的住宅阳台环境。”

`create_image_task` 仅调用一次，无第二任务，无收费生成重试。任务创建后立即将 ID、预估积分和非敏感参数写入 `/tmp/pelican-mikiko-task.json` 并通知主会话。主会话随后管理 Worker 启停。

## 下载与核验

- 文件：`docs/reports/artifacts/pelican-balcony-mcp-20261025.png`
- 下载通过 MCP `get_image_result_access` 获取访问地址，无需读取 GUI 凭据。
- 文件大小：2,944,258 bytes。
- PNG 魔数：`89504e470d0a1a0a`。
- IHDR 实际尺寸：**1448×1086**；与请求 1024×768 不一致，未缩放或改写原结果。
- SHA-256：`f438b88ecfb4eb4d34b25facaadb95490c48b1db0ec9c7e013f2a9268625f092`。
- 视觉检查：一只闭眼休息的鹈鹕坐在住宅阳台软垫椅上，羽毛及长喙清楚，背景有栏杆与盆栽，温暖自然日光，无可见文字，符合题材。

## 异常与运行副作用

1. 首次免费 preflight 因 pixel 模式同时带 `base_resolution` 被拒（“pixel size_mode does not accept ratio fields”）；删除该字段后 preflight 成功，随后才唯一一次创建，无收费重试。
2. 成功 preflight 的估价为 8、余额为 528，但 `sufficient: false` 且 `insufficient_points` 为空；create 实际成功，最终扣费与估价一致。此字段不一致如实保留。
3. 据主会话 Worker 日志：首次使用修改角色/并发的配置副本启动被 setup binding 校验拒绝，未执行生成；随后使用原已绑定 runtime 配置启动，实际为全角色 Worker，不能声称是 image-only 单并发运行。
4. 据主会话检查，全角色 Worker 还处理了 4 个历史 media job；因原对象不存在，各达到 max5 并终态失败。这是本次服务运行的既有媒体作业副作用，不能声称 Worker 仅处理本张图片。本生成代理未直接写库，也未触发第二图片生成。
