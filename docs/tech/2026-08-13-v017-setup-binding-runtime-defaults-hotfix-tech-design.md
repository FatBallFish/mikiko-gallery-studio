# v0.0.17 Setup Binding 运行时默认值兼容热修技术方案

> 日期：2026-08-13
> 状态：已确认
> 需求来源：`docs/prd/2026-08-13-v017-setup-binding-runtime-defaults-hotfix-requirements.md`
> 实施计划：`docs/plans/2026-08-13-v017-setup-binding-runtime-defaults-hotfix.md`

## 1. 生产证据与根因

v0.0.16 目标 migrator 已把线上数据库从 schema 4 迁移到 5，并把
`installations.app_version` 更新为 v0.0.16，说明 `public_id` 兼容修复已生效。升级随后
在 `ReconcileLegacyCompletedBinding` 返回摘要不匹配，尚未滚动服务；mgsctl 已恢复
runtime 中的 v0.0.12 发布身份，旧容器持续健康。

代码差异显示 v0.0.12 runtime schema 有 44 个字段，v0.0.16 有 58 个字段，唯一新增
集合正好是 14 个 Worker/媒体字段。`CurrentRuntimeSchemaVersion` 两个版本均为 1，
所以旧 `CommitProof.RuntimeSchemaVersion` 不能区分字段集合。渲染目标 runtime 时这些
字段被补为：

```text
WORKER_ROLES=image,video,media,cleanup
WORKER_MAX_CONCURRENT_TASKS=4
WORKER_IMAGE_CONCURRENCY=4
WORKER_VIDEO_CONCURRENCY=2
WORKER_MEDIA_CONCURRENCY=2
WORKER_CLEANUP_CONCURRENCY=1
MEDIA_FFMPEG_PATH=ffmpeg
MEDIA_FFPROBE_PATH=ffprobe
MEDIA_TEMP_DIR=./data/tmp
MEDIA_TEMP_DISK_PAUSE_PERCENT=75
MEDIA_TEMP_DISK_CRITICAL_PERCENT=90
WORKER_METRICS_ADDR=127.0.0.1:9091
VIDEO_ARTIFACT_ALLOW_LOOPBACK=false
VIDEO_ARTIFACT_TEST_CA_FILE=
```

Setup 摘要覆盖排序后的全部 runtime 键值（canonical 仅排除四个发布身份字段）。新增
默认字段改变了 HMAC payload，因此 v0.0.12 摘要既不匹配当前 canonical，也不匹配
现有 pre-documentation 候选。

## 2. 兼容模型

在 `internal/setup/legacy_binding_reconcile.go` 增加一个固定历史 profile：

- profile 名义上表示 v0.0.12 运行时字段集合；
- 仅省略上述 14 个常量键；
- 只有当前 bootstrap 中每个键都存在且值精确等于固定默认值时才启用；
- 为 profile 同时计算 canonical（排除发布身份）和 legacy（包含升级前发布身份）候选；
- 与现有 current、pre-documentation 候选合并后继续使用常量时间比较。

不能从当前 schema 动态读取默认值，因为未来默认值变更会无意扩大历史信任范围。
兼容 profile 必须把本次审计过的键和值固定在 Setup 包中，使安全边界可评审、可测试。

## 3. 一致性与幂等

保持现有分类规则：

1. 本地 proof 和数据库 binding 必须分别匹配某个受信任候选。
2. 两侧都不是当前 canonical 时，摘要必须完全相同，防止把不同历史状态拼接为成功。
3. 数据库摘要使用 operation、installation、revision 和 expected digest 做 CAS 更新。
4. 本地状态只允许 completed commit 在身份不变时更新摘要。
5. 本地更新失败时把数据库恢复到读取到的精确旧摘要。
6. 一侧已 canonical 时只修复另一侧；两侧已 canonical 时不写入。

数据库 schema 已先迁移到 5 不构成特殊分支。`db.Migrate` 本身幂等，重试时继续执行
同一目标迁移和 binding reconciliation，成功后才滚动服务。

## 4. 测试设计

先增加失败测试，构造当前 runtime 值，删除 14 个字段后计算 v0.0.12 历史摘要：

- canonical 历史摘要成功迁移；
- release-field legacy 历史摘要成功迁移；
- 每个字段逐一改成非默认值时拒绝且零写入；
- 从历史 profile 再多删除一个非白名单字段时拒绝；
- 当前 canonical、pre-documentation、partial migration、回滚测试继续通过。

focused 验证后运行仓库全量 verify、隔离 API smoke、committed review gate 和 ship guard。

## 5. 发布与线上恢复

通过独立分支和 PR 合入 main，在合并提交创建 annotated `v0.0.17` tag。等待 Release、
manifest、所有镜像与 `promote-latest` 成功后再重试生产升级。

生产已有备份：

```text
/var/backups/mikiko-gallery-studio/pre-v0.0.16-20260813T035938Z.dump
SHA256 59d958f08e94174f62ac3db59450e26aa4fd84b3d111691141e27df146ceaba7
```

重试前再次验证备份 SHA256 和 `pg_restore --list`，确认 v0.0.12 容器健康。使用
`mgsctl self-update --version v0.0.17 --yes` 后显式执行 v0.0.17 应用升级。不得手工改
binding 或回退 schema 5。成功后核对镜像、HTTP 健康、mgsctl doctor、安装版本以及
三条历史账号的 `public_id` 约束。

