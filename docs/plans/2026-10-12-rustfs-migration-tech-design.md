# 技术设计：MinIO 对象存储替换为 RustFS

- 日期：2026-10-12
- 需求来源：`docs/prd/2026-10-12-rustfs-migration-requirements.md`
- 变更类型：部署编排（compose 模板 + mgsctl）为主，少量应用代码（校验白名单、provider 枚举）与脚本/文档

## 1. 选型与实测结论

选定 **RustFS 1.0.1**（2026-10-03 发布，GA 于 1.0.0 / 2026-09-16），镜像
`rustfs/rustfs:1.0.1`（Docker Hub 公开、amd64+arm64 多架构）。放弃继续使用 MinIO 的
原因是其公共镜像已彻底下线，全新安装无法拉取。

针对本项目关键路径的本地实测结论（容器 `rustfs/rustfs:1.0.1`，自定义根凭据）：

| 验证项 | 结果 |
| --- | --- |
| `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` 自定义凭据 | 生效：匿名 403、错误凭据 403、正确凭据 200 |
| 健康端点 `/health/live`、`/health/ready`、`/minio/health/live` | 全部 200（`ready` 在存储+IAM 就绪后返回 200） |
| 建桶（`amazon/aws-cli:2.37.12` `s3api create-bucket` / `head-bucket`） | 成功，可作为无头 init |
| 项目自研 SigV4 客户端（`internal/storage` S3Backend）Put/Get/GetBounded/Delete | 通过 |
| 流式 PutReader/OpenReader | 通过 |
| 服务端 Copy + ListObjectsV2（前缀剥离、分页） | 通过 |
| 预签名 GET（query SigV4、Content-Disposition、Cache-Control） | 通过 |
| 命名卷首次挂载属主 | 从镜像 `/data` 继承 UID 10001，无需权限辅助容器 |

已知取舍：

- RustFS 的用户/服务账户管理走其自有 admin API 与 Web 控制台（端口 9001），
  与 `mc admin` 协议不兼容，且官方镜像内不含 `rc` 客户端。
  因此**托管部署中应用直接使用 RustFS 根凭据**（单租户、服务仅暴露在 Docker
  网络内），不再像 MinIO 一样创建受限子账号；设计上保持
  `STORAGE_S3_ACCESS_KEY_ID == RUSTFS_ACCESS_KEY`。收紧权限可后续通过 RustFS
  控制台建子账号后由管理员在后台改配置完成，不在本次范围。
- RustFS 桶/对象元数据与 MinIO 磁盘格式不互通；迁移通过 S3 API 拷贝对象（本仓库
  场景 MinIO 桶为空，实际为空迁移，但 runbook 按通用流程编写）。

## 2. 命名与配置面

| MinIO（旧） | RustFS（新） |
| --- | --- |
| 组件/模块名 `minio` | `rustfs`（mgsctl 接受 legacy `minio` 输入并规范化为 `rustfs`） |
| 服务 `minio` / `minio-init` | 服务 `rustfs` / `rustfs-init` |
| 镜像 `minio/minio:RELEASE.2025-04-22T22-12-26Z` | `rustfs/rustfs:1.0.1`（正常 `pull`，移除 `pull_policy: never`） |
| 镜像 `minio/mc:RELEASE.2025-04-16T18-13-26Z` | `amazon/aws-cli:2.37.12`（仅 init 建桶） |
| 卷 `minio-data` | `rustfs-data`（+ 新增 `rustfs-logs`） |
| 环境变量 `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` / `MINIO_API_PORT` | `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` / `RUSTFS_API_PORT` |
| 应用引导 `STORAGE_S3_ENDPOINT=http://minio:9000` | `STORAGE_S3_ENDPOINT=http://rustfs:9000` |
| 应用凭据 = mc 创建的子账号 | 应用凭据 = RustFS 根凭据 |
| 健康检查 `/minio/health/live` | `/health/ready`（更强：存储+IAM 就绪才 200，正好门控 init） |

新增 `rustfs-init.sh`（替代 `minio-init.sh`，同样以 bind mount 挂入）：

```sh
aws s3api head-bucket --bucket "$STORAGE_S3_BUCKET" ||
  aws s3api create-bucket --bucket "$STORAGE_S3_BUCKET"
```

