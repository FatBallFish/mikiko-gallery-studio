import { autoLayoutCanvasNodes, computeCanvasBounds, fitCanvasViewport, minimapGeometry, visibleCanvasNodeIDs } from './canvasLayout'
import type { CanvasDocument } from './types'

const document: CanvasDocument = {
  schema_version: 1,
  viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [
    { id: 'prompt', type: 'prompt', position: { x: 100, y: 100 }, size: { width: 240, height: 160 } },
    { id: 'generator', type: 'video_generation', position: { x: 100, y: 100 }, size: { width: 320, height: 220 } },
    { id: 'result', type: 'video', asset_id: 'asset-video', position: { x: 100, y: 100 }, size: { width: 320, height: 200 } },
    { id: 'outside', type: 'note', position: { x: -800, y: 700 }, size: { width: 200, height: 140 } },
  ],
  edges: [
    { id: 'prompt-gen', source: 'prompt', target: 'generator', input_role: 'prompt' },
    { id: 'gen-result', source: 'generator', target: 'result', input_role: 'result' },
  ],
}

const arranged = autoLayoutCanvasNodes(document, ['prompt', 'generator', 'result'])
const prompt = arranged.nodes.find((node) => node.id === 'prompt')!
const generator = arranged.nodes.find((node) => node.id === 'generator')!
const result = arranged.nodes.find((node) => node.id === 'result')!
if (!(prompt.position.x < generator.position.x && generator.position.x < result.position.x)) throw new Error(`graph layout must follow edge direction: ${JSON.stringify(arranged.nodes)}`)
if (arranged.nodes.find((node) => node.id === 'outside')?.position.x !== -800) throw new Error('automatic layout must not move unselected nodes')

const bounds = computeCanvasBounds(arranged.nodes)
const fitted = fitCanvasViewport(bounds, { width: 1200, height: 800 }, 64)
if (fitted.zoom <= 0 || fitted.zoom > 2 || !Number.isFinite(fitted.x) || !Number.isFinite(fitted.y)) throw new Error(`fit viewport must be finite and bounded: ${JSON.stringify(fitted)}`)

const minimap = minimapGeometry(arranged.nodes, fitted, { width: 1200, height: 800 }, { width: 220, height: 140 })
if (minimap.nodes.length !== 4 || minimap.viewport.width <= 0 || minimap.viewport.height <= 0) throw new Error(`minimap geometry must include nodes and viewport: ${JSON.stringify(minimap)}`)

const visible = visibleCanvasNodeIDs(document.nodes, { x: -100, y: -100, zoom: 1 }, { width: 640, height: 480 }, 40, ['outside'])
if (!visible.has('prompt') || !visible.has('generator') || !visible.has('result')) throw new Error(`viewport culling must retain visible nodes: ${Array.from(visible)}`)
if (!visible.has('outside')) throw new Error('viewport culling must retain explicitly pinned nodes such as active or selected nodes')
const culled = visibleCanvasNodeIDs(document.nodes, { x: -100, y: -100, zoom: 1 }, { width: 640, height: 480 }, 40)
if (culled.has('outside')) throw new Error('viewport culling must omit distant nodes outside the overscan area')

const largeNodes = Array.from({ length: 200 }, (_, index) => ({
  id: `large-${index}`,
  type: index % 4 === 0 ? 'image' as const : 'note' as const,
  position: { x: (index % 20) * 360, y: Math.floor(index / 20) * 260 },
  size: { width: 280, height: 200 },
}))
const largeEdges = Array.from({ length: 300 }, (_, index) => ({
  id: `large-edge-${index}`,
  source: largeNodes[index % largeNodes.length].id,
  target: largeNodes[(index + 1) % largeNodes.length].id,
  input_role: 'result' as const,
}))
const startedAt = performance.now()
const largeVisible = visibleCanvasNodeIDs(largeNodes, { x: 0, y: 0, zoom: 1 }, { width: 1280, height: 800 }, 180)
const largeMap = new Map(largeNodes.map((node) => [node.id, node]))
for (const edge of largeEdges) {
  if (!largeMap.has(edge.source) || !largeMap.has(edge.target)) throw new Error('300-edge benchmark must retain resolvable endpoints')
}
const elapsed = performance.now() - startedAt
if (!largeVisible.size || largeVisible.size >= 50) throw new Error(`200-node benchmark must cull distant media nodes: ${largeVisible.size}`)
if (elapsed >= 100) throw new Error(`200-node/300-edge interaction index must remain below the 100ms long-task budget: ${elapsed.toFixed(2)}ms`)
