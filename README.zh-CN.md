# Pic Gallery

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![OpenAPI](https://img.shields.io/badge/OpenAPI-3.x-6BA539?logo=openapiinitiative&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![License](https://img.shields.io/badge/license-not%20specified-lightgrey)

[English](./README.md) | 简体中文

Pic Gallery 是一个可自部署的 AI 图片生成平台。它定位在 API 中转服务和完整产品应用之间：对接 OpenAI、OpenRouter 等上游图片模型，并在其上封装用户体系、钱包计费、模型路由、参考图资产、公开广场、收银台、管理后台和 OpenAPI 文档能力。

## 项目概览

Pic Gallery 面向希望运营图片生成产品的团队，而不是只暴露原始模型 API。

它包含：

- 用户端 Web：提示词生图、参考图工作流、私有历史、公开广场、充值、开发者 API Key。
- 管理后台：用户运营、模型路由、价格、公开图库审核、调用记录、收银台运营、配置、审计日志和系统就绪检查。
- Go API 服务与 Worker：负责鉴权、任务队列、钱包流水、上游模型调用、支付订单、回调处理和可观测性。
- 面向外部开发者的 Native Open API 与 OpenAI 兼容图片接口。

## 功能特性

### 图片生成

- 文生图、图片编辑、参考图生成。
- 异步图片任务队列，提供 API 与 Worker 两个运行入口。
- 参考图上传、下载和存储抽象。
- 模型能力发现、质量选择、尺寸比例选择和生成数量限制。
- OpenAI 与 OpenRouter Provider 适配。
- 路由模型、真实模型、模型账号、候选模型、可见性规则和价格配置。

### 用户端产品

- 邮箱验证码登录、密码登录、会话刷新、退出登录、修改密码和重置密码。
- 注册体验额度，支持配置额度、有效期和过期提醒。
- 钱包余额桶：体验余额、订阅余额、充值余额、赠送余额和冻结余额。
- 余额、流水、预估、充值和个人中心页面。
- 私有图库、生成历史和公开申请。
- 公开广场支持游客浏览、登录后查看详情、完整提示词、点赞、收藏和同款提示词生成。
- 开发者 API Key 生命周期和 API 文档页。

### 收银台与计费

- 固定积分包和自定义金额充值。
- 充值积分进入充值余额桶，默认不过期。
- 支付订单创建、取消、查询、Mock 支付、回调接入、到账履约、退款、拒付扣回、人工完成和渠道同步。
- 支付渠道实例，商户配置加密存储。
- 内置 Alipay、WeChat Pay、EasyPay、JeePay 和 Mock 支付适配契约。
- Mock 支付用于本地和测试环境。

### 管理后台

- 管理员登录，权限门面支持 `super_admin` 与 `admin`。
- 面向上线检查的 readiness dashboard。
- 用户列表、用户详情、状态管理、密码重置、用户组分配和积分调整。
- 用户组管理和计费倍率。
- 兑换码创建、批量创建、导出、状态管理和兑换记录。
- 公开图库审核队列，支持通过、拒绝和下架。
- 调用记录、审计日志、健康/配置页、模型路由、真实模型、价格和收银台运营。

### API 与文档

- 用户端 Agent APIs：`/api/agent/*`。
- 公开 Open APIs：`/api/open/image/v1/*`。
- OpenAI 兼容接口：
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
  - `GET /v1/models`
- 运行时 API 文档接口：
  - `GET /docs/openapi.json`
  - `GET /docs/examples`
  - `GET /docs/errors`
- OpenAPI 源文件：[`api/openapi/openapi.yaml`](./api/openapi/openapi.yaml)。

## 技术栈

- 后端：Go `net/http`、Ent、PostgreSQL、Redis、JWT、decimal。
- 前端：React 19、TypeScript、Vite。
- 存储：默认本地文件系统，并预留 S3 兼容存储抽象。
- 运维：Docker Compose、Nginx、Prometheus 配置、健康检查、Smoke 脚本和 Docker E2E 脚本。

## 生产部署快速开始

`mgsctl` 是唯一受支持的生产安装与生命周期管理入口。新建单机实例推荐使用 Docker `full/single`，它包含 API、Worker、全部 Web、Gateway、PostgreSQL、Redis 和 MinIO。

安装 Docker Engine 与 Compose v2，克隆仓库后执行：

```bash
git clone https://github.com/FatBallFish/mikiko-gallery-studio.git
cd mikiko-gallery-studio
./scripts/install.sh install \
  --mode docker \
  --profile full \
  --topology single \
  --runtime-dir ./runtime \
  --image-tag latest \
  --yes
```

默认值就是 Docker、`full`、`single` 和镜像选择器 `latest`，所以上面的 mode/profile/topology 参数可以省略。mgsctl 会校验 `release-manifest.json`，把 `latest` 解析成具体 SemVer 应用版本和不可变镜像 digest，再持久化该具体版本；应用版本不再由用户填写。

安装器会下载并校验匹配平台的 mgsctl Release 产物。如果 Release 产物不可用，且当前目录是完整源码仓库，则自动降级执行 `make mgsctl`；本地构建需要 Go 和 Make。校验和不一致属于安全硬失败，绝不会降级切换二进制来源。默认 API 镜像仓库为 `docker.io/fatballfish/mikiko-gallery-studio-api`。

服务启动后打开 `http://<api-host>:8080/setup`，通过 `mgsctl setup token show --runtime-dir ./runtime` 获取一次性 Token，完成中间件连通性与首个管理员初始化。其他部署模式、参数、集群、升级和恢复方式见[生产部署](#生产部署)。

## 开发者本地调试

以下命令仅用于本地开发与贡献，不是生产安装的替代方案。

### 环境要求

- [`go.mod`](./go.mod) 声明的 Go 1.26。
- Node.js 20+；推荐 Node.js 22 LTS。
- npm。
- Docker 与 Docker Compose。

### 1. 克隆仓库

```bash
git clone https://github.com/FatBallFish/mikiko-gallery-studio.git
cd mikiko-gallery-studio
```

### 2. 准备可移植运行时配置

```bash
mkdir -p config
cp config/runtime.env.example config/runtime.env
```

API 和 Worker 会相对于工作目录读取 `./config/runtime.env`。模板中的每个字段均包含中英文运维说明；Compose 插值默认值仍在 `deployments/docker-compose/.env.example` 中维护。

### 3. 启动开发全栈

```bash
make compose-up
```

默认开发 Compose 文件会启动完整应用栈：PostgreSQL、Redis、MinIO、Mailpit、API、Worker、用户端 Web、管理端 Web 和 Nginx。只有 Nginx 暴露到宿主机。

默认入口：

- 用户端 Web：`http://127.0.0.1:8088/`
- 管理端 Web：`http://127.0.0.1:8088/admin/`
- API 与文档：通过 `/api/*`、`/docs/*`、`/v1/*`、`/healthz`、`/readyz` 代理

初始化管理员流程：

1. 请求 `GET /api/system/v1/bootstrap-status`，打开返回的 `setup_url`。
2. 使用部署工具首次输出的 setup Token；初始化完成前也可通过 `mgsctl setup token show` 找回。
3. 在 setup 中由运维人员自行填写首个管理员邮箱和密码。该账号会以 `super_admin` 身份创建，系统不预置默认管理员凭据。

如果要把服务暴露到本机开发环境之外，请先完成 `config/runtime.env` 中由 setup 管理的配置，并在 `deployments/docker-compose/.env.example` 中覆盖 Compose 专用值。

停止依赖：

```bash
make compose-down
```

如果你之前用旧配置启动过开发数据库，并遇到 `no pg_hba.conf entry` 或 `role "pic_gallery" does not exist` 这类 PostgreSQL 报错，可以重置本项目的本地开发数据卷后重新启动：

```bash
make compose-clean
make compose-up
```

`make compose-clean` 只会删除本项目 Docker Compose 开发环境的数据卷。

如果只想启动中间件，方便在宿主机上运行源码 API 和前端，请使用中间件专用 Compose 文件：

```bash
make compose-middleware-up
```

中间件栈会向宿主机暴露这些端口：

- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`
- MinIO API：`localhost:9000`
- MinIO Console：`localhost:9001`
- Mailpit UI：`localhost:8025`

### 4. 启动 API 和 Worker

使用默认全栈 Compose 时可以跳过这一步。如果只通过 `make compose-middleware-up` 启动了中间件，再打开两个终端：

```bash
make dev
```

```bash
make worker
```

API 默认监听 `http://127.0.0.1:8080`。

常用健康检查接口：

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### 5. 启动前端应用

安装依赖：

```bash
make user-web-install
make admin-web-install
```

启动用户端：

```bash
make user-web-dev
```

启动管理端：

```bash
make admin-web-dev
```

默认本地地址：

- 用户端：`http://127.0.0.1:5173`
- 管理端：`http://127.0.0.1:5174`

Vite 开发服务器默认会把 `/api` 和 `/docs` 代理到 `http://127.0.0.1:8080`。也可以覆盖后端目标：

```bash
VITE_API_PROXY_TARGET=http://127.0.0.1:8080 make user-web-dev
VITE_API_PROXY_TARGET=http://127.0.0.1:8080 make admin-web-dev
```

## 配置

运行时配置改为用 env 文件完成部署启动，用数据库和管理后台承载业务配置。

主要模板：

- [`config/runtime.env.example`](./config/runtime.env.example)：API/Worker 唯一的双语运行时配置模板。
- [`.env.example`](./.env.example)：已退役根目录 `.env` 布局的迁移提示。
- [`deployments/docker-compose/.env.example`](./deployments/docker-compose/.env.example)：本地 Compose 环境变量模板。
- [`deployments/docker-compose/.env.prod.example`](./deployments/docker-compose/.env.prod.example)：生产 Compose 环境变量模板。

后端运行配置：

- API 和 Worker 默认读取 `./config/runtime.env`，仅允许用 `APP_ENV_FILE` 覆盖文件路径。
- 进程中的同名变量不会覆盖 setup 管理的文件值。`LoadYAML` 仅保留为显式迁移/测试 API，不参与正常启动。
- 运行时文件只保留启动必需项：部署元数据、数据库、Redis、存储 bootstrap、认证和加密密钥、基础端口；首次管理员明文密码不会写入文件。
- SMTP、支付渠道、积分计费、模型供应商账号、模型路由和其他运营配置在启动后进入管理后台配置。

正式对外发布前，至少需要配置一个可用的模型 Provider、模型账号、路由模型、价格，以及一个可用支付渠道。

后台托管的敏感配置采用 write-only 契约：

- SMTP 发信配置在管理后台 `/admin/#/security-config` 配置。SMTP 密码由服务端加密存储，查询接口只返回密钥状态元信息。
- 支付渠道实例在管理后台收银台页面配置。商户密钥应通过密钥字段提交；更新实例时默认保留旧密钥，只有显式轮换或清空才会变更。
- 生产环境输入商户密钥或 SMTP 密码前，请确保管理后台和 Admin API 已通过 HTTPS/TLS 访问。

## 生产部署

`mgsctl` 是唯一受支持的生产部署入口。它会创建可移动的运行目录、生成应用密钥、渲染 Docker 或原生服务文件，并统一维护一份带中英文注释的 `config/runtime.env`。项目自身接受 HTTP 和 IP+端口访问；DNS、HTTPS 证书、反向代理和外部负载均衡由部署者负责。

### 选择部署方式

| 模式 | Profile 与拓扑 | 组件 | 中间件 | 适用场景 |
| --- | --- | --- | --- | --- |
| Docker | 仅 `full` / `single` | API、Worker、用户/管理/文档 Web、Gateway | 托管 PostgreSQL、Redis、MinIO | 新建单机实例，前置依赖最少 |
| Docker | `core` / `single` | API、Worker、用户/管理/文档 Web、Gateway | 外部 PostgreSQL、Redis、对象存储 | 已有基础设施或希望独立维护中间件 |
| Docker | `core` / `cluster` | 先部署 Control，再加入 API/Worker/Web 节点 | 共享的外部 PostgreSQL、Redis、S3 兼容存储 | 横向扩展 API 与 Worker |
| Docker | `custom` / `single` 或 `cluster` | 显式选择组件 | 仅单机 Docker 可选择托管中间件 | 拆分 Web/API/Worker 或增加监控 |
| 原生 Linux/Windows | `core` 或 `custom` / `single` 或 `cluster` | 预编译 API、Worker、便携 Gateway 与 Web 资源 | 仅外部中间件 | 不方便或不希望使用容器的主机 |

重要约束：

- `full` 仅支持 Docker `single` 加 `single` 角色；原生 `full` 和集群 `full` 会被拒绝。
- 集群 Control、API 和 Worker 节点必须使用共享的 S3 兼容存储，节点本地目录不能作为集群存储。
- 集群部署不会创建节点本地 PostgreSQL、Redis 或 MinIO，进入 Setup 前需提前准备这些服务。
- 多 API 节点需要已有负载均衡或反向代理；存活检查使用 `/healthz`，流量就绪检查使用 `/readyz`。
- 原生目标机下载并校验发布包，不要求安装 Go 或 Node.js。

### 前置条件

Docker 部署需要 Docker Engine、Compose v2、可用宿主机端口和可写运行目录。正常情况下还需要镜像仓库访问权限；当请求的 Pic Gallery 应用镜像不存在时，可以使用完整的本地源码仓库构建该镜像，但 Docker 仍需能够获取缺失的基础镜像和中间件镜像。`full` 不需要单独准备中间件；`core` 和集群部署需要能够访问 PostgreSQL、Redis 与对象存储。

原生模式支持 Linux 和 Windows 的 `amd64`、`arm64` 发布包，需要注册系统服务的权限以及外部 PostgreSQL/Redis。只有单节点部署可以使用本地存储；API 或 Worker 存在多节点时必须使用 S3 兼容存储。

导入旧配置或升级前应备份现有数据库和对象存储。不要在 mgsctl 运行目录中放入无关文件，因为破坏性卸载会主动拒绝包含非受管路径的目录。

### 安装包装脚本

Linux 和 macOS 使用 `scripts/install.sh`，Windows 使用 `scripts/install.ps1`。显式指定的 `MGSCTL_BIN` 始终优先。在完整的 Git 源码 checkout 中，包装脚本仅在 `PATH` 中 mgsctl 的构建 commit 与干净 checkout 一致时复用该工具；工具过期或源码存在未提交改动时，会执行本地构建并原子更新持久化二进制。非 Git 源码安装包仍保持 PATH 优先。如果没有可用工具，则下载匹配平台的 Release 产物、校验 SHA-256、持久化安装 mgsctl，并通过绝对路径执行本次命令。

默认安装路径是 Linux/macOS 的 `$HOME/.local/bin/mgsctl` 和 Windows 的 `%LOCALAPPDATA%\Programs\mgsctl\mgsctl.exe`。安装器会输出实际路径和 PATH 配置提示。

当 Release 产物或校验文件不可用时，只有当前目录是包含 `go.mod`、`Makefile`、`cmd/mgsctl` 的完整源码仓库，且机器具备 Go 与 Make，包装脚本才会降级执行 `make mgsctl`。它不会再次下载源码压缩包。校验和不一致（包括与 `MGSCTL_SHA256` 不一致）属于安全硬失败：保留旧 mgsctl，并禁止本地构建降级。包装脚本还会把完整源码仓库作为允许的 Docker 本地镜像构建来源传给 mgsctl。

| 包装脚本变量 | 作用 |
| --- | --- |
| `MGSCTL_BIN` | 指定本地 mgsctl 二进制，适合离线环境或源码构建 |
| `MGSCTL_INSTALL_DIR` | mgsctl 持久化安装目录；默认使用上面的用户级路径 |
| `MGSCTL_VERSION` | 指定要下载的 mgsctl 版本，默认 `latest` |
| `MGSCTL_RELEASE_BASE_URL` | 覆盖 mgsctl 与原生发布包的仓库基础 URL |
| `MGSCTL_DOWNLOAD_URL` | 覆盖完整的 mgsctl 文件下载 URL |
| `MGSCTL_SHA256` | 直接指定预期校验值，不再下载 `.sha256` 文件 |
| `MGSCTL_SOURCE_DIR` | 仅用于 Docker 镜像本地降级构建的完整源码目录；包装脚本会在可用时自动设置 |

`MGSCTL_VERSION` 选择部署工具版本。应用产物选择器默认是 `latest`：Docker 使用 `--image-tag`，原生模式使用 `--release-version`，二者都通过带校验和的 `release-manifest.json` 解析。解析后的具体版本、镜像 digest 与产物校验值共同构成集群兼容、健康检查和迁移使用的应用身份。

### 首次安装

不传 `--yes` 会进入交互式选择。交互和非交互安装都默认使用 `docker`、`full`、`single`、`latest`，只有覆盖默认值时才需要额外参数。

Docker 完整模式，使用默认 `latest` 选择器并显式指定运行目录：

```bash
./scripts/install.sh install \
  --mode docker \
  --profile full \
  --topology single \
  --runtime-dir ./runtime \
  --image-registry docker.io/fatballfish \
  --image-tag latest \
  --yes
```

使用已有中间件的 Docker 核心模式：

```bash
./scripts/install.sh install \
  --mode docker \
  --profile core \
  --topology single \
  --storage-driver s3 \
  --runtime-dir ./runtime \
  --image-tag v1.2.3 \
  --yes
```

Linux 原生核心模式：

```bash
./scripts/install.sh install \
  --mode native \
  --profile core \
  --topology single \
  --storage-driver local \
  --runtime-dir ./runtime \
  --release-version v1.2.3 \
  --yes
```

Windows 原生核心模式：

```powershell
.\scripts\install.ps1 install `
  --mode native `
  --profile core `
  --topology single `
  --runtime-dir .\runtime `
  --release-version v1.2.3 `
  --yes
```

安装只会在 `--runtime-dir` 下写入文件，典型结构如下：

```text
runtime/
├── config/runtime.env
├── config/install-state.json
├── deployment.json
├── compose.yml                 # Docker 模式
├── assets/                     # 生成的 Docker/Gateway 文件
├── bin/, web/, api/            # 原生发布包内容
├── data/
└── logs/
```

具体内容随模式和组件变化。`config/runtime.env`、`config/install-state.json` 与 `deployment.json` 共同承载安装身份和恢复状态，应始终一起保留。

Docker 安装会先拉取选中的全部镜像。如果拉取失败且本地存在包含全部应用 Dockerfile 的完整源码仓库，mgsctl 会使用请求的 registry 和 tag 仅构建当前选中的 Pic Gallery 应用镜像，然后继续启动。PostgreSQL、Redis、MinIO、Nginx、Prometheus 及 Dockerfile 基础镜像不会被项目本地替代；若不存在完整源码仓库，命令会同时输出原始拉取错误和本地降级所缺少的条件。

Setup 尚未完成时，使用完全相同的计划重试会自动续装。若另一套计划指向同一个 pending 运行目录，交互安装会询问是否只覆盖 mgsctl 生成的配置和部署资源；选择取消会保留现状。非交互自动化必须显式传入 `--overwrite`。覆盖会保留 `data/`、`logs/` 和 Docker 持久卷；Setup 已完成或现有文件无法识别为有效 pending 安装时始终拒绝覆盖。

### 安装参数

| 参数 | 可选值/默认值 | 说明 |
| --- | --- | --- |
| `--mode` | 默认 `docker`，可选 `native` | 产物与运行模式 |
| `--profile` | 默认 `full`，可选 `core`、`custom` | 只有 `custom` 可覆盖组件列表 |
| `--topology` | 默认 `single`，可选 `cluster` | 部署拓扑 |
| `--role` | `single`、`control` | 单机默认 `single`，集群默认 `control`；加入节点使用 `cluster join` |
| `--components` | 逗号分隔列表 | `custom` 必填；支持 `api`、`worker`、`user-web`、`admin-web`、`docs-web`、`gateway`、`postgres`、`redis`、`minio`、`monitoring` |
| `--runtime-dir` | `.` | 保存配置、状态、生成文件、数据和日志的可移动目录 |
| `--storage-driver` | `local`、`s3` | full、cluster 或包含 MinIO 的 custom 默认 `s3`，其他情况默认 `local` |
| `--public-api-url` | 绝对 HTTP(S) URL | 记录浏览器可访问的 API 基础地址；加入 Web 角色需要该值，并在 `cluster join` 时从 Control 获取 |
| `--image-registry` | 留空使用 Compose 默认值 | Docker 镜像前缀；当前 Compose 默认 `docker.io/fatballfish` |
| `--image-tag` | `latest` | Docker Release 选择器；安装前解析成具体版本和 digest |
| `--release-version` | `latest` | 原生 Release 选择器；下载发布包前完成解析 |
| `--api-port` | `8080` | API 宿主机端口 |
| `--gateway-port` | `80` | 选择 Gateway 时生效 |
| `--user-web-port` | `5173` | 选择用户 Web 时生效 |
| `--admin-web-port` | `5174` | 选择管理 Web 时生效 |
| `--docs-web-port` | `5175` | 选择文档 Web 时生效 |
| `--monitoring-port` | `9090` | 选择监控组件时生效 |
| `--external-gateway` | `false` | 选择 Web 但不使用托管 Gateway 时，用于确认已有外部托管/代理 |
| `--migrate` | 安装时默认 `false` | 请求 single/control 节点执行迁移；正常首次初始化由 Setup 迁移 |
| `--overwrite` | `false` | 覆盖可识别的 Setup pending 安装配置并保留数据；非交互修改计划时必须显式指定 |
| `--yes` | `false` | 非交互确认，永远不能授权删除持久化数据 |

所有端口必须在 `1-65535` 之间，重复 flag 会被拒绝，显式组件列表会按固定顺序规范化。`custom` 还有以下规则：

- Gateway 需要本地 API 和三个 Web；加入的 Web 节点只要求三个 Web。
- 选择 Web 但不选择 Gateway 时，必须传 `--external-gateway` 并自行提供托管或代理。
- Monitoring 只支持 Docker，且必须同时包含本地 API。
- 原生模式不能管理中间件或 Monitoring。
- 集群 custom 不能包含 `postgres`、`redis` 或 `minio`。
- single/control 权威节点必须包含 API；API、Worker、Web 加入角色必须使用 `cluster join`，不能直接 `install`。

带监控的 Docker 自定义部署示例：

```bash
./scripts/install.sh install \
  --mode docker \
  --profile custom \
  --topology single \
  --components api,worker,user-web,admin-web,docs-web,gateway,monitoring \
  --monitoring-port 9090 \
  --runtime-dir ./runtime \
  --image-tag v1.2.3 \
  --yes
```

### MGSCTL TUI 与服务访问摘要

在交互终端中不带参数运行 `mgsctl` 会进入 TUI。可用数字键或方向键选择，Enter 确认，Space 勾选多选项，Esc 返回上一级，Ctrl+C 或根菜单的“退出”结束程序。`mgsctl -h` 与 `mgsctl --help` 会输出完整命令帮助并正常退出。无参数调用只要输入或输出被重定向，也只会打印帮助，不会等待终端输入。

TUI 默认使用中文（`zh-CN`）。在 Language 项切换到英文（`en-US`）后，选择会写入用户配置 `mgsctl/config.json`，下次继续使用。Full/Core 的组件为只读预设，只有 Custom Profile 可以编辑组件选择。

安装成功后，mgsctl 会输出按最终组件计划生成的服务访问摘要和编号后续步骤，只列出实际部署的服务，并将仅 Docker 网络可访问的 PostgreSQL、Redis、MinIO 明确标为内部地址。交互终端安装可以显示一次性 Setup Token；重定向输出永远不显示 Token，只提示在部署主机执行 `mgsctl setup token show --runtime-dir ...`。

Docker `full/single` 默认端口如下：

| 服务 | 直连端口 | Gateway 路径 |
| --- | --- | --- |
| 用户端 | `http://127.0.0.1:5173/` | `http://127.0.0.1/` |
| 管理端 | `http://127.0.0.1:5174/` | `http://127.0.0.1/admin/` |
| 文档站 | `http://127.0.0.1:5175/` | `http://127.0.0.1/developer-docs/` |
| API 与 Setup | `http://127.0.0.1:8080/` | `http://127.0.0.1/api/` 与 `http://127.0.0.1/setup` |

远程访问时将回环地址替换为部署节点 IP，或使用运维人员配置的负载均衡/反向代理。用户端和管理端在直连端口与 Gateway 路径下都会解析到正确 API；初始化未完成时会跳转到 API 托管的 Setup 页面并保留返回地址。文档站在 Setup 期间始终可访问，不会跳转。

### 浏览器 Setup 与首个管理员

初始化完成前，API 只暴露健康检查、bootstrap 状态和 API 自身托管的 Setup 页面。直接打开 `http://<api-host>:<api-port>/setup`；用户端和管理端也会跳转到这里，并保留原始返回地址。

非交互安装不会输出一次性 Setup Token。需要在部署主机读取或轮换：

```bash
mgsctl setup status --runtime-dir ./runtime
mgsctl setup token show --runtime-dir ./runtime
mgsctl setup token reset --runtime-dir ./runtime
```

初始化仍未完成且 Token 已暴露、已使用或遗失时，执行 `token reset`。该操作会使旧 Token 和 Setup 会话失效，并只重启 API 和 Gateway。初始化成功后，Token 显示和重置都会永久关闭。

Setup 流程：

1. 确认公开 API 地址和允许访问的浏览器 Origin。
2. 配置并检测 PostgreSQL、Redis 和对象存储。Docker `full` 的连接字段由部署工具托管且只读，`core` 可编辑。
3. 填写首个管理员邮箱和密码。
4. 复核配置，点击“确认并初始化”，等待迁移和服务重启。
5. 倒计时结束后，浏览器返回原用户端或管理端路由。

应用配置期间容器重启是正常现象，不要仅因此刷新页面。如果恢复超时，依次执行 `status`、`doctor`、`restart`。若 `setup status` 仍为 pending，再重新打开 `/setup`；已认证会话会恢复持久化 operation，而不会再启动第二次迁移。若 Setup 已完成，`/setup` 会保持关闭，应根据 readiness 诊断恢复异常服务。

完成后验证：

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/setup  # 应为 404
mgsctl doctor --runtime-dir ./runtime
```

随后使用首个管理员登录后台，继续配置供应商账号、文本/图片模型、路由、价格、套餐、注册策略、支付/充值和 SMTP。这些业务配置保存在数据库，不放在 `runtime.env` 中。

### 集群部署

首先使用外部共享 PostgreSQL、Redis 和 S3 存储部署一个 Control 节点。Control 负责 Setup、迁移、集群 Token 和配置修订版本：

```bash
./scripts/install.sh install \
  --mode docker \
  --profile core \
  --topology cluster \
  --role control \
  --storage-driver s3 \
  --runtime-dir ./control \
  --public-api-url http://10.0.0.10:8080 \
  --image-tag v1.2.3 \
  --yes
```

必须先在 Control 完成 Setup，才能加入其他节点。然后按角色创建凭证：

```bash
mgsctl cluster token create --role api --ttl 10m --runtime-dir ./control
mgsctl cluster token create --role worker --ttl 10m --runtime-dir ./control
mgsctl cluster token create --role web --ttl 10m --runtime-dir ./control
```

TTL 必须大于 0 且不超过 24 小时。Token 在传输时加密、会过期、限定角色，并且只能使用一次。

在每台目标主机上使用对应角色的 Token 加入：

```bash
mgsctl cluster join \
  --server http://10.0.0.10:8080 \
  --token '<single-use-token>' \
  --mode docker \
  --runtime-dir ./node \
  --image-tag v1.2.3 \
  --api-port 8080
```

`cluster join` 还支持 `--image-registry`、`--release-version` 以及 Gateway/用户/管理/文档端口覆盖。加入时的应用版本必须与 Control 一致。Control API 会通过认证加密信封返回 installation identity、应用密钥、共享中间件配置、schema 版本和配置修订版本，不保存明文 join Token。

多个 API 节点由部署者接入负载均衡。Worker 通过共享任务队列和数据库租约消费任务。Web 角色可以把三个 Web 与 Gateway 从 API 节点拆出。installation identity、应用/schema 版本、配置修订或节点身份不一致时，加入节点会拒绝启动。

### 运行时配置

API 和 Worker 默认从运行工作目录读取 `./config/runtime.env`。只有服务管理器无法设置工作目录时，才使用 `APP_ENV_FILE` 覆盖路径；不支持 `PIC_GALLERY_ENV_FILE`。

生成文件对每个字段都包含详细中英文注释。Setup 只有在全部必填项写入成功后才会认定初始化完成。不要手动修改 `SETUP_COMPLETED`、installation/cluster ID、runtime schema 版本、配置修订或生成的安全密钥；应按字段归属使用 Setup、`upgrade`、`cluster join` 或管理后台修改。

### 状态、重启与诊断

运行时命令按以下顺序定位安装：显式 `--runtime-dir`、当前目录、当前目录下的 `./runtime`、`mgsctl/config.json` 保存的最近运行目录。因此可以从 Runtime directory 外执行常规运维，同时显式参数始终优先。

```bash
mgsctl status --runtime-dir ./runtime
mgsctl doctor --runtime-dir ./runtime
mgsctl restart --runtime-dir ./runtime
```

`doctor` 会检查必填字段、私有文件权限、runtime/manifest/state 身份、中间件连通性、就绪状态和 schema 兼容性，同时对 DSN 与密钥脱敏。查看 Docker 日志时，从 `mgsctl status` 复制容器名后直接检查：

```bash
docker logs --tail=200 <api-container-name>
docker logs --tail=200 <worker-container-name>
```

### 工具版本与手动更新

mgsctl 的普通命令不会联网检查更新。可在本机查看已安装工具版本：

```bash
mgsctl version
mgsctl version --json
```

只有管理员显式执行下面命令时，才更新 mgsctl 二进制：

```bash
mgsctl self-update
mgsctl self-update --version v1.3.0
mgsctl self-update --version v1.3.0 --yes
```

不传 `--yes` 时，self-update 必须经过交互确认。它会下载当前平台的产物和校验文件，在现有可执行文件旁暂存已验证的新文件，只替换部署工具本身；不会重启或升级已部署的 Pic Gallery 运行实例。若所选 Release 不存在，self-update 会停止；在完整源码仓库中可重新运行安装器，使用前文说明的本地 Make 降级。

| 命令 | 更新对象 | 联网行为 |
| --- | --- | --- |
| `mgsctl self-update` | mgsctl 可执行文件 | 仅在管理员显式执行时访问所选 mgsctl Release |
| `mgsctl upgrade` | 已部署的 API、Worker、Web/原生资源及可选数据库迁移 | 按参数解析指定的应用镜像或原生 Release |

### 应用升级与恢复

每次生产应用更新前都应备份 PostgreSQL 与对象存储。不传选择器时默认解析 `latest`；mgsctl 校验 `release-manifest.json` 后推导具体目标版本、拉取不可变 digest，并在 Compose 网络中从目标 API 镜像运行 `mikiko-gallery-studio-db-migrate`。

Docker single/control 节点：

```bash
mgsctl upgrade \
  --runtime-dir ./runtime \
  --image-registry docker.io/fatballfish \
  --image-tag v1.3.0
```

原生 single/control 节点：

```bash
mgsctl upgrade \
  --runtime-dir ./runtime \
  --release-version v1.3.0
```

集群先升级 Control。Control 获取分布式迁移锁，原子更新 runtime 和 manifest，只迁移一次，然后按依赖顺序滚动服务。之后升级加入的 API/Worker/Web 节点，并关闭迁移：

```bash
mgsctl upgrade \
  --runtime-dir ./node \
  --image-tag v1.3.0 \
  --migrate=false
```

原生加入节点应将 `--image-tag` 替换为 `--release-version v1.3.0`，并同样保留 `--migrate=false`。

升级迁移只支持前滚。如果迁移成功但服务滚动失败，使用完全相同的命令重试，即可恢复幂等滚动。如果服务在迁移前失败，mgsctl 会恢复并重新应用旧运行计划。不要通过改回旧镜像标签尝试降级数据库；只有发布版本提供明确恢复流程时，才从已验证备份恢复。

### 停止、卸载与永久删除

普通卸载只停止并注销服务，保留运行配置和持久化数据：

```bash
mgsctl uninstall --runtime-dir ./runtime --yes
```

普通卸载适用于移除服务但保留文件，以便备份或迁移。若要永久删除受管运行目录，以及 Docker 的 PostgreSQL/Redis/MinIO 命名卷，先查询 installation ID，再输入区分大小写的精确短语：

```bash
mgsctl setup status --runtime-dir ./runtime
mgsctl uninstall \
  --runtime-dir ./runtime \
  --delete-data \
  --confirm 'DELETE <installation-id> PERSISTENT DATA'
```

`--yes` 永远不能授权数据删除。破坏性卸载会在停止任何服务或删除任何卷之前，确认运行目录中只有 mgsctl 管理的配置、发布资源、应用数据和日志。执行前必须备份数据库与对象存储。

### 导入旧配置

旧版根目录 `.env`、`.env.prod` 或打包的 `backend.env` 不会被自动加载。需要显式导入到新的运行目录：

```bash
mgsctl import-config \
  --source .env.prod \
  --mode docker \
  --profile full \
  --topology single \
  --storage-driver s3 \
  --runtime-dir ./runtime
```

导入不会修改源文件，并拒绝覆盖已有目标。保留旧文件，直到 `doctor`、readiness、管理员登录和业务 smoke 全部通过。

### 构建与发布 Docker 镜像

自行发布镜像时，需要用相同仓库前缀和标签发布五个应用镜像：

```bash
./scripts/docker/images.sh build --tag v1.3.0 --registry docker.io/fatballfish
./scripts/docker/images.sh push --tag v1.3.0 --registry docker.io/fatballfish
```

一步创建发布版本和可选的 `latest` 标签：

```bash
./scripts/docker/images.sh release \
  --version v1.3.0 \
  --latest \
  --registry docker.io/fatballfish
```

故障恢复、原生服务行为、备份边界和部署验收测试详见 [`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md)。

## 贡献工作流

如果需要基于本项目做二次开发或参与贡献，仓库内置了一套可选的本地 AI 开发工作流。它会安装 Git hooks，并复用仓库内的工作流脚本，用于需求/方案上下文检查、统一验证、本地 review gate 和 pre-push 检查。

克隆仓库后可按需安装一次：

```bash
./scripts/workflow/install-hooks.sh
```

相关工作流文档：

- [`AGENTS.md`](./AGENTS.md)
- [`docs/org/workflow/DEVELOPMENT_WORKFLOW.md`](./docs/org/workflow/DEVELOPMENT_WORKFLOW.md)
- [`docs/org/workflow/REVIEW_GATE.md`](./docs/org/workflow/REVIEW_GATE.md)

## 验证

运行仓库统一验证：

```bash
./scripts/workflow/verify.sh
```

它会执行：

- `go test ./...`
- `go vet ./...`
- 前端共享契约检查
- 用户端 typecheck/build
- 管理端 typecheck/build

运行隔离的 API contract smoke：

```bash
BASE_URL=http://127.0.0.1:8080 ./scripts/workflow/api-smoke.sh
```

前置条件是 Bash、`curl`、Python 3、Go、可用的 Docker daemon，以及本地已有或可从镜像仓库访问 `postgres:16-alpine` 和 `redis:7-alpine`。脚本会自行启动 API、Worker、fake provider、PostgreSQL 和 Redis；`BASE_URL` 只接受带显式空闲端口的 `http://127.0.0.1:<port>` 或 `http://localhost:<port>`，不得包含 path、query、fragment 或 user info，也不会连接预先运行的 API。退出时的清理（cleanup）会停止子进程、删除临时容器，并移除临时 runtime env、存储、日志和测试数据。

运行 Docker E2E：

```bash
./scripts/e2e/run-docker-e2e.sh --start
```

停止共享本地环境并保留数据：

```bash
./scripts/dev/down.sh
```

## 项目结构

```text
api/openapi/              OpenAPI 契约与 API 文档源文件
cmd/api/                  API 服务入口
cmd/worker/               异步 Worker 入口
deployments/              Docker Compose、Nginx 与监控配置
docs/                     PRD、技术方案、计划、评审与 Runbook
internal/app/             应用启动与运行时装配
internal/config/          配置模型与加载器
internal/domain/          领域模型与核心规则
internal/http/            Handler、Middleware、Router 与路由测试
internal/provider/        上游 Provider 适配器
internal/repository/      数据库、缓存、存储仓储
internal/service/         应用服务编排
internal/worker/          异步任务执行和补偿任务
pkg/                      公共工具包
scripts/                  开发、工作流、Smoke 和 E2E 脚本
web/shared/               前端共享 API/Client 辅助代码
web/user/                 用户端 Web
web/admin/                管理端 Web
```

## 文档

- 产品需求：[`docs/prd/pic-gallery-prd.md`](./docs/prd/pic-gallery-prd.md)
- 技术方案：[`docs/tech/pic-gallery-tech-design.md`](./docs/tech/pic-gallery-tech-design.md)
- 产品缺陷闭环方案：[`docs/plans/2026-06-05-product-defect-closure-technical-design.md`](./docs/plans/2026-06-05-product-defect-closure-technical-design.md)
- 验收审计：[`docs/plans/2026-06-07-product-defect-closure-acceptance-audit.md`](./docs/plans/2026-06-07-product-defect-closure-acceptance-audit.md)
- 后端部署 Runbook：[`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md)

## 状态与路线图

Pic Gallery 仍处于持续迭代中的产品实现。当前代码库已经包含用户端、管理端、计费、收银台、公开广场、模型路由、API、Worker 和部署基础。

后续开源强化方向：

- 增加明确的开源许可证。
- 增加公开 Demo 截图和托管文档站点。
- 补充数据库迁移和版本升级文档。
- 增加初始化种子数据或首启引导，用于模型和收银台配置。
- 增加更多 Provider 适配器和生产支付渠道示例。
- 增加 CI 示例，覆盖验证、API smoke 和 Docker E2E。

## 免责声明

本项目会调用第三方 AI 模型服务和支付渠道。你需要自行配置 Provider 凭证，遵守服务条款，保护密钥，审核生成内容，并满足所在地支付、税务、隐私和平台合规要求。

## 许可证

当前仓库还没有包含 LICENSE 文件。若计划正式作为开源项目发布，请先补充许可证。
