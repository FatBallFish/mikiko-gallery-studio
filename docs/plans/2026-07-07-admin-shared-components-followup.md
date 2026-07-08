# 管理后台共性组件二次整改方案

## 背景

基于上一轮管理端 UI/UX 改造后实机走查，当前仍存在跨页面共性问题：

1. 页面组件背景不统一，同一页面混用透明背景和白色面板。
2. 部分列表/筛选组件存在边框套边框，层级显得过深。
3. 用户、审核、兑换码、订单等列表页各自实现筛选/列表/分页样式，缺少统一组件契约。
4. 系统设置不应拆为三个一级导航，应聚合为一个「系统设置」页面，通过横向子 Tab 切换通用配置、安全配置、存储配置。
5. Tab 组件需要抽象成通用组件，并支持横向/纵向布局，替换管理端已有自定义 Tab/Segmented 样式。

## 落地契约

### 1. 统一面板背景

管理端页面中的主要内容容器统一使用浅色面板背景：

```ts
const adminSurface = {
  panel: 'border border-[var(--border)] bg-[var(--surface)]',
  panelSubtle: 'border border-transparent bg-[var(--surface)]',
}
```

列表筛选区、健康卡片、统计区、表格容器不得继续使用纯透明背景作为默认状态。

### 2. 减少嵌套边框

列表页结构统一为：

```tsx
<AdminListPage>
  <ListToolbar />
  <DataSurface>
    <DataRows />
    <Pagination />
  </DataSurface>
</AdminListPage>
```

禁止默认结构：

```tsx
<Panel>
  <Panel>
    <Filters />
  </Panel>
  <Panel>
    <Table />
  </Panel>
</Panel>
```

筛选区在列表页内部默认不画外层边框，只通过背景、间距和控件边界区分。

### 3. 通用列表组件

新增 `AdminListPage`，集中承载筛选、数据区、分页：

```tsx
type AdminListPageProps = {
  filters?: ReactNode
  actions?: ReactNode
  children: ReactNode
  pagination?: ReactNode
  defaultFiltersOpen?: boolean
  collapsibleFilters?: boolean
  resultSummary?: ReactNode
}
```

新增 `ListFilterBar`，采用流式布局：

```tsx
type FilterField = {
  key: string
  label?: string
  primary?: boolean
  control: ReactNode
}

type ListFilterBarProps = {
  fields: FilterField[]
  actions?: ReactNode
  advancedOpen?: boolean
  onAdvancedOpenChange?: (open: boolean) => void
}
```

规则：

- `primary=true` 的条件始终展示。
- 其他条件在窄屏或收起状态下隐藏到高级筛选。
- 筛选区使用 flex wrap，不占用大块页面。

### 4. 系统设置聚合

导航恢复为一个一级页面：

```ts
{ id: 'system-settings', label: '系统设置' }
```

兼容别名：

```ts
general-settings -> system-settings?tab=general
security-settings -> system-settings?tab=security
storage-settings -> system-settings?tab=storage
config -> system-settings?tab=general
security-config -> system-settings?tab=security
storage-config -> system-settings?tab=storage
```

页面结构：

```tsx
<SystemSettingsPage>
  <Tabs orientation="horizontal" />
  {tab === 'general' && <ConfigPage compact />}
  {tab === 'security' && <SecurityConfigPage compact />}
  {tab === 'storage' && <StorageConfigPage compact />}
</SystemSettingsPage>
```

### 5. 通用 Tabs 组件

新增：

```tsx
type AdminTabItem<T extends string> = {
  id: T
  label: string
  description?: string
  disabled?: boolean
  badge?: ReactNode
}

type AdminTabsProps<T extends string> = {
  items: AdminTabItem<T>[]
  value: T
  onChange: (value: T) => void
  orientation?: 'horizontal' | 'vertical'
  ariaLabel: string
}
```

要求：

- 横向用于系统设置、支付配置等页面级子视图。
- 纵向用于通用配置内类目切换。
- 使用 `role="tablist"` / `role="tab"`，并通过统一样式控制 active/hover/focus。

## 验收

- `#/system-settings` 展示通用配置、安全配置、存储配置三个横向 Tab。
- 左侧导航不再出现通用配置/安全配置/存储配置三个一级项。
- 旧链接 `#/general-settings`、`#/security-settings`、`#/storage-settings` 可进入系统设置并定位到对应 Tab。
- 用户、审核、兑换码、订单至少迁移到 `AdminListPage` / `ListFilterBar`。
- 系统健康页的主要卡片和面板不再混用透明背景。
- 用户页筛选区不再边框套边框。
- 通用 Tabs 组件有 contract 覆盖横向和纵向配置。
