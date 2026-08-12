# 历史图片统一资产自动回填实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让启用 `cleanup` 角色的 Worker 在启动后自动、非阻塞、幂等地回填历史图片元数据，正常升级不再需要人工迁移命令。

**Architecture:** 复用现有 `db.MediaAssetBackfillProcessor` 的 checkpoint、事务锁和同 ID 幂等能力，将带完成复查与错误退避的自动 processor 接入 `cleanupRoleProcessor` 公平轮转。API/Worker readiness 不等待回填完成，独立 CLI 仅保留诊断用途。

**Tech Stack:** Go 1.24、Ent、PostgreSQL、现有 Worker role loop、Go testing。

---

### Task 1：固化需求与技术约束

**Files:**
- Modify: `docs/prd/2026-08-12-multimedia-creation-phase1-prd.md`
- Modify: `docs/tech/2026-08-12-multimedia-creation-phase1-tech-design.md`
- Modify: `docs/ops/multimedia-operations.md`
- Create: `docs/plans/2026-08-12-automatic-media-asset-backfill-design.md`

1. 写明服务先就绪、后台渐进回填和失败不影响可用性。
2. 写明 `cleanup` 角色、多 Worker 行锁、同 ID 幂等、完成后低频复查。
3. 移除“发布后人工执行回填”的要求，保留 CLI 诊断语义。
4. 检查所有文档不再同时存在互相冲突的发布步骤。

### Task 2：为自动 processor 编写 RED 测试

**Files:**
- Create: `internal/app/media_asset_auto_backfill_test.go`

1. 定义可注入时钟和底层 `processOnce` probe。
2. 测试有工作时立即继续参与公平轮转。
3. 测试返回 idle 后，在复查周期到期前不再调用底层 processor。
4. 测试复查到期后重新调用，并能发现新工作。
5. 测试错误后的指数退避及最大退避上限。
6. 运行 `go test ./internal/app -run 'TestAutomaticMediaAssetBackfill' -count=1`，确认因实现缺失而失败。

### Task 3：实现自动 processor 并接入 cleanup 角色

**Files:**
- Create: `internal/app/media_asset_auto_backfill.go`
- Modify: `internal/app/worker.go`
- Modify: `internal/app/worker_cleanup_test.go`

1. 实现包装 `db.MediaAssetBackfillProcessor` 的 `processOnce`，维护完成复查时间和错误退避状态。
2. 保持 context 透传，不创建脱离 Worker 生命周期的 goroutine。
3. 在 Worker 完成 schema/setup binding 校验后构造 processor。
4. 仅当 `WORKER_ROLES` 包含 `cleanup` 时把 processor 加入公平轮转。
5. 日志包含稳定事件名、批次进度、错误次数和下次重试时间，不记录对象密钥等敏感信息。
6. 运行 Task 2 测试，确认通过。

### Task 4：补充数据库并发与幂等回归

**Files:**
- Modify: `internal/repository/db/media_asset_backfill_test.go`

1. 补充多个 processor 针对同一数据库反复执行，最终只产生一份资产且 checkpoint 完成的测试。
2. 保留完成后新增历史源记录自动重开 checkpoint 的覆盖。
3. 运行 `go test ./internal/repository/db -run 'MediaAssetBackfill' -count=1`，确认通过。

### Task 5：验证、review 与交付

**Files:**
- Modify: `.review/gate.json`（由工具生成）

1. 运行 `gofmt`。
2. 运行 `go test ./internal/app ./internal/repository/db -count=1`。
3. 运行 `./scripts/workflow/verify.sh`。
4. 提交全部代码和文档。
5. 运行 `./scripts/workflow/api-smoke.sh`。
6. 运行 `./scripts/workflow/review-local.sh --scope committed`。
7. 运行 `./scripts/workflow/check-review-gate.sh`。
8. 确认工作树仅保留预期生成文件，汇报分支和验证结果；本任务不执行 tag 发布。
