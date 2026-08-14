import fs from 'node:fs'

const source = fs.readFileSync(new URL('./CanvasListPage.tsx', import.meta.url), 'utf8')
for (const required of [
  "template: 'blank'", "template: 'image_exploration'", "template: 'image_to_video'",
  'userApi.listCanvases', 'userApi.createCanvas', 'userApi.renameCanvas', 'userApi.duplicateCanvas',
  'userApi.transferCanvas', 'userApi.deleteCanvas', 'ProjectSelector', 'RefreshCw',
]) {
  if (!source.includes(required)) throw new Error(`canvas list must include ${required}`)
}
if (!source.includes('running_task_count')) throw new Error('canvas list must surface active runs before destructive actions')
if (!source.includes('Array.isArray(item.document.nodes)')) throw new Error('canvas list preview must defend against legacy null nodes')
