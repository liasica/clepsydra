/**
 * 需求与账单状态字典
 * label 为展示文案，type 为 Element Plus 标签配色，actions 为该状态下各角色可执行的操作
 * 与后端状态机白名单保持一致，页面按钮渲染与操作守卫共用此定义
 */

export type DemandStatus =
  | 'draft'
  | 'pending_estimate'
  | 'confirmed'
  | 'in_progress'
  | 'pending_acceptance'
  | 'accepted'

export type BillStatus = 'draft' | 'pending' | 'confirmed'

export type DemandAction =
  | 'edit'
  | 'submitEstimate'
  | 'confirmEstimate'
  | 'start'
  | 'finish'
  | 'accept'

export type BillAction = 'regenerate' | 'waive' | 'share' | 'revoke' | 'confirm'

type TagType = 'info' | 'primary' | 'warning' | 'success' | 'danger'

interface StatusMeta<A extends string> {
  label: string
  type: TagType
  actions: {
    admin: A[]
    client: A[]
  }
}

export const DEMAND_STATUS: Record<DemandStatus, StatusMeta<DemandAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['edit', 'submitEstimate'], client: [] }
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: { admin: [], client: ['confirmEstimate'] }
  },
  confirmed: {
    label: '已确认待开工',
    type: 'primary',
    actions: { admin: ['start'], client: [] }
  },
  in_progress: {
    label: '进行中',
    type: 'primary',
    actions: { admin: ['finish'], client: [] }
  },
  pending_acceptance: {
    label: '完成待确认',
    type: 'warning',
    actions: { admin: [], client: ['accept'] }
  },
  accepted: {
    label: '已确认',
    type: 'success',
    actions: { admin: [], client: [] }
  }
}

export const BILL_STATUS: Record<BillStatus, StatusMeta<BillAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['regenerate', 'waive', 'share'], client: [] }
  },
  pending: {
    label: '待确认',
    type: 'warning',
    actions: { admin: ['revoke'], client: ['confirm'] }
  },
  confirmed: {
    label: '已确认',
    type: 'success',
    actions: { admin: [], client: [] }
  }
}
