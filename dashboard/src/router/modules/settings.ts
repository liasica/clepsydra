import { AppRouteRecord } from '@/types/router'

// 一级菜单：无 children，交由框架自动包裹 Layout
export const settingsRoutes: AppRouteRecord = {
  name: 'SettingCenter',
  path: '/settings',
  component: '/settings/index',
  meta: { title: '设置中心', icon: 'ri:settings-3-line', roles: ['admin'] }
}
