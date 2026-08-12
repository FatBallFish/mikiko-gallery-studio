import { readFileSync } from 'node:fs'
import type { ImageTask } from '../../../shared/api-types'
import { homeContinuationView, homeRecentTaskView } from './homeGalleryModel'

const failed = task({ id: 'task_failed', status: 'failed' })
const running = task({ id: 'task_running', status: 'running' })

const failedContinuation = homeContinuationView([failed])
const runningContinuation = homeContinuationView([running])
if (failedContinuation.taskId !== failed.id || failedContinuation.action !== 'retry') {
  throw new Error(`failed Home continuation must stay bound to the failed task, got ${JSON.stringify(failedContinuation)}`)
}
if (runningContinuation.taskId !== running.id || runningContinuation.action !== 'continue') {
  throw new Error(`running Home continuation must stay bound to the running task, got ${JSON.stringify(runningContinuation)}`)
}

const failedRecent = homeRecentTaskView(failed, false)
const runningRecent = homeRecentTaskView(running, false)
if (failedRecent.action === runningRecent.action || failedRecent.action !== 'retry' || runningRecent.action !== 'continue') {
  throw new Error(`failed and running recent tasks need distinct actions, got ${JSON.stringify({ failedRecent, runningRecent })}`)
}

const homeSource = readFileSync(new URL('./HomePage.tsx', import.meta.url), 'utf8')
for (const required of [
  'recent.action',
  'continuation.taskId',
  "app.navigate(continuation.route, { taskId: continuation.taskId })",
  "app.navigate('genpic', { taskId: latestTask?.id })",
]) {
  if (!homeSource.includes(required)) {
    throw new Error(`Home must render and preserve task-bound continuation contract: ${required}`)
  }
}

const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8')
for (const required of [
  'routeTaskId',
  '<CreationPage initialTaskId={routeTaskId}',
]) {
  if (!appSource.includes(required)) throw new Error(`App must pass workspace task context: ${required}`)
}

const creationSource = readFileSync(new URL('../features/creation/CreationPage.tsx', import.meta.url), 'utf8')
if (!creationSource.includes('<ImageCreationPanel initialTaskId={initialMedia === \'video\' ? undefined : initialTaskId} />')) {
  throw new Error('CreationPage must preserve image task context through the multimedia shell')
}

const imagePanelSource = readFileSync(new URL('../features/creation/ImageCreationPanel.tsx', import.meta.url), 'utf8')
if (!imagePanelSource.includes('<WorkspacePage initialTaskId={initialTaskId} />')) {
  throw new Error('ImageCreationPanel must pass image task context to WorkspacePage')
}

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
for (const required of [
  'initialTaskId?: string',
  'userApi.getTask(taskId)',
  'selectedTaskId',
  'initialTaskError',
  'userApi.retryTask(task.id)',
  "app.navigate('genpic', { taskId: nextTask.id })",
  "app.navigate('genpic', { taskId: retry.id })",
]) {
  if (!workspaceSource.includes(required)) throw new Error(`Workspace must load and select the requested task with status/retry context: ${required}`)
}

function task(patch: Partial<ImageTask>): ImageTask {
  return {
    id: patch.id ?? 'task_1',
    title: patch.title ?? '最近任务',
    prompt: patch.prompt ?? 'cinematic image',
    task_type: patch.task_type ?? 'text_to_image',
    status: patch.status ?? 'queued',
    model_group: patch.model_group ?? 'plus',
    quality: patch.quality ?? 'auto',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    image_count: patch.image_count ?? 1,
    estimate_points: patch.estimate_points ?? '1.00000',
    progress: patch.progress ?? 0,
    provider: patch.provider ?? 'openai',
    route: patch.route ?? 'default',
    created_at: patch.created_at ?? '2026-07-10T08:00:00Z',
    updated_at: patch.updated_at ?? '2026-07-10T08:00:00Z',
    reference_assets: patch.reference_assets ?? [],
    results: patch.results ?? [],
  }
}
