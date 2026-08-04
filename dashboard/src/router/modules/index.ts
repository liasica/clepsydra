import { AppRouteRecord } from '@/types/router'
import { dashboardRoutes } from './dashboard'
import { demandsRoutes } from './demands'
import { billsRoutes } from './bills'
import { settingsRoutes } from './settings'
import { usersRoutes } from './users'
import { auditLogsRoutes } from './auditlogs'

/**
 * 导出所有模块化路由
 */
export const routeModules: AppRouteRecord[] = [
  dashboardRoutes,
  demandsRoutes,
  billsRoutes,
  settingsRoutes,
  usersRoutes,
  auditLogsRoutes
]
