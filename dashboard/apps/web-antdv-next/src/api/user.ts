import { requestClient } from '#/api/request';

/** 查询用户列表（仅超级管理员） */
export function fetchUsers() {
  return requestClient.get<Api.User.Item[]>('/api/users');
}

/** 创建用户（仅超级管理员） */
export function createUser(params: Api.User.CreateParams) {
  return requestClient.post<Api.User.Item>('/api/users', params);
}

/** 更新用户姓名或启用状态（仅超级管理员） */
export function updateUser(id: number, params: Api.User.UpdateParams) {
  return requestClient.put<Api.User.Item>(`/api/users/${id}`, params);
}

/** 重置用户密码（仅超级管理员） */
export function resetPassword(id: number, password: string): Promise<void> {
  return requestClient.put(`/api/users/${id}/password`, { password });
}
