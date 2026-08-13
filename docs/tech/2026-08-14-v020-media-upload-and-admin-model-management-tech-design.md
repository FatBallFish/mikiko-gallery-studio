# v0.0.20 媒体上传与模型管理优化技术方案

## 1. 文档信息

- 文档状态：待评审、待开发
- 编写日期：2026-08-14
- 代码基线：远端 `main`，提交 `4aca7a2`，版本 `v0.0.19`
- 需求来源：`docs/prd/2026-08-14-v020-media-upload-and-admin-model-management-requirements.md`
- 目标版本：暂定 `v0.0.20`

## 2. 现状与根因

### 2.1 上传链路分裂

当前存在两套上传链路：

```text
创作页本地图片
  -> /api/agent/image/v1/reference-assets
  -> reference_assets
  -> 仅供图片生成引用

资产页统一上传
  -> /api/agent/media/v1/uploads
  -> media_upload_sessions
  -> media_assets
  -> media_processing_jobs / media_derivatives
```

`WorkspacePage.uploadReferenceFiles` 仍调用 `userApi.uploadReferenceAsset`，因此创作页上传不会写入 `media_assets`，也不会触发资产页的 `mgs:media-assets-changed` 刷新事件。

统一上传能力已经支持多文件、1 GiB、分片、断点恢复、进度、暂停、重试和取消，问题不是缺少上传器，而是创作页没有接入统一上传器，`MediaAsset` 与 `ReferenceAsset` 之间也缺少正式关联。

### 2.2 资产预览没有降级

`MediaAssetCard` 对图片只请求 `purpose=thumbnail`。后端在没有缩略图派生资源时，仅对带 `LegacyImageID` 的图片允许原图回退；新统一资产返回 `409 DERIVATIVE_NOT_READY`。前端捕获错误后不展示错误，也不请求 `preview`，最终只剩占位符。

生成图片会经历 `ready_original -> ready`，但旧数据、派生任务失败、worker 未及时处理和临时 URL 过期都可能导致缩略图不可用。预览必须同时具备服务端合理回退和前端逐层降级。

### 2.3 资产详情缺少统一的媒体预览能力

资产列表和详情目前只复用部分卡片预览逻辑，未复用首页、创作页已有的图片全屏查看器。结果是图片详情不能二次打开全屏，视频详情无法在用户点击后进入播放器，音频和文档也没有符合媒体类型的阅读或播放交互。本期将详情组件设计为“元数据面板 + 媒体预览入口”，详情内的媒体主体根据类型分发到统一预览控制器。

### 2.4 上传入口状态需要区分功能关闭与配置保存失败

用户端资产页按钮和全局 `UploadTray` 确实受 `media_upload` 功能开关控制，但线上无法开启该开关的直接原因是管理端把请求路径拼成了 `/api/ops/admin/v1/config-tabs/features`。服务端定义的配置 Tab key 是 `site`，`features` 只是该 Tab 下的配置分类，因此该请求会返回 `config tab not found`。

配置层级必须明确区分：

| 配置概念 | 值 |
| --- | --- |
| Tab key | `site` |
| config category | `features` |
| config keys | `media_upload`、`creative_canvas`、`video_creation` |

正确保存路径为 `PUT /api/ops/admin/v1/config-tabs/site`，请求项中的 `config_category` 才使用 `features`。保存前必须读取配置列表中的最新 `site.version`，处理版本冲突；保存失败要在管理端展示接口错误，不能把配置失败误报为“功能默认关闭”。在修复管理端保存路径并验证 `GET /api/agent/features/v1` 返回值前，不能把资产页没有上传入口直接归因于默认关闭。

### 2.5 视频后台只有数据能力，没有完整管理体验

后端已有以下结构：

```text
ModelAccount
  -> ModelAccountModel
     -> VideoModelCapability
     -> VideoProviderCostRule

RouteModel(media_type=video)
  -> RouteModelCandidate
  -> VideoRouteConfig
     -> VideoPricingStrategy
        -> VideoPriceRule
```

后端支持 `seedance` 和 `minimax` 适配器，也支持多账号和单账号多模型。管理端却在三个页面底部复用 `VideoConfigurationWorkspace`，通过真实模型 ID、路由 ID 和 JSON 手工配置，未表达已有层级。

