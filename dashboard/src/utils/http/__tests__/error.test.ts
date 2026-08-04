import { describe, expect, it, vi } from 'vitest'
import { AxiosError, AxiosHeaders } from 'axios'

// error.ts 依赖 @/locales 的 $t 与 UI 提示，测试环境中以桩替换
vi.mock('@/locales', () => ({ $t: (key: string) => key }))
vi.mock('element-plus', () => ({ ElMessage: { error: vi.fn(), success: vi.fn() } }))

import { ErrorResponse, HttpError, handleError } from '../error'

/** 构造带响应体的 AxiosError */
function axiosErrorWith(status: number, body: unknown): AxiosError<ErrorResponse> {
  const error = new AxiosError<ErrorResponse>('Request failed', 'ERR_BAD_REQUEST')
  error.response = {
    data: body as ErrorResponse,
    status,
    statusText: '',
    headers: {},
    config: { headers: new AxiosHeaders() }
  }
  return error
}

describe('handleError', () => {
  it('从响应体提取业务错误码与 message', () => {
    const err = handleError(axiosErrorWith(422, { code: 42200, message: '当前状态不允许该操作' }))
    expect(err).toBeInstanceOf(HttpError)
    expect(err.code).toBe(42200)
    expect(err.message).toBe('当前状态不允许该操作')
  })

  it('响应体无业务结构时退回 HTTP 状态码', () => {
    const err = handleError(axiosErrorWith(502, 'Bad Gateway'))
    expect(err.code).toBe(502)
  })
})
