# Runbook：MinIO → RustFS 迁移与下线

适用范围：使用 mgsctl Docker full 模式（或 custom + `rustfs`/legacy `minio` 组件）部署、
且对象存储由部署托管的实例。目标是以 RustFS（`rustfs/rustfs:1.0.1`）替换已下线的
MinIO 托管服务，保持应用侧 S3 配置语义不变，并安全下线 MinIO。

> 全新安装无需本手册：包含 `rustfs` 组件的版本直接以 RustFS 安装。

## 0. 前置事实与原理

- RustFS 与 MinIO 一样提供 S3 API（端口 9000）与控制台（端口 9001）。健康端点：
  `GET /health/live`、`GET /health/ready`（storage+IAM 就绪才返回 200）以及
  MinIO 兼容路径 `/minio/health/live`。
- 凭据通过 `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` 注入（访问键不要包含 `/`）。
- 托管部署中应用直接使用 RustFS 根凭据（`STORAGE_S3_ACCESS_KEY_ID` 等于
  `RUSTFS_ACCESS_KEY`）：RustFS 的用户管理走其自有 admin API/控制台，与 `mc admin`
  不兼容，官方镜像也不含客户端，因此不再像 MinIO 一样为应用创建受限子账号。
  服务仅暴露在 Docker 网络内；如需收紧，可之后在 RustFS 控制台创建子账号并在
  后台「存储配置」中改用该账号。
- 桶由一次性任务 `rustfs-init`（`amazon/aws-cli:2.37.12`）创建：
  `aws s3api head-bucket || create-bucket`，幂等。
- MinIO 与 RustFS 磁盘格式不互通；迁移 = 通过 S3 API 拷贝对象。
- 应用 DB 中的 `object_storage_configs` 不需要任何变更：运行时 env 的
  `STORAGE_S3_*` 只是首次引导种子，已完成的安装以 DB 配置为准。

## 1. 预检（只读，不影响业务）

```bash
cd /home/<runtime>
cp config/runtime.env config/runtime.env.bak-minio-$(date +%Y%m%d%H%M%S)
cp compose.yml compose.yml.bak-minio-$(date +%Y%m%d%H%M%S)

# 1.1 DB 侧：确认默认存储与 MinIO 配置的资产引用
docker exec <postgres容器> psql -U app -d app -P pager=off -c \
  "SELECT id, code, provider, is_default, endpoint, bucket FROM object_storage_configs ORDER BY created_at;"
docker exec <postgres容器> psql -U app -d app -P pager=off -c \
  "SELECT storage_config_id, COUNT(*) FROM media_assets GROUP BY 1;"

# 1.2 MinIO 桶对象数（存量主机本地缓存里有 minio/mc 镜像）
source config/runtime.env
docker run --rm --network <项目网络> \
  -e MC_HOST_m="http://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@minio:9000" \
  minio/mc:RELEASE.2025-04-16T18-13-26Z ls m/ --json
docker run --rm --network <项目网络> \
  -e MC_HOST_m="http://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@minio:9000" \
  minio/mc:RELEASE.2025-04-16T18-13-26Z ls m/$STORAGE_S3_BUCKET --recursive --summarize
```

判定：
- 若默认存储是外部 S3/R2 且 MinIO 桶为空（对象数 0）——本仓库线上即此情况——
  步骤 3 的数据迁移为空操作，直接按顺序执行即可。
- 若 MinIO 桶有对象，步骤 3 必须执行且对比对象数一致后才可切换。

## 2. 上线 RustFS（与 MinIO 并存）

升级到包含 RustFS 的版本（`mgsctl upgrade --image-tag vX.Y.Z`）会重写
`config/runtime.env`（`DEPLOYMENT_MODULES` 中 `minio`→`rustfs`、
`MINIO_ROOT_*` 移除、`RUSTFS_*` 生成、托管 `STORAGE_S3_*` 指向
`http://rustfs:9000` 并使用 RustFS 根凭据），但**不会重渲染 compose.yml**
（历史行为：upgrade 只更新 runtime.env 与 deployment.json）。

因此在升级后、`up` 之前，手工把 compose.yml 中的 `minio` / `minio-init`
服务段替换为当前模板版本（重新执行 `mgsctl install` 会渲染，或从仓库
`deployments/docker-compose/docker-compose.prod.yml` 取 `rustfs` / `rustfs-init`
两段 + `rustfs-data` / `rustfs-logs` 卷声明），并确保
`${MGSCTL_RUNTIME_DIR}/assets/rustfs-init.sh` 存在（新版本 mgsctl 升级后
`deployment.json` 的 files 清单包含它；缺失时从安装包 assets 目录复制）。

