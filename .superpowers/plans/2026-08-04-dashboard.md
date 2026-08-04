# Clepsydra 前端（dashboard）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 Art Design Pro v3.0.2 构建 Clepsydra 管理前端（7 个页面、admin/client 双角色），以 Go embed 单二进制交付。

**Architecture:** 前端为 Art Design Pro 单应用（Vue 3.5 + TS + Vite 7 + Element Plus + Pinia 持久化），frontend 权限模式按角色过滤路由；后端补 `GET /api/auth/me` 与 embed 静态托管两处小改动；生产同源部署（`/api` 相对路径），开发经 vite proxy 转发 `localhost:8080`。

**Tech Stack:** Vue 3.5、TypeScript、Vite 7、Element Plus、Pinia、vue-router、axios、vitest；Go 侧 echo v4 + go:embed。

**Spec:** `.superpowers/specs/2026-08-04-dashboard-design.md`

## Global Constraints

- 前端框架固定为 Art Design Pro **v3.0.2**（github.com/Daymychen/art-design-pro），组件库 Element Plus
- 环境要求：Node ≥ 20.19.0、pnpm ≥ 8.8.0
- 人天一律以整数半天数与后端交互（字段 `*_half_days`），显示 `÷2`、输入 `×2`；金额为整数元
- 后端响应包装 `Envelope { code, message, data }`，`code === 0` 成功；JWT 头 `Authorization: Bearer <token>`
- 角色仅 `admin`、`client` 两种，路由 `meta.roles` 过滤
- 注释与界面文案用中文，遵循全局标点规范（中文全角标点，注释结尾不加句号）
- Go 代码提交前 `gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 无 issue；前端提交前 `pnpm lint` 无 issue
- Git 提交遵循 Conventional Commits，禁止 AI 署名
- 每个任务完成即提交，不攒大提交

## 后端既有事实（实施时直接使用，勿再考察）

- 响应辅助：`api.OK(c, data)` / `api.Fail(c, err)`（internal/api/response.go）；`api.Claims(c)` 返回 `*service.Claims{UserID, Role, Name}`
- 登录组路由：`authed := root.Group("", RequireAuth(auth))`（internal/api/router.go）；admin 组用 `RequireAdmin` 中间件
- `service.Auth` 持有 `client *ent.Client`，错误变量 `service.ErrUnauthorized`，构造 `service.NewAuth(client, cfg config.JWT)`
- handler 测试模式：`enttest.Open(t, "sqlite3", "file:<名>?mode=memory&cache=shared&_fk=1")` + `service.Seed`（admin 为 ID 1）+ `newDemandTestContext(e, method, target, body)`（自带 admin claims，定义于 demand_test.go）
- 列表接口除审计日志外不分页，`data` 直接为数组；审计日志返回 `{ total, rows }`，参数 `page`/`size`
- 后端服务地址 `:8080`（configs/config.yaml）

## 前端框架既有事实（v3.0.2 源码已考察，实施时直接使用）

- http 封装：`src/utils/http/index.ts`（拦截器读 `response.data` 的 `{ code, msg }`，成功码 `ApiStatus.success = 200`，`Authorization` 头**无 Bearer 前缀**，`request<T>` 返回 `res.data.data`）；`src/utils/http/status.ts`（ApiStatus 枚举）；`src/utils/http/error.ts`（`HttpError{ code }`、`ErrorResponse{ code, msg }`）
- 全局类型：`src/types/api/api.d.ts` 定义 `namespace Api`（`Api.Auth.LoginParams{ userName, password }` 等，需重写）；`BaseResponse` 定义于 `src/types` 下（`{ code, msg, data }`，需改 `message`）
- user store：`src/store/modules/user.ts`，含 `info: Partial<Api.Auth.UserInfo>`、`accessToken`、`setUserInfo/setLoginStatus/setToken/logOut`，localStorage 持久化
- 菜单过滤：`src/router/core/MenuProcessor.ts` 用 `userStore.info.roles`（string[]）比对路由 `meta.roles`
- 路由模块：`src/router/modules/*.ts` 每模块一文件，`modules/index.ts` 聚合导出 `routeModules`；`routes/asyncRoutes.ts` 引用它；`routes/staticRoutes.ts` 为静态路由（登录页等）
- 菜单标题：`formatMenuTitle` 先 `i18n.global.te(title)` 判断，key 不存在则原样返回 → **`meta.title` 直接写中文字符串即可**，不必动 locales
- 登录页：`src/views/auth/login/index.vue`，表单字段已是 `formData.username / formData.password`（另有演示账号 ElSelect 与 rememberPassword，需删）
- 演示路由模块：dashboard、template、widgets、examples、system、article、result、exception、safeguard、help（modules/index.ts 中聚合）
- dev 代理：`vite.config.ts` 已有 `'/api' → VITE_API_PROXY_URL`，改 `.env.development` 的 `VITE_API_PROXY_URL` 即可
- scripts：`build` = `vue-tsc --noEmit && vite build`，`lint` = `eslint`；**无 vitest**，需自行引入；自带 husky/lint-staged/commitlint（需删，子目录无 .git 会导致 `prepare` 失败）

---

### Task 1: 后端 `GET /api/auth/me`

**Files:**
- Modify: `internal/service/auth.go`（加 `Me` 方法）
- Modify: `internal/api/handler/auth.go`（加 `Me` handler）
- Modify: `internal/api/router.go`（`AuthHandler` 接口加 `Me`，authed 组注册路由）
- Modify: `internal/api/docs/openapi.yaml`（Auth 模块补 `/api/auth/me`）
- Create: `internal/api/handler/auth_test.go`

**Interfaces:**
- Consumes: `api.OK/Fail/Claims`、`service.ErrUnauthorized`、`newDemandTestContext`（既有）
- Produces: `GET /api/auth/me` → `Envelope{ data: { id, name, role } }`，凭证有效但用户已停用/删除时返回 401（前端 Task 8 依赖此接口做会话恢复）

- [ ] **Step 1: 写失败测试**

创建 `internal/api/handler/auth_test.go`：

```go
package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestAuthMeHandler 覆盖当前用户接口：正常返回本人信息，用户被停用后视为凭证失效
func TestAuthMeHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hauthme?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	svc := service.NewAuth(client, config.JWT{Secret: "s", Expire: time.Hour})
	h := NewAuth(svc)
	e := echo.New()

	c, rec := newDemandTestContext(e, http.MethodGet, "/api/auth/me", "")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Me 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, field := range []string{`"id":1`, `"name":`, `"role":"admin"`} {
		if !strings.Contains(body, field) {
			t.Errorf("响应缺少字段 %s: %s", field, body)
		}
	}

	// 停用用户后，凭证虽有效也应返回 401，保证停用即时生效
	client.User.UpdateOneID(1).SetEnabled(false).ExecX(ctx)
	c2, rec2 := newDemandTestContext(e, http.MethodGet, "/api/auth/me", "")
	_ = h.Me(c2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("停用用户应返回 401, 实际: %d, %s", rec2.Code, rec2.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/api/handler/ -run TestAuthMeHandler -count=1
```

预期：编译错误 `h.Me undefined`（Me 方法尚不存在）。

- [ ] **Step 3: 实现 service 与 handler**

`internal/service/auth.go` 文件末尾追加：

```go
// Me 按 ID 返回启用中的用户，用户不存在或已停用一律视为凭证失效
func (a *Auth) Me(ctx context.Context, userID int) (*ent.User, error) {
	u, err := a.client.User.Query().Where(user.ID(userID), user.Enabled(true)).Only(ctx)
	if err != nil {
		return nil, ErrUnauthorized
	}

	return u, nil
}
```

`internal/api/handler/auth.go` 文件末尾追加（响应结构与 Login 的 user 字段保持一致）：

```go
// Me GET /api/auth/me
func (h *Auth) Me(c echo.Context) error {
	u, err := h.svc.Me(c.Request().Context(), api.Claims(c).UserID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{
		"id":   u.ID,
		"name": u.Name,
		"role": u.Role,
	})
}
```

`internal/api/router.go` 两处修改：

```go
// AuthHandler 认证接口方法集
type AuthHandler interface {
	Login(c echo.Context) error
	Me(c echo.Context) error
}
```

在 `authed := root.Group("", RequireAuth(auth))` 之后、`authed.GET("/dashboard/todos", ...)` 之前加一行：

```go
	authed.GET("/auth/me", h.Auth.Me)
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/api/handler/ -run TestAuthMeHandler -count=1
```

预期：`ok`。再跑全量确认无回归：

```bash
go test ./... -count=1
```

- [ ] **Step 5: 补 openapi.yaml**

`internal/api/docs/openapi.yaml` 在 `/api/auth/login` 定义之后（`# 以下 8 条为登录组` 注释之前）插入：

```yaml
  /api/auth/me:
    get:
      tags: [Auth]
      operationId: authMe
      summary: 查询当前登录用户
      description: 按凭证返回当前用户精简信息，用户已停用或不存在时视为凭证失效，前端用于页面刷新后的会话恢复
      security:
        - bearerAuth: []
      responses:
        '200':
          description: 查询成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        type: object
                        description: 当前用户精简信息，字段与登录响应的 user 一致
                        properties:
                          id:
                            type: integer
                            description: 用户 ID
                          name:
                            type: string
                            description: 姓名
                          role:
                            type: string
                            enum: [admin, client]
                            description: 角色
        '401':
          $ref: '#/components/responses/Unauthorized'
        '500':
          $ref: '#/components/responses/ServerError'
```

运行文档测试确认 spec 合法：

```bash
go test ./internal/api/docs/ -count=1
```

预期：`ok`。

- [ ] **Step 6: lint 并提交**

```bash
gclint run --config .golangci.yml --new-from-rev=HEAD --timeout=10m
```

预期：无 issue。然后提交：

```bash
git add internal/service/auth.go internal/api/handler/auth.go internal/api/handler/auth_test.go internal/api/router.go internal/api/docs/openapi.yaml
git commit -m "feat: 新增当前用户接口供前端会话恢复"
```

---

### Task 2: 后端静态托管（go:embed + SPA fallback）与 Makefile

**Files:**
- Create: `internal/api/static/static.go`
- Create: `internal/api/static/static_test.go`
- Create: `internal/api/static/dist/.gitkeep`（空文件）
- Modify: `internal/api/router.go`（`Register` 末尾挂载）
- Modify: `Makefile`（`dashboard` 目标）
- Modify: `.gitignore`

**Interfaces:**
- Consumes: echo 实例（既有 `Register`）
- Produces: `static.Register(e *echo.Echo)`（生产用，读 embed）与 `static.RegisterFS(e *echo.Echo, files fs.FS)`(测试注入用)；路由规则——命中静态文件直接返回；未命中且非 `api` 前缀回退 `index.html`；`index.html` 缺失时返回 503 文本「前端未构建，请先执行 make dashboard」；`api` 前缀未命中路由仍 404

- [ ] **Step 1: 写失败测试**

创建 `internal/api/static/static_test.go`：

```go
package static

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"
)

// get 对给定 echo 实例发起 GET 请求并返回响应记录
func get(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestStaticServeAndFallback 覆盖静态命中、SPA 回退、api 前缀不回退三种路径
func TestStaticServeAndFallback(t *testing.T) {
	files := fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}

	e := echo.New()
	e.GET("/api/ping", func(c echo.Context) error { return c.String(http.StatusOK, "pong") })
	RegisterFS(e, files)

	// 根路径返回 index.html
	if rec := get(e, "/"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("根路径异常: %d, %s", rec.Code, rec.Body.String())
	}
	// 静态资源命中
	if rec := get(e, "/assets/app.js"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console") {
		t.Fatalf("静态资源异常: %d, %s", rec.Code, rec.Body.String())
	}
	// 未知前端路由回退 index.html，支持 history 模式刷新
	if rec := get(e, "/demands/3"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("SPA 回退异常: %d, %s", rec.Code, rec.Body.String())
	}
	// 已注册 api 路由正常工作
	if rec := get(e, "/api/ping"); rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Fatalf("api 路由异常: %d, %s", rec.Code, rec.Body.String())
	}
	// 未注册的 api 路径不回退页面
	if rec := get(e, "/api/nothing"); rec.Code != http.StatusNotFound {
		t.Fatalf("api 未知路径应 404, 实际: %d", rec.Code)
	}
}

// TestStaticNotBuilt 覆盖未构建（无 index.html）时的兜底提示
func TestStaticNotBuilt(t *testing.T) {
	e := echo.New()
	RegisterFS(e, fstest.MapFS{".gitkeep": {Data: []byte("")}})

	rec := get(e, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make dashboard") {
		t.Fatalf("未构建兜底异常: %d, %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/api/static/ -count=1
```

预期：编译错误（包不存在 / `RegisterFS` 未定义）。

- [ ] **Step 3: 实现 static 包**

创建空占位文件 `internal/api/static/dist/.gitkeep`（内容为空），再创建 `internal/api/static/static.go`：

```go
// Package static 托管前端构建产物并提供 SPA 回退
package static

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// distFS 前端构建产物，仓库仅含 .gitkeep 占位，make dashboard 时同步真实产物
//
//go:embed all:dist
var distFS embed.FS

// Register 以内嵌产物注册静态托管
func Register(e *echo.Echo) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	RegisterFS(e, sub)
}

// RegisterFS 以给定文件系统注册静态托管，供测试注入
func RegisterFS(e *echo.Echo, files fs.FS) {
	server := http.FileServer(http.FS(files))

	e.GET("/*", func(c echo.Context) error {
		path := strings.TrimPrefix(c.Request().URL.Path, "/")

		// api 前缀不属于页面路由，未命中时保持 404 语义
		if path == "api" || strings.HasPrefix(path, "api/") {
			return echo.ErrNotFound
		}

		if path != "" {
			if _, err := fs.Stat(files, path); err == nil {
				server.ServeHTTP(c.Response(), c.Request())
				return nil
			}
		}

		// 未命中静态文件时回退 index.html，支持 history 路由刷新
		if _, err := fs.Stat(files, "index.html"); err != nil {
			return c.String(http.StatusServiceUnavailable, "前端未构建，请先执行 make dashboard")
		}

		c.Request().URL.Path = "/"
		server.ServeHTTP(c.Response(), c.Request())
		return nil
	})
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/api/static/ -count=1
```

预期：`ok`。

- [ ] **Step 5: 挂载到 Register 并回归**

`internal/api/router.go`：import 块加 `"clepsydra/internal/api/static"`；`Register` 函数体最后（所有 `/api` 路由注册完之后）加：

```go
	// 前端静态资源与 SPA 回退，挂在最后避免遮蔽具体路由
	static.Register(e)
```

回归：

```bash
go test ./... -count=1 && go build ./...
```

预期：全部 `ok`，构建成功（embed 有 .gitkeep 占位即可通过）。

- [ ] **Step 6: Makefile 与 .gitignore**

`Makefile` 的 `.PHONY` 行改为 `.PHONY: build run test lint generate dashboard`，末尾追加：

```make
dashboard:
	cd dashboard && pnpm install --frozen-lockfile && pnpm build
	rm -rf internal/api/static/dist
	mkdir -p internal/api/static/dist
	cp -R dashboard/dist/. internal/api/static/dist/
	touch internal/api/static/dist/.gitkeep
```

`.gitignore` 末尾追加：

```
dashboard/node_modules/
dashboard/dist/
internal/api/static/dist/*
!internal/api/static/dist/.gitkeep
```

- [ ] **Step 7: lint 并提交**

```bash
gclint run --config .golangci.yml --new-from-rev=HEAD --timeout=10m
```

预期：无 issue。提交：

```bash
git add internal/api/static/ internal/api/router.go Makefile .gitignore
git commit -m "feat: 内嵌前端产物静态托管并新增 dashboard 构建目标"
```

---

### Task 3: 引入 Art Design Pro v3.0.2 源码

**Files:**
- Create: `dashboard/`（整个 v3.0.2 源码树，去除 `.git`）
- Modify: `dashboard/package.json`（删 husky/lint-staged/commitlint 相关）
- Modify: `dashboard/.env.development`（代理指向本地后端）

**Interfaces:**
- Consumes: 无
- Produces: 可运行的 `dashboard/` 工程——`pnpm dev` 起 vite 开发服务、`pnpm build` 产出 `dashboard/dist`；后续所有前端任务的工作目录

- [ ] **Step 1: 环境检查**

```bash
node -v && pnpm -v
```

预期：node ≥ 20.19.0、pnpm ≥ 8.8.0。不满足则停下向用户报告，不要自行安装。

- [ ] **Step 2: 克隆并去除仓库设施**

```bash
git clone --depth 1 -b v3.0.2 https://github.com/Daymychen/art-design-pro.git dashboard
rm -rf dashboard/.git dashboard/.github dashboard/.husky
rm -f dashboard/commitlint.config.cjs dashboard/CHANGELOG.md dashboard/CHANGELOG.zh-CN.md dashboard/README.md dashboard/README.zh-CN.md dashboard/LICENSE
```

注：LICENSE 删除的是拷贝进子目录的副本，Art Design Pro 为 MIT 协议，引用来源已记录于本计划与设计文档。

- [ ] **Step 3: 清理 package.json 的钩子与提交工具**

编辑 `dashboard/package.json`：

- `scripts` 中删除 `prepare`、`commit`、`lint:lint-staged` 三条（子目录无 `.git`，husky `prepare` 会失败）
- 删除整个 `lint-staged` 配置块（若在 package.json 内）
- `devDependencies` 中删除 `husky`、`lint-staged`、`@commitlint/cli`、`@commitlint/config-conventional`、`cz-git`、`commitizen`（按实际存在的删，不存在的跳过）
- 删除 `config.commitizen` 块（若存在）

- [ ] **Step 4: 改开发代理**

`dashboard/.env.development` 中：

```
VITE_API_PROXY_URL = http://localhost:8080
```

（`VITE_API_URL` 保持 `/`，生产同源无需改 `.env.production`。）

- [ ] **Step 5: 安装依赖并验证 dev 与 build**

```bash
cd dashboard && pnpm install
pnpm build
```

预期：install 无报错（无 husky prepare 报错），build 通过（`vue-tsc` + vite 产出 `dist/`）。dev 冒烟：

```bash
pnpm dev
```

预期：vite 启动、打开演示首页无控制台报错（人工确认后 Ctrl+C 退出）。

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add dashboard
git commit -m "feat: 引入 Art Design Pro v3.0.2 作为前端基座"
```

注：`dashboard/node_modules` 与 `dashboard/dist` 已在 Task 2 的 .gitignore 中忽略；若 Task 2 未先行完成，本步前需确认这两条忽略规则存在。

---

### Task 4: 精简演示内容并中文化默认

**Files:**
- Delete: `dashboard/src/views/`{article,change,examples,index,outside,result,safeguard,template,widgets}、`dashboard/src/views/auth/`{register,forget-password}
- Delete: `dashboard/src/mock/`
- Delete: `dashboard/src/router/modules/`{article,examples,help,result,safeguard,template,widgets,system}.ts
- Modify: `dashboard/src/router/modules/index.ts`、`dashboard/src/router/modules/dashboard.ts`
- Modify: `dashboard/src/router/routes/staticRoutes.ts`（去掉 register/forget-password 路由）
- Modify: `dashboard/src/api/`（删 `system-manage.ts`）
- Modify: 语言切换入口组件（顶栏，实施时以 `grep -rn "LanguageEnum\|setLanguage" src/components` 定位）隐藏

**Interfaces:**
- Consumes: Task 3 的工程
- Produces: 只含「登录 + 空工作台」的干净基座；`routeModules` 仅剩 dashboard 模块（Task 8 再补业务模块）；build/lint 全绿

- [ ] **Step 1: 删除演示视图与 mock**

```bash
cd dashboard
rm -rf src/views/article src/views/change src/views/examples src/views/index src/views/outside src/views/result src/views/safeguard src/views/template src/views/widgets src/views/auth/register src/views/auth/forget-password src/mock src/api/system-manage.ts
```

注：`src/views/dashboard`（工作台，Task 9 改造）、`src/views/auth/login`、`src/views/exception`（403/404/500 页，框架路由引用）与 `src/views/system`（先保留，本步结尾 grep 确认无引用后随下一步删除）暂不动。

- [ ] **Step 2: 收敛路由模块**

```bash
rm -f src/router/modules/article.ts src/router/modules/examples.ts src/router/modules/help.ts src/router/modules/result.ts src/router/modules/safeguard.ts src/router/modules/template.ts src/router/modules/widgets.ts src/router/modules/system.ts
```

重写 `src/router/modules/index.ts` 为：

```typescript
import { AppRouteRecord } from '@/types/router'
import { dashboardRoutes } from './dashboard'

/**
 * 导出所有模块化路由
 */
