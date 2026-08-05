/**
 * Crepe 代码块的 CodeMirror 配置
 *
 * 1. 语言表手动收敛：@codemirror/language-data 会带来 130+ 个动态 import 分包，
 *    对「需求描述」这种场景绝大多数用不到，这里只保留本项目技术栈相关的语言，
 *    每种语言仍然是按需加载的独立 chunk；
 * 2. 高亮配色全部走 CSS 变量（markdown.css 里的 --md-code-*），因此亮暗切换由
 *    CSS 继承完成，不需要在暗色时重建编辑器
 *
 * 只读预览（MarkdownViewer）的语法高亮改走 highlight.js（见 code-highlight.ts），
 * 不复用这里的 CodeMirror language 对象 —— @codemirror/language 是为交互式编辑器设计的
 * 重量级依赖（内建 Facet/StateField 等状态管理），只做静态高亮的话体积（gzip 约 100KB+）
 * 与 Viewer「轻量渲染器」的定位不成比例，详见 task-8-fix-report.md 的选型说明
 */

import {
  HighlightStyle,
  LanguageDescription,
  LanguageSupport,
  StreamLanguage,
  syntaxHighlighting,
} from '@codemirror/language';
import { tags } from '@lezer/highlight';

/** 代码块可选语言，按需动态加载 */
export const codeLanguages: LanguageDescription[] = [
  LanguageDescription.of({
    name: 'Go',
    alias: ['go', 'golang'],
    extensions: ['go'],
    load: () => import('@codemirror/lang-go').then((m) => m.go()),
  }),
  LanguageDescription.of({
    name: 'TypeScript',
    alias: ['ts', 'typescript'],
    extensions: ['ts'],
    load: () =>
      import('@codemirror/lang-javascript').then((m) =>
        m.javascript({ typescript: true }),
      ),
  }),
  LanguageDescription.of({
    name: 'JavaScript',
    alias: ['js', 'javascript', 'jsx'],
    extensions: ['js', 'jsx'],
    load: () =>
      import('@codemirror/lang-javascript').then((m) =>
        m.javascript({ jsx: true }),
      ),
  }),
  LanguageDescription.of({
    name: 'JSON',
    alias: ['json'],
    extensions: ['json'],
    load: () => import('@codemirror/lang-json').then((m) => m.json()),
  }),
  LanguageDescription.of({
    name: 'YAML',
    alias: ['yaml', 'yml'],
    extensions: ['yaml', 'yml'],
    load: () => import('@codemirror/lang-yaml').then((m) => m.yaml()),
  }),
  LanguageDescription.of({
    name: 'SQL',
    alias: ['sql', 'postgres', 'postgresql'],
    extensions: ['sql'],
    load: () => import('@codemirror/lang-sql').then((m) => m.sql()),
  }),
  LanguageDescription.of({
    name: 'Shell',
    alias: ['sh', 'bash', 'shell', 'zsh'],
    extensions: ['sh'],
    load: () =>
      import('@codemirror/legacy-modes/mode/shell').then(
        (m) => new LanguageSupport(StreamLanguage.define(m.shell)),
      ),
  }),
  LanguageDescription.of({
    name: 'HTML',
    alias: ['html', 'vue', 'xml'],
    extensions: ['html', 'vue'],
    load: () => import('@codemirror/lang-html').then((m) => m.html()),
  }),
  LanguageDescription.of({
    name: 'CSS',
    alias: ['css'],
    extensions: ['css'],
    load: () => import('@codemirror/lang-css').then((m) => m.css()),
  }),
  LanguageDescription.of({
    name: 'Markdown',
    alias: ['markdown', 'md'],
    extensions: ['md'],
    load: () => import('@codemirror/lang-markdown').then((m) => m.markdown()),
  }),
];

/**
 * 高亮样式：颜色全部引用 CSS 变量，实际取值由 markdown.css 的亮暗两套调色板决定。
 * basicSetup 里的 defaultHighlightStyle 是以 fallback 优先级注册的，这里的样式会覆盖它
 */
const highlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--md-code-keyword)' },
  { tag: tags.controlKeyword, color: 'var(--md-code-keyword)' },
  { tag: tags.moduleKeyword, color: 'var(--md-code-keyword)' },
  { tag: tags.operatorKeyword, color: 'var(--md-code-keyword)' },
  {
    tag: [tags.string, tags.special(tags.string)],
    color: 'var(--md-code-string)',
  },
  {
    tag: [tags.comment, tags.lineComment, tags.blockComment],
    color: 'var(--md-code-comment)',
    fontStyle: 'italic',
  },
  {
    tag: [tags.number, tags.bool, tags.null, tags.atom],
    color: 'var(--md-code-number)',
  },
  {
    tag: [tags.function(tags.variableName), tags.function(tags.propertyName)],
    color: 'var(--md-code-function)',
  },
  {
    tag: [
      tags.typeName,
      tags.className,
      tags.namespace,
      tags.standard(tags.typeName),
    ],
    color: 'var(--md-code-type)',
  },
  {
    tag: [tags.variableName, tags.propertyName],
    color: 'var(--md-code-variable)',
  },
  {
    tag: [tags.operator, tags.punctuation, tags.separator, tags.bracket],
    color: 'var(--md-code-punctuation)',
  },
  { tag: [tags.tagName, tags.heading], color: 'var(--md-code-tag)' },
  {
    tag: [tags.attributeName, tags.labelName],
    color: 'var(--md-code-attribute)',
  },
  {
    tag: tags.link,
    color: 'var(--md-code-attribute)',
    textDecoration: 'underline',
  },
  { tag: tags.invalid, color: 'var(--md-code-keyword)' },
]);

/** 追加到 Crepe 代码块的 CodeMirror 扩展 */
export const codeExtensions = [syntaxHighlighting(highlightStyle)];