### 2.6 文本模型是独立领域

文本当前使用独立的 `TextModelAccount` 和 `TextModel` 数据模型，通过默认模型驱动提示词优化。它没有接入 `RouteModel`、候选模型和独立销售价格策略。因此本期只迁移管理入口，不能在 UI 中假装文本已经具有完整路由与价格能力。

### 2.7 视频任务空列表序列化为 null

后端 `TaskPage{}` 的 `Items` 为 nil slice，序列化后为 `null`。前端 `setRows(page.items)` 后访问 `rows.length`，直接崩溃。详情中的 `Items` 和 `Attempts` 也存在相同风险。

## 3. 总体设计

### 3.1 设计原则

1. `media_assets` 是平台所有可管理媒体文件的唯一资产实体。
2. `reference_assets` 是图片生成引用实体，不拥有重复的对象存储文件。
3. 上传队列是用户端全局服务，创作页、资产页和剪切板共用。
4. 预览接口按用途提供低成本资源，前端允许逐层降级。
5. 管理后台按媒体类型组织，但不强行统一尚未统一的运行时领域模型。
6. 列表 API 永远返回数组，不用 `null` 表达空集合。
7. 路由和历史数据迁移保持向后兼容。
8. 资产详情的媒体行为按类型明确分流，避免浏览器自动加载或播放不必要的原始资源。

### 3.2 目标数据流

```text
文件选择 / 拖放 / 剪切板
  -> MediaUploadProvider.enqueue
  -> media upload session
  -> 分片上传
  -> complete upload
  -> MediaAsset(source_type=local_upload)
  -> media processing job
  -> thumbnail / preview / poster / proxy
  -> 上传完成事件
       -> 资产页刷新
       -> 创作页创建 ReferenceAsset alias
       -> 编辑来源更新
       -> 可选：提示词光标插入资产 Token
```

## 4. 数据模型设计

### 4.1 ReferenceAsset 与 MediaAsset 关联

建议为 `reference_assets` 增加可空字段：

```text
media_asset_id UUID NULL
```

并建立：

- 外键：`reference_assets.media_asset_id -> media_assets.id`
- 普通索引：`media_asset_id`
- 活跃引用唯一约束：同一用户、同一媒体资产只有一个未删除引用别名
- 历史 `reference_assets` 允许 `media_asset_id IS NULL`

不建议立即设为 `NOT NULL`，原因是：

- 历史上传引用可能没有对应 `media_assets`；
- OpenAPI 引用上传仍可能在兼容期使用旧链路；
- 直接非空迁移会重现旧库升级失败风险。

### 4.2 引用对象所有权

通过统一资产创建的引用：

```text
reference_assets.owns_object = false
reference_assets.media_asset_id = media_assets.id
```

对象清理必须检查：

- 活跃 `media_assets`；
- 活跃 `reference_assets`；
- `media_asset_references`；
- 历史任务快照与恢复引用；
- 派生资源。

删除引用不能删除 MediaAsset 原图；删除 MediaAsset 时若仍被生成任务或活跃引用使用，应软删除资产视图并延迟对象清理，直到无活跃引用。

### 4.3 来源枚举

统一使用：

```text
generated
local_upload
canvas
imported
```

本期只要求本地上传明确写入 `local_upload`。兼容读取历史 `upload`、`uploaded` 等值，但新写入不得继续产生多套枚举。

## 5. 后端 API 设计

### 5.1 统一上传接口

保留现有接口：

```text
POST   /api/agent/media/v1/uploads
GET    /api/agent/media/v1/uploads/{id}
POST   /api/agent/media/v1/uploads/{id}/parts/{part}:sign
PUT    /api/agent/media/v1/uploads/{id}/parts/{part}
POST   /api/agent/media/v1/uploads/{id}:complete
DELETE /api/agent/media/v1/uploads/{id}
```

`complete` 继续返回 `MediaAsset`，同时创建 probe/派生任务。

上传接口的安全和幂等约束：

