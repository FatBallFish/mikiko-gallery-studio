# Pic Gallery 配置管理与部署发布重构方案

日期：2026-06-16

## 1. 需求描述

### 1.1 背景与预期效果

当前项目的配置与发布部署链路存在多套并行机制：

- 后端运行时配置主要来自 `config.yaml`，但仓库内同时存在 `.env`、`configs/config.*.yaml`、`deployments/docker-compose/.env*`。
- 生产 Docker Compose 仍通过 `build:` 动态构建 API、worker、user-web、admin-web 镜像，并挂载 `config.yaml`。
- DevOps 包通过 `scripts/devops/package.sh` 按 `APP_ENV` 选择 `configs/config.<env>.yaml` 并复制为产物内 `config.yaml`。
- Makefile、本地 service 脚本、devops run 脚本和 Docker Compose 脚本职责重叠。
- SMTP、billing、cashier、provider routing、BFSS/S3 等大量非启动必需配置混入部署配置，导致运维人员难以判断首部署必须配置哪些 key，也导致业务配置修改需要重启服务。

预期效果：

1. 部署配置统一回到 `.env`，移除 `config.yaml` 作为生产配置入口。
2. `.env` 只保留“进程启动必需、基础设施连接、稳定安全密钥、极少量运维开关”。
3. SMTP、billing、cashier、模型路由、对象存储/BFSS 等非首启动必填配置迁入数据库配置表，通过管理后台配置并动态生效。
4. 部署安装流程收敛为两类：本地模式和 Docker 模式。
5. Docker 模式从镜像源拉取镜像部署，并补充构建/推送镜像脚本。
6. 本地模式覆盖 Windows、Linux、macOS，支持纯构建、构建+运行、构建+安装服务+运行。
7. 提供统一本地服务管理脚本，管理 `api-server` 与 `worker` 的 install、uninstall、stop、start、restart、status、logs。

### 1.2 参考资料

仓库现状：

- `internal/config/config.go`、`internal/config/load.go`
- `configs/config.example.yaml`、`configs/config.pro.yaml`、`configs/config.compose.prod.yaml`
- `.env.example`、`deployments/docker-compose/.env.example`
- `deployments/docker-compose/docker-compose.prod.yml`
- `scripts/devops/package.sh`
- `scripts/service/install.sh`、`scripts/service/uninstall.sh`
- `docs/plans/2026-06-08-admin-secure-config-and-cashier-packages-design.md`
- `docs/plans/2026-06-16-devops-systemd-config-file.md`

参考项目：

- `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api/deploy/docker-compose.local.yml`
- `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api/deploy/.env.example`
- `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api/deploy/docker-deploy.sh`
- `/Users/fatballfish/Documents/Projects/GoProjects/Personal/sub2api/deploy/install.sh`

外部实践参考：

- Docker Compose 支持通过 `env_file` 向容器注入环境变量，并且 `environment` 会覆盖 `env_file`：https://docs.docker.com/reference/compose-file/services/#env_file
- Docker Compose 支持先 `pull` 服务镜像再 `up` 启动：https://docs.docker.com/reference/cli/docker/compose/pull/
- Twelve-Factor 建议将部署差异配置从代码中分离，并通过环境暴露给进程：https://12factor.net/config

### 1.3 目标拆解

| 子目标 | 范围说明 | 交付标准 | 优先级 |
|---|---|---|---|
| 配置分层 | env、数据库配置、代码默认值三层职责清晰 | 生产部署只需维护一份 `.env`，业务配置可后台改 | P0 |
| 配置加载改造 | 后端从 env 加载启动配置，支持 DB 覆盖 | 优先级为数据库配置 > 本地环境变量配置 > 代码默认值 | P0 |
| 数据库配置迁移 | SMTP、billing、cashier、provider routing、storage/BFSS 分类入库 | 管理后台可查看/修改，敏感字段 write-only 加密 | P0 |
| 本地模式 | 跨 Windows/Linux/macOS 的构建、运行、安装服务 | 一套入口脚本支持 build/run/install/service ops | P0 |
| Docker 模式 | 生产 Compose 改为拉取镜像 | `docker compose pull && docker compose up -d`，不依赖源码构建 | P0 |
| 镜像发布 | 构建并推送 API、worker、web 镜像 | 支持 `--tag`，默认 `test`，正式版同时推版本 tag 和 `latest` | P0 |
| 文档迁移 | README/runbook/示例配置统一 | 运维文档只描述新流程，不保留多套入口 | P1 |

## 2. 当前实现基线

### 2.1 配置加载

当前 `internal/config/load.go`：

- 默认读取 `config.yaml`。
- `APP_CONFIG_PATH`、`PIC_GALLERY_CONFIG` 仅作为配置文件路径选择器。
- 配置内容用 YAML 反序列化。
- `applyDefaults` 补充大量业务默认值。
- 测试明确断言：旧的业务 env 覆盖会被忽略。

当前配置结构 `internal/config/config.go` 包含：

