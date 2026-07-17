# Admin UI/UX Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 基于 `docs/audits/admin-ui-ux-2026-07-07/admin-ui-ux-audit.md`，把管理后台改造成风格统一、层级清晰、交互简洁流畅的低噪声运营控制台。

**Architecture:** 先建立统一后台骨架和组件契约，再迁移高频页面，最后处理复杂配置流。核心原则是：全局 Shell 只负责导航和账号，页面内容由 `PageHeader + PageToolbar + PageSection + DataTable/Form/State` 组成；复杂编辑统一使用 Drawer/SidePanel，短表单保留 Modal；系统设置拆成独立一级页面。

**Tech Stack:** React + TypeScript + Vite + Tailwind CSS utilities + existing `web/shared/admin-api.ts` API client + existing contract scripts.

---

## 0. 输入与边界

### 输入来源

- 问题报告：`docs/audits/admin-ui-ux-2026-07-07/admin-ui-ux-audit.md`
- 截图证据：`docs/audits/admin-ui-ux-2026-07-07/screenshots/`
- 当前管理端实现：
  - `web/admin/src/App.tsx`
  - `web/admin/src/components.tsx`
  - `web/admin/src/ui/classes.ts`
  - `web/admin/src/styles.css`
  - `web/admin/src/layout/admin-navigation.ts`
  - `web/admin/src/types.ts`
  - `web/admin/src/pages/*.tsx`

### 不做的事

- 不改后端 API 语义，除非某个页面没有可支撑的字段。若需要新增 API，必须另开后端设计。
- 不更换前端框架。
- 不做营销页式重设计，不新增大面积装饰图形。
- 不以“优化一下样式”替代组件契约和页面结构整改。

### 正式编码前置

正式执行本计划前，必须遵守仓库约束：

1. 使用 `dev-start-coding`。
2. `.coding-context.json` 的 requirement/design source 至少包含：
   - `docs/audits/admin-ui-ux-2026-07-07/admin-ui-ux-audit.md`
   - `docs/plans/2026-07-07-admin-ui-ux-remediation-tech-plan.md`
3. 触碰 React/TypeScript/CSS 前使用 `dev-react-patterns`。

## 1. 设计方向与不可变契约

### 1.1 目标风格

风格定义为 **Quiet Ops Console / 低噪声运营控制台**：

- 深色为默认，但降低边框、发光、厚重卡片的数量。
- 信息优先，装饰后退。
- 面向运营和管理员的重复使用场景，强调扫描、比较、执行。
- 组件稳定，页面不得各自发明新的主按钮、Tab、表单和空状态。

### 1.2 全局页面结构契约

每个受保护页面必须符合：

```tsx
<AdminLayout route session ...>
  <PageScaffold>
    <PageHeader
      title="用户管理"
      description="按用户名、状态和分组定位用户。"
      primaryAction={<Button variant="primary">新增用户</Button>}
      secondaryActions={<Button variant="ghost">导出</Button>}
    />
    <PageToolbar>
      <SearchInput />
      <FilterSelect />
      <ToolbarActions />
    </PageToolbar>
    <PageContent>
      <ResponsiveDataTable />
    </PageContent>
  </PageScaffold>
</AdminLayout>
```

禁止：

```tsx
// 禁止每页重复渲染全局状态条
<StatusStrip>
  <StatusCell label="Current View" />
  <StatusCell label="Admin Role" />
</StatusStrip>

// 禁止卡片套卡片作为默认布局
<Card>
  <Card>
    <Table />
  </Card>
</Card>
```

### 1.3 操作层级契约

```ts
type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'text'

type PageActionModel = {
  primary?: ReactNode      // 每页最多一个
  secondary?: ReactNode[]  // 最多三个，超过进入 MoreMenu
  destructive?: never      // 危险操作不得出现在 PageHeader
}

type RowActionModel = {
  primary: '详情' | '查看'
  secondary?: ReactNode
  overflow: Array<RowAction>
}

type RowAction = {
  id: string
  label: string
  tone?: 'neutral' | 'danger'
  confirm?: ConfirmSpec
  run: () => Promise<void> | void
}
```

验收规则：

- 页面首屏只有一个最高视觉权重按钮。
- 表格行内常驻按钮最多两个。
- `删除`、`禁用`、`退款`、`清空密钥`、`设为默认写入` 必须经过确认。

### 1.4 表单容器选择契约

```ts
type EditSurface = 'modal' | 'sidePanel' | 'drawer' | 'fullPage'

function resolveEditSurface(spec: {
  fields: number
  hasSecrets?: boolean
  hasJson?: boolean
  hasSteps?: boolean
  risk?: 'low' | 'medium' | 'high'
}): EditSurface {
  if (spec.hasSteps || spec.hasSecrets || spec.hasJson || spec.risk === 'high') return 'drawer'
  if (spec.fields <= 6) return 'modal'
  if (spec.fields <= 12) return 'sidePanel'
  return 'drawer'
}
```

落地规则：

- 新增用户：Modal。
- 新增路由模型：Modal + 保存后下一步 CTA。
- 支付通道实例：Drawer + 分步。
- 存储配置：独立页面 + 右侧详情编辑 + 新建向导 Drawer。
- 安全配置：独立页面内分组表单，不塞进通用设置 Tab。

### 1.5 响应式契约