init 容器环境：`AWS_ENDPOINT_URL=http://rustfs:9000`、
`AWS_ACCESS_KEY_ID=$RUSTFS_ACCESS_KEY`、`AWS_SECRET_ACCESS_KEY=$RUSTFS_SECRET_KEY`、
`AWS_DEFAULT_REGION=$STORAGE_S3_REGION`。

RustFS 服务环境：`RUSTFS_ADDRESS=0.0.0.0:9000`、
`RUSTFS_CONSOLE_ADDRESS=0.0.0.0:9001`、`RUSTFS_CONSOLE_ENABLE=true` + 凭据两项。
镜像内已内置 `RUSTFS_VOLUMES=/data` 与 `/logs` 日志目录。9001 控制台端口沿袭
MinIO 惯例仅在 local/dev 模板发布，prod 模板只发布 API 端口（与现状一致）。

## 3. 场景 A：全新部署

mgsctl `install`（Docker full）流程不变，仅组件实现替换：

1. compose 模板渲染：`rustfs` 服务（profile `full`/`rustfs`）+ `rustfs-init` 一次性任务。
2. `populateManagedResources` 生成：`RUSTFS_ACCESS_KEY`（`rustfs-` 前缀 + 派生密钥，
   不含 `/`）、`RUSTFS_SECRET_KEY`、`STORAGE_DRIVER=s3`、
   `STORAGE_S3_ENDPOINT=http://rustfs:9000`、`STORAGE_S3_BUCKET=app-assets`、
   `STORAGE_S3_ACCESS_KEY_ID=<RUSTFS_ACCESS_KEY>`、
   `STORAGE_S3_SECRET_ACCESS_KEY=<RUSTFS_SECRET_KEY>`、`STORAGE_S3_FORCE_PATH_STYLE=true`。
3. 安装顺序：pull → up postgres/redis/rustfs（等 `health/ready`）→
   `run --rm --no-deps rustfs-init` 建桶 → up 应用服务。

存量安装 `upgrade` 兼容：加载 installation 时将模块/组件列表中的 legacy `minio`
规范化为 `rustfs`，runtime.env 重渲染时旧 `MINIO_*` 键自然消失（schema 键集变更），
compose.yml 不随 upgrade 重渲染（现状行为），存量主机按迁移 runbook 手工替换段落。

## 4. 场景 B：存量 MinIO → RustFS 迁移 runbook

（完整版随本次变更落盘 `docs/runbooks/minio-to-rustfs-migration.md`，摘要如下）

1. **预检**：确认 `object_storage_configs` 默认配置与 MinIO 配置的资产引用数；
   `mc ls --recursive` 统计 MinIO 桶对象数；记录卷名；备份 runtime.env 与 compose.yml。
2. **上线 RustFS**：拉取镜像，在 compose.yml 用 RustFS 服务段替换 MinIO 段
   （保留 minio 服务临时在册），`up -d rustfs` + `rustfs-init` 建同名桶。
3. **数据迁移**：`mc mirror`（存量主机已缓存 mc 镜像）MinIO 桶 → RustFS 桶，
   `--insecure`/空桶场景秒级完成；迁移后对象数对比。
4. **切换**：runtime.env 更新（`DEPLOYMENT_MODULES` minio→rustfs、
   `STORAGE_S3_*` 指向 rustfs、`MINIO_*`→`RUSTFS_*`），`up -d api worker` 重建。
5. **验证**：readyz/healthz；RustFS `/health/ready`；应用写读探针
   （后台存储配置 `:probe` 或 aws-cli 往返）；R2 默认存储读写回归；
   DB `object_storage_configs` 与迁移前快照完全一致。
6. **下线 MinIO**：`compose stop minio minio-init && compose rm -f minio minio-init`；
   `docker run --rm -v <minio-data>:/data -v $PWD/backups:/out alpine tar czf` 备份卷；
   确认业务正常后 `docker volume rm`（含历史孤儿卷）；移除 compose.yml 中 minio 段。
7. **回滚**：恢复备份的 compose.yml + runtime.env，`up -d`（MinIO 数据卷在删除前
   均有 tar 备份，可恢复）。

## 5. 代码改动清单

部署编排：

- `deployments/docker-compose/docker-compose.prod.yml`：minio/minio-init → rustfs/rustfs-init，
  卷、健康检查、端口变量、init bind。
