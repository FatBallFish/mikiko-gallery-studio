import type { Capability, CapabilityModelGroup, EstimateResult, ImageTask, ImageTaskType } from '../../../shared/api-types'
import { displayPoints, publicUnavailableReason, workspaceGenerateReadiness } from './workspaceGenerateReadiness'
import { workspaceReferenceMaximum } from './workspaceReferenceLimit'
import { workspaceTaskFailureView, workspaceTaskPendingView } from './workspaceTaskFailure'
import { generationSlots, workspaceProgressNodes, type WorkspaceProgressNode } from './workspaceTaskProgress'

type WorkspaceViewModelInput = {
  capability: Capability | null
  taskType: ImageTaskType
  referenceCount: number
  requiredReferencesReady: boolean
  selectedModelCode: string
  parametersReady: boolean
  prompt: string
  estimatePending: boolean
  estimateError: string
  estimate: EstimateResult | null
  busy: boolean
  task: ImageTask | null
}

export type WorkspaceParameterOption = { value: string; label: string; detail?: string }

export type WorkspaceParameterModel = {
  taskTypes: ImageTaskType[]
  models: WorkspaceParameterOption[]
  baseResolutions: string[]
  aspectRatios: string[]
  counts: number[]
  referenceLimit: number
}

export type WorkspaceTaskView = {
  state: 'idle' | 'queued' | 'running' | 'partial' | 'success' | 'failure'
  title: string
  detail: string
  rail: WorkspaceProgressNode[]
  resultCount: number
  requestedCount: number
}

export function matchWorkspaceCapabilityOption(options: string[], candidate?: string) {
  const normalized = candidate?.trim().toLowerCase()
  if (!normalized) return undefined
  return options.find((option) => option.toLowerCase() === normalized)
}

function taskModels(capability: Capability | null, taskType: ImageTaskType) {
  return capability?.model_groups.filter((model) => (
    model.task_types.includes(taskType)
    && Boolean(model.base_resolution?.length)
    && Boolean(model.aspect_ratios?.length || capability.aspect_ratios.length)
  )) ?? []
}

function selectedModel(models: CapabilityModelGroup[], code: string) {
  return models.find((model) => model.code === code)
}

function taskView(task: ImageTask | null): WorkspaceTaskView {
  if (!task) return { state: 'idle', title: '准备就绪', detail: '配置参数后开始一次新创作。', rail: [], resultCount: 0, requestedCount: 0 }
  const rail = workspaceProgressNodes(task)
  const resultCount = task.results.length
  const requestedCount = generationSlots(task).length
  if (task.status === 'queued') {
    const pending = workspaceTaskPendingView(task)
    return { state: 'queued', title: pending.title, detail: pending.detail, rail, resultCount, requestedCount }
  }
  if (task.status === 'running') {
    const pending = workspaceTaskPendingView(task)
    return { state: 'running', title: pending.title, detail: pending.detail, rail, resultCount, requestedCount }
  }
  if (task.status === 'partial_failed') {
    return { state: 'partial', title: '部分完成', detail: `已生成 ${resultCount}/${requestedCount} 张图片，未完成部分不会按成功结果计费。`, rail, resultCount, requestedCount }
  }
  if (task.status === 'succeeded') {
    return { state: 'success', title: '创作完成', detail: `${resultCount} 张结果已保存到资产。`, rail, resultCount, requestedCount }
  }
  const failure = workspaceTaskFailureView(task)
  return { state: 'failure', title: failure.title, detail: failure.reason, rail, resultCount, requestedCount }
}

export function createWorkspaceViewModel(input: WorkspaceViewModelInput) {
  const models = taskModels(input.capability, input.taskType)
  const model = selectedModel(models, input.selectedModelCode)
  const readiness = workspaceGenerateReadiness({
    busy: input.busy,
    hasModel: Boolean(model),
    taskType: input.taskType,
    referenceCount: input.referenceCount,
    requiredReferencesReady: input.requiredReferencesReady,
    unavailableReason: input.capability?.unavailable_reason,
    parametersReady: input.parametersReady,
    prompt: input.prompt,
    estimate: input.estimate,
    estimateError: input.estimateError,
  })
  const estimate = input.estimateError
    ? { state: 'error' as const, label: '预估失败', detail: input.estimateError }
    : input.estimatePending || (!input.estimate && Boolean(model) && input.parametersReady)
      ? { state: 'loading' as const, label: '正在预估', detail: '参数变化后会自动更新积分。' }
      : input.estimate && !input.estimate.sufficient
        ? { state: 'insufficient' as const, label: `${input.estimate.display_points ?? displayPoints(input.estimate.points)} 积分`, detail: `余额不足，还差 ${displayPoints(input.estimate.insufficient_points)} 积分。` }
        : input.estimate
          ? { state: 'ready' as const, label: `${input.estimate.display_points ?? displayPoints(input.estimate.points)} 积分`, detail: '生成前仅冻结预估积分，最终按实际结果结算。' }
          : { state: 'unavailable' as const, label: '等待参数', detail: readiness.reason }

  return {
    capability: models.length
      ? { state: 'ready' as const, detail: `${models.length} 个模型可用于当前任务。` }
      : { state: 'unavailable' as const, detail: publicUnavailableReason(input.capability?.unavailable_reason) },
    parameters: {
      taskTypes: input.capability?.task_types ?? [],
      models: models.map((item) => ({ value: item.code, label: item.name, detail: item.description })),
      baseResolutions: model?.base_resolution?.length ? model.base_resolution : [],
      aspectRatios: model?.aspect_ratios?.length ? model.aspect_ratios : input.capability?.aspect_ratios ?? [],
      counts: Array.from({ length: Math.max(0, Number(model?.max_output_image_count ?? input.capability?.max_image_count ?? 0)) }, (_, index) => index + 1),
      referenceLimit: workspaceReferenceMaximum(model?.max_reference_image_count),
    } satisfies WorkspaceParameterModel,
    estimate,
    generate: readiness,
    task: taskView(input.task),
  }
}
