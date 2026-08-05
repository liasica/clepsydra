import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'AuditLogList',
    path: '/audit-logs',
    component: () => import('#/views/audit-logs/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:file-list-3-line',
      order: 50,
      title: '审计日志',
    },
  },
];

export default routes;
