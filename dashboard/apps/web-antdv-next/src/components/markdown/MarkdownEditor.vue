<script lang="ts" setup>
import type { BlockKind, InlineMark, ToolbarState } from './toolbar-actions';

import { nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';

import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import {
  placeholder as cmPlaceholder,
  EditorView,
  keymap,
} from '@codemirror/view';
import { flip, offset, size } from '@floating-ui/dom';
import { CrepeBuilder } from '@milkdown/crepe/builder';
import { blockEdit } from '@milkdown/crepe/feature/block-edit';
import { codeMirror } from '@milkdown/crepe/feature/code-mirror';
import { cursor } from '@milkdown/crepe/feature/cursor';
import { imageBlock } from '@milkdown/crepe/feature/image-block';
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip';
import { listItem } from '@milkdown/crepe/feature/list-item';
import { placeholder as placeholderFeature } from '@milkdown/crepe/feature/placeholder';
import { table } from '@milkdown/crepe/feature/table';
import { remarkStringifyOptionsCtx } from '@milkdown/kit/core';
import { remarkPreserveEmptyLinePlugin } from '@milkdown/kit/preset/commonmark';
import { replaceAll } from '@milkdown/kit/utils';

import { uploadImage } from '#/api/upload';

import { codeExtensions, codeLanguages } from './code-mirror-setup';
import MarkdownToolbar from './MarkdownToolbar.vue';
import {
  applyLink,
  EMPTY_TOOLBAR_STATE,
  insertDivider,
  insertTable,
  readToolbarState,
  refocus,
  setBlock,
  toggleList,
  toggleMark,
} from './toolbar-actions';

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
    /** 编辑区最小高度，内容超出后自动增高，不做内部滚动 */
    height?: string;
    /** 空白时的占位提示 */
    placeholder?: string;
    /** 只读态：保留 Crepe 的排版，但禁用编辑与浮层交互 */
    readonly?: boolean;
  }>(),
  {
    height: '200px',
    placeholder: '输入 / 唤出命令菜单，或直接开始编写需求描述',
    readonly: false,
  },
);

/** markdown 原文，双向绑定 */
const content = defineModel<string>({ default: '' });

const hostRef = ref<HTMLDivElement>();

/** 工具栏按钮的点亮状态，跟随光标位置与文档变化刷新 */
const toolbarState = shallowRef<ToolbarState>(EMPTY_TOOLBAR_STATE);

/** 源码（Markdown）编辑模式：显示 CodeMirror 源码区，隐藏 Crepe 可视化编辑区 */
const sourceMode = ref(false);
const sourceHostRef = ref<HTMLDivElement>();
let sourceView: EditorView | undefined;

let crepe: CrepeBuilder | undefined;

/**
 * 记录最近一次由编辑器自身产出的 markdown。
 *
 * markdown 经过 ProseMirror 往返会被规范化（`*` 列表统一成 `-`、多余空行折叠），
 * 外部回写与内部输入靠这个快照区分，避免「输入 → emit → watch → replaceAll」自激循环
 */
let lastEmitted = '';

/** slash 菜单的浮层宿主，挂在裁切容器之外，卸载时要一并移除 */
let slashPortal: HTMLDivElement | undefined;

/**
 * 找一个不会裁掉浮层的挂载点
 *
 * 从编辑器往上定位「最外层的裁切祖先」，返回它的父级：既跳出了 overflow 裁切范围，
 * 又仍在 Modal 内部 —— 挂到 document.body 会落到 Dialog 的「外部」，点菜单项时会被
 * 判成外部点击，把整个弹窗关掉
 */
function findClipFreeRoot(el: HTMLElement): HTMLElement {
  let outermostClipper: HTMLElement | undefined;

  for (let node = el.parentElement; node && node !== document.body; ) {
    const { overflow, overflowX, overflowY } = getComputedStyle(node);
    if ([overflow, overflowX, overflowY].some((value) => value !== 'visible')) {
      outermostClipper = node;
    }
    node = node.parentElement;
  }

  return outermostClipper?.parentElement ?? document.body;
}

/**
 * 建一个浮层宿主给 slash 菜单
 *
 * Crepe 的样式全部写在 `.milkdown { ... }` 之下，菜单一旦挂到编辑器外就会掉光样式；
 * 我们的 --crepe-* 令牌映射又挂在 `.clepsydra-md-editor .milkdown` 上。所以这里原样
 * 复刻一层 `.clepsydra-md-editor > .milkdown` 结构，菜单挂进内层，样式与令牌都还在，
 * 同时又待在裁切容器之外，不会被 Modal 切掉
 */