```bash
docker pull rustfs/rustfs:1.0.1
docker pull amazon/aws-cli:2.37.12
docker compose --project-directory . --env-file config/runtime.env \
  --project-name <项目名> up -d rustfs
# 等待就绪
docker compose ... exec rustfs curl -fsS http://127.0.0.1:9000/health/ready
# 建桶（幂等）
docker compose ... run --rm --no-deps rustfs-init
```

## 3. 数据迁移（桶为空则跳过）

```bash
docker run --rm --network <项目网络> \
  -e MC_HOST_src="http://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@minio:9000" \
  -e MC_HOST_dst="http://$RUSTFS_ACCESS_KEY:$RUSTFS_SECRET_KEY@rustfs:9000" \
  minio/mc:RELEASE.2025-04-16T18-13-26Z mirror --overwrite src/$STORAGE_S3_BUCKET dst/$STORAGE_S3_BUCKET
# 对比对象数
docker run --rm --network <项目网络> -e MC_HOST_src=... -e MC_HOST_dst=... minio/mc:... \
  sh -c 'mc ls src/BUCKET --recursive | wc -l; mc ls dst/BUCKET --recursive | wc -l'
```

（mc 客户端对任意 S3 服务可用；此处仅把它当作存量主机上已缓存的拷贝工具。）

## 4. 切换应用并重建 api/worker

```bash
cd /home/<runtime>
docker compose --project-directory . --env-file config/runtime.env \
  --project-name <项目名> up -d --force-recreate api worker
```

中断窗口 = api/worker 重建时间（秒级）。

## 5. 验证

```bash
# 5.1 容器与健康
docker compose ... ps
docker compose ... exec rustfs curl -fsS http://127.0.0.1:9000/health/ready
curl -fsS http://127.0.0.1:<API_PORT>/readyz

# 5.2 RustFS 桶可写读（用 runtime.env 里的根凭据）
source config/runtime.env
docker run --rm --network <项目网络> \
  -e AWS_ENDPOINT_URL=http://rustfs:9000 -e AWS_ACCESS_KEY_ID=$RUSTFS_ACCESS_KEY \
  -e AWS_SECRET_ACCESS_KEY=$RUSTFS_SECRET_KEY -e AWS_DEFAULT_REGION=us-east-1 \
  amazon/aws-cli:2.37.12 s3 cp /etc/hostname s3://$STORAGE_S3_BUCKET/migration-probe.txt
docker run --rm ... amazon/aws-cli:2.37.12 s3 rm s3://$STORAGE_S3_BUCKET/migration-probe.txt

# 5.3 外部默认存储（如 R2）回归：后台「存储配置」对默认配置执行「连接测试」，
#     或上传一张图片走完生成流程。

# 5.4 DB 配置快照与预检一致（无任何变更）
docker exec <postgres容器> psql -U app -d app -Atc \
  "SELECT id||':'||code||':'||is_default FROM object_storage_configs ORDER BY id;"
```

## 6. 下线 MinIO

```bash
docker compose ... stop minio minio-init
docker compose ... rm -f minio minio-init
# 备份数据卷后删除（备份保留至业务验证结束）
mkdir -p backups
docker run --rm -v <项目名>_minio-data:/data:ro -v "$PWD/backups:/out" alpine:3.21 \
  tar -C /data -czf /out/minio-data-$(date +%Y%m%d%H%M%S).tar.gz .
# 观察 1-2 个业务周期后：
docker volume rm <项目名>_minio-data
# 同场清理历史孤儿卷（旧项目遗留，确认无容器引用后）
docker volume ls --format "{{.Name}}" | grep minio-data
docker volume rm <旧项目名>_minio-data
```

最后从 compose.yml 中删除 minio 相关段落（若第 2 步尚未删）。

## 7. 回滚

在验证失败时：恢复 `config/runtime.env.bak-minio-*` 与 `compose.yml.bak-minio-*`，
`docker compose ... up -d --force-recreate`。MinIO 数据卷在删除前一直保留
（tar 备份永久可再恢复）。DB 未做任何变更，无需回滚。
