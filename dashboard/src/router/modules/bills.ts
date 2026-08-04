import { AppRouteRecord } from '@/types/router'

// 一级菜单 + 独立详情路由，机制同 demands.ts
export const billsRoutes: AppRouteRecord[] = [
  {
    name: 'BillList',
    path: '/bills',
    component: '/bills/index',
    meta: { title: '账单管理', icon: 'ri:bill-line', roles: ['admin', 'client'] }
  },
  {
    name: 'BillDetail',
    path: '/bills/:id',
    component: '/bills/detail',
    meta: { title: '账单详情', isHide: true, roles: ['admin', 'client'] }
  }
]
