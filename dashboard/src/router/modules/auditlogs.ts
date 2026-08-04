import { AppRouteRecord } from '@/types/router'

export const auditLogsRoutes: AppRouteRecord = {
  name: 'AuditLogs',
  path: '/audit-logs',
  component: '/index/index',
  redirect: '/audit-logs/list',
  meta: {
    title: '审计日志',
    icon: 'ri:file-list-3-line',
    roles: ['admin']
  },
  children: [
    {
      path: 'list',
      name: 'AuditLogList',
      component: '/auditlogs/index',
      meta: { title: '审计日志', icon: 'ri:file-list-3-line' }
    }
  ]
}
