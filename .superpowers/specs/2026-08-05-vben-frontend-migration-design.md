# 前端迁移设计：Art Design Pro → vue-vben-admin (antdv-next)

日期：2026-08-05
状态：已确认

## 背景

用户决定把前端从 Art Design Pro v3.0.2（Element Plus）整体切换为 vue-vben-admin 的 antdv-next 应用，并要求 markdown 富文本编辑器「一定选用美观的」。

本文档取代 `2026-08-05-demand-workflow-improvements-design.md` 中的前端部分（该文档的后端部分已实施完成：commit `98792bd` 去除完成日期上限、commit `b5dd325` 创建与预估分离）。原计划中基于 Element Plus 的三个前端任务作废。

## 关键事实（调研确认）

| 项 | 值 |
|---|---|
| vben 最新版 | v5.7.0（2026-05-21） |
| 目标应用 | 目录 `apps/web-antdv-next`，包名 `@vben/web-antdv-next`（workspace 私有包，未发布 npm） |
| UI 库 | `antdv-next` ^1.4.5，独立组织 antdv-next/antdv-next 的接棒项目，非 ant-design-vue 的 next tag |
| 技术栈 | Vue 3.5 / vue-router 5 / Pinia 4 / Vite 8(Rolldown) / **Tailwind 4** / vue-i18n 11 |
| 运行时要求 | Node `^22.18.0 \|\| ^24.12.0`；**pnpm 实测为 `>=10.0.0`，`packageManager: pnpm@10.33.4`**（T1 实测，非调研时记录的 11.x） |
| 初始化方式 | 无官方 CLI。clone 后手工精简（5.0 起不再提供独立精简仓库）。**`thin` 分支是 2.8.0 老版本，不可用** |
| 构建命令 | 根无 `build:antdv-next` 脚本，用 `pnpm build --filter=@vben/web-antdv-next` |

## T1 实测修正（以下覆盖调研阶段的记录）

- **pnpm 版本**：vben v5.7.0 实际声明 `packageManager: pnpm@10.33.4`、engines `pnpm >=10.0.0`。本机 pnpm 10.33.0 满足 engines，**唯一不满足的是 Node**（本机 25.3.0）
- **corepack**：Node 25 不再内置 corepack，用 `npx corepack@latest pnpm ...` 代理调用。engines 字段未改动，靠 `.npmrc` 的 `engine-strict=false` 放行
- **`prepare` 脚本的坑（已修复）**：`dashboard-next/package.json` 的 `prepare: lefthook install` 因该目录无独立 `.git`，会向上污染父仓库 `clepsydra/.git/hooks/` 与仓库根目录。T1 已清理残留并删除该脚本

### `extraAppConfig` 产物形态（T1 已查明，T2/T12 依赖）

- 生成固定文件 **`dist/_app.config.js`**，把构建时所有 `VITE_GLOB_*` 变量写入并 freeze 到 `window._VBEN_ADMIN_PRO_APP_CONF_`
- **它是运行时配置文件，生产构建下运行时无 fallback、必读该全局变量** —— 因此**必须与产物一起 embed**，缺了它前端起不来
- 类型上有开关但在 `internal/vite-config` 里硬编码为 `true`，无环境变量入口，只能改源码关闭。**决策：不关闭，照常 embed**
- 现有 `//go:embed all:dashboard` 的 `all:` 前缀已能覆盖下划线开头的文件，Go 侧无需改动
- 产物顶层清单：`_app.config.js`、`css/`、`favicon.ico`、`index.html`、`js/`、`jse/`
- 默认产物含 `dist.zip`（1.18MB，`VITE_ARCHIVER` 默认值），T2 必须设 `VITE_ARCHIVER=false`
- 无 `.gz` 副本（`VITE_COMPRESS=none` 已是脚手架默认值）

## 已定决策

