# pic-gallery

图片生成平台项目，定位于 API 中转服务与独立图像生成应用之间：对接 OpenAI、OpenRouter 等上游图像模型，并在其上封装用户体系、参考图资产、积分计费、模型路由、运营后台和开发文档页能力。

## 当前状态

- PRD：`docs/prd/pic-gallery-prd.md`
- 技术方案：`docs/tech/pic-gallery-tech-design.md`
- 原始开发计划：`docs/plans/2026-05-19-pic-gallery-implementation-plan.md`
- 最新进度复盘：`docs/plans/2026-05-20-project-progress-review.md`
- 前端视觉规范：`docs/design/frontend-design-spec.md`
- 前端视觉方向：`docs/design/frontend-visual-directions.md`
- 2026-05-20 代码现状：
  - 后端：`T01-T07` 基本完成，`T08-T15` 已有主链路基线，但异步任务集群、管理后台后端、开放接口闭环仍待补齐
  - 前端：用户端与管理端主题系统、布局框架和页面概览已重做完成，当前仍以高保真页面壳为主，真实鉴权、数据接入和文档门户尚未闭环
  - 验证：`go test ./...`、`npm --prefix web/user run typecheck`、`npm --prefix web/admin run typecheck`、`npm --prefix web/user run build`、`npm --prefix web/admin run build` 已通过

## 目录结构

```text
api/openapi/              # OpenAPI 契约与对外接口文档源文件
cmd/api/                  # 后端服务启动入口
configs/                  # 本地与部署配置模板
deployments/              # Docker Compose、监控与后续部署资产
docs/                     # 需求、方案、评审与实施计划文档
internal/app/             # 应用装配与启动逻辑
internal/config/          # 配置加载与配置模型
internal/domain/          # 领域模型与核心规则
internal/http/            # HTTP handler、middleware、router
internal/provider/        # OpenAI / OpenRouter 等上游接入层
internal/repository/      # DB、缓存、对象存储等仓储抽象
internal/service/         # 应用服务编排层
internal/worker/          # 异步任务执行与补偿任务
pkg/                      # 公共工具包
scripts/                  # 开发、测试、部署脚本
web/user/                 # 用户 Web（React + Vite）
web/admin/                # 管理后台 Web（React + Vite）
```

## 快速开始

### 1. 准备环境变量

```bash
cp .env.example .env
```

### 2. 启动本地依赖

```bash
make compose-up
```

默认会拉起：

- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`
- MinIO API：`localhost:9000`
- MinIO Console：`localhost:9001`
- Mailpit：`localhost:8025`

### 3. 启动后端服务

```bash
make dev
```

当前已具备的代表性接口包括：

- `GET /healthz`：存活检查
- `GET /readyz`：就绪检查
- `GET /metrics`：Prometheus 指标
- `POST /api/agent/auth/v1/login/email-code`：邮箱验证码登录
- `POST /api/agent/auth/v1/session/refresh`：刷新 Access Token / Refresh Token
- `POST /api/agent/billing/v1/estimate`：积分预估
- `POST /api/agent/image/v1/reference-assets`：上传参考图
- `POST /api/agent/image/v1/tasks`：提交图片任务
- `POST /v1/images/generations`：OpenAI 兼容生图接口
- `POST /v1/images/edits`：OpenAI 兼容编辑接口

### 4. 安装并启动前端

```bash
make user-web-install
make admin-web-install
make user-web-dev
make admin-web-dev
```

## 当前统一脚本入口

- `make dev`：启动 Go API 服务
- `make test`：运行 Go 单元测试
- `make lint`：运行 Go 静态检查
- `make openapi`：检查 OpenAPI 源文件是否存在
- `make compose-up`：启动本地依赖容器
- `make compose-down`：停止本地依赖容器
- `make user-web-install`：安装用户端依赖
- `make admin-web-install`：安装管理后台依赖
- `make user-web-dev`：启动用户 Web
- `make admin-web-dev`：启动管理后台 Web

## 当前本地配置文件

- 开发配置模板：`configs/config.example.yaml`
- 本地开发配置：`configs/config.dev.yaml`
- Compose 环境变量模板：`deployments/docker-compose/.env.example`

## 接下来最优先的工作

详细顺序见 `docs/plans/2026-05-20-project-progress-review.md`，当前建议优先级为：

1. `T13`：完成真实异步任务、Worker lease、补偿与集群执行闭环
2. `T14-T15`：补齐 native Open API、管理后台后端能力、错误策略与审核链路
3. `T17-T22`：把用户端与管理端现有页面壳切到真实鉴权和 API 数据流
4. `T23-T26`：补部署资产、E2E/联调回归、上线与交付文档

## 参考资料

- NewAPI：`/Users/fatballfish/Documents/Projects/GoProjects/Public/new-api`
- Sub2Api：`/Users/fatballfish/Documents/Projects/GoProjects/Public/sub2api`
- gpt-image-playground：`/Users/fatballfish/Documents/Projects/VueProjects/gpt_image_playground`
