# 中文 Release Notes 模板设计

## 目标

每次标签发布都使用仓库内维护的中文模板生成 GitHub Release Notes，替代仅包含 `What's Changed` 的 GitHub 默认内容。Release Notes 固定包含项目简介、Feature 更新、Bugfix、优化项、快速部署教程和快速升级教程。

非目标包括依赖人工补写每个版本的 Notes、强制维护 PR 标签，以及改变现有 Release 资产、Manifest 或 Docker 镜像发布契约。

## 模板与内容

仓库新增 Release Notes Markdown 模板，使用稳定占位符表示版本号和三个变化分类。项目简介、部署前提、升级注意事项及命令结构由模板维护；版本号、提交条目和链接由渲染器生成。

模板按以下顺序输出：

1. 项目简介；
2. Feature 更新；
3. Bugfix；
4. 优化项；
5. 快速部署教程；
6. 快速升级教程。

内容以中文为主，命令、参数、文件名和产品标识保持英文。没有条目的分类显示“本版本暂无”。

快速部署使用当前 Release 标签下的 `scripts/install.sh`，并通过 `MGSCTL_VERSION` 固定控制工具版本；安装参数使用默认的 Docker、full、single、latest 方案。快速升级明确要求先备份 PostgreSQL 与对象存储，再运行 `mgsctl upgrade` 和 `mgsctl doctor`。

## 版本范围与分类

渲染器接收当前 SemVer 标签并从 Git 历史确定上一个可达标签。变化范围是上一个标签之后到当前标签之间的非 Merge 提交；首个标签从仓库根提交开始。

提交标题按 Conventional Commit 前缀分类：

- `feat` 进入 Feature 更新；
- `fix` 进入 Bugfix；
- `perf`、`refactor`、`ci`、`chore`、`docs`、`build`、`test`、`style` 进入优化项；
- 其他非 Merge 提交进入优化项，避免发布变化被静默遗漏。

每条记录保留提交标题，并生成指向 GitHub commit 的链接。渲染顺序与 Git 历史一致，保证同一标签重复执行得到相同正文。

## 发布数据流

Tagged Release workflow 的 `release` job checkout 完整 Git 历史，在创建或更新 Release 前调用渲染器生成 Notes 文件。

首次执行使用渲染后的 Notes 创建 Release；重跑时使用同一 Notes 文件更新已有 Release 正文，然后继续执行现有的缺失附件上传和逐文件一致性校验。现有 Release Manifest、Release 附件和 `latest` 镜像提升依赖关系保持不变。

## 失败处理

以下情况必须使 `release` job 失败，并且不得创建只有默认正文的 Release：

- 当前标签不是有效 SemVer；
- 当前标签无法在 Git 历史中解析；
- 模板缺失或包含未替换占位符；
- Git 历史不足以确定版本范围；
- Notes 输出为空或无法写入。

已有 Release 的资产处理仍保持幂等：同名资产必须与已发布内容完全一致，否则失败。

## 测试策略

渲染器契约测试在临时 Git 仓库创建前置标签和 `feat`、`fix`、`refactor`、`docs`、未知前缀提交，验证分类、顺序、commit 链接、版本替换和无残留占位符。单独覆盖空分类、首个 Release、无效标签、缺失模板和不可解析标签。

现有 Release contract 增加以下要求：

- 模板和渲染器存在；
- workflow checkout 完整历史并生成 Notes；
- `gh release create` 和 `gh release edit` 都使用 Notes 文件；
- workflow 不再使用 `--generate-notes`；
- 六个中文模板章节和默认部署/升级命令保持存在。

最终运行聚焦契约、仓库完整验证、committed-scope review gate 和仓库要求的发布前门禁。
