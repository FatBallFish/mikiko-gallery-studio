import { forwardRef, useCallback, useEffect, useId, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { AlertCircle, AtSign, Braces, ImagePlus, Redo2, Undo2 } from 'lucide-react'
import {
  $createLineBreakNode,
  $createParagraphNode,
  $createTextNode,
  $getNodeByKey,
  $getRoot,
  $getSelection,
  $isRangeSelection,
  $isTextNode,
  COMMAND_PRIORITY_HIGH,
  KEY_BACKSPACE_COMMAND,
  KEY_DELETE_COMMAND,
  KEY_SPACE_COMMAND,
  REDO_COMMAND,
  UNDO_COMMAND,
  TextNode,
  type EditorConfig,
  type LexicalEditor,
  type LexicalNode,
  type NodeKey,
  type SerializedTextNode,
  type Spread,
} from 'lexical'
import { LexicalComposer } from '@lexical/react/LexicalComposer'
import { ContentEditable } from '@lexical/react/LexicalContentEditable'
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin'
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary'
import { OnChangePlugin } from '@lexical/react/LexicalOnChangePlugin'
import { PlainTextPlugin } from '@lexical/react/LexicalPlainTextPlugin'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import type { ReferenceAsset } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { normalizePromptTemplateName, parsePromptTemplate, type PromptTemplateTokenKind } from './promptTemplateParser'
import { promptTemplateSegments, promptTokenSource } from './promptTemplateEditorModel'

type SerializedPromptTokenNode = Spread<{
  kind: PromptTemplateTokenKind
  name: string
  type: 'prompt-token'
  version: 1
}, SerializedTextNode>

export class PromptTokenNode extends TextNode {
  __kind: PromptTemplateTokenKind
  __name: string

  static getType() { return 'prompt-token' }

  static clone(node: PromptTokenNode) {
    return new PromptTokenNode(node.__kind, node.__name, node.__key)
  }

  static importJSON(serialized: SerializedPromptTokenNode) {
    return $createPromptTokenNode(serialized.kind, serialized.name)
      .setFormat(serialized.format)
      .setDetail(serialized.detail)
      .setStyle(serialized.style)
  }

  constructor(kind: PromptTemplateTokenKind, name: string, key?: NodeKey) {
    super(promptTokenSource(kind, name), key)
    this.__kind = kind
    this.__name = name
    this.__mode = 1
  }

  exportJSON(): SerializedPromptTokenNode {
    return { ...super.exportJSON(), kind: this.__kind, name: this.__name, type: 'prompt-token', version: 1 }
  }

  createDOM(config: EditorConfig) {
    const element = super.createDOM(config)
    element.classList.add('prompt-token')
    element.dataset.promptTokenKind = this.__kind
    element.dataset.promptTokenName = this.__name
    element.tabIndex = 0
    element.setAttribute('aria-label', `${this.__kind === 'reference' ? '资产' : '变量'}：${this.__name}`)
    return element
  }

  updateDOM(previous: this, element: HTMLElement, config: EditorConfig) {
    const changed = super.updateDOM(previous, element, config)
    element.dataset.promptTokenKind = this.__kind
    element.dataset.promptTokenName = this.__name
    element.tabIndex = 0
    element.setAttribute('aria-label', `${this.__kind === 'reference' ? '资产' : '变量'}：${this.__name}`)
    return changed
  }

  isTextEntity() { return true }
  canInsertTextBefore() { return false }
  canInsertTextAfter() { return false }
}

export function $createPromptTokenNode(kind: PromptTemplateTokenKind, name: string) {
  return new PromptTokenNode(kind, name)
}

function $isPromptTokenNode(node: LexicalNode | null | undefined): node is PromptTokenNode {
  return node?.getType() === PromptTokenNode.getType()
}

function $appendTemplate(template: string) {
  const root = $getRoot()
  root.clear()
  const paragraph = $createParagraphNode()
  root.append(paragraph)
  const segments = promptTemplateSegments(template)
  for (const segment of segments) {
    if (segment.kind === 'reference' || segment.kind === 'variable') {
      paragraph.append($createPromptTokenNode(segment.kind, segment.name ?? ''))
      continue
    }
    const lines = segment.source.split('\n')
    lines.forEach((line, index) => {
      if (index) paragraph.append($createLineBreakNode())
      if (line) paragraph.append($createTextNode(line))
    })
  }
}

export type PromptTemplateEditorHandle = {
  focus: () => void
  insertToken: (kind: PromptTemplateTokenKind, name: string) => void
}

type AutoCompleteRange = { nodeKey: NodeKey; start: number; end: number; kind: PromptTemplateTokenKind; query: string }
type HoveredToken = { kind: PromptTemplateTokenKind; name: string; x: number; y: number }

export const PromptTemplateEditor = forwardRef<PromptTemplateEditorHandle, {
  value: string
  assets: ReferenceAsset[]
  variables: Readonly<Record<string, string>>
  accessToken?: string | null
  disabled?: boolean
  autoFocus?: boolean
  expanded?: boolean
  placeholder?: string
  onChange: (value: string) => void
  onAddAsset?: () => void
}>(({
  value,
  assets,
  variables,
  accessToken,
  disabled = false,
  autoFocus = false,
  expanded = false,
  placeholder = '描述想要生成的内容...，输入 @ 引用资产，输入 $ 添加变量',
  onChange,
  onAddAsset,
}, ref) => {
  const editorRef = useRef<LexicalEditor | null>(null)
  const composingRef = useRef(false)
  const keyboardNavigatingRef = useRef(false)
  const variableNameErrorID = useId()
  const [autocomplete, setAutocomplete] = useState<AutoCompleteRange | null>(null)
  const [manualMenu, setManualMenu] = useState<PromptTemplateTokenKind | null>(null)
  const [variableName, setVariableName] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const [hovered, setHovered] = useState<HoveredToken | null>(null)

  const insertToken = useCallback((kind: PromptTemplateTokenKind, rawName: string) => {
    const normalized = normalizePromptTemplateName(rawName)
    if (normalized.error) return
    const editor = editorRef.current
    if (!editor) return
    editor.update(() => {
      const token = $createPromptTokenNode(kind, normalized.name)
      const trailing = $createTextNode(' ')
      if (autocomplete) {
        const node = $getNodeByKey(autocomplete.nodeKey)
        if ($isTextNode(node)) {
          node.select(autocomplete.start, autocomplete.end)
          const selection = $getSelection()
          if ($isRangeSelection(selection)) selection.insertNodes([token, trailing])
        }
      } else {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) selection.insertNodes([token, trailing])
        else $getRoot().getLastDescendant()?.insertAfter(token)
      }
      trailing.selectEnd()
    })
    setAutocomplete(null)
    setManualMenu(null)
    setVariableName('')
    window.setTimeout(() => editor.focus(), 0)
  }, [autocomplete])

  useImperativeHandle(ref, () => ({
    focus: () => editorRef.current?.focus(),
    insertToken,
  }), [insertToken])

  const menuKind = autocomplete?.kind ?? manualMenu
  const query = autocomplete?.query.trim().toLocaleLowerCase() ?? ''
  const menuItems = useMemo(() => {
    if (menuKind === 'reference') {
      return assets
        .filter((asset) => asset.name?.trim())
        .filter((asset) => !query || asset.name!.toLocaleLowerCase().includes(query))
        .map((asset) => ({ key: asset.id, name: asset.name!.trim(), detail: asset.mime_type || '图片' }))
    }
    if (menuKind === 'variable') {
      return Object.keys(variables)
        .filter((name) => !query || name.toLocaleLowerCase().includes(query))
        .map((name) => ({ key: name, name, detail: variables[name]?.trim() || '尚未填写' }))
    }
    return []
  }, [assets, menuKind, query, variables])

  useEffect(() => {
    setActiveIndex(0)
    keyboardNavigatingRef.current = false
  }, [menuKind, query])

  function closeMenu() {
    setAutocomplete(null)
    setManualMenu(null)
    setVariableName('')
    keyboardNavigatingRef.current = false
    window.setTimeout(() => editorRef.current?.focus(), 0)
  }

  function handleMenuKeyDown(event: React.KeyboardEvent) {
    if (!menuKind || composingRef.current) return
    const target = event.target instanceof HTMLElement ? event.target : null
    const inVariableForm = Boolean(target?.closest('.prompt-token-menu-create'))
    if (inVariableForm && event.key === 'Enter' && !keyboardNavigatingRef.current) return
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMenu()
    } else if (event.key === 'ArrowDown' && menuItems.length) {
      event.preventDefault()
      keyboardNavigatingRef.current = true
      setActiveIndex((index) => (index + 1) % menuItems.length)
    } else if (event.key === 'ArrowUp' && menuItems.length) {
      event.preventDefault()
      keyboardNavigatingRef.current = true
      setActiveIndex((index) => (index - 1 + menuItems.length) % menuItems.length)
    } else if (event.key === 'Enter' && menuItems[activeIndex]) {
      event.preventDefault()
      insertToken(menuKind, menuItems[activeIndex].name)
    }
  }

  const parsed = parsePromptTemplate(value)
  const variableDraft = variableName || autocomplete?.query || ''
  const normalizedVariableDraft = normalizePromptTemplateName(variableDraft)
  const variableDraftError = variableDraft ? normalizedVariableDraft.error : undefined
  const hoveredAsset = hovered?.kind === 'reference' ? assets.find((asset) => asset.name === hovered.name) : undefined
  const hoveredValue = hovered?.kind === 'variable' ? variables[hovered.name] : undefined

  function showTokenPreview(target: EventTarget | null) {
    const token = target instanceof HTMLElement ? target.closest<HTMLElement>('[data-prompt-token-kind]') : null
    if (!token) {
      setHovered(null)
      return
    }
    const rect = token.getBoundingClientRect()
    setHovered({ kind: token.dataset.promptTokenKind as PromptTemplateTokenKind, name: token.dataset.promptTokenName ?? '', x: rect.left + rect.width / 2, y: rect.top })
  }

  return (
    <div className="prompt-template-shell" data-expanded={expanded || undefined} onKeyDownCapture={handleMenuKeyDown}>
      <div className="prompt-template-toolbar" aria-label="提示词模板工具栏">
        <button type="button" title="插入资产" aria-label="插入资产" disabled={disabled} onClick={() => { setAutocomplete(null); setManualMenu((kind) => kind === 'reference' ? null : 'reference') }}><AtSign size={15} /><span>资产</span></button>
        <button type="button" title="插入变量" aria-label="插入变量" disabled={disabled} onClick={() => { setAutocomplete(null); setManualMenu((kind) => kind === 'variable' ? null : 'variable') }}><Braces size={15} /><span>变量</span></button>
        <span className="prompt-template-toolbar-spacer" />
        <button type="button" title="撤销" aria-label="撤销" disabled={disabled} onClick={() => editorRef.current?.dispatchCommand(UNDO_COMMAND, undefined)}><Undo2 size={15} /></button>
        <button type="button" title="重做" aria-label="重做" disabled={disabled} onClick={() => editorRef.current?.dispatchCommand(REDO_COMMAND, undefined)}><Redo2 size={15} /></button>
      </div>
      <LexicalComposer initialConfig={{
        namespace: 'PromptTemplateEditor',
        nodes: [PromptTokenNode],
        editable: !disabled,
        onError: (error) => { throw error },
        editorState: () => $appendTemplate(value),
      }}>
        <div
          className="prompt-template-editor-frame"
          onMouseOver={(event) => showTokenPreview(event.target)}
          onMouseLeave={() => setHovered(null)}
          onFocus={(event) => showTokenPreview(event.target)}
          onBlur={(event) => {
            if (event.relatedTarget instanceof HTMLElement && event.currentTarget.contains(event.relatedTarget)) {
              showTokenPreview(event.relatedTarget)
              return
            }
            setHovered(null)
          }}
        >
          <PlainTextPlugin
            contentEditable={<ContentEditable className="prompt-template-editor" aria-label="提示词" aria-invalid={Boolean(parsed.error)} />}
            placeholder={<div className="prompt-template-placeholder">{placeholder}</div>}
            ErrorBoundary={LexicalErrorBoundary}
          />
          <HistoryPlugin />
          <OnChangePlugin ignoreSelectionChange onChange={(state) => state.read(() => onChange($getRoot().getTextContent()))} />
          <EditorCapturePlugin editorRef={editorRef} autoFocus={autoFocus} />
          <ExternalValuePlugin value={value} />
          <TokenizeTextPlugin />
          <SelectedTextDeletionPlugin />
          <TokenStatusPlugin assets={assets} variables={variables} />
          <CompositionGuardPlugin composingRef={composingRef} />
          <AutoCompletePlugin composingRef={composingRef} onChange={setAutocomplete} />
        </div>
      </LexicalComposer>
      {parsed.error ? <p className="prompt-template-error"><AlertCircle size={14} />{parsed.error.message}</p> : null}
      <p className="prompt-template-count">{Array.from(value).length} / 4000</p>
      {menuKind ? (
        <div className="prompt-token-menu" role="listbox" aria-label={menuKind === 'reference' ? '选择资产' : '选择或添加变量'}>
          {menuItems.map((item, index) => (
            <button key={item.key} type="button" role="option" aria-selected={index === activeIndex} onMouseDown={(event) => event.preventDefault()} onClick={() => insertToken(menuKind, item.name)}>
              {menuKind === 'reference' ? <AtSign size={15} /> : <Braces size={15} />}
              <span><strong>{item.name}</strong><small>{item.detail}</small></span>
            </button>
          ))}
          {menuKind === 'reference' && onAddAsset ? <button type="button" onMouseDown={(event) => event.preventDefault()} onClick={() => { closeMenu(); onAddAsset() }}><ImagePlus size={15} /><span><strong>添加资产</strong><small>上传或从资产库导入</small></span></button> : null}
          {menuKind === 'variable' ? (
            <form className="prompt-token-menu-create" onSubmit={(event) => {
              event.preventDefault()
              if (!normalizedVariableDraft.error) insertToken('variable', normalizedVariableDraft.name)
            }}>
              <input autoFocus value={variableName} maxLength={64} placeholder="新变量名称" aria-invalid={Boolean(variableDraftError)} aria-describedby={variableDraftError ? variableNameErrorID : undefined} onChange={(event) => { keyboardNavigatingRef.current = false; setVariableName(event.target.value) }} />
              <button type="submit" disabled={Boolean(normalizedVariableDraft.error)}>添加</button>
              {variableDraftError ? <p id={variableNameErrorID} className="prompt-token-menu-create-error" role="alert">{variableDraftError.message}</p> : null}
            </form>
          ) : null}
          {!menuItems.length && menuKind === 'reference' ? <p>没有匹配的资产</p> : null}
        </div>
      ) : null}
      {hovered ? (
        <div className="prompt-token-preview" style={{ left: hovered.x, top: hovered.y }}>
          {hoveredAsset ? <>
            {hoveredAsset.preview_url || hoveredAsset.download_url ? <img src={userApi.imageAssetUrl(hoveredAsset.preview_url || hoveredAsset.download_url || '', accessToken)} alt="" /> : null}
            <div><strong>{hovered.name}</strong><span>{hoveredAsset.width && hoveredAsset.height ? `${hoveredAsset.width} × ${hoveredAsset.height}` : hoveredAsset.mime_type || '图片资产'}</span></div>
          </> : hovered?.kind === 'variable' ? <div><strong>{hovered.name}</strong><span>{hoveredValue?.trim() || '尚未填写变量值'}</span></div> : <div><strong>{hovered.name}</strong><span>未关联资产</span></div>}
        </div>
      ) : null}
    </div>
  )
})