export const routeModules: AppRouteRecord[] = [dashboardRoutes]
```

重写 `src/router/modules/dashboard.ts` 为（去掉 analysis/ecommerce 子页，标题直接中文，角色两者可见）：

```typescript
import { AppRouteRecord } from '@/types/router'

export const dashboardRoutes: AppRouteRecord = {
  name: 'Dashboard',
  path: '/dashboard',
  component: '/index/index',
  meta: {
    title: '工作台',
    icon: 'ri:home-smile-2-line',
    roles: ['admin', 'client']
  },
  children: [
    {
      path: 'console',
      name: 'Console',
      component: '/dashboard/console',
      meta: {
        title: '工作台',
        icon: 'ri:home-smile-2-line',
        keepAlive: false,
        fixedTab: true
      }
    }
  ]
}
```

注：`component: '/index/index'` 为框架布局容器约定；若删除的 `src/views/index` 正是该容器所在（Step 4 build 会暴露），恢复 `src/views/index/index.vue` 布局容器文件并保留。`src/views/dashboard/console` 此时仍是演示页，Task 9 重写其内容。

- [ ] **Step 3: 清理静态路由与登录页残留、隐藏语言切换**

1. `src/router/routes/staticRoutes.ts`：删除 register、forget-password 两条路由记录
2. `src/views/auth/login/index.vue`：删除演示账号选择的 `ElSelect` 块（`formData.account` 与 `setupAccount` 相关代码）；登录逻辑本任务不动（仍指向旧 API，Task 8 对接）
3. 隐藏语言切换：`grep -rn "setLanguage" src/components --include="*.vue"` 定位顶栏语言组件，用 `v-if="false"` 或删除该按钮块（保留 i18n 机制本身）
4. 全局搜索删除文件的残留引用：`grep -rn "system-manage\|/mock\|views/article\|views/template\|views/widgets" src/ || true`，逐个清掉 import 与使用处

- [ ] **Step 4: 验证并修复**

```bash
pnpm build && pnpm lint
```

预期：全绿。若 build 报缺失模块，按报错逐个清理残留引用（多为菜单搜索、快捷入口等组件里的演示数据引用演示路由）。

dev 冒烟：`pnpm dev`，登录页可打开（登录本身还不通，Task 8 对接）、控制台无报错。

- [ ] **Step 5: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "refactor: 精简演示内容收敛为登录与工作台基座"
```

---

### Task 5: vitest 基建与请求层适配

**Files:**
- Modify: `dashboard/package.json`（加 vitest）
- Create: `dashboard/vitest.config.ts`
- Modify: `dashboard/src/types/` 下 `BaseResponse` 定义（`msg` → `message`，实施时 `grep -rn "BaseResponse" src/types` 定位具体文件）
- Modify: `dashboard/src/utils/http/status.ts`（成功码 0）
- Modify: `dashboard/src/utils/http/index.ts`（Bearer 前缀、`message` 字段）
- Modify: `dashboard/src/utils/http/error.ts`（`ErrorResponse.msg` → `message`，HTTP 错误提取业务 message）
- Create: `dashboard/src/utils/http/__tests__/error.test.ts`

**Interfaces:**
- Consumes: 后端契约 `Envelope{ code, message, data }`、`code === 0` 成功、`Bearer` 头（Global Constraints）
- Produces: `request.get/post/put/del<T>` 返回解包后的 `data`；`HttpError{ code, message }` 携带业务错误码（40000/40100/40300/40400/42200/50000）；`pnpm test` 可用（后续任务的单测基建）

- [ ] **Step 1: 引入 vitest**

```bash
cd dashboard && pnpm add -D vitest
```

创建 `dashboard/vitest.config.ts`：

```typescript
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    include: ['src/**/__tests__/**/*.test.ts'],
    environment: 'node'
  }
})
```

`dashboard/package.json` 的 `scripts` 加：

```json
"test": "vitest run"
```

- [ ] **Step 2: 写失败测试（错误提取逻辑）**

创建 `dashboard/src/utils/http/__tests__/error.test.ts`：

```typescript
import { describe, expect, it, vi } from 'vitest'
import { AxiosError, AxiosHeaders } from 'axios'

// error.ts 依赖 @/locales 的 $t 与 UI 提示，测试环境中以桩替换
vi.mock('@/locales', () => ({ $t: (key: string) => key }))
vi.mock('element-plus', () => ({ ElMessage: { error: vi.fn(), success: vi.fn() } }))

import { HttpError, handleError } from '../error'

/** 构造带响应体的 AxiosError */
function axiosErrorWith(status: number, body: unknown): AxiosError {
  const error = new AxiosError('Request failed', 'ERR_BAD_REQUEST')
  error.response = {
    data: body,
    status,
    statusText: '',
    headers: {},
    config: { headers: new AxiosHeaders() }
  }
  return error
}

describe('handleError', () => {
  it('从响应体提取业务错误码与 message', () => {
    const err = handleError(axiosErrorWith(422, { code: 42200, message: '当前状态不允许该操作' }))
    expect(err).toBeInstanceOf(HttpError)
    expect(err.code).toBe(42200)
    expect(err.message).toBe('当前状态不允许该操作')
  })

  it('响应体无业务结构时退回 HTTP 状态码', () => {
    const err = handleError(axiosErrorWith(502, 'Bad Gateway'))
    expect(err.code).toBe(502)
  })
})
```

