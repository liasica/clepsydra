/**
 * HTTP 错误处理模块
 *
 * 从旧前端（Art Design Pro）的 utils/http/error.ts 摘取业务规则重装，适配 vben
 * requestClient 的拦截器体系，提供：
 *
 * - HttpError：统一错误类型，`code` 为业务错误码（如 42200 表示状态冲突）
 * - handleError：业务码优先于 HTTP 状态码的错误提取逻辑，退回时按状态码给出兜底文案
 * - showError / showSuccess：统一的错误 / 成功提示展示
 * - isHttpError：类型守卫，供业务代码判断 `error.code === 42200` 等状态冲突场景
 *
 * 不直接依赖 axios 类型——axios 只是 @vben/request 的间接依赖，未在本应用
 * package.json 中声明为直接依赖，pnpm 严格隔离下从本应用代码里 `import 'axios'`
 * 会解析失败，因此这里用结构兼容的 ResponseLikeError 做鸭子类型
 */
import { message } from 'antdv-next';

import { $t } from '#/locales';

/** 接口状态码，success 为后端业务成功码，其余用于 HTTP 状态兜底文案匹配 */
export enum ApiStatus {
  success = 0,
  error = 400,
  unauthorized = 401,
  forbidden = 403,
  notFound = 404,
  methodNotAllowed = 405,
  requestTimeout = 408,
  internalServerError = 500,
  notImplemented = 501,
  badGateway = 502,
  serviceUnavailable = 503,
  gatewayTimeout = 504,
  httpVersionNotSupported = 505,
}

/** 后端统一响应体中的错误结构 */
export interface ErrorResponse {
  /** 业务错误码 */
  code: number;
  /** 错误消息 */
  message: string;
  /** 错误附加数据 */
  data?: unknown;
}

/** 错误日志数据 */
export interface ErrorLogData {
  code: number;
  message: string;
  data?: unknown;
  timestamp: string;
  url?: string;
  method?: string;
  stack?: string;
}

/**
 * 与 Axios 错误结构兼容的最小接口，仅声明 handleError 实际读取的字段
 * 真实的 AxiosError / vben defaultResponseInterceptor 在 2xx 但业务码非 0 时
 * 构造的类响应对象都满足这个结构（后者的字段更多，属于超集）
 */
export interface ResponseLikeError {
  code?: string;
  message?: string;
  config?: {
    method?: string;
    url?: string;
  };
  response?: {
    data?: unknown;
    status: number;
  };
}

/** 统一 HTTP 错误类型 */
export class HttpError extends Error {
  public readonly code: number;
  public readonly data?: unknown;
  public readonly method?: string;
  public readonly timestamp: string;
  public readonly url?: string;

  constructor(
    message: string,
    code: number,
    options?: {
      data?: unknown;
      method?: string;
      url?: string;
    },
  ) {
    super(message);
    this.name = 'HttpError';
    this.code = code;
    this.data = options?.data;
    this.timestamp = new Date().toISOString();
    this.url = options?.url;
    this.method = options?.method;
  }

  public toLogData(): ErrorLogData {
    return {
      code: this.code,
      message: this.message,
      data: this.data,
      timestamp: this.timestamp,
      url: this.url,
      method: this.method,
      stack: this.stack,
    };
  }
}

/** 按 HTTP 状态码取兜底文案 */
const getErrorMessage = (status: number): string => {
  const errorMap: Record<number, string> = {
    [ApiStatus.unauthorized]: 'httpMsg.unauthorized',
    [ApiStatus.forbidden]: 'httpMsg.forbidden',
    [ApiStatus.notFound]: 'httpMsg.notFound',
    [ApiStatus.methodNotAllowed]: 'httpMsg.methodNotAllowed',
    [ApiStatus.requestTimeout]: 'httpMsg.requestTimeout',
    [ApiStatus.internalServerError]: 'httpMsg.internalServerError',
    [ApiStatus.badGateway]: 'httpMsg.badGateway',
    [ApiStatus.serviceUnavailable]: 'httpMsg.serviceUnavailable',
    [ApiStatus.gatewayTimeout]: 'httpMsg.gatewayTimeout',
  };

  return $t(errorMap[status] || 'httpMsg.internalServerError');
};

/**
 * 将请求错误统一转换为 HttpError
 * 业务码优先于 HTTP 状态码：响应体带 code + message 时，用业务码构造 HttpError
 * （账单 / 需求页面全靠这条判断 42200 状态冲突），其余情况退回 HTTP 状态码兜底文案
 */
export function handleError(error: ResponseLikeError): HttpError {
  // 请求被取消
  if (error.code === 'ERR_CANCELED') {
    console.warn('Request cancelled:', error.message);
    return new HttpError($t('httpMsg.requestCancelled'), ApiStatus.error);
  }

  const requestConfig = error.config;

  // 网络错误，无响应体
  if (!error.response) {
    return new HttpError($t('httpMsg.networkError'), ApiStatus.error, {
      url: requestConfig?.url,
      method: requestConfig?.method?.toUpperCase(),
    });
  }

  const statusCode = error.response.status;
  const body: unknown = error.response.data;

  // 响应体带业务错误码与 message 时，业务码优先于 HTTP 状态码
  if (
    typeof body === 'object' &&
    body !== null &&
    'code' in body &&
    'message' in body
  ) {
    const business = body as ErrorResponse;
    return new HttpError(business.message, business.code, {
      data: body,
      url: requestConfig?.url,
      method: requestConfig?.method?.toUpperCase(),
    });
  }

  // 退回 HTTP 状态码逻辑
  const errorMessage =
    (body as ErrorResponse | undefined)?.message || error.message;
  const msg = statusCode
    ? getErrorMessage(statusCode)
    : errorMessage || $t('httpMsg.requestFailed');
  return new HttpError(msg, statusCode || ApiStatus.error, {
    data: body,
    url: requestConfig?.url,
    method: requestConfig?.method?.toUpperCase(),
  });
}

/** 显示错误消息 */
export function showError(error: HttpError, showMessage: boolean = true): void {
  if (showMessage) {
    message.error(error.message);
  }
  console.error('[HTTP Error]', error.toLogData());
}

/** 显示成功消息 */
export function showSuccess(msg: string, showMessage: boolean = true): void {
  if (showMessage) {
    message.success(msg);
  }
}

/** 判断是否为 HttpError 类型，业务代码用它 + error.code === 42200 判断状态冲突 */
export const isHttpError = (error: unknown): error is HttpError => {
  return error instanceof HttpError;
};
