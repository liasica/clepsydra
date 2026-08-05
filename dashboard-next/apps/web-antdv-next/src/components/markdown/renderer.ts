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

/**
 * 任务列表支持
 *
 * markdown-it 本体不认 GFM 的 `- [ ]`，而编辑器侧（remark-gfm）是支持的，两边必须一致。
 * 这里不注入任何 HTML，只把复选框标记从文本里摘掉并给 li 打上类名，勾选框由 CSS 画出来
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

      listItemOpen.attrJoin('class', 'task-list-item');
      if (checked) {
        listItemOpen.attrJoin('class', 'task-list-item-checked');
      }
    }
    return true;
  });
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

/** 围栏代码块把语言标到 pre 上，由 CSS 画出角标，观感贴近编辑器里带语言选择器的代码块 */
function fenceLangPlugin(md: MarkdownItType) {
  md.renderer.rules.fence = (tokens, idx) => {
    const token = tokens[idx];
    const code = md.utils.escapeHtml(token?.content ?? '');
    const lang = (token?.info ?? '').trim().split(/\s+/)[0];
    const langAttr = lang ? ` data-lang="${md.utils.escapeHtml(lang)}"` : '';
    const codeClass = lang
      ? ` class="language-${md.utils.escapeHtml(lang)}"`
      : '';
    return `<pre${langAttr}><code${codeClass}>${code}</code></pre>\n`;
  };
}

const markdown = new MarkdownIt({
  // 用户输入不可信，裸 HTML 一律转义
  html: false,
  linkify: true,
  typographer: false,
})
  .use(taskListPlugin)
  .use(externalLinkPlugin)
  .use(fenceLangPlugin);

/** 把 markdown 原文渲染成可安全插入 DOM 的 HTML 字符串 */
export function renderMarkdown(source: string): string {
  const trimmed = source.trim();
  if (!trimmed) {
    return '';
  }
  return DOMPurify.sanitize(markdown.render(trimmed), {
    ADD_ATTR: ['target'],
  });
}
