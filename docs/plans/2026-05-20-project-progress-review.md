# Pic Gallery Progress Review (2026-05-20)

## 1. 复盘范围

本次复盘基于当前仓库实际代码状态，已将 Gemini 对 `web/user`、`web/admin` 的最新视觉与布局调整纳入统计，不再沿用 `README.md` 中早期“仅完成 T01-T04”的旧口径。

本文件用于回答三个问题：
- 当前项目到底已经做到哪里。
- 与 `docs/plans/2026-05-19-pic-gallery-implementation-plan.md` 的原始任务拆解相比，哪些已经落地、哪些只是壳子、哪些还没开始。
- 接下来最应该做什么，才能尽量减少返工和依赖阻塞。

> 结论先行：后端基础能力已经明显超过原 README 的描述；前端视觉主题与页面壳已经前进了一大步；真正还卡在主链路上的，是异步任务集群化、管理后台后端能力补齐、开放接口闭环，以及前端从 Demo 壳切到真实 API 集成。

## 2. 当前验证结果

2026-05-20 已完成的本地验证：
- `go test ./...` 通过
- `npm --prefix web/user run typecheck` 通过
- `npm --prefix web/admin run typecheck` 通过
- `npm --prefix web/user run build` 通过
- `npm --prefix web/admin run build` 通过

说明：当前工作区至少满足“后端单测通过 + 双前端可构建”的基础健康要求，可以继续向业务闭环推进。

## 3. 与原开发计划的逐项对照

状态口径：`已完成` / `部分完成` / `未开始`

### Track A：基础设施与公共底座（T01-T06）

- `T01` 已完成：仓库骨架、Go 入口、OpenAPI 目录、双前端目录、脚本和配置模板均已存在，关键文件见 `cmd/api/main.go`、`api/openapi/openapi.yaml`、`web/user/package.json`、`web/admin/package.json`、`Makefile`。
- `T02` 已完成：开发态依赖编排已落地，见 `deployments/docker-compose/docker-compose.dev.yml`、`scripts/dev/up.sh`、`scripts/dev/down.sh`；本地 PostgreSQL / Redis / MinIO / Mailpit 已纳入脚本化启动。
- `T03` 已完成：配置加载、配置模型、示例配置已落地，见 `internal/config/config.go`、`internal/config/load.go`、`configs/config.dev.yaml`、`configs/config.example.yaml`。
- `T04` 已完成：错误码、统一响应封装、中间件基线已落地，见 `pkg/errs/codes.go`、`pkg/httpx/json.go`、`internal/http/middleware/recovery.go`、`internal/http/middleware/requestid.go`。
- `T05` 已完成：OpenAPI 契约基线已明显展开，`api/openapi/openapi.yaml` 已覆盖健康检查、认证、Profile、计费估算、参考图、任务、历史、管理配置、OpenAI compat 等路径。
- `T06` 已完成：可观测性基线已落地，见 `internal/app/observability/logger.go`、`internal/app/observability/metrics.go`、`internal/http/handlers/metrics.go`、`internal/http/router/router.go` 中的 `/metrics` 与 `pprof` 路由。

### Track B：核心后端领域（T07-T15）

- `T07` 已完成：Ent Schema 与生成代码已较完整，见 `internal/repository/ent/schema/*.go`、`internal/repository/ent/*.go`，覆盖用户、分组、API Key、参考图、任务、结果、积分流水、动态配置、错误策略等核心实体。
- `T08` 部分完成：用户侧邮箱验证码登录、Access Token / Refresh Token、Refresh 轮换与重放阻断已实现，见 `internal/service/auth/service.go`、`internal/repository/entstore/auth_store.go`；但验证码发送仍是 mock 语义，管理员独立登录体系和完整前端接入尚未完成。
- `T09` 部分完成：计费公式、分辨率桶解析、用户倍率、图生图参考图数量参与估算已落地，见 `internal/domain/billing/calculator.go`、`internal/service/billing/service.go`；但余额查询、流水查询、实际扣减与结算闭环仍未完全落到 API 与任务完成链路。
- `T10` 部分完成：参考图上传、哈希去重、资产读取、大小校验和本地存储基线已具备，见 `internal/service/assets/service.go`、`internal/repository/entstore/assets_store.go`；但更完整的对象存储策略、预签名上传、生命周期治理还没有补全。
- `T11` 已完成：模型能力矩阵、质量解析、抽象模型到 Provider 路由选择已形成基线，见 `internal/domain/modelhub/resolver.go`、`internal/service/capabilities/service.go`。
- `T12` 已完成：OpenAI / OpenRouter Provider 抽象和测试已落地，见 `internal/provider/contracts.go`、`internal/provider/openai/client.go`、`internal/provider/openrouter/client.go`、对应 `*_test.go`。
- `T13` 部分完成：同步执行式任务服务、任务持久化、Provider 尝试与结果写回已具备，见 `internal/service/imagetask/service.go`、`internal/repository/entstore/imagetask_store.go`；但真正的异步队列、Worker lease、续约、补偿、失败恢复、集群抢占还没有完成，`internal/worker/` 仍基本为空壳。
- `T14` 部分完成：OpenAI compat 的 `generate` / `edit` / `models` 已接入，见 `internal/http/router/router.go`、`internal/http/handlers/api.go`、`internal/service/compat/service.go`；但原计划中的 native Open API 仍未完全实现，且 OpenAPI 文档中的部分业务路径仍存在“契约先行、handler 未补齐”的情况。
- `T15` 部分完成：后台配置中心 Tab 化读取与更新能力已具备，见 `internal/service/adminconfig/service.go`、`internal/http/handlers/api.go` 中 `config-tabs` 接口；但管理员认证、运营列表、价格管理 CRUD、任务审计聚合、公开图片审核后端能力仍明显不足。

