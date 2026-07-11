import React from 'react'
import { CodeBlock } from '../components/CodeBlock'
import type { Guide } from '../types'

const baseURL = 'https://api.example.com'

export const guides: Guide[] = [
  {
    id: 'quickstart', group: '开始使用', title: '快速开始', summary: '创建密钥并发起第一个异步图片生成任务。',
    searchText: 'quickstart first request node hmac task image generation 快速开始 首个请求 签名',
    content: <>
      <Lead>使用 AK/SK 为原生开放接口生成 HMAC 签名。创建任务后保存返回的任务 ID，再轮询任务详情直到进入终态。</Lead>
      <h2 id="first-request">发起第一个请求</h2>
      <CodeBlock language="javascript" label="Node.js 18+" code={`import { createHash, createHmac } from 'node:crypto'

const path = '/api/open/image/v1/tasks'
const body = JSON.stringify({
  task_type: 'text_to_image',
  route_model_code: 'basic',
  prompt: 'cinematic product scene',
  base_resolution: '1k',
  requested_size: '1:1',
  requested_output_image_count: 1,
  response_mode: 'async',
})
const timestamp = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')
const bodySHA256 = createHash('sha256').update(body).digest('base64url')
const canonical = ['POST', path, timestamp, bodySHA256].join('\n')
const signature = createHmac('sha256', process.env.MIKIKO_SECRET_KEY)
  .update(canonical).digest('base64url')

const response = await fetch('${baseURL}' + path, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Access-Key': process.env.MIKIKO_ACCESS_KEY,
    'X-Timestamp': timestamp,
    'X-Body-SHA256': bodySHA256,
    'X-Signature': signature,
  },
  body,
})
console.log(await response.json())`} />
      <Callout>Secret 仅在创建或重置时展示一次，只用于本地计算签名。不要把 Secret 写进浏览器前端或公开仓库。</Callout>
    </>,
  },
  {
    id: 'authentication', group: '开始使用', title: '认证与密钥', summary: '理解 AK/SK、Bearer 会话与密钥安全边界。',
    searchText: 'authentication api key ak sk bearer hmac signature secret security 认证 密钥 签名 安全',
    content: <>
      <Lead>用户 Web 使用 Bearer 会话；服务端集成使用用户创建的 AK/SK。Secret 只用于本地计算签名，不会随请求发送。</Lead>
      <h2 id="headers">开放接口请求头</h2>
      <CodeBlock code={`X-Access-Key: $MIKIKO_ACCESS_KEY
X-Timestamp: 2026-07-10T08:30:00Z
X-Body-SHA256: <base64url_sha256_of_exact_body>
X-Signature: <base64url_hmac_sha256>
Content-Type: application/json`} />
      <h2 id="security">安全建议</h2>
      <Bullet items={['按生产、测试和自动化环境拆分密钥。', '设置最小必要额度与 RPM，定期轮换 Secret。', '服务端保存 Secret，日志中只记录掩码。']} />
    </>,
  },
  {
    id: 'native-api', group: '图片生成', title: '原生图片 API', summary: '使用平台完整任务、参数、预估和状态能力。',
    searchText: 'native api task route_model_code base_resolution requested_size 原生 图片 任务 参数',
    content: <>
      <Lead>原生 API 暴露平台的抽象模型、能力协商、积分预估、参考图与异步任务状态，是新集成的首选入口。</Lead>
      <h2 id="capabilities">先读取能力</h2>
      <CodeBlock code={`curl ${baseURL}/api/open/image/v1/capabilities \
  -H "X-Access-Key: $MIKIKO_ACCESS_KEY" \
  -H "X-Timestamp: $TIMESTAMP" \
  -H "X-Body-SHA256: $EMPTY_BODY_SHA256" \
  -H "X-Signature: $SIGNATURE"`} />
      <h2 id="task">创建与查询任务</h2>
      <CodeBlock code={`POST /api/open/image/v1/tasks
GET  /api/open/image/v1/tasks/{task_id}`} />
    </>,
  },
  {
    id: 'openai-compatible', group: '图片生成', title: 'OpenAI 兼容接口', summary: '以熟悉的 images/generations 与 images/edits 接入。',
    searchText: 'openai compatible images generations edits 兼容 生图 编辑',
    content: <>
      <Lead>已有 OpenAI 图片 SDK 或请求封装时，可以仅替换 Base URL 与密钥。兼容接口仍由平台完成模型路由、计费和失败降级。</Lead>
      <h2 id="generations">文生图</h2>
      <CodeBlock code={`curl -X POST ${baseURL}/v1/images/generations \
  -H "Authorization: Bearer $MIKIKO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"basic","prompt":"editorial still life","size":"1024x1024","n":1}'`} />
      <h2 id="edits">图片编辑</h2>
      <CodeBlock code="POST /v1/images/edits" />
    </>,
  },
  {
    id: 'image-editing', group: '图片生成', title: '图片编辑', summary: '基于已有图片修改主体、材质或场景。',
    searchText: 'image edit editing reference asset 图片编辑 修改',
    content: <>
      <Lead>先上传参考资产，再以 image_edit 创建任务。请求中的参考资产数量必须符合所选模型实时能力。</Lead>
      <CodeBlock language="json" code={`{
  "task_type": "image_edit",
  "route_model_code": "basic",
  "prompt": "change the background to a quiet studio",
  "base_resolution": "1k",
  "requested_output_image_count": 1,
  "reference_image_count": 1,
  "reference_asset_ids": ["3f6b5ce7-6d7c-4f85-a22e-d23afef46a2b"],
  "response_mode": "async"
}`} />
    </>,
  },
  {
    id: 'reference-images', group: '图片生成', title: '参考图生成', summary: '上传、复用并管理参考图片。',
    searchText: 'reference image upload asset max count 参考图 上传 限制',
    content: <>
      <Lead>参考图通过独立资产接口上传。能力响应会返回文件大小和数量限制，客户端必须在提交任务前遵守这些限制。</Lead>
      <CodeBlock code={`POST /api/open/image/v1/reference-assets/uploads
POST /api/open/image/v1/reference-assets
GET  /api/open/image/v1/reference-assets/{asset_id}`} />
      <Callout>不要假定所有模型都支持参考图。每次展示参数前读取 capabilities。</Callout>
    </>,
  },
  {
    id: 'task-polling', group: '运行与排错', title: '任务状态与轮询', summary: '正确处理排队、生成、保存、部分成功和失败。',
    searchText: 'async polling queued running persisting partial failed status 异步 轮询 状态',
    content: <>
      <Lead>任务是异步状态机。只在终态停止轮询，并为部分成功保留已生成图片和失败明细。</Lead>
      <CodeBlock language="javascript" code={`async function waitForTask(id) {
  while (true) {
    const task = await getTask(id)
    if (["succeeded", "partial_failed", "failed", "rejected", "deleted"].includes(task.status)) return task
    await new Promise(resolve => setTimeout(resolve, 1500))
  }
}`} />
      <Bullet items={['queued：通过校验并等待调度。', 'running：模型生成或结果持久化中。', 'partial_failed：返回成功图片与失败明细。', 'failed：无可用结果，按平台规则释放或返还积分。']} />
    </>,
  },
  {
    id: 'capabilities', group: '运行与排错', title: '模型能力', summary: '从服务端能力响应构建合法参数界面。',
    searchText: 'capabilities model groups task types ratios pixel sizes limits 模型能力 比例 像素',
    content: <>
      <Lead>模型、任务类型、基础分辨率、尺寸模式、比例、像素尺寸、输出数量和参考图上限都由服务端动态返回。</Lead>
      <CodeBlock code="GET /api/open/image/v1/capabilities" />
      <Callout>不要在客户端硬编码模型参数。后台路由策略变化后，能力响应可能随时更新。</Callout>
    </>,
  },
  {
    id: 'estimates', group: '运行与排错', title: '积分预估', summary: '在提交前按完整参数获取匹配的积分预估。',
    searchText: 'estimate points billing payload key 积分 预估 计费',
    content: <>
      <Lead>预估必须与即将提交的完整 payload 一一对应。模型、尺寸、数量或参考图变化后，应使旧预估失效并重新请求。</Lead>
      <CodeBlock code="GET /api/open/image/v1/estimate?abstract_model=plus&task_type=text_to_image&base_resolution=1k&requested_size=1:1&requested_output_image_count=1&reference_image_count=0" />
    </>,
  },
  {
    id: 'errors', group: '运行与排错', title: '错误与恢复', summary: '根据错误码判断修正、重试或联系管理员。',
    searchText: 'errors retryable request id error code 错误码 重试 恢复',
    content: <>
      <Lead>错误响应包含稳定错误码、可读消息和请求 ID。先修正参数与权限错误，只对明确可重试的暂态错误执行退避重试。</Lead>
      <CodeBlock language="json" code={`{
  "error": {
    "code": "MODEL_CAPABILITY_MISMATCH",
    "message": "selected model does not support this parameter",
    "request_id": "req_..."
  }
}`} />
    </>,
  },
  {
    id: 'rate-limits', group: '运行与排错', title: '限速与幂等', summary: '控制 RPM，并避免网络重试创建重复任务。',
    searchText: 'rate limit rpm idempotency retry 429 限速 幂等',
    content: <>
      <Lead>账号、密钥和平台策略会共同限制请求速率。创建任务时复用同一个幂等键，避免超时重试产生重复资源。</Lead>
      <Bullet items={['收到 429 后读取错误信息并指数退避。', '同一次业务操作的重试必须复用幂等键。', '不同业务操作不得共享幂等键。']} />
    </>,
  },
  {
    id: 'troubleshooting', group: '运行与排错', title: '故障排查', summary: '从请求 ID、任务阶段和能力响应定位问题。',
    searchText: 'troubleshooting debug request id task stage capability 故障 排查',
    content: <>
      <Lead>排查时同时记录请求 ID、任务 ID、模型分组、任务类型和发生阶段。不要记录 Secret 或完整用户令牌。</Lead>
      <h2 id="checklist">检查顺序</h2>
      <Bullet items={['确认当前 capabilities 允许所选参数。', '确认密钥启用、未过期且额度/RPM 足够。', '查询任务详情，记录 progress_stage 与错误码。', '携带 request_id 联系平台管理员。']} />
    </>,
  },
]

function Lead({ children }: { children: string }) { return <p className="guide-lead">{children}</p> }
function Callout({ children }: { children: string }) { return <aside className="guide-callout">{children}</aside> }
function Bullet({ items }: { items: string[] }) { return <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> }
