// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import type { ImageResult } from '../../shared/api-types'
import { PublicImageDetail } from './components'

const image = {
  id: 'metadata-image',
  url: '/api/agent/image/v1/images/metadata-image',
  width: 2048,
  height: 1360,
  publish_status: 'private',
  author_name: 'Mikiko Creator',
  route_model_code: 'mikiko-image-pro',
  prompt: 'A detailed studio scene',
  task_type: 'image_edit',
  size_mode: 'pixel',
  requested_size: '2048x1360',
  base_resolution: '2k',
  aspect_ratio: '128:85',
  quality: 'high',
  output_format: 'webp',
  output_compression: 87,
  moderation: 'low',
  requested_output_image_count: 3,
} as ImageResult

const html = renderToString(createElement(PublicImageDetail, {
  image,
  imageUrl: image.url,
  showPublicStats: false,
  onCopyPrompt: () => undefined,
}))

for (const expected of [
  '实际分辨率', '2048 x 1360',
  '基础分辨率', '2k',
  '尺寸模式', '像素尺寸',
  '请求尺寸', '2048x1360',
  '任务类型', '图片编辑',
  '质量', 'high',
  '输出格式', 'WEBP',
  '压缩质量', '87%',
  '审核等级', 'low',
  '输出数量', '>3<',
]) {
  assert.ok(html.includes(expected), `image detail metadata must render ${expected}: ${html}`)
}

console.log('OK: image detail metadata contract passed')
