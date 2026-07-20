// @ts-ignore contract scripts run in tsx/node; the browser app tsconfigs do not include node types.
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../../scripts/e2e/docker-e2e.mjs', import.meta.url), 'utf8')
const browserSource = readFileSync(new URL('../../scripts/e2e/prompt-workflow-browser.py', import.meta.url), 'utf8')
const runnerSource = readFileSync(new URL('../../scripts/e2e/run-docker-e2e.sh', import.meta.url), 'utf8')

for (const required of [
  'BASE_URL="${BASE_URL:-http://127.0.0.1:8088}"',
  'USER_WEB_URL="${USER_WEB_URL:-http://127.0.0.1:8088}"',
  'ADMIN_WEB_URL="${ADMIN_WEB_URL:-http://127.0.0.1:8088/admin}"',
  'STATE_HELPER',
  'snapshot',
  'restore',
  'trap cleanup EXIT',
]) {
  if (!runnerSource.includes(required)) throw new Error(`shared local E2E runner is missing: ${required}`)
}
if (runnerSource.includes('down -v') || runnerSource.includes('CLEAN_STACK=true')) {
  throw new Error('shared local E2E runner must not expose a volume-destructive clean mode')
}

for (const required of [
  "req.url === '/v1/chat/completions'",
  "req.url === '/v1/responses'",
  'max_completion_tokens',
  'max_output_tokens',
  'async function disableExistingRouteCandidates(routeModelId)',
  '/api/ops/admin/v1/route-models/${routeModelId}/candidates',
  '/api/ops/admin/v1/route-models/${routeModelId}/candidates/${candidate.id}',
  'await disableExistingRouteCandidates(state.ids.basicRouteModelId)',
  'await disableExistingRouteCandidates(state.ids.compatRouteModelId)',
  'async function happyPathPromptOptimization()',
  '/api/ops/admin/v1/text-model-accounts',
  "createAccount('Chat', 'chat_completions')",
  "createAccount('Responses', 'responses')",
  '/api/ops/admin/v1/text-models/${chatModel.id}:test',
  '/api/ops/admin/v1/text-models/${responsesModel.id}:test',
  '/api/ops/admin/v1/text-models/${chatModel.id}:default',
  '/api/agent/text/v1/prompt-optimizations/estimate',
  '/api/agent/text/v1/prompt-optimizations',
  "estimated_points !== '0.00000'",
  "actual_points !== '0.00000'",
  "task_type: 'reference_generate'",
  "error?.code !== 'BAD_REQUEST'",
  'state.ids.textModelId = String(chatModel.id)',
  "template.includes('/text-model')",
  "await step('text model accounts and prompt optimization happy path', happyPathPromptOptimization)",
]) {
  if (!source.includes(required)) throw new Error(`Docker E2E prompt optimization coverage is missing: ${required}`)
}

for (const required of [
  'system-settings?tab=text-models',
  'Docker E2E Text Chat',
  '展开提示词编辑器',
  '优化提示词',
  '预计 0.00000 积分',
  '确认优化',
  '应用优化',
  '撤销提示词优化',
  '复制 Prompt',
  '复用配置',
  'set_viewport_size({"width": 390, "height": 844})',
  'assert_no_overlap',
  'screenshot',
]) {
  if (!browserSource.includes(required)) throw new Error(`Browser prompt workflow coverage is missing: ${required}`)
}

if (!source.includes("await step('browser prompt optimization and configuration reuse workflow', browserPromptWorkflow)")) {
  throw new Error('Docker E2E must execute the browser prompt workflow')
}
