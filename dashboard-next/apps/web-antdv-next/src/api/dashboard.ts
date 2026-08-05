import { requestClient } from '#/api/request';

/** 查询工作台待办汇总 */
export function fetchTodos() {
  return requestClient.get<Api.Dashboard.Todos>('/api/dashboard/todos');
}