- 启动基础设施：`app`、`database`、`redis`、`storage`、`auth`、`admin`
- 安全密钥：`auth.access_token_secret`、`api_key.signing_secret_encryption_key`、`cashier.provider_config_encryption_key`、`security.secure_config_encryption_key`
- 业务配置：`billing`、`cashier`、`generation_limits`、`providers`、`routing`、`docs`
- SMTP：`auth.smtp`

### 2.2 已具备的动态配置基础

当前项目已经实现：

- 普通配置表：`system_configs`，Ent schema 为 `ConfigItem`，适合非敏感业务配置。
- 敏感配置表：`secure_configs`，Ent schema 为 `SecureConfig`，支持 public value、secret encrypted、fingerprint、secret fields、version、updated by。
- SMTP 安全配置服务：`internal/service/secureconfig`，已经支持 SMTP 密码加密存储、write-only 更新和运行时解析。
- 管理后台配置服务：`internal/service/adminconfig`，已经提供 billing、generation limits、runtime、payments 等默认定义和数据库覆盖。
- Cashier 配置 facade：`internal/service/cashier/config_store.go`，已从 admin config 读取部分支付配置。

结论：迁移不需要从零建设配置中心，而应扩展现有 `system_configs` 与 `secure_configs`，并把所有仍直接依赖 `config.Config` 的业务运行时路径改为 resolver/facade。

### 2.3 部署脚本现状

当前存在三套部署/构建思路：

- Makefile：`dev`、`worker`、`compose-up`、`service-install`。
- DevOps 包：`scripts/devops/package.sh` 构建四类产物，并按 `APP_ENV` 复制 YAML。
- Docker Compose：`docker-compose.prod.yml` 直接 `build:` 本地源码镜像，并挂载 YAML。

此外服务管理分裂为：

- `deployments/devops/run-api-server.sh`、`run-worker.sh`：仅面向 Linux systemd、且安装即重启。
- `scripts/service/install.sh`、`uninstall.sh`：本地源码运行，支持 Linux/macOS，但没有统一 start/stop/status/logs。
- README 还提到 Windows PowerShell 脚本，但当前文件列表未看到 `scripts/service/install.ps1`、`uninstall.ps1`。

## 3. 技术选型与方案对比

### 3.1 方案 A：继续 YAML，但清理字段

做法：

- 保留 `config.yaml` 作为主配置。
- 精简 YAML 内容，将业务配置迁入数据库。
- Docker 仍挂载 YAML 或按环境复制 YAML。

优点：

- 代码改动最少。
- 迁移风险低。

缺点：

- 不符合“所有 `config.yaml` 改回 env 文件配置”的目标。
- 运维仍要理解 YAML 路径、挂载和 env 文件两套概念。
- DevOps 的 `APP_ENV -> config.<env>.yaml` 惯性仍会残留。

结论：不推荐。

### 3.2 方案 B：env 启动配置 + 数据库业务配置

做法：

- 启动时只读取 `.env`/进程环境变量，不再读取 YAML。
- env 仅保留启动必需项与安全主密钥。
- 业务配置使用 `system_configs`/`secure_configs`，管理后台修改后动态读取。
- Docker Compose、systemd、launchd、Windows service 都只注入 env。

优点：

- 与用户目标一致。
- 运维只维护一个 `.env`。
- 业务配置不需要重启。
- 可自然对齐 Docker Compose、systemd、Windows/macOS 服务模型。
- 与 Twelve-Factor “配置从代码中分离、通过环境暴露给进程”的方向一致。

缺点：

- 需要改造配置加载和业务配置解析路径。
- 需要补齐后台配置页和迁移逻辑。

结论：推荐。

### 3.3 方案 C：完整安装向导优先

做法：

- 参考 sub2api，首次启动只靠极少 env 启动 Web，数据库/Redis/Admin 也通过 setup wizard 写入配置。

优点：

- 运维体验最好。

缺点：

- 当前 Pic Gallery 的数据库连接是服务启动前置条件，短期要做离线 setup wizard 会显著扩大改造范围。
- 会引入“未配置 DB 也能启动”的新状态机和安全边界。

结论：作为后续演进方向，不作为本轮主方案。

## 4. 推荐总体架构

采用“env 启动配置 + 数据库动态配置 + 代码默认值”的三层模型。

```mermaid
flowchart TD
  Env[.env / process env] --> BootConfig[BootConfig 启动配置]
  BootConfig --> DB[(PostgreSQL)]
  BootConfig --> Redis[(Redis)]
  BootConfig --> API[api-server]
  BootConfig --> Worker[worker]

  API --> SystemConfigs[(system_configs)]
  API --> SecureConfigs[(secure_configs)]
  Worker --> SystemConfigs
  Worker --> SecureConfigs

  Admin[管理后台] --> AdminAPI[Ops Admin API]
  AdminAPI --> SystemConfigs
  AdminAPI --> SecureConfigs

  SystemConfigs --> RuntimeResolver[运行时配置 Resolver]
  SecureConfigs --> RuntimeResolver
  RuntimeResolver --> Business[SMTP / Billing / Cashier / Storage / Provider Routing]
```