PromptTemplateEditor.displayName = 'PromptTemplateEditor'

function EditorCapturePlugin({ editorRef, autoFocus }: { editorRef: React.MutableRefObject<LexicalEditor | null>; autoFocus: boolean }) {
  const [editor] = useLexicalComposerContext()
  useEffect(() => {
    editorRef.current = editor
    if (autoFocus) window.setTimeout(() => editor.focus(), 0)
    return () => { if (editorRef.current === editor) editorRef.current = null }
  }, [autoFocus, editor, editorRef])
  return null
}

function ExternalValuePlugin({ value }: { value: string }) {
  const [editor] = useLexicalComposerContext()
  useEffect(() => editor.update(() => {
    if ($getRoot().getTextContent() !== value) $appendTemplate(value)
  }), [editor, value])
  return null
}

function TokenizeTextPlugin() {
  const [editor] = useLexicalComposerContext()
  useEffect(() => editor.registerNodeTransform(TextNode, (node) => {
    if ($isPromptTokenNode(node) || !node.isSimpleText()) return
    const parsed = parsePromptTemplate(node.getTextContent())
    if (parsed.error || !parsed.occurrences.length) return
    const replacements: LexicalNode[] = parsed.segments.map((segment) => (
      segment.kind === 'reference' || segment.kind === 'variable'
        ? $createPromptTokenNode(segment.kind, segment.name ?? '')
        : $createTextNode(segment.source)
    )).filter((replacement) => !$isTextNode(replacement) || replacement.getTextContent().length > 0)
    const first = replacements.shift()
    if (!first) return
    node.replace(first)
    let previous = first
    for (const replacement of replacements) {
      previous.insertAfter(replacement)
      previous = replacement
    }
  }), [editor])
  return null
}

