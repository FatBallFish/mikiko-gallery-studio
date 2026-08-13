# v0.0.18 Docker 媒体工具诊断热修需求

> 日期：2026-08-13
> 状态：已确认
> 线上基线：应用与 mgsctl v0.0.17
> 目标版本：v0.0.18

## 背景

生产环境已从 v0.0.12 成功升级到 v0.0.17，数据库迁移、Setup binding 兼容、镜像滚动和
HTTP 健康检查均成功。升级后执行 `mgsctl doctor` 时，`WORKER_MEDIA_TOOLS` 错误报告
宿主机缺少 FFmpeg 和 ffprobe。

实际进入 v0.0.17 Worker 容器检查，`/usr/bin/ffmpeg` 和 `/usr/bin/ffprobe` 均存在，
版本为 6.1.2。Docker 部署把媒体工具集成在 Worker 镜像中，不要求宿主机安装；现有
doctor 却始终使用宿主机 `PATH` 检查，因而产生误报。

## 目标

1. Docker 部署且当前节点启用 Worker 的 `media` 角色时，在运行中的 Compose Worker
   服务内检查配置的 FFmpeg 和 ffprobe 命令。
2. Native 部署继续检查宿主机 `PATH`，不改变现有行为。
3. Docker 检查必须使用 runtime 中的 installation/node 身份解析 Compose project，
   不依赖写死的容器名。
4. 工具路径作为独立进程参数传入，不拼接到 shell 程序中；诊断输出不得泄漏 runtime
   环境或密钥。
5. 容器未运行、Docker/Compose 不可用或任一工具缺失时保持 fail-closed，输出统一的
   `WORKER_MEDIA_TOOLS` 失败结论。
6. 发布不可覆盖 v0.0.17，使用新补丁版本 v0.0.18；发布成功后升级生产 mgsctl 和应用，
   重新完成全部健康验收。

## 验收条件

- Docker 模式不调用宿主机 `LookPath`，而是执行 Worker 容器内检查。
- Docker Worker 内两个工具都可用时 `WORKER_MEDIA_TOOLS` 为 PASS。
- 容器检查失败时 `WORKER_MEDIA_TOOLS` 为 FAIL，且不输出敏感配置或底层命令细节。
- Native 模式的成功、缺失和 media 角色未启用行为不回归。
- focused Go tests、全量 verify、API smoke、committed review gate 和发布 Action 全部通过。
- 生产升级后 mgsctl、五个应用镜像均为 v0.0.18，所有容器健康，`mgsctl doctor` 全部
  PASS，数据库仍为 schema 5，历史 `model_accounts.public_id` 数据及约束保持有效。

## 非目标

- 不要求 Docker 宿主机安装 FFmpeg 或 ffprobe。
- 不改变 Worker 镜像中的 FFmpeg 版本或媒体处理实现。
- 不改变 Worker 临时目录检查、数据库 schema 或运行时 schema。
- 不覆盖、移动或重新发布 v0.0.17 tag。
