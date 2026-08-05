/**
 * 只读预览的代码块语法高亮
 *
 * 选型：highlight.js（按需注册语言），而不是复用编辑器 CodeMirror 的 language 对象。
 *
 * @codemirror/language 是为交互式编辑器设计的（内建 Facet/StateField 等状态管理），
 * 实测把它单独拆给 Viewer 用会拖出一个 gzip 约 100KB 的「CodeMirror 内核」共享 chunk
 * （详见 task-8-fix-report.md），与 Viewer「轻量渲染器」的定位不成比例。highlight.js
 * 只做静态高亮，core + 9 种语言合计体积小得多，且不依赖用户是否打开过编辑器。
 *
 * 代价：颜色分类不是与编辑器「同一套代码产生」，是两套独立引擎（正则/词法 vs lezer
 * 语法树）尽量映射到同一批 --md-code-* 变量，基础 token（关键字/字符串/注释/数字）能
 * 对齐，但不是逐字节保证一致 —— 已在报告里如实说明
 *
 * 这个模块本身只应该被动态 import：renderer.ts 只在检测到 fence 代码块时才会加载它，
 * 没有代码块的详情页不会拉起 highlight.js
 */

import hljs from 'highlight.js/lib/core';

interface LangEntry {
  /** 除主名字外的别名（大小写不敏感） */
  aliases: string[];
  /** 动态加载 highlight.js 的语言定义模块 */
  load: () => Promise<{ default: unknown }>;
  /** hljs.registerLanguage 使用的主名字 */
  name: string;
}

/** 按需注册的语言表，仅覆盖本项目实际会用到的技术栈；未收录语言退化为无高亮纯文本 */
const languages: LangEntry[] = [
  {
    name: 'go',
    aliases: ['golang'],
    load: () => import('highlight.js/lib/languages/go'),
  },
  {
    name: 'typescript',
    aliases: ['ts'],
    load: () => import('highlight.js/lib/languages/typescript'),
  },
  {
    name: 'javascript',
    aliases: ['js', 'jsx'],
    load: () => import('highlight.js/lib/languages/javascript'),
  },
  {
    name: 'json',
    aliases: [],
    load: () => import('highlight.js/lib/languages/json'),
  },
  {
    name: 'yaml',
    aliases: ['yml'],
    load: () => import('highlight.js/lib/languages/yaml'),
  },
  {
    name: 'sql',
    aliases: ['postgres', 'postgresql'],
    load: () => import('highlight.js/lib/languages/sql'),
  },
  {
    name: 'bash',
    aliases: ['sh', 'shell', 'zsh'],
    load: () => import('highlight.js/lib/languages/bash'),
  },
  {
    name: 'xml',
    aliases: ['html', 'vue'],
    load: () => import('highlight.js/lib/languages/xml'),
  },
  {
    name: 'css',
    aliases: [],
    load: () => import('highlight.js/lib/languages/css'),
  },
  {
    name: 'markdown',
    aliases: ['md'],
    load: () => import('highlight.js/lib/languages/markdown'),
  },
];

/** lang 字符串（不区分大小写，含别名）到语言表条目的查找 */
function resolveLangEntry(lang: string): LangEntry | undefined {
  const key = lang.toLowerCase();
  return languages.find(
    (entry) => entry.name === key || entry.aliases.includes(key),
  );
}

/** 已注册过的语言名，避免重复 registerLanguage */
const registered = new Set<string>();

async function ensureRegistered(entry: LangEntry): Promise<void> {
  if (registered.has(entry.name)) {
    return;
  }
  const mod = await entry.load();
  hljs.registerLanguage(
    entry.name,
    mod.default as Parameters<typeof hljs.registerLanguage>[1],
  );
  if (entry.aliases.length > 0) {
    hljs.registerAliases(entry.aliases, { languageName: entry.name });
  }
  registered.add(entry.name);
}

/**
 * 高亮一段围栏代码块，返回可直接插入 `<code>` 内的 HTML 字符串（highlight.js 自行完成
 * 文本转义，只产出 `<span class="hljs-...">` 结构）。
 *
 * 语言未收录（或加载/解析失败）时返回 null，调用方应回退成纯转义文本 —— 这与编辑器
 * 侧「未收录语言退化为无高亮纯文本」的行为保持一致
 */
export async function highlightFence(
  code: string,
  lang: string,
): Promise<null | string> {
  const entry = resolveLangEntry(lang);
  if (!entry) {
    return null;
  }

  try {
    await ensureRegistered(entry);
    return hljs.highlight(code, {
      ignoreIllegals: true,
      language: entry.name,
    }).value;
  } catch {
    return null;
  }
}
