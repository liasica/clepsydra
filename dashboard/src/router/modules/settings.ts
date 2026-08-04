import { AppRouteRecord } from '@/types/router'

export const settingsRoutes: AppRouteRecord = {
  name: 'Settings',
  path: '/settings',
  component: '/index/index',
  redirect: '/settings/center',
  meta: {
    title: '设置中心',
    icon: 'ri:settings-3-line',
    roles: ['admin']
  },
  children: [
    {
      path: 'center',
      name: 'SettingCenter',
      component: '/settings/index',
      meta: { title: '设置中心', icon: 'ri:settings-3-line' }
    }
  ]
}
