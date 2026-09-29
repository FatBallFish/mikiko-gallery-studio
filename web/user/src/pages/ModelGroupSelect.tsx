import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import type { CapabilityModelGroup, RouteModelGroupMeta } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ModelIcon } from '../ModelIcon'

type PickerSection = { group?: RouteModelGroupMeta; models: CapabilityModelGroup[] }

function pointsSubject(raw?: string) {
  const value = Number(raw)
  if (!Number.isFinite(value)) return raw?.trim() || '0'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

export function ModelGroupSelect({ options, value, onChange, groups }: {
  options: CapabilityModelGroup[]
  value: string
  onChange: (value: string) => void
  groups?: RouteModelGroupMeta[]
}) {
  const listboxID = useId()
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const keyboardNavigationRef = useRef(false)
  const [open, setOpen] = useState(false)
  // Two-level layout: admin-configured groups first (in configured order),
  // then models that belong to no group. A model listed in several groups
  // appears under each; the picked value is always a concrete route model.
  const sections = useMemo<PickerSection[]>(() => {
    const configured = (groups ?? []).filter((group) => options.some((option) => (option.group_codes ?? []).includes(group.code)))
    const used = new Set<string>()
    const result: PickerSection[] = configured.map((group) => {
      const models = options.filter((option) => (option.group_codes ?? []).includes(group.code))
      models.forEach((model) => used.add(model.code))
      return { group, models }
    })
    const loose = options.filter((option) => !used.has(option.code))
    if (loose.length) result.push({ models: loose })
    return result.length ? result : [{ models: options }]
  }, [options, groups])
  const flatModels = useMemo(() => sections.flatMap((section) => section.models), [sections])
  const selectedIndex = Math.max(0, flatModels.findIndex((item) => item.code === value))
  const [activeIndex, setActiveIndex] = useState(selectedIndex)
  const selected = flatModels[selectedIndex]

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
    const option = flatModels[index]
    if (!option) return
    onChange(option.code)
    closeAndFocus()
  }

  function handleKeyDown(event: React.KeyboardEvent) {
    if (!flatModels.length) return
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
      setActiveIndex((index) => (index + direction + flatModels.length) % flatModels.length)
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
        <ModelIcon iconKey={selected.icon_key} iconSvg={selected.icon_svg} size={20} alt="" />
        <span className="model-group-select-copy">
          <strong>{selected.name}</strong>
          {selected.description?.trim() ? <small>{selected.description}</small> : null}
        </span>
        <span className="model-group-select-subject" aria-label={`${pointsSubject(selected.minimum_points)} 积分`}>◈{pointsSubject(selected.minimum_points)}</span>
        <ChevronDown className={cn('model-group-select-chevron', open && 'rotate-180')} size={16} aria-hidden="true" />
      </button>
      {open ? (
        <div id={listboxID} className="model-group-select-menu" role="listbox" aria-label="选择模型分组">
          {sections.map((section, sectionIndex) => (
            <div key={section.group?.code ?? `section-${sectionIndex}`} className="model-group-select-section">
              {section.group ? (
                <div className="model-group-select-section-label" role="presentation">
                  <ModelIcon iconKey={section.group.icon_key} iconSvg={section.group.icon_svg} size={16} alt="" />
                  <span>{section.group.name}</span>
                </div>
              ) : null}
              {section.models.map((item) => {
                const index = flatModels.indexOf(item)
                return (
                  <button
                    key={`${section.group?.code ?? 'ungrouped'}-${item.code}`}
                    ref={(node) => { optionRefs.current[index] = node }}
                    type="button"
                    role="option"
                    tabIndex={index === activeIndex ? 0 : -1}
                    aria-selected={item.code === value}
                    data-active={index === activeIndex || undefined}
                    onMouseEnter={() => { keyboardNavigationRef.current = false; setActiveIndex(index) }}
                    onClick={() => select(index)}
                  >
                    <ModelIcon iconKey={item.icon_key} iconSvg={item.icon_svg} size={18} alt="" className="model-group-select-option-icon" />
                    <span className="model-group-select-copy">
                      <strong>{item.name}</strong>
                      {item.description?.trim() ? <small>{item.description}</small> : null}
                    </span>
                    <span className="model-group-select-subject" aria-label={`${pointsSubject(item.minimum_points)} 积分`}>◈{pointsSubject(item.minimum_points)}</span>
                    <Check className={cn('model-group-select-check', item.code !== value && 'invisible')} size={15} aria-hidden="true" />
                  </button>
                )
              })}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