```tsx
function AdminLayout() {
  const isNarrow = useMediaQuery('(max-width: 920px)')
  return isNarrow ? <MobileAdminShell /> : <DesktopAdminShell />
}

function MobileAdminShell() {
  return (
    <main className="min-h-screen">
      <MobileTopbar menuButton accountMenu />
      <Drawer open={navOpen}><SidebarNav /></Drawer>
      <section className="w-full min-w-0 px-4 py-4">
        {children}
      </section>
    </main>
  )
}
```

验收规则：

- `390px` 宽度截图不得出现右侧大空白。
- 侧栏不得参与移动端内容流。
- 表格在移动端必须有横向滚动容器或卡片化列表。

## 2. 新增/调整通用组件

### Task 1: 建立 Design Tokens 和低噪声基础样式

**对应报告问题：** 卡片过多、圆角过大、发光过多、命名混乱、长期使用疲劳。

**Files:**

- Modify: `web/admin/src/styles.css`
- Modify: `web/admin/src/ui/classes.ts`
- Test: `web/admin/src/ui/adminDesignSystem.contract.ts`

**实现契约：**

```ts
export const adminTokens = {
  radius: {
    xs: '6px',
    sm: '8px',
    md: '10px',
    lg: '14px',
  },
  surface: {
    page: 'var(--bg)',
    panel: 'var(--surface)',
    panelSubtle: 'var(--surface-subtle)',
  },
  focus: '0 0 0 3px color-mix(in oklch, var(--accent) 22%, transparent)',
}
```

**CSS 必须调整：**

```css
:root {
  --pg-radius-sm: 8px;
  --pg-radius-md: 10px;
  --pg-radius-lg: 14px;
  --pg-admin-card-border-alpha: 0.10;
  --pg-admin-glow-alpha: 0.10;
}

.admin-card {
  border-radius: var(--pg-radius-md);
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: none;
}
```

**验收：**

- 全局不再默认使用 `rounded-3xl` 作为后台主容器。
- `adminButton.primary` 不再使用强发光大阴影，只保留可感知的 focus/hover。
- 执行：

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
```

### Task 2: 重构 AdminLayout，移除重复状态条并修复移动端

**对应报告问题：** P0 页面骨架混乱、P0 移动端不可用。

**Files:**

- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/classes.ts`
- Modify: `web/admin/src/styles.css`
- Test: `web/admin/src/adminLayout.contract.ts`
- Test: `web/admin/src/shellResponsive.contract.ts`

**Desktop 契约：**

```tsx
function DesktopAdminShell(props) {
  return (
    <main className={adminShell.root}>
      <aside className={adminShell.sidebar}>
        <Brand />
        <SidebarNav groups={visibleNavGroups} />
        <SidebarAccount />
      </aside>
      <section className={adminShell.main}>
        <Topbar
          healthSummary={providerSummary}
          reviewCount={reviewCount}
          configDrafts={configDrafts}
          account={session}
        />
        <section className={adminShell.content}>{children}</section>
      </section>
    </main>
  )
}
```

**必须删除：**

```tsx
<div className={adminShell.statusStrip} aria-label="后台状态摘要">...</div>
```

**Mobile 契约：**

```tsx
function AdminLayout() {
  const [navOpen, setNavOpen] = useState(false)
  return (
    <main className="admin-shell">
      <header className="admin-mobile-topbar">
        <IconButton label="打开导航" onClick={() => setNavOpen(true)} />
        <CurrentPageTitle />
        <AccountMenu />
      </header>
      <SidebarNav desktop />
      <Drawer open={navOpen} onClose={() => setNavOpen(false)}>
        <SidebarNav onNavigate={() => setNavOpen(false)} />
      </Drawer>
      <section className="admin-content">{children}</section>
    </main>
  )
}
```

**验收：**

- `390x844` 截图：侧栏不重复出现在内容中。
- `1280x720` 截图：没有 Current View/Admin Role 全局状态条。
- `adminLayout.contract.ts` 检查 `adminShell.statusStrip` 不再被 `AdminLayout` 使用。

### Task 3: 统一 PageHeader、PageToolbar、PageSection

**对应报告问题：** 页面标题区不稳定、主操作位置不稳定、筛选区卡片过重。

**Files:**

- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/classes.ts`
- Test: `web/admin/src/pageScaffold.contract.ts`

**组件契约：**

```tsx
type PageHeaderProps = {
  title: string
  description?: string
  meta?: ReactNode
  primaryAction?: ReactNode
  secondaryActions?: ReactNode
}

export function PageHeader(props: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header-copy">
        <h1>{props.title}</h1>
        {props.description && <p>{props.description}</p>}
      </div>
      <div className="page-header-actions">
        {props.secondaryActions}
        {props.primaryAction}
      </div>
    </header>
  )
}

export function PageToolbar({ children, actions }) {
  return (
    <section className="page-toolbar">
      <div className="page-toolbar-controls">{children}</div>
      <div className="page-toolbar-actions">{actions}</div>
    </section>
  )
}

export function PageSection({ title, description, children, variant = 'plain' }) {
  return (
    <section className={variant === 'panel' ? 'section-panel' : 'section-plain'}>
      <header>...</header>
      {children}
    </section>
  )
}
```

**验收：**

- `PageHeader` 不再使用 `label` 作为页面标题。
- 页面标题使用 `h1`，段落说明使用普通文本。
- `PageToolbar` 不使用 `rounded-3xl` 大卡片。

### Task 4: 统一 Button、IconButton、SegmentedControl、StatusBadge

**对应报告问题：** 按钮层级不稳定、Tab/胶囊/按钮混用。

**Files:**

- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/classes.ts`
- Test: `web/admin/src/adminControls.contract.ts`