### Track C：用户 Web（T16-T19）

- `T16` 部分完成：用户端工程底座、全局主题、侧边导航、TopBar、页面框架已重做，见 `web/user/src/App.tsx`、`web/user/src/layout/UserLayout.tsx`、`web/shared/base.css`、`web/shared/user-theme.css`；但尚未切到原计划中的 React Router / TanStack Query / 状态管理真实工程化结构。
- `T17` 部分完成：账户页与访问入口页已有界面壳，见 `web/user/src/pages.tsx`；但真实登录、Token 刷新、原页跳回、API Key 管理尚未接入后端。
- `T18` 部分完成：推荐页、图生图工作台、历史页等主要页面已有新的视觉 Demo，见 `web/user/src/pages.tsx`；但参考图上传、价格实时估算、任务提交、结果轮询、历史删除/公开等业务交互尚未真实打通。
- `T19` 部分完成：开发文档页与开放能力展示已有占位壳体，见 `web/user/src/pages.tsx` 中 `AccessPage` 相关内容；但还不是基于 OpenAPI 自动整理的正式文档门户，图片广场也仍停留在展示层。

### Track D：管理后台 Web（T20-T22）

- `T20` 部分完成：管理端工程底座、极简运维主题、侧边导航、TopBar、状态条已落地，见 `web/admin/src/App.tsx`、`web/admin/src/layout/AdminLayout.tsx`、`web/shared/admin-theme.css`。
- `T21` 部分完成：配置中心、路由策略、价格策略等页面已形成新的视觉壳和信息布局，见 `web/admin/src/pages.tsx`；但真实分页、草稿发布、回滚、权限控制和表单提交流程还未接后端。
- `T22` 部分完成：概览、审核、审计等运维页面已有页面壳，见 `web/admin/src/pages.tsx`；但任务监控、告警订阅、公开图审核、审计检索等核心运维链路尚未形成可操作页面。

### Track E：部署、验证与交付（T23-T26）

- `T23` 部分完成：开发态 Compose 与 Prometheus 基线已具备，见 `deployments/docker-compose/docker-compose.dev.yml`、`deployments/monitoring/prometheus.yml`；但原方案要求的集群部署资产、生产部署清单、API/Worker 拆分部署能力仍未完成。
- `T24` 部分完成：Go 单测和部分路由/服务测试已覆盖，见 `internal/http/router/*_test.go`、`internal/service/*_test.go`、`internal/domain/*_test.go`；但 Provider Mock 策略、端到端测试、前端 E2E、回归脚本仍未补齐。
- `T25` 未开始：尚未看到系统级联调记录、环境联调 checklist、回归报告或验收沉淀。
- `T26` 未开始：尚未看到上线 Runbook、告警值班说明、容量评估、回滚方案、交付文档全集。

## 4. 与原方案相比的几个关键偏差

### 4.1 后端实际进度明显快于 README 旧描述

原 README 仍停留在“只完成 T01-T04”的口径，但实际代码已经推进到：
- OpenAPI 契约完整雏形
- Ent 数据模型生成
- 认证刷新基线
- 计费估算基线
- OpenAI / OpenRouter Provider 层
- OpenAI compat 基线
- 管理配置中心基线

