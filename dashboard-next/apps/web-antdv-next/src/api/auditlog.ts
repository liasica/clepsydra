import { requestClient } from '#/api/request';

/** 分页查询审计日志（仅超级管理员） */
export function fetchAuditLogs(query: Api.AuditLog.Query) {
  return requestClient.get<Api.AuditLog.ListData>('/api/audit-logs', {
    params: query,
  });
}