- [ ] **Step 3: 运行测试确认失败**

```bash
pnpm test
```

预期：FAIL——现版 `handleError` 读的是 `msg` 字段且不优先取业务 code（具体断言失败信息以实际输出为准）。

注：测试顶部的 `vi.mock` 桩以 error.ts 实际 import 为准——若 error.ts 引用的提示组件不是 `element-plus` 而是框架内部模块，按报错把对应模块名补进 mock 列表。

- [ ] **Step 4: 实施请求层适配**

按顺序修改四处：

1. **`BaseResponse`**（`grep -rn "BaseResponse" src/types` 定位，通常在 `src/types/common/` 下）：

```typescript
/** 后端统一响应包装 */
interface BaseResponse<T = unknown> {
  /** 业务状态码，0 表示成功 */
  code: number
  /** 提示信息 */
  message: string
  /** 业务数据 */
  data: T
}
```

2. **`src/utils/http/status.ts`**：`success = 200` 改为 `success = 0`，注释同步为「成功（后端业务码）」。其余 HTTP 状态码枚举保留（重试判断仍按 HTTP 语义）。

3. **`src/utils/http/index.ts`**：
   - 请求拦截器 `request.headers.set('Authorization', accessToken)` 改为：

```typescript
    if (accessToken) request.headers.set('Authorization', `Bearer ${accessToken}`)
```

   - 响应拦截器 `const { code, msg } = response.data` 改为 `const { code, message } = response.data`，其下 `msg` 引用同步改 `message`
   - `request<T>` 内 `res.data.msg` 改为 `res.data.message`

4. **`src/utils/http/error.ts`**：
   - `ErrorResponse` 接口的 `msg` 字段改为 `message`
   - `handleError` 中提取响应体错误处，优先取 `error.response.data` 的 `{ code, message }`（两者都存在时以业务码构造 `HttpError(message, code)`），否则退回 HTTP 状态码逻辑；文件内其余 `msg` 引用同步改名

（error.ts 的具体行号以实际代码为准，改动原则：所有读 `msg` 处统一读 `message`，业务码优先于 HTTP 状态码。）

- [ ] **Step 5: 运行测试与全量验证**

```bash
pnpm test && pnpm build && pnpm lint
```

预期：单测 PASS，build/lint 全绿（`msg` 改名会被 `vue-tsc` 全量兜底，报错处逐一改完）。

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 请求层适配后端 Envelope 契约并引入 vitest"
```

---

### Task 6: 业务工具与状态字典

**Files:**
- Create: `dashboard/src/utils/clepsydra/manday.ts`
- Create: `dashboard/src/utils/clepsydra/dict.ts`
- Create: `dashboard/src/utils/clepsydra/__tests__/manday.test.ts`
- Create: `dashboard/src/utils/clepsydra/__tests__/dict.test.ts`

**Interfaces:**
- Consumes: 无（纯函数）
- Produces:
  - `halfDaysToManday(half: number): number`、`mandayToHalfDays(manday: number): number`、`formatManday(half: number | null | undefined): string`、`formatAmount(yuan: number | null | undefined): string`
  - `DEMAND_STATUS: Record<DemandStatus, StatusMeta>`、`BILL_STATUS: Record<BillStatus, StatusMeta>`，其中 `StatusMeta = { label: string; type: 'info' | 'primary' | 'warning' | 'success' | 'danger'; actions: { admin: DemandAction[] | BillAction[]; client: ... } }`
  - `type DemandStatus`、`type BillStatus`、`type DemandAction`、`type BillAction`
  - 所有页面任务（9-14）依赖这里的换算与字典

- [ ] **Step 1: 写失败测试**

创建 `dashboard/src/utils/clepsydra/__tests__/manday.test.ts`：

```typescript
import { describe, expect, it } from 'vitest'
import { formatAmount, formatManday, halfDaysToManday, mandayToHalfDays } from '../manday'

describe('人天换算', () => {
  it('半天数转人天', () => {
    expect(halfDaysToManday(16)).toBe(8)
    expect(halfDaysToManday(1)).toBe(0.5)
    expect(halfDaysToManday(0)).toBe(0)
  })

  it('人天转半天数', () => {
    expect(mandayToHalfDays(8)).toBe(16)
    expect(mandayToHalfDays(0.5)).toBe(1)
  })

  it('格式化人天，空值显示占位符', () => {
    expect(formatManday(16)).toBe('8 人天')
    expect(formatManday(1)).toBe('0.5 人天')
    expect(formatManday(null)).toBe('—')
    expect(formatManday(undefined)).toBe('—')
  })

  it('格式化金额为千分位元，空值显示占位符', () => {
    expect(formatAmount(21600)).toBe('¥21,600')
    expect(formatAmount(0)).toBe('¥0')
    expect(formatAmount(null)).toBe('—')
  })
})
```

创建 `dashboard/src/utils/clepsydra/__tests__/dict.test.ts`：

```typescript
import { describe, expect, it } from 'vitest'
import { BILL_STATUS, DEMAND_STATUS } from '../dict'

describe('状态字典', () => {
  it('需求 6 态齐全且动作按角色区分', () => {
    expect(Object.keys(DEMAND_STATUS)).toEqual([
      'draft',
      'pending_estimate',
      'confirmed',
      'in_progress',
      'pending_acceptance',
      'accepted'
    ])
    expect(DEMAND_STATUS.draft.actions.admin).toEqual(['edit', 'submitEstimate'])
    expect(DEMAND_STATUS.draft.actions.client).toEqual([])
    expect(DEMAND_STATUS.pending_estimate.actions.client).toEqual(['confirmEstimate'])
    expect(DEMAND_STATUS.confirmed.actions.admin).toEqual(['start'])
    expect(DEMAND_STATUS.in_progress.actions.admin).toEqual(['finish'])
    expect(DEMAND_STATUS.pending_acceptance.actions.client).toEqual(['accept'])
    expect(DEMAND_STATUS.accepted.actions.admin).toEqual([])
  })

  it('账单 3 态齐全且动作按角色区分', () => {
    expect(Object.keys(BILL_STATUS)).toEqual(['draft', 'pending', 'confirmed'])
    expect(BILL_STATUS.draft.actions.admin).toEqual(['regenerate', 'waive', 'share'])
    expect(BILL_STATUS.pending.actions.admin).toEqual(['revoke'])
    expect(BILL_STATUS.pending.actions.client).toEqual(['confirm'])
    expect(BILL_STATUS.confirmed.actions.admin).toEqual([])
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd dashboard && pnpm test
```

预期：FAIL（模块不存在）。

- [ ] **Step 3: 实现**

创建 `dashboard/src/utils/clepsydra/manday.ts`：

```typescript
/**
 * 人天与金额换算工具
 * 后端人天一律以整数半天数存储（1 人天 = 2），金额以整数元存储
 */

/** 半天数转人天 */
export function halfDaysToManday(half: number): number {
  return half / 2
}

/** 人天转半天数 */
export function mandayToHalfDays(manday: number): number {
  return Math.round(manday * 2)
}

/** 格式化人天展示，空值显示占位符 */
export function formatManday(half: number | null | undefined): string {
  if (half === null || half === undefined) return '—'
  return `${halfDaysToManday(half)} 人天`
}

/** 格式化金额为带千分位的元，空值显示占位符 */
export function formatAmount(yuan: number | null | undefined): string {
  if (yuan === null || yuan === undefined) return '—'
  return `¥${yuan.toLocaleString('zh-CN')}`
}
```

创建 `dashboard/src/utils/clepsydra/dict.ts`：

```typescript
/**
 * 需求与账单状态字典
 * label 为展示文案，type 为 Element Plus 标签配色，actions 为该状态下各角色可执行的操作
 * 与后端状态机白名单保持一致，页面按钮渲染与操作守卫共用此定义
 */

export type DemandStatus =
  | 'draft'
  | 'pending_estimate'
  | 'confirmed'
  | 'in_progress'
  | 'pending_acceptance'
  | 'accepted'

export type BillStatus = 'draft' | 'pending' | 'confirmed'

export type DemandAction =
  | 'edit'
  | 'submitEstimate'
  | 'confirmEstimate'
  | 'start'
  | 'finish'
  | 'accept'

export type BillAction = 'regenerate' | 'waive' | 'share' | 'revoke' | 'confirm'

type TagType = 'info' | 'primary' | 'warning' | 'success' | 'danger'

interface StatusMeta<A extends string> {
  label: string
  type: TagType
  actions: {
    admin: A[]
    client: A[]
  }
}

export const DEMAND_STATUS: Record<DemandStatus, StatusMeta<DemandAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['edit', 'submitEstimate'], client: [] }
  },
  pending_estimate: {
    label: '待确认人天',
    type: 'warning',
    actions: { admin: [], client: ['confirmEstimate'] }
  },
  confirmed: {
    label: '已确认待开工',
    type: 'primary',
    actions: { admin: ['start'], client: [] }
  },
  in_progress: {
    label: '进行中',
    type: 'primary',
    actions: { admin: ['finish'], client: [] }
  },
  pending_acceptance: {
    label: '完成待确认',
    type: 'warning',
    actions: { admin: [], client: ['accept'] }
  },
  accepted: {
    label: '已确认',
    type: 'success',
    actions: { admin: [], client: [] }
  }
}

export const BILL_STATUS: Record<BillStatus, StatusMeta<BillAction>> = {
  draft: {
    label: '草稿',
    type: 'info',
    actions: { admin: ['regenerate', 'waive', 'share'], client: [] }
  },
  pending: {
    label: '待确认',
    type: 'warning',
    actions: { admin: ['revoke'], client: ['confirm'] }
  },
  confirmed: {
    label: '已确认',
    type: 'success',
    actions: { admin: [], client: [] }
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
pnpm test && pnpm lint
```

预期：全部 PASS。

- [ ] **Step 5: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add dashboard/src/utils/clepsydra
git commit -m "feat: 新增人天金额换算与状态字典"
```

---

### Task 7: API 类型与接口模块

**Files:**
- Modify: `dashboard/src/types/api/api.d.ts`（整体重写为 Clepsydra 契约）
- Modify: `dashboard/src/api/auth.ts`（重写）
- Create: `dashboard/src/api/demand.ts`、`dashboard/src/api/bill.ts`、`dashboard/src/api/user.ts`、`dashboard/src/api/setting.ts`、`dashboard/src/api/auditlog.ts`、`dashboard/src/api/dashboard.ts`

**Interfaces:**
- Consumes: Task 5 的 `request`（返回解包后 `data`）；Task 6 的 `DemandStatus/BillStatus` 类型
- Produces: 全部页面（Task 8-14）调用的 API 函数与 `Api.*` 类型命名空间（函数签名见各文件代码）

- [ ] **Step 1: 重写全局 API 类型**

`dashboard/src/types/api/api.d.ts` 整体替换为（保留文件头部既有的 `declare namespace Api` 外层结构与 `Common` 里仍被框架引用的类型——实施时先 `grep -rn "Api\.Common\." src/ | grep -v api.d.ts` 确认哪些 Common 类型仍被引用，保留之；`Auth`/`SystemManage` 演示命名空间整体替换）：

```typescript
/** 接口数据类型定义，与 internal/api/docs/openapi.yaml 保持一致 */
declare namespace Api {
  /** 认证 */
  namespace Auth {
    /** 登录参数 */
    interface LoginParams {
      username: string
      password: string
    }

    /** 精简用户信息，登录与 me 接口共用 */
    interface SimpleUser {
      id: number
      name: string
      role: 'admin' | 'client'
    }

    /** 登录响应 */
    interface LoginData {
      token: string
      user: SimpleUser
    }

    /** 前端会话用户信息，roles 供框架菜单过滤 */
    interface UserInfo extends SimpleUser {
      roles: string[]
    }
  }

  /** 需求 */
  namespace Demand {
    type Status = import('@/utils/clepsydra/dict').DemandStatus

    /** 需求实体 */
    interface Item {
      id: number
      title: string
      description: string
      estimated_half_days: number
      estimate_confirmed_at: string | null
      estimate_confirmed_by: number | null
      planned_start_date: string | null
      actual_start_date: string | null
      actual_end_date: string | null
      actual_half_days: number | null
      status: Status
      accept_deadline: string | null
      accepted_at: string | null
      accepted_by: number | null
      accept_auto: boolean
      accept_locked: boolean
      created_at: string
      updated_at: string
    }

    /** 创建与更新共用请求体 */
    interface SaveParams {
      title: string
      description?: string
      estimated_half_days: number
      planned_start_date?: string
    }

    /** 标记完成请求体 */
    interface FinishParams {
      actual_start_date: string
      actual_end_date: string
      actual_half_days: number
    }
  }

  /** 账单 */
  namespace Bill {
    type Status = import('@/utils/clepsydra/dict').BillStatus

    /** 账单明细行 */
    interface Item {
      id: number
      demand_id: number
      demand_title: string
      demand_status: string
      half_days: number
      amount: number
      billable: boolean
      waived: boolean
      planned_start_date: string | null
      note: string
      created_at: string
    }

    /** 账单实体，items 仅详情接口返回 */
    interface Detail {
      id: number
      period: string
      status: Status
      daily_rate: number
      base_fee: number
      total_half_days: number
      total_amount: number
      shared_at: string | null
      confirm_deadline: string | null
      confirmed_at: string | null
      confirmed_by: number | null
      confirm_auto: boolean
      created_at: string
      updated_at: string
      items?: Item[]
    }
  }

  /** 用户 */
  namespace User {
    /** 用户实体 */
    interface Item {
      id: number
      username: string
      name: string
      role: 'admin' | 'client'
      enabled: boolean
      created_at: string
      updated_at: string
    }

    /** 创建请求体 */
    interface CreateParams {
      username: string
      password: string
      name: string
      role: 'admin' | 'client'
    }

    /** 更新请求体 */
    interface UpdateParams {
      name?: string
      enabled?: boolean
    }
  }

  /** 设置与节假日 */
  namespace Setting {
    /** 设置键值对，值一律为字符串 */
    type Values = Record<string, string>

    /** 节假日记录 */
    interface Holiday {
      id: number
      date: string
      type: 'holiday' | 'workday'
      name: string
    }

    /** 节假日保存条目 */
    interface HolidayEntry {
      date: string
      type: 'holiday' | 'workday'
      name?: string
    }
  }

  /** 审计日志 */
  namespace AuditLog {
    /** 日志实体 */
    interface Item {
      id: number
      operator_id: number
      operator_name: string
      action: string
      target_type: string
      target_id: number
      detail: Record<string, unknown>
      created_at: string
    }

    /** 分页查询参数 */
    interface Query {
      target_type?: string
      target_id?: number
      page?: number
      size?: number
    }

    /** 分页响应 */
    interface ListData {
      total: number
      rows: Item[]
    }
  }

  /** 工作台 */
  namespace Dashboard {
    /** 待办汇总 */
    interface Todos {
      pending_estimate_count: number
      pending_acceptance_count: number
      pending_bill_count: number
      billing_due_date: string
      billing_due_today: boolean
      prev_bill_shared: boolean
    }
  }
}
```

注：`AuditLog.Item` 的 `operator_id/operator_name` 字段名以 openapi.yaml 的 `AuditLog` schema 为准（实施时核对 `grep -n -A30 "AuditLog:" internal/api/docs/openapi.yaml`），不一致则以后端为准修正。

- [ ] **Step 2: 重写 auth API 并新建各模块**

`dashboard/src/api/auth.ts` 整体替换：

```typescript
import request from '@/utils/http'

/** 登录 */
export function fetchLogin(params: Api.Auth.LoginParams) {
  return request.post<Api.Auth.LoginData>({
    url: '/api/auth/login',
    params
  })
}

/** 查询当前登录用户，供页面刷新后的会话恢复 */
export function fetchMe() {
  return request.get<Api.Auth.SimpleUser>({
    url: '/api/auth/me'
  })
}
```

创建 `dashboard/src/api/demand.ts`：

```typescript
import request from '@/utils/http'

/** 查询需求列表，status 为空返回全部 */
export function fetchDemands(status?: Api.Demand.Status) {
  return request.get<Api.Demand.Item[]>({
    url: '/api/demands',
    params: status ? { status } : undefined
  })
}

/** 查询需求详情 */
export function fetchDemand(id: number) {
  return request.get<Api.Demand.Item>({ url: `/api/demands/${id}` })
}

/** 创建需求 */
export function createDemand(params: Api.Demand.SaveParams) {
  return request.post<Api.Demand.Item>({ url: '/api/demands', params })
}

/** 更新需求，仅草稿可改 */
export function updateDemand(id: number, params: Api.Demand.SaveParams) {
  return request.put<Api.Demand.Item>({ url: `/api/demands/${id}`, params })
}

/** 提交预估人天，draft 流转 pending_estimate */
export function submitEstimate(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/submit-estimate` })
}

