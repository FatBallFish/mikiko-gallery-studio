// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
import type { ImageResult, ImageTask, UserProfile } from '../../../shared/api-types'
import { projectWorkspaceImageDetail } from './workspaceImageDetail'

const image = {
  id: 'image-01',
  url: '/api/agent/image/v1/images/image-01',
  download_url: '/api/agent/image/v1/images/image-01',
  width: 2048,
  height: 1360,
  publish_status: 'private',
  prompt: 'Image-specific revised prompt',
  base_resolution: '4k',
  quality: 'high',
  route_model_code: 'image-route-model',
} as ImageResult

const task = {
  id: 'task-01',
  prompt: 'Task prompt',
  task_type: 'image_edit',
  size_mode: 'pixel',
  requested_size: '2048x1360',
  base_resolution: '2k',
  quality: 'medium',
  aspect_ratio: '128:85',
  output_format: 'webp',
  output_compression: 87,
  moderation: 'low',
  requested_output_image_count: 3,
  image_count: 3,
  reference_asset_ids: ['reference-01'],
  route_model_code: 'task-route-model',
  abstract_model: 'plus-image',
  created_at: '2026-08-03T10:00:00Z',
} as ImageTask

const profile = { display_name: 'Mikiko Creator' } as UserProfile
const detail = projectWorkspaceImageDetail(image, task, profile)

assert.deepEqual({
  id: detail.id,
  author: detail.author_name,
  width: detail.width,
  height: detail.height,
  prompt: detail.prompt,
  taskType: detail.task_type,
  sizeMode: detail.size_mode,
  requestedSize: detail.requested_size,
  baseResolution: detail.base_resolution,
  quality: detail.quality,
  ratio: detail.aspect_ratio,
  outputFormat: detail.output_format,
  outputCompression: detail.output_compression,
  moderation: detail.moderation,
  outputCount: detail.requested_output_image_count,
  references: detail.reference_asset_ids,
  routeModel: detail.route_model_code,
  abstractModel: detail.abstract_model,
  createdAt: detail.created_at,
}, {
  id: 'image-01',
  author: 'Mikiko Creator',
  width: 2048,
  height: 1360,
  prompt: 'Image-specific revised prompt',
  taskType: 'image_edit',
  sizeMode: 'pixel',
  requestedSize: '2048x1360',
  baseResolution: '4k',
  quality: 'high',
  ratio: '128:85',
  outputFormat: 'webp',
  outputCompression: 87,
  moderation: 'low',
  outputCount: 3,
  references: ['reference-01'],
  routeModel: 'image-route-model',
  abstractModel: 'plus-image',
  createdAt: '2026-08-03T10:00:00Z',
})

console.log('OK: workspace image detail projection contract passed')
