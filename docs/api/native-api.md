# 原生图片与视频 API

## 任务流程

原生异步任务适合需要保存 ID、断线恢复和结果管理的生成流程。`MIKIKO_API_KEY` 使用一次显示的 SK（`sk-`）。

| 步骤 | 图片 | 视频 |
| --- | --- | --- |
| 发现模型 | `GET https://mikiko.studio/api/open/v1/image-models` | `GET https://mikiko.studio/api/open/v1/video-models` |
| 预检 | `POST https://mikiko.studio/api/open/v1/image-tasks/preflight` | `POST https://mikiko.studio/api/open/v1/video-tasks/preflight` |
| 创建 | `POST https://mikiko.studio/api/open/v1/image-tasks` | `POST https://mikiko.studio/api/open/v1/video-tasks` |
| 列表 | `GET https://mikiko.studio/api/open/v1/image-tasks` | `GET https://mikiko.studio/api/open/v1/video-tasks` |

先选自己的项目和可用模型，再发送完整预检请求。保留相同业务输入，加入报价和幂等键创建任务。具体示例见 [SDK 与任务流程](sdk-examples.md)。

## Qwen / Seedream 原生图片契约 {#native-image-contract}

读取模型的 `native_image_profile`。`operation` 可为 `generate`、`edit` 或 `layer_decomposition`；实际执行还取决于路由、权限和已确认价格。

下面是需合入完整预检请求的字段片段：

```json
{
  "operation": "edit",
  "native_options": {
    "watermark": false,
    "annotations": [{ "reference_index": 1, "label": "目标", "point": [520, 460] }],
    "seedream": { "optimize_prompt_mode": "standard" }
  }
}
```

- `native_options` 顶层支持 `seed`、`negative_prompt`、`watermark`、`annotations`、`qwen`、`seedream`。
- Qwen 的 `prompt_extend`、`prompt_extend_mode`、`enable_thinking` 放入 `qwen`。
- Seedream 的 `optimize_prompt_mode` 放入 `seedream`。
- `reference_index` 从 1 开始，`point` 与 `bbox` 二选一，输入坐标范围为 0..999。

### 结果图层

`results[].layer` 包含 `z_index`、`is_base` 和可选的 `name`、`description`、`bounding_box`。

- `bounding_box.absolute` 是像素坐标。
- 输出 `normalized` 范围为 0..1000，与输入标注的 0..999 不同。
- 保存底图、透明 PNG 和层次顺序。

## 原生图片支持表 {#native-support}

| 型号 | 参考图 / 普通输出 | 专属能力 |
| --- | --- | --- |
| Qwen 3.0 / 可选 3.0-pro | 最多 3 / 1–6 | seed、negative_prompt、提示词扩展/思考、PNG |
| Qwen 2.1-pro / 2.1-turbo | 最多 10 / 1–6 | seed、提示词扩展/思考；提示词驱动透明输出 |
| Seedream 5.0 pro | 最多 10 / 1 | 坐标标注；优化 standard/fast；拆层需确认价格 |
| Seedream 5.0 flash | 最多 10 / 1 | 坐标标注；优化 standard；拆层需确认价格 |

- Qwen：262144–4194304 像素，宽高比至 8:1。
- Seedream：921600–4624220 像素，宽高比至 16:1，JPEG/PNG；不支持 seed/negative_prompt。
- 拆层使用一张参考图，输出一张底图和最多 16 个透明层。普通数量与拆层数量独立，不能用 `n=17` 请求拆层。
- 模型声明拆层能力不代表当前路由已启用；以发现响应和预检为准。

## 管理员配置与价格确认 {#native-setup}

原生 adapter 为 `qwen_image` 和 `seedream_image`。供应商默认端点分别为 `https://dashscope.aliyuncs.com` 与 `https://ark.cn-beijing.volces.com/api/v3`；这是上游地址，开发者客户端仍调用 Mikiko。

账号模型的 `extra.native_image` 包含：

```text
model, profile?, pricing, sale_pricing
```

- `model` 填上游 Endpoint 对应的真实型号，`profile` 与能力保持一致。
- `pricing` 是成本 CNY，`sale_pricing` 是销售 CNY，显式配置后启用路由。
- 拆层成本和售价均需 `layer_base_cny`、`layer_tiers` 及 `layer_pricing_confirmed=true`。
- 未确认价格拒绝执行；确认零价也须显式记录。默认牌价不会自动应用到线上。
- CNY 与积分分开，预检绑定能力和价格版本。

## 图层清单与下载 {#layer-download}

API Key 使用 `GET https://mikiko.studio/api/open/v1/image-tasks/{id}/layers`，MCP 使用 `get_image_task_layers({id})`。

每层包含 `access_operation=get_image_result_access` 和以下输入，用于续取下载链接：

```json
{
  "task_id": "TASK_ID",
  "image_id": "IMAGE_ID",
  "purpose": "download"
}
```

浏览器专用 `GET /api/agent/image/v1/tasks/{task_id}/layers` 和 `/layers/archive` 使用浏览器会话。ZIP 包含固定 `layer-NN` 文件和 `manifest.json`，最多 17 项、图片总量不超过 128 MiB。浏览器 ZIP 地址不能当作 OAuth 下载链接。

保存任务 ID。客户端断线后，已经持久化的任务仍可继续查询。
