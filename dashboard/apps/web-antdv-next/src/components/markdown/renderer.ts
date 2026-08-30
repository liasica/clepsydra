import type { MarkdownIt as MarkdownItType } from 'markdown-it';

import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';

/**
 * 只读预览的 markdown 渲染管线
 *
 * 放在模块作用域而不是组件里，列表页渲染多条描述时共用同一个 parser 实例。
 *
 * 安全前提：需求方角色也能编写描述，属于不可信输入，因此
 * 1. markdown-it 关闭 html（`html: false`），源文里的裸 HTML 一律按纯文本转义；
 * 2. 渲染结果再过一遍 DOMPurify，防御 markdown-it 自身可能的解析缺陷
 */

/** 渲染选项 */
export interface RenderOptions {
  /**
   * 任务列表的勾选框是否可点击
   *
   * 关闭时（默认）复选框渲染为 disabled，纯展示；开启后由外层监听 change 事件落库，
   * 见 MarkdownViewer 的 toggle-task 与 toggleTaskLine
   */
  interactive?: boolean;
}

/**
 * 传给 markdown-it 渲染规则的运行时上下文
 *
 * markdown-it 的 env 是自由字典（索引签名），渲染规则从里面取本轮渲染的开关与预计算结果
 */
interface RenderEnv extends RenderOptions {
  [key: string | symbol]: unknown;
  highlightMap?: HighlightMap;
}

/** 任务列表项所在的原文行号，勾选时靠它定位要改写的那一行 */
const TASK_LINE_ATTR = 'data-task-line';

/** 任务列表项的标记类 */
const TASK_ITEM_CLASS = 'task-list-item';
const TASK_ITEM_CHECKED_CLASS = 'task-list-item-checked';

/**
 * 任务列表支持
 *
 * markdown-it 本体不认 GFM 的 `- [ ]`，而编辑器侧（remark-gfm）是支持的，两边必须一致。
 * 这里把复选框标记从文本里摘掉，给 li 打上类名与原文行号，复选框由 list_item_open 规则补出
 */
function taskListPlugin(md: MarkdownItType) {
  md.core.ruler.after('inline', 'clepsydra-task-list', (state) => {
    const { tokens } = state;
    for (const [index, token] of tokens.entries()) {
      if (token.type !== 'inline') {
        continue;
      }
      const matched = /^\[([ xX])]\s+/.exec(token.content);
      if (!matched) {
        continue;
      }
      const paragraphOpen = tokens[index - 1];
      const listItemOpen = tokens[index - 2];
      if (
        paragraphOpen?.type !== 'paragraph_open' ||
        listItemOpen?.type !== 'list_item_open'
      ) {
        continue;
      }

      const checked = matched[1] !== ' ';
      token.content = token.content.slice(matched[0].length);
      const firstChild = token.children?.[0];
      if (firstChild?.type === 'text') {
        firstChild.content = firstChild.content.slice(matched[0].length);
      }

      listItemOpen.attrJoin('class', TASK_ITEM_CLASS);
      if (checked) {
        listItemOpen.attrJoin('class', TASK_ITEM_CHECKED_CLASS);
      }

      // 复选框标记写在段落首行，取 paragraph 的行号比取整个 list_item 更准
      // （li 里还可能跟着嵌套列表或续行）
      const line = paragraphOpen.map?.[0] ?? listItemOpen.map?.[0];
      if (line !== undefined) {
        listItemOpen.attrSet(TASK_LINE_ATTR, String(line));
      }
    }
    return true;
  });
}

/**
 * 给任务列表项补出复选框
 *
 * 用真正的 `<input type="checkbox">` 而不是纯 CSS 画的方块：既拿到原生的点击热区与
 * 键盘可达性，勾选状态也由 DOM 表达，外层只要监听冒泡上来的 change 事件。
 * 外观由 markdown.css 的 appearance: none 接管
 */
function taskCheckboxPlugin(md: MarkdownItType) {
  md.renderer.rules.list_item_open = (tokens, idx, options, env, self) => {
    const token = tokens[idx];
    const open = self.renderToken(tokens, idx, options);
    // 只有任务列表项带行号属性，普通列表项原样返回
    if (!token?.attrGet(TASK_LINE_ATTR)) {
      return open;
    }

    const checked = String(token.attrGet('class') ?? '').includes(
      TASK_ITEM_CHECKED_CLASS,
    );
    const { interactive } = (env ?? {}) as RenderEnv;
    const attrs = [
      'class="task-list-checkbox"',
      'type="checkbox"',
      checked ? 'checked' : '',
      interactive ? '' : 'disabled',
    ].filter(Boolean);

    return `${open}<input ${attrs.join(' ')}>`;
  };
}

/** 外链一律新窗口打开，并阻断 opener 引用 */
function externalLinkPlugin(md: MarkdownItType) {
  md.renderer.rules.link_open = (tokens, idx, options, _env, self) => {
    const token = tokens[idx];
    token?.attrSet('target', '_blank');
    token?.attrSet('rel', 'noopener noreferrer');
    return self.renderToken(tokens, idx, options);
  };
}

