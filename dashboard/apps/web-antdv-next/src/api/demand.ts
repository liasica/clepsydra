import { requestClient } from '#/api/request';

/** 查询需求列表，可按状态、项目与优先级筛选，缺省返回全部 */
export function fetchDemands(params?: {
  priority?: Api.Demand.Priority;
  project_id?: number;
  status?: Api.Demand.Status;
}) {
  return requestClient.get<Api.Demand.Item[]>('/api/demands', { params });
}

/** 查询需求详情 */
export function fetchDemand(id: number) {
  return requestClient.get<Api.Demand.Item>(`/api/demands/${id}`);
}

/** 创建需求；登录即可操作，需求方仅可提交标题与描述，超管可携带预估人天并勾选已确认直达 confirmed */
export function createDemand(params: Api.Demand.CreateParams) {
  return requestClient.post<Api.Demand.Item>('/api/demands', params);
}

/** 更新需求标题与描述，draft 与 pending_estimate 状态允许，人天确认后锁定；需求方也可修改 */
export function updateDemand(id: number, params: Api.Demand.SaveParams) {
  return requestClient.put<Api.Demand.Item>(`/api/demands/${id}`, params);
}

/**
 * 删除需求，任意状态均可（仅超级管理员）
 *
 * 后端是软删除：记录保留，只是不再出现在列表、工作台统计与后续账单里；
 * 已生成账单存的是快照，金额与明细不受影响
 */
export function deleteDemand(id: number): Promise<void> {
  return requestClient.delete(`/api/demands/${id}`);
}

/** 提交预估人天与预计开工日期，draft 流转 pending_estimate；pending_estimate 下可重复提交修正（仅超级管理员） */
export function submitEstimate(
  id: number,
  params: Api.Demand.EstimateParams,
): Promise<void> {
  return requestClient.post(`/api/demands/${id}/submit-estimate`, params);
}

/** 需求方确认预估人天，pending_estimate 流转 confirmed */
export function confirmEstimate(id: number): Promise<void> {
  return requestClient.post(`/api/demands/${id}/confirm-estimate`);
}

/** 标记开工，confirmed 流转 in_progress（仅超级管理员） */
export function startDemand(
  id: number,
  params: Api.Demand.StartParams,
): Promise<void> {
  return requestClient.post(`/api/demands/${id}/start`, params);
}

/** 标记完成，in_progress 流转 pending_acceptance（仅超级管理员） */
export function finishDemand(
  id: number,
  params: Api.Demand.FinishParams,
): Promise<void> {
  return requestClient.post(`/api/demands/${id}/finish`, params);
}

/** 需求方验收，pending_acceptance 流转 accepted */
export function acceptDemand(id: number): Promise<void> {
  return requestClient.post(`/api/demands/${id}/accept`);
}

/** 全量覆盖需求的项目标签，传空数组即清空；任何状态可用，登录即可操作 */
export function updateDemandProjects(id: number, projectIds: number[]) {
  return requestClient.put<Api.Demand.Item>(`/api/demands/${id}/projects`, {
    project_ids: projectIds,
  });
}

/** 调整需求优先级；优先级是排期参考元数据，任何状态可用，登录即可操作 */
export function updateDemandPriority(id: number, priority: Api.Demand.Priority) {
  return requestClient.put<Api.Demand.Item>(`/api/demands/${id}/priority`, {
    priority,
  });
}
