import { createElement } from 'react'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { AdminListPage, ListFilterBar } from './components'

const dataTableSource = readFileSync(new URL('./ui/dataTable.tsx', import.meta.url), 'utf8')
const dataGridSource = readFileSync(new URL('./ui/dataGrid.ts', import.meta.url), 'utf8')

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

for (const tableContract of [
  'min-h-[50px]',
  'sticky top-0',
  'group-hover:bg-',
  'font-[family-name:var(--admin-font-mono)]',
  'overflow-x-auto overscroll-x-contain',
]) {
  if (!dataTableSource.includes(tableContract)) throw new Error(`admin data tables must use ${tableContract}`)
}

for (const gridContract of ['min-h-[50px]', 'font-[family-name:var(--admin-font-mono)]', 'duration-[var(--admin-motion-fast)]']) {
  if (!dataGridSource.includes(gridContract)) throw new Error(`legacy admin data grids must share ${gridContract}`)
}
