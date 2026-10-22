import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import {
  ChevronUpIcon,
  FilterIcon,
} from './listIcons'

/* ------------------------------------------------------------------ *
 * 通用筛选字段配置
 * ------------------------------------------------------------------ */

export type FilterFieldDef = {
  key: string
  label: string
  /** primary 字段常驻显示，非 primary 字段默认折叠，点「更多筛选」展开 */
  primary?: boolean
  /** 输入控件（input/select 等） */
  control: React.ReactNode
  /** 可选最小宽度，默认 180px */
  minWidth?: string
  /** 可选最大宽度，默认 280px，避免字段过宽 */
  maxWidth?: string
}

/* ------------------------------------------------------------------ *
 * 通用列定义
 * ------------------------------------------------------------------ */

export type ColumnDef<T> = {
  key: string
  title: string
  /** 可选列宽提示；表格布局下作为 min-width 使用 */
  width?: string
  align?: 'left' | 'right' | 'center'
  kind?: 'text' | 'number' | 'code'
  /** 单元格渲染 */
  render: (row: T) => React.ReactNode
}

/* ------------------------------------------------------------------ *
 * 筛选带：a-filters
 * ------------------------------------------------------------------ */

export function FilterBar({
  fields,
  actions,
  defaultOpen = false,
  className,
}: {
  fields: FilterFieldDef[]
  actions?: React.ReactNode
  defaultOpen?: boolean
  className?: string
}) {
  const [open, setOpen] = useState(defaultOpen)
  const primaryFields = fields.filter((field) => field.primary)
  const advancedFields = fields.filter((field) => !field.primary)

  return (
    <section className={cn('a-filters', open && 'is-expanded', className)}>
      <div className="a-filters__row">
        {primaryFields.map((field) => (
          <label key={field.key} className="a-field" style={{ minWidth: field.minWidth, maxWidth: field.maxWidth }}>
            <span>{field.label}</span>
            {field.control}
          </label>
        ))}
      </div>
      {advancedFields.length ? (
        <div className="a-filters__row a-filters__more">
          {advancedFields.map((field) => (
            <label key={field.key} className="a-field" style={{ minWidth: field.minWidth, maxWidth: field.maxWidth }}>
              <span>{field.label}</span>
              {field.control}
            </label>
          ))}
        </div>
      ) : null}
      <div className="a-filters__foot">
        <span className="a-filters__result">{fields.length} 个筛选条件</span>
        <div className="a-filters__actions">
          {advancedFields.length ? (
            <button
              type="button"
              className="a-btn a-btn--sm a-btn--quiet"
              aria-expanded={open}
              aria-label={open ? '收起筛选' : '展开更多筛选'}
              title={open ? '收起筛选' : '更多筛选'}
              onClick={() => setOpen(!open)}
            >
              {open ? <ChevronUpIcon /> : <FilterIcon />}
              <span>{open ? '收起' : '更多筛选'}</span>
            </button>
          ) : null}
          {actions}
        </div>
      </div>
    </section>
  )
}

export function FilterToolbar({
  resultSummary,
  ...props
}: React.ComponentProps<typeof FilterBar> & { resultSummary?: React.ReactNode }) {
  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <FilterBar {...props} />
      {resultSummary ? <div className="a-hint" aria-live="polite">{resultSummary}</div> : null}
    </div>
  )
}

