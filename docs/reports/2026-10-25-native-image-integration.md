# 原生图片模型与混合公开媒体实施验收

日期：2026-10-25。需求与设计见对应专项PRD/tech文档。

## 已执行验证

- 新增 internal/mcpserver/native_image_integration_test.go，10场景通过：API Key REST及OAuth principal StreamableHTTP MCP分别执行Qwen生成/编辑、Seedream生成/编辑/拆层。
- 生产handler/transport→共享preflight/create→领取租约执行worker→原生适配HTTP→本地存储→查询/access→HTTP下载逐字节核对PNG；拆层额外检查manifest/元数据/逐层access、幂等重放上游只调用一次、跨用户拒绝。
- 上游、认证validator及下载签名是fixture；并非真实厂商或授权码签发实测。现有本地DB只读检查匹配Qwen/Seedream账号/模型均0，未产生真实费用、未添加凭据。
- 隔离 scripts/workflow/api-smoke.sh exit0，使用独立PG/Redis/API/Worker/fake provider并清理。日志 /tmp/native-image-api-smoke.log，完成地址 http://127.0.0.1:50015。
- 10场景race与mcpserver/openplatform/qwenimage/seedreamimage整包回归通过，日志 /tmp/native-image-integration{,-race}.log及/tmp/native-image-focused-final.log。
- 浏览器：localhost5273真实最近任务图片加载完成，naturalWidth=1448/naturalHeight=1086，标题“阳台安睡的鹈鹕”；查看任务结果class为u-btn u-btn--primary。首页精选与广场读取新混合Feed，当前仅公开图片时不存在“公开视频”与“暂时无公开视频”分区文案。当前真实DB无公开可混排视频，混合排序分页由数据库fixture回归覆盖。
- 管理端浏览器验收：新增图片账号可选择Qwen/Seedream；切换Seedream后显示Ark官方根地址提示并隐藏OpenAI/Codex来源模式。仅打开并取消表单，未保存账号或密钥。
- 最终匿名Feed请求HTTP200，图片URL为/api/open/image/v1/gallery/images/{id}/image；独立SQLite生成视频fixture确认完整PromptTemplate不出现在匿名列表。
- 最终仓库 scripts/workflow/verify.sh exit0，Go tests/vet、全部前端契约、user/admin/docs typecheck/build、直连/前缀部署及Linux/Windows native package均通过。日志 /tmp/native-image-verify-release.log。OpenAPI/文档站通过，公开镜像51路径。
- 独立代码复核发现的收费checkpoint/角色计费/结算终结、公开Feed隐私投影与前端配置问题均已修复并复验关闭。git diff --check通过。
- 本地review-local --scope all仍BLOCK，仅heavyweight approval.status pending限制push/PR；未修改审批标记，未提交推送。
- Canvas backend图层布局与domain/存储回归通过，底图完整尺寸、层按bbox同origin/scale和z-order放置，有效小图层允许小于普通节点最小尺寸。

## 数据与计费

native_image_v1将操作与参数版本化存入现有routing snapshot，旧记录兼容；付费检查点加密保存原始response与全部图层，公开响应不含原始metadata。

原生报价冻结成本/售价CNY、兑换率及操作；拆层按实际IsBase角色/像素结算，最高17产物预留。超预留收费封顶原授权额，成本诊断保留；未知成本/毛利不伪装0。正常失败释放仍保持既有行为。

上游已提交但checkpoint双写失败的极端情况通过持久化upstream_started_at防止重领再次付费生成，任务进入IMAGE_UPSTREAM_SUBMISSION_UNCERTAIN并保留预留。该状态需人工对账，当前无自动对账/自动释放或新管理员解锁入口，错误字段与billing diagnostic可观测，不能声称全自动恢复。

## Excel

现有工作簿新增6页、52场景、156兑换敏感性、2496新增live公式；原5视频页数据/公式/样式与ZIP部件保留。LibreOffice重算4539公式零错误、7项交叉校验PASS；详见docs/reviews/2026-10-25-native-image-pricing-report.md。

Pro拆层底图费与Flash拆层费官方仍未明确，待核实费率保持空白，16拆层场景不生成完整报价/默认不启用；管理员明确确认成本和售价口径后才能启用。无自动修改已有线上价格。

## 交付边界

本轮没有真实Qwen/Seedream账号，未做厂家付费调用。Qwen重复3.0按唯一三款处理，3.0-pro额外提供可选禁用预设。Seedream坐标/标记编辑不宣称为独立mask或严格框外不变。HEIC/HEIF校验容器结构/尺寸，未引入原生codec解压压缩像素。ZIP为浏览器session授权接口；API Key/MCP使用层清单与逐层受权下载。

未commit/push/生产部署；本地API已更新用于浏览器验证，未启动共享付费Worker。
