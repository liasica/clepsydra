import type { RouteRecordRaw } from 'vue-router';

// 列表与详情均为一级路由（无 children），详情页用 hideInMenu 挂成顶层隐藏项，
// 避免侧栏出现单子项父菜单；activePath 让详情页高亮回「需求管理」这一级菜单
const routes: RouteRecordRaw[] = [
  {
    name: 'DemandList',
    path: '/demands',
    component: () => import('#/views/demands/index.vue'),
    meta: {
      authority: ['admin', 'client'],
      icon: 'ri:task-line',
      order: 10,
      title: '需求管理',
    },
  },
  {
    name: 'DemandDetail',
    path: '/demands/:id',
    component: () => import('#/views/demands/detail.vue'),
    meta: {
      activePath: '/demands',
      authority: ['admin', 'client'],
      hideInMenu: true,
      title: '需求详情',
    },
  },
];

export default routes;