function SelectedTextDeletionPlugin() {
  const [editor] = useLexicalComposerContext()
  useEffect(() => {
    const applyDOMSelection = (selection: ReturnType<typeof $getSelection>) => {
      if (!$isRangeSelection(selection) || !selection.isCollapsed()) return selection
      const domSelection = window.getSelection()
      const rootElement = editor.getRootElement()
      if (!rootElement || !domSelection || domSelection.rangeCount === 0) return selection
      const domRange = domSelection.getRangeAt(0)
      if (!rootElement.contains(domRange.commonAncestorContainer)) return selection
      selection.applyDOMRange(domRange)
      return selection
    }
    const deleteSelection = (event: KeyboardEvent) => {
      const selection = $getSelection()
      if (!$isRangeSelection(selection)) return false
      const domSelection = window.getSelection()
      const rootElement = editor.getRootElement()
      if (rootElement && domSelection && !domSelection.isCollapsed && domSelection.rangeCount > 0) {
        const domRange = domSelection.getRangeAt(0)
        const selectionIsInsideEditor = rootElement.contains(domRange.startContainer) && rootElement.contains(domRange.endContainer)
        if (selectionIsInsideEditor && domSelection.toString() === $getRoot().getTextContent()) {
          event.preventDefault()
          $appendTemplate('')
          $getRoot().selectEnd()
          return true
        }
      }
      if (selection.isCollapsed()) {
        if (!rootElement || !domSelection || domSelection.isCollapsed || domSelection.rangeCount === 0) return false
        const domRange = domSelection.getRangeAt(0)
        if (!rootElement.contains(domRange.commonAncestorContainer)) return false
        selection.applyDOMRange(domRange)
      }
      if (selection.isCollapsed()) return false
      event.preventDefault()
      selection.removeText()
      return true
    }
    const insertSpaceAtTokenBoundary = (event: KeyboardEvent) => {
      const selection = applyDOMSelection($getSelection())
      if (!$isRangeSelection(selection) || !selection.isCollapsed()) return false
      const anchorNode = selection.anchor.getNode()
      if (!$isPromptTokenNode(anchorNode)) return false
      event.preventDefault()
      const trailing = $createTextNode(' ')
      if (selection.anchor.offset === 0) anchorNode.insertBefore(trailing)
      else anchorNode.insertAfter(trailing)
      trailing.selectEnd()
      return true
    }
    const unregisterBackspace = editor.registerCommand(KEY_BACKSPACE_COMMAND, deleteSelection, COMMAND_PRIORITY_HIGH)
    const unregisterDelete = editor.registerCommand(KEY_DELETE_COMMAND, deleteSelection, COMMAND_PRIORITY_HIGH)
    const unregisterSpace = editor.registerCommand(KEY_SPACE_COMMAND, insertSpaceAtTokenBoundary, COMMAND_PRIORITY_HIGH)
    return () => {
      unregisterBackspace()
      unregisterDelete()
      unregisterSpace()
    }
  }, [editor])
  return null
}

