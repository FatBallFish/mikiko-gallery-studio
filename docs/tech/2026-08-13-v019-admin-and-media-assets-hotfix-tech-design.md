# v0.0.19 管理后台与统一资产热修复技术方案

## 1. 根因

### 1.1 管理后台白屏

`adminvideo.Snapshot` 的 slice 零值被编码为 `null`，而 `VideoConfigurationImpact` 按共享类型契约直接调用 `snapshot.impacts.filter` 及其他集合的 `.length`。空视频配置环境因此触发运行时异常。问题存在于服务输出契约与前端运行时边界两层，采用双层兼容。

### 1.2 历史图片不展示预览

`MediaAssetCard` 对图片固定请求 `purpose=thumbnail`。历史图片统一资产回填只复用既有图片对象并写入 `legacy_image_id`，不创建 `media_derivatives`；`mediaasset.Service.assetObject` 在找不到缩略图时返回 `DERIVATIVE_NOT_READY`，前端吞掉错误并展示占位符。

### 1.3 框选与悬浮回归

统一资产页重构时只迁移了逐项选择状态，没有迁移旧 Gallery 的 pointer 框选状态机。批量工具条虽然声明 fixed，但仍挂在页面内容树中；顶层 portal 是项目中既有的浮层模式，可避免祖先 containing block、层叠上下文和滚动容器影响。

## 2. 设计

### 2.1 视频快照集合规范化

- 在 `adminvideo.Service.Snapshot` 返回前调用集中规范化函数，为七个集合字段分配非 nil 空 slice。
- `deriveImpacts` 之后再规范化，保证最终返回值的 JSON 契约。
- 在 `web/shared/admin-api.ts` 增加纯函数适配器，逐项用 `Array.isArray` 判断；非数组值规范化为 `[]`，其余标量字段保留。
- `getVideoConfiguration` 请求完成后必须经过适配器，覆盖接入账号、价格、路由和视频工作区全部调用方。

### 2.2 历史图片预览回退

- 保持派生优先级不变：图片 `thumbnail640 -> thumbnail320`。
- 仅当 `media_type=image`、访问目的为 `thumbnail` 且 `legacy_image_id != nil` 时，在无可用派生后回退到资产原对象。
- 该回退使用现有短期签名/受控 content endpoint，保持私有访问、鉴权与无公开 URL 契约。
- 非历史图片、视频 poster/hover、音频 waveform 缺失时继续返回 `DERIVATIVE_NOT_READY`，不通过原件掩盖媒体处理失败。

### 2.3 统一资产框选状态机

- 把矩形规范化、相交判断和命中选择计算抽成无 DOM 的纯函数，便于用 Node contract test 覆盖。
- 页面持有 `selectionSurfaceRef`、起点、当前矩形、起始选择集合和 `suppressOpen` 标记。
- `pointerdown` 仅接受鼠标主键，允许从资产预览主命中区或网格背景开始，但排除选择/播放/重试等独立卡片控件；由原始按下目标立即捕获 pointer，避免从网格边缘拖出后丢失 move/up，同时不重定向普通卡片 click。
- 移动超过阈值后才进入框选，按每张卡片的 `data-media-asset-id` 与 `getBoundingClientRect` 计算相交项。
- 默认用命中集合替换起始选择；Command/Ctrl/Shift 使用并集。
- `pointerup/cancel/lostpointercapture` 清理状态并结束；有效拖拽后的合成 click 在短时间窗口内被抑制。
- 选区通过 portal 渲染为 `position: fixed; pointer-events: none` 的矩形。

### 2.4 批量工具条

- 使用现有 `OverlayPortal` 将工具条挂至顶层 overlay root / `document.body`。
- 保留 fixed 定位，增加安全区下边距、视口宽度约束和窄屏横向滚动。
- 工具条层级低于模态框和上传托盘的关键交互层，高于普通页面和选择模式按钮。

## 3. 测试方案

- Go：空 store 快照经 Service 后所有 slice 非 nil；JSON 解码后对应值全部为数组；历史图片 thumbnail 无派生时回退原对象，普通图片仍报派生未就绪。
- 前端契约：旧响应中所有集合为 `null` 时适配为 `[]`。
- 框选纯函数：任意方向矩形规范化、相交边界、替换选择、修饰键追加选择。
- 静态视觉契约：资产页具有 pointer 事件、卡片身份属性、portal 工具条、固定选区样式；卡片仍请求 thumbnail/poster/waveform。
- 最终运行 `verify.sh`、API smoke、管理后台和用户端浏览器验收、committed-scope review gate。

## 4. 风险控制与回滚

- 原图回退严格绑定 `legacy_image_id`，避免新资产绕过派生处理。
- 框选仅处理桌面鼠标且过滤交互控件，避免影响触屏、筛选和批量按钮。
- 前端规范化不伪造业务记录，只修复集合形态。
- 回滚可按前后端独立提交反向恢复，不涉及 schema 或数据迁移。
