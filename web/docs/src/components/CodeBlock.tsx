import { useState } from 'react'
import { Check, Copy, RotateCcw } from 'lucide-react'

export function CodeBlock({ code, language = 'bash', label }: { code: string; language?: string; label?: string }) {
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle')
  async function copy() {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard API unavailable')
      await navigator.clipboard.writeText(code)
      setCopyState('copied')
    } catch {
      const textarea = document.createElement('textarea')
      textarea.value = code
      textarea.style.cssText = 'position:fixed;opacity:0;pointer-events:none'
      document.body.appendChild(textarea)
      textarea.select()
      let copied = false
      try {
        copied = document.execCommand('copy')
      } catch {
        copied = false
      }
      textarea.remove()
      setCopyState(copied ? 'copied' : 'failed')
    }
    window.setTimeout(() => setCopyState('idle'), 1600)
  }
  const copied = copyState === 'copied'
  const failed = copyState === 'failed'
  return (
    <div className="code-block">
      <div className="code-toolbar">
        <span>{label || language}</span>
        <button type="button" onClick={() => void copy()} aria-label={copied ? '已复制代码' : failed ? '复制失败，重试' : '复制代码'}>
          {copied ? <Check size={15} aria-hidden="true" /> : failed ? <RotateCcw size={15} aria-hidden="true" /> : <Copy size={15} aria-hidden="true" />}
          {copied ? '已复制' : failed ? '重试' : '复制'}
        </button>
      </div>
      <pre><code data-language={language}>{code}</code></pre>
    </div>
  )
}
