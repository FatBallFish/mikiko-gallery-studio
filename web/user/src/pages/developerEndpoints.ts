import type { DeveloperEndpoints } from '../../../shared/api-types'

export function developerAPIExamples(config: DeveloperEndpoints) {
  if (!config.canonical_configured || !config.rest_base || !config.openai_base) return []
  const rest = config.rest_base.replace(/\/$/, '')
  return [
    {
      name: '1. 查询可用模型',
      detail: '将创建时保存的 API Key（sk-…）注入 MIKIKO_API_KEY。查询不会创建任务；从目录中选择支持的模型与尺寸。',
      code: `# MIKIKO_API_KEY 是 sk-…，不是密钥标识 ak-…
curl --fail-with-body ${JSON.stringify(`${rest}/image-models`)} \\
  -H "Authorization: Bearer $MIKIKO_API_KEY"

# OpenAI 兼容模型目录
curl --fail-with-body ${JSON.stringify(`${config.openai_base.replace(/\/$/, '')}/models`)} \\
  -H "Authorization: Bearer $MIKIKO_API_KEY"`,
    },
    {
      name: '2. OpenAI 兼容生图与保存结果',
      detail: 'Python：先安装 openai。MIKIKO_MODEL 和 MIKIKO_SIZE 填入目录支持的模型与尺寸。生图会消耗积分；此示例等待同步结果并保存第一张图片。',
      code: `import os, base64
from pathlib import Path
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["MIKIKO_API_KEY"],  # 创建时保存的 sk-…
    base_url=${JSON.stringify(config.openai_base)},
)
result = client.images.generate(
    model=os.environ["MIKIKO_MODEL"],
    prompt="一只坐在窗边的猫，柔和的自然光",
    size=os.environ["MIKIKO_SIZE"],
    n=1, response_format="b64_json",
)
Path("image.png").write_bytes(base64.b64decode(result.data[0].b64_json))`,
    },
    {
      name: '3. 原生 API：预检 → 创建 → 轮询 → 下载',
      detail: 'Python 3，无需 SDK。先按下方结构准备 image-request.json。预检与创建使用同一份参数，quote_token 来自真实预检响应。创建前会提示确认积分消耗。',
      code: `import json, os, time, uuid
from pathlib import Path
from urllib.request import Request, urlopen

key = os.environ["MIKIKO_API_KEY"]  # 实际 Bearer API Key：sk-…
def call(url, body=None):
    headers = {"Authorization": "Bearer " + key}
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    with urlopen(Request(url, data=data, headers=headers), timeout=120) as r:
        return json.load(r)["data"]

payload = json.loads(Path("image-request.json").read_text())
quote = call(${JSON.stringify(`${rest}/image-tasks/preflight`)}, payload)
print("预估积分：", quote["estimated_points"], "报价到期：", quote["expires_at"])
if input("输入 yes 确认创建付费任务：") != "yes":
    raise SystemExit("已取消")
# 同一个逻辑请求重试时复用 idempotency_key，不要重新生成。
creation = dict(payload, quote_token=quote["quote_token"],
                idempotency_key=str(uuid.uuid4()))
Path("image-create.json").write_text(json.dumps(creation))
task = call(${JSON.stringify(`${rest}/image-tasks`)}, creation)
task_url = ${JSON.stringify(`${rest}/image-tasks/`)} + task["id"]
print("任务：", task_url)  # 保存 ID，断线后可继续 GET
for _ in range(120):
    task = call(task_url)
    if task["status"] in ("succeeded", "partial_failed", "failed", "rejected", "deleted"):
        break
    time.sleep(3)
else:
    raise SystemExit("任务仍在进行，稍后使用已保存的任务 URL 查询")
if not task.get("results"):
    raise RuntimeError(task.get("error_message") or task["status"])
for index, image in enumerate(task["results"]):
    access = call(task_url + "/results/" + image["id"] + "/access",
                  {"purpose": "download"})
    # 下载链接已携带授权，不向存储服务发送 API Key。
    with urlopen(access["url"], timeout=120) as r:
        Path("result-" + str(index) + "." + (image.get("format") or "png")).write_bytes(r.read())
print("任务状态：", task["status"])`,
    },
    {
      name: 'image-request.json',
      detail: '替换三个占位值后保存文件：已有项目 ID，选择 task_types 包含 text_to_image、size_modes 支持 pixel 的模型；将其 code 填入 route_model_code，从 pixel_sizes 中选择 requested_size。参数变更或报价过期后重新预检。需要 models:read、images:write、images:read 权限。请求失败时检查响应中的错误码；创建重试使用 image-create.json 中保存的同一请求。',
      code: JSON.stringify({ project_id: '<已有项目 ID>', route_model_code: '<model_groups 中的 code>', task_type: 'text_to_image', prompt: '一只坐在窗边的猫，柔和的自然光', size_mode: 'pixel', requested_size: '<模型支持的尺寸>', output_image_count: 1 }, null, 2),
    },
  ]
}

export function endpointDisabledReason(config: DeveloperEndpoints) {
  if (config.canonical_configured) return ''
  return '此部署尚未配置可信公共地址，暂时无法提供接入示例。请联系管理员配置后重试。'
}

export function developerClientTemplates(config: DeveloperEndpoints) {
  if (!config.canonical_configured || !config.mcp_url) return []
  const url = config.mcp_url
  // JSON string quoting is also valid for TOML basic strings used here.
  return [
    { name: 'Mikiko / ZCode', format: 'JSON · .mikiko/config.json', code: JSON.stringify({ mcp: { servers: { studio: { type: 'http', url } } } }, null, 2), instructions: '在设置 → MCP 添加 HTTP 服务，或合并到工作区配置。重启会话，在 MCP 状态中完成客户端提供的浏览器授权；若版本未提供 OAuth 登录入口，请升级客户端。配置中不填 API Key 或手工 token。' },
    { name: 'Codex', format: 'TOML · ~/.codex/config.toml', code: `[mcp_servers.studio]\nurl = ${JSON.stringify(url)}`, instructions: '合并 TOML 后执行 codex mcp login studio，完成浏览器登录与同意授权；使用 codex mcp list 检查配置。' },
    { name: 'Claude Code', format: 'CLI · 终端执行', code: `claude mcp add --transport http studio '${url.replace(/'/g, "'\\''")}'`, instructions: '在 Claude Code 输入 /mcp，选择 studio，按提示 Authenticate 完成浏览器 OAuth。可通过 claude mcp list 检查配置。' },
  ]
}
