import { createElement } from 'react'
import { ActionMenu, Button, IconButton, SegmentedControl, StatusBadge } from './components'

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
