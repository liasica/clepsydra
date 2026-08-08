<script lang="ts" setup>
import type { BlockKind, InlineMark, ToolbarState } from './toolbar-actions';

import { computed, ref } from 'vue';

import {
  Bold,
  ChevronDown,
  Code,
  Italic,
  Link2,
  List,
  ListOrdered,
  ListTodo,
  Minus,
  SquareCode,
  Strikethrough,
  Table,
  TextQuote,
} from '@vben/icons';

import { Button, Dropdown, Input, Popover, Tooltip } from 'antdv-next';

/**
 * markdown 编辑器的常驻工具栏
 *
 * 纯展示层：状态由 MarkdownEditor 通过 `state` 传入，点击只往外抛事件，
 * 具体命令由 toolbar-actions.ts 执行
 */
defineOptions({ name: 'MarkdownToolbar' });

const props = defineProps<{
  /** 当前光标处的格式状态，用于点亮按钮 */
  state: ToolbarState;
}>();

const emit = defineEmits<{
  divider: [];
  link: [href: string];
  list: [kind: 'bullet' | 'ordered' | 'task'];
  mark: [mark: InlineMark];
  setBlock: [kind: Exclude<BlockKind, 'other'>];
  table: [];
}>();

const rootRef = ref<HTMLDivElement>();

/**
 * 下拉 / 气泡挂到工具栏自身而不是 body
 *
 * 编辑器常常放在 Modal 里，body 上的浮层属于 Dialog 的「外部」，click 触发的
 * 弹层会被 Dialog 的关闭层直接吞掉，表现为点了没反应
 */
function getPopupContainer(): HTMLElement {
  return rootRef.value ?? document.body;
}

/** 链接输入浮层 */
const linkOpen = ref(false);
const linkHref = ref('');

function submitLink() {
  const href = linkHref.value.trim();
  if (!href) {
    return;
  }
  emit('link', href);
  linkHref.value = '';
  linkOpen.value = false;
}

/** 块类型下拉的选项，label 同时用作按钮上的当前态文案 */
const BLOCK_OPTIONS: { key: Exclude<BlockKind, 'other'>; label: string }[] = [
  { key: 'paragraph', label: '正文' },
  { key: 'h1', label: '标题 1' },
  { key: 'h2', label: '标题 2' },
  { key: 'h3', label: '标题 3' },
  { key: 'blockquote', label: '引用' },
  { key: 'codeBlock', label: '代码块' },
];

/** 行内标记按钮：图标 + 提示文案 + 快捷键说明 */
const MARK_BUTTONS: {
  hint: string;
  icon: any;
  key: InlineMark;
  label: string;
}[] = [
  { hint: '⌘B', icon: Bold, key: 'strong', label: '加粗' },
  { hint: '⌘I', icon: Italic, key: 'emphasis', label: '斜体' },
  {
    hint: '⌘⇧X',
    icon: Strikethrough,
    key: 'strikethrough',
    label: '删除线',
  },
  { hint: '⌘E', icon: Code, key: 'inlineCode', label: '行内代码' },
];

const LIST_BUTTONS: {
  icon: any;
  key: 'bullet' | 'ordered' | 'task';
  label: string;
}[] = [
  { icon: List, key: 'bullet', label: '无序列表' },
  { icon: ListOrdered, key: 'ordered', label: '有序列表' },
  { icon: ListTodo, key: 'task', label: '任务列表' },
];

/** 下拉按钮上显示的当前块类型；落在列表 / 表格里时显示占位短横 */
const blockLabel = computed(
  () =>
    BLOCK_OPTIONS.find((item) => item.key === props.state.block)?.label ?? '—',
);

/** 用 menu 配置式而不是 <Menu><Menu.Item>：后者在 antdv-next 下渲染不出内容 */
const blockMenu = computed(() => ({
  items: BLOCK_OPTIONS.map((item) => ({ key: item.key, label: item.label })),
  onClick: ({ key }: { key: number | string }) => {
    emit('setBlock', key as Exclude<BlockKind, 'other'>);
  },
  selectedKeys: [props.state.block],
}));
</script>

