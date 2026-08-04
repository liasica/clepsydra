import request from '@/utils/http'

/** 分页查询审计日志 */
export function fetchAuditLogs(query: Api.AuditLog.Query) {
  return request.get<Api.AuditLog.ListData>({ url: '/api/audit-logs', params: query })
}
