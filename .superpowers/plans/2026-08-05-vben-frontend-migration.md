# 前端迁移 vue-vben-admin 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.
> 本项目协作约定：子代理任务**不做逐任务代码审查**，全部完成后统一全分支审查。

**Goal:** 把前端从 Art Design Pro（Element Plus）整体迁移到 vue-vben-admin v5.7.0 的 `apps/web-antdv-next` 应用，同时落地 TODO 中的 markdown 编辑器与状态标签按钮组两项。

**Architecture:** 新建 `dashboard-next/`（vben monorepo）与旧 `dashboard/` 并存开发，逐页 1:1 重写；收尾时删旧目录并 `git mv dashboard-next dashboard`，让后端集成层（embed 路径、Makefile、Dockerfile、ignore 规则）改动最小。产物 outDir 重定向到 monorepo 根 `dist/`。

**Tech Stack:** Vue 3.5 + vue-router 5 + Pinia 4 + Vite 8(Rolldown) + **Tailwind 4** + antdv-next 1.4.x + Milkdown Crepe 7.22 + vue-i18n 11

**Spec:** `.superpowers/specs/2026-08-05-vben-frontend-migration-design.md`（决策表 D1-D11、资产处置清单、环境变量映射、集成改动点均在其中，实施时以 spec 为准）

## 关于本计划的写法

vben v5.7.0 的内部 API（`requestClient` 拦截器签名、`preferences` 字段、路由 meta 名、`useVbenModal` 用法）必须**对照 `dashboard-next/` 里的实际源码**确定。本计划因此对框架适配类任务给出「目标 + 硬约束 + 验收标准」，而非可能过时的示例代码——照抄外部博客或旧版本示例是本计划明确禁止的行为。对搬运类任务（api 模块、类型、工具函数）给出精确文件清单与逐项改动要点。

## Global Constraints

- **注释一律中文**、单行注释结尾不加句末标点；标点遵循 `~/.claude/CLAUDE.md` 标点规范
- Git 提交遵循 Conventional Commits，**禁止任何 AI 署名**（`Co-Authored-By: Claude`、`Generated with Claude Code` 等，正文与 trailer 都不允许）
- 前端提交前必须 `pnpm lint` 无 issue
- **原子 CSS 用 Tailwind 4**，不引入 UnoCSS（决策 D1）
- 工具链：`.npmrc` 设 `engine-strict=false` 绕过 vben engines 检查，沿用本机 Node 25.3.0（决策 D3）
- 人天以整数半天数存储（1 人天 = 2），用 `mandayToHalfDays`/`halfDaysToManday` 换算
- 后端业务成功码 `code === 0`，状态冲突码 `42200`
- 所有接口路径保持 `/api/xxx` 硬编码在 api 模块里，`VITE_GLOB_API_URL` 设 `/`，**不要两边都带 `/api`**
- 迁移期禁止修改旧 `dashboard/` 目录（保持可构建可发布），直到 T12
- `git add` 精确指定路径，禁止 `git add -A`；不要提交 `TODO.md`（用户手写）与 `.superpowers/`

---

### T1: 脚手架落地与产物形态确认

**Files:**
- Create: `dashboard-next/`（vben v5.7.0 clone 后精简）
- Create: `dashboard-next/.npmrc`（`engine-strict=false`）
- Modify: `.gitignore`（临时加 `dashboard-next/**/node_modules/`、`dashboard-next/**/dist/`）

**Interfaces:**
- Produces: 可 `pnpm dev:antdv-next`、可 `pnpm build --filter=@vben/web-antdv-next` 的空壳应用；一份 `dist` 目录清单记录（供 T2 与 T12 使用）

- [ ] **Step 1: clone 与精简**

在仓库根目录：`git clone --depth 1 --branch v5.7.0 https://github.com/vbenjs/vue-vben-admin.git dashboard-next`，然后 `rm -rf dashboard-next/.git`。

按官方 thin 文档精简，删除：`apps/web-antd`、`apps/web-ele`、`apps/web-naive`、`apps/web-tdesign`、`apps/backend-mock`、`playground/`、`docs/`。保留 `apps/web-antdv-next`、`packages/`、`internal/`。

**禁止使用仓库的 `thin` 分支**——它是 vben 2.8.0 的老精简版（windi.config.ts、单体 src/），与 v5 无关。

- [ ] **Step 2: 清理根 scripts 与 workspace**

改根 `package.json` 的 scripts，删掉指向已删应用的条目（`dev:antd`、`dev:ele`、`dev:naive`、`dev:tdesign`、`build:antd` 等），保留/新增 `dev:antdv-next` 与 `build:antdv-next`（后者内容为 `pnpm build --filter=@vben/web-antdv-next`）。核对 `pnpm-workspace.yaml` 的 packages 通配是否仍匹配保留目录。

- [ ] **Step 3: 绕过 engine 检查并安装**

创建 `dashboard-next/.npmrc`，内容包含 `engine-strict=false`。

Run: `cd dashboard-next && corepack enable && pnpm install`
Expected: 安装成功，无 `Unsupported engine` 中断。若 `preinstall` 的 `only-allow pnpm` 或 engine 检查仍拦截，记录实际报错并在报告中说明所用规避手段（可选：`--ignore-scripts` 后手动补，或临时改 engines 字段——若改了必须在报告中明示）。

