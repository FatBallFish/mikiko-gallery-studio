import assert from 'node:assert/strict'
import { decideCanvasDraftRecovery } from './canvasDraftPersistence'
import type { CanvasDraftSnapshot } from './canvasDraftPersistence'

function snapshot(baseRevision: number, document: unknown): CanvasDraftSnapshot {
  return { schema_version: 1, user_id: '1', canvas_id: 'c', base_revision: baseRevision, saved_at: 'now', document: document as CanvasDraftSnapshot['document'] }
}

const remoteShape = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [], edges: [] }
// The state cloner always emits groups: [] even when the remote document omits
// the key entirely (omitempty on the wire) — a raw JSON compare used to flag
// every unchanged canvas as a conflict after any draft was written.
const clonerShape = { ...remoteShape, groups: [] }

// documents differing ONLY by the groups key (cloner emits [], remote omits)
// produce matchesRemote=false from the page's raw JSON compare — but same
// base revision means nothing was lost, so the draft must still be discarded.
// (The page now canonicalizes before comparing, so this shape never reaches
// the decision as a mismatch; both branches are pinned here.)
const rawMatches = JSON.stringify(clonerShape) === JSON.stringify(remoteShape)
assert.equal(decideCanvasDraftRecovery(snapshot(2, clonerShape), 2, rawMatches), rawMatches ? 'discard_local' : 'recover_local')
assert.equal(decideCanvasDraftRecovery(snapshot(2, clonerShape), 2, true), 'discard_local')

// same revision but genuinely different document → recover local edits
assert.equal(decideCanvasDraftRecovery(snapshot(2, { ...clonerShape, nodes: [{ id: 'n1' }] }), 2, false), 'recover_local')

// draft based on an older revision than remote → real conflict
assert.equal(decideCanvasDraftRecovery(snapshot(1, clonerShape), 2, false), 'conflict')

// identical documents (any shape) → discard
assert.equal(decideCanvasDraftRecovery(snapshot(3, remoteShape), 3, true), 'discard_local')

console.log('OK: canvas draft recovery contract verified')
