import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'TagList',
    path: '/tags',
    component: () => import('#/views/tags/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:price-tag-3-line',
      order: 16,
      title: '标签管理',
    },
  },
];

export default routes;
