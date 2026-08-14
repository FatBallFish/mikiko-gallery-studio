import type { CanvasDocument, CreativeCanvas } from './api-types'

function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

export function normalizeCanvasDocument(value: unknown): CanvasDocument {
  const document = asRecord(value)
  const viewport = asRecord(document.viewport)
  return {
    ...document,
    schema_version: 1,
    viewport: {
      x: typeof viewport.x === 'number' ? viewport.x : 0,
      y: typeof viewport.y === 'number' ? viewport.y : 0,
      zoom: typeof viewport.zoom === 'number' && viewport.zoom > 0 ? viewport.zoom : 1,
    },
    nodes: Array.isArray(document.nodes) ? document.nodes : [],
    edges: Array.isArray(document.edges) ? document.edges : [],
  } as CanvasDocument
}

export function normalizeCreativeCanvas(value: unknown): CreativeCanvas {
  const canvas = asRecord(value)
  return { ...canvas, document: normalizeCanvasDocument(canvas.document) } as CreativeCanvas
}
