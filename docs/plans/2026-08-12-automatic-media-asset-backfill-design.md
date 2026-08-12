# 历史图片统一资产自动回填设计

日期：2026-08-12  
状态：已确认

## 1. 目标

将历史图片到 `media_assets` 的元数据回填纳入项目正常启动后的后台处理流程。正常部署和 `mgsctl upgrade` 不再要求发布人员执行额外数据迁移命令，同时保持服务可用、数据幂等和多实例安全。

## 2. 已确认语义

1. API 和 Worker 先正常就绪，历史图片在后台渐进回填。
2. 回填失败时服务保持可用，由 Worker 自动重试并输出可定位日志。
3. 回填不读取、下载、上传或复制对象存储正文，只复制数据库元数据。
4. 独立 `media-backfill` 命令继续保留，但仅用于 dry-run、verify 和故障诊断。

## 3. 方案对比

### 3.1 推荐：接入 Worker `cleanup` 角色

在 Worker 完成 schema、setup binding、存储身份等启动校验后，将 `MediaAssetBackfillProcessor` 加入 `cleanupRoleProcessor` 的公平轮转队列。每次轮转只处理一个批次；处理完成后进入低频复查。

优点：复用现有 Worker 的取消、错误隔离、轮询退避和角色扩容能力；不阻塞 readiness；不向 API 进程引入后台数据任务；默认 `WORKER_ROLES=image,video,media,cleanup` 可自动生效。

### 3.2 不采用：同步阻塞服务启动

部署语义直观，但历史数据量、数据库压力或单行脏数据会阻塞整套服务上线，无法满足已确认的“服务先可用”要求。

### 3.3 不采用：放入 `mgsctl upgrade` 或 schema migration

会把长时间、可重试的数据 reconciliation 与确定性的 schema 变更混在一起，延长升级窗口并破坏滚动升级。多节点升级时也难以清晰界定执行者和失败恢复责任。

## 4. 运行架构

`cleanup` 角色启动时创建 `MediaAssetBackfillProcessor`，默认批次为 100 行，并把它追加到现有对象删除、导出、过期 multipart 和媒体对账 processor 列表。`cleanupRoleProcessor` 的轮转规则保证持续有历史数据时每轮最多处理一批，不长期独占 cleanup 槽位。

现有 `runWorkerRoleSlots` 已将单次 processor 错误限制在当前迭代：记录错误，等待轮询间隔后继续，因此回填错误不会退出 Worker。context 取消会终止当前数据库操作并正常退出。

## 5. 幂等与并发

1. `migration_checkpoints` 保存 `(created_at,id)` 游标和完成状态。
2. 每批事务锁定 checkpoint；PostgreSQL 多 Worker 并发时只有一个实例推进该批次。
3. 回填资产复用历史图片 UUID，并以 `ON CONFLICT (id) DO NOTHING` 防止重复创建。
4. checkpoint 只在同一事务成功提交后推进；中断或失败不会跳过未提交行。
5. 完成态再次执行时检查是否存在遗漏源记录；存在则自动重开 checkpoint。

## 6. 完成后复查

不能让已完成的回填 processor 在每秒 cleanup 轮询中反复执行全表计数。增加自动 processor 包装层：

- 未完成时连续按公平轮转处理小批次；
- 某次返回空闲后，记录下次复查时间；
- 默认低频复查周期为 6 小时；
- 单批失败使用有上限的指数退避，避免持久脏数据造成高频数据库和日志压力；
- 新增遗漏记录时，现有底层 processor 自动重开 checkpoint 并继续处理。

复查周期和退避先作为内部常量，不增加新的运营配置，避免为一次性兼容任务扩展永久配置面。

## 7. 可观测性与故障处理

启动时记录自动回填已启用及批次大小。每个成功批次记录 processed/created/skipped/done 和 checkpoint；失败记录稳定错误码、连续失败次数与下次重试时间；完成时记录累计进度。正常空闲复查不重复刷日志。

若出现非 ID 唯一键冲突等数据问题，自动 runner 保持退避重试和告警，服务继续可用。运维人员可使用 `media-backfill --dry-run` 和 `--verify` 检查，并在修正冲突后由自动 runner 接续。

## 8. 测试范围

1. 自动 processor 首次启动处理一批，完成后进入低频复查。
2. 复查未到期时不访问底层 processor；到期后可发现新数据并恢复处理。
3. 错误不被吞掉，由 Worker 循环记录并重试；包装层应用有上限退避。
4. context 取消可终止循环。
5. `cleanup` 角色关闭时不构造、不运行自动回填。
6. 现有 checkpoint 恢复、重复执行、完成后重开和冲突保护测试继续通过。
7. 完整 Go 测试、vet、前端 typecheck/build 和隔离 API smoke 通过。

## 9. 发布与回滚

发布只部署新版本并正常启动 Worker，不执行额外回填步骤。升级期间统一资产列表继续通过兼容投影读取尚未回填的图片。

回滚不会删除 `media_assets` 或 checkpoint。旧版本仍可从历史表读取；再次升级后自动 runner 从持久化 checkpoint 继续。独立 CLI 不从发布包移除，以便诊断。
