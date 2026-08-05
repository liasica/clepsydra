import type { RouteRecordRaw } from 'vue-router';

// 一级菜单：不设 children，直接作为 root 的子路由，避免侧栏出现「只有一个
// 子项还要点开」的父菜单（旧前端 commit 2dcc1ff 的扁平化改造在 vben 下的等价做法）
const routes: RouteRecordRaw[] = [
  {
    name: 'Dashboard',
    path: '/dashboard',
    component: () => import('#/views/dashboard/index.vue'),
    meta: {
      authority: ['admin', 'client'],
      affixTab: true,
      icon: 'ri:home-smile-2-line',
      order: 0,
      title: '工作台',
    },
  },
];

export default routes;
