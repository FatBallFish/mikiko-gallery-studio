# v0.0.16 升级可靠性热修技术方案

> 日期：2026-08-13
> 状态：已确认
> 需求来源：`docs/prd/2026-08-13-v016-upgrade-reliability-hotfix-requirements.md`
> 实施计划：`docs/plans/2026-08-13-v016-upgrade-reliability-hotfix.md`

## 1. 故障结论

线上 v0.0.12 的 `model_accounts` 有三条历史数据且没有 `public_id`。v0.0.13
引入的 Ent 字段使用 `.Default(uuid.New)`，该默认值只在 Go Create builder 中执行，
不是 PostgreSQL 列默认值。Ent 自动迁移尝试直接增加最终的非空字段，历史行保持 NULL，
所以 PostgreSQL 在设置 `NOT NULL` 时返回 23502。

升级器在迁移成功前不会滚动服务，并会恢复旧 runtime 配置。因此本次失败没有切换
任何应用容器，线上仍安全运行 v0.0.12；失败 DDL 事务也回滚了新增列。

mgsctl 自升级的生产 HTTP client 使用两分钟全请求超时。这个超时覆盖连接、重定向、
响应头以及整个二进制 body，在慢速但持续有进展的链路上仍会取消请求。当前错误包装
还丢失下载阶段和底层原因，TUI/CLI 也没有进度反馈。

## 2. 数据库迁移设计

继续复用 `prepareLegacyDataSQL`，因为它在全局迁移 advisory lock 内、Ent
`Schema.Create` 之前运行，正是兼容旧形态到目标 DDL 之间的适配层。

对 `model_accounts` 执行以下幂等过程：

1. 表不存在时跳过。
2. 列不存在时增加可空 `uuid` 列。
3. 仅更新 `public_id IS NULL` 的行，为每行生成独立 UUID。
4. 不设置永久数据库默认值，不修改任何已有 UUID。
5. 由随后执行的 Ent migration 建立 `UNIQUE NOT NULL` 最终约束。

UUID 生成不能依赖 `pgcrypto` 或 `uuid-ossp` 扩展，避免给老部署新增数据库扩展要求。
使用行 ID、`random()` 和 `clock_timestamp()` 生成 MD5 十六进制并按 UUID 格式转换。
数据库最终唯一索引仍是权威保护；测试覆盖三行唯一性、部分回填及重复执行。

这种两阶段策略优于给 Ent 字段增加 SQL 默认值：`public_id` 是稳定的外部别名，不应
让数据库永久默认掩盖应用写入错误；兼容工作只针对历史行，新增行继续由 Go 显式生成。

## 3. 自升级下载设计

### 3.1 超时

取消 `http.Client.Timeout` 的整个请求截止时间，改用 Transport 的分阶段边界：

- TCP 连接超时 15 秒；
- TLS 握手超时 15 秒；
- 响应头超时 30 秒；
- Expect Continue 超时 1 秒；
- body 允许超过两分钟，只要传输仍继续并且调用上下文未取消。

外部 `context.Context` 始终拥有最高优先级，SIGINT/SIGTERM 会立即停止下载并删除
临时文件。资产仍受 128 MiB 上限约束，因此没有无限磁盘增长风险。

### 3.2 重试

checksum 和 binary 是两个独立阶段，各最多三次。仅以下情况重试：临时网络错误、
连接重置、EOF/UnexpectedEOF、HTTP 408/425/429/500/502/503/504。明确 4xx、内容
超限、checksum 格式错误、checksum 不匹配和上下文取消不重试。

二进制每次重试前必须 `Seek(0, 0)` 与 `Truncate(0)`，重新计算哈希。只有完整下载、
大小限制和 SHA-256 全部通过后才进入原有 chmod/sync/原子替换流程。

### 3.3 进度

`SelfUpdateDependencies` 注入进度回调，下载器报告阶段、attempt、当前字节、总字节、
耗时和是否完成。CLI 仅在 stdout/stderr 对应终端时渲染单行进度，格式包含百分比、
已下载/总大小和平均速度；未知 Content-Length 时省略百分比。非 TTY 保持现有简洁
输出，避免破坏自动化脚本。

错误包含 `checksum` 或 `binary` 阶段、最后 attempt 和经过净化的底层错误；错误中
不得出现 Release 重定向的签名 query。

## 4. 发布与线上验证

本热修使用单一分支 `codex/v016-migration-self-update`。完成全量 verify、API smoke、
committed review gate 后通过 PR 合入 main，在 main 合并提交创建新 tag `v0.0.16`，
不得改写 v0.0.15。

线上操作顺序：确认 v0.0.12 健康，使用容器内 `pg_dump -Fc` 输出到宿主专用备份目录，
验证 SHA-256 与 `pg_restore --list`；再更新 mgsctl、执行标准 upgrade。迁移失败时不
执行手工 SQL，确认旧容器继续健康并保留日志。成功后核对容器镜像、健康状态、三条
`public_id` 唯一非空值及 installation schema/version。

## 5. 风险控制

- 兼容 SQL 在 advisory lock 内执行，避免多个 migrator 竞争回填。
- 只修改 NULL，不会改变已经发给 Provider 的回调别名。
- 失败事务由 PostgreSQL 回滚；旧服务不依赖新列，可继续运行。
- 重试只覆盖纯下载，不重试文件替换，避免重复副作用。
- 进度回调不接触 URL，避免签名参数泄漏。
- 线上备份先于迁移，并在宿主机验证可读性。
