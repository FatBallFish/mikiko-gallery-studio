# v0.0.23 后端多架构镜像发布热修技术方案

> 日期：2026-08-14
> 状态：已确认
> 需求来源：`docs/prd/2026-08-14-v023-backend-multiarch-release-fix-requirements.md`

## 1. 证据与根因

v0.0.22 Tagged Release 中，用户端镜像使用 `$BUILDPLATFORM` 后在 56 秒内完成；API 和
Worker 从同一时间开始构建，超过 20 分钟仍停留在 Buildx build/push 步骤。对应
Dockerfile 均使用未指定平台的 `golang:alpine` 构建阶段，而发布矩阵一次请求
`linux/amd64,linux/arm64`。

Buildx 会为每个目标平台执行该构建阶段。arm64 分支在 amd64 GitHub runner 上通过 QEMU
运行 Alpine、C 编译器和 Go 编译器。两个镜像都启用 CGO，API 还连续编译四个二进制，
使模拟路径的耗时显著放大。该差异与已经修复的前端 Dockerfile 模式一致，并可由
Dockerfile 平台解析和双架构无缓存构建稳定复现。

## 2. 方案选择

采用 `tonistiigi/xx` 交叉编译工具链：

- `xx` 和 Go build stage 固定为 `$BUILDPLATFORM`，所有编译命令在 runner 原生执行；
- 由 Buildx 注入 `TARGETPLATFORM`，使用 `xx-apk` 安装目标平台 musl/GCC，使用 `xx-go`
  设置目标 GOOS、GOARCH、CC 和 CGO 环境；
- 保持 `CGO_ENABLED=1` 和现有 `_LARGEFILE64_SOURCE` 编译参数，不改变当前仅接受
  PostgreSQL 运行时 URL 的规则；
- 最终 Alpine 运行层不指定 build platform，继续按目标平台分别生成；
- 使用固定版本的 `tonistiigi/xx`，避免工具链无界漂移。
- 提供可选 `ALPINE_MIRROR` build arg，默认仍使用 Alpine 官方 CDN；本地验证可显式切换
  到可达镜像源，且构建层与运行层使用同一来源。

不采用以下方案：

- 继续使用 QEMU：无法解决发布耗时和挂死风险；
- 设置 `CGO_ENABLED=0`：会移除 SQLite 驱动，构成运行时兼容性回退；
- 为每个架构维护独立 Dockerfile 或 workflow：重复度高，容易产生镜像差异。

## 3. 发布契约与测试

先扩展 `scripts/devops/release-contract-test.sh`，要求 API、Worker Dockerfile 同时包含：

- `$BUILDPLATFORM` 构建阶段；
- `TARGETPLATFORM`；
- `xx-apk` 和 `xx-go`；
- `CGO_ENABLED=1`。

红绿顺序如下：

1. 新增契约并确认在现有 Dockerfile 上按预期失败；
2. 修改 Dockerfile 后确认契约通过；
3. 对 API、Worker 执行无缓存 amd64/arm64 Buildx 构建；
4. 校验镜像和 ELF 架构，并让两种架构的 API 镜像连接隔离 PostgreSQL 执行迁移；
5. 运行全仓 verify、API smoke、committed review gate 和 ship guard；
6. PR 合入 main 后创建新 tag，等待完整 Tagged Release 和 latest 推广成功。

## 4. 兼容与回滚

运行层、文件路径、用户身份、健康检查、FFmpeg 版本和入口均不改变。若新镜像出现问题，
可继续部署 v0.0.20 或已验证的旧版本；v0.0.21 和 v0.0.22 保持不可变历史记录。
