import React from 'react'
import { CodeBlock } from '../components/CodeBlock'
import type { Guide } from '../types'
import { developerDetails } from './developerDetails'
import { DetailedGuide } from './DetailedGuide'

// Example only; never derive OAuth endpoints from this placeholder.
const baseURL = 'https://studio.example.com'
const auth = '-H "Authorization: Bearer $MIKIKO_API_KEY"'

export const guides: Guide[] = [
  {
    id: 'quickstart', group: '开始使用', title: '快速开始', summary: '使用 Bearer API Key 发现能力并创建持久化任务。',
    searchText: 'quickstart bearer first request image video 快速开始 首个请求',
    content: <>
      <Lead>在控制台创建具有所需 scopes 的 API Key，保存在服务端环境变量中。统一开发者接口使用 /api/open/v1；先发现模型，再预检完整请求，最后创建任务并保存任务 ID。</Lead>
      <h2 id="base-urls">选择正确 BaseURL</h2><p>密钥页端点源自可信公共配置。Origin 不含路径；原生 REST 使用 /api/open/v1，OpenAI SDK 使用 /v1，MCP 使用 /mcp，不重复追加前缀。示例域名必须替换为部署端点；canonical 未配时 OAuth 禁用，不能从 Host 推导 issuer。</p><p><a href="#/guide/mcp">端点、OAuth 与客户端教程</a> · <a href="#/guide/inventory">所有操作与实际字段映射</a> · <a href="#/reference">API reference</a></p><h2 id="first-request">确认身份与模型</h2>
      <CodeBlock code={`export MIKIKO_BASE_URL=${baseURL}
# MIKIKO_API_KEY 通过秘密管理器注入
curl "$MIKIKO_BASE_URL/api/open/v1/me" ${auth}
curl "$MIKIKO_BASE_URL/api/open/v1/image-models" ${auth}`} />
      <h2 id="next">提交前的步骤</h2>
      <Bullet items={['从模型响应选择允许的参数，不假设示例模型在所有部署可用。', '向 POST /api/open/v1/image-tasks/preflight 发送完整创建请求。', '向 POST /api/open/v1/image-tasks 提交相同请求，保存返回的 ID。', '查询任务详情或订阅事件；获取结果资产时重新请求访问链接。']} />
      <Callout>部署能力与权限以模型发现和接口返回为准。预检成功不保证稍后创建成功；创建时会重新检查权限、能力、价格、余额和配额。</Callout>
    </>,
  },
  {
    id: 'authentication', group: '开始使用', title: '认证与密钥', summary: '所有开发者 REST 与兼容接口使用 Bearer API Key。',
    searchText: 'authentication bearer api key scopes revoke 认证 权限 撤销',
    content: <>
      <Lead>原生开发者接口与 /v1 兼容接口统一发送 Authorization: Bearer。浏览器会话、管理员会话、支付回调和供应商回调属于独立信任边界。</Lead>
      <CodeBlock code={'Authorization: Bearer $MIKIKO_API_KEY\nContent-Type: application/json'} />
      <h2 id="scopes">权限与额度</h2>
      <Bullet items={['仅授予调用所需 scopes；已有图片密钥不会自动获得视频、删除或项目管理权限。', '每次请求都会检查当前账号状态、密钥状态、过期时间和权限。', 'RPM、总额度与日额度仍然适用；预检不扣费。', '在控制台轮换或撤销密钥；不要在前端、URL、日志或公开仓库保存令牌。']} />
    </>,
  },
  {
    id: 'native-api', group: '生成与资产', title: '原生图片与视频 API', summary: '统一发现、预检、创建、查询、事件与视频取消。',
    searchText: 'native image video task preflight cancel 原生 图片 视频 任务',
    content: <>
      <Lead>原生异步任务是持久化生成的标准入口。图片与视频共用认证和应用操作，具体参数仍由各自模型能力决定。</Lead>
      <CodeBlock code={`GET  /api/open/v1/image-models
GET  /api/open/v1/video-models
POST /api/open/v1/image-tasks/preflight
POST /api/open/v1/video-tasks/preflight
POST /api/open/v1/image-tasks
POST /api/open/v1/video-tasks
GET  /api/open/v1/image-tasks
GET  /api/open/v1/video-tasks`} />
      <h2 id="native-image-contract">Qwen / Seedream 原生图片契约</h2>
      <p>先读取模型的 native_image_profile，再预检完整请求。operation 为 generate、edit 或 layer_decomposition；是否允许执行还取决于部署路由、权限和已确认价格。普通图片数量与拆层产物数量独立，不能用 n=17 请求拆层。</p>
      <CodeBlock code={JSON.stringify({ operation: 'edit', native_options: { watermark: false, annotations: [{ reference_index: 1, label: '目标', point: [520, 460] }], seedream: { optimize_prompt_mode: 'standard' } } }, null, 2)} />
      <p>以上仅是需合入完整预检请求的原生字段片段。native_options 顶层字段为 seed、negative_prompt、watermark、annotations、qwen、seedream；Qwen 的 prompt_extend、prompt_extend_mode、enable_thinking 放入 qwen，Seedream 的 optimize_prompt_mode 放入 seedream。annotations 的 reference_index 从 1 开始，point 与 bbox 二选一，提示词坐标为 0..999。</p>
      <p>结果元数据位于 results[].layer，包含 z_index、is_base 和可选 name、description、bounding_box。bounding_box.absolute 是像素坐标，normalized 是响应的 0..1000 坐标，不能与输入标注的 0..999 混用。保留底图、透明 PNG 和层次顺序。</p>
      <h2 id="native-support">原生图片支持表</h2>
      <table><thead><tr><th>型号</th><th>参考图 / 普通输出</th><th>专属能力</th></tr></thead><tbody>
        <tr><td>Qwen 3.0 / 可选 3.0-pro</td><td>最多 3 / 1–6</td><td>seed、negative_prompt、提示词扩展/思考、PNG</td></tr>
        <tr><td>Qwen 2.1-pro / 2.1-turbo</td><td>最多 10 / 1–6</td><td>seed、提示词扩展/思考；提示词驱动透明输出</td></tr>
        <tr><td>Seedream 5.0 pro</td><td>最多 10 / 1</td><td>坐标标注；优化 standard/fast；拆层需确认价格</td></tr>
        <tr><td>Seedream 5.0 flash</td><td>最多 10 / 1</td><td>坐标标注；优化 standard；拆层价格尚待确认</td></tr>
      </tbody></table>
      <p>Qwen 输出 262144–4194304 像素，宽高比至 8:1；Seedream 输出 921600–4624220 像素，宽高比至 16:1，JPEG/PNG。Seedream 不支持 seed/negative_prompt。profile 的拆层能力不代表线上已经启用；一张参考图拆为一张底图和最多 16 个透明层，不能改写为普通批量图片。</p>
      <h2 id="native-setup">管理员配置与价格确认</h2>
      <p>原生 adapter 分别为 qwen_image 和 seedream_image，默认端点为 https://dashscope.aliyuncs.com 与 https://ark.cn-beijing.volces.com/api/v3。账号模型使用 extra.native_image = {'{model, profile?, pricing, sale_pricing}'}，model 填 Endpoint 对应的真实型号；可选 profile 必须与其能力一致。pricing 是成本 CNY，sale_pricing 是销售 CNY，需显式配置后启用路由。默认牌价不会自动应用到线上。</p>
      <p>拆层成本与售价都需已确认的 layer_base_cny、layer_tiers 和 layer_pricing_confirmed=true。Pro 底图收费、Flash 拆层价格尚待核实，未知价格拒绝执行，不能置零。确认为零也须显式记录并由操作人员确认。CNY 与积分分开，预检按配置换算并绑定能力/价格版本。</p>
      <h2 id="layer-download">图层清单与下载边界</h2>
      <p>{'浏览器会话可读取 GET /api/agent/image/v1/tasks/{task_id}/layers，以及 /layers/archive ZIP；ZIP 内包含固定 layer-NN 文件和 manifest.json，最多 17 个结果且图片总量不超过 128 MiB。API Key/MCP 使用 get_image_task_layers({id})，每层返回 access_operation=get_image_result_access 与 access_input={task_id,image_id,purpose:"download"} 以续期下载；浏览器 ZIP URL 不能当作 OAuth 下载链接。工具和 REST 路径以当前部署实际发现与接口参考为准。'}</p>
      <Callout>保存创建响应中的任务 ID。客户端断开连接不会把已经持久化的异步任务变成仅存在于连接中的作业。</Callout>
    </>,
  },
  {
    id: 'openai-compatible', group: '生成与资产', title: 'OpenAI 兼容接口', summary: '同步结果和明确区分的平台生命周期 SSE 扩展。',
    searchText: 'openai compatible synchronous stream sse lifecycle partial images 兼容 流式',
    content: <>
      <Lead>/v1/models、/v1/images/generations 与 /v1/images/edits 使用相同 Bearer API Key。默认返回同步图片结果；需要可恢复的持久任务控制时使用原生 API。</Lead>
      <CodeBlock code={`curl -X POST ${baseURL}/v1/images/generations \\
  ${auth} \\
  -H "Content-Type: application/json" \\
  -d '{"model":"REPLACE_WITH_DISCOVERED_MODEL","prompt":"editorial still life","size":"1024x1024","n":1}'`} />
      <p>已配置的新 Qwen/Seedream 模型可通过兼容接口进行普通生成与编辑，具体能力和数量按发现结果约束。独立 layer_decomposition 操作仅通过原生 REST/MCP 暴露，n 始终表示普通输出图片数量。两种原生适配器均不声明独立 mask 能力。</p>
      <h2 id="stream">流式行为</h2>
      <p>stream 扩展只表达平台任务生命周期和最终结果。排队、执行或保存状态不是部分图片。只有上游真实提供并且适配器支持时才能发送部分图片事件；不能通过拆分最终图片模拟。不能支持的参数必须返回错误。</p>
      <p>stream=true 返回 task 与 result 事件，成功结果后发送 data: [DONE]；错误以 error 事件结束，不发送成功标记。响应头 X-Stream-Protocol 为 platform-image-lifecycle-v1。默认同步等待最多两分钟，超时返回 504 TASK_WAIT_INTERRUPTED，但持久任务保留，通过 X-Task-ID 与 Location 恢复查询。partial_images 明确不支持。</p>
      <Callout>平台生命周期事件不是 OpenAI 原生 partial-image 协议。SDK 是否能消费扩展事件取决于具体 SDK；使用 fetch 或原生任务 events 接口读取平台事件。</Callout>
    </>,
  },
  {
    id: 'image-editing', group: '生成与资产', title: '图片编辑', summary: '上传自己的参考资产，并通过完整请求预检。',
    searchText: 'image editing reference assets 图片编辑 参考图',
    content: <><Lead>选择支持图片编辑的模型，上传参考文件并取得自己的资产 ID。按照接口参考构建完整请求，预检与创建必须使用相同的模型、项目、参考资产、提示词与参数。</Lead><Callout>不能引用其他用户的资产；模型支持的参考数量、格式与大小由当前能力决定。</Callout></>,
  },
  {
    id: 'reference-images', group: '生成与资产', title: '资产、上传与分组', summary: '管理持久资产、续期访问链接和项目内分组。',
    searchText: 'assets upload multipart checksum resume groups projects access 资产 上传 分组 项目',
    content: <>
      <Lead>上传使用独立会话和分片目标。任务与工具传递资产 ID，避免大型 base64 输入，也不提供任意远程 URL 导入承诺。</Lead>
      <CodeBlock code={`GET  /api/open/v1/assets
POST /api/open/v1/assets/{asset_id}/access
POST /api/open/v1/uploads
GET  /api/open/v1/uploads/{upload_id}
GET  /api/open/v1/groups
GET  /api/open/v1/projects`} />
      <h2 id="upload">上传与访问</h2><p>按初始化响应的分片大小、校验和和上传目标完成文件传输。恢复上传时先查询状态，失效的上传链接需要重新获取。访问 URL 也会过期；保存资产 ID 并重新调用 access，而不是把临时 URL 当作永久资产地址。</p>
      <h2 id="groups">持久分组</h2><p>{'任务结果访问可使用 POST /api/open/v1/image-tasks/{task_id}/results/{image_id}/access 或视频对应的 /results/{asset_id}/access，分别使用 images:read 或 videos:read，无需授予整个资产库的读取权限。'}</p><p>分组属于用户和项目，资产最多属于一个分组。删除分组只清除资产分组关系。项目转移会解析目标项目内对应名称的分组。资产分组与模型分组、密钥计费分组不同。</p>
    </>,
  },
  {
    id: 'task-polling', group: '运行与排错', title: '任务状态与事件', summary: '使用详情和生命周期事件跟踪持久化任务。',
    searchText: 'async polling events sse terminal disconnect task 异步 轮询 状态 事件',
    content: <><Lead>创建任务后保存 ID，通过详情读取当前状态。事件连接用于观察生命周期；断线后重新查询任务，不从连接结束推断任务成功或失败。</Lead><p>图片与视频的状态及取消能力可能不同。按接口参考判断终态和可取消状态；不要把“已接受取消请求”当作上游已经停止。结果可能存在持久化失败或部分成功，检查响应中的结果与错误明细。</p><Callout>事件重放、Last-Event-ID 恢复和永久事件保留只有在接口明确声明时才能依赖。</Callout></>,
  },
  {
    id: 'capabilities', group: '运行与排错', title: '模型能力', summary: '按当前模型能力构建合法请求。',
    searchText: 'capabilities image video models ratios limits 模型能力 视频 比例',
    content: <><Lead>模型发现响应决定可用任务类型、尺寸、数量、参考文件和其他参数。账号、scope 和路由配置变化可能改变结果。</Lead><CodeBlock code={'GET /api/open/v1/image-models\nGET /api/open/v1/video-models'} /><Callout>不要硬编码所有模型都支持相同图片或视频参数。</Callout></>,
  },
  {
    id: 'estimates', group: '运行与排错', title: '完整请求预检', summary: '校验请求并获取非扣费的成本预检。',
    searchText: 'preflight quote balance points expiry normalized request 预检 报价 积分',
    content: <><Lead>预检使用与创建相同的完整请求，包括项目、引用资产、提示词变量、绑定和模型参数。它检查归属、能力、余额和密钥配额，但不扣费。</Lead><CodeBlock code={'POST /api/open/v1/image-tasks/preflight\nPOST /api/open/v1/video-tasks/preflight\nGET /api/open/v1/me/balance'} /><p>积分使用五位小数字符串，避免浮点运算丢失精度。报价会过期，并绑定规范化请求、调用身份及能力和价格版本。请求参数变化后重新预检；创建仍须重新校验并原子预留费用。</p></>,
  },
  {
    id: 'errors', group: '运行与排错', title: '错误与恢复', summary: '依据真实错误响应修正请求并重试。',
    searchText: 'errors retry request id scopes quota 错误码 重试 权限',
    content: <><Lead>保留请求 ID、任务 ID 和错误码以便排查。参数、权限、账号状态或配额错误需要先修正；暂态错误使用有上限的退避重试。</Lead><p>以各接口的错误响应为准；不要假定所有兼容接口、OAuth 端点和原生接口使用同一个错误封装。</p><p><a href="#/guide/inventory">原生错误码与实际调用点清单</a> · <a href="#/guide/mcp">OAuth 精确错误分类与恢复</a> · <a href="#/reference">各操作完整错误定义</a></p></>,
  },
  {
    id: 'rate-limits', group: '运行与排错', title: '限速与幂等', summary: '控制请求速率并避免重复任务和扣费。',
    searchText: 'rate limits rpm idempotency duplicate charge 429 限速 幂等',
    content: <><Lead>同一次创建操作的网络重试复用同一个幂等键与相同请求；新操作使用新键。不要假设密钥存在就能绕过账号额度或日额度。</Lead><Bullet items={['429 时读取实际响应并退避，不并发放大重试。', '创建超时后先使用已知任务 ID 查询，避免重复提交。', '幂等键作用范围、保留时间与冲突行为以接口参考为准。']} /></>,
  },
  {
    id: 'troubleshooting', group: '运行与排错', title: '故障排查', summary: '定位权限、能力、配额与持久化问题。',
    searchText: 'troubleshooting request id progress capability debug 故障 排查',
    content: <><Lead>记录请求 ID、任务 ID、模型与失败阶段。不要记录完整令牌或上传签名 URL。</Lead><Bullet items={['确认账号和密钥仍启用，scopes 覆盖该操作。', '重新读取模型能力并预检完整请求。', '检查余额、密钥配额和 RPM。', '查询任务与资产详情，区分生成失败和结果保存失败。']} /></>,
  },
  {
    id: 'migration', group: '开始使用', title: '迁移到统一 API', summary: '直接替换旧签名接口，无兼容层。',
    searchText: 'migration legacy bearer replace endpoints 迁移 旧接口',
    content: <><Lead>旧的签名开发者用户接口已直接替换。将认证改为 Bearer API Key，并按新 schema 重新生成客户端；仅替换路径不能保证请求兼容。</Lead><CodeBlock code={`旧 capabilities → GET /api/open/v1/image-models
旧 balance → GET /api/open/v1/me/balance
旧 estimate → POST /api/open/v1/image-tasks/preflight
旧 tasks → /api/open/v1/image-tasks
旧 reference-assets → /api/open/v1/assets 与 /api/open/v1/uploads`} /><p>保留原有公开图库路径、支付回调和供应商回调。浏览器与管理员接口不属于此次开发者 API 路径迁移。为已有图片密钥增加视频或删除权限需要显式授权。</p></>,
  },
  {
    id: 'sdk-examples', group: '开始使用', title: 'SDK 与可运行示例', summary: '通过 Node.js、Python 和 OpenAI SDK 发现能力。',
    searchText: 'sdk examples node python openai fetch 示例',
    content: <><Lead>以下示例读取环境变量并调用真实模型发现接口，不依赖特定部署的示例模型。</Lead><h2 id="node">Node.js 18+</h2><CodeBlock language="javascript" code={`const base = process.env.MIKIKO_BASE_URL
const key = process.env.MIKIKO_API_KEY
if (!base || !key) throw new Error('Set MIKIKO_BASE_URL and MIKIKO_API_KEY')
const response = await fetch(base.replace(/\\/$/, '') + '/api/open/v1/image-models', {
  headers: { Authorization: 'Bearer ' + key },
})
if (!response.ok) throw new Error(await response.text())
console.log(await response.json())`} /><h2 id="python">Python 3 标准库</h2><CodeBlock language="python" code={`import json, os, urllib.request
request = urllib.request.Request(
    os.environ['MIKIKO_BASE_URL'].rstrip('/') + '/api/open/v1/image-models',
    headers={'Authorization': 'Bearer ' + os.environ['MIKIKO_API_KEY']},
)
with urllib.request.urlopen(request, timeout=30) as response:
    print(json.load(response))`} /><h2 id="openai-sdk">OpenAI Python SDK</h2><CodeBlock language="python" code={`# pip install openai
import os
from openai import OpenAI
client = OpenAI(api_key=os.environ['MIKIKO_API_KEY'],
    base_url=os.environ['MIKIKO_BASE_URL'].rstrip('/') + '/v1')
print(client.models.list())`} /><Callout>OpenAPI 是请求与响应类型的唯一来源。生成客户端时使用文档站提供的同源 OpenAPI 文件，确保同时取得它引用的 components 文件。</Callout></>,
  },
  {
    id: 'mcp', group: 'MCP 与 OAuth', title: 'MCP 连接与授权', summary: '浏览器 OAuth 授权、最小权限与连接撤销。',
    searchText: developerDetails['mcp.md'],
    content: <><DetailedGuide source={developerDetails['mcp.md']} /><p><a href="#/reference">API reference：完整请求、响应与错误定义</a></p></>,
  },
  {
    id: 'inventory', group: '开始使用', title: '能力与信任边界', summary: '区分开发者 API、现有浏览器功能、管理端和回调。',
    searchText: developerDetails['developer-field-map.md'],
    content: <><Lead>以下按实际路由、operation 和序列化结构核对字段。完整类型与各操作错误以当前 API reference 为准。</Lead><p><a href="#/reference">打开完整 API reference</a></p><DetailedGuide source={developerDetails['developer-field-map.md']} /></>,
  },
]

function Lead({ children }: { children: string }) { return <p className="guide-lead">{children}</p> }
function Callout({ children }: { children: string }) { return <aside className="guide-callout">{children}</aside> }
function Bullet({ items }: { items: string[] }) { return <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> }
