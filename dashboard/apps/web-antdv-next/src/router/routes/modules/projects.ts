import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'ProjectList',
    path: '/projects',
    component: () => import('#/views/projects/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:folder-2-line',
      order: 15,
      title: '项目管理',
    },
  },
];

export default routes;
