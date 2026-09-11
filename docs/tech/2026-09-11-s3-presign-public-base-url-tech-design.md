# S3 预签名公网基址技术方案

> 日期：2026-09-11
> 状态：已确认
> 需求来源：`docs/prd/2026-09-11-s3-presign-public-base-url-requirements.md`

## 1. 根因

`internal/storage/backend.go` 的 `S3Backend` 只持有单一 `endpoint`。`requestTarget(key)` 同时服务两类用途：

- 数据面请求（GET/PUT/DELETE/COPY 对象）；
- `TemporaryGetURL` 的 SigV4 预签名（canonical request 的 host 与最终 URL 均取自该 endpoint）。

mgsctl Docker 部署的 endpoint 是 `http://minio:9000`（容器网络），浏览器不可达。`config.StorageConfig.PublicBaseURL` 已在 env → setup → DB 存储配置 → `StorageConfigFromResolved` 全链路透传，但 S3 后端未消费。

## 2. 设计

### 2.1 S3Backend 预签名基址

`NewS3Backend` 解析 `cfg.PublicBaseURL`：为空则不启用；为绝对 http(s) URL 且无 path/query/user/fragment（仅 scheme+host[+port]，允许尾部 `/`）时存为 `presignEndpoint`，否则返回配置错误（fail-closed，避免静默生成不可用 URL）。

`requestTarget(key)` 重构为 `requestTargetFor(endpoint, key)`；数据面调用不变，`TemporaryGetURL` 在 `presignEndpoint != nil` 时改用 `requestTargetFor(b.presignEndpoint, key)` —— 签名 host、canonical URI 与返回 URL 都基于公网基址，MinIO 校验签名时看到的 Host 与签名一致，校验通过。

### 2.2 部署模板

`deployments/docker-compose/docker-compose.prod.yml` 的 `minio` 服务新增端口发布 `"${MINIO_API_PORT:-9000}:9000"`（与 API/网关一致发布到所有接口；MinIO 仅接受带签名的匿名 GET）。`MINIO_API_PORT` 由 runtime.env 提供，缺省 9000。

单机部署在管理端"存储配置"为默认 s3 配置设置 `public_base_url = http://127.0.0.1:<MINIO_API_PORT>`；跨机器访问的部署应设为浏览器可达的 MinIO 公网地址。

### 2.3 兼容

- `public_base_url` 为空：与现状完全一致（云 S3/R2 直连不受影响）。
- 存储路由按配置指纹缓存后端，更新存储配置后自动重建。
- 图片 URL 均为响应期实时投影，历史记录无需迁移。

## 3. 测试

- Go 单测：设置 presign 基址后 URL host/端口来自基址且签名验证通过（对本地 MinIO 实例或签名重算验证）；非法基址报错；未设置时与现有测试一致。
- 本地部署端到端：更新默认存储配置 public_base_url → access 端点与画廊列表返回 127.0.0.1 URL → curl 拉取图片字节成功。
- `verify.sh`、隔离 API smoke、committed review gate。
