/**
 * @zh_CN 登录页面 url 地址
 */
export const LOGIN_PATH = '/auth/login';

export interface LanguageOption {
  label: string;
  value: 'en-US' | 'zh-CN';
}

/**
 * Supported languages
 *
 * 本项目只提供 zh-CN 语言包（en-US 语言文件已删除，见 T3），此处同步只保留
 * zh-CN 选项——否则偏好设置面板「通用」tab 的语言下拉仍会列出 en-US，选中后
 * 界面文案不会真正变成英文（静默回退），是验收中发现的残留问题
 */
export const SUPPORT_LANGUAGES: LanguageOption[] = [
  {
    label: '简体中文',
    value: 'zh-CN',
  },
];
