import type { ImageTaskType } from '../../../shared/api-types'

export const WORKSPACE_REFERENCE_MAX_COUNT = 4

export function workspaceRequiredReferencesReady(taskType: ImageTaskType, referenceCount: number) {
  return taskType === 'text_to_image' || Math.max(0, Math.floor(referenceCount)) >= 1
}

export function workspaceReferenceMaximum(modelMaximum?: number, globalMaximum = WORKSPACE_REFERENCE_MAX_COUNT) {
  const globalLimit = Math.max(0, Math.floor(globalMaximum))
  if (modelMaximum === undefined) return globalLimit
  return Math.min(globalLimit, Math.max(0, Math.floor(modelMaximum)))
}

export function remainingReferenceCapacity(maximum: number, currentCount: number) {
  return Math.max(0, Math.floor(maximum) - Math.max(0, Math.floor(currentCount)))
}

export function limitReferenceSelection<T>(items: T[], remaining: number) {
  const capacity = Math.max(0, Math.floor(remaining))
  const accepted = items.slice(0, capacity)
  return {
    accepted,
    rejectedCount: Math.max(0, items.length - accepted.length),
  }
}

export function singleReferenceAddition<T>(item: T, maximum: number, currentCount: number) {
  const remaining = remainingReferenceCapacity(maximum, currentCount)
  const limited = limitReferenceSelection([item], remaining)
  return {
    item: limited.accepted[0] ?? null,
    remaining,
    rejected: limited.rejectedCount > 0,
  }
}
