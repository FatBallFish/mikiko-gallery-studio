# 首页混合媒体与 Qwen / Seedream 图片接入需求

来源：2026-10-25 用户请求及本会话批准的实施方案。

## 验收要求

1. 首页最近任务加载实际结果封面，优先结果图片标题，模型与类型为次级信息；查看任务结果使用主按钮并保留任务导航。空结果、失败、部分成功和URL过期可恢复。
2. 首页与广场在同一列表混排审核通过的图片与视频，统一查询/排序/分页；没有视频时不显示独立视频标题与空态。保留公开Prompt和媒体授权边界。
3. 接入 Qwen qwen-image-3.0、qwen-image-2.1-pro、qwen-image-2.1-turbo；qwen-image-3.0-pro 为额外可选预设（原请求3.0重复，未确认替换）。接入 Seedream 5.0 pro / flash，允许真实Endpoint ID。
4. 文生图、参考图编辑和官方支持的专属能力贯通底层、后台表单、工作台、画布、API Key与OAuth MCP。Qwen/Seedream不声明未获官方支持的独立mask；Seedream支持坐标/标记编辑和图层拆分，仅对支持型号开放。
5. 图层保留底图、透明PNG、层次/坐标/名称，支持预览下载、清单打包和画布还原。按操作保存和结算，不伪装普通批量图。
6. 图片能力按型号显式约束并参与候选、预校验、报价指纹、重试和配置版本。参数不能静默丢弃。采用原生同步适配，复用付费检查点和存储恢复。
7. 计费支持参考图与输出/拆层费用，预扣上限、按实际结算、失败释放且幂等；CNY与积分分离。未知价格不置零、不自动启用。
8. 在已有《视频模型成本与售价利润表.xlsx》中新增图片费率、场景成本、建议售价及配置页，保留原视频内容，沿用可调参数和公式并验证。
9. 完成聚焦测试、全量verify、独立复核、隔离API冒烟和浏览器验收。无有效厂商凭据时如实区分fixture与实测。不得自动commit/push/部署。

## 官方来源（2026-10-25核对）

- https://docs.bailian.console.aliyun.com/zh/model-studio/text-to-image
- https://docs.bailian.console.aliyun.com/zh/model-studio/qwen-image-generation-and-editing-api-reference
- https://docs.bailian.console.aliyun.com/zh/model-studio/model-pricing
- https://console.volcengine.com/ark/region:cn-beijing/docs/seedream-5-0-pro
- https://ark.volcengine.com/docs/image-generation-api
- https://ark.volcengine.com/docs/seedream-5-0-pro-editing-guide
- https://ark.volcengine.com/docs/model-pricing

Qwen：3.0最多3参考图，2.1最多10；n=1..6；输出262144..4194304像素、1:8..8:1。3.0支持negative_prompt；2.1支持提示词驱动透明输出。prompt_extend/enable_thinking显式参数；agent仅文生图。
Seedream：pro doubao-seedream-5-0-pro-260628；flash doubao-seedream-5-0-flash-260915；最多10参考图、普通单图、输出921600..4624220像素、1:16..16:1。pro优化standard/fast，flash standard。拆层1底图+最多16层，层为透明PNG并带bbox；point/bbox提示词坐标0..999。独立mask未列。

价格来源：Qwen3.0-pro输入0.02元/张、输出1K0.25/2K0.50；3.0输入0.02、输出0.18；2.1-pro0.25、turbo0.10。SeedreamPro首输入免费其余0.02、普通输出像素<=261万0.30否则0.60、拆层单层0.15/0.30；Flash输入免费、输出0.12。Flash拆层与Pro底图收费口径需再次确认，未确认不得自行补价。
