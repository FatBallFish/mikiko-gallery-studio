import type { ImageTask } from '../../../shared/api-types'
import { workspaceTaskCardView, workspaceTaskFailureView, workspaceTaskPendingView } from './workspaceTaskFailure'

const failedTask = {
  id: 'task-failed-001',
  status: 'failed',
  failure_reason: 'MODEL_ROUTE_NOT_FOUND: route model missing',
  error_code: 'MODEL_ROUTE_NOT_FOUND',
  request_id: 'req-123',
} as ImageTask

const failedView = workspaceTaskFailureView(failedTask)

if (failedView.title !== '生成失败') {
  throw new Error(`failed task title should be 生成失败, got ${failedView.title}`)
}

if (failedView.reason.includes('route') || failedView.reason.includes('MODEL_ROUTE_NOT_FOUND')) {
  throw new Error(`failure reason must hide internal routing details, got ${failedView.reason}`)
}

if (!failedView.meta.some((item) => item.label === '错误码' && item.value === 'MODEL_ROUTE_NOT_FOUND')) {
  throw new Error(`failure meta should include error_code, got ${JSON.stringify(failedView.meta)}`)
}

if (!failedView.meta.some((item) => item.label === '追踪 ID' && item.value === 'req-123')) {
  throw new Error(`failure meta should prefer request_id as trace id, got ${JSON.stringify(failedView.meta)}`)
}

const rejectedWithoutRequest = workspaceTaskFailureView({
  id: 'task-rejected-001',
  status: 'rejected',
  failure_reason: '余额不足',
  error_code: 'INSUFFICIENT_POINTS',
} as ImageTask)

if (rejectedWithoutRequest.title !== '生成失败') {
  throw new Error(`rejected task should still be titled 生成失败, got ${rejectedWithoutRequest.title}`)
}

if (!rejectedWithoutRequest.meta.some((item) => item.label === '追踪 ID' && item.value === 'task-rejected-001')) {
  throw new Error(`failure meta should fall back to task id, got ${JSON.stringify(rejectedWithoutRequest.meta)}`)
}

if (!rejectedWithoutRequest.reason.includes('积分余额不足')) {
  throw new Error(`insufficient points should map to user-friendly reason, got ${rejectedWithoutRequest.reason}`)
}

const emptyTerminal = workspaceTaskFailureView({
  id: 'task-cancelled-001',
  status: 'cancelled',
} as ImageTask)

if (emptyTerminal.title !== '没有可用结果') {
  throw new Error(`non-failed terminal task should be titled 没有可用结果, got ${emptyTerminal.title}`)
}

const card = workspaceTaskCardView(task({
  task_type: 'image_edit',
  status: 'partial_failed',
  route_model_code: 'plus',
  created_at: '2026-06-05T13:45:30Z',
}))

if (card.taskTypeLabel !== '图片编辑' || card.createdAtLabel !== '2026/06/05 13:45' || card.statusLabel !== '部分完成') {
  throw new Error(`workspace task card should localize type/date/status, got ${JSON.stringify(card)}`)
}

if (/T|Z$/.test(`${card.createdAtLabel}${card.statusLabel}`)) {
  throw new Error(`workspace task card should not expose ISO separators, got ${JSON.stringify(card)}`)
}

const invalidDateCard = workspaceTaskCardView(task({ created_at: 'not-a-date' }))
if (invalidDateCard.createdAtLabel !== 'not-a-date') {
  throw new Error(`workspace task card should preserve invalid dates for troubleshooting, got ${invalidDateCard.createdAtLabel}`)
}

const queued = workspaceTaskPendingView(task({ status: 'queued' }))
const running = workspaceTaskPendingView(task({ status: 'running' }))
const unknownPending = workspaceTaskPendingView(task({ status: 'processing' as ImageTask['status'] }))
if (queued.title !== '排队中' || running.title !== '生成中' || unknownPending.title !== '等待结果') {
  throw new Error(`workspace pending state should use localized titles, got ${JSON.stringify({ queued, running, unknownPending })}`)
}

const provider = workspaceTaskPendingView(task({ status: 'running', progress_stage: 'provider', progress_message: '正在调用 Studio 生成图片' }))
const persisting = workspaceTaskPendingView(task({ status: 'running', progress_stage: 'persisting' }))
const settling = workspaceTaskPendingView(task({ status: 'running', progress_stage: 'settling' }))
if (provider.title !== '正在生成图片' || provider.detail !== '正在调用 Studio 生成图片') {
  throw new Error(`provider progress should use the persisted backend message, got ${JSON.stringify(provider)}`)
}
if (persisting.title !== '正在保存结果' || settling.title !== '正在结算积分') {
  throw new Error(`real persistence and settlement stages should have distinct titles, got ${JSON.stringify({ persisting, settling })}`)
}

function task(patch: Partial<ImageTask>): ImageTask {
  return {
    id: patch.id ?? 'task-card-001',
    title: patch.title ?? '测试任务',
    prompt: patch.prompt ?? '生成一张测试图',
    task_type: patch.task_type ?? 'text_to_image',
    status: patch.status ?? 'queued',
    progress_stage: patch.progress_stage,
    progress_message: patch.progress_message,
    route_model_code: patch.route_model_code,
    model_group: patch.model_group ?? 'plus',
    base_resolution: patch.base_resolution ?? '2K',
    quality: patch.quality ?? 'auto',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    image_count: patch.image_count ?? 1,
    estimate_points: patch.estimate_points ?? '1.00000',
    progress: patch.progress ?? 0,
    provider: patch.provider ?? 'openai',
    route: patch.route ?? 'default',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
    reference_assets: patch.reference_assets ?? [],
    results: patch.results ?? [],
  }
}
