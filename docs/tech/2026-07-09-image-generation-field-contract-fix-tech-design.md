# 图片生成参数字段契约修正技术方案

> 文档版本：v1.4
>
> 创建日期：2026-07-09
>
> 关联需求：本次字段修正要求；`docs/tech/2026-07-09-auth-session-and-image-generation-parameters-tech-design.md`
>
> 方案状态：已按本方案完成字段契约修正；本轮补齐调用记录、图库展示、用户偏好和日志字段；验证结果见第 6 节

## 1. 背景与目标

图片生成参数能力改造过程中，曾出现字段命名偏移：

- 真正传给上游的质量参数被命名为 `generation_qualities`。
- 原 `qualities` 的 1K/2K/4K 分辨率档位语义与真正上游 `quality` 混用。
- 审核模式字段被命名为 `moderation_modes`。

本次修正目标是把字段契约一次性定死，后续实现不得再引入旧字段兼容：

| 字段 | 语义 | 示例 | 主要用途 |
|---|---|---|---|
| `base_resolution` | 原 `qualities`，表示基础分辨率/计费档位 | `auto`, `1k`, `2k`, `4k` | 尺寸换算、价格匹配、计费预估、最终扣费 |
| `quality` | 真正传给上游的质量参数 | `auto`, `low`, `medium`, `high` | 模型能力校验、上游请求参数 |
| `moderation` | 审核等级 | `auto`, `low` | 模型能力校验、上游请求参数 |

本轮最终补充约束：

- 后台调用记录必须同时返回 `base_resolution` 和 `quality`；`base_resolution` 存 1K/2K/4K 计费档，`quality` 存真实上游质量参数。
- 用户端图库、公开图库、同款生成上下文和默认生成偏好中，凡是 `auto/1K/2K/4K` 语义均命名为 `base_resolution`。
- 日志字段中禁止再使用 `request_quality` 表示请求质量；真实质量统一输出为 `quality`，基础分辨率统一输出为 `base_resolution`。
- 审核能力配置和请求字段只使用 `moderation`，不得使用 `moderation_modes`。

明确不做：

- 不迁移历史数据。
- 不兼容旧请求字段。
- 不保留 `generation_qualities`、`moderation_modes`、`qualities` 的生产代码入口。
- 初始化新库时也不得再创建 `requested_quality`、`resolved_quality_bucket` 这类历史任务列。
- OpenAPI、mock/demo 复制层也不得继续公开旧字段，避免后续按旧契约生成客户端或抄写实现。

## 2. 字段契约

### 2.1 模型能力配置

账号模型配置使用如下 JSON 契约：

```json
{
  "base_resolution": ["auto", "1k", "2k"],
  "quality": ["auto"],
  "max_reference_image_count": 5,
  "max_image_count": 1,
  "size_modes": ["ratio", "pixel"],
  "supported_ratios": ["1:1", "16:9"],
  "supported_pixel_sizes": ["1024x1024"],
  "output_format": ["png", "jpeg", "webp"],
  "output_compression": 100,
  "moderation": ["auto", "low"]
}
```

约束：

- `base_resolution` 必填，至少 1 个值。
- `quality` 必填，允许值为 `auto | low | medium | high`。
- `moderation` 必填，允许值为 `auto | low`。
- `output_format` 必填，允许值为 `png | jpeg | webp`。
- `output_compression` 范围 `0-100`，默认 `100`。
- `max_image_count` 小于等于 0 时规范化为 `1`。

### 2.2 计费价格配置

路由模型价格只使用 `base_resolution`，禁止使用 `quality`：

```json
{
  "route_model_id": 1,
  "task_type": "text_to_image",
  "base_resolution": "1k",
  "base_points": "8.00000",
  "reference_multiplier": "1.00000",
  "enabled": true
}
```

### 2.3 用户生图请求

比例模式：

```json
{
  "task_type": "text_to_image",
  "route_model_code": "plus",
  "size_mode": "ratio",
  "aspect_ratio": "1:1",
  "base_resolution": "1k",
  "quality": "auto",
  "output_format": "png",
  "output_compression": 100,
  "moderation": "auto",
  "requested_size": "auto",
  "requested_output_image_count": 1
}
```

像素模式：

```json
{
  "task_type": "text_to_image",
  "route_model_code": "plus",
  "size_mode": "pixel",
  "base_resolution": "auto",
  "quality": "auto",
  "output_format": "png",
  "output_compression": 100,
  "moderation": "auto",
  "requested_size": "1024x1024",
  "requested_output_image_count": 1
}
```

### 2.4 图片任务落库字段

新生成任务只落新字段，不写旧字段：

