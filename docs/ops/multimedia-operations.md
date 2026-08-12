# 多媒体创作一期运维手册

本文覆盖视频生成、媒体处理、创意画布、本地上传和历史图片统一资产回填。PostgreSQL 是任务、账本、画布 revision、上传会话和资产状态的真相源；Redis 仅承载短期通知、缓存和并发租约。任何恢复操作都不得重新调用 Provider、重新生成结果或重复扣费。

## Worker 角色与扩缩容

同一 `mikiko-gallery-studio-worker` 二进制通过 `WORKER_ROLES` 启动独立循环：

| 角色 | 负责工作 | 扩容信号 | 约束 |
| --- | --- | --- | --- |
| `image` | 现有图片生成 | 图片排队时长、租约积压 | 保留现有并发和模型限流 |
| `video` | Provider 提交、轮询、回调收敛、结果转存 | 视频排队时长、running 数、Provider 配额 | Redis 不可用且无法安全取并发令牌时停止领取新任务 |
| `media` | ffprobe、poster、preview、proxy 等派生处理 | processing job 积压、处理耗时、CPU | 与 Provider 轮询并发分离 |
| `cleanup` | 对象清理、导出、过期 multipart、媒体对账 | deletion/export/reconcile 积压 | processor 公平轮转，不能让空闲队列阻塞其他队列 |

按角色水平扩容，不要用提高单进程总并发替代容量评估。`media` 初始并发取 `min(2, CPU/2)`；每个 720P proxy job 预留约 1-2 vCPU、512 MiB-1 GiB 内存和原件 2-3 倍临时盘。扩容前确认对象存储、数据库连接池和 Provider 账号配额仍有余量。

## FFmpeg 与临时盘

Docker Worker 固定安装 FFmpeg/ffprobe；Native Worker 必须由宿主机提供可执行文件。部署后检查 Worker readiness、`ffmpeg -version`、`ffprobe -version` 和黄金媒体探测结果。媒体进程必须保持 `-nostdin`、禁止网络协议、受控线程/时长/像素/帧数/输出大小和墙钟超时。

`VIDEO_ARTIFACT_ALLOW_LOOPBACK` 与 `VIDEO_ARTIFACT_TEST_CA_FILE` 只用于 local/test 隔离验收自签 HTTPS 产物站。后者必须与前者同时开启，生产环境配置加载会拒绝任一测试能力；生产 Provider 产物仍必须使用公共可信证书、HTTPS 和账号级精确 host allowlist。

监控 `pic_gallery_worker_temporary_disk_used_percent` 与 `pic_gallery_worker_temporary_disk_free_bytes`：

- 使用率达到 75%：暂停领取新的媒体处理任务，告警并清理已结束 job 的临时文件。
- 使用率达到 90%：进入 critical，所有需要临时文件的新 job 停领；保留轮询、状态收敛和不使用临时盘的 cleanup。
- 水位恢复后由 Worker 自动继续领取。不要删除仍处于 running/leased job 的目录。

## Redis 降级

Redis 故障时，任务状态继续由 PostgreSQL 驱动，SSE 退化为轮询，可丢缓存允许重建。视频执行若无法安全取得用户、账号或模型并发令牌，必须停止领取新任务；已提交 Provider 的 attempt 继续按数据库记录查询和收敛，不能因为 Redis 丢失而盲目重提。Redis 恢复后观察租约、排队时长和重复 attempt 指标，再逐步恢复正常并发。

## 定期对账

对账只修复平台内部状态，不发起新的生成或收费：

1. 视频：核对超时 running attempt 的 Provider 状态、上游成功但缺失平台 asset 的结果转存、终态任务未结算 reservation。已有 provider task ID 时只查询原任务。
2. 媒体：扫描 `processing/ready_original` 资产。缺 probe/v1 job 时创建；active job 不动；terminal/succeeded 但派生缺失时重置同一唯一 job；派生齐全才推进 `ready`。
3. 画布：核对 generation run、task/result asset 与 attached node。恢复时复用已有 run/task/asset，按 revision 冲突规则附着，禁止重新生成。
4. 上传：核对 completed session 是否已有 asset；到期 active multipart 领取为 `expiring`，先调用原存储后端 Abort，再释放 reserved quota。Abort 失败保留配额，等待 lease 超时重试；对象已不存在等同 Abort 成功。
5. 对象删除：资产删除只进入 durable deletion job。确认引用计数和 storage identity 后再删对象；失败按现有退避重试，禁止凭 object key 猜测存储实例。

