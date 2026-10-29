# MCP 图片预检余额与预览可靠性修复

## 用户需求与授权

2026-10-28 用户提供 Codex 两组脱敏诊断，要求分析图片预览显示问题，与已确定的图片预检余额字段遗漏一起修复，完成验证后推送、打新 tag、打包并通过 mgsctl 部署服务器。沿用 docs/plans/2026-10-24-unified-api-mcp-design.md 与 docs/plans/2026-10-28-mcp-client-compatibility-response-bounds.md 的授权、付费任务幂等、响应预算及无密钥泄露约束。不改写已有标签，不合并 main，不重提付费生成任务。

## 事实与分析边界

余额 incident：成功预检报价5.00000，却输出 sufficient=false / insufficient_points为空，随后余额2257.00000。代码已纯内存复现：共享图片预检余额校验成功后未填EstimateResult，泄漏Go默认值；浏览器估价独立补全正常。修复共享服务路径，同一次余额快照产生校验与返回字段，保持不足402、keyquota、quote和create语义。

预览 incident：原R2预览、下载、刷新服务下载链接在诊断时均200 image/png，3260166 bytes，同SHA256，PNG1254×1254已本地成功解码。首次curl143没有HTTP结果，远程Markdown显示失败无客户端网络证据；不得声称图片坏、签名错、链接过期或CORS已证实。已知远程Markdown不能保证Codex渲染，下载attachment不等于inline preview。实施以检查确认的服务预览契约缺陷及可靠接入改善为准，不伪称已修客户端内部渲染器。

## 设计约束

图片预检公开DTO应准确反映同次余额，sufficient=true且差额零，必要余额摘要一致；测试充足、刚好足、不足、冻结与APIKey限制，REST/MCP共享行为。

预览使用既有资产授权与真实MIME，保留下载与预览用途区别，不能泄露存储凭据、不能新增匿名永久公开或绕过撤销。MCP响应仍小于40KiB，不能直接内嵌3.2MB原图。必要时使用有界缩略图/合规resource link及明确的客户端下载查看指引，任何新增ImageContent必须计入最终序列化预算并测试实际解码尺寸与hash；链接本身可达不能宣称所有客户端均已显示。

预览修复采用既有 get_image_result_access 的 purpose=preview 附加 MCP ImageContent 缩略图。MCP Config 提供内部授权读取回调，由应用装配连接既有 assetaccess 内容流，仅处理已经生成并校验的 canonical capability，不发 HTTP 请求任意外链。保留原 access 元数据；download 不附图片。读取限时、源数据最多16MiB、解码前像素上限32MP；复用 x/image 缩放，JPEG缩略图最长边256起，自适应降低质量/尺寸至raw最多18KiB，image只放Content一次，最终整个MCP响应仍小于40KiB。失败返回明确preview不可用信息及原访问方式，不将已成功操作变为错误，不重生成，不声称缩略图等同原图或所有客户端必能显示。

## 实施与 focused 验收

共享图片预检从 checkFunds 取得同一次 BalanceSummary，充足时填 sufficient=true、insufficient_points=0.00000 和 balance；覆盖2257/5、刚好足、不足、冻结、bucket摘要、quota拒绝及非法余额，真实REST/SDK双结果回归，余额只读取一次。视频继续使用原 MaxReservedPoints 检查。

预览采用256px白底有损JPEG；内部canonical capability严格绑定当前principal和preview用途，复用签名、audience、到期、撤销、资产校验。读取取消时关闭流，解码前后验证像素；标准库解码CPU不支持硬取消，此限制在接入文档明确。真实SDK HTTP/SSE响应16,875 bytes，JPEG12,215 bytes、256×256，image仅一次，往返SHA一致；预览/授权race及focused包回归通过。原始远程Markdown渲染失败的客户端根因仍未知，不将缩略图可靠接入误称客户端内部渲染器修复。

## 验证与交付

focused单元/HTTP/官方SDK实际封装、最终wire预算、图片bytes签名/尺寸/hash、授权跨用户/过期/撤销、独立review、全verify、隔离API smoke。真实provider不新增付费任务。生产发布要求新提交和新版本matching mgsctl、正式Actions/manifest/所有包与镜像摘要通过，freshbaseline/排空/完整可恢复备份/官方升级/doctor/health/schema/canonical/MCP及适用预览验收。未经独立实际验证不得声称Codex真实客户端全部接入成功。
