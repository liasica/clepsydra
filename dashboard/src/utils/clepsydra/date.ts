import dayjs from 'dayjs'

/** ISO 时间截取日期部分，空值显示占位符 */
export function formatDate(value: string | null | undefined): string {
  if (!value) return '—'
  return value.slice(0, 10)
}

/** ISO 时间格式化到分钟，空值显示占位符 */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  return dayjs(value).format('YYYY-MM-DD HH:mm')
}
