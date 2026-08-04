import request from '@/utils/http'
import { AppRouteRecord } from '@/types/router'

// 获取菜单列表（后端控制模式使用）
export function fetchGetMenuList() {
  return request.get<AppRouteRecord[]>({
    url: '/api/v3/system/menus'
  })
}
