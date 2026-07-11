import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRight, Search, X } from 'lucide-react'
import { buildSearchIndex, searchDocs } from '../search/searchIndex'

export function DocsSearch({ onClose }: { onClose: () => void }) {
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const dialogRef = useRef<HTMLElement>(null)
  const index = useMemo(() => buildSearchIndex(), [])
  const results = useMemo(() => searchDocs(query, index), [index, query])

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    const background = Array.from(document.querySelectorAll<HTMLElement>('.docs-topbar, .docs-layout'))
    const previousOverflow = document.body.style.overflow
    background.forEach((element) => element.setAttribute('inert', ''))
    document.body.style.overflow = 'hidden'

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('input, button, a[href], [tabindex]:not([tabindex="-1"])') ?? [])
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      background.forEach((element) => element.removeAttribute('inert'))
      document.body.style.overflow = previousOverflow
      previousFocus?.focus()
    }
  }, [onClose])

  useEffect(() => {
    setActive((value) => Math.min(value, Math.max(0, results.length - 1)))
  }, [results.length])

  useEffect(() => {
    document.getElementById(`docs-search-result-${active}`)?.scrollIntoView({ block: 'nearest' })
  }, [active])

  function open(href: string) {
    window.location.hash = href.replace(/^#/, '')
    onClose()
  }

  return (
    <div className="search-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        ref={dialogRef}
        className="search-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="搜索文档"
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown') { event.preventDefault(); setActive((value) => Math.min(Math.max(0, results.length - 1), value + 1)) }
          if (event.key === 'ArrowUp') { event.preventDefault(); setActive((value) => Math.max(0, value - 1)) }
          if (event.key === 'Enter' && results[active]) open(results[active].href)
        }}
      >
        <div className="search-input-row">
          <Search size={18} aria-hidden="true" />
          <input autoFocus value={query} onChange={(event) => { setQuery(event.target.value); setActive(0) }} placeholder="搜索指南、接口或错误处理" aria-label="搜索文档" aria-controls="docs-search-results" aria-activedescendant={results[active] ? `docs-search-result-${active}` : undefined} />
          <button type="button" onClick={onClose} aria-label="关闭搜索"><X size={17} /></button>
        </div>
        <div id="docs-search-results" className="search-results" role="listbox" aria-label="搜索结果">
          {results.map((result, indexValue) => (
            <button id={`docs-search-result-${indexValue}`} key={result.id} type="button" role="option" aria-selected={active === indexValue} className={active === indexValue ? 'active' : ''} onMouseEnter={() => setActive(indexValue)} onClick={() => open(result.href)}>
              <span><strong>{result.title}</strong><small>{result.summary}</small></span>
              <ArrowRight size={16} aria-hidden="true" />
            </button>
          ))}
          {!results.length ? <p className="search-empty">没有匹配内容。尝试搜索“任务状态”或“OpenAI”。</p> : null}
        </div>
      </section>
    </div>
  )
}
