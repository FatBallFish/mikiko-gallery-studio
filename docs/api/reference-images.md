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
3. 完成上传，保存资产 ID。
4. 中断后先查询上传状态，失效链接重新获取。

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