function createSlashPortal(host: HTMLElement): HTMLElement {
  const portal = document.createElement('div');
  portal.className =
    'clepsydra-markdown clepsydra-md-editor clepsydra-md-portal';

  const inner = document.createElement('div');
  inner.className = 'milkdown';
  portal.append(inner);

  findClipFreeRoot(host).append(portal);
  slashPortal = portal;
  return inner;
}

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
    .addFeature(imageBlock, {
      onUpload: uploadImage,
      blockUploadButton: '上传图片',
      blockUploadPlaceholderText: '或粘贴图片链接',
      blockConfirmButton: '确定',
      blockCaptionPlaceholderText: '写点图注……',
      inlineUploadButton: '上传图片',
      inlineUploadPlaceholderText: '或粘贴图片链接',
    })
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
    // 不装 Crepe 的 toolbar feature：它是跟随选区的浮动条，会盖住顶部常驻工具栏，
    // 且能力已被常驻工具栏完全覆盖
    // blockEdit 的 slash 菜单会读取已注册的 feature 列表决定显示哪些条目，放在最后装配
    .addFeature(blockEdit, {
      // 左侧块拖拽手柄要占掉 66px 正文缩进，正文会明显比同表单的 antd Input 靠右；
      // 表单场景下块排序是低频操作，关掉手柄换取与表单对齐的左边界，加块仍走 slash 菜单
      blockHandle: { shouldShow: () => false },
      slashMenu: {
        // 挂到弹窗的裁切范围之外，菜单才能用满视口高度而不是被压成一两行
        root: createSlashPortal(hostRef.value),
        /**
         * floatingUIOptions 会整体覆盖 plugin-slash 内部的 middleware 链，这里重建：
         * - flip 换成 bestFit，上下都不够时至少挑空间大的一侧
         * - size 按最终方向的可用高度下发 --md-slash-max-height，超出部分菜单内部滚动
         */
        floatingUIOptions: {
          placement: 'bottom-start',
          middleware: [
            flip({ fallbackStrategy: 'bestFit', padding: 8 }),
            offset(6),
            size({
              padding: 8,
              apply({ availableHeight, elements }) {
                // 76px 是顶部标签栏 + 上下内边距，剩下的才是可滚动的列表区
                const listHeight = Math.max(96, availableHeight - 76);
                elements.floating.style.setProperty(
                  '--md-slash-max-height',
                  `${listHeight}px`,
                );
              },
            }),
          ],
        },
      },
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
        image: { label: '图片' },
        codeBlock: { label: '代码块' },
        table: { label: '表格' },
        // 公式对应的 latex feature 未启用，从菜单里摘掉
        math: null,
      },
    });

  /**
   * 关掉「空段落保留成 `<br />`」
   *
   * commonmark preset 默认注册 remarkPreserveEmptyLinePlugin，空段落只要不是文档
   * 末节点就会被序列化成字面量 `<br />`。粘贴图片时 plugin-upload 是就地插入、不吃掉
   * 光标所在的空段落，于是描述里常留下一行 `<br />`；而 MarkdownViewer 关闭了 html
   * 渲染，详情页会把它原样显示成文本。段落间距靠 markdown 的空行本来就够用
   */
  await instance.editor.remove(remarkPreserveEmptyLinePlugin);

  instance.on((listener) => {
    listener.markdownUpdated((ctx, markdown) => {
      // 输入过程中格式也会变（如打完 `**粗**`），跟着刷新工具栏点亮状态
      toolbarState.value = readToolbarState(ctx);

      if (markdown === content.value) {
        return;
      }
      lastEmitted = markdown;
      content.value = markdown;
    });

    listener.selectionUpdated((ctx) => {
      toolbarState.value = readToolbarState(ctx);
    });
  });

  instance.setReadonly(props.readonly);

  await instance.create();
  crepe = instance;
});

onBeforeUnmount(() => {
  sourceView?.destroy();
  sourceView = undefined;
  crepe?.destroy();
  crepe = undefined;
  // 浮层宿主是手动挂到编辑器外面的，Crepe 销毁不会带走它
  slashPortal?.remove();
  slashPortal = undefined;
});