- 创建上传会话时校验用户、项目权限、文件大小上限和声明的媒体类型；`Content-Type` 只能作为提示，完成阶段必须由 worker 使用 magic bytes/ffprobe 等实际探测结果复核。
- 客户端为每个文件生成 `Idempotency-Key`，服务端在用户维度保存会话结果。相同 key 重试返回原会话/原资产，不重复创建对象或资产记录；key 过期时间至少覆盖断点恢复窗口。
- 分片号、分片大小、总大小和校验和必须服务端校验，禁止通过客户端参数越权写入其他上传会话；完成时校验所有分片已上传且顺序/大小完整。
- 取消、失败和过期会话由异步清理任务删除临时分片，清理任务必须幂等且不影响已完成资产。

### 5.2 从统一资产创建图片引用

新增推荐接口：

```http
POST /api/agent/image/v1/reference-assets:import-from-media
Content-Type: application/json

{
  "media_asset_ids": ["uuid-1", "uuid-2"]
}
```

响应：

```json
{
  "items": [
    {
      "id": "reference-uuid",
      "media_asset_id": "uuid-1",
      "name": "source.png",
      "status": "ready"
    }
  ]
}
```

服务端校验：

- 资产属于当前用户；
- `media_type=image`；
- 资产未删除；
- 原始对象可访问；
- MIME、尺寸和文件体积满足当前引用附件策略；
- 不校验资产所属项目，允许跨项目引用；
- 同一资产重复导入返回已有活跃引用，保证幂等；
- 不读取并重新写入对象存储内容。
- 访问和导入接口均按当前用户校验资产权限；项目仅用于组织，不作为跨项目引用的拦截条件。资产被软删除、病毒扫描失败或探测结果不合格时返回可区分的错误码，前端不得把它们显示成通用 404。

为支持提示词 Token，响应继续使用现有 `ReferenceAsset` 表示，并补充可选 `media_asset_id` 字段。

### 5.3 资产访问接口

保留：

```text
GET /api/agent/media/v1/assets/{id}/access?purpose=...
```

服务端用途规则：

| 媒体 | purpose | 首选资源 | 服务端降级 |
| --- | --- | --- | --- |
| 图片 | thumbnail | 640/320 缩略图 | preview 资源；必要时原图 |
| 图片 | preview | 1280 预览图 | 640/320；最后原图 |
| 图片 | download | 原图 | 无 |
| 视频 | poster | poster | 无，返回派生未就绪 |
| 视频 | hover | hover proxy | proxy |
| 视频 | preview | MP4 proxy | proxy 不可用时返回明确错误，不回退原视频 |
| 音频 | waveform | waveform | 无 |
| 音频 | preview | proxy | 支持 Range 的原图 |
| 文档 | content | 文档内容或文本代理 | 无 |

建议调整图片 thumbnail 服务端行为：只要原始图片对象可访问，即使缩略图未生成，也可以返回下一优先级。这样所有客户端都能获得基本预览；前端仍保留自身降级以兼容旧版本服务端和临时故障。`download` 用途只允许下载操作使用，不能作为列表或详情静态预览的请求用途。

所有访问投影必须再次校验资产归属、软删除状态和用途权限，并返回正确的 `Content-Type`、`Content-Length`、`Accept-Ranges`、短期缓存策略及 `ETag`。签名 URL 不能写入日志或长时间存储在前端全局状态。

### 5.4 资产详情预览与全屏控制器

新增共享 `AssetDetailPreview`（或同等职责组件），由资产详情页承载元数据和编辑操作，由预览控制器负责媒体状态及全屏生命周期。组件不得把完整签名 URL 写入全局资产列表状态，继续使用按用途获取访问投影的 `mediaAccess` 管理器。

#### 图片实现

- 详情内使用 `thumbnail -> preview` 的已有降级状态机作为静态预览；若服务端在 `preview` 用途中按策略返回原图，则由该响应完成原图降级。
- 点击图片后复用首页/创作页的全屏图片查看器，统一支持缩放、拖拽平移、适应窗口和关闭。
- 全屏打开时才获取更高分辨率的 `preview` 或原图访问地址；关闭后释放对象 URL、事件监听器和临时状态。

#### 视频实现

- 详情内只获取 `purpose=poster`，不设置 `autoplay`，不在列表或详情打开时预加载完整视频。
- 用户点击播放按钮后创建全屏播放器，只获取 `purpose=preview` 的 MP4 proxy，并设置 `controls`、`playsInline` 和必要的 Range 请求。P0 不向原视频回退；proxy 生成中或不可用时显示状态和重试入口。
- 播放器状态包括 loading、playing、paused、ended、error；退出全屏时暂停并保留当前时间，重新打开可继续从当前位置播放。
- 浏览器不支持 Fullscreen API 时退化为覆盖层预览，仍保留播放控制。

