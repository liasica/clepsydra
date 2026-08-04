/**
 * 人天与金额换算工具
 * 后端人天一律以整数半天数存储（1 人天 = 2），金额以整数元存储
 */

/** 半天数转人天 */
export function halfDaysToManday(half: number): number {
  return half / 2
}

/** 人天转半天数 */
export function mandayToHalfDays(manday: number): number {
  return Math.round(manday * 2)
}

/** 格式化人天展示，空值显示占位符 */
export function formatManday(half: number | null | undefined): string {
  if (half === null || half === undefined) return '—'
  return `${halfDaysToManday(half)} 人天`
}

/** 格式化金额为带千分位的元，空值显示占位符 */
export function formatAmount(yuan: number | null | undefined): string {
  if (yuan === null || yuan === undefined) return '—'
  return `¥${yuan.toLocaleString('zh-CN')}`
}