/** 需求方确认预估人天，pending_estimate 流转 confirmed */
export function confirmEstimate(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/confirm-estimate` })
}

/** 标记开工，confirmed 流转 in_progress */
export function startDemand(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/start` })
}

/** 标记完成，in_progress 流转 pending_acceptance */
export function finishDemand(id: number, params: Api.Demand.FinishParams) {
  return request.post<void>({ url: `/api/demands/${id}/finish`, params })
}

/** 需求方验收，pending_acceptance 流转 accepted */
export function acceptDemand(id: number) {
  return request.post<void>({ url: `/api/demands/${id}/accept` })
}
```

创建 `dashboard/src/api/bill.ts`：

```typescript
import request from '@/utils/http'

/** 查询账单列表 */
export function fetchBills() {
  return request.get<Api.Bill.Detail[]>({ url: '/api/bills' })
}

/** 查询账单详情，含明细行 */
export function fetchBill(id: number) {
  return request.get<Api.Bill.Detail>({ url: `/api/bills/${id}` })
}

/** 生成指定账期账单草稿，已存在同账期草稿时重新生成 */
export function generateBill(period: string) {
  return request.post<Api.Bill.Detail>({ url: '/api/bills/generate', params: { period } })
}

/** 切换明细行减免状态，仅草稿账单的计费行可用 */
export function toggleWaive(billId: number, itemId: number) {
  return request.post<void>({ url: `/api/bills/${billId}/items/${itemId}/waive` })
}

/** 分享账单，draft 流转 pending */
export function shareBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/share` })
}

/** 撤回已分享账单，pending 回退 draft */
export function revokeBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/revoke` })
}

/** 需求方确认账单，pending 流转 confirmed */
export function confirmBill(id: number) {
  return request.post<void>({ url: `/api/bills/${id}/confirm` })
}
```

创建 `dashboard/src/api/user.ts`：

```typescript
import request from '@/utils/http'

/** 查询用户列表 */
export function fetchUsers() {
  return request.get<Api.User.Item[]>({ url: '/api/users' })
}

/** 创建用户 */
export function createUser(params: Api.User.CreateParams) {
  return request.post<Api.User.Item>({ url: '/api/users', params })
}

/** 更新用户姓名或启用状态 */
export function updateUser(id: number, params: Api.User.UpdateParams) {
  return request.put<Api.User.Item>({ url: `/api/users/${id}`, params })
}

/** 重置用户密码 */
export function resetPassword(id: number, password: string) {
  return request.put<void>({ url: `/api/users/${id}/password`, params: { password } })
}
```

创建 `dashboard/src/api/setting.ts`：

```typescript
import request from '@/utils/http'

/** 查询全部设置项 */
export function fetchSettings() {
  return request.get<Api.Setting.Values>({ url: '/api/settings' })
}

/** 批量更新设置项 */
export function updateSettings(values: Api.Setting.Values) {
  return request.put<void>({ url: '/api/settings', params: { values } })
}

/** 查询节假日列表 */
export function fetchHolidays(year?: number) {
  return request.get<Api.Setting.Holiday[]>({
    url: '/api/holidays',
    params: year ? { year } : undefined
  })
}

/** 批量保存节假日，按日期覆盖更新 */
export function saveHolidays(entries: Api.Setting.HolidayEntry[]) {
  return request.post<void>({ url: '/api/holidays', params: { entries } })
}

/** 删除指定日期的节假日记录 */
export function deleteHoliday(date: string) {
  return request.del<void>({ url: `/api/holidays/${date}` })
}
```

创建 `dashboard/src/api/auditlog.ts`：

```typescript
import request from '@/utils/http'

/** 分页查询审计日志 */
export function fetchAuditLogs(query: Api.AuditLog.Query) {
  return request.get<Api.AuditLog.ListData>({ url: '/api/audit-logs', params: query })
}
```

创建 `dashboard/src/api/dashboard.ts`：

```typescript
import request from '@/utils/http'

/** 查询工作台待办汇总 */
export function fetchTodos() {
  return request.get<Api.Dashboard.Todos>({ url: '/api/dashboard/todos' })
}
```

注：`fetchHolidays` 的 `year` 参数与 `holidaysList` 接口实际参数以 openapi.yaml 为准（实施时核对 `sed -n '553,590p' internal/api/docs/openapi.yaml`），无 year 参数则移除。

- [ ] **Step 3: 验证**

```bash
cd dashboard && pnpm build && pnpm lint
```

预期：全绿。旧 `Api.Auth.UserInfo`（buttons/userId/userName）的框架内引用点（user store、登录页）会报类型错——本任务只修类型引用使编译通过（如 user store 的 `info` 字段类型即用新 `Api.Auth.UserInfo`），登录流程逻辑改造留给 Task 8；若报错处涉及 Task 8 要重写的函数体，以最小修改让类型对齐。

- [ ] **Step 4: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 定义接口类型与八个模块的 API 封装"
```

---

### Task 8: 登录对接、会话恢复与路由权限

**Files:**
- Modify: `dashboard/src/views/auth/login/index.vue`（登录提交逻辑）
- Modify: `dashboard/src/store/modules/user.ts`（登录/恢复动作）
- Modify: `dashboard/src/router/guards/beforeEach.ts`（会话恢复钩子，实施时按框架现有守卫结构接入）
- Create: `dashboard/src/router/modules/demands.ts`、`bills.ts`、`settings.ts`、`users.ts`、`auditlogs.ts`
- Modify: `dashboard/src/router/modules/index.ts`
- Create: 各业务页面占位文件（`src/views/demands/index.vue` 等 6 个，仅标题占位，后续任务填充）

**Interfaces:**
- Consumes: Task 7 的 `fetchLogin/fetchMe`、`Api.Auth.UserInfo`；框架 `useUserStore` 的 `setToken/setUserInfo/setLoginStatus/logOut`
- Produces: 完整登录态——登录后菜单按角色过滤；刷新后 `fetchMe` 校验并恢复；`toUserInfo(user)` 辅助（`{ ...user, roles: [user.role] }`）；六个业务路由模块（Task 9-14 往对应 `component` 路径放真实页面）

- [ ] **Step 1: user store 增加登录与恢复动作**

`dashboard/src/store/modules/user.ts` 中（在既有 actions 附近）新增两个方法并导出（保持既有方法不动）：

```typescript
    /**
     * 后端精简用户转前端会话用户，roles 数组供菜单过滤
     */
    const toUserInfo = (user: Api.Auth.SimpleUser): Api.Auth.UserInfo => ({
      ...user,
      roles: [user.role]
    })

    /**
     * 登录：存令牌与用户信息并标记登录态
     */
    const loginByPassword = async (params: Api.Auth.LoginParams) => {
      const { fetchLogin } = await import('@/api/auth')
      const data = await fetchLogin(params)
      setToken(data.token)
      setUserInfo(toUserInfo(data.user))
      setLoginStatus(true)
    }

    /**
     * 会话恢复：持有令牌时向后端校验并刷新用户信息，失败即登出
     */
    const restoreSession = async () => {
      if (!accessToken.value) return false
      try {
        const { fetchMe } = await import('@/api/auth')
        const user = await fetchMe()
        setUserInfo(toUserInfo(user))
        setLoginStatus(true)
        return true
      } catch {
        logOut()
        return false
      }
    }
```

并把 `loginByPassword`、`restoreSession` 加入 store 的 return 导出块。

注：`setToken` 若框架签名为 `setToken(accessToken, refreshToken?)` 则只传第一个参数；实施时以实际签名为准。动态 `import('@/api/auth')` 规避 store 与 http 模块的循环引用（http 引用 user store）。

- [ ] **Step 2: 登录页对接**

`dashboard/src/views/auth/login/index.vue` 的提交函数改为调用 `userStore.loginByPassword({ username: formData.username, password: formData.password })`，成功后沿用框架原有的跳转逻辑（跳 `/`），删除演示的假登录/本地校验代码与 `rememberPassword` 无关联逻辑（保留复选框可直接删除该表单项）。

- [ ] **Step 3: 路由守卫接入会话恢复**

`dashboard/src/router/guards/beforeEach.ts`：在既有「未登录跳转登录页」判断之前，加入一次性会话恢复（模块级 `let restored = false` 标记，应用生命周期内只调一次）：

```typescript
// 页面刷新后的一次性会话恢复，令牌无效则由 restoreSession 内部登出
if (!restored) {
  restored = true
  const userStore = useUserStore()
  if (userStore.accessToken && !userStore.isLogin) {
    await userStore.restoreSession()
  }
}
```

具体插入点与既有登录态判断变量名以框架实际代码为准；原则：恢复动作先于登录态检查执行。

- [ ] **Step 4: 建业务路由模块与页面占位**

创建 6 个页面占位文件，内容统一为（以需求列表为例，标题相应调整）：

```vue
<template>
  <div class="page-content">
    <h2>需求管理</h2>
  </div>
</template>

<script setup lang="ts">
// 占位页面，后续任务实现
</script>
```