#### 音频实现

- 用户点击音频主体后才创建或激活 `<audio>`，使用 `purpose=preview` 的代理地址并依赖 HTTP Range。
- 详情展示播放/暂停、当前时间、总时长和可拖拽进度条；切换资产、关闭详情或组件卸载时暂停并释放播放器。
- 多个详情实例共享一个播放器控制器，保证同一时间最多播放一个音频。

#### 文档实现

- 对文本、Markdown 等已支持文档类型请求 `purpose=content`，详情打开后自动加载内容。服务端限制单次响应最大字节数并支持超限提示，避免大文档占满浏览器内存；超过阈值时提供下载而不是强制渲染。
- Markdown 使用已有安全渲染库或统一 sanitizer：允许标题、段落、列表、代码块和安全链接，禁止脚本、事件属性、任意 HTML、外部资源标签和 `javascript:`、`data:` 等不安全协议。渲染前后均不得信任文件扩展名，按资产记录的探测类型选择纯文本或 Markdown 渲染。
- 全屏阅读使用覆盖层或 Fullscreen API，保留关闭、重试和复制文本能力；大文档采用流式/分段渲染，避免一次性阻塞详情页。

#### 不支持类型

- 通过后端媒体类型白名单和前端能力映射判断是否可预览。未知类型只展示占位、元数据和下载按钮，不尝试猜测 MIME 类型或复用其他播放器。

#### 访问与成本控制

- 详情静态预览延续缩略图优先策略；全屏图片、视频播放和文档内容只在用户明确操作或详情打开时获取。
- 视频统一使用 MP4 proxy 作为 P0 播放源；播放器支持 Range、断点和错误重试，避免重复下载。
- 音频和视频组件在关闭、切换和卸载时取消请求并清理媒体元素，防止后台继续消耗带宽。
- 全屏交互需实现焦点陷阱、Esc 退出、关闭后焦点回收和不可用 API 的覆盖层降级；播放器应设置 `preload="none"`，仅在用户点击后切换为可播放状态。

### 5.5 空列表契约

所有列表响应必须使用非 nil slice：

```go
page := TaskPage{Items: make([]TaskSummary, 0)}
```

至少修复：

- `AdminVideoTaskPage.items`
- `AdminVideoTaskDetail.items`
- `AdminVideoTaskItem.attempts`

前端 `adminApi` 增加归一化：

```ts
function normalizeAdminVideoTaskPage(raw: AdminVideoTaskPage) {
  return {
    ...raw,
    items: Array.isArray(raw?.items) ? raw.items : [],
  }
}
```

详情同样归一化嵌套数组。

## 6. 用户端架构设计

### 6.1 全局 MediaUploadProvider

将 `UploadTray` 中的任务状态和上传动作抽取为 Context：

```ts
type EnqueueOptions = {
  projectID: string
  groupName?: string
  completionMode?: 'asset-only' | 'image-reference'
  onCompleted?: (result: {
    asset: MediaAsset
    reference?: ReferenceAsset
  }) => void
}

type MediaUploadContextValue = {
  items: UploadSnapshot[]
  enqueue: (files: File[], options: EnqueueOptions) => Promise<string[]>
  pause: (localID: string) => void
  resume: (localID: string) => void
  cancel: (localID: string) => Promise<void>
  retry: (localID: string) => void
  openPicker: (options?: Partial<EnqueueOptions>) => void
  openTray: () => void
}
```

Provider 放在 `ProjectProvider` 内、各页面外，使其可读取当前项目并跨页面保持状态。

`UploadTray` 只负责渲染，不再拥有业务状态。

### 6.2 创作页接入

替换 `WorkspacePage.uploadReferenceFiles` 的旧上传调用：

1. 使用现有 `validateReferenceImageFile` 过滤。
2. 使用 `limitReferenceSelection` 应用模型引用上限。
3. 调用 `uploadQueue.enqueue(files, { completionMode: 'image-reference' })`。
4. 单项完成后合并到 `editRefs`。
5. 若来源是提示词粘贴，在对应编辑器光标插入 Token。

