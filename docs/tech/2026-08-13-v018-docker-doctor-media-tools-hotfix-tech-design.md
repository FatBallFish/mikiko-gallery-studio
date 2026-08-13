# v0.0.18 Docker 媒体工具诊断热修技术方案

> 日期：2026-08-13
> 状态：已确认
> 需求来源：`docs/prd/2026-08-13-v018-docker-doctor-media-tools-hotfix-requirements.md`

## 1. 生产证据与根因

v0.0.17 生产升级命令正常退出，应用、Worker、三个 Web 服务与网关均达到 Healthy，
数据库迁移报告 `database_schema=5`。随后 `mgsctl doctor` 仅有：

```text
FAIL WORKER_MEDIA_TOOLS: missing required media tools: FFmpeg, ffprobe
```

宿主机的 `command -v ffmpeg` 和 `command -v ffprobe` 为空；同一时刻在实际 Worker 容器
执行检查，两者分别位于 `/usr/bin/ffmpeg`、`/usr/bin/ffprobe`，版本均为 6.1.2。

根因位于 `internal/mgsctl/doctor.go`：只要启用 media 角色，就无条件通过 `exec.LookPath`
检查 mgsctl 所在操作系统。该逻辑适用于 Native 部署，但不适用于工具封装在 Worker 镜像
中的 Docker 部署。

## 2. 设计

在 `DoctorDependencies` 增加 Docker Worker 媒体工具检查依赖。`Doctor` 根据
`DEPLOYMENT_MODE` 分流：

- Docker：调用容器检查依赖，不调用宿主机 `LookPath`；
- Native：保留当前逐项 `LookPath` 检查；
- 未启用 Worker 或 media 角色：保持跳过并报告 PASS。

生产依赖通过 `docker compose exec --no-TTY worker` 在现有 Worker 服务中执行固定 shell
程序。Compose project name 由 `INSTALLATION_ID` 和可选 `CLUSTER_NODE_ID` 使用既有
`dockerProjectName` 算法生成；project directory、env file 和 compose file 均由绝对
runtime 路径确定。

固定 shell 程序仅检查两个位置参数。配置的命令名或绝对路径作为独立 argv 传入，不能
拼接到 shell 文本，从而避免命令注入。进程 stdout/stderr 丢弃，对外只返回统一错误；
Doctor 继续只展示缺失工具的通用文案。

## 3. 安全与兼容

- 不读取或输出 runtime.env 内容，Docker 进程规格沿用经过清理的宿主环境。
- 不把数据库、Redis、S3 密钥放入命令参数或错误消息。
- Docker/Compose、容器状态或工具检查任一失败都视为媒体工具不可用。
- Native 的 PATH 检查与错误文案保持不变。
- 临时目录仍检查宿主机映射路径，不在本次修复中改变。

## 4. 测试

先增加失败测试，证明 Docker media Worker 当前错误调用宿主机 `LookPath`，并要求调用
容器检查依赖。再覆盖：

- Docker 容器检查成功；
- Docker 容器检查失败且诊断不泄漏底层错误；
- 生产 Docker 进程规格使用正确 project identity、runtime 文件与安全 argv；
- Native 检查和未启用 media 角色行为不回归。

focused 测试通过后运行 `verify.sh`、隔离 API smoke、committed review gate 和 ship
guard。

## 5. 发布与生产验证

修复通过独立分支和 PR 合入 main，在合并提交创建 annotated `v0.0.18` tag。等待全部
Release 制品、五个多架构镜像及 `promote-latest` 成功后，生产执行显式 v0.0.18
self-update 与应用升级。

最终核对：mgsctl build commit、五个镜像 tag、全部容器健康、healthz/readyz、doctor
全 PASS、installation app version/schema、三条 model account public_id 非空且唯一，
以及 NOT NULL/唯一约束。
