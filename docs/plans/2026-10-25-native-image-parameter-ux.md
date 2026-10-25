# 原生图片参数与模型配置体验优化需求及技术方案

日期：2026-10-25。来源：本会话用户六项明确要求（序号跳过3）。延续未提交原生图片集成，不自动提交、推送、部署，不新增付费测试调用。

## 需求

- 创作页和画布沿用已有操作台字段、间距、输入、折叠和弹窗设计，不再呈现无样式的“模型专属参数”框。
- 随机种子不向用户展示；若保留平台默认控制，用后台有解释的可选固定种子，留空随机。
- watermark核对官方含义；官方水印默认false，不展示用户开关，不虚构自定义水印能力。
- 普通generate/edit由有效参考图片数量自动选择，与GPT Image一致；用户不选择操作。已有拆层能力作为明确独立功能入口，受能力和确认费率限制，不误触普通生成。
- Qwen prompt_extend/enable_thinking与Seedream优化按官方计费、交互要求确定后台默认，不在用户端显示。确认不额外收费且仅参数启用则默认开启；额外收费或需要额外交互则关闭。资料无法确认时保守关闭，报告具体证据限制。
- 负向提示词仅支持模型展示在主提示词下，使用同风格编辑控件，默认折叠；不与不支持型号共享遗留参数。
- 引用图片点击打开专用标记弹窗：框选和画笔/圈画可见，显示选中坐标/数量/保存状态，可撤销、一键清空所有标记。完成后引用缩略图显示“已编辑”。标记隔离到引用图片；删引用、替换图片或切换不支持模型时清理，不能把坐标/画笔泄漏给下一型号。
- 用户称Seedance的框选/圈画，需按现有实现与官方能力辨别实际Seedream图片功能；不擅自为Seedance视频声明支持。
- 后台账号模型字段使用有层次的可视化表单，整齐网格与间距，普通/拆层阶梯标明单位、像素上限与选择规则；隐藏原始extra JSON编辑，已知extra字段可视化，未知键原样保存，不意外覆盖已有账号/模型配置。

## 技术设计

- 复用shared native-image类型与现有用户/画布引用数据，抽取共享负向提示词和引用标记编辑器；主提示词现有组件可复用时优先复用，不复制一套视觉规则。
- 通过原始draft和当前references解析普通operation。历史用户draft中的seed/watermark/优化/思考等隐藏字段不继续静默影响新UI请求；API/MCP显式参数兼容需明确保留规则。
- 后台native_image.defaults（或与现有配置兼容的等价结构）统一解析默认配置；运行时、能力版本、报价及幂等指纹考虑实际影响字段，确保模型extra覆盖账号extra的语义正确且可校验。固定种子可选，禁止把未知收费开关默认开启。
- 标记弹窗复用OverlayPortal/Dialog惯例，支持Escape、焦点返回、手机最大高度和内部滚动；图像使用contain的真实显示矩形转换到0..999坐标，展示框/线及坐标。保存与取消语义清晰；清空同时清理框选与画笔数据。合成标记图仅普通参考图上传，避免重复上传/失败清除原标记。
- 后台表单分基本信息、能力、平台默认与计费，结构化编辑已知配置；纯JSON不再要求管理员编写，未知extra只读摘要并保留。说明实际成本CNY/建议售价CNY与积分换算边界，不顺带更改运行FX。

## 官方参数核验与策略

2026-10-25读取官方Qwen API参数与价格页、Seedream模型页及价格页。Qwen价格页成功复核仍未明确上述四型号prompt_extend/enable_thinking/direct/agent附加收费条款；不能从按张计费推断免费。

- Qwen水印参数默认false，Seedream watermark=true表示右下角“AI生成”；两者已读接口仅布尔开关，没有自定义文字/Logo/位置参数。本平台按用户要求显式false。
- Qwen prompt_extend默认true、direct模式；enable_thinking默认true且仅extend开启时生效、增加耗时。未确认附加费，平台保守默认extend=false、thinking=false，后台可在核实费用后开启，不向终端用户展示。
- Seedream优化API为optimize_prompt_options.mode，pro支持standard/fast，flash仅standard；默认standard，没有官方关闭值。平台不伪造none/false，后台标准默认并标注计费待确认，用户无配置项。这是无法严格关闭的厂商约束，交付报告明确说明。
- 现有框选圈画实现属于Seedream图片链路，Seedance视频没有接入，按既有功能修复Seedream，不为视频虚构能力。

来源：https://docs.bailian.console.aliyun.com/zh/model-studio/qwen-image-generation-and-editing-api-reference 、https://docs.bailian.console.aliyun.com/zh/model-studio/model-pricing 、https://console.volcengine.com/ark/region:cn-beijing/docs/seedream-5-0-pro 、https://ark.volcengine.com/docs/model-pricing 。

后台defaults契约：`extra.native_image.defaults = {seed?,watermark?,qwen?:{prompt_extend?,enable_thinking?,prompt_extend_mode?:direct|agent},seedream?:{optimize_prompt_mode?:standard|fast}}`。模型继承账号、实际参数冻结到任务snapshot；API/MCP显式options覆盖后台defaults，用户UI不再提交隐藏参数。

## 验证

聚焦自动operation、隐藏字段清理、默认策略/验证、标记坐标缩放/清空/切型号、extra无损保存与阶梯边界。执行全量verify、独立review及适用隔离API smoke；浏览器检查创作/画布/后台桌面与手机，无新增付费生成。官方水印和计费结论附来源与不确定性。保留原有所有未提交改动及密钥文件，不读取或提交swap。
