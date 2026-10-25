# 首页混合媒体与原生图片模型技术设计

需求：docs/prd/2026-10-25-native-image-models-and-public-feed.md。
批准来源：本会话 ExitPlanMode 完整方案已获批准。范围为heavyweight，不沿用上一任务审批。

## 设计

- HomePage复用已有结果、私有mediaAccess与GalleryImageFrame，封面首选可用结果、标题首选结果名称；完成按钮主样式，保留taskId导航，刷新可取消。
- 统一公共Feed以带media_type的联合投影组合公开图片/视频，数据库层统一过滤排序分页。旧接口兼容；复用现有公开详情和授权，匿名仅摘要。首页/广场共同消费，视频不再独立分区。
- 图片adapter新增qwen_image和seedream_image，采用各自原生JSON同步协议，编辑不套OpenAI multipart。复用超时/凭据/错误边界和artifact恢复。
- 在共享图片请求/能力中增加结构化native options与操作类型（generate/edit/layer_decomposition）、坐标标注及图层元数据。精确型号profile驱动动态表单和服务端约束；账号模型extra可存版本化能力，必须明确保存回填。字段参与capability version、quote和幂等指纹。模型尺寸使用显式约束，旧模型保留默认。
- 图层拆分一次上游操作返回底图+层，结果持久化z_index/bbox/name/description/format/尺寸并保留透明度。检查点保存完整结构，下载失败只恢复存储。普通count与拆层产物数分离。返回层清单、组合预览和打包下载，画布按bbox/z-order还原。
- 原生图片费率结构区分input/output/layer/size tier与CNY币种。报价冻结候选费率/售价/积分兑换率，拆层预扣最大17产物，按真实usage/产物结算且释放差额；未确认费率禁止自动启用。旧计费不受影响，人民币成本不可直接减积分。
- 后台ProviderModelsPage及RoutingPage按adapter/media_type与型号显示适用字段和测试能力；工作台/画布使用共享专属操作控件。原生API/MCP共享ImageInput/schema/操作；OpenAI兼容支持普通新模型生成编辑，专属操作原生暴露。同步OpenAPI与文档。
- Excel原文件新增图片工作表并链接既有费率参数；所有派生值保留公式，明确牌价/待核实/建议价而非线上已生效配置。

## 实施顺序

1. 固定需求设计与coding context，独立实现首页与公开Feed。
2. 固定共享native image契约，原生provider与能力profile并行实现。
3. 集成admin配置、worker/任务/计费/图层与API/MCP。
4. 用户专属交互与Excel，统一文档。
5. 聚焦测试、verify、独立审查、隔离smoke、浏览器验证。

## 验证

首页真实结果和过期URL；混排稳定排序分页/审核权限；adapter payload/错误/字段拒绝；坐标转换、图层元数据/检查点恢复/计费幂等；APIKey兼容与OAuth MCP创建查询下载；前后端类型构建；Excel公式重算与格式检查。真实厂家调用仅使用已有有效配置并最小化消费，不把模拟成功说成真实成功。
