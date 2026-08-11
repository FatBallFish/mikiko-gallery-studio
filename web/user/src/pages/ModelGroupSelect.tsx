import { useEffect, useId, useRef, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import type { CapabilityModelGroup } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'

function pointsSubject(raw?: string) {
  const value = Number(raw)
  if (!Number.isFinite(value)) return raw?.trim() || '0'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

export function ModelGroupSelect({ options, value, onChange }: {
  options: CapabilityModelGroup[]
  value: string
  onChange: (value: string) => void
}) {
  const listboxID = useId()
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const keyboardNavigationRef = useRef(false)
  const [open, setOpen] = useState(false)
  const selectedIndex = Math.max(0, options.findIndex((item) => item.code === value))
  const [activeIndex, setActiveIndex] = useState(selectedIndex)
  const selected = options[selectedIndex]

  useEffect(() => {
    if (!open) return undefined
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () => document.removeEventListener('pointerdown', closeOnOutsidePointer)
  }, [open])

  useEffect(() => {
    if (!open) return undefined
    setActiveIndex(selectedIndex)
    const frame = window.requestAnimationFrame(() => optionRefs.current[selectedIndex]?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [open, selectedIndex])

  useEffect(() => {
    if (open && keyboardNavigationRef.current) optionRefs.current[activeIndex]?.focus()
  }, [activeIndex, open])

  function closeAndFocus() {
    setOpen(false)
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }

  function select(index: number) {
    const option = options[index]
    if (!option) return
    onChange(option.code)
    closeAndFocus()
  }

  function handleKeyDown(event: React.KeyboardEvent) {
    if (!options.length) return
    if (event.key === 'Escape' && open) {
      event.preventDefault()
      closeAndFocus()
      return
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      keyboardNavigationRef.current = true
      if (!open) {
        setOpen(true)
        return
      }
      const direction = event.key === 'ArrowDown' ? 1 : -1
      setActiveIndex((index) => (index + direction + options.length) % options.length)
      return
    }
    if ((event.key === 'Enter' || event.key === ' ') && open) {
      event.preventDefault()
      select(activeIndex)
    }
  }

  if (!selected) return null
  return (
    <div
      ref={rootRef}
      className="model-group-select"
      onKeyDown={handleKeyDown}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false)
      }}
    >
      <button
        ref={triggerRef}
        id="workspace-model-group"
        type="button"
        className="model-group-select-trigger"
        data-value={value}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxID}
        onClick={() => setOpen((current) => !current)}
      >
        <span className="model-group-select-copy">
          <strong>{selected.name}</strong>
          {selected.description?.trim() ? <small>{selected.description}</small> : null}
        </span>
        <span className="model-group-select-subject" aria-label={`${pointsSubject(selected.minimum_points)} 积分`}>◈{pointsSubject(selected.minimum_points)}</span>
        <ChevronDown className={cn('model-group-select-chevron', open && 'rotate-180')} size={16} aria-hidden="true" />
      </button>
      {open ? (
        <div id={listboxID} className="model-group-select-menu" role="listbox" aria-label="选择模型分组">
          {options.map((item, index) => (
            <button
              key={item.code}
              ref={(node) => { optionRefs.current[index] = node }}
              type="button"
              role="option"
              tabIndex={index === activeIndex ? 0 : -1}
              aria-selected={item.code === value}
              data-active={index === activeIndex || undefined}
              onMouseEnter={() => { keyboardNavigationRef.current = false; setActiveIndex(index) }}
              onClick={() => select(index)}
            >
              <span className="model-group-select-copy">
                <strong>{item.name}</strong>
                {item.description?.trim() ? <small>{item.description}</small> : null}
              </span>
              <span className="model-group-select-subject" aria-label={`${pointsSubject(item.minimum_points)} 积分`}>◈{pointsSubject(item.minimum_points)}</span>
              <Check className={cn('model-group-select-check', item.code !== value && 'invisible')} size={15} aria-hidden="true" />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
