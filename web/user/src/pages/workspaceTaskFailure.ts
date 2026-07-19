import type { ImageTask, ImageTaskStatus, ImageTaskType } from '../../../shared/api-types'

export type WorkspaceTaskFailureMeta = {
  label: string
  value: string
}

export type WorkspaceTaskFailureView = {
  title: string
  reason: string
  meta: WorkspaceTaskFailureMeta[]
}

export type WorkspaceTaskCardView = {
  taskTypeLabel: string
  createdAtLabel: string
  statusLabel: string
  statusTone: 'success' | 'warning' | 'danger' | 'neutral'
}

export type WorkspaceTaskPendingView = {
  title: string
  detail: string
}

const taskTypeLabels: Record<string, string> = {
  text_to_image: '文生图',
  image_edit: '图片编辑',
}

const taskStatusLabels: Record<string, string> = {
  queued: '排队中',
  running: '生成中',
  succeeded: '已完成',
  partial_failed: '部分完成',
  failed: '失败',
  cancelled: '已取消',
  rejected: '已拒绝',
  deleted: '已删除',
}

export function workspaceTaskCardView(task: ImageTask): WorkspaceTaskCardView {
  return {
    taskTypeLabel: workspaceTaskTypeLabel(task.task_type),
    createdAtLabel: workspaceTaskDateTime(task.created_at),
    statusLabel: workspaceTaskStatusLabel(task.status),
    statusTone: workspaceTaskStatusTone(task.status),
  }
}

export function workspaceTaskPendingView(task: ImageTask): WorkspaceTaskPendingView {
  const stage = task.progress_stage?.trim().toLowerCase()
  const stageTitle = stage === 'provider' || stage === 'routing' || stage === 'running'
    ? '正在生成图片'
    : stage === 'persisting'
      ? '正在保存结果'
      : stage === 'settling'
        ? '正在结算积分'
        : null
  return {
    title: task.status === 'queued' ? '排队中' : task.status === 'running' ? stageTitle ?? '生成中' : '等待结果',
    detail: task.progress_message?.trim() || '任务状态会通过实时连接自动更新，完成后图片会出现在这里。',
  }
}

export function workspaceTaskFailureView(task: ImageTask): WorkspaceTaskFailureView {
  const errorCode = task.error_code?.trim()
  const traceID = task.request_id?.trim() || task.id
  return {
    title: task.status === 'failed' || task.status === 'rejected' ? '生成失败' : '没有可用结果',
    reason: userFriendlyFailureReason(task),
    meta: [
      errorCode ? { label: '错误码', value: errorCode } : null,
      traceID ? { label: '追踪 ID', value: traceID } : null,
    ].filter((item): item is WorkspaceTaskFailureMeta => Boolean(item)),
  }
}

function userFriendlyFailureReason(task: ImageTask) {
  const errorCode = task.error_code?.trim().toUpperCase()
  const reason = task.failure_reason?.trim() || task.error_message?.trim()
  if (errorCode === 'INSUFFICIENT_POINTS' || errorCode === 'BALANCE_INSUFFICIENT') {
    return '积分余额不足，本次任务未扣费。请充值或兑换积分后再试。'
  }
  if (errorCode === 'MODEL_ROUTE_NOT_FOUND' || errorCode === 'MODEL_ROUTE_UNAVAILABLE' || errorCode === 'MODEL_ROUTE_NO_CANDIDATE') {
    return '平台生图能力正在配置中，本次任务未扣费。请稍后再试。'
  }
  if (errorCode === 'MODEL_PRICE_NOT_FOUND' || errorCode === 'PRICE_NOT_FOUND') {
    return '平台计费配置正在更新，本次任务未扣费。请稍后再试。'
  }
  if (errorCode === 'PROVIDER_UNAVAILABLE' || errorCode === 'PROVIDER_ERROR') {
    return '图片服务暂时繁忙，本次任务未扣费。请稍后重试。'
  }
  if (errorCode === 'PROVIDER_TIMEOUT' || errorCode === 'TASK_TIMEOUT') {
    return '生成等待超时，本次任务未扣费。请稍后重试。'
  }
  if (!reason) return '本次任务没有生成图片，请调整提示词、参考图或参数后重试。'
  const normalized = reason.toLowerCase()
  if (normalized.includes('insufficient') || normalized.includes('balance') || reason.includes('余额不足') || reason.includes('积分不足')) {
    return '积分余额不足，本次任务未扣费。请充值或兑换积分后再试。'
  }
  if (normalized.includes('route') || normalized.includes('model routing') || reason.includes('路由') || reason.includes('模型')) {
    return '平台生图能力正在配置中，本次任务未扣费。请稍后再试。'
  }
  if (normalized.includes('price') || reason.includes('价格')) {
    return '平台计费配置正在更新，本次任务未扣费。请稍后再试。'
  }
  if (normalized.includes('provider') || normalized.includes('upstream') || reason.includes('供应商')) {
    return '图片服务暂时繁忙，本次任务未扣费。请稍后重试。'
  }
  if (normalized.includes('timeout') || reason.includes('超时')) {
    return '生成等待超时，本次任务未扣费。请稍后重试。'
  }
  return reason
}

function workspaceTaskTypeLabel(type: ImageTaskType | string) {
  return taskTypeLabels[type] ?? type
}

function workspaceTaskStatusLabel(status: ImageTaskStatus | string) {
  return taskStatusLabels[status] ?? status
}

function workspaceTaskStatusTone(status: ImageTaskStatus | string): WorkspaceTaskCardView['statusTone'] {
  if (status === 'succeeded') return 'success'
  if (status === 'failed' || status === 'rejected' || status === 'cancelled' || status === 'deleted') return 'danger'
  if (status === 'queued' || status === 'running' || status === 'partial_failed') return 'warning'
  return 'neutral'
}

function workspaceTaskDateTime(value?: string) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
