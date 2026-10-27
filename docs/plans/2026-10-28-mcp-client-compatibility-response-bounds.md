# MCP 客户端兼容与有界响应

## 用户需求与来源

2026-10-28 用户报告 Codex MCP OAuth 授权 invalid_request，并要求优化线上约 927 KB 的模型列表及检查、处理其他 MCP 接口，避免超过客户端单次 50 KB 读取限制。沿用 docs/plans/2026-10-24-unified-api-mcp-design.md 的共享服务、OAuth 安全与异步任务约束。本轮完成代码、验证、评审及隔离冒烟。随后用户明确授权整理提交、推送、创建新 tag、打包并通过 mgsctl 更新远端服务器；使用 v0.0.47 新版本，不改写已有标签，不合并 main。

## 已确认的授权兼容问题与设计

公开 Codex metadata 使用 native loopback 回调，无显式端口；实际请求使用随机本地端口。现 Begin 对 redirect_uris 完整字符串匹配，因此确定不兼容。默认仍完整匹配，仅 HTTP loopback IP literal 支持 RFC 8252 的端口例外；host、scheme、路径、query 和其他 URI 部分必须一致，IPv4/IPv6 不互换，拒绝 userinfo、fragment、无效端口。保存授权请求实际 redirect_uri，code/token 兑换仍完整匹配实际值。保留 HTTPS metadata 获取、SSRF、PKCE S256、资源、scope、single-use code 和刷新绑定。不得重放用户请求或记录 state/challenge/token。

## MCP 响应设计约束

客户端限制按 UTF-8 序列化后计算；预算涵盖 text 与 structuredContent 的重复数据、元数据与 JSON 包装，预留低于 50 KB 的余量（目标完整工具响应不超过 40 KiB）。检查 tools/list 以及全部 tools/call 结果。

模型默认给出供选择的紧凑摘要，通过按需详情或字段区段获得生成所需约束、能力与计价信息；不得静默丢失数据或令 agent 凭空猜测参数。列表使用有界分页，continuation 必须对应实际消费的数据，避免漏项、重复或分页前权限过滤错误。兼容现有 REST 响应，不给 REST 强行裁剪。

大详情采用明确可继续读取的字段/区段，不截断任意 JSON 字符串，不返回损坏 JSON。避免输出内嵌 binary/base64，通过既有资产访问机制提供内容。错误文本和超长用户字段同样必须有界，并明确是否省略及如何获取。

写入和付费操作执行后，返回持久操作 ID 和精简成功结果；不得仅因格式化超限将已成功操作变成可重试失败，不能自动重提付费任务。沿用授权、幂等、报价、积分和跨用户隔离语义。

## 实施记录

OAuth 已完成端口例外及统一回调合法性校验，token 阶段仍绑定实际 URI。合成 metadata 驱动服务流程覆盖 IPv4/IPv6，OAuth 包及 race/vet、HTTP handlers 均通过，独立复核通过。新增 router → handler → service 集成回归：无端口注册配合 OS 临时端口返回 303 consent，path/query 变化返回 400 invalid_request。未重放用户授权参数、未调用生产授权或 token 端点。

MCP 实施限定适配层，保持共享 REST 语义。正常小结果继续返回原结构，大结果使用受限不可变快照及专属只读工具继续读取；快照绑定当前 user/client/grant/group/credential/scopes，沿用原工具 scope，不新增 OAuth 权限。固定两分钟 TTL，进程内总量、单主体、单条字节和条目上限；重启或多副本未命中必须明确报失效。生成成功的回复必须保留成功事实和持久 ID、后续查询工具，缓存不足不得建议重提生成。URL 与报价自身到期时间不会因快照延长。超出单条容量的读结果须明确缩小查询范围，不承诺无限缓存或无限无损。

## 验证实测

合成真实 DTO（经原业务 schema 预校验）的 SDK 黑盒测试：37 工具分 8 页，目录最大完整 HTTP 响应 9,389 bytes；图片模型原 DTO 950,553 bytes，摘要完整 CallToolResult 1,091 bytes；视频模型原 DTO 949,648 bytes，摘要 1,143 bytes。两者详情通过 read_tool_result 按原路径无损恢复。中文字符串 480,000 UTF-8 bytes 分 59 页，最大完整结果 31,920 bytes。上述结果含文本与结构化内容双份，不代表读取了生产完整数据或真实 Codex 安装验收。

六组独立 SDK acceptance、缓存隔离/TTL/容量/数字精度/JSON Pointer/UTF-8 边界及原生 fake-provider 生命周期已通过；OAuth 服务/race/vet/HTTP handler 和 router 303/400 回归通过。隔离 API smoke、完整 verify 与独立评审通过。发布阶段重新执行 ship guard，并核验正式 Actions、manifest、安装包和镜像摘要后，以匹配新提交的 mgsctl 完成备份、排空、升级及上线检查。

## 验证

OAuth 正负矩阵涵盖无端口注册到随机端口、IPv6、host/path/query 不匹配、远程 URL 端口变化拒绝、token 阶段不同端口拒绝。MCP 大数据夹具覆盖 UTF-8 中文、嵌套 schema、单条超大数据、跨页无遗漏、默认与详情预算、tools/list、成功写入大结果无重试诱导；检查所有工具目录。完成 Go focused、全仓 verify、独立评审、隔离 API smoke，不使用真实凭据或新增付费任务。
