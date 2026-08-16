# 视频厂商原生计费重构技术方案索引

> 状态：用户已逐节确认，进入实施

本次重构采用以下架构：

1. 每个真实视频模型维护版本化、schema 判别的 CNY 销售费率卡。
2. `seedance_token_v1` 与 `minimax_h3_second_v1` 使用独立的强类型计算器。
3. 视频路由按能力与参数映射筛选候选，计算所有可用候选销售价并取最高值。
4. 最高 CNY 通过全局 `billing_pricing.cny_per_point` 换算积分，应用最低任务积分和向上取整步长后生成固定报价。
5. 新任务保存 v2 报价快照；旧任务保留 legacy 快照结算分支。
6. 旧视频成本、策略、价格规则退出运行时，并通过幂等迁移停用旧路由、保留历史账务。

完整技术设计：

- `docs/plans/2026-08-17-video-native-pricing-refactor-design.md`

测试先行实施计划：

- `docs/plans/2026-08-17-video-native-pricing-refactor-implementation.md`

