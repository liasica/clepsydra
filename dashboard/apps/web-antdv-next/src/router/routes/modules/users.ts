import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'UserList',
    path: '/users',
    component: () => import('#/views/users/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:user-settings-line',
      order: 40,
      title: '用户管理',
    },
  },
];

export default routes;