占位文件路径：
- `src/views/demands/index.vue`（需求管理）
- `src/views/demands/detail.vue`（需求详情）
- `src/views/bills/index.vue`（账单管理）
- `src/views/bills/detail.vue`（账单详情）
- `src/views/settings/index.vue`（设置中心）
- `src/views/users/index.vue`（用户管理）
- `src/views/auditlogs/index.vue`（审计日志）

创建 `dashboard/src/router/modules/demands.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'

export const demandsRoutes: AppRouteRecord = {
  name: 'Demands',
  path: '/demands',
  component: '/index/index',
  meta: {
    title: '需求管理',
    icon: 'ri:task-line',
    roles: ['admin', 'client']
  },
  children: [
    {
      path: '',
      name: 'DemandList',
      component: '/demands/index',
      meta: { title: '需求管理', icon: 'ri:task-line' }
    },
    {
      path: ':id',
      name: 'DemandDetail',
      component: '/demands/detail',
      meta: { title: '需求详情', isHide: true }
    }
  ]
}
```

创建 `dashboard/src/router/modules/bills.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'

export const billsRoutes: AppRouteRecord = {
  name: 'Bills',
  path: '/bills',
  component: '/index/index',
  meta: {
    title: '账单管理',
    icon: 'ri:bill-line',
    roles: ['admin', 'client']
  },
  children: [
    {
      path: '',
      name: 'BillList',
      component: '/bills/index',
      meta: { title: '账单管理', icon: 'ri:bill-line' }
    },
    {
      path: ':id',
      name: 'BillDetail',
      component: '/bills/detail',
      meta: { title: '账单详情', isHide: true }
    }
  ]
}
```

创建 `dashboard/src/router/modules/settings.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'

export const settingsRoutes: AppRouteRecord = {
  name: 'Settings',
  path: '/settings',
  component: '/index/index',
  meta: {
    title: '设置中心',
    icon: 'ri:settings-3-line',
    roles: ['admin']
  },
  children: [
    {
      path: '',
      name: 'SettingCenter',
      component: '/settings/index',
      meta: { title: '设置中心', icon: 'ri:settings-3-line' }
    }
  ]
}
```

创建 `dashboard/src/router/modules/users.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'

export const usersRoutes: AppRouteRecord = {
  name: 'Users',
  path: '/users',
  component: '/index/index',
  meta: {
    title: '用户管理',
    icon: 'ri:user-settings-line',
    roles: ['admin']
  },
  children: [
    {
      path: '',
      name: 'UserList',
      component: '/users/index',
      meta: { title: '用户管理', icon: 'ri:user-settings-line' }
    }
  ]
}
```

创建 `dashboard/src/router/modules/auditlogs.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'

export const auditLogsRoutes: AppRouteRecord = {
  name: 'AuditLogs',
  path: '/audit-logs',
  component: '/index/index',
  meta: {
    title: '审计日志',
    icon: 'ri:file-list-3-line',
    roles: ['admin']
  },
  children: [
    {
      path: '',
      name: 'AuditLogList',
      component: '/auditlogs/index',
      meta: { title: '审计日志', icon: 'ri:file-list-3-line' }
    }
  ]
}
```

重写 `dashboard/src/router/modules/index.ts`：

```typescript
import { AppRouteRecord } from '@/types/router'
import { dashboardRoutes } from './dashboard'
import { demandsRoutes } from './demands'
import { billsRoutes } from './bills'
import { settingsRoutes } from './settings'
import { usersRoutes } from './users'
import { auditLogsRoutes } from './auditlogs'

/**
 * 导出所有模块化路由
 */
export const routeModules: AppRouteRecord[] = [
  dashboardRoutes,
  demandsRoutes,
  billsRoutes,
  settingsRoutes,
  usersRoutes,
  auditLogsRoutes
]
```

注：`meta.isHide`（详情页不进菜单）、空 `path`（列表页作为模块默认页）的具体写法以框架 `AppRouteRecord` 类型与既有演示模块用法为准，若框架要求子路由 path 非空，列表页用 `path: 'list'` 并在模块 meta 加重定向。

- [ ] **Step 5: 联调验证（需真实后端）**

启动后端与前端：

```bash
cd /Users/liasica/projects/liasica/clepsydra && make run
```

另一终端：

```bash
cd dashboard && pnpm dev
```

手动验证清单：
1. 未登录访问 `http://localhost:<vite端口>/dashboard` → 跳登录页
2. 用 configs/config.yaml 的 admin 账号登录 → 进入工作台，侧栏可见全部 6 个菜单
3. 刷新页面 → 会话保持（Network 面板可见 `GET /api/auth/me` 成功）
4. 建一个 client 用户（curl 或后续用户页）：

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}' | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['token'])")
curl -s -X POST http://localhost:8080/api/users -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"username":"client1","password":"client123","name":"需求方一","role":"client"}'
```

5. 登出、用 client1 登录 → 侧栏仅见工作台/需求管理/账单管理
6. 手改 localStorage 里 token 为非法值后刷新 → 回登录页

```bash
pnpm build && pnpm lint && pnpm test
```

预期：全绿。

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 对接登录与会话恢复并建立角色路由"
```

---

### Task 9: 工作台待办

**Files:**
- Create: `dashboard/src/utils/clepsydra/date.ts`
- Create: `dashboard/src/utils/clepsydra/__tests__/date.test.ts`
- Rewrite: 工作台页面（`src/views/dashboard/console` 对应的入口 vue 文件，以 Task 4 后 `modules/dashboard.ts` 的 `component: '/dashboard/console'` 实际解析到的文件为准；该目录下演示子组件全部删除）

**Interfaces:**
- Consumes: `fetchTodos`（Task 7）、`useUserStore`（角色）
- Produces: `formatDate(value: string | null | undefined): string`（ISO 时间截取 `YYYY-MM-DD`，空值返回 `—`）、`formatDateTime(value: string | null | undefined): string`（`YYYY-MM-DD HH:mm`，空值 `—`）——Task 10-14 复用

- [ ] **Step 1: 写日期工具失败测试**

创建 `dashboard/src/utils/clepsydra/__tests__/date.test.ts`：

```typescript
import { describe, expect, it } from 'vitest'
import { formatDate, formatDateTime } from '../date'

describe('日期格式化', () => {
  it('ISO 时间截取日期', () => {
    expect(formatDate('2026-08-04T10:20:30+08:00')).toBe('2026-08-04')
    expect(formatDate('2026-08-04')).toBe('2026-08-04')
    expect(formatDate(null)).toBe('—')
    expect(formatDate(undefined)).toBe('—')
  })

  it('ISO 时间格式化到分钟', () => {
    expect(formatDateTime('2026-08-04T10:20:30+08:00')).toBe('2026-08-04 10:20')
    expect(formatDateTime(null)).toBe('—')
  })
})
```

- [ ] **Step 2: 运行确认失败后实现**

```bash
cd dashboard && pnpm test
```

预期：FAIL。创建 `dashboard/src/utils/clepsydra/date.ts`：

```typescript
import dayjs from 'dayjs'

/** ISO 时间截取日期部分，空值显示占位符 */
export function formatDate(value: string | null | undefined): string {
  if (!value) return '—'
  return value.slice(0, 10)
}

/** ISO 时间格式化到分钟，空值显示占位符 */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  return dayjs(value).format('YYYY-MM-DD HH:mm')
}
```

再跑 `pnpm test`，预期 PASS（dayjs 为 element-plus 既有依赖，无需新增）。

- [ ] **Step 3: 重写工作台页面**

工作台入口 vue 文件整体替换为：

```vue
<template>
  <div class="console-page">
    <el-row :gutter="16">
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="goDemands('pending_estimate')">
          <div class="todo-count">{{ todos?.pending_estimate_count ?? '-' }}</div>
          <div class="todo-label">待确认人天的需求</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="goDemands('pending_acceptance')">
          <div class="todo-count">{{ todos?.pending_acceptance_count ?? '-' }}</div>
          <div class="todo-label">完成待验收的需求</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="todo-card" @click="router.push('/bills')">
          <div class="todo-count">{{ todos?.pending_bill_count ?? '-' }}</div>
          <div class="todo-label">待确认的账单</div>
        </el-card>
      </el-col>
    </el-row>

    <el-alert
      v-if="isAdmin && todos && !todos.prev_bill_shared"
      class="billing-alert"
      :title="billingAlertText"
      :type="todos.billing_due_today ? 'error' : 'warning'"
      show-icon
      :closable="false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { fetchTodos } from '@/api/dashboard'
import { useUserStore } from '@/store/modules/user'
import type { DemandStatus } from '@/utils/clepsydra/dict'

defineOptions({ name: 'Console' })

const router = useRouter()
const userStore = useUserStore()
const isAdmin = computed(() => userStore.info.role === 'admin')

const todos = ref<Api.Dashboard.Todos>()

const billingAlertText = computed(() => {
  if (!todos.value) return ''
  return todos.value.billing_due_today
    ? `今天（${todos.value.billing_due_date}）是本月出账截止日，上月账单尚未分享`
    : `本月出账截止日为 ${todos.value.billing_due_date}，上月账单尚未分享`
})

/** 跳转需求列表并按状态筛选 */
function goDemands(status: DemandStatus) {
  router.push({ path: '/demands', query: { status } })
}

onMounted(async () => {
  todos.value = await fetchTodos()
})
</script>

<style scoped lang="scss">
.console-page {
  padding: 16px;

  .todo-card {
    margin-bottom: 16px;
    cursor: pointer;

    .todo-count {
      font-size: 32px;
      font-weight: 600;
      line-height: 1.2;
    }

    .todo-label {
      margin-top: 8px;
      color: var(--el-text-color-secondary);
    }
  }

  .billing-alert {
    margin-top: 8px;
  }
}
</style>
```

同目录下不再被引用的演示子组件文件全部删除（以 build 报错与 `grep -rn "dashboard/console" src/` 为准逐个清理）。

- [ ] **Step 4: 验证**

```bash
pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 冒烟（后端运行中）：admin 登录后工作台显示三张卡片与出账提醒；点击卡片跳转对应列表（占位页）；client 登录无出账提醒。

- [ ] **Step 5: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现工作台待办卡片与出账提醒"
```

---

### Task 10: 需求列表与详情

**Files:**
- Rewrite: `dashboard/src/views/demands/index.vue`
- Rewrite: `dashboard/src/views/demands/detail.vue`

**Interfaces:**
- Consumes: Task 7 的 demand API 全套、Task 6 的 `DEMAND_STATUS`/`formatManday`/`mandayToHalfDays`/`halfDaysToManday`、Task 9 的 `formatDate/formatDateTime`、`useUserStore`
- Produces: 完整需求管理交互；列表页读取 `route.query.status` 作为初始筛选（工作台跳转依赖）

- [ ] **Step 1: 实现列表页**

`dashboard/src/views/demands/index.vue` 整体替换为：

```vue
<template>
  <div class="demand-page">
    <div class="toolbar">
      <el-select v-model="status" placeholder="全部状态" clearable style="width: 180px" @change="load">
        <el-option
          v-for="(meta, key) in DEMAND_STATUS"
          :key="key"
          :label="meta.label"
          :value="key"
        />
      </el-select>
      <el-button v-if="isAdmin" type="primary" @click="openCreate">新建需求</el-button>
    </div>

    <el-table v-loading="loading" :data="list" @row-click="goDetail">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
      <el-table-column label="预估人天" width="100">
        <template #default="{ row }">{{ formatManday(row.estimated_half_days) }}</template>
      </el-table-column>
      <el-table-column label="实际人天" width="100">
        <template #default="{ row }">{{ formatManday(row.actual_half_days) }}</template>
      </el-table-column>
      <el-table-column label="预计开工" width="110">
        <template #default="{ row }">{{ formatDate(row.planned_start_date) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag :type="DEMAND_STATUS[row.status].type">{{ DEMAND_STATUS[row.status].label }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
      </el-table-column>
    </el-table>

    <demand-form-dialog v-model="createVisible" @saved="load" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchDemands } from '@/api/demand'
import { useUserStore } from '@/store/modules/user'
import { DEMAND_STATUS, type DemandStatus } from '@/utils/clepsydra/dict'
import { formatManday } from '@/utils/clepsydra/manday'
import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
import DemandFormDialog from './components/DemandFormDialog.vue'

defineOptions({ name: 'DemandList' })

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const isAdmin = computed(() => userStore.info.role === 'admin')

const status = ref<DemandStatus | ''>((route.query.status as DemandStatus) || '')
const list = ref<Api.Demand.Item[]>([])
const loading = ref(false)
const createVisible = ref(false)

/** 加载需求列表 */
async function load() {
  loading.value = true
  try {
    list.value = await fetchDemands(status.value || undefined)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createVisible.value = true
}

function goDetail(row: Api.Demand.Item) {
  router.push(`/demands/${row.id}`)
}

onMounted(load)
</script>

<style scoped lang="scss">
.demand-page {
  padding: 16px;

  .toolbar {
    display: flex;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  :deep(.el-table__row) {
    cursor: pointer;
  }
}
</style>
```