紧凑和展开编辑器的回调必须记录来源编辑器，不能上传完成后把 Token 插入另一个编辑器。

### 6.3 剪切板插件

为 `PromptTemplateEditor` 增加 `onPasteImages?: (files: File[]) => void`。

新建 Lexical 插件监听根元素 paste：

```text
ClipboardEvent
  -> clipboardData.items
  -> kind=file 且 MIME 以 image/ 开头
  -> Blob/File 标准化
  -> preventDefault
  -> onPasteImages(files)
```

文件名生成：

```text
clipboard-YYYYMMDD-HHmmss-1.png
```

若浏览器已经提供文件名则保留。非图片剪切板不调用 `preventDefault`。

上传是异步的，必须保存 Lexical Range/Selection 快照或使用待插入队列，避免用户继续输入后 Token 插入错误位置。推荐插入临时不可提交 Token，上传成功后替换为正式资产 Token，失败后替换为错误提示并允许移除。

### 6.4 预览状态机

抽取 `useMediaAssetPreview`：

```text
idle
 -> thumbnail_loading
 -> ready
 -> preview_loading
 -> original_loading
 -> failed
```

图片预览用途序列：

```ts
const previewPurposes = ['thumbnail', 'preview'] as const
```

预览状态机不得自动请求 `purpose=download`。原图只能作为服务端在 `purpose=preview` 下按策略返回的降级资源；`purpose=download` 仅用于用户明确点击下载时的独立操作，不能被资产卡片或详情静态预览复用。

每一级：

- 获取访问投影；
- `<img>` 加载失败时刷新一次访问投影；
- 再失败则进入下一层；
- 组件卸载后取消更新；
- `asset.id/status/version` 变化时重置状态机。

不要把签名 URL 长期写入资产列表状态；只在卡片内部或统一 `mediaAccess` 管理器缓存到过期时间。

### 6.5 功能开关管理

- 保留 `media_upload`、`creative_canvas`、`video_creation` 作为 `site` Tab 下 `features` 分类的站点级开关。
- 本期不改变当前默认值。是否开启由管理员配置决定，升级初始化不得覆盖已有配置，也不要求发布时额外执行数据库改动。
- 管理端固定调用 `/api/ops/admin/v1/config-tabs/site`，不能把 `features` 当作 URL 中的 Tab key。
- 保存前读取当前 `site.version`；遇到 409 版本冲突时重新拉取配置并提示用户确认后重试。
- 开启后验证：`GET /api/agent/features/v1` 返回 `media_upload=true`；资产页入口显示；`UploadTray` 挂载；上传初始化接口可用。
- 关闭时前端隐藏入口，后端继续作为最终权限边界拒绝新建上传。

## 7. 管理端架构设计

### 7.1 统一媒体类型 Tab

新增共享类型：

```ts
type AdminModelMediaTab = 'image' | 'video' | 'audio' | 'text'
```

三个页面复用相同 Tab 顺序和 URL 查询参数：

```text
#/access-accounts?media=image
#/routing?media=video
#/pricing?media=text
```

未知值回退到 `image`。Tab 状态写入 URL，刷新后保持。

### 7.2 接入账号页面组件拆分

```text
ProviderModelsPage
  -> AdminMediaTabs
  -> ImageProviderAccountsPanel
  -> VideoProviderAccountsPanel
  -> AudioProviderPlaceholder
  -> TextProviderAccountsPanel
```

视频面板复用图片页面的主从布局和公共组件：

- `FilterToolbar`
- `DataTable`
- `ActionMenu`
- `Drawer`
- `Modal`
- `Badge`
- `RefreshIconButton`

视频账号仍使用 `ModelAccount` API，筛选适配器为 `seedance|minimax`。图片筛选 `openai_compatible|openrouter`。

视频真实模型仍使用 `ModelAccountModel`，但编辑表单根据媒体类别展示不同字段。视频专属能力和成本通过现有视频配置 API 保存，不新建重复账号表。

建议新增聚合 ViewModel：

```ts
type VideoAccountModelView = {
  model: ModelAccountModel
  capability?: AdminVideoCapability
  latestCostRule?: AdminVideoCostRule
}
```

### 7.3 视频能力结构化表单

