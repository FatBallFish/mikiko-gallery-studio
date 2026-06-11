import React, { useState } from 'react'
import { AdminLayout } from './AdminLayout'
import { Dashboard } from './pages/Dashboard'
import { Monitoring } from './pages/Monitoring'
import { UserManagement } from './pages/UserManagement'
import { GroupManagement } from './pages/GroupManagement'
import { CallRecords } from './pages/CallRecords'
import { CouponManagement } from './pages/CouponManagement'
import { AuditQueue } from './pages/AuditQueue'
import { OrderManagement } from './pages/OrderManagement'
import { PackageManagement } from './pages/PackageManagement'
import { CashierManagement } from './pages/CashierManagement'
import { RouteModelPage } from './pages/RouteModelPage'
import { AccessAccountPage } from './pages/AccessAccountPage'
import { PriceConfigPage } from './pages/PriceConfigPage'
import { AuditLog } from './pages/AuditLog'
import { SystemUsers } from './pages/SystemUsers'
import { SystemSettings } from './pages/SystemSettings'

export const AdminDemo: React.FC = () => {
  const [currentPath, setCurrentPath] = useState('dashboard')

  const renderContent = () => {
    switch (currentPath) {
      case 'dashboard':
        return <Dashboard />
      case 'monitoring':
        return <Monitoring />
      case 'users':
        return <UserManagement />
      case 'groups':
        return <GroupManagement />
      case 'records':
        return <CallRecords />
      case 'coupons':
        return <CouponManagement />
      case 'audit':
        return <AuditQueue />
      case 'orders':
        return <OrderManagement />
      case 'packages':
        return <PackageManagement />
      case 'cashier':
        return <CashierManagement />
      case 'route-models':
        return <RouteModelPage />
      case 'access-accounts':
        return <AccessAccountPage />
      case 'price-config':
        return <PriceConfigPage />
      case 'logs':
        return <AuditLog />
      case 'system-users':
        return <SystemUsers />
      case 'settings':
        return <SystemSettings />
      default:
        return (
          <div className="flex flex-col items-center justify-center h-[60vh] text-white/20">
            <span className="text-4xl font-black uppercase tracking-[0.2em] mb-4">Under Construction</span>
            <span className="text-sm">Page: {currentPath} is being designed...</span>
          </div>
        )
    }
  }

  const getTitle = () => {
    switch (currentPath) {
      case 'dashboard': return '运营大盘'
      case 'monitoring': return '运维监控'
      case 'users': return '用户管理'
      case 'groups': return '用户分组'
      case 'records': return '调用记录'
      case 'coupons': return '兑换码'
      case 'audit': return '审核队列'
      case 'orders': return '订单管理'
      case 'packages': return '套餐管理'
      case 'cashier': return '收银台配置'
      case 'route-models': return '路由模型'
      case 'access-accounts': return '接入账号'
      case 'price-config': return '价格配置'
      case 'logs': return '审计日志'
      case 'system-users': return '系统账户'
      case 'settings': return '系统设置'
      default: return '管理后台'
    }
  }

  return (
    <AdminLayout 
      title={getTitle()} 
      currentPath={currentPath} 
      onNavigate={setCurrentPath}
    >
      {renderContent()}
    </AdminLayout>
  )
}
