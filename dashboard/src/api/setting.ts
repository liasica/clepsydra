import request from '@/utils/http'

/** 查询全部设置项 */
export function fetchSettings() {
  return request.get<Api.Setting.Values>({ url: '/api/settings' })
}

/** 批量更新设置项 */
export function updateSettings(values: Api.Setting.Values) {
  return request.put<void>({ url: '/api/settings', params: { values } })
}

/** 查询节假日列表，year 为空表示不筛选 */
export function fetchHolidays(year?: string) {
  return request.get<Api.Setting.Holiday[]>({
    url: '/api/holidays',
    params: year ? { year } : undefined
  })
}

/** 批量保存节假日，按日期覆盖更新 */
export function saveHolidays(entries: Api.Setting.HolidayEntry[]) {
  return request.put<void>({ url: '/api/holidays', params: { entries } })
}

/** 删除指定日期的节假日记录 */
export function deleteHoliday(date: string) {
  return request.del<void>({ url: `/api/holidays/${date}` })
}
