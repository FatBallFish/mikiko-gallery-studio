# Seedance 2.5 1080p 技术方案

> 日期：2026-09-14
> 状态：已确认
> 需求来源：`docs/prd/2026-09-14-seedance-25-1080p-requirements.md`

## 1. 现状

`internal/domain/video/native_pricing.go` 的 `seedanceModelSupportsResolution`：

```go
case strings.Contains(model, "seedance-2-5"):
    return resolution == Resolution480P || resolution == Resolution720P
```

该函数同时约束：
- `validateSeedanceRateCard`：费率卡分辨率必须在审计集合内；
- `seedanceOutputPreset` → `seedanceMinimumTokens`：token 计费估算的像素预设入口。

像素预设 switch 已包含 1080P（1920×1080），无需改动。

## 2. 改动

1. `seedanceModelSupportsResolution`：seedance-2-5 分支追加 `Resolution1080P`。上游已实测（任务 `cgt-20260914101259-640ox`，产物 1920×1080）。
2. 更新 `native_pricing_test.go` 中断言 2.5 分辨率集合的用例（如存在）。
3. 无 schema/前端改动：能力与费率均由 DB 配置驱动，前端分辨率选项来自能力接口。

## 3. 配套配置（本地服务）

- 模型 6（2.5）能力追加 `1080p`（capability_version 递增）。
- 费率卡追加 `1080p` 档位费率（expected_rate_version 递增）。

## 4. 测试

- 单测：2.5 支持 480p/720p/1080p；2.0 保持 480p/720p/1080p/4k；含 1080p 的 2.5 费率卡校验通过。
- 端到端：用户端 1080P 生成成功、实际产物 1920×1080、结算积分与估算一致。
