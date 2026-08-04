/**
 * 系统升级日志数据
 *
 * 供 upgrade.ts 版本升级检测机制使用
 * 原模板自带的历史更新记录（Art Design Pro 框架自身版本历史）与本项目无关，已清空
 * 后续 Clepsydra 项目如需版本升级公告，可在此维护
 *
 * @module utils/sys/changeLog
 */
interface UpgradeLog {
  version: string // 版本号
  title: string // 更新标题
  date: string // 更新日期
  detail?: string[] // 更新内容
  requireReLogin?: boolean // 是否需要重新登录
  remark?: string // 备注
}

export const upgradeLogList = ref<UpgradeLog[]>([])
