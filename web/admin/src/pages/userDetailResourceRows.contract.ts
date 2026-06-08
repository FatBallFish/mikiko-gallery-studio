import {
  apiKeyStatusLabel,
  apiKeyStatusTone,
  imageTaskStatusLabel,
  imageTaskStatusTone,
  imageTaskTypeLabel,
  paymentOrderStatusLabel,
  paymentOrderStatusTone,
  userDetailApiKeyRow,
  userDetailBucketRow,
  userDetailOrderRow,
  userDetailTaskRow,
} from './userDetailResourceRows'
import type { ApiKey, BalanceBucket, ImageTask, PaymentOrder } from '../../../shared/api-types'

assertStatus(paymentOrderStatusLabel('pending'), '待支付', 'payment pending label')
assertStatus(paymentOrderStatusTone('pending'), 'warning', 'payment pending tone')
assertStatus(paymentOrderStatusLabel('completed'), '已到账', 'payment completed label')
assertStatus(paymentOrderStatusTone('paid'), 'success', 'payment paid tone')
assertStatus(paymentOrderStatusLabel('partially_refunded'), '部分退款', 'payment partial refund label')
assertStatus(paymentOrderStatusTone('partially_refunded'), 'warning', 'payment partial refund tone')
assertStatus(paymentOrderStatusLabel('failed'), '支付失败', 'payment failed label')
assertStatus(paymentOrderStatusTone('failed'), 'danger', 'payment failed tone')
assertStatus(paymentOrderStatusLabel('closed'), '已关闭', 'payment closed label')
assertStatus(paymentOrderStatusTone('closed'), 'neutral', 'payment closed tone')

assertStatus(imageTaskStatusLabel('queued'), '排队中', 'task queued label')
assertStatus(imageTaskStatusTone('queued'), 'warning', 'task queued tone')
assertStatus(imageTaskStatusLabel('running'), '生成中', 'task running label')
assertStatus(imageTaskStatusLabel('succeeded'), '成功', 'task succeeded label')
assertStatus(imageTaskStatusTone('succeeded'), 'success', 'task succeeded tone')
assertStatus(imageTaskStatusLabel('partial_failed'), '部分成功', 'task partial failed label')
assertStatus(imageTaskStatusTone('partial_failed'), 'warning', 'task partial failed tone')
assertStatus(imageTaskStatusLabel('rejected'), '已拒绝', 'task rejected label')
assertStatus(imageTaskStatusTone('failed'), 'danger', 'task failed tone')
assertStatus(imageTaskStatusLabel('cancelled'), '已取消', 'task cancelled label')
assertStatus(imageTaskStatusTone('deleted'), 'neutral', 'task deleted tone')

assertStatus(apiKeyStatusLabel('active'), '启用', 'api key active label')
assertStatus(apiKeyStatusTone('active'), 'success', 'api key active tone')
assertStatus(apiKeyStatusLabel('disabled'), '禁用', 'api key disabled label')
assertStatus(apiKeyStatusTone('disabled'), 'warning', 'api key disabled tone')
assertStatus(apiKeyStatusLabel('revoked'), '已撤销', 'api key revoked label')
assertStatus(apiKeyStatusTone('revoked'), 'danger', 'api key revoked tone')
assertStatus(apiKeyStatusLabel('expired'), '已过期', 'api key expired label')
assertStatus(apiKeyStatusTone('expired'), 'warning', 'api key expired tone')

assertStatus(imageTaskTypeLabel('text_to_image'), '文生图', 'task type text-to-image label')
assertStatus(imageTaskTypeLabel('reference_to_image'), '参考生图', 'task type reference label')
assertStatus(imageTaskTypeLabel('reference_generate'), '参考生图', 'task type legacy reference label')
assertStatus(imageTaskTypeLabel('image_edit'), '图片编辑', 'task type edit label')
assertStatus(imageTaskTypeLabel('image_to_image'), '图片编辑', 'task type legacy edit label')

assertStatus(paymentOrderStatusLabel('manual_review'), 'manual_review', 'unknown payment status fallback')
assertStatus(imageTaskStatusLabel(' throttled '), 'throttled', 'unknown task status trims raw value')
assertStatus(apiKeyStatusLabel(''), '未知状态', 'empty api key status fallback')
assertStatus(imageTaskTypeLabel(' video_to_image '), 'video_to_image', 'unknown task type trims raw value')
assertStatus(imageTaskTypeLabel(''), '未知类型', 'empty task type fallback')