function TokenStatusPlugin({ assets, variables }: { assets: ReferenceAsset[]; variables: Readonly<Record<string, string>> }) {
  const [editor] = useLexicalComposerContext()
  useEffect(() => {
    const update = () => {
      const root = editor.getRootElement()
      if (!root) return
      const names = new Set(assets.map((asset) => asset.name?.trim()).filter(Boolean))
      root.querySelectorAll<HTMLElement>('[data-prompt-token-kind]').forEach((element) => {
        const kind = element.dataset.promptTokenKind
        const name = element.dataset.promptTokenName ?? ''
        const valid = kind === 'reference' ? names.has(name) : Boolean(variables[name]?.trim())
        element.classList.toggle('prompt-token--invalid', !valid)
        element.setAttribute('aria-invalid', String(!valid))
        element.title = valid ? (kind === 'reference' ? `资产：${name}` : `变量：${name}`) : (kind === 'reference' ? `未关联资产：${name}` : `变量尚未填写：${name}`)
        element.setAttribute('aria-label', element.title)
      })
    }
    update()
    return editor.registerUpdateListener(update)
  }, [assets, editor, variables])
  return null
}

function CompositionGuardPlugin({ composingRef }: { composingRef: React.MutableRefObject<boolean> }) {
  const [editor] = useLexicalComposerContext()
  useEffect(() => {
    const root = editor.getRootElement()
    if (!root) return undefined
    const begin = () => { composingRef.current = true }
    const end = () => { composingRef.current = false }
    root.addEventListener('compositionstart', begin)
    root.addEventListener('compositionend', end)
    return () => {
      root.removeEventListener('compositionstart', begin)
      root.removeEventListener('compositionend', end)
    }
  }, [composingRef, editor])
  return null
}

function AutoCompletePlugin({ composingRef, onChange }: { composingRef: React.MutableRefObject<boolean>; onChange: (range: AutoCompleteRange | null) => void }) {
  const [editor] = useLexicalComposerContext()
  useEffect(() => editor.registerUpdateListener(({ editorState }) => editorState.read(() => {
    if (composingRef.current) return
    const selection = $getSelection()
    if (!$isRangeSelection(selection) || !selection.isCollapsed()) return onChange(null)
    const node = selection.anchor.getNode()
    if (!$isTextNode(node) || $isPromptTokenNode(node)) return onChange(null)
    const end = selection.anchor.offset
    const before = node.getTextContent().slice(0, end)
    const match = before.match(/(?<!\\)([@$])([^@${}\s]*)$/u)
    if (!match) return onChange(null)
    const start = end - match[1].length - match[2].length
    onChange({ nodeKey: node.getKey(), start, end, kind: match[1] === '@' ? 'reference' : 'variable', query: match[2] })
  })), [composingRef, editor, onChange])
  return null
}