```sql
image_tasks.size_mode           varchar(16) default 'ratio'
image_tasks.base_resolution     varchar(16) default 'auto'
image_tasks.quality             varchar(16) default 'auto'
image_tasks.output_format       varchar(16) default 'png'
image_tasks.output_compression  int         default 100
image_tasks.moderation          varchar(16) default 'auto'
```

### 2.5 调用记录与图库展示

后台调用记录响应契约：

```json
{
  "task_id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
  "abstract_model": "plus",
  "base_resolution": "2k",
  "quality": "auto",
  "requested_output_image_count": 2,
  "success_output_image_count": 1
}
```

图库图片响应契约：

```json
{
  "id": "img_01",
  "task_id": "task_01",
  "route_model_code": "plus",
  "base_resolution": "2K",
  "quality": "auto",
  "aspect_ratio": "16:9"
}
```

前端展示规则：

```text
GalleryCard:
  display_resolution = image.base_resolution
  display_quality = image.quality only when explicitly showing upstream quality

ContinueEditContext:
  route_model_code = image.route_model_code
  base_resolution = image.base_resolution
  aspect_ratio = image.aspect_ratio
```

禁止字段：

```sql
image_tasks.requested_quality
image_tasks.resolved_quality_bucket
```

说明：本次明确不考虑历史数据迁移；因此初始化 SQL、Ent schema、任务创建链路保持同一套新契约即可。

## 3. 核心流程契约

### 3.1 能力规范化

```text
NormalizeCapability(raw):
  base_resolution = normalizeBaseResolution(raw.base_resolution default ["auto", "1k"])
  if empty(base_resolution):
    reject "base_resolution is required"

  quality = normalizeEnum(raw.quality default ["auto"], ["auto", "low", "medium", "high"])
  if empty(quality):
    reject "quality is required"

  moderation = normalizeEnum(raw.moderation default ["auto"], ["auto", "low"])
  if empty(moderation):
    reject "moderation is required"

  output_format = normalizeEnum(raw.output_format default ["png"], ["png", "jpeg", "webp"])
  output_compression = raw.output_compression
  if output_compression == 0:
    output_compression = 100
  if output_compression < 0 or output_compression > 100:
    reject "output_compression must be between 0 and 100"

  max_image_count = max(raw.max_image_count, 1)
  return normalized capability
```

### 3.2 候选账号匹配

```text
CandidateSupportsRequest(candidate, req):
  normalized = NormalizeResolveRequest(req)

  if normalized.quality not in candidate.quality:
    return false

  if normalized.output_format not in candidate.output_format:
    return false

  if normalized.moderation not in candidate.moderation:
    return false

  if normalized.size_mode == "ratio":
    if normalized.base_resolution not in candidate.base_resolution:
      return false
    if normalized.aspect_ratio not in candidate.supported_ratios:
      return false

  if normalized.size_mode == "pixel":
    if normalized.requested_size not in candidate.supported_pixel_sizes:
      return false

  if req.requested_output_image_count > global_max_image_count:
    return false

  if req.reference_image_count > candidate.max_reference_image_count:
    return false

  return true
```

### 3.3 计费预估与扣费

计费只依赖 `base_resolution`：

```text
Estimate(req):
  resolved = resolver.Resolve(req)
  price = findRoutePrice(
    route_model_id = resolved.route_model_id,
    task_type = req.task_type,
    base_resolution = resolved.base_resolution
  )
  if price missing:
    reject IMAGE_CAPABILITY_MISMATCH

  unit_points = price.base_points
  reference_multiplier = price.reference_multiplier if req.reference_image_count > 0 else 1
  estimated_points = unit_points * req.requested_output_image_count * reference_multiplier * user_group_multiplier
  return estimated_points, base_resolution = resolved.base_resolution
```

`quality` 不参与本期计费公式。后续如果质量参数需要加价，必须新增独立倍率字段，不得复用 `base_resolution`。

### 3.4 Worker 上游请求

```text
BuildProviderRequest(task, resolved, chunk_count):
  req.model = resolved.model_code
  req.prompt = task.prompt
  req.n = chunk_count
  req.size = resolved.requested_size
  req.quality = task.quality default "auto"
  req.output_format = task.output_format default "png"
  if req.output_format in ["jpeg", "webp"]:
    req.output_compression = task.output_compression default 100
  req.moderation = task.moderation default "auto"
  return req
```

禁止逻辑：

```text
DoNotDeriveQualityFromBaseResolution:
  if base_resolution == "4k":
    quality must NOT be auto-mapped to "high"
  if base_resolution == "2k":
    quality must NOT be auto-mapped to "medium"
  if base_resolution == "1k":
    quality must NOT be auto-mapped to "low"

  upstream quality = request.quality or model default quality
```

