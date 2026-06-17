# Pic Gallery 线上问题修复方案

日期：2026-06-17

## 1. 背景

线上测试环境暴露出用户端视觉一致性、管理后台表单可用性、系统设置排版、计费倍率、生成进度恢复、图片下载 404、对象存储/BFSS 能力缺口等问题。

本方案基于当前代码仓库和线上测试环境排查结论整理。本文只描述问题、根因和修复方向，不包含账号密码、access token 或敏感配置。

## 2. 线上图片 404 复现与结论

### 2.1 复现现象

- 测试站点：`https://test.mikiko.studio`
- 测试用户登录后进入“资产”页。
- 页面展示 2 张历史生成图，但图片加载失败。
- 图片请求示例：
  - `/api/agent/image/v1/images/e03bfa5b-3965-4e56-a47b-67eb698a6613?access_token=<redacted>`
  - `/api/agent/image/v1/images/09455011-1b7d-4152-a26e-2d862931e467?access_token=<redacted>`
- 浏览器中两张图 `naturalWidth=0`。
- 使用同一登录 token 请求图片接口，后端返回：

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "image not found"
  }
}
```

### 2.2 线上服务证据

线上部署形态：

- `pic-gallery-api.service`
  - WorkingDirectory: `/home/pic-gallery/api-server`
  - Env file: `/home/pic-gallery/api-server/.env`
- `pic-gallery-worker.service`
  - WorkingDirectory: `/home/pic-gallery/worker`
  - Env file: `/home/pic-gallery/worker/.env`
- 中间件：
  - Postgres container: `pic-gallery-middleware-postgres-1`
  - Redis container: `pic-gallery-middleware-redis-1`
  - MinIO container: `pic-gallery-middleware-minio-1`

API 和 worker 的存储配置一致但使用相对路径：

```dotenv
STORAGE_DRIVER=local
STORAGE_LOCAL_ROOT=./tmp/storage
STORAGE_SHARED_VOLUME=false
```

由于两个进程工作目录不同，实际存储目录不同：

- API 读取目录：`/home/pic-gallery/api-server/tmp/storage`
- Worker 写入目录：`/home/pic-gallery/worker/tmp/storage`

数据库 `task_images` 记录：

```text
09455011-1b7d-4152-a26e-2d862931e467
storage_driver=local
object_key=generated-images/1/91893a2a-6ec6-43c2-ade0-4a72a0f4c075/0-09455011-1b7d-4152-a26e-2d862931e467.png
file_size_bytes=3122551
mime_type=image/png

