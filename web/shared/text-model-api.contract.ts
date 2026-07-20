import { adminApi } from './admin-api'
import {
  API_PATHS,
  type ImageTaskType,
  type PromptOptimizationEstimate,
  type PromptOptimizationResult,
  type TextModel,
  type TextModelAccount,
  type TextModelConnectionTest,
} from './api-types'
import { userApi } from './user-api'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

assert(API_PATHS.ops.textModelAccounts === '/api/ops/admin/v1/text-model-accounts', 'text model account path drifted')
assert(API_PATHS.ops.textModelDefault === '/api/ops/admin/v1/text-models/{model_id}:default', 'default text model path drifted')
assert(API_PATHS.ops.textModelTest === '/api/ops/admin/v1/text-models/{model_id}:test', 'text model test path drifted')
assert(API_PATHS.agent.promptOptimizationEstimate === '/api/agent/text/v1/prompt-optimizations/estimate', 'prompt estimate path drifted')
assert(API_PATHS.agent.promptOptimizations === '/api/agent/text/v1/prompt-optimizations', 'prompt optimization path drifted')

const adminMethods: Array<keyof typeof adminApi> = [
  'listTextModelAccounts', 'createTextModelAccount', 'updateTextModelAccount', 'deleteTextModelAccount',
  'listTextModels', 'createTextModel', 'updateTextModel', 'deleteTextModel', 'setDefaultTextModel', 'testTextModel',
]
const userMethods: Array<keyof typeof userApi> = ['estimatePromptOptimization', 'optimizePrompt']
assert(adminMethods.length === 10 && userMethods.length === 2, 'text model client methods are incomplete')

const account = null as unknown as TextModelAccount
const model = null as unknown as TextModel
const probe = null as unknown as TextModelConnectionTest
const estimate = null as unknown as PromptOptimizationEstimate
const result = null as unknown as PromptOptimizationResult
void [account, model, probe, estimate, result]

const supportedTaskTypes: ImageTaskType[] = ['text_to_image', 'image_edit']
assert(supportedTaskTypes.length === 2, 'removed reference generation task type must not return')
