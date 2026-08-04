import request from '@/utils/http'

/** 登录 */
export function fetchLogin(params: Api.Auth.LoginParams) {
  return request.post<Api.Auth.LoginData>({
    url: '/api/auth/login',
    params
  })
}

/** 查询当前登录用户，供页面刷新后的会话恢复 */
export function fetchMe() {
  return request.get<Api.Auth.SimpleUser>({
    url: '/api/auth/me'
  })
}
