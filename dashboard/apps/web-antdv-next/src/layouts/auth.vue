<script lang="ts" setup>
import { computed } from 'vue';

import { AuthPageLayout } from '@vben/layouts';
import { preferences } from '@vben/preferences';

import { BrandWordmark } from '#/components/brand';
import { $t } from '#/locales';

const appName = computed(() => preferences.app.name);
const logo = computed(() => preferences.logo.source);
</script>

<template>
  <AuthPageLayout
    :app-name="appName"
    :page-description="$t('authentication.pageDesc')"
    :page-title="$t('authentication.pageTitle')"
  >
    <!-- 左上角品牌区：图标 + 字标组合成完整 lockup，尺寸比例与 assets/logo-lockup.svg 一致（64:40） -->
    <template #logo>
      <div class="absolute top-0 left-0 z-10 flex flex-1">
        <div class="text-foreground mt-4 ml-4 flex flex-1 items-center">
          <img :alt="appName" :src="logo" class="mr-2" width="42" />
          <BrandWordmark class="h-[26px]" />
        </div>
      </div>
    </template>
    <!-- 自定义工具栏 -->
    <!-- <template #toolbar></template> -->
  </AuthPageLayout>
</template>
