import { requestClient } from '#/api/request';

/** 查询全部设置项（仅超级管理员） */
export function fetchSettings() {
  return requestClient.get<Api.Setting.Values>('/api/settings');
}

/** 批量更新设置项（仅超级管理员） */
export function updateSettings(values: Api.Setting.Values): Promise<void> {
  return requestClient.put('/api/settings', { values });
}

/** 查询节假日列表，year 为空表示不筛选（仅超级管理员） */
export function fetchHolidays(year?: string) {
  return requestClient.get<Api.Setting.Holiday[]>('/api/holidays', {
    params: year ? { year } : undefined,
  });
}

/** 批量保存节假日，按日期覆盖更新（仅超级管理员） */
export function saveHolidays(
  entries: Api.Setting.HolidayEntry[],
): Promise<void> {
  return requestClient.put('/api/holidays', { entries });
}

/** 删除指定日期的节假日记录（仅超级管理员） */
export function deleteHoliday(date: string): Promise<void> {
  return requestClient.delete(`/api/holidays/${date}`);
}
