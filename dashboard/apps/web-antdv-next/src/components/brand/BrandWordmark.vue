<script setup lang="ts">
/**
 * Clepsydra 字标，取自 assets/logo-lockup.svg 的文字部分
 *
 * 以内联 SVG 渲染而非 img 引用整张 lockup 的原因：
 * - fill 使用 currentColor 才能跟随暗色主题变色（lockup 原文件文字为固定深色）
 * - 与图标（preferences.logo.source）分离，侧边栏收起时可只保留图标
 *
 * 显示尺寸由外部 class 控制（如 h-[22px]），宽度按 viewBox 比例自适应
 */
defineOptions({
  name: 'BrandWordmark',
});
</script>

<template>
  <svg
    aria-label="Clepsydra"
    class="w-auto overflow-visible"
    role="img"
    viewBox="0 0 208 44"
    xmlns="http://www.w3.org/2000/svg"
  >
    <text
      dominant-baseline="central"
      fill="currentColor"
      font-family="'Space Grotesk', 'Plus Jakarta Sans', ui-sans-serif, sans-serif"
      font-size="40"
      font-weight="700"
      letter-spacing="-0.8"
      x="0"
      y="22"
    >
      Clepsydra
    </text>
  </svg>
</template>