/* ------------------------------------------------------------------ *
 * 通用数据表格：a-table-wrap + a-table
 * ------------------------------------------------------------------ */

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  empty,
  className,
  bodyClassName,
  renderAfterRow,
  minWidth = 900,
}: {
  columns: ColumnDef<T>[]
  rows: T[]
  rowKey: (row: T) => string | number
  empty?: React.ReactNode
  className?: string
  bodyClassName?: string
  renderAfterRow?: (row: T) => React.ReactNode
  minWidth?: number
}) {
  if (!rows.length && empty) {
    return <div className={className}>{empty}</div>
  }

  return (
    <div className={cn('a-table-wrap', className)}>
      <table className={cn('a-table', bodyClassName)} style={{ minWidth }}>
        <thead>
          <tr>
            {columns.map((col) => (
              <th
                key={col.key}
                className={cn(
                  col.align === 'right' && 'is-num',
                  col.align === 'center' && 'is-center',
                )}
              >
                {col.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const afterRow = renderAfterRow?.(row)
            return (
              <React.Fragment key={rowKey(row)}>
                <tr>
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className={cn(
                        col.align === 'right' && 'is-num',
                        col.align === 'center' && 'is-center',
                        col.kind === 'code' && 'a-mono',
                      )}
                    >
                      {col.render(row)}
                    </td>
                  ))}
                </tr>
                {afterRow ? (
                  <tr className="a-table__after">
                    <td colSpan={columns.length}>{afterRow}</td>
                  </tr>
                ) : null}
              </React.Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function SkeletonRows({ rows = 5 }: { rows?: number }) {
  return (
    <div style={{ display: 'grid', gap: 1 }} role="status" aria-label="正在加载列表数据">
      {Array.from({ length: rows }, (_, index) => (
        <div key={index} className="a-skeleton" style={{ height: 52 }} />
      ))}
    </div>
  )
}

/* ------------------------------------------------------------------ *
 * 通用分页：a-pager
 * ------------------------------------------------------------------ */

const pageSizeOptions = [10, 20, 50, 100] as const

export function Pager({
  page,
  pageSize,
  total,
  onChange,
  onPageSizeChange,
}: {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
  onPageSizeChange?: (size: number) => void
}) {
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1
  const end = Math.min(page * pageSize, total)
  const hasPrev = page > 1
  const hasNext = page * pageSize < total
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const pageNumbers = Array.from({ length: Math.min(pageCount, 5) }, (_, index) => {
    const first = Math.max(1, Math.min(page - 2, pageCount - 4))
    return first + index
  })

  return (
    <>
      <div className="a-pager__info">
        显示 <b>{start}–{end}</b>，共 <b>{total}</b> 条
      </div>
      <div className="a-pager__controls">
        <button type="button" className="a-pager__btn" disabled={!hasPrev} aria-label="上一页" onClick={() => onChange(page - 1)}>‹</button>
        {pageNumbers.map((number) => (
          <button
            key={number}
            type="button"
            className={cn('a-pager__btn', number === page && 'is-active')}
            aria-current={number === page ? 'page' : undefined}
            onClick={() => onChange(number)}
          >
            {number}
          </button>
        ))}
        <button type="button" className="a-pager__btn" disabled={!hasNext} aria-label="下一页" onClick={() => onChange(page + 1)}>›</button>
      </div>
      {onPageSizeChange ? (
        <label className="a-pager__size">
          <span>每页</span>
          <select
            value={pageSize}
            onChange={(event) => onPageSizeChange(Number(event.target.value))}
            aria-label="每页条数"
          >
            {pageSizeOptions.map((size) => (
              <option key={size} value={size}>{size}</option>
            ))}
          </select>
        </label>
      ) : <span />}
    </>
  )
}

/* ------------------------------------------------------------------ *
 * 组合容器
 * ------------------------------------------------------------------ */

export function ListPage({
  filters,
  actions,
  resultSummary,
  pagination,
  children,
  className,
}: {
  filters?: React.ReactNode
  actions?: React.ReactNode
  resultSummary?: React.ReactNode
  pagination?: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('a-page-stack', className)}>
      {filters}
      <section className="a-panel a-panel--flush">
        {actions || resultSummary ? (
          <header className="a-panel__head">
            <div className="a-panel__head-copy">{resultSummary}</div>
            {actions ? <div className="a-row-actions">{actions}</div> : null}
          </header>
        ) : null}
        <div style={{ minWidth: 0, overflow: 'hidden' }}>{children}</div>
        {pagination ? <footer className="a-pager">{pagination}</footer> : null}
      </section>
    </div>
  )
}