<script lang="ts" setup>
import { computed, watch } from 'vue';

import { useAntdDesignTokens } from '@vben/hooks';
import { preferences, usePreferences } from '@vben/preferences';

import { App, ConfigProvider, StyleProvider, theme } from 'antdv-next';

import { antdLocale } from '#/locales';

defineOptions({ name: 'App' });

/**
 * 预置 antdv-next 图标样式的宿主节点，规避 @layer 层序竞态。
 *
 * @antdv-next/icons 首次插入 `.anticon` 基础样式时，若渲染点还没有 layer 上下文
 * （如登录成功提示这类脱离组件树的静态渲染），会以 prepend 方式插到 head 最前；
 * 之后带 layer 上下文的图标复用同一节点原地改写成 `@layer antd { ... }`，位置却
 * 留在 head 顶部。层序由首次出现决定，antd 层从此被压到 tailwind base（preflight）
 * 之下，登录后整站 antd 组件样式失效，直到整页刷新。这里提前把同 key 的空节点
 * 追加到 head 末尾（此时 theme.css 的
 * `@layer theme, base, components, antd, utilities;` 声明已注入），图标样式后续
 * 只会命中该节点做原地更新，层序不再被翻转
 */
const iconStyleHost = document.createElement('style');
iconStyleHost.setAttribute('vc-util-key', '@ant-design-icons');
document.head.append(iconStyleHost);

const { isDark } = usePreferences();
const { tokens } = useAntdDesignTokens();

const tokenTheme = computed(() => {
  const algorithm = isDark.value
    ? [theme.darkAlgorithm]
    : [theme.defaultAlgorithm];

  // antd 紧凑模式算法
  if (preferences.app.compact) {
    algorithm.push(theme.compactAlgorithm);
  }

  return {
    algorithm,
    token: tokens,
  };
});

watch(
  tokenTheme,
  (themeConfig) => {
    ConfigProvider.config({ theme: themeConfig });
  },
  { immediate: true },
);
</script>

<template>
  <!-- layer 把 antd 的 css-in-js 样式包进 @layer antd，层序在 theme.css 中声明，
       使写在 antd 组件上的 Tailwind 工具类（mb-4 等）不再被组件自身样式压过 -->
  <StyleProvider layer>
    <ConfigProvider :locale="antdLocale" :theme="tokenTheme">
      <App>
        <RouterView />
      </App>
    </ConfigProvider>
  </StyleProvider>
</template>
