export type WorkspaceImageAction = '标记' | '更多'

export type WorkspaceImageActionNotice = {
  title: string
  detail: string
}

export function workspaceUnavailableImageActionNotice(action: WorkspaceImageAction): WorkspaceImageActionNotice {
  if (action === '标记') {
    return {
      title: '前往资产管理',
      detail: '需要管理分组或申请公开时，请先进入资产处理；当前图片仍可下载、继续编辑或提交公开审核。',
    }
  }
  return {
    title: '使用当前可用操作',
    detail: '当前可先使用下载、继续编辑、提交公开审核或前往资产管理图片。',
  }
}
