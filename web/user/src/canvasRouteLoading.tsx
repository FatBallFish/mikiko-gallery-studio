import React, { Component, lazy, Suspense, useState } from 'react'
import type { ComponentType, ErrorInfo, ReactNode } from 'react'

// Only classify browser module-transport diagnostics at the import boundary.
// A generic TypeError (or an error thrown by the loaded page) is not retryable.
function isModuleTransportFailure(error: unknown): error is TypeError {
  return error instanceof TypeError && (
    /^Failed to fetch dynamically imported module(?::\s*\S+)?\.?$/.test(error.message)
    || /^error loading dynamically imported module(?::\s*\S+)?\.?$/i.test(error.message)
    || error.message === 'Importing a module script failed.'
  )
}

class CanvasModuleLoadFailure extends Error {
  constructor(readonly original: TypeError) {
    super('Canvas module download failed')
    this.name = 'CanvasModuleLoadFailure'
  }
}

type BoundaryProps = {
  children: ReactNode
  onRetry: () => void
  onBack: () => void
  backLabel: string
}

class CanvasRouteBoundary extends Component<BoundaryProps, { failed: boolean; error: unknown }> {
  state: { failed: boolean; error: unknown } = { failed: false, error: null }

  static getDerivedStateFromError(error: unknown) { return { failed: true, error } }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error('Canvas route failed', error instanceof CanvasModuleLoadFailure ? error.original : error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!this.state.failed) return this.props.children
    const retryable = error instanceof CanvasModuleLoadFailure
    return (
      <main className="grid min-h-full place-items-center bg-[var(--bg)] p-6 text-[var(--fg)]">
        <section className="w-full max-w-lg border border-[var(--border)] bg-[var(--surface)] p-6" role="alert">
          <h1 className="text-lg font-semibold">{retryable ? '画布模块未能加载' : '画布运行出错'}</h1>
          <p className="mt-2 text-sm leading-6 text-[var(--muted)]">{retryable
            ? '画布页面尚未打开。请检查网络连接后手动重试加载。'
            : '画布遇到运行错误，已停止显示。最近的未保存改动可能尚未写入本地草稿，返回后请检查恢复内容。'}</p>
          <div className="mt-5 flex flex-wrap gap-3">
            {retryable ? <button type="button" className="min-h-10 border border-[var(--border)] px-4 text-sm" onClick={this.props.onRetry}>重试加载</button> : null}
            <button type="button" className="min-h-10 border border-[var(--border)] px-4 text-sm" onClick={this.props.onBack}>{this.props.backLabel}</button>
            <a className="inline-flex min-h-10 items-center px-4 text-sm underline" href="#/home">返回首页</a>
          </div>
        </section>
      </main>
    )
  }
}

// Create once at module scope. Ordinary parent updates preserve the lazy identity;
// only an explicit retry replaces React.lazy's cached rejected promise.
export function createCanvasRouteLoading<Props extends object>(load: () => Promise<{ default: ComponentType<Props> }>) {
  const createPage = () => lazy(async () => {
    try {
      return await load()
    } catch (error) {
      if (isModuleTransportFailure(error)) throw new CanvasModuleLoadFailure(error)
      throw error
    }
  })
  const initialPage = createPage()
  return function CanvasRouteLoading({ pageProps, onBack, backLabel }: { pageProps: Props; onBack: () => void; backLabel: string }) {
    const [{ Page, attempt }, setAttempt] = useState(() => ({ Page: initialPage, attempt: 0 }))
    return (
      <CanvasRouteBoundary key={attempt} onRetry={() => setAttempt(({ attempt }) => ({ Page: createPage(), attempt: attempt + 1 }))} onBack={onBack} backLabel={backLabel}>
        <Suspense fallback={<div className="grid min-h-full place-items-center text-sm text-[var(--muted)]" role="status">正在载入画布...</div>}>
          <Page {...pageProps} />
        </Suspense>
      </CanvasRouteBoundary>
    )
  }
}
