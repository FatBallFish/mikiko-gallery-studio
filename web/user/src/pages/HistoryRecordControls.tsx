import { useRef, useState } from 'react'
import { Button, Modal } from '../components'
import { errorMessage } from '../useApiResource'
import { createHistoryActionLock } from './historyRecordActions'

export function useHistoryRecordActions() {
  const lock = useRef(createHistoryActionLock())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [confirmation, setConfirmation] = useState<{ all: boolean; action: () => Promise<void> } | null>(null)
  async function run(action: () => Promise<void>) {
    await lock.current.run(async () => {
      setBusy(true)
      setError('')
      try { await action(); setConfirmation(null) } catch (cause) { setError(errorMessage(cause)) } finally { setBusy(false) }
    })
  }
  return {
    busy, run,
    confirm: (all: boolean, action: () => Promise<void>) => { if (!busy) { setError(''); setConfirmation({ all, action }) } },
    feedback: <>
      {error && !confirmation ? <p className="cr-inline-error" role="alert">{error}</p> : null}
      {confirmation ? <Modal title={confirmation.all ? '清空全部失败记录' : '删除历史记录'} onClose={() => { if (!busy) setConfirmation(null) }}>
        <p>{confirmation.all ? '将清空当前项目全部完全失败的历史记录（不限当前页或筛选结果）。部分成功、已取消和已拒绝的记录会保留。' : '仅从创作历史隐藏此记录。'}</p>
        <p>已生成素材、引用资源和积分记录都会保留。</p>
        {error ? <p className="cr-inline-error" role="alert">{error}</p> : null}
        <div className="u-actions">
          <Button tone="ghost" disabled={busy} onClick={() => setConfirmation(null)}>取消</Button>
          <Button disabled={busy} onClick={() => void run(confirmation.action)}>{busy ? '处理中...' : '确认删除记录'}</Button>
        </div>
      </Modal> : null}
    </>,
  }
}
