import { AppRouteRecord } from '@/types/router'

export const billsRoutes: AppRouteRecord = {
  name: 'Bills',
  path: '/bills',
  component: '/index/index',
  redirect: '/bills/list',
  meta: {
    title: '账单管理',
    icon: 'ri:bill-line',
    roles: ['admin', 'client']
  },
  children: [
    {
      path: 'list',
      name: 'BillList',
      component: '/bills/index',
      meta: { title: '账单管理', icon: 'ri:bill-line' }
    },
    {
      path: ':id',
      name: 'BillDetail',
      component: '/bills/detail',
      meta: { title: '账单详情', isHide: true }
    }
  ]
}