配置优先级：

1. 数据库配置：`system_configs`、`secure_configs`。
2. 进程环境变量：`.env`、systemd `EnvironmentFile`、Compose `env_file`、Windows service env。
3. 代码默认值：只用于本地开发与非关键业务默认。

关键约束：

- 数据库连接自身不能被数据库配置覆盖，只能来自 env。
- Redis 连接建议只来自 env；如未来支持在线切换 Redis，需要单独设计连接重建与缓存一致性，不纳入本轮。
- 安全主密钥本轮仍只来自 env。`AUTH_ACCESS_TOKEN_SECRET`、API Key 加密密钥、收银台配置加密密钥、secure config 加密密钥涉及 token 失效、密文重加密和缓存失效策略；在确认轮换方案前，不做 DB 覆盖。

## 5. 配置文件设计

### 5.1 生产 `.env` 最小集合

建议统一文件：`.env.example`，生产复制为 `.env`。

```dotenv
# Runtime
PIC_GALLERY_ENV=production
PIC_GALLERY_ADDR=:8080
PIC_GALLERY_TIMEZONE=Asia/Shanghai
PIC_GALLERY_LOG_LEVEL=info

# Database
DATABASE_URL=postgres://pic_gallery:change-me@127.0.0.1:5432/pic_gallery?sslmode=disable
DATABASE_MAX_OPEN_CONNS=20
DATABASE_MAX_IDLE_CONNS=10
DATABASE_CONN_MAX_LIFETIME=30m

# Redis
REDIS_URL=redis://127.0.0.1:6379/0
REDIS_KEY_PREFIX=pic-gallery

# Auth and encryption secrets
AUTH_ACCESS_TOKEN_SECRET=change-me-32-bytes-min
API_KEY_SIGNING_SECRET_ENCRYPTION_KEY=change-me-32-bytes-min
CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY=change-me-32-bytes-min
PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY=change-me-32-bytes-min

# First admin bootstrap, only used when no admin exists
PIC_GALLERY_ADMIN_EMAIL=admin@example.com
PIC_GALLERY_ADMIN_PASSWORD=change-me-before-first-start
PIC_GALLERY_ADMIN_ROLE=super_admin

# Optional local storage bootstrap
STORAGE_DRIVER=local
STORAGE_LOCAL_ROOT=/var/lib/pic-gallery/storage
STORAGE_PUBLIC_BASE_URL=
STORAGE_SHARED_VOLUME=false

# Optional frontend/gateway
NGINX_PORT=80
PIC_GALLERY_IMAGE_REGISTRY=docker.io/your-org
PIC_GALLERY_IMAGE_TAG=latest
```

说明：

- 不在 `.env` 中默认暴露 SMTP、billing、cashier pricing、provider model map、OpenAI/OpenRouter API Key、BFSS/S3 详细参数。
- `STORAGE_*` 作为“首启动能落盘”的基础配置保留；BFSS/S3 详细凭据迁入 `secure_configs`，后台配置后可切换。
- `PIC_GALLERY_ADMIN_PASSWORD` 只在首次 bootstrap 使用。创建管理员后后续启动忽略，避免每次启动重置密码。
- 生产环境如果安全密钥为空或仍是 local-dev 默认值，启动失败。

### 5.2 Docker Compose `.env` 集合

Docker 部署目录保留一份 `.env.example`，但内容与根 `.env.example` 对齐，仅增加 Compose 自身变量：

```dotenv
COMPOSE_PROJECT_NAME=pic-gallery
PIC_GALLERY_IMAGE_REGISTRY=registry.cn-hangzhou.aliyuncs.com/your-namespace
PIC_GALLERY_IMAGE_TAG=latest
POSTGRES_DB=pic_gallery
POSTGRES_USER=pic_gallery
POSTGRES_PASSWORD=change-me
REDIS_PASSWORD=
NGINX_PORT=80
```

Compose 内部将应用连接写死为服务名：

- `DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable`
- `REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379/0`，无密码时使用 `redis://redis:6379/0`

### 5.3 废弃配置文件

以下文件应在迁移期后删除或转为文档说明，不再作为运行配置：

- `configs/config.dev.yaml`
- `configs/config.pro.yaml`
- `configs/config.compose.dev.yaml`
- `configs/config.compose.prod.yaml`
- `configs/config.compose.e2e.yaml`
- `configs/config.example.yaml`

如需从旧版本迁移，可从旧发布包、服务器备份或运维当前持有的旧 `config.yaml` 输入，并提供一次性迁移脚本：

```bash
scripts/config/migrate-yaml-to-db.sh --config /path/to/old/config.yaml --env .env
```

该脚本职责：

- 将启动必需项写入 `.env`。
- 将普通业务配置写入 `system_configs`。
- 将 SMTP、provider key、BFSS/S3 secret、支付密钥写入 `secure_configs`。
- 输出迁移报告，列出不能自动识别的 key。

## 6. 数据库配置设计

### 6.1 配置分类

