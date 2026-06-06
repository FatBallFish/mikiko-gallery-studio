import { workspaceUnavailableImageActionNotice } from './workspaceImageActions'

const markNotice = workspaceUnavailableImageActionNotice('标记')
if (markNotice.title !== '前往图库管理' || !markNotice.detail.includes('图库') || !markNotice.detail.includes('申请公开')) {
  throw new Error(`mark action notice should guide users to gallery/public workflow, got ${JSON.stringify(markNotice)}`)
}

const moreNotice = workspaceUnavailableImageActionNotice('更多')
if (moreNotice.title !== '使用当前可用操作' || !moreNotice.detail.includes('下载') || !moreNotice.detail.includes('继续编辑')) {
  throw new Error(`more action notice should guide users to available image actions, got ${JSON.stringify(moreNotice)}`)
}

const visibleCopy = `${markNotice.title}${markNotice.detail}${moreNotice.title}${moreNotice.detail}`
if (/暂不可用|后续|即将|版本|整理中|not available|TODO/i.test(visibleCopy)) {
  throw new Error(`workspace image action notices should avoid weak roadmap wording, got ${visibleCopy}`)
}