**组件契约：**

```tsx
type ButtonProps = {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'text'
  size?: 'sm' | 'md'
  icon?: ReactNode
  children: ReactNode
}

function Button({ variant = 'secondary', size = 'md', ...props }: ButtonProps) {
  return <button className={buttonClass({ variant, size })} {...props} />
}

type SegmentedOption<T extends string> = { value: T; label: string; disabled?: boolean }

function SegmentedControl<T extends string>({ value, options, onChange }) {
  return (
    <div role="tablist" className="segmented">
      {options.map(option => (
        <button
          role="tab"
          aria-selected={value === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
```

**验收：**

- 所有页面主 CTA 使用 `<Button variant="primary">`。
- 所有图标按钮必须带 `aria-label` 和 `title` 或 tooltip。
- 设置页三段切换若保留在局部，必须使用 `SegmentedControl`；拆页后不再需要系统设置内部 Tab。

### Task 5: 统一表格、行内操作、更多菜单

**对应报告问题：** 用户管理行内操作过多、审计/调用记录扫描效率低。

**Files:**

- Modify: `web/admin/src/components.tsx`
- Modify: `web/admin/src/ui/dataGrid.ts`
- Modify: `web/admin/src/styles.css`
- Test: `web/admin/src/dataTableActions.contract.ts`

**组件契约：**

```tsx
type DataTableColumn<T> = {
  key: string
  title: string
  width?: string
  render: (row: T) => ReactNode
}

type RowActions<T> = {
  primary: (row: T) => Action
  secondary?: (row: T) => Action
  overflow: (row: T) => Action[]
}

function ResponsiveDataTable<T>({ columns, rows, actions, empty }) {
  return (
    <div className="data-table-wrap">
      <table className="admin-table">
        ...
        {actions && (
          <td>
            <Button variant="secondary">{actions.primary(row).label}</Button>
            {actions.secondary && <Button variant="ghost">{...}</Button>}
            <ActionMenu actions={actions.overflow(row)} />
          </td>
        )}
      </table>
    </div>
  )
}
```

**ActionMenu 契约：**

```tsx
function ActionMenu({ actions }) {
  return (
    <Menu>
      {actions.map(action => (
        <MenuItem
          tone={action.tone}
          onSelect={() => action.confirm ? openConfirm(action) : action.run()}
        />
      ))}
    </Menu>
  )
}
```

**验收：**

- 用户表格行内常驻：`详情` + `更多`。
- `删除` 只出现在更多菜单，且必须确认邮箱。
- `禁用` 只出现在更多菜单或详情页，必须二次确认。

### Task 6: 统一 Modal、Drawer、SidePanel、FormField

**对应报告问题：** 表单/弹窗不统一，长表单塞普通弹窗。

**Files:**

- Modify: `web/admin/src/components.tsx`
- Create: `web/admin/src/ui/formSurfaces.contract.ts`
- Test: `web/admin/src/pages/cashierProviderDialog.contract.ts`

**组件契约：**

```tsx
function Modal({ size = 'md', title, description, children, footer }) {
  const width = { sm: 480, md: 640, lg: 840 }[size]
  return <Dialog width={width} />
}

function Drawer({ title, description, open, onClose, children, footer }) {
  return (
    <aside role="dialog" aria-modal="true" className="drawer drawer-right">
      <header />
      <section className="drawer-body">{children}</section>
      <footer className="drawer-footer">{footer}</footer>
    </aside>
  )
}

function FormField({ label, description, error, children }) {
  return (
    <div className="form-field">
      <label>{label}</label>
      {description && <p>{description}</p>}
      {children}
      {error && <p role="alert">{error}</p>}
    </div>
  )
}
```

**验收：**

- `Field` 不能再把整个控件包成 `<label>`，避免复杂控件、按钮、checkbox 嵌套语义混乱。
- 支付通道配置不再用 `Modal`。
- 存储配置不再用“系统设置里的一块内联编辑”作为唯一入口。

## 3. 信息架构与路由整改

### Task 7: 拆分系统设置为独立一级页面

**对应报告问题：** 系统设置内嵌 Tab 层级深，通用/安全/存储风险等级不同。

**Files:**

- Modify: `web/admin/src/types.ts`
- Modify: `web/admin/src/layout/admin-navigation.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/pages/index.ts`
- Create or Modify:
  - `web/admin/src/pages/GeneralSettingsPage.tsx`
  - `web/admin/src/pages/SecurityConfigPage.tsx`
  - `web/admin/src/pages/StorageConfigPage.tsx`
- Test:
  - `web/admin/src/pages/securityConfig.contract.ts`
  - `web/admin/src/pages/storageConfig.contract.ts`
  - `web/admin/src/adminPermissions.contract.ts`

**路由契约：**

```ts
export type AdminRouteId =
  | 'general-settings'
  | 'security-settings'
  | 'storage-settings'
  // existing routes...

export const navGroups = [
  {
    label: '系统',
    items: [
      { id: 'system-users', label: '系统账户' },
      { id: 'audit', label: '审计日志' },
      { id: 'general-settings', label: '通用配置' },
      { id: 'security-settings', label: '安全配置' },
      { id: 'storage-settings', label: '存储配置' },
    ],
  },
]

const routeAliases = {
  'system-settings': 'general-settings',
  config: 'general-settings',
  'security-config': 'security-settings',
  'storage-config': 'storage-settings',
}
```

**权限契约：**

```ts
ADMIN_ROUTE_PERMISSION_MAP = {
  'general-settings': 'manage:config',
  'security-settings': 'manage:dangerous_config',
  'storage-settings': 'manage:dangerous_config',
}
```

