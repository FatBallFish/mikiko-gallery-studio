import React, { useState } from 'react'
import { cn } from '../../../shared/classnames'
import { adminButton, adminFeedback } from './classes'
import {
  ChevronUpIcon,
  FilterIcon,
  XIcon,
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
  /** 可选列宽，CSS grid-template-columns 片段 */
  width?: string
  align?: 'left' | 'right' | 'center'
  kind?: 'text' | 'number' | 'code'
  /** 单元格渲染 */
  render: (row: T) => React.ReactNode
}

/* ------------------------------------------------------------------ *
 * 流式筛选条（折叠展开）
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
  const advancedFields = fields.filter((field) => !field.primary)
  const visibleFields = fields.filter((field) => field.primary || open)

  return (
    <section className={cn('flex flex-wrap items-end justify-between gap-3', className)}>
      <div className="flex min-w-0 flex-1 flex-wrap items-end gap-3">
        {visibleFields.map((field) => (
          <label
            key={field.key}
            className="grid gap-1 text-xs font-bold text-[var(--muted)]"
            style={{
              minWidth: field.minWidth ?? '180px',
              flex: '1 1 180px',
              maxWidth: field.maxWidth ?? '280px',
            }}
          >
            <span>{field.label}</span>
            {field.control}
          </label>
        ))}
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {advancedFields.length ? (
          <button
            type="button"
            className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'gap-1.5')}
            aria-expanded={open}
            aria-label={open ? '收起筛选' : '展开更多筛选'}
            title={open ? '收起筛选' : '更多筛选'}
            onClick={() => setOpen(!open)}
          >
            {open ? <ChevronUpIcon className="size-4" /> : <FilterIcon className="size-4" />}
            <span>{open ? '收起' : '更多筛选'}</span>
          </button>
        ) : null}
        {actions}
      </div>
    </section>
  )
}

export function FilterToolbar({
  resultSummary,
  ...props
}: React.ComponentProps<typeof FilterBar> & { resultSummary?: React.ReactNode }) {
  return (
    <div className="grid gap-2 rounded-lg bg-[var(--surface-solid)] p-3">
      <FilterBar {...props} />
      {resultSummary ? <div className="text-xs font-medium text-[var(--soft)]" aria-live="polite">{resultSummary}</div> : null}
    </div>
  )
}

/* ------------------------------------------------------------------ *
 * 通用数据表格
 * ------------------------------------------------------------------ */

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  empty,
  className,
  bodyClassName,
  renderAfterRow,
}: {
  columns: ColumnDef<T>[]
  rows: T[]
  rowKey: (row: T) => string | number
  empty?: React.ReactNode
  className?: string
  bodyClassName?: string
  renderAfterRow?: (row: T) => React.ReactNode
}) {
  const gridTemplate = columns
    .map((col) => col.width ?? 'minmax(120px,1fr)')
    .join(' ')

  if (!rows.length && empty) {
    return <div className={className}>{empty}</div>
  }

  return (
    <div className={cn('min-w-0 overflow-x-auto overscroll-x-contain', className)}>
      <div className="min-w-full" style={{ display: 'grid', gridTemplateColumns: gridTemplate }}>
        {/* 表头 */}
        <div className="contents">
          {columns.map((col) => (
            <div
              key={col.key}
              className={cn(
                'sticky top-0 z-[1] border-b border-[var(--border)] bg-[var(--surface-solid)] px-4 py-3 text-[11px] font-semibold text-[var(--dim)]',
                col.align === 'right' && 'text-right',
                col.align === 'center' && 'text-center',
              )}
            >
              {col.title}
            </div>
          ))}
        </div>
        {/* 数据行 */}
        <div className={cn('contents', bodyClassName)}>
          {rows.map((row) => {
            const afterRow = renderAfterRow?.(row)
            return (
              <React.Fragment key={rowKey(row)}>
                <RowFragment columns={columns} row={row} />
                {afterRow ? <div className="min-w-0" style={{ gridColumn: '1 / -1' }}>{afterRow}</div> : null}
              </React.Fragment>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function RowFragment<T>({ columns, row }: { columns: ColumnDef<T>[]; row: T }) {
  return (
    <div className="group contents" role="row">
      {columns.map((col) => (
        <div
          key={col.key}
          className={cn(
            'flex min-h-[50px] items-center border-b border-[color-mix(in_oklch,var(--border)_72%,transparent)] px-4 py-2 text-sm text-[var(--muted)] transition-colors duration-[var(--admin-motion-fast)] last:border-b-0 group-hover:bg-[color-mix(in_oklch,var(--surface-solid)_94%,var(--accent)_6%)]',
            col.align === 'right' && 'justify-end text-right font-[family-name:var(--admin-font-mono)] tabular-nums',
            col.align === 'center' && 'justify-center text-center',
            col.kind === 'code' && 'font-[family-name:var(--admin-font-mono)] text-xs',
          )}
        >
          {col.render(row)}
        </div>
      ))}
    </div>
  )
}

export function SkeletonRows({ rows = 5 }: { rows?: number }) {
  return (
    <div className="grid gap-px" role="status" aria-label="正在加载列表数据">
      {Array.from({ length: rows }, (_, index) => <div key={index} className={adminFeedback.skeletonRow} />)}
    </div>
  )
}

/* ------------------------------------------------------------------ *
 * 通用分页
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

  return (
    <>
      <div className="flex items-center gap-3">
        <span>
          显示第 {start}-{end} 条 / 共 {total} 条
        </span>
        {onPageSizeChange ? (
          <label className="flex items-center gap-1.5 text-xs font-semibold text-[var(--muted)]">
            <span>每页</span>
            <select
              value={pageSize}
              onChange={(event) => onPageSizeChange(Number(event.target.value))}
              className="min-w-[64px] rounded-md border border-[var(--border)] bg-[var(--surface-solid)] px-2 py-1 text-xs font-bold text-[var(--fg)]"
              aria-label="每页条数"
            >
              {pageSizeOptions.map((size) => (
                <option key={size} value={size}>{size} 条</option>
              ))}
            </select>
          </label>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          className={cn(adminButton.base, adminButton.ghost, adminButton.small)}
          disabled={!hasPrev}
          onClick={() => onChange(page - 1)}
        >
          上一页
        </button>
        <button
          type="button"
          className={cn(adminButton.base, adminButton.primary, adminButton.small)}
          disabled
        >
          {page}
        </button>
        <button
          type="button"
          className={cn(adminButton.base, adminButton.ghost, adminButton.small)}
          disabled={!hasNext}
          onClick={() => onChange(page + 1)}
        >
          下一页
        </button>
      </div>
    </>
  )
}

/* ------------------------------------------------------------------ *
 * 组合容器（可选筛选、可选分页）
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
    <section
      className={cn(
        'grid min-h-0 gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4 shadow-[var(--pg-shadow-sm)]',
        className,
      )}
    >
      {filters || actions ? (
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div className="min-w-0 flex-1">{filters}</div>
          {actions ? (
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 text-xs font-semibold text-[var(--muted)]">
              {actions}
            </div>
          ) : null}
        </header>
      ) : null}
      {resultSummary ? (
        <div className="flex flex-wrap items-center gap-2 text-xs font-semibold text-[var(--muted)]">
          {resultSummary}
        </div>
      ) : null}
      <div className="min-w-0 overflow-hidden">{children}</div>
      {pagination ? (
        <footer className="flex flex-wrap items-center justify-between gap-3 pt-3 text-xs text-[var(--muted)]">
          {pagination}
        </footer>
      ) : null}
    </section>
  )
}
