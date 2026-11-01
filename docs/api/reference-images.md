# 资产、上传与分组

## 常用接口

| 操作 | 地址 |
| --- | --- |
| 查询资产 | `GET https://mikiko.studio/api/open/v1/assets` |
| 创建上传 | `POST https://mikiko.studio/api/open/v1/uploads` |
| 查询上传状态 | `GET https://mikiko.studio/api/open/v1/uploads/{upload_id}` |
| 查询分组 | `GET https://mikiko.studio/api/open/v1/groups` |
| 查询项目 | `GET https://mikiko.studio/api/open/v1/projects` |

## 上传与访问 {#upload}

1. 初始化上传会话。
2. 按响应给出的分片大小、校验和及目标上传文件；目标 URL 可能属于存储服务，按响应使用。
3. 完成上传，保存返回的统一资产 ID。完成上传不代表媒体处理完成。
4. 通过 `GET /api/open/v1/assets/{asset_id}`（MCP：`get_asset`）轮询，直到 `status` 为 `ready` 或 `ready_original`。`processing` 时等待，`failed` 时重新上传；已删除、类型不匹配及其他用户的资产不可引用。
5. 中断后先查询上传状态，失效链接重新获取。

这套上传服务与画布、普通工作台共用。资产会出现在 `list_assets` 中；同一个 ID 可直接用于图片编辑的 `reference_asset_ids`、命名绑定的 `asset_id`、模型支持的 `mask_asset_id`，以及视频的 `inputs[].asset_id`。无需创建或转换参考图 alias；历史参考 ID 由服务端兼容解析。

任务和 MCP 工具传递资产 ID，不把大型 base64 放入工具输入。任意远程 URL 导入不属于已支持流程。

访问链接会过期。使用 `POST https://mikiko.studio/api/open/v1/assets/{asset_id}/access` 获取新链接，长期保存资产 ID。

## 任务结果访问

- 图片：`POST https://mikiko.studio/api/open/v1/image-tasks/{task_id}/results/{image_id}/access`，需要 `images:read`。
- 视频：`POST https://mikiko.studio/api/open/v1/video-tasks/{task_id}/results/{asset_id}/access`，需要 `videos:read`。

这两项操作无需授予整个资产库的读取权限。

## 持久分组 {#groups}

- 分组属于用户和项目，资产最多属于一个分组。
- 删除分组只清除资产的分组关系。
- 转移项目时会解析目标项目中对应名称的分组。
- 资产分组、模型分组与密钥计费分组含义不同。
