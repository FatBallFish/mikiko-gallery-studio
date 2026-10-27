# SDK 与可运行示例

`MIKIKO_API_KEY` 填创建或轮换时一次显示的 SK（`sk-`），不是 AK（`ak-`）。将它注入服务端环境变量，不写进代码。

## Node.js 18+ {#node}

保存为 `models.mjs`，运行 `node models.mjs`。此请求只读取模型，不创建收费任务。

```javascript
const key = process.env.MIKIKO_API_KEY
if (!key) throw new Error('请设置 MIKIKO_API_KEY（一次显示的 SK）')
const response = await fetch('https://mikiko.studio/api/open/v1/image-models', {
  headers: { Authorization: `Bearer ${key}` },
})
if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`)
console.log(await response.json())
```

## Python 3 标准库 {#python}

保存为 `models.py`，运行 `python3 models.py`。

```python
import json
import os
import urllib.request

request = urllib.request.Request(
    'https://mikiko.studio/api/open/v1/image-models',
    headers={'Authorization': 'Bearer ' + os.environ['MIKIKO_API_KEY']},
)
with urllib.request.urlopen(request, timeout=30) as response:
    print(json.load(response))
```

## OpenAI Python SDK {#openai-sdk}

安装 `openai` 后读取兼容模型目录：

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ['MIKIKO_API_KEY'],
    base_url='https://mikiko.studio/v1',
)
print(client.models.list())
```

生成时调用 `client.images.generate(...)`，模型和参数必须取自发现结果。生成可能产生费用。

### 等待与流式结果

- 同步等待最多两分钟；`504 TASK_WAIT_INTERRUPTED` 后使用 `X-Task-ID` 或 `Location` 查询。
- `stream=true` 是平台生命周期扩展，SDK 未必支持，见[兼容接口](openai-compatible.md)。
- `partial_images` 不支持。

## 原生图片任务流程

下面按顺序展示请求。先发现项目和模型，再将自己的 ID 和支持的尺寸填入 `request.json`。创建步骤可能产生费用。

```bash
curl https://mikiko.studio/api/open/v1/projects \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
curl https://mikiko.studio/api/open/v1/image-models \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

### 准备业务输入

```json
{
  "project_id": "REPLACE_WITH_OWNED_PROJECT_ID",
  "route_model_code": "REPLACE_WITH_DISCOVERED_MODEL",
  "operation": "generate",
  "task_type": "text_to_image",
  "prompt": "窗边的一瓶鲜花",
  "size_mode": "pixel",
  "requested_size": "1024x1024",
  "output_image_count": 1
}
```

### 预检

```bash
curl https://mikiko.studio/api/open/v1/image-tasks/preflight \
  -H "Authorization: Bearer $MIKIKO_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @request.json
```

从响应 `data.quote_token` 读取报价，将它和本次操作的唯一 `idempotency_key` 加入相同业务输入，保存为 `create.json`。

### 创建与查询

```bash
curl https://mikiko.studio/api/open/v1/image-tasks \
  -H "Authorization: Bearer $MIKIKO_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @create.json

# TASK_ID 填创建响应中的 data.id
curl "https://mikiko.studio/api/open/v1/image-tasks/$TASK_ID" \
  -H "Authorization: Bearer $MIKIKO_API_KEY"
```

- 网络重试复用相同业务输入和幂等键；参数变化后重新预检。
- 图片创建读取 `data.id`；视频创建读取 `data.task.id` 和 `data.replayed`。
- 结果就绪后请求资产访问链接；长期保存任务和资产 ID。
- 积分保留为小数字符串。完整响应类型见[接口参考](../../web/docs/public/openapi/openapi.yaml)。

## MCP 客户端

MCP 地址为 `https://mikiko.studio/mcp`，通过浏览器 OAuth 授权，不把 SK 填入客户端配置。见 [MCP 接入](mcp.md)。
