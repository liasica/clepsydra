import { AppRouteRecord } from '@/types/router'

// 一级菜单 + 独立详情路由：两项均无 children，各自触发框架的一级路由自动 Layout
// 包裹机制；详情路由 isHide 后不会出现在菜单，但 path 前缀匹配仍保证权限与
// 访问路径 /demands/:id 可达
export const demandsRoutes: AppRouteRecord[] = [
  {
    name: 'DemandList',
    path: '/demands',
    component: '/demands/index',
    meta: { title: '需求管理', icon: 'ri:task-line', roles: ['admin', 'client'] }
  },
  {
    name: 'DemandDetail',
    path: '/demands/:id',
    component: '/demands/detail',
    meta: { title: '需求详情', isHide: true, roles: ['admin', 'client'] }
  }
]
