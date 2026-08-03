export type LandingStage = 'attention' | 'interest' | 'desire' | 'action'

export const landingActionInk = '#111218'

export function landingAssetUrl(baseUrl: string, assetPath: string) {
  const normalizedBase = baseUrl.endsWith('/') ? baseUrl : `${baseUrl}/`
  return `${normalizedBase || '/'}${assetPath.replace(/^\/+/, '')}`
}

export type LandingAction = {
  label: string
  kind: 'create' | 'docs'
}

export type LandingAsset = {
  webp: string
  avif: string
  width: number
  height: number
}

function landingAsset(name: string, width: number, height: number): LandingAsset {
  return {
    webp: `/landing/${name}.webp`,
    avif: `/landing/${name}.avif`,
    width,
    height,
  }
}

export type LandingCapability = {
  id: 'generate' | 'edit' | 'reference' | 'estimate'
  title: string
  detail: string
  columns: 2 | 3 | 5 | 7
  rows: 1 | 2
  image?: LandingAsset
  action: LandingAction['kind']
}

export const landingChapters = {
  hero: {
    title: ['连接灵感，也连接模型', '让每张图成为可用资产'],
    summary: '面向创作者与开发者的统一图片生成平台。从参数配置、积分预估到结果归档，在同一条清晰路径中完成。',
    actions: [
      { label: '开始创作', kind: 'create' },
      { label: '阅读 API 文档', kind: 'docs' },
    ] satisfies LandingAction[],
  },
  sections: [
    { stage: 'attention', title: '统一图片生成平台' },
    { stage: 'interest', title: '从意图到结果，能力各有边界' },
    { stage: 'desire', title: '一次配置，完整走完生成链路' },
    { stage: 'action', title: '把下一张图交给清晰的流程' },
  ] satisfies Array<{ stage: LandingStage; title: string }>,
  capabilities: [
    {
      id: 'generate',
      title: '文生图，从一句描述开始',
      detail: '选择抽象模型、质量、比例与数量，不必先理解每个上游的参数差异。',
      columns: 7,
      rows: 2,
      image: landingAsset('studio-showcase-1280', 1280, 720),
      action: 'create',
    },
    {
      id: 'edit',
      title: '图片编辑，沿用已有画面',
      detail: '上传图片并描述修改目标，模型能力不支持时会在提交前明确阻止。',
      columns: 5,
      rows: 1,
      image: landingAsset('capability-edit', 2048, 1152),
      action: 'create',
    },
    {
      id: 'reference',
      title: '参考图生成',
      detail: '用一张或多张参考图约束主体与方向。',
      columns: 3,
      rows: 1,
      image: landingAsset('capability-reference', 1536, 1024),
      action: 'create',
    },
    {
      id: 'estimate',
      title: '积分预估',
      detail: '生成前看见本次消耗。',
      columns: 2,
      rows: 1,
      image: landingAsset('capability-estimate', 1536, 1024),
      action: 'create',
    },
  ] satisfies LandingCapability[],
  workflow: {
    image: landingAsset('workflow-strip', 2048, 768),
    statement: '选择任务类型与抽象模型，配置质量、比例和数量；平台先完成能力校验与积分预估，再进入排队、生成、保存，结果最终回到你的历史资产。',
    steps: ['配置意图', '确认预估', '跟随任务状态', '保存生成结果'],
  },
  modes: [
    {
      id: 'words',
      title: '从文字构建画面',
      detail: '中文与英文提示词均可用于文生图，尺寸、质量与数量按当前模型能力开放。',
      image: landingAsset('mode-text', 1024, 1536),
    },
    {
      id: 'edit',
      title: '在原图上继续',
      detail: '图片编辑需要图片输入；不兼容的模型不会进入扣费与生成流程。',
      image: landingAsset('mode-edit', 1024, 1536),
    },
    {
      id: 'reference',
      title: '让参考图参与表达',
      detail: '参考图数量、格式和大小先校验，输入数量与最终输出数量分别计算。',
      image: landingAsset('mode-reference', 1024, 1536),
    },
  ],
  developer: {
    title: '前端创作与程序接入，共用同一套能力边界',
    detail: '使用平台原生接口创建与查询任务，或通过 OpenAI 兼容的图片生成与编辑接口接入现有应用。AK/SK、任务状态和积分流水保持可追踪。',
    terms: ['原生生成 API', 'OpenAI 兼容', '异步任务查询', 'AK / SK', '模型能力', '积分预估'],
  },
  closing: {
    title: '灵感已经就位，剩下的交给一条清晰链路',
    detail: '进入生成页面完成第一张作品，或从 API 文档开始接入。',
  },
} as const