| 分类 | 存储表 | 是否敏感 | 示例 key | 是否动态生效 |
|---|---|---|---|---|
| `auth_security` | `system_configs` | 否 | access token TTL、refresh cookie name | 部分动态 |
| `smtp` | `secure_configs` | 是 | host、port、username、password | 是 |
| `billing_pricing` | `system_configs` | 否 | cny_per_point、task_multipliers | 是 |
| `billing_trial` | `system_configs` | 否 | signup_trial | 是 |
| `payments` | `system_configs` + 业务表 | 部分敏感 | visible_methods、provider_instances | 是 |
| `storage` | `system_configs` + `secure_configs` | 是 | driver、bucket、access key | 需连接重建 |
| `providers` | `secure_configs` | 是 | OpenAI/OpenRouter base_url、api_key | 是 |
| `routing` | `system_configs` | 否 | provider_model_map、fallback providers | 是 |
| `generation_limits` | `system_configs` | 否 | prompt/image/ref limits | 是 |
| `runtime` | `system_configs` | 否 | worker_max_concurrent_tasks | 部分动态 |
| `docs` | `system_configs` | 否 | title、base_path | 是 |

### 6.2 已有表复用

普通配置继续使用：

```text
system_configs(config_category, config_key, scope, config_value, version, updated_by, updated_at)
```

敏感配置继续使用：

```text
secure_configs(config_category, config_key, public_value, secret_encrypted, secret_fingerprint, secret_fields, version, updated_by)
```

新增建议：

- 增加 `description`、`value_schema`、`updated_at` 到普通配置元数据可先不落表，首期通过代码定义。
- 增加配置审计日志表 `config_audit_logs`，记录配置变更但不记录明文 secret。

### 6.3 运行时 Resolver

新增或扩展以下 resolver：

- `RuntimeConfigResolver`
  - `GetBillingConfig(ctx)`
  - `GetGenerationLimits(ctx)`
  - `GetProviderRouting(ctx)`
  - `GetStorageConfig(ctx)`
  - `GetWorkerRuntime(ctx)`
- `SecureProviderConfigResolver`
  - `GetProviderConfig(ctx, provider)`
  - `ListEnabledProviders(ctx)`
- `StorageBackendResolver`
  - 根据 DB 配置构建 local/S3/BFSS backend。
  - 对连接型 backend 使用短 TTL 缓存，例如 30s。

缓存策略：

- API 请求路径可每 30s 刷新一次配置快照，避免每次查 DB。
- 配置更新后通过 Redis pub/sub 或本地 version polling 触发失效。
- worker 对 routing、billing、provider、storage 配置同样使用 snapshot，保证任务执行期间配置一致。

### 6.4 敏感字段处理

延续已有安全契约：

- GET/list/detail 不返回明文。
- PUT/POST 支持 `secrets` 与 `clear_secrets`。
- 更新时未传 secret 表示保留旧值。
- 禁止将 `******` 等脱敏占位符写回。
- 审计只记录 `has_secret`、fingerprint、字段名、更新人。

## 7. 部署流程设计

### 7.1 本地模式

建议新增统一入口：

```text
scripts/local/pic-gallery.ps1
scripts/local/pic-gallery.sh
```

为了真正支持 Windows/Linux/macOS，推荐最终实现为一个 Go 小工具：

```text
tools/pgctl/main.go
```

脚本只是 wrapper：

```bash
go run ./tools/pgctl --mode local build
go run ./tools/pgctl --mode local run
go run ./tools/pgctl --mode local install --components api,worker
```

命令设计：

```bash
scripts/local/pgctl build --components api,worker,user-web,admin-web
scripts/local/pgctl run --components api,worker
scripts/local/pgctl up --build --components all
scripts/local/pgctl install --components api,worker --user
scripts/local/pgctl uninstall --components api,worker --user
scripts/local/pgctl start --components api,worker
scripts/local/pgctl stop --components api,worker
scripts/local/pgctl restart --components api,worker
scripts/local/pgctl status --components api,worker
scripts/local/pgctl logs --component api --follow
```

本地模式的三个操作档位：

| 档位 | 命令 | 行为 |
|---|---|---|
| 纯构建 | `pgctl build` | Go build API/worker，npm build web，不启动 |
| 构建+运行 | `pgctl up --build` | 构建后以前台或后台进程运行 |
| 构建+安装服务+运行 | `pgctl install --start` | 构建后安装到系统服务并启动 |

服务托管适配：

- Linux：systemd user/system service。
- macOS：launchd LaunchAgents/LaunchDaemons。
- Windows：Windows Service，优先用 Go service 库或 `sc.exe` 包装已构建 exe。

### 7.2 本地服务管理脚本

统一服务名：

- `pic-gallery-api`
- `pic-gallery-worker`

服务配置：

- Linux systemd：使用 `EnvironmentFile=/opt/pic-gallery/.env` 或用户指定路径。
- macOS launchd：在 plist 中注入 env 或通过 wrapper 读取 `.env`。
- Windows service：通过 wrapper exe 读取 `.env` 后启动子进程，避免 Windows Service 环境变量维护困难。

日志：

