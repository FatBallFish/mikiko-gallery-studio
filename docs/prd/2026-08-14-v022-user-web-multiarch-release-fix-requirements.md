# v0.0.22 用户端多架构镜像发布热修需求

> 日期：2026-08-14
> 状态：已确认
> 问题版本：v0.0.21
> 目标版本：v0.0.22

## 背景

v0.0.21 已完成管理后台媒体模型管理和画布兼容修复，并通过本地验证、PR 评审及
GitHub Release 的全仓验证。发布进入并行制品构建后，26 个任务成功，用户端 Docker
镜像长期停留在 `Build and publish immutable image`，超过 4 小时仍未完成。上一版本
同一任务约 2 分 30 秒完成。

用户端 Dockerfile 在多架构镜像构建时没有固定前端构建阶段的平台。Buildx 因此会在
目标 `linux/arm64` 模拟环境中执行 Node、npm 和 Vite 构建。本次前端画布依赖和产物
增大后，该模拟构建路径实际挂死。管理端与文档站已经把 Node 构建阶段固定在
`$BUILDPLATFORM`，用户端遗漏了相同配置。

## 目标

1. 用户端多架构镜像只在原生构建平台执行 Node/npm/Vite 构建，最终 Nginx 运行层仍按
   `linux/amd64` 和 `linux/arm64` 分别生成。
2. 发布契约必须检查用户端 Dockerfile 的构建平台声明，避免后续回归。
3. 不改变用户端运行时配置、镜像入口、健康检查或业务功能。
4. 不移动或覆盖已经推送的 v0.0.21 tag；取消其挂死流水线，使用新补丁版本发布。

## 验收条件

- 发布契约在用户端 Dockerfile 缺少 `FROM --platform=$BUILDPLATFORM` 时失败。
- 修复后发布契约、全仓验证和本地双平台 Docker 构建验证通过。
- 热修复 PR 合入 main 后创建新 annotated tag。
- Tagged Release 的校验、所有 Docker 镜像、原生与离线制品、发布清单、GitHub Release
  和 latest 推广全部成功。
- 新 tag 解析到对应的 main 合并提交，镜像元数据与 Release 制品完整。

## 非目标

- 不修改 v0.0.21 已交付的模型管理和画布业务实现。
- 不调整前端 bundle 拆包策略或画布依赖。
- 不覆盖或删除 v0.0.21 tag 及其历史运行记录。