| # | 议题 | 决策 | 理由 |
|---|---|---|---|
| D1 | 原子 CSS | **Tailwind 4**，放弃 UnoCSS | vben 主题系统靠 Tailwind 4 `@theme` 实现，6 个 ui-kit 内部包模板全是 Tailwind 类名；业务侧书写体验与 UnoCSS 一致 |
| D2 | UI 库 | antdv-next（用户指定） | 知悉风险：1.x 新项目、star 约 855，遇 bug 社区答案少。缓解：业务页面组件用法保持朴素 |
| D3 | 工具链 | **绕过 engine 检查**：`.npmrc` 设 `engine-strict=false`，沿用本机 Node 25.3.0 | 用户选择。pnpm 由 corepack 按 vben 声明版本拉起 |
| D4 | 迁移方式 | 新建 `dashboard-next/` 并存，切换时删旧目录并 `git mv` 改名回 `dashboard/` | 后端同期改造中，原地替换会让中间提交点前端不可用；12 个页面需 1:1 对照重写 |
| D5 | 产物目录 | app `vite.config.ts` 把 `build.outDir` 重定向到 monorepo 根 `dist/`（即最终 `dashboard/dist/`），显式设 `emptyOutDir` | Makefile、Dockerfile、.gitignore、.dockerignore 四处零改动 |
| D6 | markdown 编辑器 | **Milkdown + `@milkdown/crepe`** v7.22.0 | 五个候选中唯一 Notion 风所见即所得（slash 菜单、拖拽手柄、bubble toolbar）；markdown 一等公民；官方 Vue 3 绑定；月更节奏。备选 md-editor-v3 |
| D7 | 只读预览 | 另做轻量 `MarkdownViewer`（markdown-it + DOMPurify + 共享 `.markdown-body` 样式） | Crepe 只读态仍拉起完整 ProseMirror，列表/详情页不值当 |
| D8 | 表格表单 | antdv-next 原生 `Table`/`Form` 为主，对话框用 vben `useVbenModal` | 12 个页面表格都朴素（无虚拟滚动/编辑单元格/导出），VxeTable 是额外学习成本与体积 |
| D9 | 测试策略 | 不补组件测试，改为**书面验收清单**（角色 × 状态 × 动作全枚举），最后逐条打勾 | `dict.ts` 动作矩阵已有单测覆盖（权限逻辑唯一来源）；页面层主要是模板翻译，补组件测试会在重写后立刻过时 |
| D10 | i18n | 保留 vben i18n 框架，只提供 zh-CN | vben 路由 meta 与内置组件文案走 `$t()`，移除要改一堆内部包引用 |
| D11 | 前端 CI | 收尾时加 lint + test job | 当前前端零 CI 检查，迁移后代码全新，加门槛成本低 |

## 资产处置

### 可直接搬（约 600 行）

| 资产 | 路径 | 改动 |
|---|---|---|
| 8 个 API 模块（172 行） | `dashboard/src/api/*.ts` | 只改 import（`@/utils/http` → vben `requestClient`）。**删 `api/menu.ts`**（指向不存在的 `/api/v3/system/menus`，Art 模板残留） |
| 全局类型（227 行） | `dashboard/src/types/api/api.d.ts` | 原样搬 |
| `manday.ts`（26 行） | `dashboard/src/utils/clepsydra/manday.ts` | 原样搬 |
| `date.ts`（13 行） | `dashboard/src/utils/clepsydra/date.ts` | 原样搬 |
| `dict.ts`（87 行） | `dashboard/src/utils/clepsydra/dict.ts` | `TagType` 从 EP 语义色换成 antdv-next 语义色，**加映射表而非硬改字面量**。`actions.{admin,client}` 是按钮级权限唯一来源，原样保留 |
| 3 个单测（74 行） | `src/utils/clepsydra/__tests__/` | 原样可用 |

### 摘业务逻辑重装（约 175 行）

**请求层**——不搬 `utils/http/index.ts`，基于 vben `requestClient` 重装，但下列 5 条业务规则必须复刻，否则功能退化：

1. **业务码优先于 HTTP 状态码**——响应体带 `code`+`message` 时用业务码。账单/需求的 42200 状态冲突全靠这条
2. **登录接口豁免**——登录失败不走通用 401 兜底、不误登出、不占防抖窗口
3. **POST/PUT 的 `params` → `data` 自动填充**——8 个 api 模块全写的 `params`
4. `ApiStatus.success = 0`（后端业务成功码是 0）
5. 13 个错误文案 key 落到 vben i18n 或硬编码中文

**user store 业务片段**（276 行里只有 55 行是业务）：`toUserInfo()`、`loginByPassword()`、`restoreSession()`、**persist 配置 `omit: ['isLogin']`**（这是修过的 bug，commit `84f486d`，不排除会导致 `restoreSession` 分支永不执行）。

**路由守卫 4 条业务规则**：

1. 一次性会话恢复（`restored` 标记，保证刷新只打一次 `/api/auth/me`）
2. **404 提前放行**——history 模式手输不存在路径会整页冷启动，不在权限校验前放行会被误判无权限重定向首页，404 页永远出不来
3. catch-all 不算可匿名访问的静态路由——否则未登录手输任意地址直接落 404 而非跳登录
4. `routeInitFailed` / `routeInitInProgress` 双标记防死循环与并发初始化

### 直接丢弃（约 31,800 行）

