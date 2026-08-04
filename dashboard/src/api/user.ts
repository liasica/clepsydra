import request from '@/utils/http'

/** 查询用户列表 */
export function fetchUsers() {
  return request.get<Api.User.Item[]>({ url: '/api/users' })
}

/** 创建用户 */
export function createUser(params: Api.User.CreateParams) {
  return request.post<Api.User.Item>({ url: '/api/users', params })
}

/** 更新用户姓名或启用状态 */
export function updateUser(id: number, params: Api.User.UpdateParams) {
  return request.put<Api.User.Item>({ url: `/api/users/${id}`, params })
}

/** 重置用户密码 */
export function resetPassword(id: number, password: string) {
  return request.put<void>({ url: `/api/users/${id}/password`, params: { password } })
}
