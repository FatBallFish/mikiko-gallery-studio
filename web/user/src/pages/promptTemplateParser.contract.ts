import { readFileSync } from 'node:fs'
import { parsePromptTemplate } from './promptTemplateParser'

type Fixture = {
  valid: Array<{
    name: string
    template: string
    canonical: string
    references: string[]
    variables: string[]
    occurrences: Array<{ kind: 'reference' | 'variable'; name: string; start: number; end: number }>
  }>
  invalid: Array<{ name: string; template: string; rule: string; offset: number }>
}

function assertDeepEqual(actual: unknown, expected: unknown, message: string) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${message}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`)
  }
}

const fixtureURL = new URL('../../../../testdata/prompt-template-fixtures.json', import.meta.url)
const fixtures = JSON.parse(readFileSync(fixtureURL, 'utf8')) as Fixture

for (const fixture of fixtures.valid) {
  const result = parsePromptTemplate(fixture.template)
  if (result.error) throw new Error(`${fixture.name}: unexpected ${result.error.rule}`)
  assertDeepEqual(result.canonical, fixture.canonical, `${fixture.name} canonical`)
  assertDeepEqual(result.referenceNames, fixture.references, `${fixture.name} references`)
  assertDeepEqual(result.variableNames, fixture.variables, `${fixture.name} variables`)
  assertDeepEqual(result.occurrences, fixture.occurrences, `${fixture.name} occurrences`)
}

for (const fixture of fixtures.invalid) {
  const result = parsePromptTemplate(fixture.template)
  if (!result.error || result.error.rule !== fixture.rule || result.error.offset !== fixture.offset) {
    throw new Error(`${fixture.name}: expected ${fixture.rule}@${fixture.offset}, got ${JSON.stringify(result.error)}`)
  }
}

const manyVariables = Array.from({ length: 51 }, (_, index) => `{{$变量${index + 1}}}`).join('')
const variableLimit = parsePromptTemplate(manyVariables)
if (variableLimit.error?.rule !== 'variable_limit') {
  throw new Error(`distinct variable limit must match Go parser: ${JSON.stringify(variableLimit.error)}`)
}

const namespaces = parsePromptTemplate('{{@主体}}{{@主体}}{{@subject}}{{@Subject}}{{$主体}}')
assertDeepEqual(namespaces.referenceNames, ['主体', 'subject', 'Subject'], 'reference names must be case-sensitive and deduplicated')
assertDeepEqual(namespaces.variableNames, ['主体'], 'resource and variable namespaces must remain independent')
