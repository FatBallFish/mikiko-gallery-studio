import { normalizeCanvasDocument, normalizeCreativeCanvas } from './canvas-document'

const normalized = normalizeCanvasDocument({
  schema_version: 1,
  viewport: { x: 0, y: 0, zoom: 1 },
  nodes: null,
  edges: null,
})
if (!Array.isArray(normalized.nodes) || normalized.nodes.length !== 0) throw new Error('null canvas nodes must normalize to an empty array')
if (!Array.isArray(normalized.edges) || normalized.edges.length !== 0) throw new Error('null canvas edges must normalize to an empty array')

const canvas = normalizeCreativeCanvas({
  id: 'canvas-1', project_id: 'project-1', name: 'Blank', revision: 1, metadata_version: 1,
  node_count: 0, edge_count: 0, running_task_count: 0, failed_task_count: 0, status: 'active',
  document: { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: null, edges: null },
})
if (canvas.document.nodes.length !== 0 || canvas.document.edges.length !== 0) throw new Error('creative canvas projection must normalize legacy null arrays')
