import { createElement } from 'react'
import { Button, PageHeader, PageSection, PageToolbar } from './components'

createElement(PageHeader, {
  title: '用户管理',
  description: '按用户名、状态和分组定位用户。',
  primaryAction: createElement(Button, { variant: 'primary' }, '新增用户'),
  secondaryActions: createElement(Button, { variant: 'ghost' }, '导出'),
})

createElement(PageToolbar, {
  children: createElement('input', { placeholder: '搜索' }),
  actions: createElement(Button, { variant: 'text' }, '清空'),
})

createElement(PageSection, {
  title: '列表',
  description: '稳定承载后台页面段落。',
  variant: 'plain',
  children: createElement('div'),
})
