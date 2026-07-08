import { createElement } from 'react'
import { AdminListPage, ListFilterBar } from './components'

const filters = createElement(ListFilterBar, {
  fields: [
    { key: 'query', label: '关键词', primary: true, control: createElement('input', { placeholder: '搜索' }) },
    { key: 'status', label: '状态', primary: true, control: createElement('select') },
    { key: 'sort', label: '排序', control: createElement('select') },
  ],
  actions: createElement('button', { type: 'submit' }, '筛选'),
})

createElement(AdminListPage, {
  filters,
  resultSummary: createElement('span', null, '共 10 条'),
  pagination: createElement('div', null, '分页'),
  children: createElement('table'),
})

if (!filters.props.fields.some((field: { key: string; primary?: boolean }) => field.key === 'query' && field.primary)) {
  throw new Error('ListFilterBar should keep primary filters always visible')
}

if (!filters.props.fields.some((field: { key: string; primary?: boolean }) => field.key === 'sort' && !field.primary)) {
  throw new Error('ListFilterBar should support collapsible advanced filters')
}
