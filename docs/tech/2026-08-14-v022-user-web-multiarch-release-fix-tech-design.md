# v0.0.22 用户端多架构镜像发布热修技术方案

> 日期：2026-08-14
> 状态：已确认
> 需求来源：`docs/prd/2026-08-14-v022-user-web-multiarch-release-fix-requirements.md`

## 1. 证据与根因

v0.0.21 Tagged Release 的 `verify` 在 9 分 43 秒内成功，随后 27 个并行构建任务中
26 个成功。仅 `build-images (user-web, ...)` 从 2026-08-14 04:40 UTC 开始，超过
4 小时仍停留在 Buildx 的 build/push 步骤。v0.0.20 的同一任务耗时约 2 分 30 秒。

五个镜像的 Dockerfile 对照结果：

- `Dockerfile.admin-web` 和 `Dockerfile.docs-web` 使用
  `FROM --platform=$BUILDPLATFORM node:${NODE_VERSION} AS build`；
- `Dockerfile.user-web` 使用 `FROM node:${NODE_VERSION} AS build`；
- 发布矩阵统一请求 `linux/amd64,linux/arm64`。

因此用户端 Node 构建阶段会随目标平台执行。`arm64` 分支在 GitHub 的 amd64 runner 上
通过 QEMU 运行 npm/Vite，前端规模增大后形成实际挂死。运行层的 Nginx 本身需要保留
目标平台差异，问题只在与最终 CPU 架构无关的静态前端构建阶段。

## 2. 修复设计

对 `Dockerfile.user-web` 做单点修改：Node 构建阶段声明
`--platform=$BUILDPLATFORM`。Buildx 会在 runner 原生架构上构建一次可复用的静态资源，
再分别复制到 amd64/arm64 Nginx 运行层。

在 `scripts/devops/release-contract-test.sh` 增加用户端 Dockerfile 文件检查，并要求存在
与管理端、文档站一致的构建平台声明。该契约在 Tagged Release 的 `verify` 阶段执行，
可在进入昂贵的镜像矩阵前阻断回归。

## 3. 测试策略

采用红绿测试：

1. 先增加用户端 Dockerfile 契约，运行 release contract，确认因现有声明缺失而失败；
2. 修改 Dockerfile 后重跑，确认契约通过；
3. 运行全仓 verify 和 ship guard；
4. 使用 Buildx 对用户端 Dockerfile 执行 `linux/amd64,linux/arm64` 本地构建验证；若本地
   driver 不支持一次加载 manifest，则分别构建两个目标平台并验证镜像静态内容与健康
   配置；
5. PR 合并后以新 tag 运行完整 Tagged Release，并核对所有任务、发布清单与 Release。

## 4. 兼容与回滚

该变更不触及最终镜像内容语义、API、数据库或运行时配置。若新发布出现问题，可继续
使用 v0.0.20 镜像；v0.0.21 tag 保留为不可变历史，不复用其版本号。
