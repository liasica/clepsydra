<script lang="ts" setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';

import { CrepeBuilder } from '@milkdown/crepe/builder';
import { blockEdit } from '@milkdown/crepe/feature/block-edit';
import { codeMirror } from '@milkdown/crepe/feature/code-mirror';
import { cursor } from '@milkdown/crepe/feature/cursor';
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip';
import { listItem } from '@milkdown/crepe/feature/list-item';
import { placeholder as placeholderFeature } from '@milkdown/crepe/feature/placeholder';
import { table } from '@milkdown/crepe/feature/table';
import { toolbar } from '@milkdown/crepe/feature/toolbar';
import { remarkStringifyOptionsCtx } from '@milkdown/kit/core';
import { replaceAll } from '@milkdown/kit/utils';

import { codeExtensions, codeLanguages } from './code-mirror-setup';

import '@milkdown/crepe/theme/common/style.css';
import '@milkdown/crepe/theme/frame.css';
import './markdown.css';
import './crepe-theme.css';

/**
 * Notion 风所见即所得 markdown 编辑器
 *
 * 基于 @milkdown/crepe 的 builder API 逐个装配 feature —— 聚合入口 `@milkdown/crepe`
 * 会静态 import katex / language-data / AI 等全部依赖，运行时的 `features: { ... : false }`
 * 关不掉打包体积，只有 builder + 子路径 feature 才能真正把 Latex、图片上传、TopBar、AI
 * 摘出去。
 *
 * 体积较大（ProseMirror + CodeMirror），使用方必须通过 components/markdown/index.ts
 * 暴露的异步组件引入，避免进入主 chunk
 */
defineOptions({ name: 'MarkdownEditor' });

const props = withDefaults(
  defineProps<{
    /** 编辑区最小高度，内容超出后自动增高（不做内部滚动，避免浮层被裁切） */
    height?: string;
    /** 空白时的占位提示 */
    placeholder?: string;
    /** 只读态：保留 Crepe 的排版，但禁用编辑与浮层交互 */
    readonly?: boolean;
  }>(),
  {
    height: '320px',
    placeholder: '输入 / 唤出命令菜单，或直接开始编写需求描述',
    readonly: false,
  },
);

/** markdown 原文，双向绑定 */
const content = defineModel<string>({ default: '' });

const hostRef = ref<HTMLDivElement>();

let crepe: CrepeBuilder | undefined;

/**
 * 记录最近一次由编辑器自身产出的 markdown。
 *
 * markdown 经过 ProseMirror 往返会被规范化（`*` 列表统一成 `-`、多余空行折叠），
 * 外部回写与内部输入靠这个快照区分，避免「输入 → emit → watch → replaceAll」自激循环
 */
let lastEmitted = '';

onMounted(async () => {
  if (!hostRef.value) {
    return;
  }

  const instance = new CrepeBuilder({
    root: hostRef.value,
    defaultValue: content.value ?? '',
  });

  lastEmitted = content.value ?? '';

  // remark 序列化默认用 `*` 做无序列表符号、`***` 做分割线，改成更常见的 `-` 与 `---`，
  // 减少同一份内容在编辑器里进出一趟后产生的无谓 diff
  instance.editor.config((ctx) => {
    ctx.update(remarkStringifyOptionsCtx, (prev) => ({
      ...prev,
      bullet: '-' as const,
      rule: '-' as const,
    }));
  });

  instance
    .addFeature(cursor)
    .addFeature(listItem)
    .addFeature(placeholderFeature, { text: props.placeholder, mode: 'block' })
    .addFeature(linkTooltip, { inputPlaceholder: '粘贴或输入链接' })
    .addFeature(table)
    .addFeature(codeMirror, {
      languages: codeLanguages,
      extensions: codeExtensions,
      searchPlaceholder: '搜索语言',
      noResultText: '没有匹配的语言',
      copyText: '复制',
      previewLabel: '预览',
      previewToggleText: (previewOnlyMode: boolean) =>
        previewOnlyMode ? '编辑' : '隐藏',
    })
    .addFeature(toolbar, {
      boldLabel: '加粗',
      italicLabel: '斜体',
      strikethroughLabel: '删除线',
      codeLabel: '行内代码',
      linkLabel: '链接',
    })
    // blockEdit 的 slash 菜单会读取已注册的 feature 列表决定显示哪些条目，放在最后装配
    .addFeature(blockEdit, {
      textGroup: {
        label: '文本',
        text: { label: '正文' },
        h1: { label: '标题 1' },
        h2: { label: '标题 2' },
        h3: { label: '标题 3' },
        h4: { label: '标题 4' },
        h5: { label: '标题 5' },
        h6: { label: '标题 6' },
        quote: { label: '引用' },
        divider: { label: '分割线' },
      },
      listGroup: {
        label: '列表',
        bulletList: { label: '无序列表' },
        orderedList: { label: '有序列表' },
        taskList: { label: '任务列表' },
      },
      advancedGroup: {
        label: '高级',
        codeBlock: { label: '代码块' },
        table: { label: '表格' },
        // 图片与公式对应的 feature 未启用，这里一并从菜单里摘掉
        image: null,
        math: null,
      },
    });

  instance.on((listener) => {
    listener.markdownUpdated((_ctx, markdown) => {
      if (markdown === content.value) {
        return;
      }
      lastEmitted = markdown;
      content.value = markdown;
    });
  });

  instance.setReadonly(props.readonly);

  await instance.create();
  crepe = instance;
});

onBeforeUnmount(() => {
  crepe?.destroy();
  crepe = undefined;
});

// 外部回写（表单异步加载详情等）时把整篇内容换掉
watch(content, (next = '') => {
  if (!crepe || next === lastEmitted) {
    return;
  }
  lastEmitted = next;
  crepe.editor.action(replaceAll(next));
});

watch(
  () => props.readonly,
  (value) => {
    crepe?.setReadonly(value);
  },
);

defineExpose({
  /** 取当前 markdown 原文（提交前兜底用，正常走 v-model 即可） */
  getMarkdown: () => crepe?.getMarkdown() ?? content.value ?? '',
});
</script>

<template>
  <div
    class="clepsydra-markdown clepsydra-md-editor"
    :class="{ 'clepsydra-md-editor--readonly': readonly }"
    :style="{ '--md-editor-min-height': height }"
  >
    <div ref="hostRef"></div>
  </div>
</template>