- [ ] **Step 2: 实现新建/编辑对话框组件**

创建 `dashboard/src/views/demands/components/DemandFormDialog.vue`：

```vue
<template>
  <el-dialog
    :model-value="modelValue"
    :title="demand ? '编辑需求' : '新建需求'"
    width="520px"
    @update:model-value="emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="标题" prop="title">
        <el-input v-model.trim="form.title" maxlength="200" />
      </el-form-item>
      <el-form-item label="描述" prop="description">
        <el-input v-model="form.description" type="textarea" :rows="3" />
      </el-form-item>
      <el-form-item label="预估人天" prop="manday">
        <el-input-number v-model="form.manday" :min="0.5" :step="0.5" />
      </el-form-item>
      <el-form-item label="预计开工" prop="plannedStartDate">
        <el-date-picker v-model="form.plannedStartDate" type="date" value-format="YYYY-MM-DD" clearable />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { createDemand, updateDemand } from '@/api/demand'
import { halfDaysToManday, mandayToHalfDays } from '@/utils/clepsydra/manday'
import { formatDate } from '@/utils/clepsydra/date'

const props = defineProps<{
  modelValue: boolean
  /** 传入则为编辑模式 */
  demand?: Api.Demand.Item
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  title: '',
  description: '',
  manday: 1,
  plannedStartDate: '' as string | ''
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  manday: [{ required: true, message: '请输入预估人天', trigger: 'change' }]
}

/** 对话框打开时按编辑对象回填表单 */
function syncForm() {
  form.title = props.demand?.title ?? ''
  form.description = props.demand?.description ?? ''
  form.manday = props.demand ? halfDaysToManday(props.demand.estimated_half_days) : 1
  form.plannedStartDate = props.demand?.planned_start_date
    ? formatDate(props.demand.planned_start_date)
    : ''
}

/** 保存：编辑走更新接口，否则创建 */
async function save() {
  await formRef.value?.validate()
  saving.value = true
  try {
    const params: Api.Demand.SaveParams = {
      title: form.title,
      description: form.description || undefined,
      estimated_half_days: mandayToHalfDays(form.manday),
      planned_start_date: form.plannedStartDate || undefined
    }
    if (props.demand) {
      await updateDemand(props.demand.id, params)
    } else {
      await createDemand(params)
    }
    ElMessage.success('已保存')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 3: 实现详情页**

`dashboard/src/views/demands/detail.vue` 整体替换为：

```vue
<template>
  <div v-loading="loading" class="demand-detail">
    <el-card v-if="demand">
      <template #header>
        <div class="card-header">
          <span class="title">#{{ demand.id }} {{ demand.title }}</span>
          <el-tag :type="statusMeta.type">{{ statusMeta.label }}</el-tag>
        </div>
      </template>

      <el-alert
        v-if="demand.status === 'pending_acceptance' && demand.accept_deadline"
        :title="`确认截止时间：${formatDateTime(demand.accept_deadline)}，逾期将自动确认`"
        type="warning"
        show-icon
        :closable="false"
        class="deadline-alert"
      />

      <el-descriptions :column="2" border>
        <el-descriptions-item label="描述" :span="2">{{ demand.description || '—' }}</el-descriptions-item>
        <el-descriptions-item label="预估人天">{{ formatManday(demand.estimated_half_days) }}</el-descriptions-item>
        <el-descriptions-item label="人天确认时间">
          {{ formatDateTime(demand.estimate_confirmed_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="预计开工">{{ formatDate(demand.planned_start_date) }}</el-descriptions-item>
        <el-descriptions-item label="实际开工">{{ formatDate(demand.actual_start_date) }}</el-descriptions-item>
        <el-descriptions-item label="实际完成">{{ formatDate(demand.actual_end_date) }}</el-descriptions-item>
        <el-descriptions-item label="实际人天">{{ formatManday(demand.actual_half_days) }}</el-descriptions-item>
        <el-descriptions-item label="验收时间">{{ formatDateTime(demand.accepted_at) }}</el-descriptions-item>
        <el-descriptions-item label="验收方式">{{ acceptWay }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatDateTime(demand.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatDateTime(demand.updated_at) }}</el-descriptions-item>
      </el-descriptions>

      <div class="actions">
        <template v-if="actions.includes('edit')">
          <el-button type="primary" @click="editVisible = true">编辑</el-button>
        </template>
        <el-button v-if="actions.includes('submitEstimate')" type="warning" @click="run('提交人天确认', () => submitEstimate(demand!.id))">
          提交人天确认
        </el-button>
        <el-button v-if="actions.includes('confirmEstimate')" type="primary" @click="run('确认预估人天', () => confirmEstimate(demand!.id))">
          确认人天
        </el-button>
        <el-button v-if="actions.includes('start')" type="primary" @click="run('标记开工', () => startDemand(demand!.id))">
          开工
        </el-button>
        <el-button v-if="actions.includes('finish')" type="warning" @click="finishVisible = true">标记完成</el-button>
        <el-button v-if="actions.includes('accept')" type="success" @click="run('确认验收', () => acceptDemand(demand!.id))">
          确认验收
        </el-button>
      </div>
    </el-card>

    <demand-form-dialog v-model="editVisible" :demand="demand" @saved="load" />
    <demand-finish-dialog v-model="finishVisible" :demand-id="demandId" @finished="load" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  acceptDemand,
  confirmEstimate,
  fetchDemand,
  startDemand,
  submitEstimate
} from '@/api/demand'
import { useUserStore } from '@/store/modules/user'
import { DEMAND_STATUS, type DemandAction } from '@/utils/clepsydra/dict'
import { formatManday } from '@/utils/clepsydra/manday'
import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
import { HttpError } from '@/utils/http/error'
import DemandFormDialog from './components/DemandFormDialog.vue'
import DemandFinishDialog from './components/DemandFinishDialog.vue'

defineOptions({ name: 'DemandDetail' })

const route = useRoute()
const userStore = useUserStore()

const demandId = Number(route.params.id)
const demand = ref<Api.Demand.Item>()
const loading = ref(false)
const editVisible = ref(false)
const finishVisible = ref(false)

const statusMeta = computed(() => DEMAND_STATUS[demand.value!.status])

const actions = computed<DemandAction[]>(() => {
  if (!demand.value) return []
  const role = userStore.info.role === 'admin' ? 'admin' : 'client'
  return DEMAND_STATUS[demand.value.status].actions[role]
})

const acceptWay = computed(() => {
  if (!demand.value?.accepted_at) return '—'
  if (demand.value.accept_locked) return '出账锁定自动确认'
  if (demand.value.accept_auto) return '逾期自动确认'
  return '需求方确认'
})

/** 加载详情 */
async function load() {
  loading.value = true
  try {
    demand.value = await fetchDemand(demandId)
  } finally {
    loading.value = false
  }
}

/**
 * 通用操作执行：二次确认后调接口并刷新
 * 42200 状态冲突时刷新详情让页面回到真实状态
 */
async function run(name: string, action: () => Promise<unknown>) {
  await ElMessageBox.confirm(`确定${name}吗？`, '操作确认', { type: 'warning' })
  try {
    await action()
    ElMessage.success(`${name}成功`)
  } catch (error) {
    if (error instanceof HttpError && error.code === 42200) await load()
    return
  }
  await load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.demand-detail {
  padding: 16px;

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;

    .title {
      font-size: 16px;
      font-weight: 600;
    }
  }

  .deadline-alert {
    margin-bottom: 16px;
  }

  .actions {
    margin-top: 16px;
  }
}
</style>
```

- [ ] **Step 4: 实现完成对话框**

创建 `dashboard/src/views/demands/components/DemandFinishDialog.vue`：

```vue
<template>
  <el-dialog
    :model-value="modelValue"
    title="标记完成"
    width="480px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="实际开工" prop="actualStartDate">
        <el-date-picker v-model="form.actualStartDate" type="date" value-format="YYYY-MM-DD" />
      </el-form-item>
      <el-form-item label="实际完成" prop="actualEndDate">
        <el-date-picker v-model="form.actualEndDate" type="date" value-format="YYYY-MM-DD" />
      </el-form-item>
      <el-form-item label="实际人天" prop="manday">
        <el-input-number v-model="form.manday" :min="0.5" :step="0.5" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">提交</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { finishDemand } from '@/api/demand'
import { mandayToHalfDays } from '@/utils/clepsydra/manday'

const props = defineProps<{
  modelValue: boolean
  demandId: number
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  finished: []
}>()

const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  actualStartDate: '',
  actualEndDate: '',
  manday: 1
})

const rules: FormRules = {
  actualStartDate: [{ required: true, message: '请选择实际开工日期', trigger: 'change' }],
  actualEndDate: [{ required: true, message: '请选择实际完成日期', trigger: 'change' }],
  manday: [{ required: true, message: '请输入实际人天', trigger: 'change' }]
}

