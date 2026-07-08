import { createElement } from 'react'
import { AdminTabs } from './components'

type SettingsTab = 'general' | 'security' | 'storage'

const items: Array<{ id: SettingsTab; label: string; description?: string; disabled?: boolean }> = [
  { id: 'general', label: '通用配置' },
  { id: 'security', label: '安全配置', disabled: true },
  { id: 'storage', label: '存储配置', description: 'S3 / R2 多实例' },
]

createElement(AdminTabs<SettingsTab>, {
  ariaLabel: '系统设置分区',
  orientation: 'horizontal',
  items,
  value: 'general',
  onChange: (value) => {
    if (!items.some((item) => item.id === value)) throw new Error('tab value must come from configured items')
  },
})

createElement(AdminTabs<SettingsTab>, {
  ariaLabel: '通用配置类目',
  orientation: 'vertical',
  items,
  value: 'storage',
  onChange: (value) => {
    if (value === 'security') throw new Error('disabled tab should not be selected by callers')
  },
})
