import { existsSync, readFileSync } from 'node:fs'

const workspaceURL = new URL('./WorkspacePage.tsx', import.meta.url)
const viewModelURL = new URL('./workspaceViewModel.ts', import.meta.url)
const parametersURL = new URL('./workspaceParameters.ts', import.meta.url)

if (existsSync(parametersURL)) {
  throw new Error('compatibility workspace must not add a parameter model for unsupported generation fields')
}

const workspace = readFileSync(workspaceURL, 'utf8')
const viewModel = readFileSync(viewModelURL, 'utf8')

const unsupportedState = [
  'sizeMode',
  'pixelSize',
  'outputFormat',
  'outputCompression',
  'moderation',
]
for (const field of unsupportedState) {
  if (workspace.includes(field) || viewModel.includes(field)) {
    throw new Error(`compatibility workspace state must not include unsupported field ${field}`)
  }
}

for (const field of ['sizeModes', 'pixelSizes']) {
  if (viewModel.includes(field)) {
    throw new Error(`workspace view model must not expose unsupported parameter ${field}`)
  }
}

const requestStart = workspace.indexOf('const estimatePayload')
const requestEnd = workspace.indexOf('const estimateKey', requestStart)
const requestSource = workspace.slice(requestStart, requestEnd)
for (const field of ['size_mode', 'pixel_size', 'quality', 'output_format', 'output_compression', 'moderation']) {
  if (new RegExp(`\\b${field}\\s*:`).test(requestSource)) {
    throw new Error(`workspace API request must not include unsupported field ${field}`)
  }
}
