import { AppRouteRecord } from '@/types/router'

// 一级菜单：无 children，交由框架自动包裹 Layout，避免侧栏出现可展开子级
export const dashboardRoutes: AppRouteRecord = {
  name: 'Console',
  path: '/dashboard',
  component: '/dashboard/console',
  meta: {
    title: '工作台',
    icon: 'ri:home-smile-2-line',
    roles: ['admin', 'client'],
    keepAlive: false,
    fixedTab: true
  }
}