e03bfa5b-3965-4e56-a47b-67eb698a6613
storage_driver=local
object_key=generated-images/1/91893a2a-6ec6-43c2-ade0-4a72a0f4c075/0-e03bfa5b-3965-4e56-a47b-67eb698a6613.png
file_size_bytes=3492072
mime_type=image/png
```

实际文件存在于 worker 目录：

```text
/home/pic-gallery/worker/tmp/storage/generated-images/1/91893a2a-6ec6-43c2-ade0-4a72a0f4c075/0-09455011-1b7d-4152-a26e-2d862931e467.png
/home/pic-gallery/worker/tmp/storage/generated-images/1/91893a2a-6ec6-43c2-ade0-4a72a0f4c075/0-e03bfa5b-3965-4e56-a47b-67eb698a6613.png
```

API 目录没有对应 generated image 文件。

### 2.3 根因

根因确认：线上 API 与 worker 都使用 `local` 存储，且 `STORAGE_LOCAL_ROOT=./tmp/storage` 是相对路径。由于 systemd 的 `WorkingDirectory` 不同，worker 写入的生成图在 worker 本地目录，API 下载接口在 API 本地目录读取，导致 API 找不到对象并返回 404。

这不是前端图片展示问题，也不是鉴权问题。它是 local storage 在多进程部署下未共享目录导致的存储割裂问题。

### 2.4 修复方向

短期止血：

1. 把 API 与 worker 的 `STORAGE_LOCAL_ROOT` 改为同一个绝对路径，例如 `/home/pic-gallery/storage`。
2. 设置 `STORAGE_SHARED_VOLUME=true`。
3. 将已有文件从两个进程目录合并迁移到共享目录：
   - `/home/pic-gallery/worker/tmp/storage/*`
   - `/home/pic-gallery/api-server/tmp/storage/*`
4. 重启 API 与 worker。
5. 回归验证历史生成图、参考图、头像等对象都能读取。

长期方案：

1. 引入 BFSS/S3 对象存储作为生成图默认存储。
2. API 不再中转图片字节，改为返回对象存储临时访问地址。
3. 增加多存储配置、图片存储位置记录、迁移同步任务和容量统计。

## 3. 问题清单与修复方案

### 3.1 用户端落地页、登录注册页风格不一致

结论：确认是问题。

根因：

- `LandingPage` 和 `LoginPage` 仍使用独立营销风格，包括 Luminous Vault 文案、大标题、装饰背景和独立卡片视觉。
- `HomePage`、`WorkspacePage` 已经使用新的产品化工作台风格，二者视觉语言不一致。

修复方案：

- 重构落地页、登录页、注册页为统一产品入口。
- 复用用户端导航、色彩 token、按钮、面板、页脚和工作台式信息组织。
- 落地页保持“可转化”但不再使用与主应用割裂的营销模板。
- 登录/注册页减少大装饰背景，使用与主页一致的密度、字体和控件。

验收标准：

- 未登录访问 landing/login/register 与登录后的 home/workspace 视觉一致。
- 移动端不出现标题、按钮、表单重叠。
- 登录错误、注册验证码、密码显隐等状态完整。

### 3.2 管理后台字体小、不清晰

结论：确认是问题。

根因：

- 后台全局样式存在 10px 级别 label/metadata。
- 输入框、select、textarea 字体没有统一放大。
- 登录页和各模块表单未共享明确的表单字体规范。

修复方案：

- 建立后台表单 typography token：
  - `--admin-label-font-size: 12px` 或 `13px`
  - `--admin-control-font-size: 15px` 或 `16px`
  - `--admin-helper-font-size: 13px`
  - `--admin-table-font-size: 14px`
- 所有 `input/select/textarea` 统一字体、行高、placeholder 颜色和 focus 样式。
- 登录页、系统设置、账号管理、收银台、用户管理、路由配置统一使用这些 token。
- 极小字体只保留在非关键元信息，且必须保证可读。

验收标准：

- 后台表单输入文字在 13-14 寸笔记本屏幕上清晰可读。
- 登录框、配置页、弹窗表单字号一致。
- 不出现同一表单内 label/input/helper 字号混乱。

### 3.3 系统设置排版错乱

结论：确认是问题。

根因：

- `ConfigPage` 当前以配置分类/左侧 rail 组织，仍像“配置项编辑器”。
- “编辑通用配置”里面又嵌套一批竖向配置，用户难以建立配置归属。
- 支付、SMTP、存储等本应具备专门业务表单的配置仍混在泛化配置里。

修复方案：

顶层改为横向 Tab：

1. 通用设置
2. 安全策略
3. 存储配置
4. 邮箱配置
5. 支付设置

建议归类：

- 通用设置：生成限制、文档入口、公共画廊、审核策略、OpenAI 兼容基本开关。
- 安全策略：注册登录、会话、API Key、安全限制。
- 存储配置：local、S3/BFSS、多存储、默认写入目标、连接测试。
- 邮箱配置：SMTP、发信人、TLS、测试邮件。
- 支付设置：收银台开关、套餐、自定义金额、支付方式、支付实例。

如果顶层 Tab 配置量过大，再拆竖向子 Tab：

- 存储配置：存储列表、默认策略、迁移任务、容量统计。
- 支付设置：支付总览、套餐、展示方式、渠道实例、密钥与回调。

验收标准：

- 顶层配置不再从上到下堆叠。
- 支付配置不再藏在通用配置中。
- 每个配置项 label/input/helper 字号统一。

### 3.4 后台表单输入模式不合理

结论：确认是问题。

根因：

- `ProviderModelsPage` 添加模型时币种字段是纯文本。
- `CashierPage` 充值套餐币种也是纯文本。
- 系统配置的 `list/map/object` 会退化为 JSON 或类 JSON 文本编辑。
- 支付渠道实例虽然已有部分结构化表单，但仍存在 JSON 高级字段暴露过多。

修复方案：

- 币种字段改为“预设下拉 + 允许自定义输入”：
  - 常用项：`CNY`, `USD`, `HKD`, `JPY`, `EUR`
  - 自定义值必须大写、长度 3-8、保存前 trim。
- 质量档位改为统一质量字典：
  - `auto`, `1k`, `2k`, `4k`
  - 显示层可展示 `1K/2K/4K`，保存层统一小写。
- 支付配置拆成可视化字段：
  - 支付宝：`app_id`, `gateway_url`, `merchant_private_key`, `alipay_public_key`, `notify_url`
  - 微信支付：`mch_id`, `app_id`, `api_v3_key`, `serial_no`, `private_key`, `notify_url`
  - 易支付：`pid`, `key`, `gateway_url`, `sign_type`
  - 极Pay/JeePay：`mch_no`, `app_id`, `way_code`, `api_key`, `gateway_url`, `channel_extra`
- JSON 编辑保留为“高级配置/原始 JSON”，默认折叠，且保存前 schema 校验。

验收标准：

- 常见字段不用手输枚举值。
- 支付渠道无需手写 JSON 即可完成基础配置。
- 高级 JSON 无效时给出字段级错误，不覆盖已有可用配置。

### 3.5 特殊分组倍率 0.5 不生效

结论：确认存在逻辑风险，需要修复为统一计费上下文。

现状：

- 普通用户 Web 估价和创建任务会传 `user.GroupMultiplier`。
- OpenAPI/API Key 路径使用静态 `Billing.UserGroupMultipliers[groupCode]`。
- RouteModel 路径会走 `ListVisibleRouteModels`，倍率来自路由模型可见分组匹配后的 `EffectiveMultiplier`。
- 对 public route model，`effectiveMultiplier` 会把 `1` 纳入候选倍率；如果该 route model 没有把用户特价组配置到可见组里，最终可能使用 1。

根因：

- “用户分组倍率”和“路由模型可见分组倍率”混在一起使用。
- Web、OpenAPI、RouteModel、余额展示没有统一的 pricing context resolver。
- API Key 的 group code 与用户真实分组/多分组之间缺少统一换算。

修复方案：

- 新增统一计费上下文：
  - `UserID`
  - `APIKeyID`
  - `GroupCodes`
  - `BillingMultiplier`
  - `VisibilityGroupCodes`
- 分组倍率以用户当前有效分组为权威。
- 多分组用户默认取最低有效倍率，除非产品明确选择“主分组倍率”。
- RouteModel 可见性只判断能不能用；计费倍率使用统一 `BillingMultiplier`。
- API Key 路径需要根据 API Key 所属用户和 API Key group code 共同解析，不能只读静态配置。

验收标准：

- 设置用户特价分组 0.5 后，Web 估价、Web 创建任务、OpenAPI 估价、OpenAPI 创建任务一致生效。
- 任务记录 `effective_multiplier` 与 pricing snapshot 一致。
- 用户余额展示中的 `user_group_multiplier` 与扣费一致。

### 3.6 生图页切换后丢失每张图生成状态

结论：确认是前端状态问题。

根因：

- 输出台每张图的展示依赖组件内部 `skeletonPhase` 计时。
- 从“当前创作”切走后组件卸载，再切回来状态重置，只能重新显示 loading。
- 任务真实状态没有足够驱动 UI 恢复“每张图生成情况”。

修复方案：

- 最小修复：把 `skeletonPhase` 提升到 `WorkspacePage`，按 `taskID` 存储。
- 更稳妥修复：根据任务数据推导输出槽位：
  - `status=running`
  - `output_image_count`
  - `results.length`
  - `created_at/started_at`
  - `progress_stage`
- 后端后续补充每张图 progress slot 最佳，但本轮可先用前端推导恢复。

验收标准：

- 开始创作后切换到其他页面再回来，仍能看到每张图状态槽位。
- 已完成的局部结果不回退为纯 Loading。
- 任务完成后展示结果不受影响。

### 3.7 成功图片 404

结论：确认是线上存储配置问题，同时暴露出诊断日志不足和架构风险。

根因：

- local storage 使用相对路径。
- API 与 worker 的工作目录不同。
- Worker 写入 `/home/pic-gallery/worker/tmp/storage`。
- API 读取 `/home/pic-gallery/api-server/tmp/storage`。
- 下载接口隐藏了底层错误，统一返回 `image not found`，日志没有打印 object key、driver、存储读错误。

修复方案：

短期：

- 使用共享绝对存储目录。
- 合并迁移已有 local 文件。
- 启动时如果非 local 环境且 `storage.driver=local`，强制要求 `storage.shared_volume=true` 和绝对路径。
- 下载失败日志增加：
  - image id
  - user id
  - storage driver
  - object key
  - backend error

长期：

- 默认改用 BFSS/S3 对象存储。
- API 返回临时访问 URL，不再中转图片字节。

### 3.8 API 中转图片带来性能压力

结论：确认是架构问题。

根因：

- `storage.Backend` 当前只有 `Put/Get/Delete`。
- 图片下载接口由 API 读取对象并写回响应体。
- 图片越多、越大，API 带宽和连接占用越高。

修复方案：

- 扩展 storage backend：
  - `PresignGet(ctx, objectKey, ttl) (url, expiresAt, error)`
  - `Head(ctx, objectKey) (size, contentType, etag, error)`
- 列表接口返回 `asset_url` 或 `download_url` 临时链接。
- 前端直接使用对象存储 URL 加载图片。
- TTL 建议：
  - 私有图片预览：10 分钟
  - 下载：5 分钟
  - 公开图片：可走 CDN 或更长缓存策略

业界依据：

- AWS S3 官方文档支持 GET/PUT presigned URL，控制有效期和权限范围。
- AWS CLI 默认预签名 URL 1 小时，最大 7 天；本项目不应使用过长 TTL。

### 3.9 后台缺少 BFSS 配置入口

结论：确认是功能缺口。

根因：

- 当前 `StorageConfig` 是单配置。
- 管理后台只展示静态 bucket 文案，缺少真实配置能力。
- `task_images` 只有 `storage_driver/object_key`，没有 `storage_config_id`。

修复方案：

- 新增后台“存储配置”模块：
  - local/S3/BFSS 类型
  - endpoint、region、bucket、prefix
  - access key/secret key write-only
  - 默认写入目标
  - 连接测试
  - 启停状态
- 敏感字段保存到 `secure_configs` 或专门 encrypted 字段。
- 配置发布前必须 test 通过。

### 3.10 多 BFSS、迁移同步、容量统计

结论：确认是架构升级需求。

修复方案：

- 新增多存储配置表。
- 图片、参考图、头像等对象记录具体 `storage_config_id`、bucket、object key。
- 读写通过 storage registry 按 `storage_config_id` 路由。
- 后台新增迁移任务：
  - source storage
  - target storage
  - scope
  - dry run
  - resume/retry
  - copy 成功后更新对象记录
- 容量统计：
  - 每个 storage/bucket 图片数量
  - 总容量
  - 最近写入时间
  - 迁移中/失败对象数

验收标准：

- 新图按默认存储写入。
- 历史图按记录中的 storage config 读取。
- 迁移后历史图片仍可访问。
- 后台能看到每个 bucket 的数量和容量。

## 4. 疑问解答与建议修复

### 4.1 质量列表与价格质量档位是否同源

结论：不是同一个数据源。

- 接入账号添加模型里的质量列表：真实上游模型能力，来自 `ModelAccountModel.qualities`。
- 价格配置里的质量档位：路由模型计费配置，来自 `RouteModelPrice.quality`。
- 最终关联方式：运行时解析请求质量为一个质量 bucket，然后同时要求：
  - 候选真实模型支持该 quality
  - 路由模型价格表存在该 quality

风险：

- 两处手工维护，容易出现大小写、命名和缺项不一致。
- `auto` 的解析与价格表质量不一致时，会出现估价/创作失败。

建议修复：

- 新增统一质量字典或配置枚举。
- 存储统一小写：`auto`, `1k`, `2k`, `4k`。
- 价格配置新增时只允许从字典选择，必要时可创建自定义质量。
- 真实模型质量和价格质量都引用同一字典展示。
- 后台提供“质量覆盖检查”：列出 route price 有但候选不支持、候选支持但未定价的项。

### 4.2 单图成本计算时币种是否参与

结论：当前币种不参与用户扣费计算。

现有扣费：

- 用户侧扣费按积分，不按货币汇率实时换算。
- 传统模型公式：

```text
actual_points =
  base_unit_points
  * task_multiplier
  * (1 + reference_extra_multiplier)
  * user_group_multiplier
  * success_output_image_count
```

- RouteModel 公式：

```text
charged_points =
  route_model_price.base_points
  * task_multiplier
  * effective_multiplier
  * output_image_count
```

当前额外发现：

- 后台“接入账号-添加模型”的 `cost_per_image` 映射到运行态候选时只进入了 `InputCost`。
- 最终 `calculateProviderCost` 读取的是 `candidate.OutputCost`。
- 这可能导致调用记录里的 `provider_cost` 长期为 0，影响成本和毛利统计。

建议修复：

- 明确区分：
  - 用户扣费积分：不受币种影响。
  - 上游成本：保留原始货币金额和币种。
  - 毛利统计：如果要跨币种比较，必须引入汇率快照。
- 运行态映射修复：

```text
candidate.OutputCost = model.CostPerImage
candidate.Currency = model.Currency
```

- 调用记录增加或规范化：
  - `provider_cost`
  - `provider_cost_currency`
  - `provider_cost_cny`
  - `fx_rate_snapshot`
- 没有汇率配置时，毛利不跨币种计算，只展示原币种成本。

### 4.3 各类优先级、权重、排序、兜底顺序如何生效

当前语义：

- 接入账户 `优先级/权重`：
  - 旧路由/账号级候选排序语义。
  - 在 RouteModel 新路径中不是主要决策字段。
- 路由模型 `排序`：
  - 只影响用户侧可见模型列表展示顺序。
  - 不决定上游调用优先级。
- 候选模型 `优先级`：
  - RouteModel 内部候选的第一层排序。
  - 数值越小越优先。
- 候选模型 `权重`：
  - 同优先级 bucket 内做稳定哈希权重选择主候选。
  - 权重小于等于 0 时按 1 处理。
- 候选模型 `兜底顺序`：
  - 主候选失败后，同优先级或后续候选按兜底顺序尝试。

建议修复：

- 后台把“账号级路由字段”和“路由模型候选字段”分组显示，避免误解。
- RouteModel 候选编辑页增加说明：

```text
调用选择顺序：
1. 按候选优先级从小到大分组。
2. 在同一优先级内按权重稳定选择主候选。
3. 主候选失败后，按兜底顺序尝试其它候选。
4. 路由模型排序只影响用户看到的模型列表顺序。
```

- 后台提供“模拟路由”功能：
  - 输入用户/分组、任务类型、质量、尺寸。
  - 输出候选顺序、命中的主候选、兜底链路和计费倍率。

## 5. 建议实施优先级

### P0：线上可用性与计费正确性

1. 修复线上 local storage 共享目录，恢复历史图片访问。
2. 增加下载失败诊断日志。
3. 统一用户分组倍率计费上下文。
4. 修复生成页切换后输出槽位恢复。
5. 修复 provider cost 映射，避免成本统计失真。

### P1：后台配置与表单体验

1. 管理后台字体统一放大。
2. 系统设置改顶层横向 Tab。
3. 币种、质量等字段改枚举/可自定义输入。
4. 支付配置改结构化表单。
5. 用户端 landing/login/register 风格统一。

### P2：存储架构升级

1. 多 BFSS/S3 配置。
2. 对象存储预签名 URL。
3. 存储迁移同步任务。
4. Bucket 数量和容量统计。
5. 存储配置后台连接测试、灰度切换和回滚。

## 6. 风险与待确认

| 项目 | 风险 | 建议 |
|---|---|---|
| 多分组倍率 | 取最低倍率还是主分组倍率属于产品策略 | 建议默认取最低有效倍率，需产品确认 |
| RouteModel public 可见性 | public 模型是否仍应享受用户分组折扣 | 建议可见性与计费倍率分离，需确认 |
| BFSS 协议 | BFSS 是否完全 S3 兼容、签名方式和 endpoint 规则 | 需运维/存储负责人确认 |
| 历史文件迁移 | local、MinIO、未来 BFSS 之间迁移存在失败重试和一致性风险 | 需先做 dry run 和可恢复迁移任务 |
| 成本币种 | 是否需要按汇率统一折算毛利 | 需财务/运营确认 |