/** 提交完成信息，转入待验收状态 */
async function save() {
  await formRef.value?.validate()
  saving.value = true
  try {
    await finishDemand(props.demandId, {
      actual_start_date: form.actualStartDate,
      actual_end_date: form.actualEndDate,
      actual_half_days: mandayToHalfDays(form.manday)
    })
    ElMessage.success('已提交，等待需求方验收')
    emit('update:modelValue', false)
    emit('finished')
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 5: 验证**

```bash
cd dashboard && pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 联调（后端运行中）走完整链路：admin 新建 → 提交人天确认 → client 确认人天 → admin 开工 → 标记完成 → client 验收；每步后状态徽标与按钮随之变化；工作台卡片跳转的初始筛选生效。

- [ ] **Step 6: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现需求列表详情与全状态流转操作"
```

---

### Task 11: 账单列表与详情

**Files:**
- Rewrite: `dashboard/src/views/bills/index.vue`
- Rewrite: `dashboard/src/views/bills/detail.vue`

**Interfaces:**
- Consumes: Task 7 的 bill API 全套、Task 6 的 `BILL_STATUS`/`formatManday`/`formatAmount`、Task 9 的 `formatDate/formatDateTime`、Task 10 同款 `run` 模式
- Produces: 完整账单管理交互

- [ ] **Step 1: 实现列表页**

`dashboard/src/views/bills/index.vue` 整体替换为：

```vue
<template>
  <div class="bill-page">
    <div class="toolbar">
      <span></span>
      <div v-if="isAdmin" class="generate">
        <el-date-picker v-model="period" type="month" value-format="YYYY-MM" placeholder="选择账期" />
        <el-button type="primary" :disabled="!period" @click="generate">生成账单</el-button>
      </div>
    </div>

    <el-table v-loading="loading" :data="list" @row-click="goDetail">
      <el-table-column prop="period" label="账期" width="100" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="BILL_STATUS[row.status].type">{{ BILL_STATUS[row.status].label }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="计费人天" width="110">
        <template #default="{ row }">{{ formatManday(row.total_half_days) }}</template>
      </el-table-column>
      <el-table-column label="账单总额" width="130">
        <template #default="{ row }">{{ formatAmount(row.total_amount) }}</template>
      </el-table-column>
      <el-table-column label="分享时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.shared_at) }}</template>
      </el-table-column>
      <el-table-column label="确认截止" width="150">
        <template #default="{ row }">{{ formatDateTime(row.confirm_deadline) }}</template>
      </el-table-column>
      <el-table-column label="确认时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.confirmed_at) }}</template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { fetchBills, generateBill } from '@/api/bill'
import { useUserStore } from '@/store/modules/user'
import { BILL_STATUS } from '@/utils/clepsydra/dict'
import { formatAmount, formatManday } from '@/utils/clepsydra/manday'
import { formatDateTime } from '@/utils/clepsydra/date'

defineOptions({ name: 'BillList' })

const router = useRouter()
const userStore = useUserStore()
const isAdmin = computed(() => userStore.info.role === 'admin')

const list = ref<Api.Bill.Detail[]>([])
const loading = ref(false)
const period = ref('')

/** 加载账单列表 */
async function load() {
  loading.value = true
  try {
    list.value = await fetchBills()
  } finally {
    loading.value = false
  }
}

/** 生成指定账期的账单草稿 */
async function generate() {
  const bill = await generateBill(period.value)
  ElMessage.success(`账期 ${period.value} 账单已生成`)
  router.push(`/bills/${bill.id}`)
}

function goDetail(row: Api.Bill.Detail) {
  router.push(`/bills/${row.id}`)
}

onMounted(load)
</script>

<style scoped lang="scss">
.bill-page {
  padding: 16px;

  .toolbar {
    display: flex;
    justify-content: space-between;
    margin-bottom: 16px;

    .generate {
      display: flex;
      gap: 8px;
    }
  }

  :deep(.el-table__row) {
    cursor: pointer;
  }
}
</style>
```

- [ ] **Step 2: 实现详情页**

`dashboard/src/views/bills/detail.vue` 整体替换为：

```vue
<template>
  <div v-loading="loading" class="bill-detail">
    <el-card v-if="bill">
      <template #header>
        <div class="card-header">
          <span class="title">{{ bill.period }} 账单</span>
          <el-tag :type="statusMeta.type">{{ statusMeta.label }}</el-tag>
        </div>
      </template>

      <el-alert
        v-if="bill.status === 'pending' && bill.confirm_deadline"
        :title="`确认截止时间：${formatDateTime(bill.confirm_deadline)}，逾期将自动确认`"
        type="warning"
        show-icon
        :closable="false"
        class="deadline-alert"
      />

      <el-descriptions :column="3" border>
        <el-descriptions-item label="人天单价">{{ formatAmount(bill.daily_rate) }}</el-descriptions-item>
        <el-descriptions-item label="基础维护费">{{ formatAmount(bill.base_fee) }}</el-descriptions-item>
        <el-descriptions-item label="计费人天">{{ formatManday(bill.total_half_days) }}</el-descriptions-item>
        <el-descriptions-item label="账单总额">{{ formatAmount(bill.total_amount) }}</el-descriptions-item>
        <el-descriptions-item label="分享时间">{{ formatDateTime(bill.shared_at) }}</el-descriptions-item>
        <el-descriptions-item label="确认时间">
          {{ formatDateTime(bill.confirmed_at) }}{{ bill.confirm_auto ? '（逾期自动确认）' : '' }}
        </el-descriptions-item>
      </el-descriptions>

      <h4 class="items-title">账单明细</h4>
      <el-table :data="bill.items ?? []">
        <el-table-column prop="demand_id" label="需求 ID" width="90" />
        <el-table-column prop="demand_title" label="需求标题" min-width="180" show-overflow-tooltip />
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag :type="row.billable ? 'primary' : 'info'">{{ row.billable ? '计费' : '展示' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态快照" width="120">
          <template #default="{ row }">{{ demandStatusLabel(row.demand_status) }}</template>
        </el-table-column>
        <el-table-column label="人天" width="90">
          <template #default="{ row }">{{ formatManday(row.half_days) }}</template>
        </el-table-column>
        <el-table-column label="金额" width="110">
          <template #default="{ row }">{{ formatAmount(row.amount) }}</template>
        </el-table-column>
        <el-table-column label="预计开工" width="110">
          <template #default="{ row }">{{ formatDate(row.planned_start_date) }}</template>
        </el-table-column>
        <el-table-column label="减免" width="110">
          <template #default="{ row }">
            <el-switch
              v-if="row.billable && canWaive"
              :model-value="row.waived"
              @click.stop
              @change="waive(row)"
            />
            <el-tag v-else-if="row.waived" type="danger">已减免</el-tag>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" min-width="120" show-overflow-tooltip />
      </el-table>

      <div class="actions">
        <el-button v-if="actions.includes('regenerate')" @click="regenerate">重新生成</el-button>
        <el-button v-if="actions.includes('share')" type="primary" @click="run('分享账单', () => shareBill(bill!.id))">
          分享给需求方
        </el-button>
        <el-button v-if="actions.includes('revoke')" type="warning" @click="run('撤回账单', () => revokeBill(bill!.id))">
          撤回
        </el-button>
        <el-button v-if="actions.includes('confirm')" type="success" @click="run('确认账单', () => confirmBill(bill!.id))">
          确认账单
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  confirmBill,
  fetchBill,
  generateBill,
  revokeBill,
  shareBill,
  toggleWaive
} from '@/api/bill'
import { useUserStore } from '@/store/modules/user'
import { BILL_STATUS, DEMAND_STATUS, type BillAction, type DemandStatus } from '@/utils/clepsydra/dict'
import { formatAmount, formatManday } from '@/utils/clepsydra/manday'
import { formatDate, formatDateTime } from '@/utils/clepsydra/date'
import { HttpError } from '@/utils/http/error'

defineOptions({ name: 'BillDetail' })

const route = useRoute()
const userStore = useUserStore()

const billId = Number(route.params.id)
const bill = ref<Api.Bill.Detail>()
const loading = ref(false)

const statusMeta = computed(() => BILL_STATUS[bill.value!.status])

const actions = computed<BillAction[]>(() => {
  if (!bill.value) return []
  const role = userStore.info.role === 'admin' ? 'admin' : 'client'
  return BILL_STATUS[bill.value.status].actions[role]
})

const canWaive = computed(() => actions.value.includes('waive'))

/** 明细状态快照转中文标签，未知值原样展示 */
function demandStatusLabel(status: string) {
  return DEMAND_STATUS[status as DemandStatus]?.label ?? status
}

/** 加载账单详情 */
async function load() {
  loading.value = true
  try {
    bill.value = await fetchBill(billId)
  } finally {
    loading.value = false
  }
}

/** 通用操作：二次确认后执行并刷新，42200 冲突时刷新回真实状态 */
async function run(name: string, action: () => Promise<unknown>) {
  await ElMessageBox.confirm(`确定${name}吗？`, '操作确认', { type: 'warning' })
  try {
    await action()
    ElMessage.success(`${name}成功`)
  } catch (error) {
    if (error instanceof HttpError && error.code === 42200) await load()
    return
  }
  await load()
}

/** 切换明细行减免并重算总额 */
async function waive(row: Api.Bill.Item) {
  try {
    await toggleWaive(billId, row.id)
  } finally {
    await load()
  }
}

/** 草稿重新生成：按当前账期重算明细 */
async function regenerate() {
  await ElMessageBox.confirm('重新生成将丢弃当前草稿的减免调整，确定吗？', '操作确认', {
    type: 'warning'
  })
  await generateBill(bill.value!.period)
  ElMessage.success('已重新生成')
  await load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.bill-detail {
  padding: 16px;

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;

    .title {
      font-size: 16px;
      font-weight: 600;
    }
  }

  .deadline-alert {
    margin-bottom: 16px;
  }

  .items-title {
    margin: 20px 0 12px;
  }

  .actions {
    margin-top: 16px;
  }
}
</style>
```

- [ ] **Step 3: 验证**

```bash
cd dashboard && pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 联调：admin 生成上月账单 → 详情可见计费行/展示行 → 切换减免总额变化 → 分享 → client 登录确认；撤回后回草稿可再调整。

- [ ] **Step 4: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现账单列表详情与减免分享确认操作"
```

---

### Task 12: 设置中心（参数 + 节假日维护）

**Files:**
- Rewrite: `dashboard/src/views/settings/index.vue`

**Interfaces:**
- Consumes: Task 7 的 `fetchSettings/updateSettings/fetchHolidays/saveHolidays/deleteHoliday`、Task 6 换算工具
- Produces: 完整设置中心页面（admin 专属）

- [ ] **Step 1: 实现设置中心页**

`dashboard/src/views/settings/index.vue` 整体替换为：

```vue
<template>
  <div class="settings-page">
    <el-card header="参数设置" class="params-card">
      <el-form v-loading="loading" :model="form" label-width="200px" style="max-width: 560px">
        <el-form-item label="人天单价（元）">
          <el-input-number v-model="form.dailyRate" :min="2" :step="2" />
          <span class="tip">须为正偶数，保证 0.5 人天金额为整数</span>
        </el-form-item>
        <el-form-item label="每月基础维护费（元）">
          <el-input-number v-model="form.baseFee" :min="0" :step="100" />
        </el-form-item>
        <el-form-item label="需求确认窗口（天）">
          <el-input-number v-model="form.demandConfirmWindow" :min="1" />
        </el-form-item>
        <el-form-item label="账单确认窗口（天）">
          <el-input-number v-model="form.billConfirmWindow" :min="1" />
        </el-form-item>
        <el-form-item label="窗口口径">
          <el-radio-group v-model="form.windowUnit">
            <el-radio value="natural">自然日</el-radio>
            <el-radio value="workday">工作日</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="周六算工作日">
          <el-switch v-model="form.saturdayAsWorkday" />
        </el-form-item>
        <el-form-item label="账单包含的需求状态">
          <el-checkbox-group v-model="form.billIncludeStatuses">
            <el-checkbox v-for="(meta, key) in DEMAND_STATUS" :key="key" :value="key">
              {{ meta.label }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveSettings">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="holiday-card">
      <template #header>
        <div class="holiday-header">
          <span>节假日维护</span>
          <div class="holiday-actions">
            <el-date-picker
              v-model="year"
              type="year"
              value-format="YYYY"
              placeholder="筛选年份"
              style="width: 120px"
              @change="loadHolidays"
            />
            <el-button @click="importVisible = true">导入 holiday-cn</el-button>
            <el-button type="primary" @click="addVisible = true">新增</el-button>
          </div>
        </div>
      </template>

      <el-table v-loading="holidayLoading" :data="holidays" max-height="480">
        <el-table-column prop="date" label="日期" width="120" />
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-tag :type="row.type === 'holiday' ? 'danger' : 'warning'">
              {{ row.type === 'holiday' ? '休息日' : '调休补班' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button type="danger" link @click="removeHoliday(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增单条 -->
    <el-dialog v-model="addVisible" title="新增节假日" width="420px">
      <el-form :model="addForm" label-width="70px">
        <el-form-item label="日期">
          <el-date-picker v-model="addForm.date" type="date" value-format="YYYY-MM-DD" />
        </el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="addForm.type">
            <el-radio value="holiday">休息日</el-radio>
            <el-radio value="workday">调休补班</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model.trim="addForm.name" placeholder="如：春节" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!addForm.date" @click="addHoliday">保存</el-button>
      </template>
    </el-dialog>

    <!-- 批量导入 holiday-cn 年度 JSON -->
    <el-dialog v-model="importVisible" title="导入 holiday-cn 年度数据" width="560px">
      <p class="import-tip">
        粘贴 holiday-cn（github.com/NateScarlet/holiday-cn）年度 JSON 文件内容，按日期覆盖更新
      </p>
      <el-input v-model="importText" type="textarea" :rows="10" placeholder='{"year": 2026, "days": [...]}' />
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!importText.trim()" @click="importHolidays">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteHoliday,
  fetchHolidays,
  fetchSettings,
  saveHolidays,
  updateSettings
} from '@/api/setting'
import { DEMAND_STATUS } from '@/utils/clepsydra/dict'

defineOptions({ name: 'SettingCenter' })

const loading = ref(false)
const saving = ref(false)

const form = reactive({
  dailyRate: 1200,
  baseFee: 12000,
  demandConfirmWindow: 5,
  billConfirmWindow: 3,
  windowUnit: 'natural',
  saturdayAsWorkday: true,
  billIncludeStatuses: [] as string[]
})

/** 拉取设置并回填表单，后端值一律为字符串 */
async function loadSettings() {
  loading.value = true
  try {
    const values = await fetchSettings()
    form.dailyRate = Number(values.daily_rate ?? 1200)
    form.baseFee = Number(values.base_fee ?? 12000)
    form.demandConfirmWindow = Number(values.demand_confirm_window ?? 5)
    form.billConfirmWindow = Number(values.bill_confirm_window ?? 3)
    form.windowUnit = values.window_unit ?? 'natural'
    form.saturdayAsWorkday = values.saturday_as_workday !== 'false'
    form.billIncludeStatuses = (values.bill_include_statuses ?? '').split(',').filter(Boolean)
  } finally {
    loading.value = false
  }
}

/** 保存设置，全部转回字符串 */
async function saveSettings() {
  saving.value = true
  try {
    await updateSettings({
      daily_rate: String(form.dailyRate),
      base_fee: String(form.baseFee),
      demand_confirm_window: String(form.demandConfirmWindow),
      bill_confirm_window: String(form.billConfirmWindow),
      window_unit: form.windowUnit,
      saturday_as_workday: String(form.saturdayAsWorkday),
      bill_include_statuses: form.billIncludeStatuses.join(',')
    })
    ElMessage.success('设置已保存')
  } finally {
    saving.value = false
  }
}

const holidays = ref<Api.Setting.Holiday[]>([])
const holidayLoading = ref(false)
const year = ref(String(new Date().getFullYear()))
const addVisible = ref(false)
const importVisible = ref(false)
const importText = ref('')

const addForm = reactive({
  date: '',
  type: 'holiday' as 'holiday' | 'workday',
  name: ''
})

/** 加载节假日列表并按年份过滤 */
async function loadHolidays() {
  holidayLoading.value = true
  try {
    const all = await fetchHolidays()
    holidays.value = year.value ? all.filter((h) => h.date.startsWith(year.value)) : all
  } finally {
    holidayLoading.value = false
  }
}

/** 新增单条节假日 */
async function addHoliday() {
  await saveHolidays([{ date: addForm.date, type: addForm.type, name: addForm.name || undefined }])
  ElMessage.success('已保存')
  addVisible.value = false
  addForm.date = ''
  addForm.name = ''
  await loadHolidays()
}

/** 解析 holiday-cn 年度 JSON 并批量导入 */
async function importHolidays() {
  let entries: Api.Setting.HolidayEntry[]
  try {
    const parsed = JSON.parse(importText.value) as {
      days?: { name: string; date: string; isOffDay: boolean }[]
    }
    if (!Array.isArray(parsed.days) || parsed.days.length === 0) throw new Error('缺少 days')
    entries = parsed.days.map((d) => ({
      date: d.date,
      type: d.isOffDay ? 'holiday' : 'workday',
      name: d.name
    }))
  } catch {
    ElMessage.error('JSON 解析失败，请粘贴完整的 holiday-cn 年度文件内容')
    return
  }

  await saveHolidays(entries)
  ElMessage.success(`已导入 ${entries.length} 条`)
  importVisible.value = false
  importText.value = ''
  await loadHolidays()
}

/** 删除单条节假日 */
async function removeHoliday(row: Api.Setting.Holiday) {
  await ElMessageBox.confirm(`确定删除 ${row.date}（${row.name || '未命名'}）吗？`, '操作确认', {
    type: 'warning'
  })
  await deleteHoliday(row.date)
  ElMessage.success('已删除')
  await loadHolidays()
}

onMounted(() => {
  loadSettings()
  loadHolidays()
})
</script>

<style scoped lang="scss">
.settings-page {
  padding: 16px;

  .params-card {
    margin-bottom: 16px;

    .tip {
      margin-left: 12px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }
  }

  .holiday-header {
    display: flex;
    align-items: center;
    justify-content: space-between;

    .holiday-actions {
      display: flex;
      gap: 8px;
    }
  }

  .import-tip {
    margin-bottom: 8px;
    color: var(--el-text-color-secondary);
  }
}
</style>
```

注：`bill_include_statuses` 复选框全量列出 6 态供选择，与后端设置项语义一致（默认勾选 accepted、in_progress、confirmed 三态来自后端返回值，前端不写死）。

- [ ] **Step 2: 验证**

```bash
cd dashboard && pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 联调：改单价保存后重新进入页面值保持；新增节假日出现在表格；粘贴 holiday-cn 2026 年 JSON 导入成功；删除一条生效。

- [ ] **Step 3: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现设置中心参数配置与节假日维护"
```

---

### Task 13: 用户管理

**Files:**
- Rewrite: `dashboard/src/views/users/index.vue`

**Interfaces:**
- Consumes: Task 7 的 `fetchUsers/createUser/updateUser/resetPassword`、Task 9 的 `formatDateTime`
- Produces: 完整用户管理页面（admin 专属）

- [ ] **Step 1: 实现用户管理页**

`dashboard/src/views/users/index.vue` 整体替换为：

```vue
<template>
  <div class="user-page">
    <div class="toolbar">
      <span></span>
      <el-button type="primary" @click="openCreate">新建用户</el-button>
    </div>

    <el-table v-loading="loading" :data="list">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="username" label="用户名" width="140" />
      <el-table-column prop="name" label="姓名" min-width="120" />
      <el-table-column label="角色" width="110">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
            {{ row.role === 'admin' ? '超级管理员' : '需求方' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="150">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="warning" @click="openReset(row)">重置密码</el-button>
          <el-button link :type="row.enabled ? 'danger' : 'success'" @click="toggleEnabled(row)">
            {{ row.enabled ? '停用' : '启用' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新建 -->
    <el-dialog v-model="createVisible" title="新建用户" width="440px">
      <el-form ref="createRef" :model="createForm" :rules="createRules" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model.trim="createForm.username" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="createForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="姓名" prop="name">
          <el-input v-model.trim="createForm.name" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-radio-group v-model="createForm.role">
            <el-radio value="client">需求方</el-radio>
            <el-radio value="admin">超级管理员</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="create">保存</el-button>
      </template>
    </el-dialog>

    <!-- 编辑姓名 -->
    <el-dialog v-model="editVisible" title="编辑用户" width="440px">
      <el-form label-width="80px">
        <el-form-item label="姓名">
          <el-input v-model.trim="editForm.name" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码 -->
    <el-dialog v-model="resetVisible" title="重置密码" width="440px">
      <el-form ref="resetRef" :model="resetForm" :rules="resetRules" label-width="80px">
        <el-form-item label="新密码" prop="password">
          <el-input v-model="resetForm.password" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveReset">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { createUser, fetchUsers, resetPassword, updateUser } from '@/api/user'
import { formatDateTime } from '@/utils/clepsydra/date'

defineOptions({ name: 'UserList' })

const list = ref<Api.User.Item[]>([])
const loading = ref(false)
const saving = ref(false)

const createVisible = ref(false)
const editVisible = ref(false)
const resetVisible = ref(false)
const createRef = ref<FormInstance>()
const resetRef = ref<FormInstance>()

const createForm = reactive<Api.User.CreateParams>({
  username: '',
  password: '',
  name: '',
  role: 'client'
})

const editForm = reactive({ id: 0, name: '' })
const resetForm = reactive({ id: 0, password: '' })

const createRules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, min: 6, message: '密码至少 6 位', trigger: 'blur' }],
  name: [{ required: true, message: '请输入姓名', trigger: 'blur' }]
}

const resetRules: FormRules = {
  password: [{ required: true, min: 6, message: '密码至少 6 位', trigger: 'blur' }]
}

/** 加载用户列表 */
async function load() {
  loading.value = true
  try {
    list.value = await fetchUsers()
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createForm.username = ''
  createForm.password = ''
  createForm.name = ''
  createForm.role = 'client'
  createVisible.value = true
}

async function create() {
  await createRef.value?.validate()
  saving.value = true
  try {
    await createUser({ ...createForm })
    ElMessage.success('用户已创建')
    createVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function openEdit(row: Api.User.Item) {
  editForm.id = row.id
  editForm.name = row.name
  editVisible.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    await updateUser(editForm.id, { name: editForm.name })
    ElMessage.success('已保存')
    editVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function openReset(row: Api.User.Item) {
  resetForm.id = row.id
  resetForm.password = ''
  resetVisible.value = true
}

async function saveReset() {
  await resetRef.value?.validate()
  saving.value = true
  try {
    await resetPassword(resetForm.id, resetForm.password)
    ElMessage.success('密码已重置')
    resetVisible.value = false
  } finally {
    saving.value = false
  }
}

/** 启用/停用切换，带二次确认 */
async function toggleEnabled(row: Api.User.Item) {
  const action = row.enabled ? '停用' : '启用'
  await ElMessageBox.confirm(`确定${action}用户「${row.name}」吗？`, '操作确认', { type: 'warning' })
  await updateUser(row.id, { enabled: !row.enabled })
  ElMessage.success(`已${action}`)
  await load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.user-page {
  padding: 16px;

  .toolbar {
    display: flex;
    justify-content: space-between;
    margin-bottom: 16px;
  }
}
</style>
```

- [ ] **Step 2: 验证**

```bash
cd dashboard && pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 联调：新建 client 用户 → 新用户可登录；停用后该用户刷新页面被登出（Task 1 的 me 校验生效）；重置密码后旧密码失效。

- [ ] **Step 3: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现用户管理增改停用与重置密码"
```

---

### Task 14: 审计日志

**Files:**
- Rewrite: `dashboard/src/views/auditlogs/index.vue`

**Interfaces:**
- Consumes: Task 7 的 `fetchAuditLogs`、Task 9 的 `formatDateTime`
- Produces: 分页审计日志页面（admin 专属，只读）

- [ ] **Step 1: 实现审计日志页**

`dashboard/src/views/auditlogs/index.vue` 整体替换为：

```vue
<template>
  <div class="auditlog-page">
    <div class="toolbar">
      <el-select v-model="query.target_type" placeholder="目标类型" clearable style="width: 140px" @change="search">
        <el-option label="需求" value="demand" />
        <el-option label="账单" value="bill" />
        <el-option label="用户" value="user" />
        <el-option label="设置" value="setting" />
      </el-select>
      <el-input-number v-model="targetId" placeholder="目标 ID" :min="1" :controls="false" style="width: 120px" />
      <el-button type="primary" @click="search">查询</el-button>
    </div>

    <el-table v-loading="loading" :data="rows">
      <el-table-column type="expand">
        <template #default="{ row }">
          <pre class="detail-json">{{ JSON.stringify(row.detail, null, 2) }}</pre>
        </template>
      </el-table-column>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="operator_name" label="操作人" width="120" />
      <el-table-column prop="action" label="动作" min-width="160" />
      <el-table-column prop="target_type" label="目标类型" width="100" />
      <el-table-column prop="target_id" label="目标 ID" width="90" />
      <el-table-column label="时间" width="160">
        <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.size"
      :total="total"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[20, 50, 100]"
      class="pagination"
      @change="load"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { fetchAuditLogs } from '@/api/auditlog'
import { formatDateTime } from '@/utils/clepsydra/date'

defineOptions({ name: 'AuditLogList' })

const rows = ref<Api.AuditLog.Item[]>([])
const total = ref(0)
const loading = ref(false)
const targetId = ref<number>()

const query = reactive<Api.AuditLog.Query>({
  target_type: undefined,
  page: 1,
  size: 20
})

/** 加载当前页 */
async function load() {
  loading.value = true
  try {
    const data = await fetchAuditLogs({
      ...query,
      target_type: query.target_type || undefined,
      target_id: targetId.value || undefined
    })
    rows.value = data.rows
    total.value = data.total
  } finally {
    loading.value = false
  }
}

/** 条件变化时回到第一页查询 */
function search() {
  query.page = 1
  load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.auditlog-page {
  padding: 16px;

  .toolbar {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
  }

  .detail-json {
    padding: 8px 16px;
    margin: 0;
    overflow-x: auto;
    font-size: 12px;
  }

  .pagination {
    justify-content: flex-end;
    margin-top: 16px;
  }
}
</style>
```

注：`target_type` 下拉的取值（demand/bill/user/setting）以后端 AuditLog 实际写入的 target_type 枚举为准，实施时 `grep -rn "TargetType\|target_type" internal/service/audit.go` 核对并调整选项。

- [ ] **Step 2: 验证**

```bash
cd dashboard && pnpm build && pnpm lint && pnpm test
```

预期：全绿。dev 联调：此前联调产生的确认/流转/减免操作均有日志；按类型与 ID 筛选正确；展开行可见 detail JSON；翻页正常。

- [ ] **Step 3: 提交**

```bash
cd /Users/liasica/projects/liasica/clepsydra
git add -A dashboard
git commit -m "feat: 实现审计日志分页查询"
```

---

### Task 15: 全链路验收与收尾

**Files:**
- Modify: `TODO.md`（勾选前端条目）

**Interfaces:**
- Consumes: 全部前置任务
- Produces: 可交付的单二进制部署产物

- [ ] **Step 1: 单二进制全链路构建**

```bash
cd /Users/liasica/projects/liasica/clepsydra
make dashboard
make build
```

预期：前端构建产物同步到 `internal/api/static/dist/`，`bin/clepsydra` 构建成功，`git status` 中无 `internal/api/static/dist` 下的未忽略文件（仅 .gitkeep 保持入库）。

- [ ] **Step 2: 单二进制冒烟**

```bash
./bin/clepsydra -c configs/config.yaml
```

浏览器访问 `http://localhost:8080/`（不再经 vite）逐项确认：

1. 登录页正常加载（静态资源全部同源命中）
2. admin 登录 → 工作台/需求/账单/设置/用户/审计日志全部可用
3. 直接刷新 `http://localhost:8080/demands/1` 等深层路由 → SPA fallback 正常返回页面
4. `http://localhost:8080/docs` 接口文档不受影响
5. `curl http://localhost:8080/api/nothing` → 404 JSON 而非页面
6. client 账号登录 → 菜单只见工作台/需求/账单，直接输入 `/users` 地址被权限拦截

- [ ] **Step 3: 全量质量门槛**

```bash
go test ./... -count=1
cd dashboard && pnpm test && pnpm build && pnpm lint
cd .. && gclint run --config .golangci.yml --new-from-rev=origin/master --timeout=10m
```

预期：全部通过（gclint 的 `--new-from-rev` 按本分支起点调整）。

- [ ] **Step 4: 勾选 TODO 并提交**

`TODO.md` 中前端条目改为：

```markdown
- [x] 前端使用 [Art Design Pro](https://github.com/Daymychen/art-design-pro) 框架
```

```bash
git add TODO.md
git commit -m "docs: 勾选前端框架待办"
```

---

## 计划自审记录

- **Spec 覆盖**：设计文档六节逐一对应——工程形态（Task 3/4）、构建交付（Task 2/15）、页面与路由（Task 8-14）、请求层与认证（Task 5/1/8）、换算与字典（Task 6）、测试门槛（各任务 Step + Task 15）；范围外条目未混入
- **占位符**：无 TBD；对依赖框架实际代码的改造点（error.ts 行级改动、守卫插入点、工作台文件路径、AuditLog 字段名、holidays 参数），已给出定位命令与修改原则，属「以实际源码为准」的受控开放点而非未定义行为
- **类型一致性**：`Api.*` 命名空间、`DEMAND_STATUS/BILL_STATUS`、`formatManday/formatAmount/formatDate/formatDateTime`、`toUserInfo/loginByPassword/restoreSession`、`static.Register/RegisterFS` 在各任务间引用一致

