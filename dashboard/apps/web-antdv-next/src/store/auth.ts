import type { Recordable, UserInfo } from '@vben/types';

import { ref } from 'vue';
import { useRouter } from 'vue-router';

import { LOGIN_PATH } from '@vben/constants';
import { preferences } from '@vben/preferences';
import { resetAllStores, useAccessStore, useUserStore } from '@vben/stores';

import { notification } from 'antdv-next';
import { defineStore } from 'pinia';

import { fetchLogin, fetchMe } from '#/api/auth';
import { $t } from '#/locales';
import { resetAccessInitState } from '#/router/guard';

/**
 * 后端精简用户（SimpleUser）转前端会话用户，补 roles 数组供框架按角色过滤菜单
 * 角色只有 admin 与 client 两种；无头像 / 首页定制需求，其余字段留空走框架默认值
 */
function toUserInfo(user: Api.Auth.SimpleUser): UserInfo {
  return {
    avatar: preferences.app.defaultAvatar,
    desc: '',
    homePath: '',
    realName: user.name,
    roles: [user.role],
    token: '',
    userId: String(user.id),
    username: user.name,
  };
}

export const useAuthStore = defineStore('auth', () => {
  const accessStore = useAccessStore();
  const userStore = useUserStore();
  const router = useRouter();

  const loginLoading = ref(false);

  /**
   * 登录：调用后端 /api/auth/login，存令牌与用户信息
   * 登录失败时抛出的错误（已由 request.ts 的登录接口豁免规则转成 HttpError，
   * 携带后端原始 message）交由登录页自行捕获展示，这里不吞错也不弹兜底提示
   * @param params 登录表单数据
   */
  async function authLogin(
    params: Recordable<any>,
    onSuccess?: () => Promise<void> | void,
  ) {
    let userInfo: null | UserInfo = null;
    try {
      loginLoading.value = true;
      const { token, user } = await fetchLogin({
        password: params.password,
        username: params.username,
      });

      if (token) {
        accessStore.setAccessToken(token);

        userInfo = toUserInfo(user);
        userStore.setUserInfo(userInfo);

        if (accessStore.loginExpired) {
          accessStore.setLoginExpired(false);
        } else {
          onSuccess
            ? await onSuccess?.()
            : await router.push(
                userInfo.homePath || preferences.app.defaultHomePath,
              );
        }

        if (userInfo?.realName) {
          notification.success({
            description: `${$t('authentication.loginSuccessDesc')}:${userInfo?.realName}`,
            duration: 3,
            title: $t('authentication.loginSuccess'),
          });
        }
      }
    } finally {
      loginLoading.value = false;
    }

    return {
      userInfo,
    };
  }

  /**
   * 退出登录：后端未提供登出接口（JWT 无状态），仅清空本地状态并跳回登录页
   */
  async function logout(redirect: boolean = true) {
    resetAllStores();
    resetAccessInitState();
    accessStore.setLoginExpired(false);

    // 回登录页带上当前路由地址
    await router.replace({
      path: LOGIN_PATH,
      query: redirect
        ? {
            redirect: encodeURIComponent(router.currentRoute.value.fullPath),
          }
        : {},
    });
  }

  /**
   * 会话恢复：向后端 /api/auth/me 校验当前令牌并刷新用户信息
   * 由路由守卫（router/guard.ts）在 userStore.userInfo 为空时调用一次；
   * 令牌失效时 /api/auth/me 返回 401，已由 request.ts 的通用 401 兜底
   * （doReAuthenticate）接管登出，这里无需重复处理失败分支
   */
  async function fetchUserInfo() {
    const user = await fetchMe();
    const userInfo = toUserInfo(user);
    userStore.setUserInfo(userInfo);
    return userInfo;
  }

  function $reset() {
    loginLoading.value = false;
  }

  return {
    $reset,
    authLogin,
    fetchUserInfo,
    loginLoading,
    logout,
  };
});
