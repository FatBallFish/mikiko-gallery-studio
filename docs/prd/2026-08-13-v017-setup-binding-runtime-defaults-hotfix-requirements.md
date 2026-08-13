# v0.0.17 Setup Binding 运行时默认值兼容热修需求

> 日期：2026-08-13
> 状态：已确认
> 线上基线：应用 v0.0.12，mgsctl v0.0.16
> 目标版本：v0.0.17

## 背景

线上从应用 v0.0.12 升级 v0.0.16 时，数据库 schema 已成功迁移到版本 5，
`model_accounts.public_id` 也已完成历史数据回填，但升级在服务滚动前被 Setup binding
校验拒绝：`setup binding does not match the requested commit`。升级器恢复了 v0.0.12
runtime 和部署清单，所有旧容器继续健康运行。

v0.0.12 到 v0.0.16 新增了 14 个 Worker/媒体运行时字段，但运行时 schema 版本仍为
1。升级渲染器会把这些字段及默认值写入目标 runtime；v0.0.12 完成 Setup 时生成的
摘要不包含这些字段。现有兼容器只支持更早的两个开发文档字段，因此无法验证这类
合法历史摘要。

## 目标

1. 支持已完成 Setup 的 v0.0.12 安装通过标准 `mgsctl upgrade` 升级到下一补丁版本。
2. 仅兼容以下 14 个新增字段，且所有字段必须等于受信任渲染器默认值：
   `WORKER_ROLES`、`WORKER_MAX_CONCURRENT_TASKS`、`WORKER_IMAGE_CONCURRENCY`、
   `WORKER_VIDEO_CONCURRENCY`、`WORKER_MEDIA_CONCURRENCY`、
   `WORKER_CLEANUP_CONCURRENCY`、`MEDIA_FFMPEG_PATH`、`MEDIA_FFPROBE_PATH`、
   `MEDIA_TEMP_DIR`、`MEDIA_TEMP_DISK_PAUSE_PERCENT`、
   `MEDIA_TEMP_DISK_CRITICAL_PERCENT`、`WORKER_METRICS_ADDR`、
   `VIDEO_ARTIFACT_ALLOW_LOOPBACK`、`VIDEO_ARTIFACT_TEST_CA_FILE`。
3. 数据库 binding 与本地 `install-state.json` 必须继续满足 installation、operation、
   config revision、runtime schema version 和摘要一致性约束。
4. 验证成功后通过现有 compare-and-swap 与回滚机制把两处摘要统一为当前 canonical
   摘要；重复升级必须幂等。
5. 发布不可覆盖 v0.0.16，修复使用新补丁版本 v0.0.17。
6. 线上已有数据库 schema 5 状态必须可安全重试，不执行手工摘要或 schema SQL。

## 验收条件

- 用 v0.0.12 字段集合计算的 canonical 和 release-field legacy 摘要均可被识别并迁移。
- 任一白名单字段不等于指定默认值时，兼容候选不可启用且不得写入数据库或本地状态。
- 任意非白名单字段、摘要损坏、数据库与本地历史摘要不一致继续 fail-closed。
- 当前 canonical、既有 pre-documentation 兼容、partial reconciliation 和回滚行为不回归。
- focused Go tests、全量 verify、API smoke、committed review gate 和发布 Action 全部通过。
- 线上 v0.0.12 服务在重试前保持健康，既有 PostgreSQL 自定义格式备份可读且 SHA256
  已记录。
- 线上升级后所有应用容器运行 v0.0.17，健康检查通过；`installations` 记录
  v0.0.17/schema 5；历史 `model_accounts.public_id` 非空且唯一。

## 非目标

- 不接受任意未知字段的省略，不根据数据库摘要猜测字段集合。
- 不修改运行时 schema 版本或重写已有 runtime 格式。
- 不手工修改生产 `setup_request_digest`、`install-state.json` 或数据库 schema。
- 不覆盖、移动或重新发布 v0.0.16 tag。

