import { defineOverridesPreferences } from '@vben/preferences';

/**
 * @description 项目配置文件
 * 只需要覆盖项目中的一部分配置，不需要的配置不用覆盖，会自动使用默认配置
 * !!! 更改配置后请清空缓存，否则可能不生效
 */
export const overridesPreferences = defineOverridesPreferences({
  // overrides
  app: {
    name: import.meta.env.VITE_APP_TITLE,
    // 前端角色路由过滤（admin / client 两个角色），不走后端下发菜单
    accessMode: 'frontend',
    // 默认首页改为本项目的工作台路由（vben 默认值 /analytics 是脚手架示例页，已删除）
    defaultHomePath: '/dashboard',
    // 只提供 zh-CN 语言包，锁定默认语言避免随 vben 升级漂移
    locale: 'zh-CN',
  },
  logo: {
    // 替换 vben 默认的 unpkg 远程 logo，使用项目自身 logo（public/logo.svg）
    source: '/logo.svg',
  },
  theme: {
    // 默认亮色，主题色与暗色切换开关保留，暗色可手动切换
    mode: 'light',
  },
  widget: {
    // 只有 zh-CN 一种语言，关闭语言切换部件
    languageToggle: false,
    // 后端未提供通知接口，关闭通知部件，避免展示 vben 脚手架的示例通知数据
    notification: false,
  },
  copyright: {
    // 登录页版权栏默认写死 Vben 官网外链，替换为项目自身信息
    companyName: 'Clepsydra',
    companySiteLink: '',
    date: `${new Date().getFullYear()}`,
  },
});