说明：`base_resolution` 只表示基础分辨率/计费档位；`quality` 才是真正上游质量字段。GPT / Codex / OpenAI Compatible 适配逻辑均不得用 `base_resolution` 反推 `quality`。

## 4. 落地范围

### 4.1 后端

- `internal/domain/modelhub/capability.go`
  - 定义并规范化 `base_resolution`、`quality`、`output_format`、`output_compression`、`moderation`。
- `internal/domain/billing`
  - 计费请求、结果和价格快照使用 `base_resolution`。
  - 计费解析器接口命名为 `baseResolutionResolver`，禁止把基础分辨率继续称为 quality。
- `internal/domain/imagetask`
  - 任务和测试请求保留真实 `quality`，同时保留 `base_resolution`。
  - 图库领域对象 `GalleryImage` 同时返回 `base_resolution` 与真实 `quality`。
- `internal/domain/modelhub`
  - 路由解析、候选匹配、尺寸换算统一使用 `BaseResolution` 命名表示 1K/2K/4K 基础分辨率。
  - `CalculateImageSize(baseResolution, aspectRatio)` 只接受基础分辨率，不得传入真实上游 `quality`。
- `internal/http/handlers/api.go`
  - estimate、task create、model account model write、test image、route price write 均使用新字段。
- `internal/service/modeladmin`
  - 模型配置保存走统一能力规范化。
  - route price 校验错误文案改为 `base_resolution`。
- `internal/service/imagetask`
  - Worker 构造上游请求时直接透传任务 `quality`。
  - 对 `gpt-image-2` Images API 来源不再根据 `base_resolution` 自动映射 `low/medium/high`。
  - fanout 日志输出 `base_resolution` 与 `quality`，不再输出误导性的 `request_quality`。
- `internal/domain/admincallrecord` 与 `internal/repository/entstore/admin_call_record_store.go`
  - 后台调用记录响应新增 `base_resolution`。
  - `quality` 从 `image_tasks.quality` 映射，不再从 `image_tasks.base_resolution` 映射。
- `internal/repository/entstore/model_admin_store.go`
  - 保存、读取、路由快照均传递新字段。
- `internal/repository/ent/schema/imagetask.go`
  - 任务表模型只定义 `base_resolution`、`quality`、`moderation`。
- `internal/repository/ent/migrations/000001_init.sql`
  - fresh DB 初始化任务表使用 `size_mode/base_resolution/quality/output_format/output_compression/moderation`。
  - 不再创建 `requested_quality/resolved_quality_bucket`。

### 4.2 前端

- `web/shared/api-types.ts`
  - `RouteModelPrice` 和写请求字段改为 `base_resolution`。
  - `EstimateRequest` 拆分 `base_resolution` 与真实 `quality`。
  - `EstimateResult` 使用 `base_resolution`，不再定义 `resolved_quality`。
  - `GenerationPreferences` 使用 `base_resolution` 表示默认基础分辨率。
  - `GalleryImage`、`ImageResult`、`CallRecord` 同时定义 `base_resolution` 与真实 `quality`。
- `web/shared/user-api.ts`
  - estimate/task create query/body 传递 `base_resolution`、`quality`、`output_format`、`output_compression`、`moderation`。
- `web/shared/image-size.ts`
  - 导出 `calculateImageSizeForBaseResolution(baseResolution, ratio)`，避免再用 `calculateImageSizeForQuality` 混淆基础分辨率与上游质量。
- `web/shared/mock-api.ts`
  - mock 计费倍率变量使用 `baseResolutionMultiplier`，mock 像素尺寸映射使用 `mockBaseResolutionBucket`。
- `web/shared/admin-api.ts`
  - 账号模型和价格配置映射新字段。
- `web/admin/src/pages/PricingPage.tsx`
  - 页面状态和提交 payload 使用 `base_resolution`。
- `web/admin/src/pages/ProviderModelsPage.tsx`
  - “基础分辨率”和“质量参数”拆开。
  - 增加最大出图数、输出格式、压缩质量、审核等级配置。
  - 测试出图弹窗中基础分辨率状态命名为 `baseResolution`，避免再出现 `requestedQuality` 语义漂移。
- `web/admin/src/pages/callRecordRows.ts`
  - 调用记录详情展示 `base_resolution · quality · source_channel`。
- `web/user/src/pages/WorkspacePage.tsx`
  - 比例模式选择项状态命名为 `baseResolution`，请求中固定发送 `base_resolution` 和真实 `quality`。
- `web/user/src/pages/GalleryPage.tsx`、`web/user/src/pages/PublicGalleryPage.tsx`
  - 同款生成上下文写入 `base_resolution`，不再写入旧 `quality=1K/2K/4K`。
