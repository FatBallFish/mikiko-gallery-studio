import { useEffect, useState } from 'react'

export const COMPACT_WORKSPACE_QUERY = '(max-width: 760px)'

export function workspaceParametersHidden(compact: boolean, expanded: boolean) {
  return compact && !expanded
}

export function workspaceGenerateActionVisibility(compact: boolean, expanded: boolean) {
  return {
    compact: compact && !expanded,
    full: !compact || expanded,
  }
}

function compactViewportMatches() {
  return globalThis.window?.matchMedia?.(COMPACT_WORKSPACE_QUERY).matches ?? false
}

export function useCompactWorkspaceViewport() {
  const [compact, setCompact] = useState(compactViewportMatches)

  useEffect(() => {
    const query = window.matchMedia(COMPACT_WORKSPACE_QUERY)
    const update = () => setCompact(query.matches)
    update()
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])

  return compact
}
