import { ApiError } from '../../../../shared/http-client'

export type VideoFieldErrors = Record<string, string>

type FieldError = { field?: unknown; code?: unknown; rule?: unknown; message?: unknown; name?: unknown }

const fieldLabels: Record<string, string> = {
  task_type: '生成方式', prompt: '提示词', prompt_template: '提示词', prompt_variables: '变量', duration_seconds: '时长',
  resolution: '清晰度', aspect_ratio: '比例', generate_audio: '音频模式', output_count: '数量',
  'inputs.first_frame': '首帧', 'inputs.last_frame': '尾帧',
}

export function videoFieldErrors(error: unknown): VideoFieldErrors {
  if (!(error instanceof ApiError) || !error.details) return {}
  const candidates = Array.isArray(error.details.field_errors)
    ? error.details.field_errors
    : typeof error.details.field === 'string' ? [error.details] : []
  const result: VideoFieldErrors = {}
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    const item = candidate as FieldError
    const field = typeof item.field === 'string' ? item.field : ''
    if (!field) continue
    result[field] = fieldErrorMessage(field, typeof item.code === 'string' ? item.code : typeof item.rule === 'string' ? item.rule : '', typeof item.name === 'string' ? item.name : '')
  }
  return result
}

function fieldErrorMessage(field: string, rule: string, name: string) {
  if (field === 'prompt_variables' && name) return `变量“${name}”尚未填写`
  if (field === 'inputs.first_frame' && rule === 'required') return '请选择首帧图片'
  if (field === 'inputs.last_frame' && rule === 'required') return '请选择尾帧图片'
  if (field === 'inputs.first_frame.size_bytes') return '首帧文件超过当前模型限制'
  if (field === 'inputs.last_frame.size_bytes') return '尾帧文件超过当前模型限制'
  if (field.endsWith('.format')) return `${inputRoleLabel(field)}格式不受当前模型支持`
  if (field.endsWith('.media_type')) return `${inputRoleLabel(field)}类型不受当前模型支持`
  const label = fieldLabels[field] ?? field
  if (rule === 'required') return `请填写${label}`
  if (rule === 'too_large' || rule === 'too_long') return `${label}超过当前模型限制`
  if (rule === 'out_of_range') return `${label}超出允许范围`
  return `当前模型不支持所选${label}`
}

function inputRoleLabel(field: string) {
  return field.includes('last_frame') ? '尾帧' : '首帧'
}
