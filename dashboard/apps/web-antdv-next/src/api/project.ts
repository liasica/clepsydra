import { requestClient } from '#/api/request';

/** 查询项目列表，登录即可，供管理页、下拉与筛选使用 */
export function fetchProjects() {
  return requestClient.get<Api.Project.Item[]>('/api/projects');
}

/** 创建项目（仅超级管理员），名称唯一 */
export function createProject(params: Api.Project.SaveParams) {
  return requestClient.post<Api.Project.Item>('/api/projects', params);
}

/** 更新项目（仅超级管理员），全量覆盖名称、颜色与备注 */
export function updateProject(id: number, params: Api.Project.SaveParams) {
  return requestClient.put<Api.Project.Item>(`/api/projects/${id}`, params);
}

/** 删除项目（仅超级管理员），自动解除与需求的关联，需求本身不受影响 */
export function deleteProject(id: number): Promise<void> {
  return requestClient.delete(`/api/projects/${id}`);
}