- `deployments/docker-compose/rustfs-init.sh` 新增；`minio-init.sh` 删除。
- `deployments/assets.go`：嵌入 `rustfs-init.sh`（`RustFSInit()`）。
- `deployments/docker-compose/docker-compose.local.yml`、`deployments/devops/middleware-compose.yml`：
  同步替换（dev 默认凭据 `rustfsadmin/rustfsadmin`，发布 9001 控制台）。
- `deployments/docker-compose/.env.example`、`config/runtime.env.example`：MINIO_* → RUSTFS_*。

mgsctl：

- `internal/mgsctl/components.go`：`ComponentRustFS`，legacy `minio` 别名规范化，preset 与文案。
- `internal/mgsctl/runtime.go`：`populateManagedResources` 按第 3 节生成；部署文件输出
  `assets/rustfs-init.sh`。
- `internal/mgsctl/docker.go`：托管服务与 init spec 更名；profile 集合。
- `internal/mgsctl/upgrade.go`：加载后规范化 modules/components。
- `internal/mgsctl/install_summary.go`：端点文案 RustFS API/Console。
- `internal/config/runtime_schema.go`：`RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY`
  （requiredDockerFull、owner mgsctl）、`RUSTFS_API_PORT`。

应用（小改）：

- `internal/domain/storageconfig/types.go`：新增 `ProviderRustFS`（保留 `ProviderMinIO`
  兼容存量 DB 行）。
- `internal/service/storageconfig/service.go`：托管 HTTP endpoint 白名单
  （原 `isManagedMinIOEndpoint`）同时接受 `minio:9000`（存量）与 `rustfs:9000`。
- `api/openapi/components/schemas/admin.yaml` + `web/admin/src/pages/StorageConfigPage.tsx`：
  provider 枚举新增 `rustfs`（映射 driver=s3、force_path_style=true）。
- `internal/localbootstrap/run.go`：requiredModules `minio`→`rustfs`（与 local.yml 同步）。

脚本/测试：

- `internal/storage/rustfs_live_test.go`：env 门控 RustFS 实测（本次新增，CI 自动跳过）。
- `scripts/dev/up.sh`、`scripts/dev/migrate-unified-local.sh`：服务/卷名更新。
- `scripts/workflow/multimedia-acceptance.sh` + `internal/acceptance/multimedia_load_test.go`：
  MinIO 容器 → RustFS 容器 + aws-cli 建桶，环境变量更名 `MULTIMEDIA_S3_*`。
- `scripts/e2e/setup-docker-e2e.sh`、`cluster-docker-e2e.sh`、`run-docker-e2e.sh`、
  `docker-e2e.mjs`：同上替换。

文档：`README.md`、`README.zh-CN.md`、`docs/runbooks/backend-deployment.md`
（移除“必须预置 MinIO 镜像缓存”要求）、新增 `docs/runbooks/minio-to-rustfs-migration.md`。

## 6. 测试计划

- 单测：runtime.env schema 渲染含 RUSTFS_* 且不含 MINIO_*；模块规范化
  （`minio`→`rustfs`）；storageconfig 白名单接受 rustfs 主机、拒绝其他 HTTP endpoint；
  provider 规范化接受 rustfs。
- env 门控 live probe：对真实 RustFS 的全能力往返（AC3）。
- `./scripts/workflow/verify.sh`（go test/vet + 双前端 typecheck/build）。
- `./scripts/workflow/review-local.sh --scope committed` + gate。
- `./scripts/workflow/api-smoke.sh`（后端/部署变更必跑；本地栈不涉 MinIO，不受影响）。
- 本地 compose 冒烟：middleware profile 起 rustfs 验证健康检查与建桶脚本。

## 7. 线上执行计划（159.195.14.17）

1. 发布含本变更的新版本并 `mgsctl upgrade --image-tag vX.Y.Z`（upgrade 不重渲染
   compose.yml，线上 compose.yml 由 runbook 手工替换 RustFS 段）。
2. 按第 4 节 runbook 执行：预检（已确认 R2 默认 + MinIO 空桶 + bootstrap-s3 无引用）
   → RustFS 上线建桶 → mc mirror 空迁移 → runtime.env 切换 → api/worker 重建 →
   全量验证（readyz、R2 回归、DB 快照一致）→ MinIO 停用、卷 tar 备份后删除。
3. 全程不动 DB `object_storage_configs`（含 `bootstrap-s3` 历史行，其 endpoint
   指向已下线的 minio 主机但非默认且无资产引用，属惰性记录）。
