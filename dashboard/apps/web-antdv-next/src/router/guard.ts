import type { Router } from 'vue-router';

import { LOGIN_PATH } from '@vben/constants';
import { preferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';
import { startProgress, stopProgress } from '@vben/utils';

import { accessRoutes, coreRouteNames } from '#/router/routes';
import { useAuthStore } from '#/store';

import { generateAccess } from './access';

/**
 * 通用守卫配置
 * @param router
 */
function setupCommonGuard(router: Router) {
  // 记录已经加载的页面
  const loadedPaths = new Set<string>();

  router.beforeEach((to) => {
    to.meta.loaded = loadedPaths.has(to.path);

    // 页面加载进度条
    if (!to.meta.loaded && preferences.transition.progress) {
      startProgress();
    }
    return true;
  });

  router.afterEach((to) => {
    // 记录页面是否加载,如果已经加载，后续的页面切换动画等效果不在重复执行

    loadedPaths.add(to.path);

    // 关闭页面加载进度条
    if (preferences.transition.progress) {
      stopProgress();
    }
  });
}

/**
 * 动态路由 / 用户信息初始化的进行中 Promise。
 *
 * 并发导航（如刷新瞬间被连续触发两次守卫）复用同一个 Promise，避免重复请求
 * /api/auth/me、重复生成并注册动态路由；初始化成功后 accessStore.isAccessChecked
 * 转为 true，后续导航直接短路返回，天然保证「刷新只恢复一次会话」（业务规则 1）
 */
let accessInitPromise: null | Promise<void> = null;

/**
 * 初始化失败标记。只有非 401 的意外错误（网络故障、后端异常等）才会置位——
 * 401 已由 request.ts 的通用拦截器接管登出，不属于「初始化失败」。一旦置位，
 * 后续导航不再自动重试，统一导向错误页，防止反复请求形成死循环（业务规则 4）。
 *
 * 该标记与 accessInitPromise 均为模块级变量，生命周期跟随当前页面；发生失败后
 * 唯一的恢复路径是刷新页面（内部错误兜底页本身不带任何可导航的布局），刷新会
 * 重新执行本模块顶层代码，天然重置这两个标记，因此无需额外的登出联动重置
 */
let routeInitFailed = false;

/**
 * 重置动态路由初始化状态。
 *
 * `accessInitPromise` 与 `routeInitFailed` 是模块级变量，只在页面刷新时随模块
 * 重新加载而重置；但登出不刷新页面（SPA 内跳转），若不在此处显式清空，同一次
 * 页面生命周期内的「登出再登录」会复用上一个用户已 resolve 的旧 Promise ——
 * `accessInitPromise ??=` 不会重新触发 `initAccess()`，导致新用户的
 * `accessStore.isAccessChecked` 永远无法被重新置为 true，守卫反复重定向到同一
 * 目标路由，最终被 vue-router 判定为死循环并中止导航（登录页表现为「登录失败」）。
 * 由 authStore.logout()（含 401 自动登出）统一调用，保证两条路径都会重置
 */
function resetAccessInitState() {
  accessInitPromise = null;
  routeInitFailed = false;
}

/**
 * 获取用户信息（会话恢复）并按角色生成可访问的菜单 / 路由
 */
async function initAccess(router: Router): Promise<void> {
  const accessStore = useAccessStore();
  const userStore = useUserStore();
  const authStore = useAuthStore();

  // userStore.userInfo 为空说明是刷新后的一次性会话恢复（T6 的 restoreSession
  // 语义），已有值时说明是同一次应用生命周期内的后续导航，不重复请求 /api/auth/me
  const userInfo = userStore.userInfo || (await authStore.fetchUserInfo());
  const userRoles = userInfo.roles ?? [];

  // 生成菜单和路由（前端模式：按 meta.authority 过滤，见 routes/modules/*.ts）
  const { accessibleMenus, accessibleRoutes } = await generateAccess({
    roles: userRoles,
    router,
    routes: accessRoutes,
  });

  accessStore.setAccessMenus(accessibleMenus);
  accessStore.setAccessRoutes(accessibleRoutes);
  accessStore.setIsAccessChecked(true);
}

/**
 * 权限访问守卫配置
 * @param router
 */
function setupAccessGuard(router: Router) {
  router.beforeEach(async (to, from) => {
    const accessStore = useAccessStore();
    const userStore = useUserStore();

    // 基本路由，这些路由不需要进入权限拦截。
    //
    // 注意：404 兜底路由（FallbackNotFound，见 routes/core.ts 的
    // fallbackNotFoundRoute）不在 coreRouteNames 里，不能当成可匿名访问的
    // 静态路由处理——否则未登录时手输任意地址会直接落在 404 页而不是跳转
    // 登录页（业务规则 3：catch-all 不算可匿名访问的静态路由）
    if (coreRouteNames.includes(to.name as string)) {
      if (to.path === LOGIN_PATH && accessStore.accessToken) {
        return decodeURIComponent(
          (to.query?.redirect as string) ||
            userStore.userInfo?.homePath ||
            preferences.app.defaultHomePath,
        );
      }
      return true;
    }

    // accessToken 检查
    if (!accessStore.accessToken) {
      // 明确声明忽略权限访问权限，则可以访问
      if (to.meta.ignoreAccess) {
        return true;
      }

      // 没有访问权限，跳转登录页面
      if (to.fullPath !== LOGIN_PATH) {
        return {
          path: LOGIN_PATH,
          // 如不需要，直接删除 query
          query:
            to.fullPath === preferences.app.defaultHomePath
              ? {}
              : { redirect: encodeURIComponent(to.fullPath) },
          // 携带当前跳转的页面，登录后重新跳转该页面
          replace: true,
        };
      }
      return to;
    }

    // 是否已经生成过动态路由
    if (accessStore.isAccessChecked) {
      return true;
    }

    // 上一次初始化非 401 失败过，不再自动重试，统一导向错误页（业务规则 4）
    if (routeInitFailed) {
      return to.name === 'FallbackInternalError'
        ? true
        : { name: 'FallbackInternalError', replace: true };
    }

    // 注意：这里不能提前对 to.name === 'FallbackNotFound' 做 404 短路。
    // 业务路由（如 /dashboard）在动态路由注册完成前，同样只能匹配到兜底的
    // catch-all——此时 to.name 也是 'FallbackNotFound'，与真正不存在的路径
    // 无法区分，提前放行会把本该正常打开的业务页面误判成 404
    //
    // 业务规则 2（404 提前放行）在 vben 里由下面「生成路由表」之后的
    // resolve 步骤天然满足：注册动态路由后按 to.fullPath 重新 resolve，
    // 真正不存在的路径依旧落回 catch-all 展示 404；不像旧前端 Art Design Pro
    // 那样需要「按菜单权限校验路径」，因此不会出现校验时把 404 误判为无权限
    // 而重定向首页、导致 404 页永远出不来的问题

    // 生成路由表：并发导航复用同一个进行中的 Promise（业务规则 1 & 4）
    accessInitPromise ??= initAccess(router).catch((error) => {
      // 401 已由 request.ts 的通用拦截器接管登出（doReAuthenticate 已清空
      // accessToken），不算「初始化失败」，避免重新登录后被这次失败误伤
      if (accessStore.accessToken) {
        routeInitFailed = true;
      }
      throw error;
    });

    try {
      await accessInitPromise;
    } catch (error) {
      // 允许下一次导航（如重新登录后）重新发起初始化
      accessInitPromise = null;
      console.error('[RouteGuard] 动态路由初始化失败:', error);

      // 401 场景：logout() 已经在跳登录页，这里放弃当前导航即可，不重复处理
      if (!accessStore.accessToken) {
        return false;
      }
      return { name: 'FallbackInternalError', replace: true };
    }

    const userInfo = userStore.userInfo;
    const redirectPath = (from.query.redirect ??
      (to.path === preferences.app.defaultHomePath
        ? userInfo?.homePath || preferences.app.defaultHomePath
        : to.fullPath)) as string;

    return {
      ...router.resolve(decodeURIComponent(redirectPath)),
      replace: true,
    };
  });
}

/**
 * 项目守卫配置
 * @param router
 */
function createRouterGuard(router: Router) {
  /** 通用 */
  setupCommonGuard(router);
  /** 权限访问 */
  setupAccessGuard(router);
}

export { createRouterGuard, resetAccessInitState };