- [ ] **Step 4: 首次构建并记录产物形态**

Run: `cd dashboard-next && pnpm build --filter=@vben/web-antdv-next`
然后：`ls -R apps/web-antdv-next/dist | head -60`

**这一步是 T1 的核心产出**，必须回答：
1. `extraAppConfig: true`（internal/vite-config 硬编码）生成了什么文件？文件名与内容是什么？
2. 它是否是运行时配置文件？是否会在运行时覆盖 `VITE_GLOB_API_URL`？能否关闭？
3. 产物里有没有 `dist.zip`（`VITE_ARCHIVER` 默认 true 的产物）？
4. 有没有 `.gz` 副本（`VITE_COMPRESS`）？
5. 顶层目录清单（`js/`、`css/`、`assets/` 等），确认 `//go:embed all:dashboard` 能覆盖。

把答案写进报告，T2 与 T12 依赖这些事实。

- [ ] **Step 5: dev 冒烟**

Run: `cd dashboard-next && pnpm dev:antdv-next`
访问默认端口 5999，确认页面能起来（此时还是 vben 默认 demo 内容，用默认账号能进后台即可），然后停掉。

- [ ] **Step 6: 提交**

先在根 `.gitignore` 追加 `dashboard-next/**/node_modules/` 与 `dashboard-next/**/dist/`，确认 `git status` 里没有 node_modules 与 dist。

```bash
git add .gitignore dashboard-next
git commit -m "chore: 引入 vue-vben-admin v5.7.0 脚手架并精简为 antdv-next 单应用"
```

---

### T2: 环境变量与构建配置定型

**Files:**
- Modify: `dashboard-next/apps/web-antdv-next/.env`、`.env.development`、`.env.production`
- Modify: `dashboard-next/apps/web-antdv-next/vite.config.ts`

**Interfaces:**
- Consumes: T1 的 dist 目录清单（尤其 `extraAppConfig` 与 archiver 结论）
- Produces: 产物落在 `dashboard-next/dist/`；dev 时 `/api` 代理到 `http://localhost:8080`；history 路由模式

- [ ] **Step 1: 环境变量**

按 spec 的「环境变量映射」表设置。确切值：

- `VITE_BASE=/`
- `VITE_GLOB_API_URL=/`
- `VITE_ROUTER_HISTORY=history`（生产环境默认是 `hash`，必须改）
- `VITE_ARCHIVER=false`（默认 true 会生成 `dist.zip` 被一起 embed，体积翻倍）
- `VITE_NITRO_MOCK=false`
- `VITE_COMPRESS=none`
- `VITE_APP_STORE_SECURE_KEY=` 换掉默认占位 `please-replace-me-with-your-own-key`，用一个本项目自己的随机串
- `VITE_PWA=false`

应用标题类变量（`VITE_APP_TITLE` 之类，以实际字段名为准）设为 `Clepsydra`。

- [ ] **Step 2: vite.config.ts —— 产物目录重定向**

在 app 的 `vite.config.ts` 的 `vite` 字段里设置 `build.outDir` 指向 monorepo 根的 `dist`（相对 app 目录是 `../../dist`），并显式设 `build.emptyOutDir: true`（outDir 在 Vite root 之外会警告）。

目的：产物落在 `dashboard-next/dist/`，改名回 `dashboard/` 后就是 `dashboard/dist/`，与现有 Makefile、Dockerfile、ignore 规则完全对齐（决策 D5）。

- [ ] **Step 3: vite.config.ts —— dev 代理**

配置 `vite.server.proxy`，把 `/api` 代理到 `http://localhost:8080`，`changeOrigin: true`，**不要 rewrite 掉 `/api` 前缀**（后端 `router.go` 的 `e.Group("/api")` 依赖它）。

- [ ] **Step 4: 验证**

Run: `cd dashboard-next && pnpm build --filter=@vben/web-antdv-next && ls dist`
Expected: 产物出现在 `dashboard-next/dist/`，**没有** `dist.zip`，**没有** `.gz` 文件。

Run: `grep -r "mock-napi.vben.pro" dashboard-next/apps --include="*.env*"`
Expected: 无输出（默认 mock 地址已清除）。

- [ ] **Step 5: 提交**

```bash
git add dashboard-next/apps/web-antdv-next
git commit -m "chore: 前端环境变量与构建产物路径定型"
```

---

### T3: 品牌与主题定型

**Files:**
- Modify: `dashboard-next/apps/web-antdv-next/src/preferences.ts`
- Modify: `dashboard-next/apps/web-antdv-next/index.html`
- Modify/Delete: `dashboard-next/apps/web-antdv-next/src/locales/`（只留 zh-CN）

**Interfaces:**
- Produces: 空壳应用外观已是 Clepsydra 的样子；语言只有中文

- [ ] **Step 1: preferences**

在 `preferences.ts` 里设置：站点名 `Clepsydra`、默认语言 `zh-CN`、默认亮暗（推荐 `auto` 或 `light`，自行判断）、`app.accessMode` 为 `frontend`（本项目是前端角色路由过滤，不是后端下发菜单）。

