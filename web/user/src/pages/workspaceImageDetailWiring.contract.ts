// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import { readFileSync } from 'node:fs'

const components = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
const workspace = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

assert.ok(components.includes('detailImage?: ImageResult'), 'workspace previews must carry a complete image detail snapshot')
assert.ok(workspace.includes('previewImage?.detailImage ??'), 'workspace detail modal must prefer the complete snapshot')
assert.ok(workspace.includes("from './workspaceImageDetail'"), 'workspace must use the shared image detail projector')

const projectorCalls = workspace.match(/projectWorkspaceImageDetail\(/g)?.length ?? 0
assert.ok(projectorCalls >= 3, `current output, history cards, and history dialog must share the projector; calls=${projectorCalls}`)

console.log('OK: workspace image detail wiring contract passed')
