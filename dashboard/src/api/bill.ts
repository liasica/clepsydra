import request from '@/utils/http'

/** 查询账单列表 */
export function fetchBills() {
  return request.get<Api.Bill.Detail[]>({ url: '/api/bills' })
}

/** 查询账单详情，含明细行 */
export function fetchBill(id: number) {
  return request.get<Api.Bill.Detail>({ url: `/api/bills/${id}` })
}

/** 生成指定账期账单草稿，已存在同账期草稿时重新生成 */
export function generateBill(period: string) {
  return request.post<Api.Bill.Detail>({ url: '/api/bills/generate', params: { period } })
}

/** 切换明细行减免状态，仅草稿账单的计费行可用 */
export function toggleWaive(billId: number, itemId: number) {
  return request.post<void>({ url: `/api/bills/${billId}/items/${itemId}/waive` })
}

/** 分享账单，draft 流转 pending */
export function shareBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/share` })
}

/** 撤回已分享账单，pending 回退 draft */
export function revokeBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/revoke` })
}

/** 需求方确认账单，pending 流转 confirmed */
export function confirmBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/confirm` })
}
