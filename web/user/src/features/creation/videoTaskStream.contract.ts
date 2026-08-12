import fs from 'node:fs'

const source = fs.readFileSync(new URL('./VideoCreationPanel.tsx', import.meta.url), 'utf8')

for (const required of [
  'new EventSource(userApi.videoTaskStreamUrl(token, projects.selectedProjectID))',
  "source.addEventListener('task'",
  'userApi.getVideoTask(taskID)',
  'userApi.listVideoTasks({ project_id: projects.selectedProjectID, limit: 20 })',
  'window.setTimeout',
]) {
  if (!source.includes(required)) throw new Error(`video task stream must include ${required}`)
}
if (source.includes('window.setInterval')) throw new Error('video tasks must not use permanent polling when SSE is available')
if (source.includes('window.location.reload()')) throw new Error('task refresh must remain local to the video task')