这意味着当前项目的主要问题已经不是“从零搭骨架”，而是“把已存在的模块收口成真正可联调、可部署的产品链路”。

### 4.2 前端视觉层推进快于业务接入层

Gemini 已经把用户端和管理端的整体页面观感、全局 theme、侧边导航、TopBar、布局骨架显著推进；但从工程角度看，当前仍主要是：
- Hash 页面切换
- 页面内静态数据
- 缺少真实鉴权态
- 缺少 API 数据源与错误反馈
- 缺少任务轮询、重试、空态、异常态的业务闭环

也就是说，前端现在更接近“高保真产品壳”，还不是“可交付产品前端”。

### 4.3 最大技术风险仍然与原计划判断一致

原方案把以下几项列为高风险优先事项，这个判断现在依然成立：
- `T09` 计费公式必须尽早稳定，否则前后端与结算都会返工。
- `T10 + T12` 图生图 / 参考图与双 Provider 差异是高风险点。
- `T13` 任务状态机、lease、补偿决定了集群部署可行性。
- `T14` OpenAI compat / Open API 的接口形态如果晚收口，会直接影响前端和第三方接入。

### 4.4 计划中的技术选型与当前前端结构存在偏差

原计划写的是 React Router + TanStack Query + Zustand；当前前端实际结构仍以单 App + hash 切页为主。这不是坏事，但意味着下一阶段要做真实业务接入时，必须先决定：
- 是直接在现有结构上补 API 接入，还是
- 先把路由、鉴权和数据层按正式架构补齐，再接业务。

如果不先统一这一点，后面接登录、历史、配置中心、审核台时会出现第二次重构。

### 4.5 后端 HTTP 框架也与原计划存在实际偏差

原计划的 Tech Stack 写的是 Gin，但当前代码实际使用的是标准库 `net/http` 路由与中间件组合，见 `internal/http/router/router.go`、`internal/http/handlers/api.go`。这本身不构成问题，但后续若继续按原计划拆任务或补文档，需要统一口径，避免再按 Gin 习惯去设计 middleware、context 和路由注册方式。

## 5. 建议的下一阶段执行顺序

建议不要再继续做“纯视觉层补丁”，而是转到真正的主链路闭环。推荐顺序如下：

### P1：先补后端主链路缺口

1. 完成 `T13`：把同步式任务执行升级为真实异步任务体系。
   - 引入任务入队、Worker 抢占、lease 续约、超时回收、失败补偿。
   - API 进程与 Worker 进程职责分离，为集群部署做准备。
2. 完成 `T14`：把契约与实现对齐。
   - 补齐 native Open API 的实际 handler。
   - 对齐 `api/openapi/openapi.yaml` 与 `internal/http/router/router.go` 的真实覆盖面。
   - 收口 OpenAI / OpenRouter 错误映射与重试策略。
3. 完成 `T15`：补齐管理后台后端核心能力。
   - 管理员登录态。
   - 配置草稿/发布/回滚链路。
   - 公开图片审核 API。
   - 任务审计与聚合指标接口。

### P2：再把现有前端壳切到真实产品流

4. 用户端先接 `T17-T19` 的最小闭环。
   - 登录与 silent refresh。
   - 参考图上传。
   - 实时价格估算。
   - 创建任务、轮询结果、历史列表、公开申请。
   - 开发文档页改为真正的 OpenAPI 门户。
5. 管理端再接 `T20-T22` 的最小闭环。
   - 配置读取与提交。
   - 审核列表与审核动作。
   - 任务/错误/审计查询。

### P3：最后补部署、测试、交付

6. 完成 `T23-T24`。
   - 产出集群部署清单或至少生产态部署骨架。
   - 增加 Provider Mock、集成测试、前端 E2E。
7. 完成 `T25-T26`。
   - 联调回归。
   - 上线手册、回滚方案、值班与告警文档。

## 6. 文档同步建议

本次复盘后，建议这样划分文档职责：
- `docs/plans/2026-05-19-pic-gallery-implementation-plan.md`：保留为“原始任务拆解与依赖关系”文档。
- `docs/plans/2026-05-20-project-progress-review.md`：作为当前事实进度来源。
- `README.md`：只保留高层状态概览，并指向本复盘文档，不再直接维护细粒度任务状态。

## 7. 一句话判断

如果现在要继续推进，最划算的下一步不是再修页面细节，而是先把 `T13 + T14 + T15` 补实；只有这样，Gemini 已经做好的前端外壳才能真正接上业务血肉，而不会在第二轮联调时整体返工。
