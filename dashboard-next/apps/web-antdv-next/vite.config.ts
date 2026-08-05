import process from 'node:process';

import { defineConfig } from '@vben/vite-config';

import { loadEnv } from 'vite';

export default defineConfig(async ({ mode }) => {
  // dev 代理目标可通过 VITE_API_PROXY_URL 覆盖，默认 8080 与入库 config.example.yaml 一致；
  // 本机后端若监听其它端口，在不入库的 .env.development.local 中单独覆盖，勿改这里的默认值
  const { VITE_API_PROXY_URL = 'http://localhost:8080' } = loadEnv(
    mode,
    process.cwd(),
  );

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
            target: VITE_API_PROXY_URL,
            ws: true,
          },
        },
      },
    },
  };
});
