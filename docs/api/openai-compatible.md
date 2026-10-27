# OpenAI 兼容接口

## 发起图片请求

`MIKIKO_API_KEY` 使用一次显示的 SK（`sk-`），不是 AK（`ak-`）。先发现可用模型，再替换示例中的模型标识。执行生成请求可能产生费用。

```bash
curl https://mikiko.studio/v1/models \
  -H "Authorization: Bearer $MIKIKO_API_KEY"

curl https://mikiko.studio/v1/images/generations \
  -H "Authorization: Bearer $MIKIKO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"REPLACE_WITH_DISCOVERED_MODEL","prompt":"窗边的一瓶鲜花","size":"1024x1024","n":1}'
```

- `POST https://mikiko.studio/v1/images/generations` 使用 JSON。
- `POST https://mikiko.studio/v1/images/edits` 使用 multipart 文件上传。
- 尺寸、数量和编辑能力以模型发现结果为准。
- `n` 是普通输出数量；图层拆分使用原生 REST/MCP 的 `layer_decomposition`。
- Qwen/Seedream 原生适配器不声明独立 mask 能力。

## 同步结果与恢复

默认返回同步图片结果。响应头 `X-Task-ID` 与 `Location` 提供持久任务查询入口。

同步等待最多两分钟。收到 `504 TASK_WAIT_INTERRUPTED` 时任务可能仍在运行，先查询已有任务，避免重复生成。

## 流式行为 {#stream}

- `stream=true` 是平台生命周期扩展，返回 `task` 与最终 `result` 事件。
- 响应头为 `X-Stream-Protocol: platform-image-lifecycle-v1`。
- 成功结果后发送 `data: [DONE]`；失败以 `error` 事件结束，不发送成功标记。
- `partial_images` 明确不支持。排队进度不是部分图片。

SDK 未必能消费平台事件；可用 `fetch` 或原生任务 events 接口读取。完整类型见[接口参考](../../web/docs/public/openapi/openapi.yaml)。
