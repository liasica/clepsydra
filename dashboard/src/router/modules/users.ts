import { AppRouteRecord } from '@/types/router'

// 一级菜单：无 children，交由框架自动包裹 Layout
export const usersRoutes: AppRouteRecord = {
  name: 'UserList',
  path: '/users',
  component: '/users/index',
  meta: { title: '用户管理', icon: 'ri:user-settings-line', roles: ['admin'] }
}
