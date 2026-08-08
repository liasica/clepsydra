import { requestClient } from '#/api/request';

/**
 * 上传图片，返回可直接写进 markdown 的访问地址
 *
 * 后端按 MIME 白名单与大小上限校验，落盘用随机文件名；读取接口不鉴权，
 * 因此返回的地址可以直接作为 <img src>
 */
export async function uploadImage(file: File): Promise<string> {
  // 必须走 requestClient.upload：requestClient.post 会套上默认的
  // `Content-Type: application/json`，FormData 被当 JSON 序列化后
  // 后端收不到 multipart，报「未收到上传文件」
  const { url } = await requestClient.upload<{ url: string }>('/api/uploads', {
    file,
  });

  return url;
}