`components/core/` 83 文件、`utils/`（除 clepsydra）39 文件、`hooks/core/` 13 文件、`assets/styles/` 14 文件、`types/`（除 api）11 文件、`router/` core+guards。业务页面**一个 Art 封装组件都没用**（无 ArtTable/ArtSearchBar），全是裸 Element Plus，反向工程成本为零；业务侧零主题定制。

## 环境变量映射

| 现有 | vben 对应 | 目标值 |
|---|---|---|
| `VITE_BASE_URL` | `VITE_BASE` | `/`（同时驱动 vue-router history base） |
| `VITE_API_URL` | `VITE_GLOB_API_URL` | `/`（默认值是 vben mock 地址，**必须改**）。保持 8 个 api 模块的 `/api/xxx` 硬编码不变，别两边都带 `/api` |
| `VITE_API_PROXY_URL` | app `vite.config.ts` 的 `vite.server.proxy` | dev target `http://localhost:8080` |
| — | `VITE_ROUTER_HISTORY` | **`history`**（默认 hash，后端 fallback 已配套） |
| — | `VITE_ARCHIVER` | **`false`**（默认 true 会额外生成 `dist.zip`，会被一起 embed 进二进制） |
| — | `VITE_APP_STORE_SECURE_KEY` | 必须换掉默认占位值（Pinia 持久化加密密钥） |
| — | `VITE_NITRO_MOCK` | `false` |
| — | `VITE_COMPRESS` | `none`（旧前端的 `.gz` 副本被 embed 但 Go 侧不做协商，是纯死重） |

## 后端集成改动点

集成链路保持：`dashboard/dist/` → `assets/dashboard/` → `//go:embed all:dashboard` → 二进制。

- **Makefile**：安装/构建命令改为 monorepo 根安装 + `pnpm build --filter=@vben/web-antdv-next`。采纳 D5 后 `rm -rf` / `cp -R` / `touch .gitkeep` 三行不用改。**`touch assets/dashboard/.gitkeep` 务必保留**——它是干净检出时让 embed 目录存在的唯一文件，没有它编译直接失败
- **Dockerfile**（风险最高）：依赖安装层会退化。现在拷 3 个文件就能 install（Art 是单包），vben workspace 需要 `apps/*/package.json` + `packages/**/package.json` + `internal/*/package.json` 全部在场。采用多行 COPY 保留缓存层。`corepack prepare pnpm@11.20.0` 的注释（「package.json 未声明 packageManager」）将不成立，需对齐 vben 声明版本
- **运行阶段 `COPY assets ./assets` 不要动**——它服务的是 `assets/holidays/2026.json`（运行时磁盘读取），与前端无关。迁移中把 `assets/` 改成纯 embed 目录会静默丢掉节假日数据
- **ignore 规则**：`dashboard/node_modules/`、`dashboard/dist/` 需补 monorepo 递归规则 `dashboard/**/node_modules/`、`dashboard/**/dist/`。`assets/dashboard/*` + `!assets/dashboard/.gitkeep` 两条不动
- **Go 侧零改动**：`assets/assets.go`、`router.go` 挂载顺序、`static.go` 前缀排除均不变（前提是 Makefile target 仍叫 `dashboard`）
- **CI**：零改动（前端构建发生在 Dockerfile 阶段一内部）

## 随迁的业务改动

迁移的同时落地 TODO 中两项（后端已就绪）：

1. **需求描述用 markdown 编辑器**：创建只填标题 + 描述，需求方角色可创建/修改，确认人天后锁定；人天预估走 `submit-estimate` 弹窗
2. **设置中心「账单包含的需求状态」改标签按钮组**：复用 `dict.ts` 的 label + 语义色，选中态填充、未选描边

## 待验证项（实施中确认）

1. ~~`extraAppConfig` 产物形态~~ —— **T1 已查明，见上文**
2. outDir 重定向到 Vite root 之外的警告/副作用（`emptyOutDir` 必须显式设 true）
3. Crepe CSS 与 Tailwind 4 preflight 的互相覆盖——建议把 Crepe 容器 scope 起来
4. antdv-next 的 CSS-in-JS 与 vben `@vben/styles/antdv-next` 适配层在自定义主题色下的表现

## 风险

- **零回归网**：109 行纯函数单测 vs 1,784 行页面代码。缓解靠书面验收清单 + 并存期新旧对照
- **描述字段 XSS 面**：需求方角色也能写 markdown，预览渲染必须 `html: false` + DOMPurify
- **Crepe 体积**：gzip ~447KB。缓解：编辑器路由级懒加载 + 关掉 Latex/CodeMirror 等未用 feature + 预览走轻量 Viewer
- **markdown 往返规范化**：markdown → ProseMirror doc → markdown 会规范化（`*` 列表统一成 `-`、多余空行折叠）。存的仍是纯 markdown 但非字节级原样，对需求描述场景无影响
