import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'SettingCenter',
    path: '/settings',
    component: () => import('#/views/settings/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:settings-3-line',
      order: 30,
      title: '设置中心',
    },
  },
];

export default routes;