- Linux：`journalctl -u pic-gallery-api -f`
- macOS：`~/Library/Logs/pic-gallery/api.log` 或 launchd stdout/stderr path。
- Windows：优先写文件到 `%ProgramData%\PicGallery\logs\`，后续可接 Event Log。

### 7.3 Docker 模式

生产 Compose 不再包含应用镜像 `build:`。

推荐服务结构：

```yaml
services:
  api:
    image: ${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}/pic-gallery-api:${PIC_GALLERY_IMAGE_TAG:-latest}
    env_file: .env
    environment:
      DATABASE_URL: postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable
      REDIS_URL: redis://redis:6379/0

  worker:
    image: ${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}/pic-gallery-worker:${PIC_GALLERY_IMAGE_TAG:-latest}
    env_file: .env

  user-web:
    image: ${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}/pic-gallery-user-web:${PIC_GALLERY_IMAGE_TAG:-latest}

  admin-web:
    image: ${PIC_GALLERY_IMAGE_REGISTRY:-docker.io/your-org}/pic-gallery-admin-web:${PIC_GALLERY_IMAGE_TAG:-latest}
```

部署命令：

```bash
docker compose --env-file .env -f docker-compose.yml pull
docker compose --env-file .env -f docker-compose.yml up -d
```

不再使用：

```bash
docker compose up -d --build
```

参考 sub2api 的部署准备脚本，新增：

```bash
deploy/docker-prepare.sh
```

职责：

- 下载/复制 `docker-compose.yml` 和 `.env.example`。
- 生成 `POSTGRES_PASSWORD`、`AUTH_ACCESS_TOKEN_SECRET`、`API_KEY_SIGNING_SECRET_ENCRYPTION_KEY`、`CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY`、`PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY`。
- 创建 `data/`、`postgres_data/`、`redis_data/`、`storage/`。
- 设置 `.env` 权限为 `0600`。
- 输出下一步命令。

### 7.4 镜像构建与推送

新增：

```bash
scripts/docker/build-images.sh
scripts/docker/push-images.sh
scripts/docker/release-images.sh
```

也可以合并为：

```bash
scripts/docker/images.sh build --tag test
scripts/docker/images.sh push --tag test --registry registry.cn-hangzhou.aliyuncs.com/ns
scripts/docker/images.sh release --version v1.2.3 --latest
```

规则：

- 未传 tag 时默认 `test`。
- 正式发布时必须传 `--version vX.Y.Z`，脚本同时打：
  - `vX.Y.Z`
  - `latest`
- 镜像名：
  - `pic-gallery-api`
  - `pic-gallery-worker`
  - `pic-gallery-user-web`
  - `pic-gallery-admin-web`
- 支持 registry：
  - 阿里云私有镜像源：`registry.cn-hangzhou.aliyuncs.com/<namespace>`
  - DockerHub：`docker.io/<org>`
- 推送前执行：
  - `docker buildx build` 或普通 `docker build`
  - `docker image inspect`
  - 可选 `docker run --rm image --version`

## 8. 启动配置加载改造

### 8.1 新配置结构

将当前 `config.Config` 拆为两类：

```go
type BootConfig struct {
    App      AppConfig
    Database DatabaseConfig
    Redis    RedisConfig
    Auth     AuthBootConfig
    Admin    AdminBootstrapConfig
    Security SecurityKeysConfig
    Storage  StorageBootstrapConfig
    HTTP     HTTPBootConfig
    Docs     DocsBootConfig
}
```

仍保留业务结构体，但不再作为启动配置直接加载：

- `BillingConfig`
- `CashierConfig`
- `GenerationLimitsConfig`
- `ProvidersConfig`
- `RoutingConfig`
- `SMTPConfig`

这些结构体由 resolver 从 DB/env/default 组装。

### 8.2 env 解析

建议新增：

```go
func LoadEnv(path string) (BootConfig, error)
```

加载顺序：

1. 如果显式传 `--env-file` 或通用路径覆盖变量，读取指定文件。
2. 否则读取工作目录下的 `./config/runtime.env`。
3. 文件中的运行时字段为权威值，进程同名变量不覆盖。
4. 应用代码默认值。
5. 校验生产环境必填项。

兼容期：

- `LoadYAML(path string)` 保留但仅作为显式迁移/测试入口。
- API/worker 默认 `Load("")` 只走 env；如果 `APP_CONFIG_PATH`/`PIC_GALLERY_CONFIG` 存在也不会回退为生产配置入口。
- 一个小版本后删除 YAML path selector。

### 8.3 CLI 参数

API/worker 支持：

```bash
pic-gallery-api --env-file ./config/runtime.env
pic-gallery-worker --env-file ./config/runtime.env
```

环境变量：

```dotenv
APP_ENV_FILE=./config/runtime.env
```

优先级：

```text
命令行 --env-file > APP_ENV_FILE > 当前工作目录 ./config/runtime.env
```

注意：Docker Compose 已通过 `env_file` 注入环境，容器内不需要再读取 `.env` 文件。

## 9. 配置迁移映射

| 当前 YAML 路径 | 新位置 | 说明 |
|---|---|---|
| `app.env` | `PIC_GALLERY_ENV` | 启动配置 |
| `app.addr` | `PIC_GALLERY_ADDR` | 启动配置 |
| `database.*` | `DATABASE_*` | 启动配置 |
| `redis.*` | `REDIS_*` | 启动配置 |
| `auth.access_token_secret` | `AUTH_ACCESS_TOKEN_SECRET` | 安全主密钥，本轮不做 DB 覆盖 |
| `auth.smtp.*` | `secure_configs:smtp/default` | 后台配置，env 仅迁移兜底 |
| `admin.seed_*` | `PIC_GALLERY_ADMIN_*` | 仅首次 bootstrap |
| `api_key.signing_secret_encryption_key` | `API_KEY_SIGNING_SECRET_ENCRYPTION_KEY` | 安全主密钥，本轮不做 DB 覆盖 |
| `cashier.provider_config_encryption_key` | `CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY` | 安全主密钥，本轮不做 DB 覆盖 |
| `security.secure_config_encryption_key` | `PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY` | 安全主密钥，本轮不做 DB 覆盖 |
| `http.cors_allowed_origins` | `CORS_ALLOWED_ORIGINS` 或 DB | 运维/业务均可 |
| `billing.*` | `system_configs` | 后台配置 |
| `cashier.enabled/mock/order limits/site_base_url` | `system_configs:payments` | 后台配置 |
| `generation_limits.*` | `system_configs:generation_limits` | 后台配置 |
| `providers.*.base_url/api_key` | `secure_configs:providers/<name>` | 后台配置 |
| `routing.*` | `system_configs:routing` | 后台配置 |
| `storage.driver/local_root/public_base_url/shared_volume` | env bootstrap + DB 覆盖 | local 必需可在 env |
| `storage.s3.*` / BFSS | `secure_configs:storage/default` | 后台配置 |
| `worker.max_concurrent_tasks` | `system_configs:runtime` | 可动态调整，worker snapshot 生效 |
| `docs.*` | `system_configs:docs` | 后台配置或默认值 |

## 10. 业务流程

### 10.1 首次 Docker 部署

```mermaid
sequenceDiagram
  participant Ops as 运维
  participant Prep as docker-prepare.sh
  participant Compose as Docker Compose
  participant API as api-server
  participant DB as PostgreSQL
  participant Admin as 管理后台

  Ops->>Prep: 执行准备脚本
  Prep->>Ops: 生成 .env、compose、数据目录
  Ops->>Compose: docker compose pull
  Ops->>Compose: docker compose up -d
  Compose->>API: 注入 env 并启动
  API->>DB: 迁移 schema / bootstrap admin / seed 默认配置
  Ops->>Admin: 登录后台
  Admin->>API: 配置 SMTP、存储、模型供应商、计费、支付
  API->>DB: 写入 system_configs / secure_configs
