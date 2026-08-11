export type PromptTemplateTokenKind = 'reference' | 'variable'

export type PromptTemplateOccurrence = {
  kind: PromptTemplateTokenKind
  name: string
  start: number
  end: number
}

export type PromptTemplateDiagnostic = {
  field: 'prompt' | 'name'
  name: string
  offset: number
  rule: string
  message: string
}

export type PromptTemplateSegment = {
  kind: 'text' | PromptTemplateTokenKind
  source: string
  name?: string
  start: number
  end: number
}

export type PromptTemplateParseResult = {
  template: string
  canonical: string
  segments: PromptTemplateSegment[]
  occurrences: PromptTemplateOccurrence[]
  referenceNames: string[]
  variableNames: string[]
  error?: PromptTemplateDiagnostic
}

export type PromptTemplateLimits = {
  maxTemplateCodePoints: number
  maxNameCodePoints: number
  maxOccurrences: number
  maxVariables: number
}

const DEFAULT_LIMITS: PromptTemplateLimits = {
  maxTemplateCodePoints: 4000,
  maxNameCodePoints: 64,
  maxOccurrences: 100,
  maxVariables: 50,
}

export function parsePromptTemplate(template: string, overrides: Partial<PromptTemplateLimits> = {}): PromptTemplateParseResult {
  const limits = { ...DEFAULT_LIMITS, ...overrides }
  const codePoints = Array.from(template)
  const result: PromptTemplateParseResult = {
    template,
    canonical: '',
    segments: [],
    occurrences: [],
    referenceNames: [],
    variableNames: [],
  }
  if (codePoints.length > limits.maxTemplateCodePoints) {
    return withError(result, diagnostic('prompt', '', limits.maxTemplateCodePoints, 'template_length', '提示词模板超过长度限制'))
  }

  const canonical: string[] = []
  const references = new Set<string>()
  const variables = new Set<string>()
  let textStart = 0
  const flushText = (end: number) => {
    if (end <= textStart) return
    const source = codePoints.slice(textStart, end).join('')
    appendTextSegment(result.segments, { kind: 'text', source, start: textStart, end })
    canonical.push(source)
  }

  for (let index = 0; index < codePoints.length;) {
    if (codePoints[index] === '\\') {
      const escaped = placeholderPrefix(codePoints, index + 1)
      if (escaped) {
        const closeAt = findClosing(codePoints, index + 1 + escaped.prefixLength)
        if (closeAt >= 0) {
          flushText(index)
          const end = closeAt + 2
          const source = codePoints.slice(index, end).join('')
          appendTextSegment(result.segments, { kind: 'text', source, start: index, end })
          canonical.push(source)
          index = end
          textStart = end
          continue
        }
      }
    }

    const prefix = placeholderPrefix(codePoints, index)
    if (!prefix) {
      index++
      continue
    }
    flushText(index)
    const nameStart = index + prefix.prefixLength
    const closeAt = findClosing(codePoints, nameStart)
    if (closeAt < 0) {
      return withError(result, diagnostic('prompt', '', index, 'unclosed', '提示词占位符未闭合'))
    }
    const rawName = codePoints.slice(nameStart, closeAt)
    if (rawName.some((value) => value === '{' || value === '}')) {
      return withError(result, diagnostic('prompt', '', index, 'nested', '提示词占位符不能嵌套'))
    }
    const normalized = normalizePromptTemplateName(rawName.join(''), limits.maxNameCodePoints)
    if (normalized.error) {
      return withError(result, { ...normalized.error, offset: index })
    }
    const name = normalized.name
    const end = closeAt + 2
    result.segments.push({ kind: prefix.kind, source: codePoints.slice(index, end).join(''), name, start: index, end })
    result.occurrences.push({ kind: prefix.kind, name, start: index, end })
    if (result.occurrences.length > limits.maxOccurrences) {
      return withError(result, diagnostic('prompt', name, index, 'occurrence_limit', '提示词占位符数量超过限制'))
    }
    if (prefix.kind === 'reference') {
      if (!references.has(name)) {
        references.add(name)
        result.referenceNames.push(name)
      }
      canonical.push(`{{@${name}}}`)
    } else {
      if (!variables.has(name)) {
        if (variables.size >= limits.maxVariables) {
          return withError(result, diagnostic('prompt', name, index, 'variable_limit', '提示词变量数量超过限制'))
        }
        variables.add(name)
        result.variableNames.push(name)
      }
      canonical.push(`{{$${name}}}`)
    }
    index = end
    textStart = end
  }
  flushText(codePoints.length)
  result.canonical = canonical.join('')
  return result
}

export function normalizePromptTemplateName(raw: string, maxCodePoints = 64): { name: string; error?: PromptTemplateDiagnostic } {
  const name = raw.trim().normalize('NFC')
  if (!name) return { name: '', error: diagnostic('name', '', 0, 'name_empty', '占位符名称不能为空') }
  if (Array.from(name).length > maxCodePoints) {
    return { name, error: diagnostic('name', name, 0, 'name_length', `占位符名称不能超过 ${maxCodePoints} 个字符`) }
  }
  for (const value of Array.from(name)) {
    const codePoint = value.codePointAt(0) ?? 0
    if (value === '{' || value === '}' || value === '\n' || value === '\r' || value === '\t' || codePoint <= 0x1f || (codePoint >= 0x7f && codePoint <= 0x9f)) {
      return { name, error: diagnostic('name', name, 0, 'name_character', '占位符名称包含不允许的字符') }
    }
  }
  return { name }
}

function placeholderPrefix(codePoints: string[], index: number): { kind: PromptTemplateTokenKind; prefixLength: number } | null {
  if (index < 0 || index + 2 >= codePoints.length || codePoints[index] !== '{' || codePoints[index + 1] !== '{') return null
  if (codePoints[index + 2] === '@') return { kind: 'reference', prefixLength: 3 }
  if (codePoints[index + 2] === '$') return { kind: 'variable', prefixLength: 3 }
  return null
}

function findClosing(codePoints: string[], start: number) {
  for (let index = start; index + 1 < codePoints.length; index++) {
    if (codePoints[index] === '}' && codePoints[index + 1] === '}') return index
  }
  return -1
}

function appendTextSegment(segments: PromptTemplateSegment[], segment: PromptTemplateSegment) {
  const previous = segments[segments.length - 1]
  if (previous?.kind === 'text' && previous.end === segment.start) {
    previous.source += segment.source
    previous.end = segment.end
    return
  }
  segments.push(segment)
}

function diagnostic(field: 'prompt' | 'name', name: string, offset: number, rule: string, message: string): PromptTemplateDiagnostic {
  return { field, name, offset, rule, message }
}

function withError(result: PromptTemplateParseResult, error: PromptTemplateDiagnostic): PromptTemplateParseResult {
  result.error = error
  return result
}