关掉用不到的偏好面板项（如多语言切换开关，若只留 zh-CN）。**不要**关掉主题色/暗色切换——暗色是验收项。

- [ ] **Step 2: 语言包**

只保留 zh-CN，删除 en-US 等其他语言包文件与其注册处。保留 i18n 框架本身（决策 D10）。

- [ ] **Step 3: index.html 与图标**

改 `<title>` 为 Clepsydra，替换 favicon（沿用旧 `dashboard/public/` 里的图标，若无则保留 vben 默认，报告中说明）。

- [ ] **Step 4: 验证并提交**

Run: `cd dashboard-next && pnpm dev:antdv-next`，确认标题、语言、主题正常，暗色切换可用。

```bash
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 前端品牌与主题定型为 Clepsydra"
```

---

### T4: 请求层装配与 API 模块迁移

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/api/request.ts`（或沿用 vben 既有的 `src/api/request.ts`，以实际结构为准）
- Create: `dashboard-next/apps/web-antdv-next/src/api/{auth,demand,bill,setting,user,auditlog,dashboard}.ts`（从旧目录搬）
- Create: `dashboard-next/apps/web-antdv-next/src/types/api/api.d.ts`
- Create: `dashboard-next/apps/web-antdv-next/src/utils/http/error.ts`（`HttpError` + `isHttpError`）
- Create: 对应的 `error.test.ts`

**Interfaces:**
- Consumes: 旧 `dashboard/src/api/*.ts`、`dashboard/src/types/api/api.d.ts`、`dashboard/src/utils/http/`
- Produces: 全部业务接口函数可调用；`HttpError`（带 `code` 字段）；`isHttpError()`；业务页面靠 `error.code === 42200` 判断状态冲突

- [ ] **Step 1: 读旧实现，提取 5 条业务规则**

先读 `dashboard/src/utils/http/index.ts` 与 `dashboard/src/utils/http/error.ts`，确认这 5 条规则的具体实现位置与逻辑（spec 已列出，此处是核对）：

1. **业务码优先于 HTTP 状态码**——响应体带 `code`+`message` 时用业务码构造 `HttpError`（`error.ts:142-149` 附近）
2. **登录接口豁免**——登录失败不走通用 401 兜底、不误登出、不占防抖窗口（`index.ts:30-32,95,103,198-205` 附近）
3. **POST/PUT 的 `params` → `data` 自动填充**——8 个 api 模块全写 `params`（`index.ts:177-185` 附近）
4. `ApiStatus.success = 0`
5. 13 个错误文案 key

- [ ] **Step 2: 基于 vben requestClient 重装**

读 vben 自带的请求层实现（`apps/web-antdv-next/src/api/request.ts` 与 `@vben/request` 包），在其拦截器体系里落地上述 5 条规则。**不要照搬旧 axios 封装的整个文件**，要用 vben 的 `requestClient` + 它的 `errorMessageResponseInterceptor` / `authenticateResponseInterceptor` 等既有机制。

`HttpError` 与 `isHttpError` 单独放 `src/utils/http/error.ts`，业务页面 import 它判断 42200。

- [ ] **Step 3: 搬 api 模块与类型**

从 `dashboard/src/api/` 搬 7 个业务模块（`auth.ts`、`demand.ts`、`bill.ts`、`setting.ts`、`user.ts`、`auditlog.ts`、`dashboard.ts`），**删掉 `menu.ts`**（指向后端不存在的 `/api/v3/system/menus`，是 Art 模板残留）。

每个文件只改 import 行（`@/utils/http` → vben `requestClient`），函数签名与 url 保持不变。

**注意后端契约已变**（commit `b5dd325`）：
- `createDemand` / `updateDemand` 的请求体现在只有 `{title, description}`
- `submitEstimate(id, params)` 现在带请求体 `{estimated_half_days, planned_start_date?}`

若旧 `demand.ts` 还是老签名，按新契约改，并同步 `api.d.ts`：`SaveParams` 收窄为 `{title: string; description?: string}`，新增 `EstimateParams {estimated_half_days: number; planned_start_date?: string}`。

搬 `dashboard/src/types/api/api.d.ts` 到新目录（保持 `declare namespace Api`），确认 `@` 别名在 vben 里指向同样的位置。

- [ ] **Step 4: 测试**

搬 `dashboard/src/utils/http/__tests__/error.test.ts`（35 行），改 2 处 mock（`vi.mock('element-plus')` → vben 的 message 组件；`vi.mock('@/locales')` → vben i18n）。

Run: `cd dashboard-next && pnpm test`（以 vben 实际的 test 脚本名为准）
Expected: error.test.ts 全绿

- [ ] **Step 5: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 请求层装配与业务 API 模块迁移"
```

---

### T5: 业务工具与字典迁移

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/utils/clepsydra/{manday,date,dict}.ts`
- Create: `dashboard-next/apps/web-antdv-next/src/utils/clepsydra/__tests__/{manday,date,dict}.test.ts`

**Interfaces:**
- Produces:
  - `halfDaysToManday(half: number): number`、`mandayToHalfDays(manday: number): number`、`formatManday(half): string`、`formatAmount(yuan): string`
  - `formatDate(v): string`、`formatDateTime(v): string`
  - `DEMAND_STATUS: Record<DemandStatus, StatusMeta<DemandAction>>`、`BILL_STATUS`、类型 `DemandStatus`/`BillStatus`/`DemandAction`/`BillAction`
  - **新增** `tagColor(type: TagType): string` 映射函数（EP 语义色 → antdv-next 色值）

- [ ] **Step 1: 原样搬 manday.ts 与 date.ts**

从 `dashboard/src/utils/clepsydra/` 搬这两个文件，零改动。

**一处例外**：`formatManday` 改为对 0 也显示占位符——需求创建后未预估时 `estimated_half_days` 为 0，显示「0 人天」会误导：

```ts
/** 格式化人天展示，空值与未预估（0）显示占位符 */
export function formatManday(half: number | null | undefined): string {
  if (!half) return '—'
  return `${halfDaysToManday(half)} 人天`
}
```

- [ ] **Step 2: 搬 dict.ts 并加语义色映射**

原样搬 `dict.ts`（含 `DEMAND_STATUS`、`BILL_STATUS` 与全部 `actions.{admin,client}` 白名单——这是按钮级权限的唯一来源，一个字都不要改）。

`TagType` 保持现有的 `'info' | 'primary' | 'warning' | 'success' | 'danger'` 字面量不动，**另加一个映射函数**把它翻译成 antdv-next 的 Tag color（决策：加映射表而非硬改字面量，日后换 UI 库只改映射）：

```ts
/** Element Plus 语义色 → antdv-next Tag color 映射，隔离 UI 库差异 */
export function tagColor(type: TagType): string {
  const map: Record<TagType, string> = {
    info: 'default',
    primary: 'processing',
    warning: 'warning',
    success: 'success',
    danger: 'error'
  }
  return map[type]
}
```

antdv-next 的实际 color 取值需对照其 Tag 组件文档核实（`processing`/`default`/`success`/`warning`/`error` 是 antd 的预设状态色），若取值不同以实际为准并在报告中说明。

**同时按后端新契约更新 `DEMAND_STATUS` 的 actions**（这两项是 TODO 要求，后端已就绪）：

```ts
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['edit', 'submitEstimate'], client: ['edit'] }
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: { admin: ['edit', 'submitEstimate'], client: ['edit', 'confirmEstimate'] }
  },
