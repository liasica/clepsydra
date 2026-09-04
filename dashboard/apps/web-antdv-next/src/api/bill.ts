import { requestClient } from '#/api/request';

/** 查询账单列表 */
export function fetchBills() {
  return requestClient.get<Api.Bill.Detail[]>('/api/bills');
}

/** 查询账单详情，含明细行 */
export function fetchBill(id: number) {
  return requestClient.get<Api.Bill.Detail>(`/api/bills/${id}`);
}

/** 创建账单：输入名称并选择需求（仅超级管理员） */
export function createManualBill(name: string, demandIds: number[]) {
  return requestClient.post<Api.Bill.Detail>('/api/bills/manual', {
    name,
    demand_ids: demandIds,
  });
}

/** 查询可加入账单的需求，排除已在任何账单中的需求（仅超级管理员） */
export function fetchSelectableDemands() {
  return requestClient.get<Api.Demand.Item[]>('/api/bills/selectable-demands');
}

/** 向账单添加需求明细，已支付账单拒绝（仅超级管理员） */
export function addBillItem(billId: number, demandId: number): Promise<void> {
  return requestClient.post(`/api/bills/${billId}/items`, {
    demand_id: demandId,
  });
}

/** 从账单移除明细，已支付账单拒绝（仅超级管理员） */
export function removeBillItem(billId: number, itemId: number): Promise<void> {
  return requestClient.delete(`/api/bills/${billId}/items/${itemId}`);
}

/** 切换明细行减免状态，待确认与待支付账单可用（仅超级管理员） */
export function toggleWaive(billId: number, itemId: number): Promise<void> {
  return requestClient.post(`/api/bills/${billId}/items/${itemId}/waive`);
}

/** 确认账单，待确认流转待支付（需求方或超级管理员） */
export function confirmBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/confirm`);
}

/** 标记账单已支付，支付后锁定（仅超级管理员） */
export function payBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/pay`);
}

/** 编辑账单，缺省字段不修改，已支付账单拒绝（仅超级管理员） */
export function updateBill(
  id: number,
  params: Api.Bill.UpdateParams,
): Promise<void> {
  return requestClient.request(`/api/bills/${id}`, {
    data: params,
    method: 'PATCH',
  });
}

/** 编辑账单明细行，缺省字段不修改，已支付账单拒绝（仅超级管理员） */
export function updateBillItem(
  billId: number,
  itemId: number,
  params: Api.Bill.UpdateItemParams,
): Promise<void> {
  return requestClient.request(`/api/bills/${billId}/items/${itemId}`, {
    data: params,
    method: 'PATCH',
  });
}