<template>
  <div ref="rootRef" class="md-toolbar">
    <Dropdown
      :get-popup-container="getPopupContainer"
      :menu="blockMenu"
      :trigger="['click']"
    >
      <button class="md-toolbar__block" type="button">
        <span>{{ blockLabel }}</span>
        <ChevronDown class="md-toolbar__caret" />
      </button>
    </Dropdown>

    <span class="md-toolbar__divider"></span>

    <Tooltip
      v-for="item in MARK_BUTTONS"
      :key="item.key"
      :mouse-enter-delay="0.4"
    >
      <template #title>{{ item.label }} {{ item.hint }}</template>
      <button
        class="md-toolbar__btn"
        :class="{ 'md-toolbar__btn--active': state.marks[item.key] }"
        type="button"
        @click="emit('mark', item.key)"
      >
        <component :is="item.icon" />
      </button>
    </Tooltip>

    <Popover
      v-model:open="linkOpen"
      :disabled="state.selectionEmpty"
      :get-popup-container="getPopupContainer"
      placement="bottom"
      trigger="click"
    >
      <template #content>
        <div class="md-toolbar__link">
          <Input
            v-model:value="linkHref"
            placeholder="https://"
            size="small"
            @press-enter="submitLink"
          />
          <Button size="small" type="primary" @click="submitLink">确定</Button>
        </div>
      </template>
      <Tooltip :mouse-enter-delay="0.4">
        <template #title>
          {{ state.selectionEmpty ? '先选中要加链接的文字' : '链接' }}
        </template>
        <button
          class="md-toolbar__btn"
          :class="{ 'md-toolbar__btn--disabled': state.selectionEmpty }"
          type="button"
        >
          <Link2 />
        </button>
      </Tooltip>
    </Popover>

    <span class="md-toolbar__divider"></span>

    <Tooltip
      v-for="item in LIST_BUTTONS"
      :key="item.key"
      :mouse-enter-delay="0.4"
    >
      <template #title>{{ item.label }}</template>
      <button
        class="md-toolbar__btn"
        type="button"
        @click="emit('list', item.key)"
      >
        <component :is="item.icon" />
      </button>
    </Tooltip>

    <span class="md-toolbar__divider"></span>

    <Tooltip :mouse-enter-delay="0.4">
      <template #title>引用</template>
      <button
        class="md-toolbar__btn"
        :class="{ 'md-toolbar__btn--active': state.block === 'blockquote' }"
        type="button"
        @click="emit('setBlock', 'blockquote')"
      >
        <TextQuote />
      </button>
    </Tooltip>

    <Tooltip :mouse-enter-delay="0.4">
      <template #title>代码块</template>
      <button
        class="md-toolbar__btn"
        :class="{ 'md-toolbar__btn--active': state.block === 'codeBlock' }"
        type="button"
        @click="emit('setBlock', 'codeBlock')"
      >
        <SquareCode />
      </button>
    </Tooltip>

    <Tooltip :mouse-enter-delay="0.4">
      <template #title>表格</template>
      <button class="md-toolbar__btn" type="button" @click="emit('table')">
        <Table />
      </button>
    </Tooltip>

    <Tooltip :mouse-enter-delay="0.4">
      <template #title>分割线</template>
      <button class="md-toolbar__btn" type="button" @click="emit('divider')">
        <Minus />
      </button>
    </Tooltip>
  </div>
</template>

<style scoped>
/* 工具栏是编辑器外框内的一条 header，与正文之间用与外框同色的细线分隔 */
.md-toolbar {
  position: relative; /* 作为下拉 / 气泡的定位上下文，见 getPopupContainer */
  display: flex;
  flex-wrap: wrap;
  gap: 1px;
  align-items: center;
  padding: 4px 6px;
  border-bottom: 1px solid var(--ant-color-border, hsl(var(--border)));
}

.md-toolbar__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  color: var(--ant-color-text-secondary, hsl(var(--muted-foreground)));
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: 6px;
  transition:
    background-color 0.15s,
    color 0.15s;
}

.md-toolbar__btn:hover {
  color: var(--ant-color-text, hsl(var(--foreground)));
  background: var(--ant-control-item-bg-hover, hsl(var(--accent)));
}

/* 特异度要压过上面的 :hover，否则鼠标停在按钮上时点亮态会被 hover 底色盖掉 */
.md-toolbar__btn.md-toolbar__btn--active,
.md-toolbar__btn.md-toolbar__btn--active:hover {
  color: var(--ant-color-primary, hsl(var(--primary)));
  background: color-mix(
    in srgb,
    var(--ant-color-primary, hsl(var(--primary))) 16%,
    transparent
  );
}

.md-toolbar__btn--disabled {
  color: var(--ant-color-text-quaternary, hsl(var(--muted-foreground) / 50%));
  cursor: not-allowed;
}

.md-toolbar__btn--disabled:hover {
  color: var(--ant-color-text-quaternary, hsl(var(--muted-foreground) / 50%));
  background: transparent;
}

.md-toolbar__btn :deep(svg) {
  width: 16px;
  height: 16px;
}

.md-toolbar__link {
  display: flex;
  gap: 6px;
  align-items: center;
  width: 260px;
}

/* 块类型下拉：做成一个带文字的矮按钮，宽度固定避免切换层级时工具栏抖动 */
.md-toolbar__block {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  justify-content: space-between;
  width: 76px;
  height: 28px;
  padding: 0 6px 0 8px;
  font-size: 13px;
  color: var(--ant-color-text, hsl(var(--foreground)));
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: 6px;
  transition: background-color 0.15s;
}

.md-toolbar__block:hover {
  background: var(--ant-control-item-bg-hover, hsl(var(--accent)));
}

.md-toolbar__caret {
  width: 14px;
  height: 14px;
  color: var(--ant-color-text-quaternary, hsl(var(--muted-foreground)));
}

.md-toolbar__divider {
  width: 1px;
  height: 16px;
  margin: 0 5px;
  background: var(--ant-color-border, hsl(var(--border)));
}
</style>
