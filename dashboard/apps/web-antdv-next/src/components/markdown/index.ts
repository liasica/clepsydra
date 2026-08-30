import { defineAsyncComponent } from 'vue';

/**
 * markdown 编辑器（Crepe / ProseMirror + CodeMirror）
 *
 * 体积在整个前端里属于最大的一块，只有进入编辑态才需要，这里统一包成异步组件，
 * 保证它落在独立 chunk 而不是页面主包。页面里直接 `<MarkdownEditor v-model="..." />`
 * 即可，加载期间由使用方自行套骨架屏 / loading
 */
const MarkdownEditor = defineAsyncComponent(
  () => import('./MarkdownEditor.vue'),
);

export { default as MarkdownViewer } from './MarkdownViewer.vue';
export { toggleTaskLine } from './renderer';
export { MarkdownEditor };