```

### 10.2 运行中配置修改

```mermaid
sequenceDiagram
  participant Admin as 管理员
  participant API as Ops API
  participant DB as system_configs/secure_configs
  participant Redis as Redis PubSub
  participant Worker as worker

  Admin->>API: PUT 配置
  API->>DB: 校验并保存
  API->>Redis: publish config changed
  API->>API: 失效本地配置缓存
  Redis->>Worker: 通知配置变更
  Worker->>DB: 下一次 snapshot 刷新
```

### 10.3 异常路径

| 场景 | 行为 |
|---|---|
| `.env` 缺少数据库连接 | 进程启动失败，日志明确指出缺失 key |
| 生产环境安全密钥为空 | 进程启动失败 |
| DB 配置读取失败 | API 对应业务返回明确错误；worker 保留上一次有效 snapshot 并告警 |
| DB 配置格式错误 | 后台写入时拒绝；若历史脏数据存在，resolver 回退 env/default 并告警 |
| Redis pub/sub 不可用 | 退化为周期性轮询 version |
| 管理员写入 secret 占位符 | 返回 400，不覆盖旧 secret |
| storage 配置切换失败 | 保留旧 backend，后台测试连接失败，不发布为 active |
| worker 并发配置调低 | 新任务租约按新并发限制执行，已执行任务不强杀 |

## 11. 接口设计

复用现有 Ops Admin API 分类：`/api/ops/admin/v1/...`。

需要补齐或扩展：

```http
GET /api/ops/admin/v1/config/tabs
GET /api/ops/admin/v1/config/tabs/{tab_key}
PUT /api/ops/admin/v1/config/tabs/{tab_key}
```

新增推荐：

```http
GET /api/ops/admin/v1/security/providers
PUT /api/ops/admin/v1/security/providers/{provider}
POST /api/ops/admin/v1/security/providers/{provider}/test

GET /api/ops/admin/v1/security/storage
PUT /api/ops/admin/v1/security/storage
POST /api/ops/admin/v1/security/storage/test

