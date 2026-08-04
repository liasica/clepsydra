import { AppRouteRecord } from '@/types/router'

export const usersRoutes: AppRouteRecord = {
  name: 'Users',
  path: '/users',
  component: '/index/index',
  redirect: '/users/list',
  meta: {
    title: '用户管理',
    icon: 'ri:user-settings-line',
    roles: ['admin']
  },
  children: [
    {
      path: 'list',
      name: 'UserList',
      component: '/users/index',
      meta: { title: '用户管理', icon: 'ri:user-settings-line' }
    }
  ]
}
