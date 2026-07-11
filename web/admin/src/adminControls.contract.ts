import { createElement } from 'react'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { ActionMenu, Button, IconButton, SegmentedControl, StatusBadge } from './components'

const componentsSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const dataTableSource = readFileSync(new URL('./ui/dataTable.tsx', import.meta.url), 'utf8')

createElement(Button, { variant: 'primary', size: 'md' }, '保存')
createElement(Button, { variant: 'danger', size: 'sm' }, '删除')
createElement(IconButton, { label: '刷新', title: '刷新列表', onClick: () => undefined })

createElement(SegmentedControl, {
  value: 'all',
  options: [
    { value: 'all', label: '全部' },
    { value: 'enabled', label: '启用' },
  ],
  onChange: (value: string) => {
    if (!value) throw new Error('segmented value should be stable')
  },
})

createElement(StatusBadge, { tone: 'warning', children: '待处理' })

createElement(ActionMenu, {
  actions: [
    { id: 'disable', label: '禁用', confirm: { title: '确认禁用', expectedValue: 'user@example.com' }, run: () => undefined },
    { id: 'delete', label: '删除', tone: 'danger', confirm: { title: '确认删除', expectedValue: 'user@example.com' }, run: () => undefined },
  ],
})

for (const primitive of [
  'export function MetricStrip',
  'export function InlineFeedback',
  'export function Drawer',
  'export function Modal',
  'export function EmptyBlock',
  'export function LoadingBlock',
]) {
  if (!componentsSource.includes(primitive)) throw new Error(`admin controls must expose ${primitive}`)
}

for (const interaction of [
  "event.key === 'ArrowDown'",
  "event.key === 'ArrowUp'",
  'previousFocus?.focus()',
  'focusableElements(dialog)',
]) {
  if (!componentsSource.includes(interaction)) throw new Error(`admin menus and overlays must support ${interaction}`)
}

for (const dataPrimitive of ['export function FilterToolbar', 'export function SkeletonRows']) {
  if (!dataTableSource.includes(dataPrimitive)) throw new Error(`admin data controls must expose ${dataPrimitive}`)
}
