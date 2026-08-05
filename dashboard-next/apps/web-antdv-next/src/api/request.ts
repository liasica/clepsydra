/**
 * 该文件可自行根据业务逻辑进行调整
 *
 * 在 vben requestClient 的拦截器体系里复刻旧前端（Art Design Pro）
 * utils/http 摘取的 5 条业务规则，详见 utils/http/error.ts 头部说明：
 *
 * 1. 业务码优先于 HTTP 状态码——见 handleError（业务/需求/账单页面靠 error.code === 42200 判断状态冲突）
 * 2. 登录接口豁免——见 isLoginRequest 及其在两个响应拦截器里的短路分支
 * 3. POST/PUT 的 params → data——通过 requestClient.post/put(url, data) 的调用方式保证，业务模块见 #/api/{demand,bill,...}
 * 4. 业务成功码固定为 0——见 defaultResponseInterceptor 的 successCode 配置
 * 5. 13 个错误文案 key——见 #/locales/langs/zh-CN/httpMsg.json，由 handleError 按状态码取用
 */
import type { RequestClientOptions } from '@vben/request';

import { useAppConfig } from '@vben/hooks';
import { preferences } from '@vben/preferences';
import {
  authenticateResponseInterceptor,
  defaultResponseInterceptor,
  RequestClient,
} from '@vben/request';
import { useAccessStore } from '@vben/stores';

import { $t } from '#/locales';
import { useAuthStore } from '#/store';
import {
  ApiStatus,
  handleError,
  HttpError,
  showError,
} from '#/utils/http/error';

import { refreshTokenApi } from './core';

const { apiURL } = useAppConfig(import.meta.env, import.meta.env.PROD);

// clepsydra 登录接口路径。登录失败的错误提示交由登录页自行处理，不走通用未授权兜底、
// 不误触发登出、不占用会话恢复状态，防御后端未来把登录失败语义调整为 401 时被误判为
// 会话过期（历史 bug，参考 commit 84f486d：登录密码错误实测为 HTTP 400 而非 401）
const LOGIN_URL = '/api/auth/login';

/** 判断请求是否为登录接口 */
function isLoginRequest(url?: string): boolean {
  return !!url?.includes(LOGIN_URL);
}

function createRequestClient(baseURL: string, options?: RequestClientOptions) {
  const client = new RequestClient({
    ...options,
    baseURL,
  });

  /**
   * 重新认证逻辑
   */
  async function doReAuthenticate() {
    console.warn('Access token or refresh token is invalid or expired. ');
    const accessStore = useAccessStore();
    const authStore = useAuthStore();
    accessStore.setAccessToken(null);
    if (
      preferences.app.loginExpiredMode === 'modal' &&
      accessStore.isAccessChecked
    ) {
      accessStore.setLoginExpired(true);
    } else {
      await authStore.logout();
    }
  }

  /**
   * 刷新token逻辑
   */
  async function doRefreshToken() {
    const accessStore = useAccessStore();
    const resp = await refreshTokenApi();
    const newToken = resp.data;
    accessStore.setAccessToken(newToken);
    return newToken;
  }

  function formatToken(token: null | string) {
    return token ? `Bearer ${token}` : null;
  }

  // 请求头处理
  client.addRequestInterceptor({
    fulfilled: async (config) => {
      const accessStore = useAccessStore();

      config.headers.Authorization = formatToken(accessStore.accessToken);
      config.headers['Accept-Language'] = preferences.app.locale;
      return config;
    },
    rejected: (error) => {
      // 请求配置阶段出错（如拦截器自身抛错），与响应错误分开提示
      showError(
        new HttpError($t('httpMsg.requestConfigError'), ApiStatus.error),
      );
      return Promise.reject(error);
    },
  });

  // 处理返回的响应数据格式，业务成功码固定为 0（规则 4）
  client.addResponseInterceptor(
    defaultResponseInterceptor({
      codeField: 'code',
      dataField: 'data',
      successCode: ApiStatus.success,
    }),
  );

  // token 过期的处理；登录接口豁免，避免登录失败被误判为会话过期而误登出（规则 2）
  const authenticateInterceptor = authenticateResponseInterceptor({
    client,
    doReAuthenticate,
    doRefreshToken,
    enableRefreshToken: preferences.app.enableRefreshToken,
    formatToken,
  });
  client.addResponseInterceptor({
    rejected: (error) => {
      if (isLoginRequest(error?.config?.url)) {
        throw error;
      }
      return authenticateInterceptor.rejected?.(error) ?? Promise.reject(error);
    },
  });

  // 统一转换为 HttpError：业务码优先于 HTTP 状态码（规则 1），业务代码用 error.code 判断
  // 状态冲突（如 42200）；登录接口的错误展示交由登录页自行处理，不重复弹出（规则 2）；
  // HTTP 401 已由上一级拦截器处理会话状态（弹窗或跳转登录），不再重复弹出通用提示——
  // 这里按原始 HTTP 状态码判断，而非转换后的业务码（业务码是 40100 而非 401）
  client.addResponseInterceptor({
    rejected: (error) => {
      const httpError = handleError(error);
      const isLogin = isLoginRequest(error?.config?.url);
      const isUnauthorizedStatus =
        error?.response?.status === ApiStatus.unauthorized;
      if (!isLogin && !isUnauthorizedStatus) {
        showError(httpError);
      }
      return Promise.reject(httpError);
    },
  });

  return client;
}

export const requestClient = createRequestClient(apiURL, {
  responseReturn: 'data',
});

export const baseRequestClient = new RequestClient({ baseURL: apiURL });
