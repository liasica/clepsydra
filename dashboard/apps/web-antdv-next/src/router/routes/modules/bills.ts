import type { RouteRecordRaw } from 'vue-router';

// 机制同 demands.ts：一级菜单 + 独立详情路由，详情页 hideInMenu 挂成顶层隐藏项
const routes: RouteRecordRaw[] = [
  {
    name: 'BillList',
    path: '/bills',
    component: () => import('#/views/bills/index.vue'),
    meta: {
      authority: ['admin', 'client'],
      icon: 'ri:bill-line',
      order: 20,
      title: '账单管理',
    },
  },
  {
    name: 'BillDetail',
    path: '/bills/:id',
    component: () => import('#/views/bills/detail.vue'),
    meta: {
      activePath: '/bills',
      authority: ['admin', 'client'],
      hideInMenu: true,
      title: '账单详情',
    },
  },
];

export default routes;
