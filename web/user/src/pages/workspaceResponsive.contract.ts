import { COMPACT_WORKSPACE_QUERY, workspaceGenerateActionVisibility, workspaceParametersHidden } from './workspaceResponsive'

if (COMPACT_WORKSPACE_QUERY !== '(max-width: 760px)') {
  throw new Error(`workspace compact breakpoint must match the CSS layout, got ${COMPACT_WORKSPACE_QUERY}`)
}

const cases = [
  { compact: true, expanded: false, hidden: true },
  { compact: true, expanded: true, hidden: false },
  { compact: false, expanded: false, hidden: false },
  { compact: false, expanded: true, hidden: false },
]

for (const testCase of cases) {
  const actual = workspaceParametersHidden(testCase.compact, testCase.expanded)
  if (actual !== testCase.hidden) {
    throw new Error(`workspace parameter visibility mismatch for ${JSON.stringify(testCase)}: got ${actual}`)
  }
}

for (const testCase of cases) {
  const visibility = workspaceGenerateActionVisibility(testCase.compact, testCase.expanded)
  const visibleCount = Number(visibility.compact) + Number(visibility.full)
  if (visibleCount !== 1) {
    throw new Error(`workspace must expose exactly one generate action for ${JSON.stringify(testCase)}, got ${JSON.stringify(visibility)}`)
  }
  if (visibility.compact !== (testCase.compact && !testCase.expanded)) {
    throw new Error(`compact generate visibility mismatch for ${JSON.stringify(testCase)}, got ${JSON.stringify(visibility)}`)
  }
}
