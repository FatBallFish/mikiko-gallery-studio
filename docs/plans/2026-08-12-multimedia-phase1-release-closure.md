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

## 3. 待收口问题

1. Worker 启动在 schema 和 completed binding 校验前解析 FFmpeg，导致无 FFmpeg 的干净 CI 环境返回错误顺序不符，并使 `v0.0.13` Release Action 失败。
2. Provider 提交成功但 Worker 在保存任务 ID 前崩溃时，真实 Seedance/MiniMax adapter 缺少可恢复的幂等或 reconcile 语义。
3. 项目资产统计、转移和删除未完整覆盖统一媒体资产、视频任务和活动上传。
4. Provider artifact 下载的 DNS 校验与实际连接分离，存在 DNS 重绑定窗口。
5. metered 视频缺少实际 usage 时按冻结报价直接结算，违反按实际 usage 结算和待对账要求。

每项必须先添加能稳定复现的回归测试，再实施最小修复。若进一步评审发现阻塞问题，继续在同一分支修复。

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
