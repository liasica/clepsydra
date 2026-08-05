<script lang="ts" setup>
import { ref, watch } from 'vue';

import { renderMarkdown, renderMarkdownWithHighlight } from './renderer';

import './markdown.css';

/**
 * markdown 只读预览
 *
 * 列表 / 详情页不值得为了渲染一段描述拉起完整的 ProseMirror（Crepe 的只读态依然会），
 * 这里走 markdown-it 直出 HTML，配合 .markdown-body 与编辑器共用同一套排版令牌。
 * 渲染与消毒的细节见 renderer.ts
 *
 * 代码块语法高亮是异步的（按需加载语言包），采取「先同步渲染无高亮版本、高亮算好后
 * 再替换」的两段式：避免为了等高亮而让整个描述迟迟不出现，多数情况下语言包体积很小、
 * 替换发生在下一帧左右，肉眼不会有明显闪烁
 */
defineOptions({ name: 'MarkdownViewer' });

const props = withDefaults(
  defineProps<{
    /** markdown 原文 */
    content?: string;
    /** 内容为空时的占位文案 */
    emptyText?: string;
  }>(),
  {
    content: '',
    emptyText: '暂无描述',
  },
);

const html = ref('');

watch(
  () => props.content,
  (source = '') => {
    html.value = renderMarkdown(source);

    // 用 token 标记本轮渲染，异步高亮回来时内容若已经变化（竞态）则丢弃结果
    const token = source;
    void renderMarkdownWithHighlight(source).then((highlighted) => {
      if ((props.content ?? '') === token) {
        html.value = highlighted;
      }
    });
  },
  { immediate: true },
);
</script>

<template>
  <div class="clepsydra-markdown">
    <!-- eslint-disable-next-line vue/no-v-html -- 内容已经过 markdown-it（html: false）+ DOMPurify 双重处理 -->
    <div v-if="html" class="markdown-body" v-html="html"></div>
    <div v-else class="markdown-body markdown-body--empty">{{ emptyText }}</div>
  </div>
</template>
