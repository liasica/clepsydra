import { AppRouteRecord } from '@/types/router'

// 一级菜单：无 children，交由框架自动包裹 Layout
export const auditLogsRoutes: AppRouteRecord = {
  name: 'AuditLogList',
  path: '/audit-logs',
  component: '/auditlogs/index',
  meta: { title: '审计日志', icon: 'ri:file-list-3-line', roles: ['admin'] }
}