const orderRow = userDetailOrderRow({
  id: 7,
  order_no: 'PG202606050001',
  plan_id: 1,
  plan_code: 'points_100',
  plan_name: '100 积分包',
  provider: 'mock',
  status: 'completed',
  currency: 'CNY',
  amount_cny: '29.90000',
  points: '100.00000',
  bonus_points: '0.00000',
  expires_at: '2026-06-05T14:33:00Z',
  created_at: '2026-06-05T14:03:45Z',
  updated_at: '2026-06-05T14:33:00Z',
} satisfies PaymentOrder)
assertStatus(orderRow.orderNo, 'PG202606050001', 'order row order number')
assertStatus(orderRow.statusLabel, '已到账', 'order row localized status')
assertStatus(orderRow.statusTone, 'success', 'order row status tone')
assertStatus(orderRow.amountCny, '29.90000', 'order row amount')
assertStatus(orderRow.points, '100.00000', 'order row points')
assertStatus(orderRow.createdAtLabel, '2026/06/05 14:03', 'order row stable created time')
assertStatus(userDetailOrderRow({ ...orderRowSource(), created_at: 'bad-date' }).createdAtLabel, 'bad-date', 'order row invalid date fallback')

const taskRow = userDetailTaskRow({
  id: 'task-abcdef123456',
  title: '任务',
  prompt: 'prompt',
  task_type: 'reference_to_image',
  status: 'queued',
  route_model_name: 'Flux Pro',
  abstract_model: 'pro',
  model_group: 'pro',
  quality: '2K',
  aspect_ratio: '1:1',
  image_count: 1,
  actual_points: '',
  estimated_points: '12.50000',
  estimate_points: '10.00000',
  progress: 0,
  provider: 'openai',
  route: 'pro',
  created_at: '2026-06-05T14:03:45Z',
  updated_at: '2026-06-05T14:03:45Z',
  reference_assets: [],
  results: [],
} satisfies ImageTask)
assertStatus(taskRow.shortId, 'task-abc', 'task row short id')
assertStatus(taskRow.statusLabel, '排队中', 'task row localized status')
assertStatus(taskRow.statusTone, 'warning', 'task row status tone')
assertStatus(taskRow.typeLabel, '参考生图', 'task row type label')
assertStatus(taskRow.modelLabel, 'Flux Pro', 'task row model prefers route model name')
assertStatus(taskRow.pointsLabel, '12.50000', 'task row falls back to estimated points')

const apiKeyRow = userDetailApiKeyRow({
  id: 'key-1',
  name: '',
  access_key: 'pk_live_123',
  status: 'active',
  scopes: [],
  expires_at: null,
  created_at: '2026-06-05T14:03:45Z',
  last_used_at: null,
} satisfies ApiKey)
assertStatus(apiKeyRow.name, '未命名密钥', 'api key row empty name fallback')
assertStatus(apiKeyRow.statusLabel, '启用', 'api key row localized status')
assertStatus(apiKeyRow.statusTone, 'success', 'api key row status tone')
assertStatus(apiKeyRow.groupCode, '-', 'api key row missing group fallback')
assertStatus(apiKeyRow.accessKey, 'pk_live_123', 'api key row access key')
assertStatus(apiKeyRow.lastUsedAtLabel, '未调用', 'api key row never used label')
assertStatus(
  userDetailApiKeyRow({ ...apiKeyRowSource(), last_used_at: '2026-06-05T14:03:45Z' }).lastUsedAtLabel,
  '2026/06/05 14:03',
  'api key row stable last used time',
)

const bucketRow = userDetailBucketRow({
  bucket: 'trial',
  available_points: '18.00000',
  next_expiring_at: '2026-06-12T00:00:00Z',
  expire_warning: true,
} satisfies BalanceBucket, 0)
assertStatus(bucketRow.key, 'trial-0', 'bucket row stable key')
assertStatus(bucketRow.label, '体验额度', 'bucket row localized fallback label')
assertStatus(bucketRow.availablePoints, '18.00000', 'bucket row available points')
assertStatus(bucketRow.expiresAtLabel, '2026/06/12 00:00', 'bucket row next expiring time')
assertStatus(bucketRow.expiryTone, 'warning', 'bucket row expiry warning tone')
assertStatus(
  userDetailBucketRow({ bucket: 'recharge', available_points: '100.00000' } satisfies BalanceBucket, 1).expiresAtLabel,
  '长期有效',
  'bucket row long lived label',
)
assertStatus(
  userDetailBucketRow({ bucket: 'gift', available_points: '1.00000', expires_at: 'invalid-date' } satisfies BalanceBucket, 2).expiresAtLabel,
  'invalid-date',
  'bucket row invalid date fallback',
)

function assertStatus(actual: string, expected: string, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${expected}, got ${actual}`)
  }
}

function orderRowSource(): PaymentOrder {
  return {
    id: 8,
    order_no: 'PG202606050002',
    plan_id: 1,
    plan_code: 'points_100',
    plan_name: '100 积分包',
    provider: 'mock',
    status: 'pending',
    currency: 'CNY',
    amount_cny: '29.90000',
    points: '100.00000',
    bonus_points: '0.00000',
    expires_at: '2026-06-05T14:33:00Z',
    created_at: '2026-06-05T14:03:45Z',
    updated_at: '2026-06-05T14:33:00Z',
  }
}

function apiKeyRowSource(): ApiKey {
  return {
    id: 'key-2',
    name: 'prod key',
    access_key: 'pk_live_456',
    status: 'active',
    scopes: [],
    expires_at: null,
    created_at: '2026-06-05T14:03:45Z',
    last_used_at: null,
  }
}
