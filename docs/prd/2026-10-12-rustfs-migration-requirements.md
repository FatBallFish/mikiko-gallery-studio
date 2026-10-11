# 需求文档：MinIO 对象存储替换为 RustFS

- 日期：2026-10-12
- 状态：已确认（用户在任务指令中明确授权全链路执行：方案设计 → 开发测试 → 线上真实替换、数据迁移、MinIO 下线）
- 关联技术设计：`docs/plans/2026-10-12-rustfs-migration-tech-design.md`

## 背景

MinIO 已从公共镜像仓库全面下线（Docker Hub 仓库 404、quay.io 要求认证），当前仓库的
`minio` / `minio-init` 服务被迫使用 `pull_policy: never` 并依赖运维预置本地镜像缓存，
新机器无法完成全新安装。需要选型一个可持续拉取、S3 兼容的自托管对象存储替换 MinIO。
经调研与实测，选定 RustFS（Rust 实现、Apache-2.0、S3 兼容、Docker Hub 公开多架构镜像）。

线上环境（159.195.14.17）实际情况已核实：

- 生产默认存储是 Cloudflare R2（`object_storage_configs` 表 `cloudflare` 行，
  provider=r2、is_default=true、604 个 `media_assets` 全部挂在该配置）。
- MinIO 仅作为 Docker full 模式的托管组件部署：桶 `app-assets` 为空（0B）、
  `bootstrap-s3` 行（endpoint http://minio:9000）非默认且无任何资产引用。

## 需求

- R1 新部署场景：mgsctl 全新安装（Docker full 模式）使用 RustFS 替代 MinIO，
  所有镜像可从公共仓库直接拉取，无需运维预置镜像缓存。
- R2 迁移场景：已部署 MinIO 的存量安装可以迁移到 RustFS，提供覆盖
  预检、数据迁移、切换、验证、回滚、MinIO 下线的操作手册并可执行。
- R3 应用零改动兼容：应用侧 S3 客户端（自研 SigV4）对 RustFS 的
  读写、流式、服务端拷贝、列举、预签名能力全部可用，不修改 DB 中既有存储配置。
- R4 线上无缝替换：保持线上 R2（BFSS）配置与数据完全不动；线上 `bootstrap-s3`
  环境变量引导配置指向 RustFS；替换期间业务中断最小化（一次 api/worker 重启窗口）。
- R5 历史数据完整性：MinIO 桶内对象（若存在）完整迁移至 RustFS 同名桶，
  迁移前后有对象数/校验对比；线上已确认为空桶，仍需执行并记录对比结果。
- R6 MinIO 下线：替换验证通过后，停止并移除 MinIO 服务与镜像引用，
  备份 MinIO 数据卷后删除，清理遗留的孤儿卷。

## 验收标准

- AC1 `./scripts/workflow/verify.sh`、review gate、api-smoke 全部通过。
- AC2 全新安装路径（compose 模板 + mgsctl 生成的 runtime.env）不再引用
  minio/mc 镜像与 MINIO_* 变量；RustFS 服务健康检查通过、init 正确建桶。
- AC3 环境变量门控的 RustFS 实测（`internal/storage` live probe）覆盖
  Put/Get/Bounded/Streaming/Copy/ListObjects/Presigned 且全部通过。
- AC4 线上替换后：RustFS 容器健康、桶存在且对象数与迁移前 MinIO 一致、
  应用 readyz 正常、R2 默认存储读写不受影响、DB `object_storage_configs` 无任何变更。
- AC5 MinIO 容器/卷下线且留有备份；线上服务恢复正常运行。
