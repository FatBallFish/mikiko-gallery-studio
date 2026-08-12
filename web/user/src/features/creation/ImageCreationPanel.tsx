import { WorkspacePage } from '../../pages/WorkspacePage'

export function ImageCreationPanel({ initialTaskId }: { initialTaskId?: string }) {
  return <WorkspacePage initialTaskId={initialTaskId} />
}