// 外部回写（表单异步加载详情等）时把整篇内容换掉
watch(content, (next = '') => {
  if (next === lastEmitted) {
    return;
  }
  lastEmitted = next;
  if (sourceMode.value && sourceView) {
    sourceView.dispatch({
      changes: { from: 0, to: sourceView.state.doc.length, insert: next },
    });
    return;
  }
  crepe?.editor.action(replaceAll(next));
});

watch(
  () => props.readonly,
  (value) => {
    // 只读态没有源码入口，正处于源码模式时先把改动写回可视化编辑器
    if (value && sourceMode.value) {
      exitSourceMode();
    }
    crepe?.setReadonly(value);
  },
);

/**
 * 进入源码模式：取 Crepe 当前的 markdown 原文，就地建一个 CodeMirror 编辑器。
 * markdown 语言包与代码块的语言表一样按需加载，首次切换才拉取对应 chunk
 */
async function enterSourceMode() {
  if (!crepe || props.readonly) {
    return;
  }
  sourceMode.value = true;
  await nextTick();
  const { markdown } = await import('@codemirror/lang-markdown');
  // 等待语言包期间可能已被切回或卸载
  if (!sourceMode.value || !sourceHostRef.value || sourceView) {
    return;
  }
  sourceView = new EditorView({
    parent: sourceHostRef.value,
    doc: crepe.getMarkdown(),
    extensions: [
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      EditorView.lineWrapping,
      cmPlaceholder('编写 Markdown 源码'),
      markdown(),
      // 复用代码块的高亮样式（--md-code-* 调色板），亮暗切换由 CSS 变量完成
      ...codeExtensions,
      EditorView.updateListener.of((update) => {
        if (!update.docChanged) {
          return;
        }
        // 源码模式下 v-model 跟随每次输入，表单提交时拿到的始终是最新原文
        const next = update.state.doc.toString();
        lastEmitted = next;
        if (next !== content.value) {
          content.value = next;
        }
      }),
    ],
  });
  sourceView.focus();
}

/** 退出源码模式：销毁 CodeMirror，把源码整篇写回 Crepe */
function exitSourceMode() {
  const text = sourceView?.state.doc.toString();
  sourceView?.destroy();
  sourceView = undefined;
  sourceMode.value = false;
  if (crepe && text !== undefined) {
    // 写回后 markdownUpdated 会带出规范化后的 markdown，同步给 v-model
    crepe.editor.action(replaceAll(text));
  }
}

function toggleSourceMode() {
  if (sourceMode.value) {
    exitSourceMode();
  } else {
    void enterSourceMode();
  }
}

/**
 * 工具栏动作统一入口
 *
 * 每个动作都是「执行命令 → 把焦点还给正文 → 重算点亮状态」，抽出来避免在每个
 * 事件处理里重复这三步
 */
function runAction(action: (ctx: Parameters<typeof refocus>[0]) => void) {
  if (!crepe || props.readonly || sourceMode.value) {
    return;
  }
  crepe.editor.action((ctx) => {
    action(ctx);
    refocus(ctx);
    toolbarState.value = readToolbarState(ctx);
  });
}

defineExpose({
  /** 取当前 markdown 原文（提交前兜底用，正常走 v-model 即可） */
  getMarkdown: () =>
    sourceMode.value && sourceView
      ? sourceView.state.doc.toString()
      : (crepe?.getMarkdown() ?? content.value ?? ''),
});
</script>

<template>
  <div
    class="clepsydra-markdown clepsydra-md-editor"
    :class="{ 'clepsydra-md-editor--readonly': readonly }"
    :style="{ '--md-editor-min-height': height }"
  >
    <MarkdownToolbar
      v-if="!readonly"
      :source-mode="sourceMode"
      :state="toolbarState"
      @divider="runAction(insertDivider)"
      @link="(href: string) => runAction((ctx) => applyLink(ctx, href))"
      @list="(kind) => runAction((ctx) => toggleList(ctx, kind))"
      @mark="(mark: InlineMark) => runAction((ctx) => toggleMark(ctx, mark))"
      @set-block="
        (kind: Exclude<BlockKind, 'other'>) =>
          runAction((ctx) => setBlock(ctx, kind))
      "
      @table="runAction(insertTable)"
      @toggle-source="toggleSourceMode"
    />
    <!-- Crepe 实例保持常驻，切到源码模式时仅隐藏，避免反复重建 -->
    <div v-show="!sourceMode" ref="hostRef"></div>
    <div v-show="sourceMode" ref="sourceHostRef" class="clepsydra-md-source"></div>
  </div>
</template>
