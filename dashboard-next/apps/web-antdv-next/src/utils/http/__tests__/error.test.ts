import type { ResponseLikeError } from '../error';

import { describe, expect, it, vi } from 'vitest';

import { handleError, HttpError } from '../error';

// error.ts 依赖 #/locales 的 $t 与 antdv-next 的 message 提示，测试环境中以桩替换
// vi.mock 会被提升到模块顶部执行，写在 import 之后不影响 mock 生效
vi.mock('#/locales', () => ({ $t: (key: string) => key }));
vi.mock('antdv-next', () => ({
  message: { error: vi.fn(), success: vi.fn() },
}));

/** 构造带响应体的类 Axios 错误，不依赖 axios（未在本应用声明为直接依赖） */
function errorWith(status: number, body: unknown): ResponseLikeError {
  return {
    message: 'Request failed',
    config: { url: '/api/demands/1', method: 'PUT' },
    response: { status, data: body },
  };
}

describe('handleError', () => {
  it('从响应体提取业务错误码与 message', () => {
    const err = handleError(
      errorWith(422, { code: 42_200, message: '当前状态不允许该操作' }),
    );
    expect(err).toBeInstanceOf(HttpError);
    expect(err.code).toBe(42_200);
    expect(err.message).toBe('当前状态不允许该操作');
  });

  it('响应体无业务结构时退回 HTTP 状态码', () => {
    const err = handleError(errorWith(502, 'Bad Gateway'));
    expect(err.code).toBe(502);
  });
});
