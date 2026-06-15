# 管理后台 redesign demo 逐页差异审计

> 更新时间：2026-06-12
>
> 目标：逐个页面对比 `web/redesign-demo` 与 Docker 真实后台 `http://127.0.0.1:8088/admin`，同时覆盖暗色与亮色主题，记录差异、修复动作与剩余风险。

## 对比原则

- demo 只作为布局、主题、密度、组件语言参考；真实后台必须保留真实 API、权限和业务动作。
- 暗色与亮色主题都必须使用同一套 `data-theme` token，不允许页面局部硬编码旧浅色背景。
- 对普通 `admin` 账号不可见的高权限页面不强行展示；这类差异归类为“权限正确性差异”。

## 页面审计矩阵

| 真实路由 | demo 参考 | 暗色差异 | 亮色差异 | 当前动作 | 状态 |
|---|---|---|---|---|---|
| `#/login` | 登录页规范（技术方案 7.1） | 已改为 Mikiko 单卡控制台登录；不再是旧浅色分栏 | 已接入同一套主题按钮和 `data-theme` token | 单卡控制台登录 + 明暗主题切换 | 已处理 |
| `#/dashboard` | `Dashboard.tsx` | 已补 demo 同款 4 指标卡与“模型调用分布 / 用户消费榜”双栏，数据来自真实 provider/users/operations | 亮色首屏移除内部 PageHeader，运营明细折叠保留 | 保留真实运营指标、上线风险和最新用户表；风险/用户表进入运营明细折叠区 | 已处理 |
| `#/monitoring` | `Monitoring.tsx` | 已改为真实数据驱动的 Infrastructure / SLA / Provider / Readiness 单页结构 | 已通过主题按钮验证 token 跟随切换 | 新增 `MonitoringPage`，替代 Health+Readiness 拼接 | 已处理 |
| `#/users` | `UserManagement.tsx` | 已从通用 dataGrid 改为 demo 语义的“用户信息/分组/余额/活跃/状态/操作”表格 | 已通过 Docker 亮色 DOM/theme 复核 | 保留详情、禁用/启用、分组、积分、限额、密码、删除真实动作 | 已处理 |
| `#/user-groups` | `GroupManagement.tsx` | 本轮从横向卡片流改回 demo 表格骨架，保留倍率/默认/状态/编辑 | 暗色差异降至 `0.0208`，亮色差异降至 `0.0304` | 表格列保留模型可见性入口；真实分组摘要折叠为次级信息 | 已处理 |
| `#/call-records` | `CallRecords.tsx` | 已补 demo 风格查询面板、时间 pill、区间统计卡、三组分布条与明细表；本轮移除首屏页面头和外层大容器，低频筛选默认折叠 | 已通过 Docker 亮色 DOM/theme 复核 | 保留真实筛选、分页、错误详情展开；Provider/入口/状态为主筛选，任务 ID/错误码进入高级筛选 | 已处理 |
| `#/redeem` | `CouponManagement.tsx` | 已从通用 dataGrid 改为 demo 语义的兑换码表格；当前 Docker 数据为空时展示 token 化空态 | 已通过 Docker 亮色 DOM/theme 复核；奖励/核销进度表头需有数据时再截图确认 | 保留创建、批量生成、导出、状态变更、核销记录真实动作 | 已处理 |
| `#/reviews` | `AuditQueue.tsx` | 已从表格改为审核卡片流；主筛选收敛为待审核/已通过/已驳回，已下架/全部保留为次级筛选 | 亮色截图仍受真实环境仅 1 条待审数据影响；结构差异已小幅下降 | 保留真实审核 API、决策理由和审计写入 | 已处理 |
| `#/orders` | `OrderManagement.tsx` | 已修正主色、壳层、active nav、订单概览语义 | 已通过 Docker 亮色 DOM/theme 复核 | 已截图验证 + DOM/theme 复核 | 已处理 |
| `#/packages` | `PackageManagement.tsx` | 已修路由 tab 串页，改为套餐卡片首屏；本轮单 tab 模式隐藏孤立 tab，自定义金额默认折叠 | 已通过 Docker 亮色 DOM/theme 复核 | 套餐卡片保留真实新增/编辑/删除，次级自定义金额配置通过按钮展开 | 已处理 |
| `#/cashier-config` | `CashierManagement.tsx` | 已将首屏改为支付通道 / UI Settings / Risk & Sync 面板，配置表仍保留在 tab 中 | 已通过 Docker 亮色 DOM/theme 复核 | 默认进入 overview 面板，保留支付方式和渠道实例真实编辑功能 | 已处理 |
| `#/routing` | `RouteModelPage.tsx` | 已从“路由表 + 候选表”改为 demo 风格路由行内展开候选真实模型 | 已通过 Docker 亮色 DOM/theme 复核 | 保留新增/编辑路由、新增/编辑候选、分组可见性与真实 API | 已处理 |
| `#/access-accounts` | `AccessAccountPage.tsx` | 已改为 demo 风格工具栏 + 账号表 + 行内展开支持模型子表；首屏默认折叠，测试结果 modal 已去除 `bg-white`/旧 subtle | 已通过 Docker 亮色截图复核，并退出最高差异列表 | 保留账号 CRUD、模型 CRUD、真实测试出图 | 已处理 |
| `#/pricing` | `PriceConfigPage.tsx` | 已加入计费规则说明，并按路由模型/任务类型聚合；首屏默认折叠质量项以对齐 demo | 本轮去除内部 PageHeader，亮色截图差异从 `0.0395` 降至 `0.0380` | 聚合价格行 + 展开质量项；保留新增/编辑真实价格配置 | 已处理 |
| `#/audit` | `AuditLog.tsx` | 已改为 demo 风格审计日志卡片时间线，保留搜索/动作筛选/导出 | 已通过 Docker 亮色 DOM/theme 复核 | 卡片时间线 | 已处理 |
| `#/system-users` | `SystemUsers.tsx` | 页面已改为 demo 语义表；普通 admin 访问会被权限正确地切回可访问页面 | 同左 | 保留真实系统管理员 CRUD；需 super_admin 数据账号再做实页截图 | 权限正确性差异 |
| `#/system-settings` | `SystemSettings.tsx` | 已改为 demo 风格 General / Security / Storage 三段结构；本轮隐藏状态条和重表单首屏，配置编辑与 SMTP 高级配置默认折叠 | 已通过 Docker 亮色 DOM/theme 复核 | 保留真实配置类目保存、SMTP 保存与测试邮件流程 | 已处理 |

