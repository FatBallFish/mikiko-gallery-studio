import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { BookOpen, Braces, Menu, Search, X } from 'lucide-react'
import { guides } from './content/guides'
import { DocsSearch } from './components/DocsSearch'
import type { GuideId } from './types'

const ReferencePage = lazy(() => import('./ReferencePage'))

type Route = { kind: 'guide'; id: GuideId } | { kind: 'reference' }

function currentRoute(): Route {
  const value = window.location.hash.replace(/^#\/?/, '')
  if (value === 'reference') return { kind: 'reference' }
  const id = value.replace(/^guide\//, '') as GuideId
  return { kind: 'guide', id: guides.some((guide) => guide.id === id) ? id : 'quickstart' }
}

export default function App() {
  const [route, setRoute] = useState<Route>(() => currentRoute())
  const [searchOpen, setSearchOpen] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const [toc, setToc] = useState<Array<{ id: string; label: string }>>([])
  const guide = route.kind === 'guide' ? guides.find((item) => item.id === route.id) ?? guides[0] : null
  const groups = useMemo(() => Array.from(new Set(guides.map((item) => item.group))), [])

  useEffect(() => {
    const update = () => { setRoute(currentRoute()); setMobileOpen(false); window.scrollTo({ top: 0 }) }
    window.addEventListener('hashchange', update)
    if (!window.location.hash) window.location.hash = '/guide/quickstart'
    return () => window.removeEventListener('hashchange', update)
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const isEditing = target?.matches('input, textarea, select, [contenteditable="true"]')
      if (event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey && !isEditing) {
        event.preventDefault()
        setSearchOpen(true)
      }
      if (event.key === 'Escape') setMobileOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  useEffect(() => {
    if (!guide) { setToc([]); return }
    const frame = window.requestAnimationFrame(() => {
      setToc(Array.from(document.querySelectorAll<HTMLElement>('.guide-article h2[id]')).map((heading) => ({ id: heading.id, label: heading.textContent || heading.id })))
    })
    return () => window.cancelAnimationFrame(frame)
  }, [guide])

  return (
    <div className="docs-app">
      <header className="docs-topbar">
        <a className="docs-brand" href="#/guide/quickstart" aria-label="Mikiko Studio API 文档首页"><span>M</span><strong>Mikiko API</strong><small>Docs</small></a>
        <div className="docs-top-actions">
          <button className="docs-search-trigger" type="button" aria-label="搜索文档" onClick={() => setSearchOpen(true)}><Search size={16} /><span>搜索文档</span><kbd>/</kbd></button>
          <span className="docs-version">API 0.2</span>
          <a className="docs-console-link" href="../" target="_blank" rel="noreferrer">返回控制台</a>
          <button className="docs-mobile-toggle" type="button" aria-label={mobileOpen ? '关闭导航' : '打开导航'} aria-expanded={mobileOpen} onClick={() => setMobileOpen((value) => !value)}>{mobileOpen ? <X size={19} /> : <Menu size={19} />}</button>
        </div>
      </header>

      <div className="docs-layout">
        <aside className={`docs-sidebar ${mobileOpen ? 'open' : ''}`}>
          <nav aria-label="开发者文档">
            {groups.map((group) => <div className="nav-group" key={group}><strong>{group}</strong>{guides.filter((item) => item.group === group).map((item) => <a key={item.id} href={`#/guide/${item.id}`} aria-current={guide?.id === item.id ? 'page' : undefined}>{item.title}</a>)}</div>)}
            <div className="nav-group"><strong>接口定义</strong><a href="#/reference" aria-current={route.kind === 'reference' ? 'page' : undefined}><Braces size={15} />完整接口参考</a></div>
          </nav>
        </aside>
        {mobileOpen ? <button className="docs-nav-backdrop" type="button" aria-label="关闭导航" onClick={() => setMobileOpen(false)} /> : null}

        <main className={route.kind === 'reference' ? 'docs-main reference-main' : 'docs-main'} inert={mobileOpen ? true : undefined}>
          {route.kind === 'reference' ? <Suspense fallback={<div className="reference-loading" role="status">正在加载接口参考...</div>}><ReferencePage /></Suspense> : guide ? (
            <article className="guide-article">
              <div className="guide-heading"><span><BookOpen size={15} />{guide.group}</span><h1>{guide.title}</h1><p>{guide.summary}</p></div>
              <div className="guide-content">{guide.content}</div>
              <GuidePager id={guide.id} />
            </article>
          ) : null}
        </main>

        {guide && toc.length ? <aside className="docs-toc" aria-label="本页目录" inert={mobileOpen ? true : undefined}><strong>本页内容</strong>{toc.map((item) => <a key={item.id} href={`#${item.id}`}>{item.label}</a>)}</aside> : null}
      </div>
      {searchOpen ? <DocsSearch onClose={() => setSearchOpen(false)} /> : null}
    </div>
  )
}

function GuidePager({ id }: { id: GuideId }) {
  const index = guides.findIndex((guide) => guide.id === id)
  const previous = guides[index - 1]
  const next = guides[index + 1]
  return <nav className="guide-pager" aria-label="指南分页">{previous ? <a href={`#/guide/${previous.id}`}><small>上一篇</small><strong>{previous.title}</strong></a> : <span />}{next ? <a href={`#/guide/${next.id}`}><small>下一篇</small><strong>{next.title}</strong></a> : <a href="#/reference"><small>继续</small><strong>完整接口参考</strong></a>}</nav>
}
