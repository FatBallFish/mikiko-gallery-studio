import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { ArrowLeft, Check, ChevronDown } from 'lucide-react'
import type { CapabilityModelGroup, RouteModelGroupMeta } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { pointsText } from '../../../shared/pointsDisplay'
import { ModelIcon } from '../ModelIcon'

type PickerSection = { group?: RouteModelGroupMeta; models: CapabilityModelGroup[] }

function pointsSubject(raw?: string) {
  if (!raw?.trim()) return '0'
  return pointsText(raw)
}

const UNGROUPED = '__ungrouped__'

// Progressive drill-down model picker. The dropdown is a single full-width
// column: level 1 lists the model groups, tapping one slides to level 2 with
// that group's route models and a back row. Selecting always yields a
// concrete route model code. When the dropdown opens with a selection
// already made, it starts on that model's group so one tap adjusts it.
// A model without its own icon inherits the group icon it is listed under.
export function ModelGroupSelect({ options, value, onChange, groups }: {
  options: CapabilityModelGroup[]
  value: string
  onChange: (value: string) => void
  groups?: RouteModelGroupMeta[]
}) {
  const listboxID = useId()
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [level, setLevel] = useState<1 | 2>(1)
  const [activeGroupCode, setActiveGroupCode] = useState<string>(UNGROUPED)

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

  const hasGroups = sections.length > 1 || sections.some((section) => section.group)
  const selectedModel = options.find((item) => item.code === value) ?? options[0]
  const selectedSection = sections.find((section) => section.models.some((model) => model.code === selectedModel?.code))
  const selectedGroup = selectedSection?.group
  const activeSection = sections.find((section) => (section.group?.code ?? UNGROUPED) === activeGroupCode) ?? sections[0]

  useEffect(() => {
    if (!open) return undefined
    // Reopening with a selection lands on level 2 of that model's group;
    // otherwise level 1.
    setLevel(selectedSection ? 2 : 1)
    setActiveGroupCode(selectedSection?.group?.code ?? UNGROUPED)
    const frame = window.requestAnimationFrame(() => panelRef.current?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [open, selectedSection])

  useEffect(() => {
    if (!open) return undefined
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () => document.removeEventListener('pointerdown', closeOnOutsidePointer)
  }, [open])

  function closeAndFocus() {
    setOpen(false)
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }

  function select(code: string) {
    onChange(code)
    closeAndFocus()
  }

  function handleKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'Escape' && open) {
      event.preventDefault()
      if (level === 2 && hasGroups) {
        event.stopPropagation()
        setLevel(1)
        return
      }
      closeAndFocus()
    }
  }

  if (!selectedModel) return null
  return (
    <div
      ref={rootRef}
      className={cn('model-group-select', hasGroups && 'is-drilldown')}
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
        <ModelIcon
          iconKey={selectedModel.icon_key}
          iconSvg={selectedModel.icon_svg}
          groupIconKey={selectedGroup?.icon_key}
          groupIconSvg={selectedGroup?.icon_svg}
          size={20}
          alt=""
          className="model-group-select-trigger-icon"
        />
        <span className="model-group-select-copy">
          <strong>{selectedModel.name}</strong>
          {selectedModel.description?.trim() ? <small>{selectedModel.description}</small> : null}
        </span>
        {selectedModel.minimum_points ? <span className="model-group-select-subject" aria-label={`最低消耗 ${pointsSubject(selectedModel.minimum_points)}`}>◈{pointsSubject(selectedModel.minimum_points)}</span> : null}
        <ChevronDown className={cn('model-group-select-chevron', open && 'rotate-180')} size={16} aria-hidden="true" />
      </button>
      {open ? (
        <div
          id={listboxID}
          ref={panelRef}
          tabIndex={-1}
          className={cn('model-group-select-menu', level === 2 && 'is-level2')}
          role="listbox"
          aria-label="选择模型分组"
        >
          {hasGroups ? (
            <div className="model-group-select-page" data-level={level}>
              <div className="model-group-select-level" data-level="1">
                <ul className="model-group-select-list" role="group" aria-label="模型分组">
                  {sections.map((section) => {
                    const key = section.group?.code ?? UNGROUPED
                    return (
                      <li key={key}>
                        <button
                          type="button"
                          className="model-group-select-row"
                          onClick={() => {
                            setActiveGroupCode(key)
                            setLevel(2)
                          }}
                        >
                          {section.group ? (
                            <ModelIcon iconKey={section.group.icon_key} iconSvg={section.group.icon_svg} size={20} alt="" />
                          ) : (
                            <ModelIcon size={20} alt="" />
                          )}
                          <span className="model-group-select-row-copy">
                            <strong>{section.group?.name ?? '其他模型'}</strong>
                            <small>{section.models.length} 个模型</small>
                          </span>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </div>
              <div className="model-group-select-level" data-level="2">
                <button
                  type="button"
                  className="model-group-select-back"
                  onClick={() => setLevel(1)}
                  aria-label="返回模型分组列表"
                >
                  <ArrowLeft size={15} aria-hidden="true" />
                  {activeSection?.group ? (
                    <>
                      <ModelIcon iconKey={activeSection.group.icon_key} iconSvg={activeSection.group.icon_svg} size={18} alt="" />
                      <span>{activeSection.group.name}</span>
                    </>
                  ) : (
                    <span>其他模型</span>
                  )}
                </button>
                <ul className="model-group-select-list" role="group" aria-label="路由模型">
                  {(activeSection?.models ?? []).map((item) => (
                    <li key={item.code}>
                      <button
                        type="button"
                        role="option"
                        aria-selected={item.code === value}
                        className={cn('model-group-select-row', item.code === value && 'is-active')}
                        onClick={() => select(item.code)}
                      >
                        <ModelIcon
                          iconKey={item.icon_key}
                          iconSvg={item.icon_svg}
                          groupIconKey={activeSection?.group?.icon_key}
                          groupIconSvg={activeSection?.group?.icon_svg}
                          size={22}
                          alt=""
                        />
                        <span className="model-group-select-row-copy" title={item.description?.trim() || undefined}>
                          <strong>{item.name}</strong>
                          {item.description?.trim() ? <small>{item.description}</small> : null}
                        </span>
                        {item.minimum_points ? <span className="model-group-select-subject" aria-label={`最低消耗 ${pointsSubject(item.minimum_points)}`}>◈{pointsSubject(item.minimum_points)}</span> : null}
                        <Check className={cn('model-group-select-check', item.code !== value && 'invisible')} size={15} aria-hidden="true" />
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          ) : (
            <ul className="model-group-select-list" role="listbox" aria-label="路由模型">
              {options.map((item) => (
                <li key={item.code}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={item.code === value}
                    className={cn('model-group-select-row', item.code === value && 'is-active')}
                    onClick={() => select(item.code)}
                  >
                    <ModelIcon iconKey={item.icon_key} iconSvg={item.icon_svg} size={22} alt="" />
                    <span className="model-group-select-row-copy" title={item.description?.trim() || undefined}>
                      <strong>{item.name}</strong>
                      {item.description?.trim() ? <small>{item.description}</small> : null}
                    </span>
                    {item.minimum_points ? <span className="model-group-select-subject" aria-label={`最低消耗 ${pointsSubject(item.minimum_points)}`}>◈{pointsSubject(item.minimum_points)}</span> : null}
                    <Check className={cn('model-group-select-check', item.code !== value && 'invisible')} size={15} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  )
}
