# Seedance 2.5 支持 1080p 生成与定价需求

> 日期：2026-09-14
> 状态：已确认（仓库所有者提出）
> 线上基线：v0.0.27（含 native video pricing）

## 背景

火山方舟 Ark 的 `doubao-seedance-2-5-260628` 官方支持 1080p 输出，且已用真实账号实测验证（提交 5 秒 1080p 任务成功，产物为 1920×1080 H.264）。当前代码的 `seedanceModelSupportsResolution` 将 seedance-2-5 的审计定价预设限制在 480p/720p，导致 1080p 无法通过费率卡校验、无法接入路由能力，用户端无法选择 1080p。

## 目标

放开 seedance-2-5 的 1080p：能力声明、审计定价预设、费率卡校验、计费（token 估算）与用户端选择全链路可用。

## 验收条件

- `seedanceModelSupportsResolution("doubao-seedance-2-5-260628", 1080p)` 返回 true；像素预设复用现有 1920×1080 长短边。
- 含 1080p 的 seedance-2-5 费率卡可通过 `ValidateRateCard`。
- 本地服务配置 1080p 能力 + 费率后，用户端创作页可选 1080P，端到端生成成功且按 token 计费结算正确。
- 单元测试更新并通过；不回归 2.0 的分辨率集合。

## 非目标

- 不引入 4K（2.5 官方 4K 支持情况未实测）。
- 不改变 token 估算公式与 2026-08 规则版本。
