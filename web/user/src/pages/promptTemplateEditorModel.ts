import type { PromptReferenceBinding, PromptVariableInput } from '../../../shared/api-types'
import { normalizePromptTemplateName, parsePromptTemplate, type PromptTemplateSegment, type PromptTemplateTokenKind } from './promptTemplateParser'

export type PromptEditorSegment = Pick<PromptTemplateSegment, 'kind' | 'source'> & { name?: string }

export function promptTemplateSegments(template: string): PromptEditorSegment[] {
  const parsed = parsePromptTemplate(template)
  if (parsed.error) return [{ kind: 'text', source: template }]
  return parsed.segments.map(({ kind, source, name }) => ({ kind, source, name }))
}
export function promptTemplateText(segments: readonly PromptEditorSegment[]) {
  return segments.map((segment) => segment.source).join('')
}

export function promptVariableNames(template: string) {
  const parsed = parsePromptTemplate(template)
  return parsed.error ? [] : parsed.variableNames
}

export function reconcilePromptVariables(template: string, current: Readonly<Record<string, string>>) {
  return Object.fromEntries(promptVariableNames(template).map((name) => [name, current[name] ?? '']))
}

export function promptVariableValidation(template: string, values: Readonly<Record<string, string>>, maxValueCodePoints = 4000) {
  const names = promptVariableNames(template)
  const missing = names.filter((name) => !values[name]?.trim())
  const tooLong = names.filter((name) => Array.from(values[name] ?? '').length > maxValueCodePoints)
  return { valid: missing.length === 0 && tooLong.length === 0, missing, tooLong }
}

export function buildPromptVariableInputs(template: string, values: Readonly<Record<string, string>>): PromptVariableInput[] {
  return promptVariableNames(template).map((name) => ({ name, value: values[name] ?? '' }))
}

export function buildPromptReferenceBindings(
  template: string,
  assets: ReadonlyArray<{ id: string; name?: string | null }>,
): { bindings: PromptReferenceBinding[]; unresolved: string[] } {
  const parsed = parsePromptTemplate(template)
  if (parsed.error) return { bindings: [], unresolved: [] }
  const byName = new Map<string, { id: string; name: string }>()
  for (const asset of assets) {
    const normalized = normalizePromptTemplateName(asset.name ?? '')
    if (!normalized.error && asset.id) byName.set(normalized.name, { id: asset.id, name: normalized.name })
  }
  const bindings: PromptReferenceBinding[] = []
  const unresolved: string[] = []
  for (const name of parsed.referenceNames) {
    const asset = byName.get(name)
    if (asset) bindings.push({ name, asset_id: asset.id })
    else unresolved.push(name)
  }
  return { bindings, unresolved }
}

export function renamePromptReference(template: string, previousName: string, nextName: string) {
  const parsed = parsePromptTemplate(template)
  if (parsed.error) return template
  const normalizedPrevious = normalizePromptTemplateName(previousName)
  const normalizedNext = normalizePromptTemplateName(nextName)
  if (normalizedPrevious.error || normalizedNext.error) return template
  return parsed.segments.map((segment) => (
    segment.kind === 'reference' && segment.name === normalizedPrevious.name
      ? promptTokenSource('reference', normalizedNext.name)
      : segment.source
  )).join('')
}

export function promptTokenSource(kind: PromptTemplateTokenKind, name: string) {
  return kind === 'reference' ? `{{@${name}}}` : `{{$${name}}}`
}

export function expandedPromptCodePointLength(
  template: string,
  variables: Readonly<Record<string, string>>,
  bindings: readonly PromptReferenceBinding[],
) {
  const parsed = parsePromptTemplate(template)
  if (parsed.error) return Array.from(template).length
  const referenceIndexes = new Map(bindings.map((binding, index) => [binding.name, index + 1]))
  return Array.from(parsed.segments.map((segment) => {
    if (segment.kind === 'variable') return variables[segment.name ?? ''] ?? ''
    if (segment.kind === 'reference') {
      const index = referenceIndexes.get(segment.name ?? '')
      return index ? `图片${index}` : segment.source
    }
    return segment.source
  }).join('')).length
}