## 本轮已确认截图

- Demo 订单页：`tmp/demo-orders-reference.png`
- 真实后台订单页暗色：`tmp/admin-orders-dark-final.png`
- 真实后台套餐页亮色：`tmp/admin-packages-final.png`
- Demo/Docker 逐页截图差异：`tmp/admin-visual-audit/summary.json`
  - 最新批次覆盖 30 个页面/主题组合。
  - `#/pricing` 亮色差异从 `0.1533` 降至 `0.0395`，主要修复为移除多余状态卡/反馈条、默认折叠质量明细、主表回到 demo 聚合列。
  - `#/access-accounts` 亮色差异从 `0.0842` 继续降出最高差异列表，主要修复为移除页面说明/统计条/默认展开，首屏回到 demo 工具栏 + 折叠表格。
  - `#/reviews` 亮色差异从 `0.0574` 降至 `0.0548`，剩余差异主要来自真实环境只有 1 条待审数据，而 demo 使用 3 条 mock 卡片。
  - `#/call-records` 本轮首屏结构收敛后，暗色差异从 `0.0369` 降至 `0.0315`，亮色差异从 `0.0440` 降至 `0.0396`；剩余差异主要来自真实调用明细文本量远高于 demo mock 表格。
  - `#/packages` 本轮单页套餐结构收敛后，暗色差异从 `0.0339` 降至 `0.0289`，亮色差异从 `0.0424` 降至 `0.0371`；共享 `CashierPage` 的 `#/orders` 与 `#/cashier-config` 未出现明显回退。
  - `#/system-settings` 本轮配置页首屏收敛后，暗色差异从 `0.0323` 降至 `0.0233`，亮色差异从 `0.0386` 降至 `0.0300`；剩余差异主要来自真实配置字段与 demo 静态站点字段不同。
  - 本轮修正截图脚本等待条件：Docker 页面等待“正在请求真实后台 API。”消失，demo 页面等待 Dashboard 模拟加载消失，避免把 loading 态误计入页面差异。
  - `#/dashboard` 本轮首屏收敛后，亮色差异从 `0.0378` 降至 `0.0352`；真实页从多指标长首屏收敛为 demo 4 指标卡 + 双栏洞察，额外上线风险和最新用户表保留在折叠区。
  - `#/pricing` 本轮去除重复内部 PageHeader 后，亮色差异从 `0.0395` 降至 `0.0380`；保留真实新增/编辑价格配置流程。
  - `#/user-groups` 本轮回到 demo 表格骨架后，暗色差异从 `0.0261` 降至 `0.0208`，亮色差异从 `0.0346` 降至 `0.0304`；剩余差异主要来自真实环境只有 1 个分组，demo 有 3 个样本分组。
  - `#/monitoring` 曾尝试固定 6 张 Infrastructure 卡，但亮色差异轻微回退至 `0.0372`，已回到真实 Provider/Readiness 摘要结构；当前亮色差异 `0.0366`。
  - 当前最高差异：`reviews` light `0.0548`、`call-records` light `0.0396`、`pricing` light `0.0384`、`cashier-config` light `0.0376`、`packages` light `0.0371`、`monitoring` light `0.0366`、`dashboard` light `0.0348`、`routing` light `0.0343`。
- Docker 逐路由 DOM/theme 复核：`tmp/admin-theme-route-audit.latest.json`
  - 覆盖 `#/login`、`#/dashboard`、`#/monitoring`、`#/users`、`#/user-groups`、`#/call-records`、`#/redeem`、`#/reviews`、`#/orders`、`#/packages`、`#/cashier-config`、`#/routing`、`#/access-accounts`、`#/pricing`、`#/audit`、`#/system-settings`。
  - 暗色、亮色各一轮；marker 全通过；大块纯白 surface 计数为 0。

## 下轮优先级

1. `#/call-records` 是当前最高的可改业务页；继续下降需要进一步收敛真实明细文本量和 demo 聚合表格密度。
2. `#/pricing` 已完成标题收敛，后续若继续下降应优先对齐真实路由行数与 demo 多行样本，而不是改变真实价格数据。
3. `#/cashier-config` light `0.0376` 已证明 tab/按钮微调可能回退，下一轮需先截图定位具体卡片密度差异再改。
4. `#/reviews` 剩余差异以真实数据条数为主；如要继续下降，需要准备多条待审核样本，而不是引入 mock。
5. `#/monitoring`、`#/routing`、`#/users` 处于中低差异区间，后续可按截图继续微调列距、卡片高度和次级文案密度。
6. `#/system-users` 普通 admin 不可见属于权限正确性差异；需要 super_admin 账号验证真实系统账户表格。
