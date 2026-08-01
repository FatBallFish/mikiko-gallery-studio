# 中文 Release Notes 模板需求

## 背景

当前标签发布使用 GitHub 自动生成正文，Release 页面只有 `What's Changed` 和完整变更链接，无法让使用者快速理解项目、版本价值以及安装升级方式。

## 功能需求

1. 仓库维护统一的 Release Notes 模板，每次标签发布均基于模板生成正文。
2. Release Notes 以中文为主，命令、参数、路径和产品标识保持英文。
3. 模板固定包含项目简介、Feature 更新、Bugfix、优化项、快速部署教程和快速升级教程。
4. Feature、Bugfix 和优化项从上一个标签到当前标签之间的非 Merge 提交自动生成。
5. `feat` 提交归入 Feature，`fix` 提交归入 Bugfix，`perf/refactor/ci/chore/docs/build/test/style` 提交归入优化项；其他提交也归入优化项以避免遗漏。
6. 每条变化包含提交标题和 GitHub commit 链接；空分类显示“本版本暂无”。
7. 快速部署使用 `scripts/install.sh install --yes` 的默认 Docker/full/single/latest 路径，并固定当前版本的 mgsctl 下载。
8. 快速升级提示先备份 PostgreSQL 与对象存储，再执行 `mgsctl upgrade` 和 `mgsctl doctor`。
9. Workflow 重跑时必须使用同一模板结果同步已有 Release 正文，不依赖人工编辑。
10. Notes 渲染失败时不得退回 `--generate-notes` 或创建正文不完整的新 Release。

## 验收标准

- 标签发布生成的正文完整包含六个规定章节。
- 提交能按 Conventional Commit 前缀稳定分类，并生成正确链接。
- 首个标签、空分类和未知提交前缀均有确定行为。
- 同一标签重复渲染得到相同正文，Workflow 重跑不会产生不同格式。
- Release 附件、Manifest、Docker 镜像和 `latest` 提升流程保持原有行为。
- 发布契约、仓库完整验证和 committed-scope review gate 通过。

## Approved Design

See `docs/plans/2026-08-01-release-notes-template-design.md`.