/** 围栏代码块预计算好的高亮结果，key 见 fenceHighlightKey */
type HighlightMap = Map<string, string>;

/** 高亮结果按「语言 + 原文」做 key，避免同一语言不同代码块相互串味 */
function fenceHighlightKey(lang: string, code: string): string {
  return `${lang} ${code}`;
}

/**
 * 围栏代码块把语言标到 pre 上，由 CSS 画出角标，观感贴近编辑器里带语言选择器的代码块。
 *
 * 若 env.highlightMap 里已经有该代码块的高亮结果（renderMarkdownWithHighlight 异步算好
 * 的），直接用高亮后的 HTML；否则退化为纯转义文本，与「语言未收录」的表现一致
 */
function fenceLangPlugin(md: MarkdownItType) {
  md.renderer.rules.fence = (tokens, idx, _options, env: unknown) => {
    const token = tokens[idx];
    const rawCode = token?.content ?? '';
    const lang = (token?.info ?? '').trim().split(/\s+/)[0];
    const langAttr = lang ? ` data-lang="${md.utils.escapeHtml(lang)}"` : '';
    const codeClass = lang
      ? ` class="language-${md.utils.escapeHtml(lang)}"`
      : '';

    const highlightMap = (env as undefined | { highlightMap?: HighlightMap })
      ?.highlightMap;
    const highlighted = lang
      ? highlightMap?.get(fenceHighlightKey(lang, rawCode))
      : undefined;
    const body = highlighted ?? md.utils.escapeHtml(rawCode);

    return `<pre${langAttr}><code${codeClass}>${body}</code></pre>\n`;
  };
}

const markdown = new MarkdownIt({
  // 用户输入不可信，裸 HTML 一律转义
  html: false,
  linkify: true,
  typographer: false,
})
  .use(taskListPlugin)
  .use(taskCheckboxPlugin)
  .use(externalLinkPlugin)
  .use(fenceLangPlugin);

/** DOMPurify 白名单：外链的 target、以及任务列表项的行号 */
const PURIFY_CONFIG = { ADD_ATTR: ['target', TASK_LINE_ATTR] };

/** 把 markdown 原文渲染成可安全插入 DOM 的 HTML 字符串（无代码块语法高亮） */
export function renderMarkdown(
  source: string,
  options: RenderOptions = {},
): string {
  const trimmed = source.trim();
  if (!trimmed) {
    return '';
  }
  const env: RenderEnv = { interactive: options.interactive };
  return DOMPurify.sanitize(markdown.render(trimmed, env), PURIFY_CONFIG);
}

/**
 * 带围栏代码块语法高亮的渲染管线。
 *
 * 内容里没有语言标注的代码块时，直接复用 renderMarkdown 的同步结果，不会触发任何额外
 * 加载；否则按需动态 import code-highlight.ts —— highlight.js 核心与具体语言包只在
 * 真正用得到时才下载，详情页里没有代码块的正文完全零增量
 */
export async function renderMarkdownWithHighlight(
  source: string,
  options: RenderOptions = {},
): Promise<string> {
  const trimmed = source.trim();
  if (!trimmed) {
    return '';
  }

  const fenceTokens = markdown
    .parse(trimmed, {})
    .filter((token) => token.type === 'fence' && token.info.trim());
  if (fenceTokens.length === 0) {
    return renderMarkdown(source, options);
  }

  const { highlightFence } = await import('./code-highlight');

  const highlightMap: HighlightMap = new Map();
  await Promise.all(
    fenceTokens.map(async (token) => {
      const [lang] = token.info.trim().split(/\s+/);
      if (!lang) {
        return;
      }
      const key = fenceHighlightKey(lang, token.content);
      if (highlightMap.has(key)) {
        return;
      }
      const highlighted = await highlightFence(token.content, lang);
      if (highlighted !== null) {
        highlightMap.set(key, highlighted);
      }
    }),
  );

  const env: RenderEnv = { interactive: options.interactive, highlightMap };
  return DOMPurify.sanitize(markdown.render(trimmed, env), PURIFY_CONFIG);
}

/** 任务列表行的前缀：列表符号（`-` / `*` / `+` / `1.`）加复选框标记 */
const TASK_LINE_RE = /^(\s*(?:[*+-]|\d+[).])\s+\[)[ xX](])/;

/**
 * 把原文里第 line 行的任务列表标记改写为 checked，返回新的 markdown 原文
 *
 * 行号取自渲染时打在 li 上的 data-task-line。渲染走的是 trim 后的原文，这里也先 trim
 * 保证行号对齐，返回值可直接作为新的描述提交。
 * 行号越界或该行不是任务列表项（原文已被别处改动）时返回 null，调用方据此放弃本次改写
 */
export function toggleTaskLine(
  source: string,
  line: number,
  checked: boolean,
): null | string {
  const lines = source.trim().split('\n');
  const target = lines[line];
  if (target === undefined) {
    return null;
  }

  const matched = TASK_LINE_RE.exec(target);
  if (!matched) {
    return null;
  }

  lines[line] =
    `${matched[1]}${checked ? 'x' : ' '}${matched[2]}${target.slice(matched[0].length)}`;
  return lines.join('\n');
}