GET /api/ops/admin/v1/runtime/config-effective
```

`config-effective` 用于运维排查：

- 返回每个 key 的来源：`db`、`env`、`default`。
- 敏感值只返回是否配置和 fingerprint。
- 只允许 Ops admin 访问。

幂等性：

- PUT 配置使用 `version` 乐观锁。
- secret 更新保留三态语义。

限流：

- 管理配置接口按管理员维度限流，例如 60 req/min。
- `test` 类接口按配置对象维度限流，防止 SMTP/对象存储/供应商被滥测。

## 12. 架构变更

新增：

- `tools/pgctl` 本地管理 CLI。
- `scripts/docker/images.sh` 镜像构建/推送脚本。
- `deploy/docker-prepare.sh` Docker 部署准备脚本。
- `deploy/docker-compose.yml` 或替换 `deployments/docker-compose/docker-compose.prod.yml` 为纯镜像部署。
- `internal/config/env.go` 或替换 `load.go` 为 env loader。
- runtime config resolver/facade。
- storage/provider secure config 管理 API 与后台页面。

移除或废弃：

- `configs/config.*.yaml` 生产运行依赖。
- `APP_CONFIG_PATH`、`PIC_GALLERY_CONFIG` 作为 API/worker 默认启动路径选择器。
- DevOps 包按 `APP_ENV` 自动复制 YAML。
- Docker Compose 生产 `build:`。
- 分裂的 service install/uninstall 脚本入口。

## 13. 兼容性设计

1. 新老版本服务端并存：迁移期保留 YAML loader fallback，老版本仍可读取 YAML；新版本优先 env/DB。滚动升级时不要立即删除 YAML 文件。
2. 数据库变更兼容：`system_configs`、`secure_configs` 已存在；新增配置项以代码默认定义为主，不要求一次性写满 DB。
3. 新版服务端兼容老客户端：配置改造不改变用户端/管理端核心业务 API；新增后台页面独立上线。
4. 新版客户端兼容老服务端：管理后台新增的配置页要根据接口 404/feature flag 隐藏或提示版本不支持。
5. 客户端本地持久化：不涉及客户端本地存储格式。
6. 策略/配置向前兼容：配置 JSON 必须有 schema version；resolver 遇到未知字段忽略，遇到缺失字段补默认。
7. 定制化兼容：支持 `PIC_GALLERY_IMAGE_REGISTRY`、自定义 `.env` 路径、自定义 service name；不再通过维护多份 YAML 做定制化。

## 14. 稳定性与监控

### 14.1 关键指标

- 配置读取失败次数：`pic_gallery_config_resolve_errors_total`
- 配置缓存命中率：`pic_gallery_config_cache_hit_ratio`
- 配置快照版本：`pic_gallery_config_snapshot_version`
- secret 解密失败次数：`pic_gallery_secure_config_decrypt_errors_total`
- worker 配置刷新延迟：`pic_gallery_worker_config_refresh_lag_seconds`
- storage/provider test 失败次数：`pic_gallery_config_test_failures_total`

### 14.2 告警建议

| 指标 | 阈值 | 级别 |
|---|---:|---|
| secure config 解密失败 | 5 分钟内 > 0 | P0 |
| API 配置 resolver 连续失败 | 5 分钟内 > 10 | P1 |
| worker 配置刷新延迟 | > 120s | P2 |
| DB 配置 version 冲突异常升高 | 10 分钟内 > 20 | P2 |
| 生产环境使用 local-dev secret | 启动即失败 | P0 |

### 14.3 回滚

- 应用回滚：API/worker 镜像整体回滚到上一 tag。
- 配置回滚：`system_configs` 和 `secure_configs` 通过审计记录恢复上一版本。
- 部署回滚：Docker Compose 将 `PIC_GALLERY_IMAGE_TAG` 改回上一版本并 `pull && up -d`。
- env 回滚：`.env` 由运维自行备份，准备脚本不覆盖已有 `.env`，除非显式 `--force`。

## 15. 实施计划

### 阶段 1：配置盘点与 schema 固化

- 梳理 `config.Config` 中每个字段的归属：env、DB、默认值、废弃。
- 为 `system_configs` 每个 tab 增加 schema/校验函数。
- 补齐 storage/provider secure config 的领域模型。

验收：

- 输出完整配置映射表。
- 单测覆盖默认值、env、DB 覆盖优先级。

### 阶段 2：env loader

- 新增 `LoadEnv`。
- 支持 `.env` 文件解析。
- 生产必填校验。
- 保留 YAML 兼容 fallback 并打印 warning。

验收：

- `go test ./internal/config`
- 旧 YAML 测试仍通过或迁移为兼容测试。

### 阶段 3：业务配置 resolver

- Billing/generation/routing/providers/storage/worker runtime 改为 resolver。
- API 与 worker 启动时注入 resolver。
- 配置缓存与失效机制。

验收：

- 修改后台配置不重启即可影响新请求或新任务。
- 旧 env/default 在 DB 无配置时可兜底。

### 阶段 4：后台配置页补齐

- 增加供应商配置页。
- 增加对象存储配置页。
- 增加有效配置诊断页。
- 完善已有 SMTP、payments、billing 页面入口。

验收：

- secret 字段 write-only。
- 测试连接不泄露密钥。

### 阶段 5：本地模式脚本

- 实现 `tools/pgctl` 或等价跨平台脚本。
- 覆盖 build/run/install/start/stop/restart/status/logs。
- 替换 Makefile 中 service 入口。

验收：

- macOS、Linux、Windows 至少完成 dry-run 或 CI 验证。

### 阶段 6：Docker 模式与镜像发布

- 生产 Compose 删除 `build:`。
- 新增镜像构建/推送脚本。
- 新增 Docker 部署准备脚本。
- README/runbook 更新。

验收：

- `scripts/docker/images.sh build --tag test`
- `scripts/docker/images.sh push --tag test`
- `docker compose pull && docker compose up -d`
- API `/readyz` 通过。

### 阶段 7：清理旧入口

- 删除或移动 `configs/config.*.yaml`。
- 删除 DevOps 自动复制 YAML。
- 清理 README 中 `config.yaml` 部署说明。
- 保留迁移文档。

## 16. 测试策略

### 16.1 单元测试

- env loader：必填校验、默认值、类型转换、duration/list/map 解析。
- config resolver：DB > env > default 优先级。
- secure config：加密、解密、fingerprint、保留旧 secret、清空 secret。
- storage/provider resolver：配置格式校验。

### 16.2 集成测试

- 启动 API，使用 env 连接测试 DB/Redis。
- 写入 billing 配置后，新建任务计费使用新值。
- 写入 SMTP 配置后，验证码发送链路使用 DB 配置。
- 写入 provider API key 后，任务执行使用 DB 配置。

### 16.3 部署测试

- 本地模式：
  - `pgctl build`
  - `pgctl up --build`
  - `pgctl install --start`
  - `pgctl logs`
- Docker 模式：
  - `docker compose config`
  - `docker compose pull`
  - `docker compose up -d`
  - `./scripts/workflow/api-smoke.sh`

### 16.4 回归范围

- 登录/注册/验证码。
- API key 生成与 OpenAI-compatible 接口。
- 图片任务创建、worker 执行、文件落盘/对象存储读取。
- 计费扣费与充值套餐。
- 管理后台配置、收银台、安全配置页面。

## 17. 风险评估

| 风险 | 概率 | 影响 | 应对 |
|---|---:|---:|---|
| 配置优先级变更导致生产行为变化 | 中 | 高 | 迁移期输出 effective config，保留 YAML fallback，一个版本后删除 |
| secret 主密钥丢失导致密文不可解 | 中 | 高 | `.env` 权限 0600，部署脚本生成并提示备份，监控解密失败 |
| worker 运行中配置切换造成任务不一致 | 中 | 中 | 任务开始时固定 snapshot，新任务使用新配置 |
| storage 动态切换影响历史文件访问 | 中 | 高 | 区分 active storage 与 historical storage，切换前必须 test，必要时只允许新增写入目标 |
| Docker 镜像 tag 管理混乱 | 中 | 中 | release 脚本强制版本 tag，latest 只由 release 命令更新 |

## 18. 待人工确认项

1. 镜像仓库命名空间：阿里云私有仓库 namespace、DockerHub org 分别是什么？
2. 是否需要多架构镜像：`linux/amd64` 之外是否需要 `linux/arm64`？
3. Windows 本地服务管理是否必须使用原生 Windows Service，还是允许以任务计划程序/前台进程方式过渡？
4. 对象存储/BFSS 是否需要支持历史文件多 backend 读取？如果生产已经有存量 BFSS 文件，必须确认迁移策略。
5. 支付 public 证书、公钥类字段哪些必须允许 env 配置，哪些可以完全后台配置？
6. `AUTH_ACCESS_TOKEN_SECRET` 等安全主密钥是否真的允许 DB 覆盖？如果允许，需要确认密钥轮换和缓存失效策略。
7. 管理后台是否已有“系统设置/安全设置/供应商设置”的最终信息架构设计？若没有，需要补一个页面结构方案。
8. 是否保留 DevOps 二进制包模式？用户目标中“最终变成两个：本地模式和 Docker模式”，本方案按移除独立 DevOps 包处理，需要确认。
9. 生产数据库、Redis 是否总是由 Compose 托管？如果支持外部托管，需要 `.env.example` 同时覆盖外部连接样例。
10. 是否需要提供从旧 `config.yaml` 自动迁移到 DB 的命令，还是允许人工重新在后台配置？

## 19. 自检

- [x] 覆盖配置文件、部署脚本、Docker Compose、镜像发布、本地服务管理。
- [x] 明确配置优先级：数据库配置 > 环境变量配置 > 代码默认值。
- [x] 明确 `config.yaml` 废弃路径与迁移期兼容。
- [x] 明确 SMTP、billing、cashier、provider、BFSS/storage 的新归属。
- [x] 对照 sub2api 提取可复用部署体验：镜像拉取部署、准备脚本生成 secret、业务配置后台化。
- [x] 列出待补充资料。