**验收：**

- 左侧导航不再出现 `系统设置` 汇总项。
- 直接访问 `#/system-settings` 自动跳到 `#/general-settings`。
- 没有 `manage:dangerous_config` 的管理员看不到安全配置、存储配置。

### Task 8: 导航分组重组和命名清理

**对应报告问题：** 中英混排、业务归属不清。

**Files:**

- Modify: `web/admin/src/layout/admin-navigation.ts`
- Modify: `web/admin/src/components.tsx`
- Test: `web/admin/src/adminNavigation.contract.ts`

**导航契约：**

```ts
export const navGroups: AdminNavGroup[] = [
  { label: '概览', items: ['dashboard', 'monitoring'] },
  { label: '用户与内容', items: ['users', 'user-groups', 'reviews', 'redeem'] },
  { label: '交易', items: ['orders', 'packages', 'cashier-config'] },
  { label: '模型与生成', items: ['access-accounts', 'routing', 'pricing', 'call-records'] },
  { label: '系统', items: ['system-users', 'audit', 'general-settings', 'security-settings', 'storage-settings'] },
]
```

**命名调整：**

- `运维监控` -> `系统健康`
- `价格配置` -> `价格策略`
- `接入账号` 保留，但页面副标题解释为“上游模型账号与模型列表”。
- 删除导航分组中的 `/ OVERVIEW`、`/ BUSINESS` 等英文。

**验收：**

- 导航分组标题不再含 `/`。
- Topbar 不再显示 Current View/Admin Role 英文标签。

## 4. 页面整改任务

### Task 9: 登录页调整为运维登录入口

**对应报告问题：** 登录页辅助信息像营销卖点。

**Files:**

- Modify: `web/admin/src/pages/LoginPage.tsx`
- Test: `web/admin/src/pages/adminLoginCopy.contract.ts`

**契约：**

```tsx
<LoginPage>
  <BrandBlock title="Mikiko Admin" subtitle="运营控制台" />
  <LoginForm />
  <SystemFacts>
    <Fact label="环境" value={import.meta.env.MODE} />
    <Fact label="API" value="readyz" />
    <Fact label="版本" value={buildVersion} />
  </SystemFacts>
</LoginPage>
```

**验收：**

- 不再出现“模型健康 / 支付配置 / 审核队列 / 审计留痕”作为营销式标签。
- 登录表单保持唯一主 CTA：`进入控制台`。

### Task 10: 运营大盘重排

**对应报告问题：** 首屏焦点混乱、空图表假进度条。

**Files:**

- Modify: `web/admin/src/pages/OverviewPage.tsx`
- Modify: `web/admin/src/pages/overviewRows.ts`
- Test: `web/admin/src/pages/overviewRows.contract.ts`

**页面契约：**

```tsx
<PageHeader title="运营总览" description="今日生成、积分消耗、待处理风险。" />
<KpiGrid max={4}>
  <KpiCard label="总用户数" />
  <KpiCard label="今日生图" />
  <KpiCard label="今日消耗积分" />
  <KpiCard label="待处理" />
</KpiGrid>
<TwoColumn>
  <TrendPanel />
  <TodoPanel />
</TwoColumn>
```

**空图表契约：**

```tsx
function ProviderDistribution({ rows }) {
  if (!rows.length) return <EmptyBlock title="暂无模型调用" detail="配置模型账号并产生调用后展示分布。" />
  return <DistributionChart rows={rows} />
}
```

**验收：**

- `No providers` 不再显示 100% 进度条。
- Provider Health 从大盘首屏移到系统健康页面。

### Task 11: 系统健康页改成阻断项优先

**对应报告问题：** 运维监控缺少任务感。

**Files:**

- Modify: `web/admin/src/pages/MonitoringPage.tsx`
- Modify: `web/admin/src/pages/readinessRows.ts`
- Modify: `web/admin/src/pages/overviewReadinessRows.ts`
- Test: `web/admin/src/pages/readinessRows.contract.ts`

**排序契约：**

```ts
function sortHealthRows(rows: HealthRow[]) {
  const weight = { fail: 0, warn: 1, pass: 2, unknown: 3 }
  return [...rows].sort((a, b) => weight[a.status] - weight[b.status])
}
```

**页面契约：**

```tsx
<PageHeader title="系统健康" primaryAction={<Button>重新检查</Button>} />
<HealthSummary pass warn fail lastCheckedAt />
<HealthIssueList rows={sortHealthRows(rows)} />
<ProviderHealthTable />
```

**验收：**

- 阻断项在首屏靠前。
- 每个阻断项都有明确操作：`去配置`、`重试`、`查看日志`。

### Task 12: 用户管理压缩筛选区和行内操作

**对应报告问题：** 筛选区过重，行内操作过多。

**Files:**

- Modify: `web/admin/src/pages/UsersPage.tsx`
- Modify: `web/admin/src/pages/userRows.ts`
- Test: `web/admin/src/pages/userRows.contract.ts`
- Test: `web/admin/src/pages/userPointAdjustment.contract.ts`

**Toolbar 契约：**

```tsx
<PageToolbar
  search={<SearchInput placeholder="搜索用户名 / 邮箱 / ID" />}
  filters={<>
    <StatusFilter />
    <GroupFilter />
    <SortFilter />
  </>}
  actions={<Button variant="text">清空</Button>}
/>
```

**行操作契约：**