- `web/user/src/pages/ProfilePage.tsx`
  - 默认生成偏好文案改为“默认基础分辨率”，字段改为 `base_resolution`。

### 4.3 脚本

- `scripts/e2e/docker-e2e.mjs`
  - route price 创建和查找使用 `base_resolution`。

### 4.4 OpenAPI 与 Demo

- `api/openapi/components/parameters/common.yaml`
  - `RequestedQuality` 参数改为 `BaseResolution`。
  - 新增 `Quality`、`OutputFormat`、`OutputCompression`、`Moderation` 查询参数。
- `api/openapi/components/schemas/agent.yaml`
  - `EstimateResponse` 返回 `base_resolution`。
  - `CreateImageTaskRequest` 使用 `route_model_code + base_resolution + quality + moderation`。
  - `CapabilityItem` 暴露 `base_resolution`、`quality`、`output_format`、`moderation`。
- `api/openapi/components/schemas/admin.yaml`
  - Provider model 能力字段使用 `supported_base_resolution`、`quality`、`output_format`、`moderation`。
  - AdminCallRecord 增加 `base_resolution`，并保留真实 `quality`。
- `api/openapi/components/schemas/common.yaml`
  - 私有图库、公开图库列表和公开图库详情均增加 `base_resolution`。
- `web/redesign-demo`
  - mock estimate、open-api query、价格示例同步使用 `base_resolution`。

## 5. 验收标准

- 生产代码扫描不出现旧字段：
  - `generation_qualities`
  - `moderation_modes`
  - `requested_quality`
  - `resolved_quality`
  - `resolved_quality_bucket`
  - `supported_qualities`
  - `qualities`
- 管理后台价格策略创建/编辑请求体使用 `base_resolution`。
- 管理后台账号模型配置保存后，DB 和路由快照均包含：
  - `base_resolution`
  - `quality`
  - `max_image_count`
  - `output_format`
  - `output_compression`
  - `moderation`
- 用户端 estimate 和 create task 均拆分发送 `base_resolution` 与真实 `quality`。
- 用户端图库、公开图库和同款生成上下文使用 `base_resolution` 展示/恢复 1K/2K/4K 档位。
- 后台调用记录 `base_resolution` 与 `quality` 字段语义正确，不再把基础分辨率塞入 `quality`。
- 日志使用 `base_resolution` 和 `quality`，不再出现 `request_quality`。
- 计费预估和扣费只按 `base_resolution` 找价格。
- 模型能力匹配会校验真实 `quality` 和 `moderation`。
- Worker 上游请求中的 `quality` 来自真实 `quality` 字段，不得由 `base_resolution` 派生。
- OpenAPI 中不再出现 `requested_quality`、`resolved_quality_bucket`、`supported_qualities`。
- fresh DB 初始化 SQL 中不再出现 `requested_quality`、`resolved_quality_bucket`。

## 6. 已执行验证

```bash
rg -n "calculateImageSizeForQuality|QualityBucket|qualityBucket|qualityMulti|generation_qualities|moderation_modes|requested_quality|resolved_quality|resolved_quality_bucket|supported_qualities|\\bqualities\\b|GenerationQualities|ModerationModes|RequestedQuality|ResolvedQuality|SupportedQualities|Qualities" internal web/shared web/admin/src web/user/src api scripts --glob '!node_modules' --glob '!web/**/dist'
go test ./internal/domain/modelhub ./internal/domain/billing ./internal/service/billing ./internal/service/imagetask -count=1
npm --prefix web/user run typecheck
npm --prefix web/admin run typecheck
./scripts/workflow/verify.sh
```

验证结果：

- 字段扫描：无命中。生产链路中不再出现旧字段入口，也不再出现把基础分辨率命名为 `QualityBucket/qualityBucket/qualityMulti/calculateImageSizeForQuality` 的实现。
- Go 定向测试：`internal/domain/modelhub`、`internal/domain/billing`、`internal/service/billing`、`internal/service/imagetask` 通过。
- 前端类型检查：`web/user`、`web/admin` 通过。
- 仓库级验证：`./scripts/workflow/verify.sh` 通过，覆盖 `go test ./...`、`go vet ./...`、前端 contract、`web/user` typecheck/build、`web/admin` typecheck/build。`web/admin` build 仅有 Vite chunk size warning，不影响通过。

## 7. 后续准出建议

- 本轮字段修正已完成定向验证；在合并包含 Token 续期、Worker fanout、能力配置等更大范围改造前，仍需执行仓库级 `./scripts/workflow/verify.sh` 和 review gate。
- 若后续 `quality` 需要参与加价，必须新增独立质量倍率字段和价格表维度，不得复用或污染 `base_resolution`。
