import { requestClient } from '#/api/request';

/** 查询标签列表，登录即可，供管理页、下拉与筛选使用 */
export function fetchTags() {
  return requestClient.get<Api.Tag.Item[]>('/api/tags');
}

/** 创建标签（仅超级管理员），名称唯一，颜色由后端按名称生成并固化 */
export function createTag(params: Api.Tag.SaveParams) {
  return requestClient.post<Api.Tag.Item>('/api/tags', params);
}

/** 更新标签名称（仅超级管理员），颜色保持创建时的固化值不变 */
export function updateTag(id: number, params: Api.Tag.SaveParams) {
  return requestClient.put<Api.Tag.Item>(`/api/tags/${id}`, params);
}

/** 删除标签（仅超级管理员），自动解除与需求的关联，需求本身不受影响 */
export function deleteTag(id: number): Promise<void> {
  return requestClient.delete(`/api/tags/${id}`);
}