```tsx
const userRowActions = {
  primary: (user) => ({ label: '详情', run: () => openDetail(user) }),
  overflow: (user) => [
    { label: user.disabled ? '启用' : '禁用', confirm: ... },
    { label: '调整分组', run: ... },
    { label: '调整积分', run: ... },
    { label: '设置限额', run: ... },
    { label: '重置密码', run: ... },
    { label: '删除', tone: 'danger', confirm: ... },
  ],
}
```

**验收：**

- 表格行内不再同时显示 7 个操作按钮。
- 删除/禁用必须确认。
- 筛选控件不再放在大圆角卡片中。

### Task 13: 用户分组增加影响范围面板

**对应报告问题：** 分组和模型可见性关系不清晰。

**Files:**

- Modify: `web/admin/src/pages/UserGroupsPage.tsx`
- Modify: `web/admin/src/pages/userGroupRows.ts`
- Test: `web/admin/src/pages/userGroupRows.contract.ts`

**契约：**

```tsx
<ResponsiveDataTable
  columns={[
    '分组',
    '倍率',
    '成员数',
    '可见模型',
    '状态',
    '操作',
  ]}
/>
<SidePanel title="分组影响范围">
  <Metric label="成员数" />
  <Link href="#/routing?group=basic">查看模型可见性</Link>
</SidePanel>
```

**验收：**

- 每个分组展示成员数和模型可见性入口。
- 跳转到路由模型页时带 group query，路由页应读取并筛选。

### Task 14: 调用记录改为排障优先

**对应报告问题：** 统计分布抢占排障主任务。

**Files:**

- Modify: `web/admin/src/pages/CallRecordsPage.tsx`
- Modify: `web/admin/src/pages/callRecordRows.ts`
- Test: `web/admin/src/pages/callRecordRows.contract.ts`

**页面契约：**

```tsx
<PageHeader title="调用记录" />
<PageToolbar>
  <SearchInput placeholder="request_id / task_id / user / error_code" />
  <StatusFilter />
  <ChannelFilter />
</PageToolbar>
<LogTable rows={records} expandable />
<RightDrawer open={selectedRecord}>
  <PayloadPreview />
  <RouteSnapshot />
  <ErrorDetail />
</RightDrawer>
```

**验收：**

- 默认首屏是记录表，不是分布卡片。
- 展开详情显示错误码、上游账号、路由快照、prompt 摘要。

### Task 15: 兑换码页面拆清主/次操作

**对应报告问题：** 批量生成、状态变更、导出混杂。

**Files:**

- Modify: `web/admin/src/pages/RedeemPage.tsx`
- Modify: `web/admin/src/pages/redeemRows.ts`
- Test: `web/admin/src/pages/redeemRows.contract.ts`

**契约：**

```tsx
<PageHeader
  title="兑换码"
  primaryAction={<Button>创建兑换码</Button>}
  secondaryActions={<MoreMenu items={['批量生成', '导出']} />}
/>
<StatusChips value={status} options={['全部','可用','已用完','过期','禁用']} />
```

**验收：**

- 批量生成不与创建兑换码同等视觉权重。
- 少数据/空数据状态显示下一步 CTA。

### Task 16: 审核队列改为三栏处理流

**对应报告问题：** 审核高频处理流不明确。

**Files:**

- Modify: `web/admin/src/pages/ReviewPage.tsx`
- Modify: `web/admin/src/pages/reviewRows.ts`
- Test: `web/admin/src/pages/reviewRows.contract.ts`

**三栏契约：**

```tsx
<ReviewWorkbench>
  <ReviewQueue rows selectedId onSelect />
  <ReviewPreview image prompt metadata />
  <ReviewActions
    approve
    reject
    reasonTemplates
    batchAction={selectedIds.length > 1}
  />
</ReviewWorkbench>
```

**验收：**

- 选择审核项后无需打开新弹窗即可预览和处理。
- 拒绝原因模板使用 Popover/Dropdown，不在页面中展开大块说明。

### Task 17: 订单管理默认表格化

**对应报告问题：** 订单页首屏偏图表，异常处理路径不清。

**Files:**

- Modify: `web/admin/src/pages/CashierPage.tsx`
- Optionally Create: `web/admin/src/pages/OrdersPage.tsx`
- Test: `web/admin/src/pages/cashierOrderRows.contract.ts`

**契约：**

```tsx
<OrdersPage>
  <PageHeader title="订单管理" />
  <PageToolbar>
    <SearchInput placeholder="订单号 / 用户 / 渠道流水号" />
    <QuickFilters options={['待支付','支付失败','待回调','退款中','同步失败']} />
  </PageToolbar>
  <OrderTable />
</OrdersPage>
```

**验收：**

- `#/orders` 默认展示订单表格。
- 图表移入 `概览` 子视图，不作为默认首屏。

### Task 18: 套餐管理从 CashierPage 拆出

**对应报告问题：** 套餐和支付配置共用页面导致心智混乱。

**Files:**

- Create: `web/admin/src/pages/PackagesPage.tsx`
- Create: `web/admin/src/pages/packageRows.ts`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/pages/index.ts`
- Test: `web/admin/src/pages/packageRows.contract.ts`

**契约：**

```tsx
<PackagesPage>
  <PageHeader title="套餐管理" primaryAction={<Button>新增套餐</Button>} />
  <PackageGrid>
    <PackageCard name price points status sortOrder actions />
  </PackageGrid>
  <SidePanel title="编辑套餐" />
</PackagesPage>
```

**验收：**

- `#/packages` 不再渲染 `CashierPage`。
- 套餐编辑强调价格、积分、状态、排序，不显示支付通道配置。

