<script lang="ts" setup>
import { computed } from 'vue';

import { renderMarkdown } from './renderer';

import './markdown.css';

/**
 * markdown 只读预览
 *
 * 列表 / 详情页不值得为了渲染一段描述拉起完整的 ProseMirror（Crepe 的只读态依然会），
 * 这里走 markdown-it 直出 HTML，配合 .markdown-body 与编辑器共用同一套排版令牌。
 * 渲染与消毒的细节见 renderer.ts
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

const html = computed(() => renderMarkdown(props.content ?? ''));
</script>

<template>
  <div class="clepsydra-markdown">
    <!-- eslint-disable-next-line vue/no-v-html -- 内容已经过 markdown-it（html: false）+ DOMPurify 双重处理 -->
    <div v-if="html" class="markdown-body" v-html="html"></div>
    <div v-else class="markdown-body markdown-body--empty">{{ emptyText }}</div>
  </div>
</template>
