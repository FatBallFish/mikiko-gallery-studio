# 路由图片销售价格与 Seedream 排队任务修复

日期：2026-10-26。需求与设计见 [本轮计划](../plans/2026-10-26-unified-route-image-pricing.md)。本报告在验证期间更新；未完成的条目不代表通过。

## 已确认的原因

原生 Qwen/Seedream 报价原先读取接入型号 `extra.native_image.sale_pricing`，因此路由未配置传统 1K/2K/4K 价格也可报价、冻结与创建。接入型号 `pricing` 是成本，`sale_pricing` 是旧销售价；不是没有售价而以成本代替。原生按像素区间选每张费率，不是已有的官方像素 token 计费。

拆层开关独立于完整能力及计费配置。当前 Seedream 显式 profile 虽声明拆层，但缺少最大图层数与拆层尺寸模式，成本确认未开启且缺图层阶梯，销售底图和图层阶梯也缺失。普通输出图阶梯不代替拆层阶梯。不能以自动填零价或将未确认改成已确认解锁。

原 Seedream 任务 `da80b39f-89d2-5fbb-8b3b-0e5b2ae62187` 在排查时为 queued，没有 lease、开始时间、上游提交记录或产物；原因是 API 恢复后没有 Worker 消费。只冻结 0.73600 积分。

## 原任务的实际结果与恢复失误

恢复时重新检查只有此条图片任务与一条历史非付费媒体 probe，原 runtime 与 install-state/数据库绑定一致。使用当前源码构建 Worker 并按原完整 roles 启动；但遗漏了新增 Ent 字段与现有数据库结构的配套检查。Worker 加载路由时查询新列 `route_models.image_pricing`，当前数据库尚无该列，导致目标任务在 2026-10-25 18:10:39 UTC 以 `INTERNAL_ERROR: failed to load model routing config` 失败。

没有调用上游、没有产物，也没有重复提交。系统自动退回原冻结的 0.73600 积分，余额恢复为 512.06400，冻结为零。账本记录为原 reserve 与 refund。原任务已经终态，不自动重试或替用户新建收费任务。

自建 Worker PID 6905 在确认路径、没有 running 工作后优雅停止；历史非付费 probe 已成功。原 API 保持运行。部署必须先确认 schema、API、Worker 配套，再启动消费。

## 实现与验收状态

实现已完成：路由独立 `image_pricing` 版本化规则、按操作销售配置、可信输入尺寸及逐图 token、历史快照兼容、后台售价表单、用户拆层具体中文原因。迁移工具只在有效候选旧售价一致且无冲突时生成计划，配置通过后台 API 应用；不默选最大候选售价或更改币种。

独立复核发现并关闭了 12 项 P2：契约 3 项（省略更新清价、缺费率误转零、通用候选能力）、计费 3 项（通用路由依赖旧价、汇率哈希、拆层仅图层成功过扣）、后台 6 项（草稿保护、partial 配置、拆层 auto、成本缺项、legacy 像素交集）。另补中文缺项与原生路由说明。聚焦回归通过。

完整 `./scripts/workflow/verify.sh` 退出 0；包含 Go tests/vet、前后端契约、用户/后台/docs 类型检查构建及发布产物验证。隔离 `./scripts/workflow/api-smoke.sh` 最终退出 0，日志 `/tmp/unified-route-pricing-api-smoke-final2.log`。REST/MCP 10 个模拟原生生命周期子场景通过，包括报价、结算、幂等及冻结清零；这些不代表新的真实厂商生成。

本地无收费 API 验收 61 PASS、0 FAIL、0 BLOCKED，结果 `/tmp/unified-route-pricing-acceptance/api-result.json`：六条路由 generate/edit 带一/二张 owned 参考图的 1K/2K 报价与旧售价一致，全部拒绝 4096×4096；Seedream Pro 首张免费、第二张输入增加 0.022 CNY；两个 Seedream 拆层均返回具体缺项并拒绝估价；参考资产 IDs 与显式数量不一致会拒绝。像素 token 逐图取整、首免、归属与缺尺寸拒绝由 unit/fake 测试验证，没有修改实际路由为 token 模式。

## 本地部署与迁移

新完整备份及 archive 解压校验通过，目录 `runtime-local/schema-backup-20261026-021930-PjoK7W`（0700，内容私密 0600）。普通官方 migration 补 nullable jsonb `route_models.image_pricing`，版本保持 9/1；安装 binding、runtime.env、install-state 校验均未变。

新 API PID 48857、Worker PID 48899 使用原绑定配置，API healthz/readyz 200，Worker 心跳 healthy、初始化成功，无 schema/routing 错误。启动前后全部可执行队列为零。旧 API 50777 已优雅退出。

六条原生 route（ID 3–8）通过 admin API 逐条 GET 比对、PUT、readback 迁移，仅新增 generate/edit 销售价，旧 provider sale 数据保留供审计但新任务不再读取；无拆层销售规则。价格币种、输入/输出金额、免费张数、像素阈值与旧配置逐项一致。FX 保持 0.3125 CNY/point。保护快照确认成本、原任务、余额/账本、系统/安全配置及 route 非售价字段未变。

浏览器 DOM 验收确认：迁移后四条 Qwen 路由可用，两条 Seedream 显示“部分可用”，已开放生成/编辑、未开放拆层。Seedream Pro 价格抽屉显示首张输入免费、输入价 0.022 CNY、输出像素阶梯 0.44/0.75 CNY，并逐项列出最大层数、拆层尺寸、成本确认及图层成本档位缺项。Qwen 表单显示实际像素范围，不提供不支持的 4K；原生无传统价格行时不再误报无候选。未通过浏览器保存任何验收草稿或创建任务。用户端主要以 61 项真实 GET 验收和前端契约验证，本轮未追加真实厂商生成或完整移动端视觉验收。

本地全改动 review gate 为 BLOCK，唯一原因是 heavyweight `approval.status=pending`，禁止发布门禁未完成；没有伪造审批。本轮未 commit/push 或远程部署。