### Task 19: 收银台配置拆分区块，支付通道改 Drawer 分步

**对应报告问题：** 层级深、长弹窗、JSON 不友好。

**Files:**

- Modify: `web/admin/src/pages/CashierPage.tsx`
- Create: `web/admin/src/pages/cashierProviderWizard.ts`
- Test:
  - `web/admin/src/pages/cashierProviderOptions.contract.ts`
  - `web/admin/src/pages/cashierVisibleMethodRows.contract.ts`
  - `web/admin/src/pages/cashierTrialConfig.contract.ts`

**页面契约：**

```tsx
<CashierConfigPage>
  <SegmentedControl value={section} options={[
    '支付方式',
    '渠道实例',
    '风控设置',
    '体验额度',
  ]} />
  {section === '渠道实例' && <ProviderInstanceTable />}
  <ProviderWizardDrawer />
</CashierConfigPage>
```

**Wizard 状态机：**

```ts
type ProviderWizardStep = 'basic' | 'limits' | 'fields' | 'secrets' | 'review'

function nextStep(step: ProviderWizardStep, draft: ProviderDraft): ProviderWizardStep {
  if (step === 'basic') return 'limits'
  if (step === 'limits') return 'fields'
  if (step === 'fields') return hasSecretFields(draft.provider_type) ? 'secrets' : 'review'
  if (step === 'secrets') return 'review'
  return 'review'
}
```

**JSON 契约：**

```tsx
<AdvancedJsonSection defaultOpen={false}>
  <JsonField value={draft.configJson} />
</AdvancedJsonSection>
```

**验收：**

- 添加支付通道不再打开普通 Modal。
- Mock/Alipay/Wechat/JeePay 常用字段结构化展示。
- JSON 区域默认折叠，且标注“高级”。

### Task 20: 路由模型增加配置向导与可用状态

**对应报告问题：** 新增路由后没有下一步，是否可用不清。

**Files:**

- Modify: `web/admin/src/pages/RoutingPage.tsx`
- Modify: `web/admin/src/pages/routingRows.ts`
- Test: `web/admin/src/pages/routingRows.contract.ts`

**可用状态契约：**

```ts
type RouteReadiness = 'ready' | 'missing_candidate' | 'missing_price' | 'disabled'

function routeReadiness(route, candidates, prices): RouteReadiness {
  if (!route.enabled) return 'disabled'
  if (!candidates.some(c => c.enabled)) return 'missing_candidate'
  if (!prices.some(p => String(p.route_model_id) === String(route.id) && p.enabled)) return 'missing_price'
  return 'ready'
}
```

**保存后 CTA：**

```tsx
onRouteCreated(route) {
  openNextStep({
    title: '路由已创建',
    primary: { label: '继续配置候选模型', run: () => openCandidateDrawer(route) },
    secondary: { label: '稍后配置', run: close },
  })
}
```

**验收：**

- 空状态下只显示 `新增路由模型`。
- 新路由创建后出现下一步。
- 列表显示“可用 / 缺候选 / 缺价格 / 已停用”。

### Task 21: 接入账号改主从布局

**对应报告问题：** 账号和底层模型关系不清。

**Files:**

- Modify: `web/admin/src/pages/ProviderModelsPage.tsx`
- Modify: `web/admin/src/pages/providerModelRows.ts`
- Test: `web/admin/src/pages/providerModelRows.contract.ts`

**契约：**

```tsx
<SplitView>
  <AccountList selectedAccountId onSelect />
  <AccountDetail>
    <AccountSummary />
    <AccountActions healthCheck rotateSecret enableDisable />
    <AccountModelTable />
  </AccountDetail>
</SplitView>
```

**验收：**

- 新增模型必须在某个账号上下文中发起。
- 账号健康、启停、密钥轮换在详情操作区。

### Task 22: 价格策略按路由模型分组并显示缺失风险

**对应报告问题：** 依赖路由模型但缺少先后关系提示。

**Files:**

- Modify: `web/admin/src/pages/PricingPage.tsx`
- Modify: `web/admin/src/pages/pricingRows.ts`
- Test: `web/admin/src/pages/pricingRows.contract.ts`

**契约：**

```tsx
<PricingPage>
  <MissingPriceAlert routesWithoutPrice />
  <PriceGroupList>
    <PriceGroup route readiness prices />
  </PriceGroupList>
  <HelpDrawer title="计费规则说明" />
</PricingPage>
```

**验收：**

- 启用路由缺价格时显示 warning 行和 `补齐价格` 按钮。
- 计费规则说明不再和配置表同层争抢主视线。

### Task 23: 审计日志改日志表格密度

**对应报告问题：** 搜索与导出不突出，日志扫描效率低。

**Files:**

- Modify: `web/admin/src/pages/AuditPage.tsx`
- Modify: `web/admin/src/pages/auditRows.ts`
- Test: `web/admin/src/pages/auditRows.contract.ts`

**契约：**

```tsx
<AuditPage>
  <PageToolbar search actionFilter dateRange actions={<Button>导出</Button>} />
  <LogTable columns={['时间','操作者','动作','对象','结果']} />
</AuditPage>
```

**验收：**

- 导出在 toolbar 右侧。
- 每行首列为时间，便于按时间扫描。

### Task 24: 系统账户增强安全信息

**对应报告问题：** 敏感页与普通用户管理区分不足。

**Files:**

