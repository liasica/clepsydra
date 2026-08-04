import request from '@/utils/http'

/** 查询工作台待办汇总 */
export function fetchTodos() {
  return request.get<Api.Dashboard.Todos>({ url: '/api/dashboard/todos' })
}