后台视频任务页只提供三类幂等恢复：重新转存、重新处理派生资源、重新结算。重新结算仅适用于全部结果已进入 `succeeded/failed/cancelled` 终态且 `settlement_status != finalized` 的任务；它只释放过期租约并让 Video Worker 复用原 reservation 和账本幂等键继续收敛，不调用 Provider。运行中任务和已完成结算的任务会返回冲突，不能通过手工改状态绕过。

## 媒体正文与 Local 兼容例外

S3、R2 和 MinIO 模式的上传、预览与下载正文必须走短时签名 URL，应用 API 只返回授权元数据。Local filesystem 没有独立对象端点，因此 `/content` 是技术方案允许的兼容例外：应用使用 `io.Reader` 和 HTTP Range 流式输出，不把完整文件读入内存，也不经过 JSON/base64。反向代理必须关闭该路径的整包缓冲并保留 Range；生产环境若媒体流量明显增长，应迁移到支持直传直取的对象存储，不能通过提高 API 内存限制维持 Local 模式。

## 历史图片统一资产回填

回填只复制 `task_images` 元数据到 `media_assets`，两表使用同一 UUID，并复用原 storage config/driver/object key。该过程不读取、下载、上传或复制对象正文。只能在 `DEPLOYMENT_ROLE=single` 或 `control` 的节点显式运行；不要放入普通服务启动或 schema migration。

Docker 使用目标 API 镜像的一次性容器，Native 使用发布包中的 `bin/mikiko-gallery-studio-media-backfill`。命令读取与服务相同的 `APP_ENV_FILE`，也可显式传 `--env-file`。

先执行只读计划：

```bash
mikiko-gallery-studio-media-backfill --env-file ./config/runtime.env --dry-run --batch-size 100 --max-batches 10
```

确认 JSON 中 `would_create`、存储身份和批次规模合理后，再限批写入：

```bash
mikiko-gallery-studio-media-backfill --env-file ./config/runtime.env --batch-size 100 --max-batches 20
```

重复执行会从 `migration_checkpoints` 的 `(created_at,id)` 稳定游标恢复；单行使用 `ON CONFLICT (id) DO NOTHING` 保证同 ID 重放幂等，其他唯一键冲突会失败并要求人工核对。`max-batches=0` 表示运行到完成。中断进程不会推进未提交批次的 checkpoint。

完成后执行聚合和确定性抽样校验：

```bash
mikiko-gallery-studio-media-backfill --env-file ./config/runtime.env --verify --sample-size 100
```

验收要求：`completed=true`、verification 的 source/asset count 与 bytes 一致、各 storage identity 聚合一致、样本的 asset ID、legacy image ID、user、project、object key 全部一致。历史图片缺 project 时，预期项目是该用户的默认项目。

## 发布、回滚与故障恢复

采用 expand-first：先部署新增表、字段、索引和兼容双读，再启用新 Worker/API，最后执行历史回填。旧图片表、旧读取路由和对象不得在本期收缩。代码回滚时停用视频/画布/上传创建入口和新 Worker 角色，保留新增 schema、checkpoint、已生成资产和所有存储配置；让已提交 Provider 的任务继续由兼容版本收敛，必要时只查询原 provider task。

故障处理顺序：

1. 停止受影响角色领取新任务，不删除数据库终态、Provider ID、reservation 或对象引用。
2. 核对 PostgreSQL 状态、Worker readiness、对象存储和 Redis；用 request/task/attempt/job ID 串联日志。
3. 恢复依赖后先启动 `cleanup` 和对应 `video/media` 角色，让 durable job 与对账器收敛。
4. 对账账本 reservation 与终态、Provider 成功结果与 asset、asset 与派生对象；发现差异时修平台投影，不重提 Provider。
5. 指标稳定且无重复结算、重复对象或 checkpoint 停滞后，再恢复创建入口和正常并发。

发布阻断条件包括重复扣费、重复 Provider 任务、跨用户/项目资产暴露、回填 ID/object/project/user 不一致、临时盘持续超过 90%、对象删除引用判断失真或任务无法在对账后收敛。
