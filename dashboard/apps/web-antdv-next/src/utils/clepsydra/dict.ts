/**
 * 需求与账单状态字典
 * label 为展示文案，type 为 Element Plus 标签配色，actions 为该状态下各角色可执行的操作
 * 与后端状态机白名单保持一致，页面按钮渲染与操作守卫共用此定义
 *
 * 权限约定：超级管理员拥有全部权限，因此 admin 恒为 client 的超集——
 * 需求方专属的确认类操作（confirmEstimate、accept、confirm）超管同样可代为执行
 * waive / addItem / editItem / removeItem 为明细区交互动作，不渲染为顶部按钮
 */

export type DemandStatus =
  | 'accepted'
  | 'confirmed'
  | 'draft'
  | 'in_progress'
  | 'pending_acceptance'
  | 'pending_estimate';

export type BillStatus = 'paid' | 'pending' | 'unpaid';

export type DemandAction =
  | 'accept'
  | 'confirmEstimate'
  | 'delete'
  | 'edit'
  | 'finish'
  | 'start'
  | 'submitEstimate';

export type BillAction =
  | 'addItem'
  | 'confirm'
  | 'edit'
  | 'editItem'
  | 'pay'
  | 'removeItem'
  | 'waive';

type TagType = 'danger' | 'info' | 'primary' | 'success' | 'warning';

interface StatusMeta<A extends string> {
  label: string;
  type: TagType;
  actions: {
    admin: A[];
    client: A[];
  };
}

/** Element Plus 语义色 → antdv-next Tag color 映射，隔离 UI 库差异 */
export function tagColor(type: TagType): string {
  const map: Record<TagType, string> = {
    info: 'default',
    primary: 'processing',
    warning: 'warning',
    success: 'success',
    danger: 'error',
  };
  return map[type];
}

export const DEMAND_STATUS: Record<DemandStatus, StatusMeta<DemandAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['edit', 'submitEstimate', 'delete'], client: ['edit'] },
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: {
      // 代确认属兜底操作，排在超管自身的编辑与修改预估之后
      admin: ['edit', 'submitEstimate', 'confirmEstimate', 'delete'],
      client: ['edit', 'confirmEstimate'],
    },
  },
  confirmed: {
    label: '已确认待开工',
    type: 'primary',
    actions: { admin: ['start', 'delete'], client: [] },
  },
  in_progress: {
    label: '进行中',
    type: 'primary',
    actions: { admin: ['finish', 'delete'], client: [] },
  },
  pending_acceptance: {
    label: '完成待确认',
    type: 'warning',
    actions: { admin: ['accept', 'delete'], client: ['accept'] },
  },
  accepted: {
    label: '已确认',
    type: 'success',
    actions: { admin: ['delete'], client: [] },
  },
};

export const BILL_STATUS: Record<BillStatus, StatusMeta<BillAction>> = {
  pending: {
    label: '待确认',
    type: 'warning',
    actions: {
      admin: ['confirm', 'edit', 'waive', 'addItem', 'editItem', 'removeItem'],
      client: ['confirm'],
    },
  },
  unpaid: {
    label: '待支付',
    type: 'primary',
    actions: {
      admin: ['pay', 'edit', 'waive', 'addItem', 'editItem', 'removeItem'],
      client: [],
    },
  },
  paid: {
    label: '已支付',
    type: 'success',
    actions: { admin: [], client: [] },
  },
};
