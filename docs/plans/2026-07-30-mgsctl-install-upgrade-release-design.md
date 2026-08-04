# MGSCTL 安装、升级与发布设计

## 目标与边界

本设计统一安装默认值、产物驱动的应用版本、runtime 定位、升级迁移、TUI 配置与 tag 发布流程。工具直接更名为 `mgsctl`，应用产物统一使用 `mikiko-gallery-studio-*`。历史设计文档保留当时事实，不做机械改名。

非目标包括自动迁移旧 `deployctl` 命令、旧环境变量、旧 Release 资产或旧 runtime 控制字段，以及对脚本化 CLI 输出进行国际化。

## 发布事实来源

Tag `vX.Y.Z` workflow 在发布前构建所有产物，并生成经过校验的 `release-manifest.json`。Manifest 至少包含 schema version、application version、Git SHA、每个 Release 资产的名称与 SHA-256，以及五个 Docker 镜像的 repository、version tag 和多架构 digest。

五个镜像为：

- `docker.io/fatballfish/mikiko-gallery-studio-api`
- `docker.io/fatballfish/mikiko-gallery-studio-worker`
- `docker.io/fatballfish/mikiko-gallery-studio-user-web`
- `docker.io/fatballfish/mikiko-gallery-studio-admin-web`
- `docker.io/fatballfish/mikiko-gallery-studio-docs-web`

镜像写入 `org.opencontainers.image.version`、`org.opencontainers.image.revision` 和 source label。版本 tag 与 `latest` 必须由同一次 buildx 发布或通过 digest alias 生成。只有所有构建、校验和 Release 上传成功后才更新 `latest`，避免部分发布成为默认版本。

## 应用版本解析

依赖方向改为“产物 selector 决定逻辑版本”。Docker 的 `latest` 和 Native 的 latest Release 只作为解析入口，不写入最终 runtime。

Docker resolver 下载并校验 Manifest，选择安装组件对应的镜像 digest，交叉校验 OCI version/revision，并得出具体 `vX.Y.Z`。最终写入具体 `APPLICATION_VERSION`、版本 tag 和 digest。Native resolver 使用同一 Manifest 选择具体 Release 资产和版本。

安装、升级、数据库迁移、集群加入、心跳和 schema 兼容检查共享同一个已解析 application version。公开 CLI/TUI 不再接受 Application version。固定版本仍通过 image tag 或 release version selector 表达。

本地源码 fallback 为干净工作树生成基于 Git commit 的开发版本；脏工作树追加内容哈希。所有本地构建镜像注入相同版本元数据，禁止回退到无法区分内容的裸 `dev`。

## 安装与 TUI

安装默认计划为 Docker、full、single 和 latest。非交互 `mgsctl install --yes` 不再要求显式提供 mode/profile/topology。显式 CLI flag 始终覆盖默认值。

TUI 表单根据 mode、profile、role 和 components 动态生成：

- Application version 不再出现。
- 普通表单不显示 Public API URL、Image registry 或 Release version；高级 CLI 保留这些能力。
- full 联动现有 9 个 full 组件和 S3，不包含 Monitoring。
- core/full 组件以只读勾选状态展示，custom 允许修改。
- 端口只在对应组件存在时显示，并预填 API `8080`、Gateway `80`、User `5173`、Admin `5174`、Docs `5175`、Monitoring `9090`。

TUI 文案通过稳定 message key 查表。默认 locale 是 `zh-CN`，可在根菜单切换 `en-US`。切换立即更新菜单、字段、帮助、校验、Review 和确认文案，并原子写入 `os.UserConfigDir()/mgsctl/config.json`。文件缺失或损坏时使用中文并保留可操作性；写入失败在 TUI 中提示，但不改变当前会话已选择的语言。

同一个配置文件记录最近一次成功安装的 runtime 绝对路径。配置 schema 有显式版本，语言和 runtime 更新使用同一原子读改写实现。

## Runtime 解析

需要安装上下文的命令共享 runtime resolver：