```

其余四个状态的 actions 不变。

- [ ] **Step 3: 搬测试**

搬 3 个测试文件。`dict.test.ts` 需要补：新的 actions 断言（client 在 draft/pending_estimate 有 edit）、`tagColor` 映射断言。`manday.test.ts` 补 `formatManday(0) === '—'` 断言。

Run: `cd dashboard-next && pnpm test`
Expected: 3 个测试文件全绿

- [ ] **Step 4: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 业务工具与状态字典迁移"
```

---

### T6: 认证与会话

**Files:**
- Modify: `dashboard-next/apps/web-antdv-next/src/store/auth.ts`（vben 自带的 auth store）
- Modify: `dashboard-next/apps/web-antdv-next/src/views/_core/authentication/login.vue`（vben 自带登录页）

**Interfaces:**
- Consumes: T4 的 `login()`、`fetchMe()` api
- Produces: `loginByPassword(username, password)`、`restoreSession()`、`toUserInfo(user)`；user store 持久化排除 `isLogin`

- [ ] **Step 1: 读旧 user store 的业务片段**

读 `dashboard/src/store/modules/user.ts:206-240`，提取这 4 项（276 行里只有 55 行是业务）：

1. `toUserInfo(user)`——后端 `SimpleUser` → 前端 `UserInfo`，**补 `roles: [user.role]`** 供菜单过滤
2. `loginByPassword()`——调 `/api/auth/login`，存 token 与用户信息
3. `restoreSession()`——持 token 时调 `/api/auth/me` 校验，失败即登出
4. **persist 配置 `omit: ['isLogin']`**——这是修过的 bug（commit `84f486d`）：不排除会导致刷新后 `accessToken && !isLogin` 恒为 false，`restoreSession` 分支永不执行

- [ ] **Step 2: 装到 vben auth store**

vben 自带 `useAuthStore` 与 `useUserStore`，把上述业务逻辑落到它们的既有方法里（不要另起一套 store）。持久化配置按 vben 的 pinia-plugin-persistedstate 用法排除 `isLogin` 等瞬时字段。

- [ ] **Step 3: 登录页**

用 vben 的 `AuthPageLayout` + `useVbenForm` 重写登录页，接后端 `/api/auth/login`。

**硬约束**：登录失败必须显示后端返回的 `message`（如「用户名或密码错误」），不能吞成通用文案——这是修过的 bug，实测后端登录失败返回的是 400 而非 401。

- [ ] **Step 4: 联调验证**

启后端（`go run ./cmd/...` 或已有二进制，监听 8080），`pnpm dev:antdv-next`，验证四项：

1. 正确账号密码能登录进后台
2. 错误密码显示后端的具体错误文案，不跳转
3. 登录后刷新页面不掉线
4. 打开 Network 面板，刷新页面时 `/api/auth/me` **只打一次**

