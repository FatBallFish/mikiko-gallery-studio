# MGSCTL 安装、升级与发布需求

## 背景

现有 `deployctl` 的安装默认值、应用版本输入、升级迁移执行环境和 TUI 配置体验存在不一致。工具名称过于宽泛，发布流水线也没有同时交付独立应用包和 Docker 镜像。

## 功能需求

1. 工具直接更名为 `mgsctl`，不保留 `deployctl` 命令、旧 Release 资产或 `DEPLOYCTL_*` 用户接口兼容。
2. 所有 `pic-gallery-*` 应用产物统一更名为 `mikiko-gallery-studio-*`。
3. Install 默认使用 `docker + full + single`；`mgsctl install --yes` 可直接采用这些默认值。
4. Docker image selector 默认是 `latest`，Native release selector 默认是最新 GitHub Release。
5. `APPLICATION_VERSION` 不允许人工输入，必须由已验证的发布产物自动推导。
6. Tag Release 必须发布 `release-manifest.json`。Manifest 记录应用版本、Git SHA、五个 Docker 镜像的不可变 digest、Release 资产名和 SHA-256。
7. Docker 的 `latest` 只能作为解析入口。安装和升级必须解析出具体 `vX.Y.Z`，校验所有所选镜像版本一致，并持久化具体版本和 digest。
8. Native 模式从 Release Manifest 推导并持久化具体 Release 版本。
9. 无目标版本参数的 `mgsctl upgrade` 默认升级到最新发布；CLI 仍允许固定具体 image tag 或 release version。
10. Runtime 解析优先级为显式参数、当前目录、当前目录下的 `runtime/`、用户配置中最近一次成功安装的 runtime。
11. Docker 数据库迁移必须在对应 Compose project/network 内使用目标版本迁移器运行，不能由宿主机使用容器 DNS 地址直连 PostgreSQL。
12. Native 发布包必须包含目标版本迁移器，升级时不得使用旧版控制工具内置的迁移逻辑。
13. Profile 切换到 `full` 时，TUI 必须联动勾选 full 预设的 9 个组件并选择 S3；Monitoring 不属于 full。
14. Core/full 组件只读展示，custom 才允许修改组件选择。
15. Install TUI 普通表单不显示 Application version、Public API URL、Image registry 或 Release version；对应高级 CLI 能力保留。
16. TUI 按模式和组件动态显示配置项，并预填 Gateway `80`、User Web `5173`、Admin Web `5174`、Docs Web `5175`、Monitoring `9090`。
17. TUI 默认中文，可即时切换英文，并在用户配置目录的 `mgsctl/config.json` 中持久化语言偏好。
18. 非 TUI 的脚本化输出不纳入本次国际化范围。
19. Tag `vX.Y.Z` push 自动构建并上传 `mgsctl`、API、Worker、三个前端发布包及校验文件。
20. 同一 workflow 构建并推送五个 `docker.io/fatballfish/mikiko-gallery-studio-*` 多架构镜像，并在全部发布检查通过后更新 `latest`。
21. setup binding 摘要算法演进必须通过显式 `mgsctl upgrade` 迁移兼容。迁移只能在数据库绑定和 `install-state.json` 可由旧算法严格复算命中时执行，任意无法验证的摘要仍须 fail-closed。

## 已确认决策

- Full Profile 不包含 Monitoring。
- Docker Hub 使用 `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN` 登录。
- 采用统一 Release Manifest，而不是把 `latest` 或镜像 tag 字符串直接当作逻辑版本。
- `mgsctl` 直接更名，不提供旧命令或旧环境变量兼容。
- 已有 runtime 的旧工具锁和 Compose 控制字段不提供兼容迁移。
- 现有 `runtime/` 中的用户数据不得被开发或测试流程复用、覆盖或删除。

## 验收标准

- CLI、传统交互、TUI 和安装脚本最终得到一致的默认安装计划。
- TUI 切换 full 后显示正确的组件、存储和端口默认值，生成参数能够通过 CLI parser 和安装计划校验。
- 首次启动 TUI 使用中文，切换英文后即时生效，重启后仍保持英文；损坏的偏好文件安全回退中文。
- Docker 和 Native 安装、升级都从经过校验的 Manifest 得到具体应用版本，运行配置中不存在裸 `latest` 或裸 `dev`。
- 从 runtime 外运行 `mgsctl upgrade` 能定位最近安装；Docker full 升级能在 Compose 网络内完成迁移并恢复健康。
- Release 资产、校验和、Manifest、OCI labels 和镜像 digest 相互一致。
- Docker Hub 同时存在 `vX.Y.Z` 和指向相同 digest 的 `latest` 多架构镜像。
- 从旧摘要版本升级时，目标迁移器使用升级前发布身份复算旧摘要，并在启动目标服务前同步更新数据库绑定和 `install-state.json`；普通 API/Worker 启动不自动修复摘要。
- 仓库当前用户文档和可执行入口中不存在 `deployctl` 或 `pic-gallery-*` 品牌残留。
- `./scripts/workflow/verify.sh`、隔离 API smoke、Docker 升级 E2E 和 committed-scope review gate 通过。

## Approved Design

See `docs/plans/2026-07-30-mgsctl-install-upgrade-release-design.md`.
