import React from 'react'
import { cn } from '../../shared/classnames'
import { rdAdmin } from './admin-classes'

interface NavItemProps {
  icon: React.ReactNode
  label: string
  active?: boolean
  badge?: string
  onClick?: () => void
}

const NavItem: React.FC<NavItemProps> = ({ icon, label, active, badge, onClick }) => (
  <button
    onClick={onClick}
    className={cn(rdAdmin.navLink, active && rdAdmin.navLinkActive)}
  >
    <span className={rdAdmin.navIcon}>{icon}</span>
    <span>{label}</span>
    {badge && <span className={rdAdmin.navBadge}>{badge}</span>}
  </button>
)

interface AdminLayoutProps {
  children: React.ReactNode
  title: string
  currentPath: string
  onNavigate: (path: string) => void
}

export const AdminLayout: React.FC<AdminLayoutProps> = ({ children, title, currentPath, onNavigate }) => {
  const [theme, setTheme] = React.useState<'dark' | 'light'>(() => {
    if (typeof window === 'undefined') return 'dark'
    return window.localStorage.getItem('pic_gallery_admin_demo_theme') === 'light' ? 'light' : 'dark'
  })

  React.useEffect(() => {
    window.localStorage.setItem('pic_gallery_admin_demo_theme', theme)
  }, [theme])

  const nextTheme = theme === 'dark' ? 'light' : 'dark'

  return (
    <div className={cn(rdAdmin.layout, theme === 'light' && 'theme-light')} data-theme={theme}>
      {/* Sidebar */}
      <aside className={rdAdmin.sidebar}>
        <div className={rdAdmin.brand}>
          <div className={rdAdmin.brandOrb}>M</div>
          <span className={rdAdmin.brandText}>Mikiko Admin</span>
        </div>

        <nav className={rdAdmin.nav}>
          <div className={rdAdmin.navSection}>
            <h3 className={rdAdmin.navSectionTitle}>概览 / Overview</h3>
            <NavItem 
              icon={<ChartIcon />} 
              label="运营大盘" 
              active={currentPath === 'dashboard'} 
              onClick={() => onNavigate('dashboard')}
            />
            <NavItem 
              icon={<ActivityIcon />} 
              label="运维监控" 
              active={currentPath === 'monitoring'} 
              onClick={() => onNavigate('monitoring')}
            />
          </div>

          <div className={rdAdmin.navSection}>
            <h3 className={rdAdmin.navSectionTitle}>业务管理 / Business</h3>
            <NavItem 
              icon={<UsersIcon />} 
              label="用户管理" 
              active={currentPath === 'users'} 
              onClick={() => onNavigate('users')}
            />
            <NavItem
              icon={<GroupIcon />}
              label="用户分组"
              active={currentPath === 'groups'}
              onClick={() => onNavigate('groups')}
            />
            <NavItem
              icon={<ListIcon />}
              label="调用记录"
              active={currentPath === 'records'}
              onClick={() => onNavigate('records')}
            />
            <NavItem 
              icon={<TicketIcon />} 
              label="兑换码" 
              active={currentPath === 'coupons'} 
              onClick={() => onNavigate('coupons')}
            />
            <NavItem 
              icon={<ShieldIcon />} 
              label="审核队列" 
              active={currentPath === 'audit'} 
              badge="12"
              onClick={() => onNavigate('audit')}
            />
          </div>

          <div className={rdAdmin.navSection}>
            <h3 className={rdAdmin.navSectionTitle}>商业化 / Commercial</h3>
            <NavItem 
              icon={<CreditCardIcon />} 
              label="订单管理" 
              active={currentPath === 'orders'} 
              onClick={() => onNavigate('orders')}
            />
            <NavItem 
              icon={<BoxIcon />} 
              label="套餐管理" 
              active={currentPath === 'packages'} 
              onClick={() => onNavigate('packages')}
            />
            <NavItem 
              icon={<LayoutIcon />} 
              label="收银台配置" 
              active={currentPath === 'cashier'} 
              onClick={() => onNavigate('cashier')}
            />
          </div>

          <div className={rdAdmin.navSection}>
            <h3 className={rdAdmin.navSectionTitle}>路由与模型 / Models</h3>
            <NavItem 
              icon={<ZapIcon />} 
              label="路由模型" 
              active={currentPath === 'route-models'} 
              onClick={() => onNavigate('route-models')}
            />
            <NavItem 
              icon={<CloudIcon />} 
              label="接入账号" 
              active={currentPath === 'access-accounts'} 
              onClick={() => onNavigate('access-accounts')}
            />
            <NavItem 
              icon={<CoinsIcon />} 
              label="价格配置" 
              active={currentPath === 'price-config'} 
              onClick={() => onNavigate('price-config')}
            />
          </div>

          <div className={rdAdmin.navSection}>
            <h3 className={rdAdmin.navSectionTitle}>系统 / System</h3>
            <NavItem 
              icon={<FileTextIcon />} 
              label="审计日志" 
              active={currentPath === 'logs'} 
              onClick={() => onNavigate('logs')}
            />
            <NavItem 
              icon={<SystemUserIcon />} 
              label="系统账户" 
              active={currentPath === 'system-users'} 
              onClick={() => onNavigate('system-users')}
            />
            <NavItem 
              icon={<SettingsIcon />} 
              label="系统设置" 
              active={currentPath === 'settings'} 
              onClick={() => onNavigate('settings')}
            />
          </div>
        </nav>

        <div className="p-6 border-t border-white/5">
          <div className="flex items-center gap-3">
            <div className="size-10 rounded-xl bg-white/5 flex items-center justify-center font-bold text-white/40">
              AD
            </div>
            <div className="flex flex-col">
              <span className="text-sm font-bold text-white">Admin</span>
              <span className="text-[10px] text-white/30">Super Administrator</span>
            </div>
          </div>
        </div>
      </aside>

      {/* Main Content */}
      <main className={rdAdmin.main}>
        <header className={rdAdmin.topbar}>
          <div className="flex items-center gap-4">
            <h1 className={rdAdmin.pageTitle}>{title}</h1>
            <div className="h-4 w-px bg-white/10" />
            <div className="flex items-center gap-2 text-[10px] font-bold text-white/30 uppercase tracking-widest">
              <span>Admin</span>
              <ChevronRightIcon className="size-3" />
              <span className="text-[var(--accent)]">{title}</span>
            </div>
          </div>

          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/5 border border-white/5">
              <div className="size-2 rounded-full bg-emerald-500 animate-pulse" />
              <span className="text-[10px] font-bold text-white/50 uppercase tracking-wider">System Online</span>
            </div>
            <button
              type="button"
              aria-label={`切换到${nextTheme === 'light' ? '亮色' : '暗色'}模式`}
              title={`切换到${nextTheme === 'light' ? '亮色' : '暗色'}模式`}
              onClick={() => setTheme(nextTheme)}
              className="size-10 rounded-xl bg-white/5 border border-white/5 flex items-center justify-center text-white/50 hover:text-white transition-colors"
            >
              {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
            </button>
            <button className="size-10 rounded-xl bg-white/5 border border-white/5 flex items-center justify-center text-white/50 hover:text-white transition-colors">
              <BellIcon />
            </button>
          </div>
        </header>

        <div className={rdAdmin.content}>
          {children}
        </div>
      </main>
    </div>
  )
}

// Icons
const ChartIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/></svg>
const ActivityIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
const UsersIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
const TicketIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M2 9a3 3 0 0 1 0 6v2a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-2a3 3 0 0 1 0-6V7a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z"/><path d="M13 5v2"/><path d="M13 17v2"/><path d="M13 11v2"/></svg>
const ShieldIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
const CreditCardIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="20" height="14" x="2" y="5" rx="2"/><line x1="2" x2="22" y1="10" y2="10"/></svg>
const BoxIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/></svg>
const LayoutIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><line x1="3" x2="21" y1="9" y2="9"/><line x1="9" x2="9" y1="21" y2="9"/></svg>
const ZapIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
const CloudIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17.5 19c.1 0 .2 0 .3 0A5.5 5.5 0 0 0 16 8.1l-1.3-.1A7.5 7.5 0 0 0 2 12a7.5 7.5 0 0 0 12.3 5.8l1.2 1.2M17.5 19h.3"/><path d="M12 12l2 2 4-4"/></svg>
const CoinsIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6"/><path d="M18.09 10.37A6 6 0 1 1 10.34 18"/><path d="M7 6h1v4"/><path d="m16.71 13.88.7.71-2.82 2.82"/></svg>
const FileTextIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="16" x2="8" y1="13" y2="13"/><line x1="16" x2="8" y1="17" y2="17"/><line x1="10" x2="8" y1="9" y2="9"/></svg>
const SettingsIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.1a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
const BellIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>
const SunIcon = () => <svg viewBox="0 0 24 24" fill="none" className="size-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>
const MoonIcon = () => <svg viewBox="0 0 24 24" fill="none" className="size-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>
const ListIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="8" x2="21" y1="6" y2="6"/><line x1="8" x2="21" y1="12" y2="12"/><line x1="8" x2="21" y1="18" y2="18"/><line x1="3" x2="3.01" y1="6" y2="6"/><line x1="3" x2="3.01" y1="12" y2="12"/><line x1="3" x2="3.01" y1="18" y2="18"/></svg>
const GroupIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
const SystemUserIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><circle cx="12" cy="11" r="3"/></svg>
const ChevronRightIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m9 18 6-6-6-6"/></svg>
