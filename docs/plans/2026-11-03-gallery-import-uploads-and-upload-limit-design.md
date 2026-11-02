# 创作台资产导入与上传限制统一设计

日期：2026-11-03
需求：`docs/prd/2026-11-03-gallery-import-uploads-and-upload-limit-requirements.md`

## 问题 1：从资产导入缺少上传图片

根因：弹窗数据源 `listGalleryImages` → `ListGalleryByUser` 只查询 `ImageResult`（生成结果图）；上传图片位于统一媒体资产表，永远不出现在弹窗。统一改造只统一了上传与存储链路，未改弹窗数据源。

设计（纯前端，复用既有后端端点）：

- `workspaceGalleryImport.ts` 引入 `GalleryImportItem = GalleryImage & { import_kind }`：
  - 生成图经 `generatedImageAsGalleryItem` 标记；上传资产经 `uploadedAssetAsGalleryItem` 由 `MediaAsset` 投影（`prompt`/`name` ← 资产名，`image_group` ← `group_name`，含 `mime_type`/`file_size_bytes` 供型号校验）。
  - 过滤规则：上传项不要求预置 URL（缩略图懒加载）；来源筛选 `(filter.source === 'uploaded') === uploaded`；其余过滤字段沿用原语义（上传项模型/比例为空，仅在"全部"下出现）。
- `WorkspacePage.openGalleryImport` 并行拉取 `listGalleryImages` 与 `listMediaAssets({ media_type: 'image', source_type: 'local_upload', project_id, limit: 100 })`，上传列表在前合并展示。`source_type=local_upload` 避免生成图重复（生成图的媒体行 `source_type=generated`，ID 相同）。
- 上传项缩略图用 `GalleryUploadThumb` 懒加载 `getMediaAssetAccess(thumbnail)`，失败回退 `preview`，临期由 `RefreshableMediaImage` 触发刷新——与资产页卡片模式一致。
- `confirmGalleryImport` 用 `splitGalleryImportSelection` 按来源拆分：上传资产沿用统一媒体语义，经 `getReadyMediaReference`（等待 ready 后直接投影，**不创建 reference alias**，与 `workspaceUnifiedUpload.contract.ts` 的既有约束一致）；生成图走 `importReferenceAssetsFromGallery`（携带 generation_snapshot）。两类结果统一进入现有 `admitReferences` 准入与预留流程。

## 问题 2：上传大小限制统一为 min(平台策略, 上游硬限制)

根因：限制分散在三处——平台附件策略（可配置，默认 20MB，约束上传入口）、native 参考图前端硬编码（`native-image.ts`）、native 参考图后端硬编码（`nativeimage.ValidateReference`）。

设计：

- 前端 `native-image.ts`：`nativeImageReferencePolicy(profile, operation?, platformMaxBytes = 20MB)` 的 `maxBytes = min(platformMaxBytes, 上游限制)`；上游限制以 `qwen_image: 10MB` 显式成表，其余（seedream 系）30MB。工作台与画布的各调用点透传 capability 的 `reference_image_max_bytes`；拿不到时用默认 20MB，与上传入口一致。
- 后端 `internal/domain/nativeimage/reference.go`：上游限制提取为 `QwenReferenceMaxBytes`/`SeedreamReferenceMaxBytes` 与 `ReferenceMaxBytes(adapter)`（未知 adapter 取最严 10MB）。`ValidateReference` 保持上游硬限制兜底，行为不变；平台策略仍由上传端点强制。
- 效果：万象 10MB（上游事实）、即梦由 30MB 展示收敛为平台值 20MB（上传入口本就 ≤20MB，展示与实际一致）、GPT/统一上传 20MB，单一配置源为管理后台"附件策略"。
- 视频参考图不受影响：上传走统一媒体上传（平台策略），Wan 上游 20MB/100MB 由 provider 客户端兜底。

## 测试

- `workspaceGalleryImport.contract.ts`：上传项合并展示、来源筛选、名称/标签搜索、选择拆分。
- `native-image.contract.ts`：min() 语义（默认平台值、调高不超上游、调低生效）。
- `internal/domain/nativeimage/boundaries_test.go`：`ReferenceMaxBytes` 与既有边界用例。
