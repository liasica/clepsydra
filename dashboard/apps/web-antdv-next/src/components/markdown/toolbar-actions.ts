/**
 * 常驻工具栏的命令层
 *
 * 这里只负责「怎么改文档」与「当前光标处于什么状态」，不含任何 UI。
 * MarkdownToolbar.vue 拿到这些纯函数后只管画按钮，两者可以各自独立修改
 */
import type { Ctx } from '@milkdown/kit/ctx';
import type { MarkType, NodeType } from '@milkdown/kit/prose/model';
import type { EditorState } from '@milkdown/kit/prose/state';

import { commandsCtx, editorViewCtx } from '@milkdown/kit/core';
import {
  blockquoteSchema,
  codeBlockSchema,
  createCodeBlockCommand,
  emphasisSchema,
  headingSchema,
  inlineCodeSchema,
  insertHrCommand,
  listItemSchema,
  strongSchema,
  toggleEmphasisCommand,
  toggleInlineCodeCommand,
  toggleLinkCommand,
  toggleStrongCommand,
  turnIntoTextCommand,
  wrapInBlockquoteCommand,
  wrapInBlockTypeCommand,
  wrapInBulletListCommand,
  wrapInHeadingCommand,
  wrapInOrderedListCommand,
} from '@milkdown/kit/preset/commonmark';
import {
  insertTableCommand,
  strikethroughSchema,
  toggleStrikethroughCommand,
} from '@milkdown/kit/preset/gfm';

/** 工具栏关心的行内标记 */
export type InlineMark = 'emphasis' | 'inlineCode' | 'strikethrough' | 'strong';

/** 工具栏关心的块类型，`other` 表示表格 / 列表等不在下拉里的块 */
export type BlockKind =
  | 'blockquote'
  | 'codeBlock'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'other'
  | 'paragraph';

/** 光标所在位置的格式状态，用于点亮工具栏按钮 */
export interface ToolbarState {
  block: BlockKind;
  marks: Record<InlineMark, boolean>;
  /** 选区为空（只有光标）：链接按钮没有可套用的文字，需要置灰 */
  selectionEmpty: boolean;
}

export const EMPTY_TOOLBAR_STATE: ToolbarState = {
  block: 'paragraph',
  marks: {
    emphasis: false,
    inlineCode: false,
    strong: false,
    strikethrough: false,
  },
  selectionEmpty: true,
};

/**
 * 选区是否命中某个 mark
 *
 * 光标态（empty）看的是「下一次输入会带上的标记」，即 storedMarks；有选区时才去问
 * 文档该区间是否整体带这个标记
 */
function isMarkActive(state: EditorState, type: MarkType): boolean {
  const { $from, empty, from, to } = state.selection;
  return empty
    ? !!type.isInSet(state.storedMarks || $from.marks())
    : state.doc.rangeHasMark(from, to, type);
}

/** 光标所在的最内层块节点 */
function currentBlock(state: EditorState) {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    const node = $from.node(depth);
    if (node.isBlock && node.type.name !== 'doc') {
      return node;
    }
  }
  return null;
}

function resolveBlockKind(state: EditorState, ctx: Ctx): BlockKind {
  const node = currentBlock(state);
  if (!node) {
    return 'paragraph';
  }

  const name = node.type.name;
  if (name === headingSchema.type(ctx).name) {
    const level = Number(node.attrs.level) || 1;
    // 下拉里只放到 H3，更深的层级按「其它」处理，避免下拉标签出现没有的选项
    return level <= 3 ? (`h${level}` as BlockKind) : 'other';
  }
  if (name === codeBlockSchema.type(ctx).name) {
    return 'codeBlock';
  }
  if (name === 'paragraph') {
    // 段落可能被 blockquote 包着，往外看一层决定显示「引用」还是「正文」
    const { $from } = state.selection;
    for (let depth = $from.depth - 1; depth > 0; depth--) {
      const parent = $from.node(depth);
      if (parent.type.name === blockquoteSchema.type(ctx).name) {
        return 'blockquote';
      }
      if (parent.type.name === listItemSchema.type(ctx).name) {
        return 'other';
      }
    }
    return 'paragraph';
  }
  return 'other';
}

