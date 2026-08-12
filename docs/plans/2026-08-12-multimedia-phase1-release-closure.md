# 多媒体创作一期统一收口与发布计划

日期：2026-08-12
状态：执行中
唯一交付分支：`codex/multimedia-phase1-release-closure`
基线提交：`4bdf55068cc188c2adcdda170aa6c02c4ddc7b65`
目标版本：`v0.0.14`

## 1. 目标

以已经合入 `main` 的多媒体创作一期完整代码为唯一基线，统一承接发布失败修复和最终代码评审发现，不再从历史功能分支重复同步或新建分支。完成全部修复、验证和评审后，通过一个 PR 合入 `main`，创建不可变的新版本标签并确认发布成功。

## 2. 已合入内容审计

| 内容 | 权威提交或 PR | 审计结论 |
| --- | --- | --- |
| v0.0.12 体验修复 | PR #39 / `1fdfb5b` | 已合入 `main` |
| 提示词后续交互与多媒体 PRD/技术方案/原型 | PR #40 / `8968119` | 已合入 `main` |
| 多媒体创作一期实现 | PR #41 / `4bdf550` | 已合入 `main` |
| 功能分支最终 tree | `54153f6` | 与 `4bdf550` tree 完全一致，无遗漏 |
| `v0.0.13` 标签 | `4bdf550` | 标签已公开，但 Release Action 在 verify 阶段失败 |

历史开发分支只是已合并 PR 的保留指针，不再作为交付来源。可访问的历史 worktree 均无未提交修改；主工作区只有用户已有的未跟踪 `runtime/`，不得修改或纳入提交。

## 3. 收口问题与复核结论

| 问题 | 当前结论 | 证据 |
| --- | --- | --- |
| Worker 在 schema/binding 前解析 FFmpeg | 本分支修复 | `TestWorkerNormalStartupVerifiesCompletedBindingBeforeRuntimeServices` 在空 PATH 下完成 red-green |
| Provider 提交崩溃恢复 | 已由 `54153f6` 关闭 | Seedance/MiniMax 均传递同一 idempotency key 并实现 reconcile；真实 adapter 和 Worker 恢复测试通过 |
| 项目删除/转移遗漏多媒体 | 已由 `54153f6` 关闭 | 统计和转移覆盖 MediaAsset、VideoTask、MediaUploadSession；专用数据库测试通过 |
| artifact DNS 重绑定 | 已由 `54153f6` 关闭 | 校验后的 IP 固定到请求专属 `DialContext`；非公网网段和地址固定测试通过 |
| metered usage 缺失仍结算 | 已由 `54153f6` 关闭 | 缺失 usage 时 `UsagePending` 等待 probe，settlement claim 被阻止；数据库测试通过 |

若进一步评审发现阻塞问题，继续在同一分支按 TDD 修复，不再创建新的交付分支。

## 4. 准出标准

1. 定向单元/集成测试全部通过。
2. `./scripts/workflow/verify.sh` 通过。
3. `./scripts/workflow/api-smoke.sh` 通过。
4. `./scripts/workflow/multimedia-acceptance.sh` 和浏览器验收通过。
5. committed-scope 本地 review gate 和 ship guard 通过。
6. 唯一分支提交、推送并通过一个 PR 合入最新 `main`。
7. 创建 `v0.0.14`，Release Action 全部 job 成功。
8. GitHub Release、manifest、校验和、各平台包及五个版本镜像完整，`latest` digest 与版本 tag 一致。

公开的 `v0.0.13` 标签不得移动或静默重建。
