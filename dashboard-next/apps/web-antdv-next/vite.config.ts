import { defineConfig } from '@vben/vite-config';

export default defineConfig(async () => {
  return {
    application: {},
    vite: {
      build: {
        // 产物重定向到 monorepo 根 dist/，改名回 dashboard/ 后即 dashboard/dist/，
        // 与现有 Makefile、Dockerfile、ignore 规则对齐
        emptyOutDir: true,
        outDir: '../../dist',
      },
      server: {
        proxy: {
          '/api': {
            changeOrigin: true,
            // 不 rewrite，保留 /api 前缀 —— 后端 router.go 的 e.Group("/api") 依赖它
            target: 'http://localhost:8080',
            ws: true,
          },
        },
      },
    },
  };
});