主要字段结构化：

- 模型代码、显示名称；
- 任务类型；
- 分辨率；
- 比例；
- 时长；
- 音频模式；
- 首帧、尾帧、参考素材；
- 最大参考数；
- Provider 原生最大 n（1-10）；
- 是否启用；
- 能力版本。

厂商特殊字段保存在 `extra` 或 capability JSON 的扩展区。结构化字段和 JSON 必须通过一个转换层生成请求，不能让两个表单分别写入并互相覆盖。

### 7.4 路由页面拆分

图片继续使用现有 `RoutingPage` 逻辑。视频面板使用：

```text
VideoRouteList
  -> VideoRouteDetail
     -> CandidateTable
     -> VisibleCombinationEditor
     -> DefaultsEditor
     -> PricingStrategyBinding
```

`VideoRouteDetail` 根据选中的 RouteModel ID 自动加载 `VideoRouteConfig`，管理员不再手填路由 ID。

候选模型只显示视频账号下具备有效视频能力的模型。保存前仍由服务端校验：能力、成本、价格、安全线和可见组合是否完整。

文本 Tab 本期复用 `TextModelDefaultReadiness`，展示默认模型及其账号，不提供虚假的候选路由编辑。

### 7.5 价格页面拆分

```text
PricingPage
  -> ImagePricingPanel
  -> VideoPricingPanel
     -> StrategyList
     -> StrategyEditor
     -> PriceRuleList
     -> PricingSimulation
  -> AudioPricingPlaceholder
  -> TextModelCostPanel
```

视频策略、规则和试算 API 保持不变。由选中策略和路由自动带入 ID，避免自由文本 ID 输入。

文本面板复用现有 `TextModel` 的输入/输出百万 Token 成本编辑能力，并明确这是模型成本配置，不是完整销售价格策略。

### 7.6 文本模型迁移

- 将 `TextModelsPage` 重构为可嵌入面板，分别供“接入账号/文本”和“价格策略/文本”复用。
- 从 `SystemSettingsPage.tabItems` 删除 `text-models`。
- 旧 `#/system-settings?tab=text-models` 跳转到 `#/access-accounts?media=text`。
- 权限使用 `manage:models`；敏感凭据编辑可继续要求 `manage:dangerous_config`，避免迁移页面后权限扩大。

### 7.7 媒体策略迁移

为 `MediaPolicyPage` 增加嵌入模式参数：

```ts
type MediaPolicyPageProps = {
  compact?: boolean
  refreshGeneration?: number
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}
```

系统设置新增 `media-policy` Tab，顺序紧跟 `attachment-policy`。将其接入系统设置的脏状态、忙状态和刷新代次。

导航处理：

- 从 `navGroups` 移除独立菜单；
- route ID 暂时保留；
- `App.tsx` 遇到 `media-policy` 时执行兼容跳转；
- 权限映射继续保留 `manage:config`。

### 7.8 图片任务改名

不改变 `call-records` route ID 和 API。仅统一修改：

- 导航名称；
- route title；
- 页面标题；
- 加载、刷新、错误和空状态文案；
- 相关前端契约测试。

## 8. 数据迁移与兼容

### 8.1 Schema 迁移顺序

1. 新增可空 `reference_assets.media_asset_id`。
2. 新增外键和索引；外键采用 `ON DELETE SET NULL` 或等价的应用层软删除语义，不能因删除媒体资产阻断历史引用读取。
3. 不执行全表强制回填，不增加 NOT NULL。
4. 新上传流程开始写入关联。
5. 后续启动时幂等任务可根据对象身份、来源图片 ID 和用户归属回填可确定的数据。

### 8.2 回填规则

只有满足以下条件才自动关联：

- user_id 相同；
- storage_config_id、storage_driver、bucket、object_key 完全一致；
- 或 `source_image_result_id == media_assets.legacy_image_id`；
- 匹配结果唯一；
- 两端均未删除。

无法唯一判断的记录保持 null，不猜测关联。

迁移和回填必须具备：

- 批次/游标进度记录，启动重试不会重复创建关联；
- 单批次限流和超时，避免阻塞 API；
- 回填失败记录可观测但不影响主服务启动；
- 回填完成后再次执行结果不发生变化（幂等）。

### 8.3 旧客户端兼容

