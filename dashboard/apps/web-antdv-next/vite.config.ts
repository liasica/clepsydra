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
      // 产物落在包内默认的 dist/（即 dashboard/apps/web-antdv-next/dist/），
      // 与 turbo.json 的 build.outputs（按包目录解析为 apps/web-antdv-next/dist/**）保持一致，
      // 避免 outDir 重定向到 monorepo 根导致 turbo 缓存声明与实际产物目录错配；
      // analyze 模式的诊断产物单独落到 node_modules/.cache/ 下，不与 dist/ 共用目录 ——
      // turbo 缓存命中时只解压不清空 outDir，两者混在一起会让上一次 analyze 的过期
      // chunk 残留下来，被 make dashboard 一并 cp 进 assets/dashboard/ 并 embed 进二进制
      ...(mode === 'analyze' && {
        build: { outDir: 'node_modules/.cache/analyze-dist' },
      }),
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
