import { existsSync, readFileSync } from 'node:fs'

const streamURL = new URL('./workspaceTaskStream.ts', import.meta.url)
if (!existsSync(streamURL)) {
  throw new Error('workspace task stream needs an executable bounded recovery model')
}

const streamModel = await import('./workspaceTaskStream')
const {
  WORKSPACE_STREAM_MAX_RETRIES,
  closeWorkspaceStreamGeneration,
  createWorkspaceStreamGeneration,
  nextWorkspaceStreamRetry,
  workspaceStreamEventIsCurrent,
  workspaceStreamRecoveryIsCurrent,
} = streamModel
const markWorkspaceStreamHealthy = (streamModel as Record<string, unknown>).markWorkspaceStreamHealthy
if (typeof markWorkspaceStreamHealthy !== 'function') {
  throw new Error('workspace stream model must expose markWorkspaceStreamHealthy')
}

if (WORKSPACE_STREAM_MAX_RETRIES !== 3) {
  throw new Error(`workspace stream must allow the first connection plus 3 retries, got ${WORKSPACE_STREAM_MAX_RETRIES}`)
}

const generation = createWorkspaceStreamGeneration('old-access-token')
const retries = [1, 2, 3].map(() => nextWorkspaceStreamRetry(generation))
if (retries.some((item) => !item.retry) || generation.retryCount !== 3) {
  throw new Error(`the first three stream recoveries must remain available, got ${JSON.stringify({ retries, generation })}`)
}
const exhausted = nextWorkspaceStreamRetry(generation)
if (exhausted.retry || generation.retryCount !== 3) {
  throw new Error(`stream recovery must stop after 3 retries, got ${JSON.stringify({ exhausted, generation })}`)
}
markWorkspaceStreamHealthy(generation)
if (Number(generation.retryCount) !== 0) {
  throw new Error(`an actually healthy stream must reset its recovery budget, got ${generation.retryCount}`)
}
const afterHealthy = nextWorkspaceStreamRetry(generation)
if (!afterHealthy.retry || afterHealthy.attempt !== 1 || Number(generation.retryCount) !== 1) {
  throw new Error(`the next independent disconnect must restart at attempt 1, got ${JSON.stringify({ afterHealthy, generation })}`)
}

const refreshed = createWorkspaceStreamGeneration('new-access-token')
if (refreshed.id === generation.id || refreshed.token === generation.token || refreshed.retryCount !== 0) {
  throw new Error('a refreshed access token must create a distinct stream generation and URL input')
}
if (workspaceStreamEventIsCurrent(generation, refreshed)) {
  throw new Error('callbacks from a closed old-token source must not commit into the refreshed stream')
}
if (!workspaceStreamEventIsCurrent(refreshed, refreshed)) {
  throw new Error('callbacks from the active stream generation must remain valid')
}
closeWorkspaceStreamGeneration(refreshed)
markWorkspaceStreamHealthy(refreshed)
if (workspaceStreamEventIsCurrent(refreshed, refreshed)) {
  throw new Error('marking a closed old source healthy must not let its queued callbacks commit')
}
if (!workspaceStreamRecoveryIsCurrent(refreshed, refreshed)) {
  throw new Error('a closed source recovery must remain current until a newer generation replaces it')
}

const pageSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
for (const required of [
  'streamRef.current?.close()',
  'nextWorkspaceStreamRetry',
  'markWorkspaceStreamHealthy',
  'closeWorkspaceStreamGeneration',
  'workspaceStreamEventIsCurrent',
  'workspaceStreamRecoveryIsCurrent',
  'await userApi.listTasks',
  'app.session?.token',
  "source.addEventListener('open'",
  'markStreamHealthy()',
  'streamTokenRef.current !== token',
]) {
  if (!pageSource.includes(required)) {
    throw new Error(`workspace SSE recovery must include ${required}`)
  }
}

const historyHandlerStart = pageSource.indexOf("source.addEventListener('history'")
const taskHandlerStart = pageSource.indexOf("source.addEventListener('task'")
const errorHandlerStart = pageSource.indexOf("source.addEventListener('error'")
const historyHandler = pageSource.slice(historyHandlerStart, taskHandlerStart)
const taskHandler = pageSource.slice(taskHandlerStart, errorHandlerStart)
if (!historyHandler.includes('markStreamHealthy()') || !taskHandler.includes('markStreamHealthy()')) {
  throw new Error('valid history and task events must mark the current stream healthy')
}
if (taskHandler.indexOf('markStreamHealthy()') > taskHandler.indexOf("next.project_id !== selectedProjectID")) {
  throw new Error('valid events from another project must still mark the SSE connection healthy before list filtering')
}
if (taskHandler.indexOf('refreshAccountRef.current()') > taskHandler.indexOf("next.project_id !== selectedProjectID")) {
  throw new Error('terminal events from another project must still refresh the global account balance')
}

const restRecoveryStart = pageSource.indexOf('const tasks = await userApi.listTasks({ project_id: selectedProjectID })')
const restRecoveryEnd = pageSource.indexOf('} catch {', restRecoveryStart)
if (pageSource.slice(restRecoveryStart, restRecoveryEnd).includes('markStreamHealthy')) {
  throw new Error('REST compensation success must not reset the SSE recovery budget')
}

if (/source\.onerror\s*=\s*\(\)\s*=>\s*\{[\s\S]*?new EventSource/.test(pageSource)) {
  throw new Error('EventSource errors must not reconnect directly with the stale access-token URL')
}