- 旧的 reference upload API 保留至少一个版本周期。
- 旧客户端上传的引用仍可用于生成，但不会自动成为统一资产，除非服务端在兼容 handler 内同步创建 MediaAsset。
- 推荐兼容 handler 内部逐步改为调用统一上传服务，避免继续产生孤立数据。

## 9. 可观测性

新增结构化指标或日志：

- `media_upload_total{source=asset_page|workspace|clipboard,result}`
- `media_reference_import_total{result}`
- `media_preview_access_total{purpose,result}`
- `media_preview_fallback_total{from,to}`
- `media_detail_preview_open_total{media_type,result}`
- `media_detail_playback_total{media_type,result}`
- `media_derivative_not_ready_total{media_type,purpose}`
- `admin_video_task_list_normalized_total`

日志上下文至少包括：

- request_id；
- user_id；
- asset_id；
- upload_id；
- project_id；
- preview purpose；
- derivative status；
- fallback level。

不得记录 API Key、签名 URL 完整查询参数或用户文件内容。

## 10. 实施拆分

### 阶段一：稳定性和入口

1. 修复视频任务空列表契约。
2. 完成图片任务改名。
3. 媒体策略迁移至系统设置。
4. 修复 `media_upload` 管理端保存路径与资产页入口状态同步。

### 阶段二：统一上传与引用

1. 增加 MediaAsset/ReferenceAsset 关联字段和服务。
2. 新增 import-from-media API。
3. 抽取 MediaUploadProvider。
4. 创作页接入统一上传。
5. 提示词剪切板图片插件。
6. 上传完成事件和资产列表同步。

### 阶段三：预览可靠性

1. 后端图片访问降级。
2. 前端预览状态机。
3. URL 过期刷新。
4. 资产详情统一预览控制器和图片全屏查看器复用。
5. 视频 poster/MP4 proxy 按用户操作加载，音频进度播放和文档安全渲染。
6. 历史数据和派生失败场景验证。

### 阶段四：管理后台模型配置重构

1. 三个页面增加统一媒体 Tab。
2. 视频账号和模型主从管理。
3. 视频路由列表与详情。
4. 视频价格策略与规则列表。
5. 文本模型入口迁移。
6. 音频占位页面。

## 11. 测试设计

### 11.1 Go 单元与仓储测试

- 统一上传完成创建 `source_type=local_upload` 的 MediaAsset。
- import-from-media 不发生对象存储读写和复制。
- 上传声明为图片但内容为非图片、超过 1 GiB 或不在媒体白名单时被拒绝；重复 `Idempotency-Key` 不产生重复会话/资产。
- 重复导入返回同一活跃 ReferenceAsset。
- 跨项目资产可以创建引用。
- 其他用户资产返回 404。
- 删除引用不删除 MediaAsset 对象。
- 删除 MediaAsset 时活跃引用阻止对象清理。
- thumbnail 缺失时按设计回退。
- `purpose=download` 不能被预览组件调用；视频 `purpose=preview` 只返回 MP4 proxy，proxy 不可用时返回明确错误。
- 资产详情按媒体类型返回正确预览能力，不支持类型不触发错误的播放器请求。
- 视频任务空列表 JSON 为 `[]`。
- 视频详情嵌套列表为空数组。
- 历史 nullable 关联迁移成功。
- 配置 Tab 更新使用 `site` 路径并能保存三个 feature flags。
- 错误路径 `/config-tabs/features` 返回 404，且管理端不会生成该请求。
- 保存后 features 查询与前端入口状态一致；显式关闭时入口隐藏、上传初始化被拒绝。
- `site.version` 更新后再次保存使用最新版本，冲突时有明确提示。

### 11.2 用户端契约与组件测试

