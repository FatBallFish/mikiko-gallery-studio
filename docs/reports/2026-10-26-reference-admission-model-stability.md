# 引用准入与模型稳定选择修复报告

> 阶段性历史声明：本文正文记录本阶段结束时的实现、验证、运行状态及审批事实；其中“当前”“API 尚未恢复”“浏览器 BLOCKED”“heavy approval pending”等表述仅适用于当时，不作为本轮发布的实时阻碍清单，也不追溯改变历史验证结果或旧审批。后续进展请分别参阅[本地服务恢复报告](2026-10-26-local-service-recovery.md)、[统一路由图片定价报告](2026-10-26-unified-route-image-pricing.md)及[Seedream 拆层 capability_changed 修复报告](2026-10-26-seedream-layer-capability-failure.md)；后续恢复或修复不自动补齐本文未执行的浏览器或真实付费验收，各报告仍以其所列验证范围和限制为准。本轮发布授权单独记录于[当前全部变更发布与生产升级](../plans/2026-10-26-production-release-native-image.md#授权)，不代表此前各阶段已获追溯审批。

日期：2026-10-26。

来源：[同名实施计划](../plans/2026-10-26-reference-admission-model-stability.md)。

状态：**代码验证 PASS、隔离 API smoke PASS、独立 review PASS；浏览器验收与本地 API 恢复 BLOCKED，review gate 因 heavy approval pending 为 BLOCKED。** 全量 verify 实际 exit 0，日志：`/tmp/reference-admission-verify-complete.log`，覆盖 Go 测试、vet、全部 contracts、user/admin/docs 类型检查与构建，以及 native Linux/Windows package。代码已实现并 freeze；本地测试实例 API 尚未恢复，live 修复未验证。

## 用户五点要求

1. **引用在加入前准入。** 图片创作、视频创作和画布控制台应在上传、资产选择、连接之前拒绝不支持类型及超上限引用，并通过 Toast 说明原因。组合数量与并发上传按当前有效能力校验，不能先加入再截断或删除；已有不兼容引用应保留，禁用生成并说明处理方式。
2. **显式型号选择稳定。** 能力变化、引用变化和估价失败不得触发自动切换到 GPT 或其他型号。仅首次无选择时可选默认；失效、下线、无权限或不支持当前操作时，保留选择、展示原因并禁止生成，覆盖图片、视频和画布。
3. **图片操作能力分开判定。** 自动生成或编辑按当前引用判定，不能因此替换用户选择集合。区分 generate、edit、layer profile；缺失或未知能力不能误判为引用上限 0，更不能据此删除引用。
4. **参数展开按钮动作真实有效。** 移动端保持有效；桌面端若无实际效果，应移除桌面图标或使动作真正生效，不保留无效按钮。
5. **Seedream 拆层入口与可测状态准确。** 需要能力 profile 操作支持和已确认成本、售价；用户控制台应有独立可见入口，条件不足时明确展示原因，不能默默隐藏。普通参考编辑仍可导入，不得误用拆层引用上限。本轮不触发付费拆层实测。

## 根因与实现状态

| 范围 | 已查根因 / contract 问题 | 修复目标 |
| --- | --- | --- |
| Workspace 型号与引用 | 按 task 过滤模型后触发 fallback；引用路径存在 `slice(0, …)` 截断处理 | 显式选择保持稳定；按当前 model + task/profile 计算能力，在加入前拒绝；已有不兼容引用保留并阻断生成 |
| user-api 能力投影 | 漏传 layer confirmed 状态 | 用户端获得准确拆层确认状态；缺失或未确认时说明阻断原因，不推断为可用 |
| 视频公开 contract | 缺失逐 task/role 的 input capability，无法准确表达各任务、角色的引用准入约束 | 公开契约、类型及用户端处理一致，按 task/role 校验输入能力与上限 |

视频根因为逐 task/role 的 input capability 缺失；此前报告误写的 `missing_role_count` 字段不存在，不属于本次公开 contract。

代码已实现并全部 freeze。Workspace 3 个 P2、Canvas/video 3 个 P2 均经独立复核关闭，独立 review PASS；全量 verify exit 0，代码验证 PASS。初次验证曾 timeout，旧 contract 已修复，最终完整重跑通过。浏览器验收因本地 API 恢复阻塞为 BLOCKED。

拆层入口初始缺失字段已证实来自旧 API；normalizer 已补。准确准入口径为能力（profile ops + layer flag）、price confirmed（cost/sale 均 confirmed）和 1 个引用，同时满足后才可放行。字段补齐不等于浏览器行为已验收。

## 本地测试实例 API 不可用与浏览器验收阻塞

本节涉及本地测试实例，不涉及 production 部署。原 `/tmp/e2e-config/runtime.env` 与 `/tmp/e2e-config/install-state.json` 均已缺失。主线重启原 API 前未确认这两个文件存在，导致本地测试实例原 API 当前不可用。

新 API 已 build，但新 API 的 8080 启动 fail closed，报 `BOOTSTRAP_CONFIG_INVALID`；该结果不能作为原 8091 API 启动成功或恢复成功的证据。该失败进程 PID `80724` 已通过 TERM 停止，未遗留 8080 错误 bootstrap 进程。前端 `5273`、admin `5274` 仍运行，但无 `8091` API；前端进程存活不代表端到端可用。

已只读检查 6 个候选与 DB binding 的配对，均无 match，且无官方 hash 还原途径。**不得修改 binding。** 本地测试实例恢复需原配对备份：`/tmp/e2e-config/runtime.env` 和 `/tmp/e2e-config/install-state.json`；不得通过改 binding 或擅改配置绕过配对校验。

**浏览器验收当前为 blocked，live 修复未验证。** 隔离 API smoke exit 0 不代表原 8091 恢复，也不能替代实际浏览器验收；待原配对备份及恢复结果确认后，才可补充浏览器验证结论。

## 拆层如何测与验收条件

拆层可测试状态必须同时核对以下条件，不能只凭型号名称或入口出现判定可用。

- **能力条件：** profile 的 ops 明确支持拆层，且 layer flag 满足拆层启用条件；generate/edit/layer 分别判定，不用普通编辑能力替代拆层能力。
- **价格条件：** cost 与 sale 均为 confirmed。unknown 价格不得转换为 0，不得据此展示免费或放行，也不得为通过验收擅改配置。
- **输入条件：** 拆层使用 **1 个引用**；普通参考编辑使用自身能力上限，不套用拆层的单引用限制。
- **入口条件：** 用户控制台提供独立拆层入口；条件不足时展示具体原因，保留用户当前型号选择。
- **结算条件：** 按实际结果结算。实际结果数量、计费依据与最终结算一致性需补证据，不能以请求数量或估价代替最终结果。

### NativeControls 文案 contract 与待执行步骤

NativeControls 的真实按钮 label 为 **「图层拆分」** / **「退出图层拆分」**；status 顺序为 **「拆层需要一张参考图」→「成本售价需确认」→「独立功能提交有效报价」**。此处为代码 contract 记录，入口及状态尚未经浏览器实测。

恢复 API 后，浏览器无付费验收按以下准确 label 执行，目前各步骤均未执行、状态为 blocked。

1. 在能力满足 profile ops + layer flag 的型号下，检查「图层拆分」入口及进入后「退出图层拆分」的切换，确认保留显式型号选择。
2. 在拆层模式下引用未满足 1 张时，核对 status「拆层需要一张参考图」及提交阻断；正常参考编辑仍按自身上限处理。
3. 满足 1 张参考图但 cost/sale 未 confirmed 时，核对 status「成本售价需确认」及提交阻断；unknown 价格不作 0，不修改配置。
4. 能力、cost/sale confirmed、1 张参考图均满足时，核对 status「独立功能提交有效报价」及有效报价状态，仅检查 UI 和报价，不触发 paid 提交。
5. 通过「退出图层拆分」返回，检查模式切换及引用、型号保留行为。按实际结果结算仍需独立结果证据，不能由入口或报价状态推定通过。

本轮**无 paid 调用、无付费拆层实测、未擅改配置**。拆层入口的浏览器行为和实际结果结算证据待补，不宣称拆层端到端通过。

## 最终验证记录

| 验证项 | 状态 | 结果 / 验收边界 |
| --- | --- | --- |
| 图片准入：不支持类型、0/满额、混合批次、并发上传、预约与去重 | 已实现，代码验证 PASS；浏览器 BLOCKED | 拒绝发生在加入前，Toast 原因、原列表保留情况未经浏览器验收 |
| 型号稳定：引用/能力变化、估价失败、无效或无权限型号 | 已实现，代码验证 PASS；浏览器 BLOCKED | 图片、视频、画布保留显式选择及阻断原因未经浏览器验收 |
| generate/edit/layer 能力隔离与未知能力处理 | 已实现，代码验证 PASS | profile 对应关系、无错误 max 0 推断、无截断删引用 |
| 视频逐 task/role input capability 与引用上限公开 contract | 已实现，代码验证 PASS | 按任务、角色校验输入能力，公开契约与用户端处理一致 |
| Canvas 连接、上传完成回调重校验 | 已实现，代码验证 PASS；浏览器 BLOCKED | 当前目标与引用签名变化时的实际行为未经浏览器验收 |
| 桌面/移动端参数展开与收起 | 已实现，代码验证 PASS；浏览器 BLOCKED | 按钮可见性及动作效果未经浏览器验收 |
| NativeControls 按钮与 status 文案 contract | 代码验证 PASS；浏览器 BLOCKED | 「图层拆分」/「退出图层拆分」；「拆层需要一张参考图」→「成本售价需确认」→「独立功能提交有效报价」 |
| 拆层 profile ops + layer flag、cost/sale confirmed、1 引用、独立入口 | 浏览器 BLOCKED，入口未实测 | 各条件满足与不满足时的行为、明确阻断原因未实测；恢复后按上述准确 label 验收 |
| 拆层按实际结果结算 | 未做 paid 实测 | 无实际付费结果结算证据，不宣称付费端到端通过 |
| 本地测试实例 API 恢复 | BLOCKED | 缺失原配对备份 `/tmp/e2e-config/runtime.env` 与 `/tmp/e2e-config/install-state.json`；6 个候选与 DB binding 均无 match，无官方 hash 还原途径，不得改 binding |
| 浏览器无付费验证：图片、视频、画布、Toast、参数按钮、拆层入口 | BLOCKED，live 修复未验证 | 原 8091 当前不可用；新 8080 fail closed 不能视作 8091 成功；失败进程已 TERM 停止 |
| 全量 verify / release 检查 | PASS，实际 exit 0 | 日志：`/tmp/reference-admission-verify-complete.log`；Go 测试、vet、全部 contracts、user/admin/docs 类型检查与构建、native Linux/Windows package 全部通过 |
| diff check | PASS | 不替代 review gate |
| review-local gate（`--scope all`） | BLOCKED，exit 1，仅 heavy approval pending | 不视为 committed-scope PASS；无 commit |
| 独立代码复核 | PASS，所列 P2 全部关闭 | Workspace 3 个 P2、Canvas/video 3 个 P2 均经独立复核关闭 |
| 隔离 API smoke（后端改动适用） | PASS，exit 0 | 日志：`/tmp/reference-admission-api-smoke.log`；不代表原 API 恢复或 live/browser 验收通过 |

## 收尾状态

**代码完成并 freeze，代码验证 PASS、隔离 smoke PASS、独立 review PASS。** 全量 verify 已实际 exit 0，不再有运行中的验证结果。review gate 因 heavy approval pending 为 BLOCKED；浏览器验收及本地测试实例 API 恢复为 BLOCKED，live 修复未验证。

本地测试实例恢复的实际阻塞是缺失原配对备份 `/tmp/e2e-config/runtime.env` 和 `/tmp/e2e-config/install-state.json`。需要这两个文件的原配对备份才能继续恢复，不得修改 DB binding 绕过校验；此事不涉及 production 部署。

本轮无 paid、无 commit、无 push、无部署，未修改 DB binding。报告维护不修改代码或 `.coding-context.json`。代码验证通过与运行实例恢复、浏览器验收分别记录，不将整体任务标记为全部完成。
