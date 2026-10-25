# 原生图片真实 E2E 与移动端实施记录

> 阶段性历史声明：本文正文记录本阶段结束时的配置、真实任务、产物、验证及审批事实；其中“当前”“待确认”“发布门禁未通过 / heavy approval.pending”等表述仅适用于当时，不作为本轮发布的实时阻碍清单，也不追溯改变历史验证结果或旧审批。后续进展请分别参阅[本地服务恢复报告](2026-10-26-local-service-recovery.md)、[统一路由图片定价报告](2026-10-26-unified-route-image-pricing.md)及[Seedream 拆层 capability_changed 修复报告](2026-10-26-seedream-layer-capability-failure.md)；本文真实任务及产物证据不扩大为后续实现的真实付费验收，各报告仍以其所列验证范围和限制为准。本轮发布授权单独记录于[当前全部变更发布与生产升级](../plans/2026-10-26-production-release-native-image.md#授权)，不代表此前各阶段已获追溯审批。

日期：2026-10-25。

本报告记录当前实际配置、从用户 UI 提交的 8 个真实任务、本地产物检查结果与移动端验收进展。配置、任务与运行状态依据本轮执行记录；产物类型、尺寸、字节数与 SHA-256 通过本地文件检查取得。

## 当前实际配置

| 项目 | 当前值 |
| --- | --- |
| 图片供应商账号 ID 3 | Qwen，北京，`qwen_image` |
| 图片供应商账号 ID 4 | Ark，`seedream_image` |
| 输出模式 | single output（单图输出） |
| 拆层 | 价格未确认，保持禁用 |
| Secret 来源 | `api-keys.env`；密钥内容不纳入报告 |
| 运行时兑换率 FX | **0.3125 CNY/point** |
| UI 兑换率文案 | 仍显示 **0.01**，与运行时口径不一致，待确认 |

模型路由候选与型号对应如下。候选 ID 为路由候选 ID，不是供应商账号 ID。

| 路由候选 ID | 供应商账号 ID | 型号 / upstream_model_code |
| --- | --- | --- |
| 3 | 3 | Qwen 3.0 / `qwen-image-3.0` |
| 4 | 3 | Qwen 2.1 Pro / `qwen-image-2.1-pro` |
| 5 | 3 | Qwen 2.1 Turbo / `qwen-image-2.1-turbo` |
| 6 | 3 | Qwen 3.0 Pro / `qwen-image-3.0-pro` |
| 7 | 4 | Seedream 5 Pro / `doubao-seedream-5-0-pro-260628` |
| 8 | 4 | Seedream 5 Flash / `doubao-seedream-5-0-flash-260915` |

8 个任务的 `upstream_model_code` 均与预期型号匹配。

## 真实用户 UI 提交与结算

8 单均从用户 UI 提交：6 单生成、2 单参考编辑；两次编辑均使用 1 张参考图（1 ref）。结果为 **7 成功、1 超时退款**。首单等待 600 秒后 timeout，未重试。

点数列单位为 point；成本列单位为 CNY。成本为当前任务记录口径，供应商账单尚未核对。超时单 actual 0 表示本地实际消费点数为 0，不代表已确认供应商未计费。

| Task ID | 型号 / 操作 | 参考图 | 结果 | Reserve | Actual / Consume | Refund | 记录成本 CNY |
| --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| `d11b0a75-3aa4-52c4-887e-4913a1d3c6d1` | Qwen 3.0 生成 | 0 | 600 秒 timeout，未重试 | 0.960 | 0 | 0.960 | 未核对 |
| `e837a743-91b7-5a3b-b43f-950f633bd19c` | Qwen 2.1 Pro 生成 | 0 | 成功 | 1.248 | 1.248 | 0 | 0.25 |
| `244f483f-389f-5dcf-9570-4a05eecf9cd1` | Qwen 2.1 Turbo 生成 | 0 | 成功 | 0.640 | 0.640 | 0 | 0.10 |
| `f781e9fb-74c6-57fc-93c9-3b36179ca7dc` | Qwen 3.0 Pro 生成 | 0 | 成功 | 1.248 | 1.248 | 0 | 0.25 |
| `190213f1-29eb-5080-bc59-f9c04f59df14` | Seedream 5 Pro 生成 | 0 | 成功 | 1.408 | 1.408 | 0 | 0.30 |
| `b918f962-5f55-54e4-970d-a002d393c16c` | Seedream 5 Flash 生成 | 0 | 成功 | 0.736 | 0.736 | 0 | 0.12 |
| `ba193c07-e5b7-5993-a7a8-68d38457ad3f` | Seedream 5 Pro 编辑 | 1 | 成功 | 1.408 | 1.408 | 0 | 0.30 |
| `529d73d8-37fd-543b-98c9-dd68cf556cf4` | Qwen 2.1 Pro 编辑 | 1 | 成功 | 1.248 | 1.248 | 0 | 0.25 |
| **合计** | **8 单** | **2** | **7 成功，1 超时退款** | **8.896** | **7.936** | **0.960** | **1.57（成功单记录成本）** |

钱包与流水记录：

- 可用余额：`520 → 512.064` point，减少 `7.936` point，与总消费一致。
- 最终冻结：`0`。
- Ledger 共 `16` 条：`8 reserve + 7 consume + 1 refund`。
- 预留核对：`8.896 − 0.960 = 7.936` point。
- 按运行时 FX 折算，消费点数对应 `7.936 × 0.3125 = 2.480 CNY`；该值是用户消费折算额，供应商记录成本合计为 `1.57 CNY`，两者口径不同。

## 本地产物检查

产物位于 `docs/reports/artifacts/`，文件名包含 Task ID。使用 `file` 检查 magic、`sips` 检查像素尺寸、Python 标准库计算文件字节数及 SHA-256。7 个成功单产物均为 `1024 × 1024`；超时单未发现对应文件。

| Task ID | 本地产物 | Magic / 类型 | 尺寸 | 字节数 |
| --- | --- | --- | --- | ---: |
| `d11b0a75-3aa4-52c4-887e-4913a1d3c6d1` | 无对应产物 | — | — | — |
| `e837a743-91b7-5a3b-b43f-950f633bd19c` | [Qwen 2.1 Pro](artifacts/qwen-image-2.1-pro-e837a743-91b7-5a3b-b43f-950f633bd19c-816c0d66-d1fa-53de-b47f-ecea4d16a6c4.png) | PNG，8-bit RGBA，非交错 | 1024 × 1024 | 1,913,255 |
| `244f483f-389f-5dcf-9570-4a05eecf9cd1` | [Qwen 2.1 Turbo](artifacts/qwen-image-2.1-turbo-244f483f-389f-5dcf-9570-4a05eecf9cd1-3233e86c-2f87-5e76-870b-394e7067674d.png) | PNG，8-bit RGBA，非交错 | 1024 × 1024 | 1,844,029 |
| `f781e9fb-74c6-57fc-93c9-3b36179ca7dc` | [Qwen 3.0 Pro](artifacts/qwen-image-3.0-pro-f781e9fb-74c6-57fc-93c9-3b36179ca7dc-cb6757ec-7095-5354-a88b-322ab3025db6.png) | PNG，8-bit RGB，非交错 | 1024 × 1024 | 1,790,588 |
| `190213f1-29eb-5080-bc59-f9c04f59df14` | [Seedream 5 Pro](artifacts/doubao-seedream-5-0-pro-260628-190213f1-29eb-5080-bc59-f9c04f59df14-dedf69b1-9536-5553-8643-3a936faf6d19.jpg) | JPEG / JFIF 1.01，baseline，8-bit，3 components | 1024 × 1024 | 261,740 |
| `b918f962-5f55-54e4-970d-a002d393c16c` | [Seedream 5 Flash](artifacts/doubao-seedream-5-0-flash-260915-b918f962-5f55-54e4-970d-a002d393c16c-ff2ef614-1167-593a-a665-1f874daa9da2.jpg) | JPEG / JFIF 1.01，baseline，8-bit，3 components | 1024 × 1024 | 401,695 |
| `ba193c07-e5b7-5993-a7a8-68d38457ad3f` | [Seedream Pro 编辑](artifacts/seedream-pro-edit-ba193c07-e5b7-5993-a7a8-68d38457ad3f-1a4a0a48-7ed9-51ea-ad57-2d0b7a76dd03.jpg) | JPEG / JFIF 1.01，baseline，8-bit，3 components | 1024 × 1024 | 280,254 |
| `529d73d8-37fd-543b-98c9-dd68cf556cf4` | [Qwen Pro 编辑](artifacts/qwen-pro-edit-529d73d8-37fd-543b-98c9-dd68cf556cf4-afbe4316-ab48-5790-b3b2-5ae6878888a7.png) | PNG，8-bit RGBA，非交错 | 1024 × 1024 | 1,942,148 |

| Task ID | 文件 SHA-256 |
| --- | --- |
| `e837a743-91b7-5a3b-b43f-950f633bd19c` | `d414208b132b1284566717322c78c794fbeb1d0920767ddb95228930dcb1e064` |
| `244f483f-389f-5dcf-9570-4a05eecf9cd1` | `1b9d8a5cfe6ced8630cd8427cc9a1e26aaefc64d55bf7c3600f6a1dc26cae3af` |
| `f781e9fb-74c6-57fc-93c9-3b36179ca7dc` | `cb3e936b46ed9b20e16e3bdb2a1a785bd548f035e5804a0f5d88e6414f8a639b` |
| `190213f1-29eb-5080-bc59-f9c04f59df14` | `794270cc0aff219ac75fe2ff42693f423d87817d32c16a3a63b704e0ed0162a5` |
| `b918f962-5f55-54e4-970d-a002d393c16c` | `7800010e3cbc2da7387c073822a5d51c407221a9f83fc4a0792b2d9ee0421485` |
| `ba193c07-e5b7-5993-a7a8-68d38457ad3f` | `e9e717badfa6adb8ffeb724c09de6b9c9ae2061748070b53531c96c99cbe6d1a` |
| `529d73d8-37fd-543b-98c9-dd68cf556cf4` | `86fdfa6d085ef80820ae76e3f5fc9b22ecb7cdf74a964930f9e374723a664a57` |

两张参考编辑产物均已实际查看：Qwen 与 Seedream 的结果均可见黄昏光线和绿植变化，同时保留鹈鹕与藤椅，变化符合本次编辑要求；此结论限于上述可见内容，未进行全图逐项评审。Qwen 编辑结果最终页面图片 `complete=true`，`naturalWidth=1024`、`naturalHeight=1024`。浏览器最终恢复为 1280×900 浅色。

## 移动端实现与验收进展

本轮移动端实现已做，已取得以下真实浏览器证据。菜单短屏裁切与 video 屏外问题已修复并复测通过，局部验证、限定范围的独立最终代码复核及完整 verify 均通过；发布门禁仍未通过。

实现范围：

- 手机创作操作台背景接近不透明，覆盖浅色 / 深色、安全区及短屏适配。
- 上传队列沿用现有业务并保持常驻；收起为 48px 单图标浮标，待处理任务角标不计完成 / 取消任务。
- 浮标支持二维拖动、左右贴边半隐藏、点击 / 聚焦唤出；处理 pointer capture、pointer cancel 和拖动后的 click 抑制。
- 按用户本地保存浮标位置；窗口 resize 与 visualViewport 变化时重新约束可见范围。
- 展开队列按浮标位置确定面板几何并限制高度、保持滚动；画布编辑时隐藏展示，任务保持挂载。菜单短屏裁切与 video 面板屏外问题已完成修复及下述复测。

对应需求设计见 [真实原生生图与手机浮动上传需求设计](../plans/2026-10-25-native-image-real-e2e-mobile-upload.md)。

### 已取得的真实移动端证据

以下为本轮实际浏览器操作记录，仅对列明的场景确认结果。

| 视口 / 场景 | 实测结果 |
| --- | --- |
| 320×568，浅色 | 操作台展开背景不透明，通过 |
| 375×667，浅色 | 操作台展开背景不透明，通过 |
| 390×844，深色，图片模式 | 操作台展开背景不透明，通过 |
| 667×375，横屏 | 操作可触达，resize 后位置 clamp 通过 |
| 375 宽视口，queue 交互 | 点击展开队列、打开操作台时队列自动收起，通过 |
| 390 宽视口，深色，queue 展开 | 已观察截图：队列完整展开可见，未触及底部导航 |
| 浮标几何 | 浮标尺寸 48px；左贴边 `x = -15`；390 宽视口记录 `y = 71` |
| 390 宽视口，真实 drag 路径 | 拖动后左侧半隐藏通过，未误打开队列 |
| 320×568，深色，Portal 模型菜单 | 末项 scrollIntoView 后 rect `y=442.9, h=57.2`；截图确认无 footer 裁切或浮标遮挡；CUA 真实屏幕点击 `(150,467)` 成功选中 Flash |
| 320 宽视口，修复后 video 面板 | rect `x=0, y=38.24, w=314, h=465.76`，背景 `rgb(12,14,21)`；底部 `504 < 568`（导航边界），截图按钮可见，通过 |

普通 `Playwright.click` 在模型菜单场景仍出现适配器 timeout；该调用未通过。上述 Flash 选择通过来自 CUA 真实屏幕点击。

### 浏览器限制与验证状态

IAB 不支持文件上传，因此实际上传队列角标及暂停 / 失败态无法完成完整浏览器实测；这些状态由 contract 覆盖，不记为浏览器实测通过。390 深色队列完整展开可见且未触底部导航已有截图观察；完整队列滚动、画布隐藏时持续上传、聚焦 / pointer cancel 未取得逐项浏览器实测证据，不扩大已通过场景的结论。

| 检查项 | 当前状态 / 证据 |
| --- | --- |
| 真实用户 UI 8 单与结算 | 已记录：7 成功、1 超时退款，见任务及钱包表 |
| 成功单本地产物 magic / 尺寸 / hash | 已完成文件检查，见产物表 |
| 移动端操作台背景、横屏可触达与 resize clamp | 已取得上述视口的真实浏览器证据 |
| queue 点击展开 / 打开操作台自动收起 | 375 宽视口实测通过 |
| queue 完整展开可见、未触底部导航 | 390 深色截图已观察 |
| 真实拖动、左侧半隐藏与拖后不误开 | 390 宽视口实测通过 |
| 上传队列角标、暂停 / 失败态 | IAB 文件上传能力限制，无法完整浏览器实测；contract 覆盖 |
| 聚焦唤出、pointer cancel、完整队列滚动、画布隐藏时持续上传 | 未取得逐项浏览器实测证据，不计入实测通过 |
| 菜单短屏裁切、video 屏外 | 已修复并复测通过，见上表；普通 Playwright.click 的适配器 timeout 仍保留 |
| 局部代码修复验证 | 局部验证均通过；完整 verify 结果另列 |
| `scripts/workflow/verify.sh` | **PASS，exit 0**；所有 contracts、Go test / vet、user / admin / docs typecheck / build 及脚本 native package 检查通过；最终日志 `/tmp/native-real-mobile-verify-final-pass.log` |
| mcpProxy fixture 验证稳定性修复 | 最终 diff 只读复核通过，未见 P1 / P2；清理与断言细节见下文 |
| 独立最终代码复核 | **通过（限定范围）**：独立只读复核 ModelGroupSelect、VideoCreationPanel、Workspace raw nativeDraft 及直接依赖，无可证新增 P1 / P2 |
| 发布门禁 | **未通过**：`review-local --scope all` exit 1，唯一 BLOCK 为 `heavy approval.pending` |
| API 隔离 smoke | **通过，exit 0**；隔离地址 `http://127.0.0.1:55434`，日志 `/tmp/native-real-mobile-smoke-final.log` |
| 供应商账单核对 | 尚未核对，包括首单超时后供应商是否计费 |
| UI 0.01 与运行时 FX 0.3125 的展示口径 | 尚待确认 |

### 验证稳定性修复

此前 full verify 在旧 mcpProxy fixture 断言通过后挂于清理阶段：Vite 的 `watch: null` 合并未生效，导致 fsevents 残留。该挂起为 fixture 清理问题，未判定为产品失败。

fixture 已作最小修复，最终 diff 经只读复核，未见 P1 / P2：无 `process.exit`；resolved watch 断言、stream release、两段 close 与 env restore 均位于 `finally`，OK 标记置于最后。修复后完整 verify 已实际 **PASS，exit 0**，所有 contracts、Go test / vet、user / admin / docs typecheck / build 及脚本 native package 检查通过，最终日志为 `/tmp/native-real-mobile-verify-final-pass.log`。

## 安全审计与发布门禁

- `api-keys.env` 已 ignored 且未跟踪；`.api-keys.env.swp` 未跟踪，未读取、未删除。
- 安全审计覆盖 63 项 diff 与 112 份代码 / 报告文本，未确认真实 secret 泄露。
- context 两个来源均为本轮 plan。
- `review-local --scope all` 返回 exit 1，唯一 BLOCK 为 `heavy approval.pending`；发布门禁未通过。独立最终代码复核通过不等于发布门禁通过。
- 本轮未请求 commit / push。

## 运行收尾与边界

- 临时 Worker PID `22126` 在任务完成后已发送 TERM，退出码 `0`。
- API 仍运行于端口 `8091`。
- 首单 timeout 未重试。
- 拆层价格尚未确认，保持禁用；本轮未完成拆层真实验收。
- 供应商账单未核对，不能据本地退款推断供应商成本为零。
- 菜单短屏裁切与 video 屏外问题已修复并复测通过，API 隔离 smoke、限定范围的独立最终代码复核与完整 verify 均通过；完整 verify 为 **PASS，exit 0**，发布门禁仍因 `heavy approval.pending` 未通过。