- 上传 Provider 的队列状态、并发、暂停、恢复和完成回调。
- 资产页批量文件加入队列。
- 工作区上传完成合并引用并广播资产变化。
- 粘贴图片阻止默认行为，粘贴文本不阻止。
- 多图片受引用数量上限约束。
- 紧凑和展开编辑器正确插入 Token。
- 图片预览按 thumbnail、preview 顺序降级；preview 用途可由服务端按策略返回原图，download 仅用于用户显式下载。
- URL 加载失败只刷新一次，避免死循环。
- 页面刷新后重新获取访问投影。
- 图片详情点击后复用全屏查看器，缩放、拖拽、关闭状态正确。
- 视频详情初始不请求播放资源；点击后请求 poster/MP4 proxy 并支持播放、暂停、进度拖拽和退出全屏。
- 音频详情点击后才创建播放器，播放进度可拖拽，切换详情后停止上一个音频。
- Markdown 文档自动加载并安全渲染，脚本和不安全链接被过滤；全屏阅读可打开和关闭。
- 文档超出内容大小阈值时返回可识别错误/下载建议；访问投影拒绝其他用户、软删除资产和过期签名请求。
- 未知媒体类型只显示元数据和下载入口，不尝试图片、视频或音频预览。

### 11.3 管理端契约与组件测试

- 媒体 Tab URL 状态和未知值回退。
- 视频账号筛选 Seedance/MiniMax。
- 单账号多模型展示和编辑。
- 视频路由候选选择器只展示有效视频模型。
- 视频价格列表、试算、启停和删除。
- 文本配置迁移后权限不扩大。
- 空视频任务页面不崩溃。
- 旧媒体策略地址跳转。
- `call-records` 地址仍可访问且显示“图片任务”。

### 11.4 集成与 smoke

完整执行：

```bash
./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh
./scripts/workflow/api-smoke.sh
```

新增真实 API smoke 场景：

1. 创建项目并批量上传两张图片。
2. 完成上传并轮询派生状态。
3. 从 MediaAsset 创建 ReferenceAsset。
4. 使用引用创建图片任务。
5. 查询资产列表并验证本地上传来源。
6. 请求 thumbnail；删除派生记录的测试场景验证 fallback。
7. 查询空视频任务列表并断言 `items=[]`。

## 12. 风险与控制

### 12.1 对象误删风险

统一资产和引用共享对象后，对象清理必须基于完整引用集合判断，并遵循外键 `ON DELETE SET NULL`/软删除语义。发布前需重点 review object cleanup，并增加共享对象删除、历史引用读取和异步重试测试。

### 12.2 大文件内存风险

创作页不得继续使用旧的整文件 multipart 内存上传承载大文件。统一使用现有分片上传；剪切板图片仍需经过同一限制和队列。

### 12.3 功能开关配置路径与误判风险

Tab key 与配置分类混淆会导致管理端收到 404，进而让运营人员误以为开关默认关闭或功能尚未发布。实现和验收必须同时检查请求路径、`site.version`、保存结果及 features 查询结果；不能用升级初始化强行覆盖管理员配置。后端仍需保留开关校验，避免仅依赖前端隐藏入口。

### 12.4 后台重构误改运行时风险

页面重构只改变管理入口和请求组织，不修改视频路由候选选择算法。若后续统一视频权重和故障回退，应建立独立需求和技术方案。

### 12.5 文本领域过度抽象风险

文本当前只服务提示词优化。本期仅迁移入口，不能为了视觉统一而创建无运行意义的文本路由和销售价格记录。

### 12.6 媒体详情预览资源与安全风险

视频和音频若在详情打开时自动加载或未正确释放媒体元素，会造成不必要的带宽消耗和后台播放；必须遵循用户操作后加载、Range 请求、切换时暂停和卸载时清理。Markdown 内容若直接透传 HTML 可能引入脚本或恶意链接，必须使用白名单渲染和 sanitizer，并补充恶意内容测试。

### 12.7 上传与访问安全风险

客户端扩展名和 MIME 均可伪造，必须以服务端探测结果和白名单作为最终判断；分片上传、完成接口和访问投影均需校验用户/项目权限、会话归属和资源状态。签名地址应短期有效并设置合理缓存，不能把完整 URL 写入日志。上传会话过期、重复完成和网络重试必须可恢复且幂等，临时分片需要异步清理，避免对象存储泄漏。

## 13. 完成定义

满足以下条件才可认为本需求完成：

- PRD 中所有本期验收标准通过；
- 统一上传不产生重复对象；
- 资产预览在无缩略图时可用；
- 视频任务空数据不崩溃；
- 管理后台四类 Tab 和视频主从配置可用；
- 历史数据与旧 URL 兼容；
- 完整 verify、review gate 和 API smoke 通过；
- 代码评审未发现阻断问题。
