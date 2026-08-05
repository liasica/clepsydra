/**
 * 人天与金额换算工具
 * 后端人天一律以整数半天数存储（1 人天 = 2），金额以整数元存储
 */

/** 半天数转人天 */
export function halfDaysToManday(half: number): number {
  return half / 2;
}

/** 人天转半天数 */
export function mandayToHalfDays(manday: number): number {
  return Math.round(manday * 2);
}

/**
 * 格式化人天展示，空值与未预估（0）显示占位符
 *
 * 仅用于需求侧字段（estimated_half_days / actual_half_days）：这两个字段业务上
 * 0 等价于「尚未提交预估 / 尚未完成」——SubmitEstimate 与 Finish 均校验人天必须为正，
 * 一旦提交就不可能回落到 0，因此 0 与空值语义等价，可以合并显示为占位符
 */
export function formatManday(half: null | number | undefined): string {
  if (!half) return '—';
  return `${halfDaysToManday(half)} 人天`;
}

/**
 * 格式化人天展示（严格版），仅空值显示占位符，0 视为真实值正常显示
 *
 * 用于账单聚合字段（total_half_days）与明细行（half_days）：账单人天可以合法为 0
 * （如整月无计费/展示需求，仅收基础维护费），后端 handler/bill_dto.go 特意去掉
 * omitempty 以保证 0 能抵达前端，这里必须用 === null/undefined 判断，
 * 不能沿用需求侧 formatManday 的 !half 语义，否则真实的 0 人天会被误显示为占位符
 */
export function formatMandayStrict(half: null | number | undefined): string {
  if (half === null || half === undefined) return '—';
  return `${halfDaysToManday(half)} 人天`;
}

/** 格式化金额为带千分位的元，空值显示占位符 */
export function formatAmount(yuan: null | number | undefined): string {
  if (yuan === null || yuan === undefined) return '—';
  return `¥${yuan.toLocaleString('zh-CN')}`;
}
