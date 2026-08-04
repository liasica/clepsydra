import request from '@/utils/http'

/** 查询需求列表，status 为空返回全部 */
export function fetchDemands(status?: Api.Demand.Status) {
  return request.get<Api.Demand.Item[]>({
    url: '/api/demands',
    params: status ? { status } : undefined
  })
}

/** 查询需求详情 */
export function fetchDemand(id: number) {
  return request.get<Api.Demand.Item>({ url: `/api/demands/${id}` })
}

/** 创建需求 */
export function createDemand(params: Api.Demand.SaveParams) {
  return request.post<Api.Demand.Item>({ url: '/api/demands', params })
}

/** 更新需求，仅草稿可改 */
export function updateDemand(id: number, params: Api.Demand.SaveParams) {
  return request.put<Api.Demand.Item>({ url: `/api/demands/${id}`, params })
}

/** 提交预估人天，draft 流转 pending_estimate */
export function submitEstimate(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/submit-estimate` })
}

/** 需求方确认预估人天，pending_estimate 流转 confirmed */
export function confirmEstimate(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/confirm-estimate` })
}

/** 标记开工，confirmed 流转 in_progress */
export function startDemand(id: number, params: Api.Demand.StartParams) {
  return request.post<void>({ url: `/api/demands/${id}/start`, params })
}

/** 标记完成，in_progress 流转 pending_acceptance */
export function finishDemand(id: number, params: Api.Demand.FinishParams) {
  return request.post<void>({ url: `/api/demands/${id}/finish`, params })
}

/** 需求方验收，pending_acceptance 流转 accepted */
export function acceptDemand(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/accept` })
}
