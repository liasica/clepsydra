# Clepsydra 前端（dashboard）设计

> 日期：2026-08-04
> 状态：已经用户逐节确认，待终审
> 关联：后端设计见 [2026-08-04-clepsydra-design.md](2026-08-04-clepsydra-design.md)

## 一、背景与已确认决策

后端（Go + echo + ent）已完成，API 按 Auth、Users、Demands、Bills、Settings、Holidays、AuditLogs、Dashboard 共 8 个模块提供，统一 `Envelope { code, message, data }` 包装，JWT Bearer 认证（单 token，72h），角色仅 `admin`（超级管理员）与 `client`（需求方）两种。`dashboard/` 目录当前为空。

本次已确认的关键决策：

1. **框架用 Art Design Pro v3.0.2**（github.com/Daymychen/art-design-pro）：单应用仓库，Vue 3.5 + TypeScript + Vite 7 + Element Plus + Pinia（持久化）。此决策覆盖先前的「Vben Admin5」与更早的「vue-starter」约定，基于 Vben 的组件库选择（antd）与 monorepo 精简方案随之作废
2. **部署用 Go embed 单二进制**：前端构建产物嵌入 Go 二进制，echo 同源托管页面与 API，无 CORS
3. **权限用 frontend 模式**：路由表静态定义，按角色过滤，不做后端下发菜单

## 二、工程形态与构建交付

### 引入方式

浅克隆 `Daymychen/art-design-pro` v3.0.2 源码，去掉其 `.git`，作为普通目录放入 `dashboard/` 入库。

改造清单：

- 删除演示内容：多套 dashboard 示例、widgets / examples 等演示页面与路由、mock 假数据登录
- 保留框架能力：布局系统、主题与暗黑模式、frontend 权限模式、axios 请求封装、Pinia 持久化
- i18n 保留机制、默认 zh-CN、隐藏语言切换入口
- dev 模式 vite proxy：`/api` → `http://localhost:8080`
- 环境要求：Node ≥ 20.19.0、pnpm ≥ 8.8.0，实施前检查

### 构建与交付

- `pnpm build` 产出 `dashboard/dist`
- Makefile 新增 `dashboard` 目标：构建前端并同步产物到 `internal/api/static/dist/`
- 新增 `internal/api/static/static.go`：`go:embed all:dist`，echo 注册静态服务 + SPA fallback——非 `/api`、`/docs` 的未知路径回退 `index.html`，支持 history 路由刷新
- 仓库仅提交 `dist/.gitkeep`（`all:` 前缀会嵌入点文件，保证目录非空、`go build` 恒通过）；构建产物整体被 gitignore 忽略，工作区始终干净。SPA fallback 检测到 `index.html` 缺失时返回「请先执行 make dashboard」纯文本提示，API 与 `/docs` 不受影响
- API base 用相对路径 `/api`，路由用 history 模式

## 三、页面与路由权限

### 路由与角色

| 路由 | 页面 | admin | client |
|---|---|---|---|
| `/login` | 登录 | — | — |
| `/dashboard` | 工作台 | ✓ | ✓ |
| `/demands`、`/demands/:id` | 需求列表/详情 | 全操作 | 查看 + 确认 |
| `/bills`、`/bills/:id` | 账单列表/详情 | 全操作 | 查看 + 确认 |
| `/settings` | 设置中心（参数 + 节假日维护） | ✓ | ✗ |
| `/users` | 用户管理 | ✓ | ✗ |
| `/audit-logs` | 审计日志（只读） | ✓ | ✗ |

### 工作台

调 `GET /api/dashboard/todos`，卡片呈现三类待办：待确认需求、待确认账单、出账提醒（截止日当天账单未分享），点击跳转对应列表页。两个角色看到各自视角的待办。

### 需求详情

操作按钮按「状态 × 角色」映射（后端状态机白名单的前端镜像）：

- admin：`draft` 可编辑、提交人天确认（submit-estimate）；`confirmed` 可开工（start）；`in_progress` 可标记完成（finish，填实际开工/完成日期与实际人天）
- client：`pending_estimate` 可确认人天（confirm-estimate）；`pending_acceptance` 可验收（accept）
- 6 个状态均显示徽标；`pending_acceptance` 状态醒目展示确认截止时间

### 账单详情

- 头部：账期、状态（3 态徽标）、单价/基础维护费快照、合计人天与合计金额、确认截止时间
- 明细表区分**计费行**（人天 × 单价，含减免标记，减免后金额为 0 但人天仍展示）与**展示行**（进行中/已确认未开工，只列预估人天与预期开始日期，不计费）
- admin：草稿可重新生成、行级减免开关、分享；已分享未确认可撤回。client：确认

## 四、前后端对接与数据流

### 请求层适配

改造 Art Design Pro 的 axios 封装：

- 统一解包 `Envelope`：`code === 0` 返回 `data`；非 0 抛业务错误并以 `message` 弹提示
- `40100`（未登录/凭证失效）→ 清除本地会话跳登录页；`42200`（状态流转冲突）→ 提示后自动刷新详情数据；其余错误码只提示
- token 由 Pinia user store 注入 `Authorization: Bearer <token>`（框架已有机制，对接即可）

### 认证流

- `POST /api/auth/login` 返回 `{ token, user: { id, name, role } }`，一并存入持久化 store
- **后端配套改动一**：补 `GET /api/auth/me`（返回当前用户，登录组），页面刷新时校验 token 并恢复用户信息；同步补 openapi.yaml（Auth 模块）
- **后端配套改动二**：第二节的 embed 静态托管
- 后端改动仅此两处，均为小改动

### 数据换算与展示字典

集中在 utils / 常量模块，配 vitest 单测：

- 人天：后端一律整数半天数（`estimated_half_days`、`actual_half_days`、`total_half_days`、`half_days`）→ 显示 `÷2` 为「x 人天」；输入组件步进 0.5、提交 `×2`
- 金额：整数元（单价约束为正偶数，半天金额无小数），千分位展示，无需分元换算
- 状态字典：需求 6 态、账单 3 态的「文案 + 徽标色 + 该状态下各角色可用操作」三合一映射，页面按钮渲染与守卫共用同一份定义

## 五、测试与质量门槛

- 纯逻辑单测（vitest）：人天换算、状态-操作映射、Envelope 解包
- 提交门槛：ESLint 无 issue + `vue-tsc` typecheck 通过
- e2e 不在本期范围

## 六、本期范围外

- e2e 测试
- 外部推送通知（邮件/企业微信）
- 账单导出（PDF/Excel）
- 移动端深度适配（框架自带基础响应式，确认操作页面桌面优先、移动可用即可）
