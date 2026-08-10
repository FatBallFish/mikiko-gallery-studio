import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
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
  PASTE_COMMAND,
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
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin'
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
    this.setMode('token')
  }

  exportJSON(): SerializedPromptTokenNode {
    return { ...super.exportJSON(), kind: this.__kind, name: this.__name, type: 'prompt-token', version: 1 }
  }

  createDOM(config: EditorConfig) {
    const element = super.createDOM(config)
    element.classList.add('prompt-token')
    element.dataset.promptTokenKind = this.__kind
    element.dataset.promptTokenName = this.__name
    return element
  }

  updateDOM(previous: this, element: HTMLElement, config: EditorConfig) {
    const changed = super.updateDOM(previous, element, config)
    element.dataset.promptTokenKind = this.__kind
    element.dataset.promptTokenName = this.__name
    return changed
  }

  isTextEntity() { return true }
  canInsertTextBefore() { return false }
  canInsertTextAfter() { return false }
}

export function $createPromptTokenNode(kind: PromptTemplateTokenKind, name: string) {
  return new PromptTokenNode(kind, name)
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

  useEffect(() => setActiveIndex(0), [menuKind, query])

  function closeMenu() {
    setAutocomplete(null)
    setManualMenu(null)
    setVariableName('')
    window.setTimeout(() => editorRef.current?.focus(), 0)
  }

  function handleMenuKeyDown(event: React.KeyboardEvent) {
    if (!menuKind || composingRef.current) return
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMenu()
    } else if (event.key === 'ArrowDown' && menuItems.length) {
      event.preventDefault()
      setActiveIndex((index) => (index + 1) % menuItems.length)
    } else if (event.key === 'ArrowUp' && menuItems.length) {
      event.preventDefault()
      setActiveIndex((index) => (index - 1 + menuItems.length) % menuItems.length)
    } else if (event.key === 'Enter' && menuItems[activeIndex]) {
      event.preventDefault()
      insertToken(menuKind, menuItems[activeIndex].name)
    }
  }

  const parsed = parsePromptTemplate(value)
  const hoveredAsset = hovered?.kind === 'reference' ? assets.find((asset) => asset.name === hovered.name) : undefined
  const hoveredValue = hovered?.kind === 'variable' ? variables[hovered.name] : undefined

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
          onMouseOver={(event) => {
            const target = (event.target as HTMLElement).closest<HTMLElement>('[data-prompt-token-kind]')
            if (!target) return
            const rect = target.getBoundingClientRect()
            setHovered({ kind: target.dataset.promptTokenKind as PromptTemplateTokenKind, name: target.dataset.promptTokenName ?? '', x: rect.left + rect.width / 2, y: rect.top })
          }}
          onMouseLeave={() => setHovered(null)}
        >
          <RichTextPlugin
            contentEditable={<ContentEditable className="prompt-template-editor" aria-label="提示词" aria-invalid={Boolean(parsed.error)} />}
            placeholder={<div className="prompt-template-placeholder">{placeholder}</div>}
            ErrorBoundary={LexicalErrorBoundary}
          />
          <HistoryPlugin />
          <OnChangePlugin ignoreSelectionChange onChange={(state) => state.read(() => onChange($getRoot().getTextContent()))} />
          <EditorCapturePlugin editorRef={editorRef} autoFocus={autoFocus} />
          <ExternalValuePlugin value={value} />
          <TokenizeTextPlugin />
          <TokenStatusPlugin assets={assets} variables={variables} />
          <PlainTextPastePlugin />
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
            <form className="prompt-token-menu-create" onSubmit={(event) => { event.preventDefault(); insertToken('variable', variableName || autocomplete?.query || '') }}>
              <input autoFocus value={variableName} maxLength={64} placeholder="新变量名称" onChange={(event) => setVariableName(event.target.value)} />
              <button type="submit" disabled={!normalizePromptTemplateName(variableName || autocomplete?.query || '').name}>添加</button>
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
    if (node instanceof PromptTokenNode || !node.isSimpleText()) return
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
      })
    }
    update()
    return editor.registerUpdateListener(update)
  }, [assets, editor, variables])
  return null
}

function PlainTextPastePlugin() {
  const [editor] = useLexicalComposerContext()
  useEffect(() => editor.registerCommand(PASTE_COMMAND, (event) => {
    const text = event instanceof ClipboardEvent ? event.clipboardData?.getData('text/plain') : ''
    if (text === undefined) return false
    event?.preventDefault()
    const selection = $getSelection()
    if ($isRangeSelection(selection)) selection.insertText(text ?? '')
    return true
  }, COMMAND_PRIORITY_HIGH), [editor])
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
    if (!$isTextNode(node) || node instanceof PromptTokenNode) return onChange(null)
    const end = selection.anchor.offset
    const before = node.getTextContent().slice(0, end)
    const match = before.match(/(^|[\s([{])([@$])([^@${}\s]*)$/u)
    if (!match || (match.index !== undefined && match.index > 0 && before[match.index - 1] === '\\')) return onChange(null)
    const start = end - match[2].length - match[3].length
    onChange({ nodeKey: node.getKey(), start, end, kind: match[2] === '@' ? 'reference' : 'variable', query: match[3] })
  })), [composingRef, editor, onChange])
  return null
}