- Modify: `web/admin/src/pages/SystemUsersPage.tsx`
- Test: `web/admin/src/pages/systemUsers.contract.ts`

**契约：**

```tsx
<AdminUserTable columns={[
  '管理员',
  '角色',
  '权限摘要',
  '安全状态',
  '最近登录',
  '操作',
]} />
```

**验收：**

- 删除/禁用管理员需要二次确认。
- 页面顶部显示“敏感操作会进入审计日志”的提示。

### Task 25: 通用配置页面收敛低风险配置

**对应报告问题：** Token/Cookie 安全项误放在通用配置。

**Files:**

- Create/Modify: `web/admin/src/pages/GeneralSettingsPage.tsx`
- Modify: `web/admin/src/pages/ConfigPage.tsx`
- Test: `web/admin/src/pages/configRows.contract.ts`

**契约：**

```tsx
const generalCategories = [
  'site',
  'docs',
  'public_gallery',
]

const forbiddenInGeneral = [
  'auth',
  'smtp',
  'moderation',
  'generation_limit',
]
```

**验收：**

- 访问令牌有效期、刷新 cookie、SMTP 不出现在通用配置。
- 页面主操作为右上角 `编辑通用配置`，不是整行大按钮。

### Task 26: 安全配置分组并声明保存模式

**对应报告问题：** 开关像即时操作但保存机制不明，范围过宽。

**Files:**

- Modify: `web/admin/src/pages/SecurityConfigPage.tsx`
- Create: `web/admin/src/pages/securityConfigRows.ts`
- Test: `web/admin/src/pages/securityConfig.contract.ts`

**SwitchRow 契约：**

```tsx
type SaveMode = 'auto' | 'manual'

function SwitchRow({ title, description, checked, saveMode, onChange }) {
  return (
    <section className="switch-row">
      <div>
        <strong>{title}</strong>
        <p>{description}</p>
        <em>{saveMode === 'auto' ? '自动保存' : '修改后需保存'}</em>
      </div>
      <Switch checked={checked} onChange={onChange} />
    </section>
  )
}
```

**分组契约：**

```tsx
<SecurityConfigPage>
  <SettingsSection title="认证与会话" />
  <SettingsSection title="SMTP 邮件" />
  <SettingsSection title="内容安全" />
  <SettingsSection title="生成限制" />
</SecurityConfigPage>
```

**验收：**

- 每个开关旁展示保存模式。
- SMTP 配置不在页面底部被弱化。

### Task 27: 存储配置独立页和 R2/S3 新建向导

**对应报告问题：** 多实例存储管理在系统设置中拥挤，默认名截断，操作层级不清。

**Files:**

- Modify: `web/admin/src/pages/StorageConfigPage.tsx`
- Modify: `web/admin/src/pages/storageConfig.contract.ts`
- Modify: `web/shared/admin-api.ts`
- Test: `web/admin/src/pages/storageConfig.contract.ts`

**布局契约：**

```tsx
<StorageSettingsPage>
  <PageHeader
    title="存储配置"
    primaryAction={<Button>新增存储</Button>}
    secondaryActions={<Button>测试默认存储</Button>}
  />
  <SplitView>
    <StorageConfigList selectedId onSelect />
    <StorageConfigDetail selected />
  </SplitView>
</StorageSettingsPage>
```

**列表字段契约：**

```ts
type StorageListItemView = {
  name: string
  code: string
  driverProvider: string
  endpointOrRoot: string
  isDefault: boolean
  readEnabled: boolean
  writeEnabled: boolean
  lastProbeStatus: 'never' | 'success' | 'failed'
}
```

**新建向导状态机：**

```ts
type StorageWizardStep = 'kind' | 'endpoint' | 'credentials' | 'probe' | 'review'

function nextStorageStep(step: StorageWizardStep, draft: StorageConfigDraft): StorageWizardStep {
  if (step === 'kind') return draft.driver === 'local' ? 'review' : 'endpoint'
  if (step === 'endpoint') return 'credentials'
  if (step === 'credentials') return 'probe'
  if (step === 'probe') return draft.lastProbeStatus === 'success' ? 'review' : 'probe'
  return 'review'
}

function canSetDefault(config: StorageConfigView) {
  return config.status === 'enabled'
    && config.read_enabled
    && config.write_enabled
    && config.last_probe.status === 'success'
}
```

**操作层级契约：**

- `测试连接`：secondary。
- `设为默认写入`：primary only when `canSetDefault(config)`。
- `设为只读`：danger-adjacent，需要确认，因为会影响写入。
- `保存`：只在编辑态固定到底部保存条。

**验收：**

- 默认存储名完整显示，不得截断到无法辨认。
- 新增 R2/S3 必须经过 Probe 成功后才能设为默认。
- 多个配置下列表和详情不拥挤。

## 5. 截图回归与验收

### Task 28: 建立管理端截图回归脚本

**对应报告问题：** 页面风格容易再次分叉。

**Files:**

- Create: `scripts/visual/admin-ui-snapshot.mjs`
- Create: `docs/audits/admin-ui-ux-2026-07-07/acceptance-checklist.md`
- Modify: `package.json` only if repository already has visual script convention; otherwise不要加全局脚本。

**截图清单：**

```ts
const desktopRoutes = [
  'dashboard',
  'monitoring',
  'users',
  'user-groups',
  'reviews',
  'orders',
  'packages',
  'cashier-config',
  'access-accounts',
  'routing',
  'pricing',
  'call-records',
  'audit',
  'system-users',
  'general-settings',
  'security-settings',
  'storage-settings',
]

const mobileRoutes = ['dashboard', 'users', 'storage-settings']
```

