# S3 预签名媒体 URL 使用公网可达基址需求

> 日期：2026-09-11
> 状态：已确认（仓库所有者报告：Docker 部署后用户端历史生成图片全部无法展示）

## 背景

mgsctl 管理的 Docker full 部署使用内置 MinIO（对象存储 endpoint 为容器网络地址 `http://minio:9000`，未发布到宿主机）。图片生成结果的预览/下载 URL 通过 S3 SigV4 预签名生成，签名目标 host 即配置的 endpoint，浏览器拿到的 URL 形如 `http://minio:9000/app-assets/...?X-Amz-...`，在浏览器侧不可解析，导致用户端创作页、历史任务、画廊的所有图片全部 404/DNS 失败。

存储配置中已有 `public_base_url` 字段（env `STORAGE_PUBLIC_BASE_URL`、管理端存储配置、setup 均支持），当前仅被存储透传，S3 后端完全未使用。

## 目标

1. S3 后端在 `public_base_url` 配置为绝对 http(s) URL 时，预签名 GET URL 使用该基址的 scheme+host（含端口）作为签名与访问目标；数据面读写仍使用内网 endpoint。
2. 未配置或非法的 `public_base_url` 保持现状（按 endpoint 签名），不回归云上 S3/R2 直连场景。
3. mgsctl 管理的 Docker 部署将 MinIO API 端口发布到宿主机（端口可用 `MINIO_API_PORT` 覆盖），使单机部署的浏览器可通过 `http://127.0.0.1:<port>` 访问预签名对象。
4. 预签名 URL 仍由服务端按响应实时投影（现状），配置变更后历史图片同样恢复。

## 验收条件

- 设置 `public_base_url` 后，`/api/agent/image/v1/images/{id}/access` 与画廊/历史列表返回的 `preview_url`/`download_url` host 为 public_base_url 的 host，且该 URL 可直接 GET 返回图片字节（签名通过）。
- 未设置 `public_base_url` 时行为与现状一致（单测覆盖两种路径）。
- mgsctl doctor 存储探针、verify、API smoke 通过。
- 本地 Docker 部署实测：用户端历史图片恢复展示。

## 非目标

- 不改变 SigV4 签名算法与数据面（上传/下载/复制）行为。
- 不实现网关反向代理 MinIO 的路径改写方案（子路径会破坏 SigV4 canonical URI）。
- 不修改 ent 存储配置 schema（复用现有 public_base_url 字段）。