/** 读取当前格式状态；编辑器尚未就绪时回落到空状态 */
export function readToolbarState(ctx: Ctx): ToolbarState {
  const view = ctx.get(editorViewCtx);
  const { state } = view;

  // Milkdown 在 EditorView 构造完成后才注入真实 view；构造期间 NodeView（如列表项
  // 序号同步）派发的事务会同步触发 listener 走到这里，此时 ctx 里还是占位对象。
  // 若在这里抛错，异常会沿 applyTransaction 掀翻编辑器初始化，编辑器从此无响应
  if (!state) {
    return EMPTY_TOOLBAR_STATE;
  }

  return {
    block: resolveBlockKind(state, ctx),
    marks: {
      emphasis: isMarkActive(state, emphasisSchema.type(ctx)),
      inlineCode: isMarkActive(state, inlineCodeSchema.type(ctx)),
      strong: isMarkActive(state, strongSchema.type(ctx)),
      strikethrough: isMarkActive(state, strikethroughSchema.type(ctx)),
    },
    selectionEmpty: state.selection.empty,
  };
}

/** 把某个块类型包裹到当前块上 */
function wrapInBlockType(ctx: Ctx, nodeType: NodeType, attrs?: object) {
  ctx.get(commandsCtx).call(wrapInBlockTypeCommand.key, { attrs, nodeType });
}

/** 行内标记开关 */
export function toggleMark(ctx: Ctx, mark: InlineMark) {
  const commands = ctx.get(commandsCtx);
  const keyMap = {
    emphasis: toggleEmphasisCommand.key,
    inlineCode: toggleInlineCodeCommand.key,
    strong: toggleStrongCommand.key,
    strikethrough: toggleStrikethroughCommand.key,
  } as const;
  commands.call(keyMap[mark]);
}

/**
 * 切换块类型
 *
 * 先 turnIntoText 回到段落再套目标类型，否则 H2 → 引用之类的跨类型切换会失败
 */
export function setBlock(ctx: Ctx, kind: Exclude<BlockKind, 'other'>) {
  const commands = ctx.get(commandsCtx);

  if (kind === 'paragraph') {
    commands.call(turnIntoTextCommand.key);
    return;
  }

  commands.call(turnIntoTextCommand.key);

  switch (kind) {
    case 'blockquote': {
      commands.call(wrapInBlockquoteCommand.key);
      break;
    }
    case 'codeBlock': {
      commands.call(createCodeBlockCommand.key);
      break;
    }
    default: {
      commands.call(wrapInHeadingCommand.key, Number(kind.slice(1)));
    }
  }
}

/** 无序 / 有序 / 任务列表 */
export function toggleList(ctx: Ctx, kind: 'bullet' | 'ordered' | 'task') {
  const commands = ctx.get(commandsCtx);

  if (kind === 'bullet') {
    commands.call(wrapInBulletListCommand.key);
    return;
  }
  if (kind === 'ordered') {
    commands.call(wrapInOrderedListCommand.key);
    return;
  }
  // 任务列表没有独立命令，按 Crepe slash 菜单的做法包一个带 checked 属性的 listItem
  wrapInBlockType(ctx, listItemSchema.type(ctx), { checked: false });
}

/** 给选中文字套上链接 */
export function applyLink(ctx: Ctx, href: string) {
  ctx.get(commandsCtx).call(toggleLinkCommand.key, { href });
}

/** 当前选区是否为空：为空时「链接」按钮无处可套，需要禁用 */
export function isSelectionEmpty(ctx: Ctx): boolean {
  return ctx.get(editorViewCtx).state.selection.empty;
}

/** 插入分割线 */
export function insertDivider(ctx: Ctx) {
  ctx.get(commandsCtx).call(insertHrCommand.key);
}

/** 插入 3x3 表格（含表头行） */
export function insertTable(ctx: Ctx) {
  ctx.get(commandsCtx).call(insertTableCommand.key);
}

/** 命令执行后把焦点还给编辑器，否则点完按钮光标会留在按钮上 */
export function refocus(ctx: Ctx) {
  ctx.get(editorViewCtx).focus();
}
