import type { PromptOptimizationEstimate, PromptOptimizationResult } from '../../../shared/api-types'

// Guide kinds mirror the backend promptoptimizer.VideoGuideKind values;
// "universal" is the fallback and never surfaces as a model-specific badge.
const PROMPT_GUIDE_LABELS: Record<string, string> = {
  seedance_2_5: 'Seedance 2.5',
  seedance_2_0: 'Seedance 2.0',
  wan_3_0: '万相3.0',
  minimax_h3: 'MiniMax H3',
}

export function promptGuideLabel(guide?: string): string {
  return guide ? PROMPT_GUIDE_LABELS[guide] ?? '' : ''
}

export type PromptOptimizationState = {
  stage: 'idle' | 'estimating' | 'confirming' | 'optimizing' | 'comparing' | 'applied' | 'error'
  originalPrompt: string
  estimate: PromptOptimizationEstimate | null
  result: PromptOptimizationResult | null
  error: string
}

export function initialPromptOptimizationState(): PromptOptimizationState {
  return { stage: 'idle', originalPrompt: '', estimate: null, result: null, error: '' }
}

export function beginPromptOptimization(state: PromptOptimizationState, prompt: string) {
  if (['estimating', 'confirming', 'optimizing'].includes(state.stage)) return state
  return { ...initialPromptOptimizationState(), stage: 'estimating' as const, originalPrompt: prompt }
}

export function receivePromptEstimate(state: PromptOptimizationState, estimate: PromptOptimizationEstimate) {
  return { ...state, stage: 'confirming' as const, estimate, error: '' }
}

export function confirmPromptOptimization(state: PromptOptimizationState) {
  if (state.stage !== 'confirming' || !state.estimate) return state
  return { ...state, stage: 'optimizing' as const, error: '' }
}

export function receivePromptOptimization(state: PromptOptimizationState, result: PromptOptimizationResult) {
  return { ...state, stage: 'comparing' as const, result, error: '' }
}

export function applyOptimizedPrompt(state: PromptOptimizationState) {
  if (state.stage !== 'comparing' || !state.result) return { state, prompt: state.originalPrompt }
  return { state: { ...state, stage: 'applied' as const }, prompt: state.result.optimized_prompt }
}

export function undoPromptOptimization(state: PromptOptimizationState, currentPrompt: string) {
  if (state.stage !== 'applied') return { state, prompt: currentPrompt }
  return { state: initialPromptOptimizationState(), prompt: state.originalPrompt }
}

export function failPromptOptimization(state: PromptOptimizationState, error: string) {
  return { ...state, stage: 'error' as const, error }
}
