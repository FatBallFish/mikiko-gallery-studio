import * as landingContent from './landingContent'

const { landingActionInk, landingChapters } = landingContent
const landingAssetUrl = 'landingAssetUrl' in landingContent
  ? landingContent.landingAssetUrl as (baseUrl: string, assetPath: string) => string
  : null

if (!landingAssetUrl) throw new Error('landing content must expose a base-path-aware asset URL helper')

if (landingAssetUrl('/studio/', '/landing/hero-gallery.webp') !== '/studio/landing/hero-gallery.webp') {
  throw new Error('landing assets must respect a non-root Vite base path')
}

if (landingAssetUrl('/', '/landing/workspace.webp') !== '/landing/workspace.webp') {
  throw new Error('landing assets must preserve root deployments')
}

if (landingActionInk !== '#111218') {
  throw new Error(`landing action contrast ink drifted: ${landingActionInk}`)
}

const serialized = JSON.stringify(landingChapters)

for (const claim of ['文生图', '图片编辑', '参考图', 'OpenAI 兼容', '积分预估']) {
  if (!serialized.includes(claim)) throw new Error(`missing real capability: ${claim}`)
}

for (const banned of ['99.9%', '全球顶尖', 'SECTION 01', '创作工作台']) {
  if (serialized.includes(banned)) throw new Error(`landing page contains unsupported or generic copy: ${banned}`)
}

const chapterOrder = landingChapters.sections.map((section) => section.stage)
if (chapterOrder.join(',') !== 'attention,interest,desire,action') {
  throw new Error(`landing AIDA order drifted: ${chapterOrder.join(',')}`)
}

if (landingChapters.hero.actions.length !== 2) {
  throw new Error(`hero must expose exactly two actions, got ${landingChapters.hero.actions.length}`)
}

const heroActionKinds = landingChapters.hero.actions.map((action) => action.kind).sort()
if (heroActionKinds.join(',') !== 'create,docs') {
  throw new Error(`hero actions must lead to creation and external docs, got ${heroActionKinds.join(',')}`)
}

if (landingChapters.capabilities.length < 3 || landingChapters.capabilities.length > 5) {
  throw new Error(`bento must contain 3-5 intentional items, got ${landingChapters.capabilities.length}`)
}

const occupiedCells = landingChapters.capabilities.reduce(
  (total, capability) => total + capability.columns * capability.rows,
  0,
)
if (occupiedCells !== 24) {
  throw new Error(`desktop bento must occupy all 24 cells, got ${occupiedCells}`)
}
