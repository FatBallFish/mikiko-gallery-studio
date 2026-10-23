import { createElement } from 'react'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { Button, PageHeader, PageSection, PageToolbar } from './components'

const componentsSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')

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

for (const pageHeaderContract of ['a-page-head', 'a-page-head__copy', 'a-page-head__actions', 'a-kicker']) {
  if (!componentsSource.includes(pageHeaderContract)) {
    throw new Error(`PageHeader must use the Command Deck page header contract: ${pageHeaderContract}`)
  }
}

if (!componentsSource.includes('{resolvedDescription ? <p>{resolvedDescription}</p> : null}')) {
  throw new Error('PageHeader must render its description inside the Command Deck copy column')
}
