import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { createElement, useEffect, useState } from 'react'
import type { ComponentType, ReactElement } from 'react'
import { act, create } from 'react-test-renderer'
import type { ReactTestInstance, ReactTestRenderer } from 'react-test-renderer'
import { createCanvasRouteLoading } from './canvasRouteLoading'

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })

type PageModule = { default: ComponentType<Record<string, never>> }
const deferred = () => {
  let resolve!: (value: PageModule) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<PageModule>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const button = (root: ReactTestInstance, label: string) => root.findAllByType('button').find((item) => item.children.join('') === label)
const click = (root: ReactTestInstance, label: string) => {
  const target = button(root, label)
  assert.ok(target, `missing button: ${label}`)
  target.props.onClick()
}
type Mount = (element: ReactElement) => Promise<ReactTestRenderer>

async function withRenderer(run: (mount: Mount, diagnostics: unknown[][]) => Promise<void>) {
  const original = console.error
  const diagnostics: unknown[][] = []
  console.error = (...args: unknown[]) => { diagnostics.push(args) }
  let renderer: ReactTestRenderer | undefined
  try {
    await run(async (element) => {
      let mounted!: ReactTestRenderer
      await act(async () => { mounted = create(element) })
      renderer = mounted
      return mounted
    }, diagnostics)
  } finally {
    if (renderer) await act(async () => renderer!.unmount())
    console.error = original
  }
}

test('download rejection stays local; only manual retry reimports and preserves siblings', async () => {
  await withRenderer(async (mount, diagnostics) => {
    const first = deferred()
    let calls = 0, siblingMounts = 0, siblingUnmounts = 0
    let bump!: () => void
    const Route = createCanvasRouteLoading(() => {
      calls++
      return calls === 1 ? first.promise : Promise.resolve({ default: () => createElement('p', null, 'canvas ready') })
    })
    function PersistentUpload() {
      const [queue, setQueue] = useState(7)
      useEffect(() => { siblingMounts++; return () => { siblingUnmounts++ } }, [])
      return createElement('button', { onClick: () => setQueue(queue + 1) }, `queue ${queue}`)
    }
    function Host() {
      const [tick, setTick] = useState(0)
      bump = () => setTick(tick + 1)
      return createElement('div', null,
        createElement('nav', null, 'navigation'),
        createElement(Route, { pageProps: {}, onBack() {}, backLabel: 'back' }),
        createElement(PersistentUpload))
    }
    const renderer = await mount(createElement(Host))
    assert.equal(renderer.root.findByProps({ role: 'status' }).children[0], '正在载入画布...')
    const failure = new TypeError('Failed to fetch dynamically imported module: http://localhost/canvas.js')
    await act(async () => first.reject(failure))
    assert.equal(renderer.root.findAllByProps({ role: 'alert' }).length, 1)
    assert.equal(renderer.root.findByType('nav').children[0], 'navigation')
    await act(async () => click(renderer.root, 'queue 7'))
    await act(async () => bump())
    assert.equal(calls, 1, 'parent updates must not retry')
    assert.ok(diagnostics.some((args) => args[0] === 'Canvas route failed' && args[1] === failure && typeof args[2] === 'string'))
    await act(async () => click(renderer.root, '重试加载'))
    assert.equal(calls, 2, 'retry must replace the rejected lazy instance')
    assert.equal(renderer.root.findByType('p').children[0], 'canvas ready')
    assert.ok(button(renderer.root, 'queue 8'))
    assert.equal(siblingMounts, 1)
    assert.equal(siblingUnmounts, 0)
  })
})

test('failed retry remains actionable without further automatic imports', async () => {
  await withRenderer(async (mount) => {
    let calls = 0
    const Route = createCanvasRouteLoading(async () => { calls++; throw new TypeError('Importing a module script failed.') })
    const element = createElement(Route, { pageProps: {}, onBack() {}, backLabel: 'back' })
    const renderer = await mount(element)
    await act(async () => click(renderer.root, '重试加载'))
    await act(async () => renderer.update(element))
    assert.equal(calls, 2)
    assert.ok(button(renderer.root, '重试加载'))
  })
})

for (const mode of ['evaluation', 'null rejection', 'generic import TypeError', 'runtime transport-looking TypeError']) {
  test(`${mode} is reported without retry; same-route updates do not reset`, async () => {
    await withRenderer(async (mount, diagnostics) => {
      const failure = mode === 'null rejection' ? null : mode === 'evaluation' ? new Error('evaluation failed')
        : new TypeError(mode === 'generic import TypeError' ? 'Failed to fetch' : 'Failed to fetch dynamically imported module: http://localhost/canvas.js')
      let calls = 0, back = 0
      const Route = createCanvasRouteLoading(async () => {
        calls++
        if (mode !== 'runtime transport-looking TypeError') throw failure
        return { default: () => { throw failure } }
      })
      const element = (key: string) => createElement(Route, { key, pageProps: {}, onBack() { back++ }, backLabel: 'back' })
      const renderer = await mount(element('canvas-a'))
      assert.equal(button(renderer.root, '重试加载'), undefined)
      assert.match(JSON.stringify(renderer.toJSON()), /未保存改动可能尚未写入/)
      await act(async () => renderer.update(element('canvas-a')))
      assert.equal(calls, 1)
      await act(async () => click(renderer.root, 'back'))
      assert.equal(back, 1)
      assert.ok(diagnostics.some((args) => args[0] === 'Canvas route failed' && args[1] === failure))
    })
  })
}

test('explicit canvas navigation resets the boundary; ordinary updates preserve editor state', async () => {
  await withRenderer(async (mount) => {
    let renderCount = 0
    const Route = createCanvasRouteLoading(async () => ({ default: ({ crash }: { crash: boolean }) => {
      const [draft, setDraft] = useState('draft')
      renderCount++
      if (crash) throw new Error('runtime failed')
      return createElement('button', { onClick: () => setDraft('unsaved draft') }, draft)
    } }))
    const element = (key: string, crash = false) => createElement(Route, { key, pageProps: { crash }, onBack() {}, backLabel: 'back' })
    const renderer = await mount(element('a'))
    await act(async () => click(renderer.root, 'draft'))
    await act(async () => renderer.update(element('a')))
    assert.ok(button(renderer.root, 'unsaved draft'))
    await act(async () => renderer.update(element('a', true)))
    const failedCount = renderCount
    await act(async () => renderer.update(element('a')))
    assert.equal(renderCount, failedCount)
    assert.equal(renderer.root.findAllByProps({ role: 'alert' }).length, 1)
    await act(async () => renderer.update(element('b')))
    assert.ok(button(renderer.root, 'draft'))
  })
})

test('App wires only canvas routes and keeps providers and upload outside recovery', () => {
  const app = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
  assert.match(app, /<CanvasEditorRoute key=\{routeCanvasId\}/)
  assert.match(app, /<Shell scrollMode="document"><CanvasListRoute/)
  assert.match(app, /<ProjectProvider key=\{projectUserID\} userID=\{projectUserID\}>\{page\}\{featureFlags.media_upload \? <UploadTray hidden=/)
  const helper = readFileSync(new URL('./canvasRouteLoading.tsx', import.meta.url), 'utf8')
  assert.doesNotMatch(helper, /location\.reload|setTimeout|setInterval|Date\.now|localStorage|sessionStorage|removeCanvasDraft/)
})
