import {
  applyOptimizedPrompt,
  beginPromptOptimization,
  confirmPromptOptimization,
  failPromptOptimization,
  initialPromptOptimizationState,
  receivePromptEstimate,
  receivePromptOptimization,
  undoPromptOptimization,
} from './workspacePromptOptimization'

let state = initialPromptOptimizationState()
state = beginPromptOptimization(state, 'a portrait in summer rain')
if (state.stage !== 'estimating' || state.originalPrompt !== 'a portrait in summer rain') throw new Error('optimization must begin with estimate')
const duplicate = beginPromptOptimization(state, 'different prompt')
if (duplicate !== state) throw new Error('duplicate optimization must be ignored while busy')
state = receivePromptEstimate(state, { quote: 'quote', expires_at: '2026-07-20T00:00:00Z', estimated_points: '0.00000', model: { id: 1, model_code: 'gpt', display_name: 'GPT', api_style: 'responses' } })
if (state.stage !== 'confirming' || state.estimate?.estimated_points !== '0.00000') throw new Error('zero-point estimate must still require confirmation')
state = confirmPromptOptimization(state)
if (state.stage !== 'optimizing') throw new Error('confirm must start optimization')
state = receivePromptOptimization(state, { run_id: 'run', optimized_prompt: 'A cinematic portrait in warm summer rain', input_tokens: 8, output_tokens: 12, estimated_points: '0.00000', actual_points: '0.00000' })
if (state.stage !== 'comparing') throw new Error('result must be compared before applying')
const applied = applyOptimizedPrompt(state)
if (applied.prompt !== 'A cinematic portrait in warm summer rain' || applied.state.stage !== 'applied') throw new Error('apply must explicitly change the prompt')
const undone = undoPromptOptimization(applied.state, applied.prompt)
if (undone.prompt !== 'a portrait in summer rain' || undone.state.stage !== 'idle') throw new Error('undo must restore original once')
const failed = failPromptOptimization(beginPromptOptimization(initialPromptOptimizationState(), 'original prompt'), 'network failed')
if (failed.stage !== 'error' || failed.originalPrompt !== 'original prompt') throw new Error('failure must preserve original prompt')
