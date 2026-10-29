import type { Guide } from '../types'
import source0 from '../../../../docs/api/quickstart.md?raw'
import source1 from '../../../../docs/api/authentication.md?raw'
import source2 from '../../../../docs/api/native-api.md?raw'
import source3 from '../../../../docs/api/openai-compatible.md?raw'
import source4 from '../../../../docs/api/image-editing.md?raw'
import source5 from '../../../../docs/api/reference-images.md?raw'
import source6 from '../../../../docs/api/task-polling.md?raw'
import source7 from '../../../../docs/api/capabilities.md?raw'
import source8 from '../../../../docs/api/estimates.md?raw'
import source9 from '../../../../docs/api/errors.md?raw'
import source10 from '../../../../docs/api/rate-limits.md?raw'
import source11 from '../../../../docs/api/troubleshooting.md?raw'
import source12 from '../../../../docs/api/migration.md?raw'
import source13 from '../../../../docs/api/sdk-examples.md?raw'
import source14 from '../../../../docs/api/mcp.md?raw'
import source15 from '../../../../docs/api/developer-field-map.md?raw'

export const guides: Guide[] = [
  { id: 'quickstart', group: '开始使用', title: '快速开始', summary: '使用 Bearer API Key 发现能力并创建持久化任务。', searchText: source0, content: source0 },
  { id: 'authentication', group: '开始使用', title: '认证与密钥', summary: '所有开发者 REST 与兼容接口使用 Bearer API Key。', searchText: source1, content: source1 },
  { id: 'native-api', group: '生成与资产', title: '原生图片与视频 API', summary: '统一发现、预检、创建、查询、事件与视频取消。', searchText: source2, content: source2 },
  { id: 'openai-compatible', group: '生成与资产', title: 'OpenAI 兼容接口', summary: '同步结果和明确区分的平台生命周期 SSE 扩展。', searchText: source3, content: source3 },
  { id: 'image-editing', group: '生成与资产', title: '图片编辑', summary: '上传自己的参考资产，并通过完整请求预检。', searchText: source4, content: source4 },
  { id: 'reference-images', group: '生成与资产', title: '资产、上传与分组', summary: '管理持久资产、续期访问链接和项目内分组。', searchText: source5, content: source5 },
  { id: 'task-polling', group: '运行与排错', title: '任务状态与事件', summary: '使用详情和生命周期事件跟踪持久化任务。', searchText: source6, content: source6 },
  { id: 'capabilities', group: '运行与排错', title: '模型能力', summary: '按当前模型能力构建合法请求。', searchText: source7, content: source7 },
  { id: 'estimates', group: '运行与排错', title: '完整请求预检', summary: '校验请求并获取非扣费的成本预检。', searchText: source8, content: source8 },
  { id: 'errors', group: '运行与排错', title: '错误与恢复', summary: '依据真实错误响应修正请求并重试。', searchText: source9, content: source9 },
  { id: 'rate-limits', group: '运行与排错', title: '限速与幂等', summary: '控制请求速率并避免重复任务和扣费。', searchText: source10, content: source10 },
  { id: 'troubleshooting', group: '运行与排错', title: '故障排查', summary: '定位权限、能力、配额与持久化问题。', searchText: source11, content: source11 },
  { id: 'migration', group: '开始使用', title: '迁移到统一 API', summary: '直接替换旧签名接口，无兼容层。', searchText: source12, content: source12 },
  { id: 'sdk-examples', group: '开始使用', title: 'SDK 与可运行示例', summary: '通过 Node.js、Python 和 OpenAI SDK 发现能力。', searchText: source13, content: source13 },
  { id: 'mcp', group: 'MCP 与 OAuth', title: 'MCP 连接与授权', summary: '浏览器 OAuth 授权、最小权限与连接撤销。', searchText: source14, content: source14 },
  { id: 'inventory', group: '开始使用', title: '能力与信任边界', summary: '区分开发者 API、现有浏览器功能、管理端和回调。', searchText: source15, content: source15 },
]
