import { requestClient } from '#/api/request';

/** 查询账单列表 */
export function fetchBills() {
  return requestClient.get<Api.Bill.Detail[]>('/api/bills');
}

/** 查询账单详情，含明细行 */
export function fetchBill(id: number) {
  return requestClient.get<Api.Bill.Detail>(`/api/bills/${id}`);
}

/** 生成指定账期账单草稿，已存在同账期草稿时重新生成（仅超级管理员） */
export function generateBill(period: string) {
  return requestClient.post<Api.Bill.Detail>('/api/bills/generate', { period });
}

/** 切换明细行减免状态，仅草稿账单的计费行可用（仅超级管理员） */
export function toggleWaive(billId: number, itemId: number): Promise<void> {
  return requestClient.post(`/api/bills/${billId}/items/${itemId}/waive`);
}

/** 分享账单，draft 流转 pending（仅超级管理员） */
export function shareBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/share`);
}

/** 撤回已分享账单，pending 回退 draft（仅超级管理员） */
export function revokeBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/revoke`);
}

/** 需求方确认账单，pending 流转 confirmed */
export function confirmBill(id: number): Promise<void> {
  return requestClient.post(`/api/bills/${id}/confirm`);
}
