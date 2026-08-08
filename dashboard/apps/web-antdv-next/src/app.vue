<script lang="ts" setup>
import { computed, watch } from 'vue';

import { useAntdDesignTokens } from '@vben/hooks';
import { preferences, usePreferences } from '@vben/preferences';

import { App, ConfigProvider, StyleProvider, theme } from 'antdv-next';

import { antdLocale } from '#/locales';

defineOptions({ name: 'App' });

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
