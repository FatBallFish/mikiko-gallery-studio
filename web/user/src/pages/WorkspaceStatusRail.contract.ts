import { renderToStaticMarkup } from 'react-dom/server'
import { createElement } from 'react'
import type { WorkspaceTaskView } from './workspaceViewModel'
import { WorkspaceStatusRail } from './WorkspaceStatusRail'

const running = renderToStaticMarkup(createElement(WorkspaceStatusRail, { task: taskView('running'), startedAt: new Date(Date.now() - 2_000).toISOString() }))
for (const expected of ['role="status"', 'aria-live="polite"', 'aria-atomic="true"', 'aria-busy="true"', 'aria-label="创作进度"', '生成中', '图像生成', 'aria-current="step"', '已用时']) {
  if (!running.includes(expected)) throw new Error(`running rail should render ${expected}, got ${running}`)
}

const partial = renderToStaticMarkup(createElement(WorkspaceStatusRail, { task: taskView('partial') }))
if (!partial.includes('部分完成') || !partial.includes('1 / 2')) {
  throw new Error(`partial rail should expose a textual partial result, got ${partial}`)
}

const failed = renderToStaticMarkup(createElement(WorkspaceStatusRail, { task: taskView('failure') }))
if (!failed.includes('生成失败') || !failed.includes('data-status="failed"') || !failed.includes('aria-busy="false"')) {
  throw new Error(`failed rail should expose failure without color alone, got ${failed}`)
}

function taskView(state: WorkspaceTaskView['state']): WorkspaceTaskView {
  return {
    state,
    title: state === 'running' ? '生成中' : state === 'partial' ? '部分完成' : '生成失败',
    detail: state === 'failure' ? '服务暂时不可用，请稍后重试。' : '任务状态正在实时更新。',
    resultCount: state === 'partial' ? 1 : 0,
    requestedCount: state === 'partial' ? 2 : 1,
    rail: [
      { phase: 'validating', label: '参数校验', status: 'done' },
      { phase: 'queued', label: '队列调度', status: 'done' },
      { phase: 'generating', label: '图像生成', status: state === 'failure' ? 'failed' : state === 'running' ? 'active' : 'done' },
      { phase: 'storing', label: '结果入库', status: state === 'partial' ? 'done' : 'idle' },
    ],
  }
}
