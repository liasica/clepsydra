import { AppRouteRecord } from '@/types/router'

export const demandsRoutes: AppRouteRecord = {
  name: 'Demands',
  path: '/demands',
  component: '/index/index',
  redirect: '/demands/list',
  meta: {
    title: '需求管理',
    icon: 'ri:task-line',
    roles: ['admin', 'client']
  },
  children: [
    {
      path: 'list',
      name: 'DemandList',
      component: '/demands/index',
      meta: { title: '需求管理', icon: 'ri:task-line' }
    },
    {
      path: ':id',
      name: 'DemandDetail',
      component: '/demands/detail',
      meta: { title: '需求详情', isHide: true }
    }
  ]
}
