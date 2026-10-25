# 原生图片参数与模型配置体验报告

> 阶段性历史声明：本文正文记录本阶段结束时的实现、验证、运行状态及审批事实；其中“当前”“待完成”“BLOCK / heavy pending”等表述仅适用于当时，不作为本轮发布的实时阻碍清单，也不追溯改变历史验证结果或旧审批。后续进展请分别参阅[本地服务恢复报告](2026-10-26-local-service-recovery.md)、[统一路由图片定价报告](2026-10-26-unified-route-image-pricing.md)及[Seedream 拆层 capability_changed 修复报告](2026-10-26-seedream-layer-capability-failure.md)，各报告仍以其所列验证范围和限制为准。本轮发布授权单独记录于[当前全部变更发布与生产升级](../plans/2026-10-26-production-release-native-image.md#授权)，不代表此前各阶段已获追溯审批。

日期：2026-10-25。

状态：本轮报告收尾，已执行范围内真实浏览器验收通过。后台默认语义已定：新任务冻结 effective options，API 显式参数覆盖后台默认。用户提供的结果为 verify PASS、隔离 smoke PASS、review PASS；创作 modal refresh/clear 与 Canvas 真实 graph UI 本轮操作已通过。Canvas 白屏已定位为开发依赖缓存问题，重启 5273 并使用 `--force` 后消失。gate 仍为 BLOCK（heavy pending）；未执行画笔上传后端链路，本轮无付费调用，不宣称该链路或 gate 通过。

需求及技术方案：[原生图片参数与模型配置体验优化](../plans/2026-10-25-native-image-parameter-ux.md)。本稿依据用户提供的官方核验结论和该计划整理，未在本次文档整理中重新访问官方页面或执行实现验证。

## 用户目标

- 创作页与画布复用已有操作台样式，移除突兀的模型专属参数框。随机种子、水印、提示词优化与思考参数不向终端用户展示；仅支持负向提示词的型号展示同风格、默认折叠的输入项。
- 普通生成或编辑由有效参考图片数量自动选择；拆层保留独立入口，受能力与确认费率限制。
- 引用图片通过专用弹窗进行框选、画笔或圈画，展示标记及坐标、数量、保存状态，支持撤销与全部清空，保存后缩略图显示“已编辑”。删除、替换引用或切换不支持型号时清理相关标记。
- 后台以分层可视化表单编辑账号和模型配置，解释平台默认及计费单位、阶梯边界；保留未知 extra 键，避免覆盖现有配置。
- 水印按用户要求显式关闭；未知附加费不得推断免费，也不得据此默认开启可关闭的优化或思考功能。

## 官方核验记录与平台策略

以下为本次接收的核验记录及计划中已有结论；附加费仍有证据限制。

### Qwen Image

官方参数页：<https://docs.bailian.console.aliyun.com/zh/model-studio/qwen-image-generation-and-editing-api-reference>。

官方价格页：<https://docs.bailian.console.aliyun.com/zh/model-studio/model-pricing>。

- `watermark` 默认 `false`；开启水印涉及右下角“AI生成”标识，`false` 表示关闭，不能将其表述为关闭后仍添加该标识。平台遵用户要求显式传入 `false`。
- `prompt_extend` 官方默认 `true`，扩写模式默认 `direct`。
- `enable_thinking` 官方默认 `true`，仅在 `prompt_extend=true` 时生效，开启会增加耗时。
- 官方 `model-pricing` 已成功读取，但未明确本轮涉及四个型号的 extend/thinking 附加费条款。按张计费说明不足以证明这些选项免费。
- 平台保守默认 `prompt_extend=false`、`enable_thinking=false`；扩写模式保留 `direct`。费用核实后可由后台调整，用户端隐藏这些字段。

### Seedream

官方模型页：<https://console.volcengine.com/ark/region:cn-beijing/docs/seedream-5-0-pro>。

计划列出的官方价格来源：<https://ark.volcengine.com/docs/model-pricing>；本稿不据此新增费用已确认的结论。

- 优化参数为 `optimize_prompt_options.mode`。Pro 支持 `standard` / `fast`，Flash 仅支持 `standard`，官方默认 `standard`。
- 官方没有 `off` 值。平台保留 `standard`，隐藏用户配置字段，不构造 `off`、`none` 或 `false` 作为厂商关闭值。
- 优化附加费未知，后台及后续结果应明确标注待确认。这是无法通过当前官方参数严格关闭的厂商约束，不能表述为已关闭优化或已确认免费。
- `watermark=true` 表示右下角“AI生成”标识；平台遵用户要求显式使用 `false`。

已核验接口中的水印为布尔开关，没有自定义文字、Logo 或位置字段；平台不声明自定义水印能力。

当前框选、画笔及圈画属于 **Seedream 图片**链路。用户此前称为 Seedance 的操作应按这一实际范围记录，不能据此声明 Seedance 视频已支持该能力。

## defaults 契约

后台默认语义已确定：创建新任务时解析并冻结 effective options，API 显式参数覆盖后台默认。以下为配置契约；实现检查结果见后文，具体覆盖范围以用户提供的证据为限。

```text
extra.native_image.defaults = {
  seed?,
  watermark?,
  qwen?: {
    prompt_extend?,
    enable_thinking?,
    prompt_extend_mode?: direct | agent
  },
  seedream?: {
    optimize_prompt_mode?: standard | fast
  }
}
```

- 模型 defaults 继承账号 defaults；模型级覆盖账号级，嵌套字段合并、校验和未知键保留行为需验证。
- 平台基线：固定种子可选，留空随机；`watermark=false`；Qwen 扩写与思考均为 `false`，模式为 `direct`；Seedream 优化为 `standard`，附加费未知。Flash 不应接受 `fast`。
- API/MCP 显式 `options` 覆盖后台 defaults；创建新任务时，将合并后的 effective options 冻结到任务 snapshot，后续后台默认变更不得回写已创建任务的冻结参数。报价、能力版本及幂等指纹需考虑实际影响字段。
- 用户 UI 不再提交隐藏参数，历史 draft 中的 seed、水印、优化和思考值不得继续静默影响新 UI 请求。API/MCP 显式参数兼容与 UI 历史字段清理需分别验证。

## 实现与检查状态

以下结果由用户及主线程提供，本次仅更新报告，未重新执行测试、浏览器检查、secret 扫描或独立复核。verify、smoke、review 的 PASS 与 gate 的 BLOCK 分别记录；真实浏览器验收通过的范围及未执行链路在后文明确列出。

| 范围 | 当前记录 | 证据及限制 |
| --- | --- | --- |
| backend/defaults | 局部测试通过；新任务冻结 effective options、API 显式覆盖语义已定 | 本稿记录用户提供的结果 |
| backend review | “不确定响应重复付费”P1 已修复关闭 | 不据此声明真实付费链路已测试 |
| admin 可视化配置与计费表单 | 局部测试及 build 通过；“型号/币种重标费率”P2 已修复关闭 | 本稿记录用户提供的结果 |
| layerprice 公开字段 | 已完成 | 具体字段及证据位置可由主线程补充 |
| full verify | **PASS**，最终 exit 0 | 日志：`/tmp/native-parameter-ux-verify-final.log` |
| 隔离 API smoke | **PASS**，exit 0；fake provider `127.0.0.1:51430` | 隔离 fake 调用，不代表真实付费供应商链路验证 |
| review | **PASS**；user review 4 项 P2 全部关闭，独立 contract PASS | backend P1、admin P2 关闭状态如上 |
| gate | **BLOCK**：heavy pending | 不得将 verify、smoke、review PASS 表述为 gate PASS |
| secret 检查 | 2 处匹配均为 fixture，无真实 secret（用户提供结论） | 本次报告更新未读取 secret 或重新扫描 |
| Canvas 真实 graph UI | **PASS（本轮实测范围）**：真实 8 node 画布完成引用添加、点标记保存与重载持久化、切型号清标记及负向提示词检查 | 白屏源于开发依赖缓存；未执行画笔上传后端链路 |
| 最后诊断清理 | main 临时诊断已移除，diff empty；移除后 typecheck PASS | full verify exit 0 时已包含最新 refresh 修改 |
| public pricing confirmed | 输出严格 boolean 的确认已完成 | 不将 truthy 值等同于严格布尔确认 |

已关闭的 user review 4 项 P2：删除引用时的 pending 状态、排序索引、上传 race（竞态）、自动换型时的原图处理。独立 contract 已 PASS。

首轮 full verify 曾在 Go test / vet 通过后，因 shared `native-image.contract` 相对加载 `ReferenceMarkModal` 失败而中断；该首轮失败记录保留为历史，最终 verify 已 exit 0，不再列为当前待修复项。

## 引用标记弹窗浏览器检查结果

以下为用户提供的最终浏览器确认，本次未重新执行浏览器测试：

- modal 失败时正确保留 draft。
- 已有 refresh button；点击后图片成功 loaded，记录尺寸值为 `1024`。此前缺少刷新入口的问题不再列为待补入口。
- 此前桌面框选 `(176, 270) → (754, 686)` 保存后显示 saved badge 已确认。
- 最终检查确认框选 1 个、圈画 1 个，合计 2 个标记。闭合圈画显示 bounds `(291, 349)–(638, 604)` / `8 points`，已使用范围和点数表达。
- 已实际点击“一键清空”：标记数量变为 `0`，状态为未保存；再保存后，“已编辑”badge 为 `0`。该结果取代此前“清除所有尚未点击”的记录。
- 此前 `390` 宽度样式检查记录为图标按钮 2 列；最终刷新后的样式未再次截图，不宣称已有最终手机截图证据。

上述创作 modal 的最终 refresh/clear 操作已通过；不据此扩展声明未执行的画笔上传后端链路通过。

## Canvas 真实 graph UI 最终验收

根据用户及主线程提供的最终实测记录，本轮真实浏览器验收通过：

- 先前打开真实 8 node graph 的白屏，在仅重启端口 `5273` 的开发服务并使用 `--force` 后消失。根因为 4 个依赖请求返回 `504 Outdated`，与 metadata hash 不一致有关，并非产品 hook 问题。typecheck/build 此前未检出该开发缓存故障。
- main 中临时诊断已移除，diff empty；最后移除后 typecheck PASS。此前 full verify exit 0 已覆盖最新 refresh 修改，日志为 `/tmp/native-parameter-ux-verify-final.log`。
- 在用户标记为 `db10..` 的既有真实 8 node 测试画布中，选择“图片3”Seedream 节点，添加已有 Pokemon 图片引用。
- modal 中添加点标记 `(508, 487)` 后保存，显示“已编辑”badge；后端记录为 `r401`。reload 后重新选择该节点，仍显示“已编辑”，持久化实测通过。
- 切换到 Qwen3 后“已编辑”badge 消失，标记已清理；该引用图片保留。
- Qwen3 的负向提示词默认 collapsed，位于主 prompt 下；展开后复用 `PromptTemplateEditor`，包含 placeholder 与 `0/4000` 计数。

测试数据变更：本次在既有测试画布新增 1 个已有图片引用，并添加点标记；随后切换 Qwen 清除标记，最终保留该引用。本轮无付费调用，未通过浏览器执行画笔上传后端链路。

后台布局证据：platform defaults 的布局已实测良好；profile 后续表单只有 contract 检查证据，没有截图，不宣称已完成该表单的浏览器视觉验收。

## 完成结果与验收边界

- [x] 最终 verify PASS，exit 0；日志为 `/tmp/native-parameter-ux-verify-final.log`。
- [x] 隔离 API smoke PASS，exit 0，fake provider 为 `127.0.0.1:51430`。
- [x] review PASS，user review 4 项 P2 全部关闭，独立 contract PASS；backend P1、admin P2 已修复关闭。
- [x] 浏览器确认刷新后图片 loaded、桌面框选 saved badge、框选与圈画计数、圈画 bounds + point count，以及清空到 0、未保存、再保存后的 badge 0。
- [ ] gate 解除 BLOCK：heavy pending，等待主线程后续结果。
- [x] Canvas 真实 8 node graph 本轮浏览器验收通过：引用添加、点标记保存与 reload 持久化、切 Qwen 清标记及负向提示词控件表现已确认。
- [x] 白屏已定位为开发依赖缓存问题，重启 5273 并使用 `--force` 后消失；main 临时诊断移除、diff empty，最后 typecheck PASS。
- [ ] 画笔上传后端链路未通过浏览器执行；不声明该链路通过。
- [ ] 最终刷新后的 `390` 宽度样式未再次截图；后续如补截图应单独记录，不沿用此前截图宣称最终状态。
- [ ] 主线程如需逐条映射 defaults 继承与覆盖、snapshot、报价/能力版本/幂等指纹、历史 UI 字段清理、自动 operation、标记隔离及 admin extra 无损保存等验收项，应补充对应测试证据；本稿不从汇总 PASS 推测各项具体覆盖范围。
- [ ] Qwen 四个型号 extend/thinking 及 Seedream 优化附加费仍缺少明确官方证据，维持上述保守策略与未知标记。

## 本轮最终验收边界

本轮报告已收尾，已执行范围内验收通过：verify、隔离 smoke、review 均 PASS，创作 modal refresh/clear 及 Canvas 真实 graph UI 的所列操作通过。gate 仍 BLOCK（heavy pending），不表示已获发布或推送门禁通过。

本轮无付费模型调用。图片编辑标记及上传测试使用已有图片（existing 图片），允许产生本地新资产；Canvas 本轮执行的是已有引用添加与点标记持久化等所列操作，未通过浏览器执行画笔上传后端链路，不宣称真实付费生成、模型编辑或该上传链路通过。最终 `390` 宽度样式未复拍，profile 后续表单仅有 contract 证据，这些均保留为验收边界。

## 本次文档整理边界

本次仅维护此报告，不修改需求计划或实现文件，不执行付费调用，不读取 secret，不提交。实现与验收结论来自用户及主线程提供的结果；本次未重新执行检查，未将未执行项目记为通过。
