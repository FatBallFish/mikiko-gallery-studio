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

## 快速开始

### 环境要求

- [`go.mod`](./go.mod) 声明的 Go 1.26。
- Node.js 20+；推荐 Node.js 22 LTS。
- npm。
- Docker 与 Docker Compose。

### 1. 克隆仓库

```bash
git clone https://github.com/fatballfish/pic-gallery.git
cd pic-gallery
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

开发环境默认管理员账号：

- 邮箱：`admin@example.com`
- 密码：`admin123456`

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

## 本机源码部署

当本机已经具备 Go、Node.js、npm、PostgreSQL、Redis，或中间件由 Docker 启动、应用进程跑在宿主机上时，使用本机源码部署模式。

1. 准备 env 文件：

   ```bash
   mkdir -p config
   cp config/runtime.env.example config/runtime.env
   $EDITOR config/runtime.env
   ```

2. 构建全部组件，或只构建需要的组件：

   ```bash
   ./scripts/local/pgctl.sh build
   ./scripts/local/pgctl.sh build --components api,worker
   ```

3. 以前台或后台方式从源码运行：

   ```bash
   ./scripts/local/pgctl.sh up --components api,worker --background
   ```

4. 如果希望退出登录或重启机器后 API/Worker 仍自动运行，可以安装成本机服务：

   ```bash
   ./scripts/local/pgctl.sh install --components api,worker --user
   ./scripts/local/pgctl.sh status --components api,worker --user
   ```

5. 管理已安装服务：

   ```bash
   ./scripts/local/pgctl.sh start --components api,worker --user
   ./scripts/local/pgctl.sh stop --components api,worker --user
   ./scripts/local/pgctl.sh restart --components api,worker --user
   ./scripts/local/pgctl.sh logs --components api,worker --user
   ./scripts/local/pgctl.sh uninstall --components api,worker --user
   ```

如果 API/Worker 跑在宿主机，但 PostgreSQL/Redis 等中间件由 Docker 管理，先启动中间件：

```bash
make compose-middleware-up
./scripts/local/pgctl.sh up --components api,worker --background
```

API 默认监听 `http://127.0.0.1:8080`。部署后可用下面命令检查：

```bash
curl http://127.0.0.1:8080/readyz
```

### 操作系统服务

本机服务管理脚本支持 Linux、macOS 和 Windows。Shell 脚本默认管理 API 和 Worker；建议显式传入 `--components api,worker`，避免把前端开发服务器作为服务安装。

### Linux

Linux 使用 `systemd`。

安装用户级服务：

```bash
./scripts/service/manage.sh install --components api,worker --user
```

卸载用户级服务：

```bash
./scripts/service/manage.sh uninstall --components api,worker --user
```

安装系统级服务：

```bash
sudo ./scripts/service/manage.sh install --components api,worker
```

卸载系统级服务：

```bash
sudo ./scripts/service/manage.sh uninstall --components api,worker
```

### macOS

macOS 使用 `launchd`。

安装用户级服务：

```bash
./scripts/service/manage.sh install --components api,worker --user
```

卸载用户级服务：

```bash
./scripts/service/manage.sh uninstall --components api,worker --user
```

安装系统级守护进程：

```bash
sudo ./scripts/service/manage.sh install --components api,worker
```

卸载系统级守护进程：

```bash
sudo ./scripts/service/manage.sh uninstall --components api,worker
```

### Windows

Windows 源码服务安装使用计划任务托管，避免普通 Go/Vite 前台进程必须额外依赖服务包装器。

安装服务：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/manage.ps1 install -Components "api,worker" -EnvFile "config/runtime.env"
```

卸载服务：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/manage.ps1 uninstall -Components "api,worker"
```

查看状态：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/service/manage.ps1 status -Components "api,worker"
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

## Docker Compose 部署

生产 Compose 文件位于 [`deployments/docker-compose/docker-compose.prod.yml`](./deployments/docker-compose/docker-compose.prod.yml)。生产 Compose 从 `PIC_GALLERY_IMAGE_REGISTRY` 拉取预构建镜像，不再从本地源码构建，也不再挂载 `config.yaml`。

### 新项目 Clone 后首次部署

当一台新服务器刚 clone 仓库，并希望由 Docker Compose 管理 PostgreSQL、Redis、API、Worker、前端容器和 Nginx 时，使用这个流程。

