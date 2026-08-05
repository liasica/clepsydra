import { requestClient } from '#/api/request';

/** 登录 */
export function fetchLogin(params: Api.Auth.LoginParams) {
  return requestClient.post<Api.Auth.LoginData>('/api/auth/login', params);
}

/** 查询当前登录用户，供页面刷新后的会话恢复 */
export function fetchMe() {
  return requestClient.get<Api.Auth.SimpleUser>('/api/auth/me');
}