- [ ] **Step 5: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 登录与会话恢复接入后端"
```

---

### T7: 路由与守卫

**Files:**
- Create/Modify: `dashboard-next/apps/web-antdv-next/src/router/routes/modules/*.ts`
- Modify: `dashboard-next/apps/web-antdv-next/src/router/guard.ts`（vben 自带的 access guard）

**Interfaces:**
- Consumes: T6 的 auth store
- Produces: 6 个业务路由模块（工作台、需求、账单、设置、用户、审计日志）；按 `meta.authority` + role 过滤菜单

- [ ] **Step 1: 路由模块改写**

读旧 `dashboard/src/router/modules/*.ts`（6 个文件 78 行），把 path / component 路径 / title / icon（`ri:*` remixicon，vben 也支持 iconify）搬过来，meta 字段名改成 vben 的：

| 旧（Art） | 新（vben） |
|---|---|
| `roles: ['admin']` | `authority: ['admin']` |
| `isHide: true` | `hideInMenu: true` |
| `keepAlive` | `keepAlive` |
| `fixedTab` | `affixTab` |
| 排序字段 | `order` |

**菜单扁平化**：旧前端做过「单菜单不嵌套展开」的改造（commit `2dcc1ff`，用户明确要求过「哪怕就一个菜单也不要展开」）。vben 的菜单机制不同，需要确认单个子路由的模块是否会渲染成可展开的父菜单——若会，改成一级菜单项。详情页路由（需求详情、账单详情）用 `hideInMenu: true` 挂成顶层隐藏项。

- [ ] **Step 2: 守卫的 4 条业务规则**

vben 自带 access guard，把这 4 条规则落位（读旧 `dashboard/src/router/guards/beforeEach.ts` 核对细节）：

1. **一次性会话恢复**——`restored` 标记，保证刷新一次只打一次 `/api/auth/me`
2. **404 提前放行**——history 模式手输不存在路径会整页冷启动，若不在权限校验前放行静态路由与 404 路由，会被误判无权限重定向到首页，**404 页永远出不来**
3. **catch-all 不算可匿名访问的静态路由**——否则未登录手输任意地址直接落 404 而非跳登录
4. `routeInitFailed` / `routeInitInProgress` 双标记防死循环与并发初始化

- [ ] **Step 3: 验证**

用 admin 与 client 两个角色分别登录，验证：

1. 菜单项按角色正确显示/隐藏，且**没有只含一个子项还要点开的父菜单**
2. 已登录手输 `/nonexistent` → 显示 404 页（不是跳首页）
3. 未登录手输 `/demands` → 跳登录页，登录后回跳 `/demands`
4. 深层路由（如 `/demands/1`）直接刷新不白屏

- [ ] **Step 4: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 业务路由与权限守卫迁移"
```

---

### T8: Markdown 编辑器与预览组件

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/components/markdown/MarkdownEditor.vue`
- Create: `dashboard-next/apps/web-antdv-next/src/components/markdown/MarkdownViewer.vue`
- Create: `dashboard-next/apps/web-antdv-next/src/components/markdown/markdown.css`（共享 `.markdown-body` 样式）

**Interfaces:**
- Produces:
  - `MarkdownEditor`：props `{ modelValue: string; height?: string }`，emit `update:modelValue`。内部用 `@milkdown/crepe`
  - `MarkdownViewer`：props `{ content: string }`。内部用 `markdown-it` + `DOMPurify`

- [ ] **Step 1: 安装依赖**

```bash
cd dashboard-next && pnpm --filter @vben/web-antdv-next add @milkdown/crepe @milkdown/kit markdown-it dompurify
pnpm --filter @vben/web-antdv-next add -D @types/markdown-it
```

（具体包名以 Milkdown v7.22 官方 Vue 集成文档为准，`@milkdown/vue` 是否需要视 Crepe 用法而定——Crepe 可以直接命令式创建，不一定需要 Vue 绑定包。）

- [ ] **Step 2: MarkdownEditor**

用 `@milkdown/crepe` 的 `Crepe` 类在 `onMounted` 里挂载到容器 div，`onUnmounted` 里 destroy。

**硬约束：**

1. **主题用 `frame`**（中性），不要用默认的 `crepe` 主题（暖棕奶油色 + Georgia 衬线，与 antd 蓝完全两个调子）
2. 覆盖 CSS 变量对齐 antd：`--crepe-color-primary` 等，具体变量名以 Crepe 主题源码为准
3. **暗色联动**——跟随 vben 的暗色状态切换 Crepe 亮暗主题
4. **关闭未用 feature**：Latex、（视情况）CodeMirror，减小体积
5. v-model 双向绑定：`defaultValue` 吃 markdown，监听 `markdownUpdated` 事件 emit 出去
6. **容器 scope 隔离**——Crepe 自带 CSS 与 Tailwind 4 preflight 可能互相覆盖，把 Crepe 容器包一层 scoped 类名

- [ ] **Step 3: MarkdownViewer**

`markdown-it` 渲染 + `DOMPurify.sanitize()`。

**硬约束：**
- `markdown-it` 配置 **`html: false`**——需求方角色也能写描述，这是用户输入
- 输出必须过 DOMPurify
- 样式用共享的 `.markdown-body` 类（标题层级、列表、代码块、引用块、表格），亮暗两套

- [ ] **Step 4: 懒加载**

`MarkdownEditor` 用 `defineAsyncComponent` 包一层供页面使用（Crepe gzip ~447KB，不能进主 chunk）。`MarkdownViewer` 轻量，可以直接引入。

- [ ] **Step 5: 视觉验收**

建一个临时演示页（或直接在 T9 的需求表单里验），用真实 markdown 内容（含标题、列表、任务列表、代码块、引用、表格、加粗斜体）检查：

1. 亮色下编辑器观感——slash 菜单、拖拽手柄、bubble toolbar 是否正常
2. 暗色下编辑器观感
3. Viewer 渲染与编辑器所见是否一致（同一份 markdown）
4. Crepe 样式没有被 Tailwind preflight 破坏（列表符号、标题字号还在）

**用户对这一项的要求是「一定要美观」**——若 `frame` 主题 + 变量覆盖后观感仍差，在报告里说明并附截图路径，不要凑合交付。

- [ ] **Step 6: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next dashboard-next/pnpm-lock.yaml
git commit -m "feat: markdown 编辑器与预览组件"
```

---

### T9: 需求模块页面

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/views/demands/index.vue`（列表）
- Create: `.../views/demands/detail.vue`（详情）
- Create: `.../views/demands/components/DemandFormDialog.vue`（创建/编辑）
- Create: `.../views/demands/components/DemandEstimateDialog.vue`（提交人天确认，**新增**）
- Create: `.../views/demands/components/DemandStartDialog.vue`（开工）
- Create: `.../views/demands/components/DemandFinishDialog.vue`（完成）

**Interfaces:**
- Consumes: T4 的 demand api、T5 的 `DEMAND_STATUS`/`formatManday`/`formatDate`、T8 的 `MarkdownEditor`/`MarkdownViewer`
- Produces: 需求全流程页面

- [ ] **Step 1: 对照旧实现重写列表页**

读 `dashboard/src/views/demands/index.vue`，用 antdv-next `Table` 重写。列：ID、标题、预估人天、实际人天、预计开工、状态（Tag，用 `tagColor(meta.type)`）、更新时间。顶部状态筛选 `Select` + 「新建需求」按钮。

**「新建需求」按钮对所有登录角色可见**（后端已开放给需求方，commit `b5dd325`），不再限 admin。

行点击进详情。

- [ ] **Step 2: 详情页**

读 `dashboard/src/views/demands/detail.vue` 重写。用 antdv-next `Descriptions` 展示字段，**描述字段用 `MarkdownViewer` 渲染**（旧的是纯文本插值）。

操作按钮按 `DEMAND_STATUS[status].actions[role]` 渲染（`role` 取 `admin` 或 `client`）：
- `edit` → 打开 DemandFormDialog
- `submitEstimate` → 打开 DemandEstimateDialog（**旧版是直接调接口，现在必须是弹窗填数据**）
- `confirmEstimate` / `accept` → 二次确认后直接调接口
- `start` / `finish` → 打开对应弹窗

`pending_acceptance` 状态且有 `accept_deadline` 时，顶部显示确认截止时间的 Alert。

**42200 冲突处理**：任何操作失败且 `error.code === 42200` 时刷新详情（让页面回到真实状态）。**旧版的 start/finish 对话框式操作漏了这条**（留档 triage），新版四个弹窗都要补上。

- [ ] **Step 3: DemandFormDialog（创建/编辑）**

**按后端新契约**：表单只有两个字段——标题（`Input`，必填，maxlength 200）+ 描述（**`MarkdownEditor`**）。不再有预估人天与预计开工日期。

编辑模式回填现有值。保存调 `createDemand` 或 `updateDemand`。

对话框宽一些（860px 左右）给编辑器留空间。

- [ ] **Step 4: DemandEstimateDialog（新增）**

表单：预估人天（`InputNumber`，min 0.5，step 0.5，必填）+ 预计开工日期（`DatePicker`，可选，清空即清除）。

提交调 `submitEstimate(id, { estimated_half_days: mandayToHalfDays(manday), planned_start_date })`。

**打开时回填当前值**——`pending_estimate` 状态下可重复提交修正预估（后端支持）。

- [ ] **Step 5: 开工与完成弹窗**

- DemandStartDialog：实际开工日期（必填），调 `startDemand(id, {actual_start_date})`
- DemandFinishDialog：实际开工 + 实际完成 + 实际人天（三项必填），调 `finishDemand`

**不要加「完成日期不能晚于今天」的前端限制**——后端已明确放开（commit `98792bd`），用户要求允许填未来日期。

- [ ] **Step 6: 联调验证**

用 admin 与 client 两个账号，走完整状态链路：

1. client 创建需求（只填标题 + markdown 描述）→ 成功，状态 draft
2. client 编辑该需求 → 成功
3. admin 提交人天确认（弹窗填 4 半天 + 预计开工）→ 状态 pending_estimate
4. admin 再次提交修正为 6 半天 → 状态不变，数值已改
5. client 确认人天 → 状态 confirmed，此时 client 与 admin 的编辑按钮都消失
6. admin 开工 → in_progress
7. admin 标记完成，**完成日期填未来日期** → 成功，状态 pending_acceptance
8. client 确认验收 → accepted
9. 详情页描述的 markdown 渲染正确

- [ ] **Step 7: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 需求模块页面迁移并接入 markdown 编辑器"
```

---

### T10: 账单模块页面

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/views/bills/index.vue`
- Create: `.../views/bills/detail.vue`

**Interfaces:**
- Consumes: T4 的 bill api、T5 的 `BILL_STATUS`/`formatAmount`/`formatManday`
- Produces: 账单列表与详情页

- [ ] **Step 1: 对照旧实现重写**

读 `dashboard/src/views/bills/index.vue` 与 `detail.vue`，用 antdv-next 组件 1:1 重写。

列表：账期、状态 Tag、总人天、总金额、生成时间。

详情：汇总信息（人天单价、基础维护费、总人天、总金额）+ 明细表（需求标题、需求状态、人天、金额、是否减免）+ 操作按钮。

- [ ] **Step 2: 操作按钮**

按 `BILL_STATUS[status].actions[role]` 渲染：`regenerate`（重新生成）、`waive`（逐行减免开关）、`share`（分享）、`revoke`（撤回）、`confirm`（需求方确认）。

同样处理 42200 冲突刷新。

- [ ] **Step 3: 联调验证**

1. admin 生成账单 → 草稿态，明细正确
2. 逐行切换减免 → 金额实时更新
3. 分享 → pending 态，admin 侧出现撤回按钮
4. client 确认 → confirmed 态

- [ ] **Step 4: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 账单模块页面迁移"
```

---

### T11: 设置、用户、审计日志、工作台

**Files:**
- Create: `dashboard-next/apps/web-antdv-next/src/views/settings/index.vue`
- Create: `.../views/users/index.vue`
- Create: `.../views/audit-logs/index.vue`
- Create: `.../views/dashboard/index.vue`（工作台）

**Interfaces:**
- Consumes: T4 的 setting/user/auditlog/dashboard api、T5 的字典
- Produces: 4 个页面

- [ ] **Step 1: 设置中心**

对照 `dashboard/src/views/settings/index.vue` 重写。参数区：人天单价、每月基础维护费、需求确认窗口、账单确认窗口、窗口口径（自然日/工作日）、周六算工作日。

**「账单包含的需求状态」改成标签按钮组**（TODO 要求，旧版是 checkbox-group）：遍历 `DEMAND_STATUS`，每项渲染成可点击的标签，**选中态填充对应语义色、未选态描边**。用 antdv-next 的 `Tag` + `checkable`（`CheckableTag`）或自绘，以观感为准。存储格式不变（逗号分隔字符串）。

节假日区：表格 + 年份筛选 + 新增单条对话框 + holiday-cn 年度 JSON 批量导入对话框。

**注意**：旧版留档过一个问题——`bill_include_statuses` 清空保存时后端报错文案不友好。前端可加一条「至少选一个状态」的提示规避。

- [ ] **Step 2: 用户管理**

对照重写：列表 + 新建/编辑对话框 + 启停开关 + 重置密码对话框。

- [ ] **Step 3: 审计日志**

对照重写：分页表格 + 筛选（操作类型、目标类型 demand/bill）+ 展开行看 detail JSON。

**旧版留档问题**：detail 为 nil 时展开是空白——新版给个「无详情」占位。

- [ ] **Step 4: 工作台**

对照 `dashboard/src/views/dashboard/` 重写：三张待办卡片（待确认人天、待验收、待确认账单，点击跳对应列表并带状态筛选）+ 出账截止提醒 Alert。

- [ ] **Step 5: 联调验证**

四个页面逐个走一遍读写操作，确认与后端联通。设置页重点验证标签按钮组的选中/取消与保存回填。

- [ ] **Step 6: lint 并提交**

```bash
cd dashboard-next && pnpm lint
git add dashboard-next/apps/web-antdv-next
git commit -m "feat: 设置、用户、审计日志、工作台页面迁移"
```

---

### T12: 集成切换

**Files:**
- Delete: `dashboard/`（旧）
- Rename: `dashboard-next/` → `dashboard/`
- Modify: `Makefile`（约 18-23 行的 dashboard 目标）
- Modify: `Dockerfile`（约 5、8、10、14、33 行）
- Modify: `.gitignore`、`.dockerignore`

**Interfaces:**
- Consumes: T9/T10/T11 完成的完整前端
- Produces: 单二进制内嵌新前端，容器可跑

- [ ] **Step 1: 删旧目录并改名**

```bash
git rm -r --cached dashboard && rm -rf dashboard
git mv dashboard-next dashboard
```

（若 `git mv` 因未跟踪文件报错，用普通 `mv` 后 `git add`。）

改完后 `.gitignore` 里 T1 加的 `dashboard-next/**` 临时规则回收，改为 `dashboard/**/node_modules/`、`dashboard/**/dist/`（monorepo 需要递归规则，旧的单层 `dashboard/node_modules/` 不够）。

`assets/dashboard/*` 与 `!assets/dashboard/.gitkeep` 两条**不要动**。

- [ ] **Step 2: Makefile**

`dashboard` 目标里的安装/构建命令改为：`cd dashboard && pnpm install --frozen-lockfile && pnpm build --filter=@vben/web-antdv-next`。

采纳 T2 的 outDir 重定向后，后面 `rm -rf assets/dashboard/*`、`cp -R dashboard/dist/.`、`touch assets/dashboard/.gitkeep` 三行**不用改**。

**`touch assets/dashboard/.gitkeep` 务必保留**——`rm -rf` 会连它一起删，而它是干净检出时让 `assets/dashboard/` 存在的唯一文件，没有它 `go:embed` 直接编译失败。

- [ ] **Step 3: Dockerfile**

**依赖安装层**：现在 `COPY dashboard/package.json dashboard/pnpm-lock.yaml dashboard/pnpm-workspace.yaml ./` 拷 3 个文件（Art 是单包）。vben workspace 需要 `apps/*/package.json` + `packages/**/package.json` + `internal/*/package.json` 全部在场才能 `--frozen-lockfile`。精简后包数固定（约 25-30 个），写成多行 COPY 保留缓存层（CI 里 `cache-to: type=gha,mode=max` 依赖这个）。

`COPY --from=dashboard-builder /src/dashboard/dist/.` —— 采纳 outDir 重定向后不变。

`corepack prepare pnpm@11.20.0 --activate` 那行的注释（「package.json 未声明 packageManager 字段」）**将不成立**——vben 根 package.json 声明了 `packageManager: pnpm@11.16.0`。简化为 `corepack enable` 并删注释，或显式对齐到 vben 声明的版本。

**运行阶段的 `COPY assets ./assets` 不要动**——它服务的是 `assets/holidays/2026.json`（运行时磁盘读取），与前端无关。

Node 基础镜像：`node:24-alpine` 需确认解析到 ≥24.12（vben engines 要求）。由于我们在 `.npmrc` 里设了 `engine-strict=false`，即使版本不符也不会被拦，但建议钉具体 tag 更稳。

- [ ] **Step 4: .dockerignore**

`dashboard/node_modules/`、`dashboard/dist/` 补成递归规则。

- [ ] **Step 5: 三项验证**

```bash
make dashboard
```
Expected: 产物落进 `assets/dashboard/`，`.gitkeep` 还在

```bash
go test ./internal/api/static/...
```
Expected: PASS

```bash
go build -o /tmp/clepsydra-test ./cmd/... && /tmp/clepsydra-test
```
访问 `/`（前端）、`/docs`（接口文档）、`/api/demands`（401 JSON）三个 URL 确认正常，然后停掉。

```bash
docker build -t clepsydra-test .
```
Expected: 构建成功

- [ ] **Step 6: 提交**

```bash
git add -u
git add dashboard .gitignore .dockerignore Makefile Dockerfile
git commit -m "chore: 前端切换为 vue-vben-admin 并对齐构建集成"
```

---

### T13: 端到端验收与收尾

**Files:**
- Create: `.superpowers/sdd/vben-acceptance-checklist.md`（验收清单）
- Modify: `TODO.md`
- Create: `.github/workflows/` 里的前端 job（可选）

**Interfaces:**
- Consumes: T12 的完整集成
- Produces: 逐条打勾的验收清单、勾选的 TODO

- [ ] **Step 1: 写验收清单并逐条执行**

按「角色 × 状态 × 动作」全枚举写成 markdown 清单，至少覆盖：

**需求模块**（2 角色 × 6 状态）：
- draft：admin 可见 edit/submitEstimate，client 可见 edit
- pending_estimate：admin 可见 edit/submitEstimate，client 可见 edit/confirmEstimate
- confirmed：admin 可见 start，client 无操作，**双方 edit 均消失**
- in_progress：admin 可见 finish
- pending_acceptance：client 可见 accept，顶部显示截止时间
- accepted：双方无操作

**账单模块**（2 角色 × 3 状态）：draft（admin: regenerate/waive/share）、pending（admin: revoke，client: confirm）、confirmed（无）

**路由与会话**：history 深链刷新、404 页、未登录跳转与回跳、刷新不掉线、`/api/auth/me` 只打一次

**视觉**：全部页面暗色模式扫一遍；markdown 编辑器在亮暗两色下的观感；markdown 预览与编辑一致

**其他**：完成日期填未来日期能提交；设置页标签按钮组选中/取消/保存回填；二进制体积对比迁移前后

- [ ] **Step 2: 补前端 CI job（可选，决策 D11）**

在 `.github/workflows/` 加一个 job：setup-node（用最新版 actions）+ corepack + `pnpm install` + `pnpm lint` + `pnpm test`。

**所有 action 必须用最新稳定版**——新增前核对官方仓库最新 release tag。

- [ ] **Step 3: 勾选 TODO**

`TODO.md` 中四项改为已完成：

```markdown
- [x] 需求创建和预估人天、预计开工日期分开
- [x] 需求创建、修改使用富文本（markdown存储）编辑器，创建时不需要预估人天和预计开工日期，需求方角色可进行需求创建、修改，当确认好人天后无法修改
- [x] 「账单包含的需求状态」做成好看的标签按钮组
- [x] 前端切换为 vue-vben-admin
```

- [ ] **Step 4: 提交**

```bash
git add TODO.md .github
git commit -m "docs: 勾选前端迁移相关待办"
```