```bash
cp deployments/docker-compose/.env.prod.example deployments/docker-compose/.env.prod
$EDITOR deployments/docker-compose/.env.prod

docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml pull
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml up -d
```

首次启动前至少要设置这些值：

- `PIC_GALLERY_IMAGE_REGISTRY`
- `PIC_GALLERY_IMAGE_TAG`
- `POSTGRES_PASSWORD`
- `AUTH_ACCESS_TOKEN_SECRET`
- `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`
- `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`
- `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`
- `PIC_GALLERY_ADMIN_EMAIL`
- `PIC_GALLERY_ADMIN_PASSWORD`
- `CORS_ALLOWED_ORIGINS`

检查部署状态：

```bash
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml ps
curl http://localhost:${NGINX_PORT:-80}/readyz
```

也可以生成独立部署目录并自动生成密钥。注意保持 `docker-compose/` 目录和 `nginx/` 目录同级，这样 Compose 文件里的 Nginx 相对挂载路径才能正确解析：

```bash
mkdir -p pic-gallery-deploy && cd pic-gallery-deploy
mkdir -p docker-compose
cd docker-compose
/path/to/pic-gallery/deployments/docker-compose/prepare.sh
cd ..
cp -R /path/to/pic-gallery/deployments/nginx ./nginx
cd docker-compose
$EDITOR .env.prod
docker compose --env-file .env.prod -f docker-compose.yml pull
docker compose --env-file .env.prod -f docker-compose.yml up -d
```

生产栈包含 PostgreSQL、Redis、API、Worker、用户端 Web、管理端 Web、Nginx、共享存储和可选 Prometheus。PostgreSQL、Redis、API、Worker 和前端容器都在同一个 Compose network 中，不对宿主机发布端口；Nginx 是唯一公开入口。

默认公开路由：

- 用户端 Web：`http://localhost:${NGINX_PORT:-80}/`
- 管理端 Web：`http://localhost:${NGINX_PORT:-80}/admin/`
- API 与文档：通过 `/api/*`、`/docs/*`、`/v1/*`、`/healthz`、`/readyz` 代理

部署细节见 [`docs/runbooks/backend-deployment.md`](./docs/runbooks/backend-deployment.md)。

### 后续版本更新

Docker 部署更新时，先发布新镜像，再只修改 `.env.prod` 里的 `PIC_GALLERY_IMAGE_TAG`，然后拉取并重启：

```bash
$EDITOR deployments/docker-compose/.env.prod
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml pull
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml up -d
docker compose --env-file deployments/docker-compose/.env.prod \
  -f deployments/docker-compose/docker-compose.prod.yml ps
curl http://localhost:${NGINX_PORT:-80}/readyz
```

本机源码部署更新时，拉取新代码、重新构建并重启已安装服务：

```bash
git pull
./scripts/local/pgctl.sh build --components api,worker
./scripts/local/pgctl.sh restart --components api,worker --user
curl http://127.0.0.1:8080/readyz
```

回滚使用同一套流程：Docker 模式把 `PIC_GALLERY_IMAGE_TAG` 改回上一个镜像标签；本机模式把 Git 代码切回上一个版本后重新构建并重启。

### 本地模式与 Docker 模式

本地源码运行使用 [`scripts/local/pgctl.sh`](./scripts/local/pgctl.sh)：

```bash
mkdir -p config
cp config/runtime.env.example config/runtime.env
./scripts/local/pgctl.sh build --components api,worker
./scripts/local/pgctl.sh up --components api,worker --background
./scripts/service/manage.sh status --user
```

Docker 模式使用 Compose 和镜像仓库：

- `docker-compose.local.yml` 是开发与 Docker E2E 共用的唯一环境，构建本地镜像并保留 PostgreSQL 和对象存储卷。
- `docker-compose.prod.yml` 面向部署，只拉取预构建镜像。

镜像构建与发布：

```bash
./scripts/docker/images.sh build --tag test --registry docker.io/your-org
./scripts/docker/images.sh push --tag test --registry docker.io/your-org
./scripts/docker/images.sh release --version v1.2.3 --latest --registry docker.io/your-org
```

## 开发指南

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

对运行中的 API 执行 smoke：

```bash
BASE_URL=http://127.0.0.1:8080 ./scripts/workflow/api-smoke.sh
```

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
