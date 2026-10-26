# 14项创作、媒体和开发者接入改进验收

需求与设计：docs/plans/2026-10-27-studio-media-developer-ux.md。分支 feat/studio-ux-developer-onboarding；本轮未commit/push/deploy，未新增付费生成，未修改生产配置或绑定。

## 已实现

1/3. 创作台模型下提供常规生图/图层拆分；画布参数下拉同语义。常规按引用自动generate/edit；拆层单图必填、输出1，保留用户模式与多图，能力/费率/异步引用完成均重校验。
2. 画布双击节点仅移动视口并适度fit，zoom上限1.25，考虑控制台边距；5px拖动阈值防误移，显式详情按钮保留。
4-7. 图片视频共用公开状态操作，分组tag置后，视频像素/格式/时长；公开状态服务端分页前筛选同时用于浏览器与开发者API/MCP。
8/9. 可信端点投影、密钥页复制配置、完整API/字段/错误/OAuth教程、OpenAPI自动同步真实operation输入输出；已连接应用和consent错误准确分类，DCR时间窗口恢复。
10. 视频点赞收藏持久化、计数、viewer状态、Feed热榜和筛选，鉴权附件下载/Range和撤回边界。
11-14. 媒体标题固定两行顶对齐、topbar防收缩和手机紧凑布局、四等级Toast及稳定生命周期、首页完成100%静止条。

## 自动和独立验证

- 生成模式、引用准入、canvas viewport、媒体公开/排序/分页/legacyID、Toast相关聚焦契约与三端类型构建通过。
- OAuth challenge/discovery/DCR/PKCE/consent/token/refresh/MCP SDK tools/revoke隔离HTTP链路通过；重放/并发/跨用户/限流覆盖。
- PostgreSQL16 schema10→11官方迁移、旧数据/身份保持、唯一约束、重复迁移及新旧schema门禁通过；专用测试容器清理。
- 视频互动PG/HTTP验证幂等、20次并发重复写、同UUID图片视频隔离、Feed filters/hot、附件GET/HEAD206/416、撤回拒绝。
- 独立复核发现两项P2并已修复复核通过：列表补读期间撤回不会让整页失败；图片更新响应始终投影legacy权威公开状态。
- 独立生成/画布/视觉代码复核未发现确定新增P1/P2。
- 隔离API smoke通过。最终完整 `./scripts/workflow/verify.sh` exit0，日志 `/tmp/studio-ux-verify-complete.log`；三端typecheck/build通过。首次全量中的DCR Retry-After时间断言已改确定时钟并通过count20/race，第二轮旧源码契约写死Error已更新为Warn且保留原子准入校验。

## 浏览器记录与边界

真实本地用户登录。首页完成态aria100、fill等于轨道、animation none；桌面topbar72px，手机60px。资产标题两行槽位、公开状态和组tag、视频时长真实可见；320账户按钮裁切已修并复验right308。390/1280页面无横向溢出。

本地两个Vite共用缓存造成Lexical optimize deps504，导致创作/编辑器动态import失败；仅停止本轮自己的服务，改用5293和独立缓存，保留用户旧5193服务。新入口创作恢复，常规默认和不支持能力禁用已真实验证。

主线补做真实画布DOM双击事件验证：原生locator dblclick工具超时，改由已观测article派发同一dblclick事件。节点“图片节点3”原始left1025.53/top1010.69/width420/height340保持，屏幕矩形变为left377.50/right902.50/top216.50/bottom641.50，中心x640位于1280视口中心，zoom由37%变为125%。父视口transform改变，当前可见节点样式均对应原值；虚拟化导致不可见节点卸载，不能以全DOM列表减少推断节点移动。原生指针微抖/所有节点数据不变由纯几何及store回归覆盖，未把DOM事件替代验证称为完整物理双击验收。

本地Seedream未确认拆层能力/价格，未为测试解锁，拆层完整交互由集中契约与输入回归覆盖，不声称真实付费拆层成功。本地广场无公开视频，视频互动依隔离PG/HTTP及组件契约，不声称真实广场视频点击已验。

生产Gallery尚无canonical HTTPS入口，OAuth/MCP保持未配置；现有mgsctl upgrade不重渲染旧nginx落盘资源且无法合法更新setup绑定URL。本轮不改生产域名/digest。需实际部署配置及真实Mikiko/Codex/Claude授权后才能宣称生产客户端接入成功；当前仅隔离协议链路通过。