**视觉断言：**

```ts
assert(document.documentElement.scrollWidth <= window.innerWidth + 4)
assert(!document.body.innerText.includes('CURRENT VIEW'))
assert(!document.body.innerText.includes('ADMIN ROLE'))
assert(countVisiblePrimaryButtons() <= 1)
```

**验收：**

- 脚本生成 desktop + mobile 截图。
- 任一页面横向溢出、重复状态条、多个 primary 按钮时失败。

### Task 29: 最终验证

**Commands:**

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
npm --prefix web/user run typecheck
npm --prefix web/user run build
./scripts/workflow/verify.sh
```

如果只改 admin 且耗时敏感，可以先跑：

```bash
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
./scripts/workflow/verify-contracts.sh
```

交付前必须跑完整：

```bash
./scripts/workflow/verify.sh
```

## 6. 问题到任务映射

| 报告问题 | 修复任务 |
| --- | --- |
| 移动端和窄屏布局不可用 | Task 2, Task 28 |
| 页面骨架缺少统一规范 | Task 2, Task 3 |
| 组件语义混乱，按钮层级不稳定 | Task 4, Task 5 |
| 按钮层级不稳定 | Task 4, Task 5 |
| 卡片过多、信息密度下降 | Task 1, Task 3 |
| 表单和弹窗不统一 | Task 6 |
| 命名和语言混用造成认知负担 | Task 8 |
| 中英混排和命名负担 | Task 8 |
| 系统设置层级深 | Task 7, Task 25, Task 26, Task 27 |
| 登录页辅助信息像营销 | Task 9 |
| 大盘空图表误导 | Task 10 |
| 运维监控缺少任务感 | Task 11 |
| 用户管理行内操作过多 | Task 12 |
| 用户分组影响范围不清 | Task 13 |
| 调用记录排障效率低 | Task 14 |
| 兑换码操作混杂 | Task 15 |
| 审核队列处理流不清 | Task 16 |
| 订单页默认图表化 | Task 17 |
| 套餐管理复用 CashierPage | Task 18 |
| 收银台配置层级深、长弹窗 | Task 19 |
| 路由模型缺后续配置引导 | Task 20 |
| 接入账号与模型关系不清 | Task 21 |
| 价格配置缺失风险不明显 | Task 22 |
| 审计日志扫描效率低 | Task 23 |
| 系统账户敏感性不足 | Task 24 |
| 存储配置多实例管理拥挤 | Task 27 |
| 系统设置：安全策略范围过宽、保存机制不明 | Task 7, Task 26 |
| 新增支付通道长弹窗和 JSON 暴露 | Task 6, Task 19 |
| 风格容易再次分叉 | Task 28 |

## 7. 实施顺序

推荐按以下顺序执行，避免大范围冲突：

1. Task 1-6：通用骨架和组件，不改业务逻辑。
2. Task 7-8：路由和导航重组。
3. Task 25-27：系统设置拆页，优先解决最新关注点。
4. Task 12、19、20、21、22：高复杂操作页。
5. Task 9-11、13-18、23-24：其余页面统一。
6. Task 28-29：截图回归和最终验证。

每完成一个阶段都应生成截图，对照 `docs/audits/admin-ui-ux-2026-07-07/screenshots/` 检查问题是否消失。

## 8. 自审记录

### Review 1

检查项：

- 是否逐项覆盖报告中的 P0/P1/P2 和页面级问题：通过，见“问题到任务映射”。
- 是否存在“优化一下”“调整一下”这种不可执行措辞：发现初稿中“视觉精修”过泛，已改为 Task 1 的 token/radius/shadow 契约和 Task 28 的截图断言。
- 是否给通用组件写了伪代码契约：通过，覆盖 Layout、Button、DataTable、ActionMenu、Modal/Drawer、SwitchRow、StorageWizard。
- 是否能实际解决系统设置层级深问题：通过，方案要求拆成 `general-settings/security-settings/storage-settings` 三个一级路由，并保留 alias。
- 是否能实际解决移动端问题：通过，方案要求桌面/移动 Shell 分离，且截图脚本断言横向溢出。
- 是否有验证方式：通过，包含 contract、typecheck、build、verify、截图回归。

结论：Review 1 发现的泛化表述已修正，无阻塞问题。

### Review 2

检查项：

- 路由拆分是否可能破坏历史链接：已通过 `routeAliases` 保留 `system-settings/config/security-config/storage-config`。
- 权限是否会泄露危险配置：已明确 `security-settings/storage-settings` 使用 `manage:dangerous_config`。
- 存储配置是否符合多 S3/R2 目标：已定义列表/详情/向导/Probe/Set Default 契约，覆盖默认写入和历史读取识别。
- 支付通道长表单是否仍可能塞 Modal：已由 `resolveEditSurface` 和 Task 19 强制 Drawer 分步。
- 表格行内操作是否明确收敛：已定义 `RowActionModel` 和用户页验收。

结论：Review 2 未发现新的方案偏移。

### Review 3

检查项：

- 是否过度重构导致无法分阶段落地：实施顺序已将通用骨架、路由、系统设置、高复杂页面拆分，允许阶段验收。
- 是否需要后端变更：默认不需要；如个别页面缺少字段，计划要求另开后端设计，不在本方案混做。
- 是否符合仓库工作流：已声明正式编码前使用 `dev-start-coding` 和 `dev-react-patterns`，并给出验证命令。

结论：Review 3 未发现问题。方案可进入实施。
