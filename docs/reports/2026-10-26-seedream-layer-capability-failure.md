# Seedream 拆层 capability_changed 修复报告

日期：2026-10-26。用户报告拆层测试失败，要求查因修复。

## 根因

最新拆层任务 `c180d205-dcb5-59e1-a8fc-f9f34ab560df` 于本地02:59:23创建，Seedream Flash，`operation=layer_decomposition/task_type=image_edit`。创建返回本地409 `capability_changed`。没有上游调用、产物、冻结或扣费，账本零条。任务失败前已重新获取能力和多次有效报价，不能解释为单纯使用配置更新前的旧页面。

报价 `billing.EstimateContext` 使用 1<<30 的虚拟限额；创建使用实际全局限额（输出5、参考图4）。Seedream profile 输出1、参考图10，可见 image_edit 能力 hash 分别纳入1/10与1/4，即使只传一张参考图也不同，导致新报价提交被误拒绝。text_to_image 将参考图上限置零，因此此前普通文生图不受此差异影响。不是厂商拆层接口报错。

## 修复

billing.Service 初始化保存实际 GenerationLimits，默认与配置加载一致5/4；通过仅初始化使用的SetGenerationLimits注入自定义值，EstimateContext不再使用虚拟不限额。production app与API组合初始化均注入真实配置，统一会话/OpenAPI/兼容预估与创建版本。不删除能力版本校验、不提高平台上限、不修改用户拆层费率。Worker已有执行路径不生成用户EstimateContext报价，不需要为此重启。

## 验证

先失败回归 `/tmp/seedream-capability-limits-red.log` 明确复现edit/layer fresh quote被create共同预检拒绝，generate通过。修复后 `/tmp/seedream-capability-limits-green.log` 通过默认/自定义2引用/零值默认 × generate/edit/layer九组场景，版本与金额一致；真实售价变化仍409。

六个相关Go包tests/vet通过，独立bounded review无P1/P2，billing/imagetask相关race回归通过。本地review-local --scope all退出0。隔离API冒烟退出0（`/tmp/seedream-layer-fix-smoke.log`）。完整verify首次被此前私密目录的两个一次性Go工具重复main阻断；为两个本会话工具补build ignore后重跑，完整verify最终退出0（`/tmp/seedream-layer-fix-verify-final.log`），涵盖Go tests/vet、全部契约、前后端及文档构建和发布产物检查。

## 本地更新与无收费验收

仅更新API，新PID5034，binary `/tmp/seedream-readonly-acceptance-20261026-73HBqb/api-next`，同目录日志及部署回执。8091 readyz200。Worker保持原PID48899，不重启；runtime.env/install-state绑定与指纹未变，无schema或费率变化。更新前图片/视频/子任务/恢复队列均无活动任务，原API binary保留可回滚。

旧API同参数GET拆层报价版本为ddbf4e61…2fa3，与原失败创建解析器版本不同；新API同参数报价为42c3674e…d4dbf，与原失败任务服务端resolved版本完整一致，账号/路由配置更新时间保持失败时值。公开capabilities没有version字段，未将其伪称比较指标。创建共同预检的修复回归同时证明新报价可过相同真实limits解析，旧配置变化仍拒绝。

验收采用本轮独立白名单脚本，只允许正常登录及capabilities/estimate/balance GET，禁止POSTtasks，禁代理重定向，凭据token仅内存。用户验收前后及red到green：image_tasks29、video_tasks4、ledger66、余额均不变。未自动重试原失败任务或创建真实厂商收费任务，尚未验证真实拆层产物成功。未commit/push，保留全部既有改动。