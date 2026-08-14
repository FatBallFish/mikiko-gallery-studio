# v0.0.23 后端多架构镜像发布热修需求

> 日期：2026-08-14
> 状态：已确认
> 问题版本：v0.0.22
> 目标版本：v0.0.23

## 背景

v0.0.22 修复用户端镜像在 QEMU 中执行 Node 构建导致的发布挂死后，用户端双架构镜像
在 56 秒内成功。随后 API 和 Worker 镜像长期停留在 `Build and publish immutable image`。
检查发现两个后端 Dockerfile 的 Go 构建阶段同样会跟随目标平台运行；在 GitHub 的 amd64
runner 上构建 arm64 镜像时，包含 CGO 的 Go 编译因此落入 QEMU 模拟路径。

API 和 Worker 当前镜像明确使用 `CGO_ENABLED=1`。热修不得通过关闭 CGO 改变既有编译
特征或潜在兼容面；当前运行时 Schema 仍按既有规则只接受 PostgreSQL。

## 目标

1. API 和 Worker 的 Go 编译在 runner 原生平台执行，并为 `linux/amd64`、`linux/arm64`
   生成正确的目标架构二进制。
2. 保留 CGO 编译特征，不改变后端运行时行为、入口、健康检查及 Worker 内置 FFmpeg。
3. 发布契约覆盖 API、Worker 构建平台、交叉编译和 CGO 要求，阻止同类回归。
4. 本地完成无缓存双架构构建，并检查镜像架构、二进制架构和 PostgreSQL 迁移可运行性。
5. 不移动或覆盖 v0.0.21、v0.0.22 tag；通过新补丁版本完成全量发布。

## 验收条件

- 修复前新增发布契约因 API、Worker 缺少原生构建平台和交叉编译声明而失败。
- 修复后发布契约、全仓验证、API smoke 和本地双架构 Docker 构建通过。
- amd64/arm64 镜像中的 API、Worker 二进制架构与镜像平台一致，API 迁移程序可连接隔离的
  PostgreSQL 并完成迁移。
- PR 合入 main 后创建新的 annotated tag。
- Tagged Release 的所有镜像、离线制品、发布清单、GitHub Release 和 `latest` 推广全部成功。

## 非目标

- 不修改模型管理、画布或其他业务功能。
- 不删除或覆盖已经存在的 tag 和历史发布记录。
- 不把后端 Docker 镜像改为 `CGO_ENABLED=0`，也不改变现有数据库 URL 校验规则。