1. 显式 `--runtime-dir`；
2. 当前目录的 `deployment.json`；
3. 当前目录下 `runtime/deployment.json`；
4. 用户配置中的最近安装目录。

resolver 不递归扫描父目录、任意子目录或整个磁盘。若候选歧义、保存路径失效或没有候选，错误列出已检查位置和显式修复命令。安装成功后才更新保存路径，失败或不完整安装不能覆盖上一次有效值。

## 升级与数据库迁移

无目标参数的 upgrade 默认解析 latest。整体顺序是解析并校验目标、准备目标镜像或 Native 包、写入目标 runtime/manifest、运行目标版本迁移器、滚动服务和健康检查。

Docker 模式先按 Manifest digest 拉取目标镜像。迁移器是 API 镜像内的 `mikiko-gallery-studio-db-migrate` 二进制，通过对应 Compose project/network 的一次性容器运行并读取挂载的 runtime 配置。即使 custom 部署没有 API 组件，仍显式使用 Manifest 中的 API migration image。PostgreSQL 不发布宿主端口，也不改写持久 DSN。

Native 发布包包含 `mikiko-gallery-studio-db-migrate`。升级先准备目标包，再执行该目标版本二进制，避免控制工具版本与应用迁移版本耦合。

解析、下载、校验或迁移失败时恢复旧 runtime 配置，旧服务继续运行。迁移成功但服务滚动失败时保留目标配置并要求以同一目标重试，不尝试逆向数据库迁移。

setup binding 摘要算法发生兼容性演进时，由目标版本迁移器接收升级前的发布身份并复算旧摘要。只有数据库绑定和本地 completed install-state 的操作 ID、安装 ID、配置 revision 均一致，且各自摘要严格等于旧摘要或当前摘要时，才允许使用 compare-and-swap 统一到当前摘要。文件写入失败时回滚本次数据库摘要更新；无法验证的摘要保持 fail-closed。该迁移仅由 `mgsctl upgrade` 调用，普通 API/Worker 启动继续只做严格验证。

## 直接更名

二进制、命令、Go 包、Make target、安装路径、安装器变量、Release 资产、self-update、帮助、文档、runtime 锁、临时文件和 Compose 控制变量统一改为 `mgsctl` / `MGSCTL_*`。删除旧命令、旧变量和旧资产解析逻辑。

API、Worker、Gateway、前端、迁移器、Native bundle 和独立发布包统一使用 `mikiko-gallery-studio-*`。数据库表、公开 API 路由或与品牌无关的稳定协议标识不做无意义改名。

## GitHub Actions

`vX.Y.Z` tag workflow 包含：

1. SemVer、Go、前端、安装器和 release contract 验证；
2. `mgsctl` 的 Linux/macOS/Windows amd64/arm64 构建；
3. API、Worker Linux amd64/arm64 包和三个平台无关前端包；
4. 五镜像的 linux/amd64、linux/arm64 buildx 发布；
5. Release Manifest、SHA-256 和 OCI metadata 交叉校验；
6. GitHub Release 资产上传；
7. 成功后的 Docker Hub latest digest promotion。

Docker Hub 登录使用 `DOCKERHUB_USERNAME` 与 `DOCKERHUB_TOKEN`。Workflow 不创建 tag，不在普通分支 push 时发布。

## 测试与验收

Go 测试覆盖 CLI/TUI/脚本默认值、动态表单、profile 联动、端口、i18n 配置、runtime resolver、Manifest 解析、Docker/Native 迁移分派和失败恢复。Shell/PowerShell 契约覆盖安装路径、环境变量、资产名、校验和与源码 fallback。Release 契约覆盖资产、Manifest、镜像名、labels、digest 和 job 依赖。

Docker E2E 从独立临时 runtime 完成 full 安装和 Setup，再从 runtime 外执行默认 latest upgrade，验证 Compose 网络内迁移和服务健康恢复。测试不得读取、复用、修改或删除仓库现有 `runtime/`。

最终运行 repository verify、隔离 API smoke、Docker upgrade E2E、committed-scope review 和 review gate。
