# 项目管理与需求多选项目实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 引入轻量「项目」标签实体，需求可多选项目归类，支持列表筛选与账单明细展示。

**Architecture:** 后端新增 `Project` ent schema 与 `Demand` 的多对多 edge；项目 CRUD 仅超管，`GET /projects` 全员可读；需求创建带 `project_ids`，独立接口 `PUT /demands/:id/projects` 不受状态限制地全量覆盖标签；账单明细响应实时组装项目。前端新增项目管理页、需求表单多选、列表筛选与 tag 展示。

**Tech Stack:** Go + ent + echo + sqlite（测试）；Vue3 + Vben Admin（antdv-next）。

**Spec:** `.superpowers/specs/2026-08-10-project-tags-design.md`

## Global Constraints

- 注释一律中文，句末不加标点；中文标点全角、英文标点半角后跟空格（详见用户全局 CLAUDE.md 标点规范）
- Git 提交遵循 Conventional Commits，禁止任何 AI 署名
- 每个任务提交前：`go build ./... && go test ./... -count=1` 通过；涉及 Go 代码的提交跑 `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 无 issue
- 前端代码改动后：`cd dashboard && pnpm lint` 无 issue、`pnpm check:type` 通过
- 项目不依赖 nexa 包、不做 DI 注入，手动装配（见 `cmd/clepsydra/main.go`）
- `git commit` 时报 `Can't find lefthook in PATH` 是环境噪音，可忽略

---

### Task 1: Project schema 与 Demand 多对多 edge

**Files:**
- Create: `internal/ent/schema/project.go`
- Modify: `internal/ent/schema/demand.go`

**Interfaces:**
- Produces: ent 生成代码 —— `ent.Project` 实体、`Demand` 侧 `AddProjectIDs/ClearProjects/QueryProjects/WithProjects/HasProjectsWith`、`Project` 侧 `WithDemands/Edges.Demands`，中间表 `project_demands` 外键级联删除

- [ ] **Step 1: 写 Project schema**

创建 `internal/ent/schema/project.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Project 项目，轻量标签形态：需求可归属多个项目，用于筛选归类
// 不做软删除：删除即物理删除，与需求的关联由中间表外键级联清除
type Project struct {
	ent.Schema
}

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
		field.String("color").Optional(), // antdv 预设 tag 色名（如 blue），空串表示默认色
		field.Text("remark").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("demands", Demand.Type),
	}
}
```

- [ ] **Step 2: 给 Demand 加反向 edge**

修改 `internal/ent/schema/demand.go`，import 增加 `"entgo.io/ent/schema/edge"`，在 `Fields()` 与 `Indexes()` 之间新增：

```go
// Edges 需求与项目的多对多关联：项目是轻量归类标签，不影响人天与账单金额
func (Demand) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("projects", Project.Type).Ref("demands"),
	}
}
```

- [ ] **Step 3: 代码生成并验证**

```bash
make generate && go build ./... && go test ./... -count=1
```

Expected: 生成 `internal/ent/project*` 等文件，编译与全量测试通过（现有测试用 enttest 自动迁移，schema 新增不破坏既有用例）。

- [ ] **Step 4: 提交**

```bash
git add internal/ent && git commit -m "feat(ent): 新增项目实体与需求多对多关联"
```

---

### Task 2: 项目 service

**Files:**
- Create: `internal/service/project.go`
- Test: `internal/service/project_test.go`

**Interfaces:**
- Consumes: Task 1 生成的 ent 代码；已有 `Audit.Record(ctx, actor, action, targetType string, targetID int, payload map[string]any)`、`ErrBadRequest(msg)`、`ErrNotFound`、`Actor`
- Produces:
  - `NewProject(client *ent.Client, audit *Audit) *Project`
  - `(*Project) List(ctx) ([]*ent.Project, error)` —— 预加载 `Edges.Demands` 供 handler 统计关联数
  - `(*Project) Create(ctx, actor Actor, name, color, remark string) (*ent.Project, error)`
  - `(*Project) Update(ctx, actor Actor, id int, name, color, remark string) (*ent.Project, error)`
  - `(*Project) Delete(ctx, actor Actor, id int) error`
  - 审计动作：`project.create`、`project.update`、`project.delete`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/project_test.go`（复用 demand_test.go 的包级 `admin` Actor）：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

// newProjectEnv 构建 Project 测试环境
func newProjectEnv(t *testing.T, name string) (*ent.Client, *Project) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	return client, NewProject(client, NewAudit(client))
}

// TestProjectCRUD 覆盖创建、更新、删除与列表的正常路径
func TestProjectCRUD(t *testing.T) {
	_, svc := newProjectEnv(t, "pcrud")
	ctx := context.Background()

	p, err := svc.Create(ctx, admin, "小程序", "blue", "小程序相关需求")
	if err != nil {
		t.Fatalf("创建项目失败: %v", err)
	}
	if p.Name != "小程序" || p.Color != "blue" {
		t.Errorf("创建结果异常: %+v", p)
	}

	p, err = svc.Update(ctx, admin, p.ID, "小程序端", "green", "")
	if err != nil {
		t.Fatalf("更新项目失败: %v", err)
	}
	if p.Name != "小程序端" || p.Color != "green" || p.Remark != "" {
		t.Errorf("更新结果异常: %+v", p)
	}

	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("列表失败: %v, len=%d", err, len(rows))
	}

	if err = svc.Delete(ctx, admin, p.ID); err != nil {
		t.Fatalf("删除项目失败: %v", err)
	}
	rows, _ = svc.List(ctx)
	if len(rows) != 0 {
		t.Errorf("删除后列表应为空, len=%d", len(rows))
	}
}

// TestProjectValidation 覆盖空名称、重名与不存在记录的错误路径
func TestProjectValidation(t *testing.T) {
	_, svc := newProjectEnv(t, "pvalid")
	ctx := context.Background()

	if _, err := svc.Create(ctx, admin, "", "", ""); err == nil {
		t.Error("空名称应报错")
	}

	p1, _ := svc.Create(ctx, admin, "官网", "", "")
	p2, _ := svc.Create(ctx, admin, "后台", "", "")

	if _, err := svc.Create(ctx, admin, "官网", "", ""); err == nil {
		t.Error("重名创建应报错")
	}
	if _, err := svc.Update(ctx, admin, p2.ID, "官网", "", ""); err == nil {
		t.Error("更新为已有名称应报错")
	}
	// 名称不变的更新不应误报重名
	if _, err := svc.Update(ctx, admin, p1.ID, "官网", "red", ""); err != nil {
		t.Errorf("原名更新不应报错: %v", err)
	}

	if _, err := svc.Update(ctx, admin, 999, "任意", "", ""); err != ErrNotFound {
		t.Errorf("更新不存在项目应返回 ErrNotFound, got %v", err)
	}
	if err := svc.Delete(ctx, admin, 999); err != ErrNotFound {
		t.Errorf("删除不存在项目应返回 ErrNotFound, got %v", err)
	}
}

// TestProjectDemandCountAndDetach 关联需求数统计（软删需求不计入）与删除项目解除关联
func TestProjectDemandCountAndDetach(t *testing.T) {
	client, svc := newProjectEnv(t, "pcount")
	ctx := context.Background()

	p, _ := svc.Create(ctx, admin, "官网", "", "")
	d1 := client.Demand.Create().SetTitle("需求一").AddProjectIDs(p.ID).SaveX(ctx)
	client.Demand.Create().SetTitle("需求二").AddProjectIDs(p.ID).SaveX(ctx)

	rows, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if n := len(rows[0].Edges.Demands); n != 2 {
		t.Errorf("关联需求数 = %d, want 2", n)
	}

	// 软删需求后不再计入
	client.Demand.DeleteOneID(d1.ID).ExecX(ctx)
	rows, _ = svc.List(ctx)
	if n := len(rows[0].Edges.Demands); n != 1 {
		t.Errorf("软删后关联需求数 = %d, want 1", n)
	}

	// 删除项目自动解除关联，需求本身不受影响
	if err = svc.Delete(ctx, admin, p.ID); err != nil {
		t.Fatalf("删除项目失败: %v", err)
	}
	d2 := client.Demand.Query().WithProjects().AllX(ctx)
	for _, d := range d2 {
		if len(d.Edges.Projects) != 0 {
			t.Errorf("需求 %d 的项目关联应已解除", d.ID)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/service/ -run 'TestProject' -count=1
```

Expected: 编译错误 `undefined: NewProject`。

- [ ] **Step 3: 实现 service**

创建 `internal/service/project.go`：

```go
package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/project"
)

// Project 项目服务，轻量标签的增删改查
type Project struct {
	client *ent.Client
	audit  *Audit
}

// NewProject 构建项目服务
func NewProject(client *ent.Client, audit *Audit) *Project {
	return &Project{client: client, audit: audit}
}

// List 查询全部项目，预加载关联需求供 handler 统计关联数；
// 关联需求查询会走 Demand 的软删除拦截器，已软删需求不计入
func (s *Project) List(ctx context.Context) ([]*ent.Project, error) {
	return s.client.Project.Query().
		WithDemands().
		Order(ent.Asc(project.FieldID)).
		All(ctx)
}

// Create 创建项目，名称必填且唯一
func (s *Project) Create(ctx context.Context, actor Actor, name, color, remark string) (*ent.Project, error) {
	if name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}

	exists, err := s.client.Project.Query().Where(project.Name(name)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("项目名称已存在")
	}

	p, err := s.client.Project.Create().
		SetName(name).
		SetColor(color).
		SetRemark(remark).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "project.create", "project", p.ID, map[string]any{
		"name": name,
	})

	return p, nil
}

// Update 全量更新项目名称、颜色与备注
func (s *Project) Update(ctx context.Context, actor Actor, id int, name, color, remark string) (*ent.Project, error) {
	if name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}

	// 重名检查排除自身，允许原名保存
	exists, err := s.client.Project.Query().
		Where(project.Name(name), project.IDNEQ(id)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("项目名称已存在")
	}

	p, err := s.client.Project.UpdateOneID(id).
		SetName(name).
		SetColor(color).
		SetRemark(remark).
		Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "project.update", "project", id, map[string]any{
		"name": name,
	})

	return p, nil
}

// Delete 物理删除项目，与需求的关联由中间表外键级联清除，需求本身不受影响
func (s *Project) Delete(ctx context.Context, actor Actor, id int) error {
	p, err := s.client.Project.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if err = s.client.Project.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "project.delete", "project", id, map[string]any{
		"name": p.Name,
	})

	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/service/ -run 'TestProject' -count=1 -v
```

Expected: 3 个用例全部 PASS。若 `TestProjectDemandCountAndDetach` 中删除项目报外键错误，说明中间表未级联，改为在 `Delete` 里先 `s.client.Project.UpdateOneID(id).ClearDemands().Exec(ctx)` 再删除。

- [ ] **Step 5: 提交**

```bash
git add internal/service/project.go internal/service/project_test.go
git commit -m "feat(service): 项目标签增删改查与关联需求数统计"
```

---

### Task 3: 项目 handler、路由与装配

**Files:**
- Create: `internal/api/handler/project.go`
- Modify: `internal/api/router.go`（接口定义 + Handlers 字段 + 路由）
- Modify: `cmd/clepsydra/main.go`（装配）
- Modify: `internal/api/docs/openapi.yaml`、`internal/api/docs/docs_test.go`
- Test: `internal/api/handler/project_test.go`

**Interfaces:**
- Consumes: Task 2 的 `service.Project` 全部方法；handler 包已有 `parseID(c)`、`actor(c)`（demand.go 中定义，同包可用）
- Produces: 路由 `GET /api/projects`（登录组）、`POST/PUT/DELETE /api/projects...`（admin 组）；`projectDTO` JSON：`{id, name, color, remark, demand_count, created_at, updated_at}`

- [ ] **Step 1: 写失败测试**

创建 `internal/api/handler/project_test.go`：

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestProjectHandlerCRUD 覆盖项目接口的创建、列表、更新与删除
func TestProjectHandlerCRUD(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hproject?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	h := NewProject(service.NewProject(client, service.NewAudit(client)))
	e := echo.New()

	do := func(method, path, body string) *httptest.ResponseRecorder {
		var reader *strings.Reader
		if body == "" {
			reader = strings.NewReader("")
		} else {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("claims", &service.Claims{UserID: 1, Role: "admin", Name: "管理员"})
		// 带 :id 的路径手动设置路由参数
		if parts := strings.Split(path, "/"); len(parts) == 4 {
			c.SetParamNames("id")
			c.SetParamValues(parts[3])
		}
		var err error
		switch {
		case method == http.MethodGet:
			err = h.List(c)
		case method == http.MethodPost:
			err = h.Create(c)
		case method == http.MethodPut:
			err = h.Update(c)
		case method == http.MethodDelete:
			err = h.Delete(c)
		}
		if err != nil {
			t.Fatalf("%s %s 错误: %v", method, path, err)
		}
		return rec
	}

	rec := do(http.MethodPost, "/api/projects", `{"name":"官网","color":"blue","remark":"备注"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("创建响应异常: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/projects", "")
	if !strings.Contains(rec.Body.String(), `"demand_count":0`) {
		t.Errorf("列表应含 demand_count: %s", rec.Body.String())
	}

	// 从服务层取回 ID 再走更新与删除
	rows, _ := service.NewProject(client, service.NewAudit(client)).List(ctx)
	id := strconv.Itoa(rows[0].ID)

	rec = do(http.MethodPut, "/api/projects/"+id, `{"name":"官网二期","color":"green"}`)
	if !strings.Contains(rec.Body.String(), "官网二期") {
		t.Errorf("更新响应异常: %s", rec.Body.String())
	}

	rec = do(http.MethodDelete, "/api/projects/"+id, "")
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Errorf("删除响应异常: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/api/handler/ -run 'TestProjectHandler' -count=1
```

Expected: 编译错误 `undefined: NewProject`（handler 包）。

- [ ] **Step 3: 实现 handler**

创建 `internal/api/handler/project.go`：

```go
package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Project 项目管理接口
type Project struct {
	svc *service.Project
}

// NewProject 构建项目 handler
func NewProject(svc *service.Project) *Project {
	return &Project{svc: svc}
}

// projectRequest 创建 / 更新请求体
type projectRequest struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Remark string `json:"remark"`
}

// projectDTO 项目响应结构；demand_count 为关联需求数（不含已软删需求），
// 仅列表接口的查询预加载了关联，创建 / 更新响应中恒为 0，前端以列表为准
type projectDTO struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Remark      string    `json:"remark"`
	DemandCount int       `json:"demand_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// newProjectDTO 将 ent.Project 映射为响应结构
func newProjectDTO(p *ent.Project) projectDTO {
	return projectDTO{
		ID:          p.ID,
		Name:        p.Name,
		Color:       p.Color,
		Remark:      p.Remark,
		DemandCount: len(p.Edges.Demands),
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// List GET /api/projects
func (h *Project) List(c echo.Context) error {
	rows, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	dtos := make([]projectDTO, 0, len(rows))
	for _, p := range rows {
		dtos = append(dtos, newProjectDTO(p))
	}

	return api.OK(c, dtos)
}

// Create POST /api/projects
func (h *Project) Create(c echo.Context) error {
	var req projectRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	p, err := h.svc.Create(c.Request().Context(), actor(c), req.Name, req.Color, req.Remark)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newProjectDTO(p))
}

// Update PUT /api/projects/:id
func (h *Project) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req projectRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	p, err := h.svc.Update(c.Request().Context(), actor(c), id, req.Name, req.Color, req.Remark)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newProjectDTO(p))
}

// Delete DELETE /api/projects/:id
func (h *Project) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Delete(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
```

- [ ] **Step 4: 注册路由与装配**

修改 `internal/api/router.go`：

在 `DemandHandler` 接口后新增：

```go
// ProjectHandler 项目管理接口方法集
type ProjectHandler interface {
	List(c echo.Context) error
	Create(c echo.Context) error
	Update(c echo.Context) error
	Delete(c echo.Context) error
}
```

`Handlers` struct 加字段 `Project ProjectHandler`（放在 `Demand` 之后）。

路由注册：登录组（`authed.POST("/demands", ...)` 附近）加：

```go
authed.GET("/projects", h.Project.List)
```

admin 组（`adminGroup.DELETE("/demands/:id", ...)` 附近）加：

```go
adminGroup.POST("/projects", h.Project.Create)
adminGroup.PUT("/projects/:id", h.Project.Update)
adminGroup.DELETE("/projects/:id", h.Project.Delete)
```

修改 `cmd/clepsydra/main.go`：服务装配处加 `projectSvc := service.NewProject(client, audit)`，`api.Handlers{...}` 里加 `Project: handler.NewProject(projectSvc),`。

- [ ] **Step 5: 更新 openapi.yaml 与路由计数**

`internal/api/docs/openapi.yaml`：

`tags:` 列表加（跟随现有风格）：

```yaml
  - name: 项目
    description: 项目标签管理，需求可多选项目归类
```

`paths:` 加（对照现有 `/api/users` 的写法，含 `security`、`responses` 结构）：

```yaml
  /api/projects:
    get:
      tags: [项目]
      summary: 项目列表
      description: 全部项目及各自关联需求数（不含已软删需求），登录即可访问，供管理页、下拉与筛选使用
      responses:
        '200':
          description: 项目数组
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - type: object
                    properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/Project'
    post:
      tags: [项目]
      summary: 创建项目（仅超级管理员）
      description: 名称必填且唯一，重名返回业务错误
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ProjectSave'
      responses:
        '200':
          description: 创建后的项目
  /api/projects/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema:
          type: integer
    put:
      tags: [项目]
      summary: 更新项目（仅超级管理员）
      description: 全量更新名称、颜色与备注
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ProjectSave'
      responses:
        '200':
          description: 更新后的项目
    delete:
      tags: [项目]
      summary: 删除项目（仅超级管理员）
      description: 物理删除并自动解除与需求的关联，需求本身不受影响
      responses:
        '200':
          description: 删除成功
```

`components.schemas` 加：

```yaml
    Project:
      type: object
      description: 项目，轻量标签形态
      properties:
        id:
          type: integer
          description: 项目 ID
        name:
          type: string
          description: 项目名称，唯一
        color:
          type: string
          description: antdv 预设 tag 色名（如 blue），空串表示默认色
        remark:
          type: string
          description: 备注
        demand_count:
          type: integer
          description: 关联需求数（不含已软删需求），仅项目列表接口有效
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
    ProjectSave:
      type: object
      description: 项目创建 / 更新请求体
      required: [name]
      properties:
        name:
          type: string
          description: 项目名称，唯一
        color:
          type: string
          description: antdv 预设 tag 色名，可空
        remark:
          type: string
          description: 备注，可空
```

修改 `internal/api/docs/docs_test.go`：`expectedRouteCount` 从 34 改为 38，注释同步为「…+ 10 条登录组（含 auth/me）+ 27 条 admin 组」（本任务新增 GET /projects 登录组 1 条、admin 组 3 条；Task 5 还会 +1，此处先按 38）。

**注意**：若 docs_test 有 openapi 路径与 router 一致性断言，运行后按报错补齐。

- [ ] **Step 6: 运行测试**

```bash
go build ./... && go test ./internal/api/... -count=1
```

Expected: 全部 PASS（含 docs 路由计数）。

- [ ] **Step 7: 提交**

```bash
git add internal/api cmd/clepsydra/main.go
git commit -m "feat(api): 项目管理接口与路由，列表全员可读、写操作仅超管"
```

---

### Task 4: 需求 service 关联项目

**Files:**
- Modify: `internal/service/demand.go`
- Test: `internal/service/demand_projects_test.go`（新建）
- Modify: 所有 `svc.Create(...)` / `svc.List(...)` 调用点（编译错误驱动，主要在 `internal/service/*_test.go`、`internal/api/handler/demand.go`、`internal/api/handler/demand_test.go`、`internal/task/`（若有））

**Interfaces:**
- Consumes: Task 1 生成的 ent edge 方法
- Produces:
  - `Create(ctx, actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool, projectIDs []int)` —— 尾部新增 `projectIDs`
  - `UpdateProjects(ctx, actor Actor, id int, projectIDs []int) (*ent.Demand, error)` —— 任何状态可用，全量覆盖
  - `List(ctx, status string, projectID int)` —— `projectID > 0` 时按项目筛选；返回值预加载 `Edges.Projects`
  - `Get` 返回值预加载 `Edges.Projects`
  - 审计动作：`demand.update_projects`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/demand_projects_test.go`：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// projectFixtures 建两个项目供关联测试使用
func projectFixtures(t *testing.T, client *ent.Client) (int, int) {
	t.Helper()
	ctx := context.Background()

	p1 := client.Project.Create().SetName("官网").SaveX(ctx)
	p2 := client.Project.Create().SetName("小程序").SaveX(ctx)

	return p1.ID, p2.ID
}

// TestDemandCreateWithProjects 创建需求携带项目关联，并校验无效项目 ID 被拒绝
func TestDemandCreateWithProjects(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-create")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	d, err := svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1, p2, p1})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got := svc.mustGet(ctx, t, d.ID)
	if len(got.Edges.Projects) != 2 {
		t.Errorf("关联项目数 = %d, want 2（重复 ID 应去重）", len(got.Edges.Projects))
	}

	if _, err = svc.Create(ctx, admin, "需求二", "", 0, nil, false, []int{999}); err == nil {
		t.Error("不存在的项目 ID 应报错")
	}
}

// TestDemandUpdateProjects 覆盖任意状态改标签、全量覆盖与清空
func TestDemandUpdateProjects(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-update")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	d, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1})

	// 推进到 accepted 之外的锁定态验证不受状态限制：直接改库到 accepted
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").ExecX(ctx)

	got, err := svc.UpdateProjects(ctx, admin, d.ID, []int{p2})
	if err != nil {
		t.Fatalf("已验收需求改标签失败: %v", err)
	}
	if len(got.Edges.Projects) != 1 || got.Edges.Projects[0].ID != p2 {
		t.Errorf("覆盖结果异常: %+v", got.Edges.Projects)
	}

	// 空数组清空
	got, err = svc.UpdateProjects(ctx, admin, d.ID, nil)
	if err != nil {
		t.Fatalf("清空标签失败: %v", err)
	}
	if len(got.Edges.Projects) != 0 {
		t.Errorf("清空后仍有关联: %+v", got.Edges.Projects)
	}

	if _, err = svc.UpdateProjects(ctx, admin, d.ID, []int{999}); err == nil {
		t.Error("不存在的项目 ID 应报错")
	}
	if _, err = svc.UpdateProjects(ctx, admin, 999, []int{p1}); err != ErrNotFound {
		t.Errorf("不存在的需求应返回 ErrNotFound, got %v", err)
	}
}

// TestDemandListFilterByProject 按项目筛选需求列表
func TestDemandListFilterByProject(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-list")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	_, _ = svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1})
	_, _ = svc.Create(ctx, admin, "需求二", "", 0, nil, false, []int{p2})
	_, _ = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil)
	_ = client

	rows, err := svc.List(ctx, "", p1)
	if err != nil || len(rows) != 1 || rows[0].Title != "需求一" {
		t.Fatalf("按项目筛选异常: %v, len=%d", err, len(rows))
	}

	rows, _ = svc.List(ctx, "", 0)
	if len(rows) != 3 {
		t.Errorf("不筛选应返回全部, len=%d", len(rows))
	}
	// 列表应预加载项目关联
	if rows[len(rows)-1].Edges.Projects == nil {
		t.Error("列表应预加载 Edges.Projects")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/service/ -run 'TestDemandCreateWithProjects|TestDemandUpdateProjects|TestDemandListFilterByProject' -count=1
```

Expected: 编译错误（Create 参数数量不符 / UpdateProjects 未定义）。

- [ ] **Step 3: 实现 service 变更**

修改 `internal/service/demand.go`，import 增加 `"clepsydra/internal/ent/project"`：

`List` 改为：

```go
// List 按状态与项目筛选需求，status 为空、projectID 为 0 表示不筛选；预加载项目标签
func (s *Demand) List(ctx context.Context, status string, projectID int) ([]*ent.Demand, error) {
	q := s.client.Demand.Query().WithProjects().Order(ent.Desc(demand.FieldID))
	if status != "" {
		q = q.Where(demand.StatusEQ(demand.Status(status)))
	}
	if projectID > 0 {
		q = q.Where(demand.HasProjectsWith(project.ID(projectID)))
	}

	return q.All(ctx)
}
```

`Get` 改为预加载：

```go
// Get 按 ID 查询需求，预加载项目标签
func (s *Demand) Get(ctx context.Context, id int) (*ent.Demand, error) {
	d, err := s.client.Demand.Query().
		Where(demand.ID(id)).
		WithProjects().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return d, err
}
```

`Create` 签名尾部加 `projectIDs []int`，在 `create := s.client.Demand.Create()...` 之前校验、之后关联：

```go
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool, projectIDs []int) (*ent.Demand, error) {
	// ……原有 title / estimatedHalfDays / confirmed 校验不变……

	ids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	create := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays).
		AddProjectIDs(ids...)

	// ……其余不变……
}
```

审计 payload 在原有字段之后追加（仅非空时）：

```go
	if len(ids) > 0 {
		payload["project_ids"] = ids
	}
```

新增两个方法：

```go
// UpdateProjects 全量覆盖需求的项目标签，任何状态均可：
// 标签是归类元数据，不影响人天与账单金额，存量已完成需求也要能补打标签
func (s *Demand) UpdateProjects(ctx context.Context, actor Actor, id int, projectIDs []int) (*ent.Demand, error) {
	ids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	err = s.client.Demand.UpdateOneID(id).
		ClearProjects().
		AddProjectIDs(ids...).
		Exec(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_projects", "demand", id, map[string]any{
		"project_ids": ids,
	})

	return s.Get(ctx, id)
}

// normalizeProjectIDs 去重并校验项目 ID 均存在，空切片直接通过
func (s *Demand) normalizeProjectIDs(ctx context.Context, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[int]bool, len(ids))
	uniq := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}

	n, err := s.client.Project.Query().Where(project.IDIn(uniq...)).Count(ctx)
	if err != nil {
		return nil, err
	}
	if n != len(uniq) {
		return nil, ErrBadRequest("项目不存在")
	}

	return uniq, nil
}
```

**注意**：`UpdateProjects` 走 `UpdateOneID`，软删除 mixin 的 `excludeDeletedHook` 会附加未删条件，对已软删需求返回 NotFound，符合预期。

- [ ] **Step 4: 修复所有编译错误**

```bash
go build ./... 2>&1 | head -30
```

逐一更新调用点：

- `internal/api/handler/demand.go` 的 `h.svc.Create(...)` 尾部暂传 `nil`、`h.svc.List(...)` 尾部暂传 `0`（Task 5 再接真实参数）
- 各测试文件里 `svc.Create(ctx, actor, ...)` 尾部加 `, nil`；`svc.List(ctx, "...")` 尾部加 `, 0`
- 其他包（如 `internal/task/`、`internal/service/bill*.go`）若有调用同样补默认值

- [ ] **Step 5: 全量测试**

```bash
go test ./... -count=1
```

Expected: 全部 PASS（含新增 3 个用例）。

- [ ] **Step 6: 提交**

```bash
git add -A && git commit -m "feat(demand): 创建携带项目标签、任意状态改标签与按项目筛选"
```

---

### Task 5: 需求 handler 与路由变更

**Files:**
- Modify: `internal/api/handler/demand.go`
- Modify: `internal/api/router.go`（DemandHandler 接口 + 路由）
- Modify: `internal/api/docs/openapi.yaml`、`internal/api/docs/docs_test.go`
- Test: `internal/api/handler/demand_test.go`（追加用例）

**Interfaces:**
- Consumes: Task 4 的 `Create(..., projectIDs []int)`、`UpdateProjects`、`List(ctx, status, projectID)`
- Produces: `POST /api/demands` 接受 `project_ids: []int`；`PUT /api/demands/:id/projects`（登录组）；`GET /api/demands?project_id=`；需求响应 JSON 带 `edges.projects`

- [ ] **Step 1: 写失败测试**

在 `internal/api/handler/demand_test.go` 末尾追加：

```go
// TestDemandProjectsHandler 覆盖创建带项目与独立改标签接口
func TestDemandProjectsHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdproj?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	p := client.Project.Create().SetName("官网").SaveX(ctx)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建带项目
	body := `{"title":"需求一","project_ids":[` + strconv.Itoa(p.ID) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.Create(c); err != nil {
		t.Fatalf("创建错误: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("创建响应异常: %s", rec.Body.String())
	}

	rows, _ := svc.List(ctx, "", 0)
	id := strconv.Itoa(rows[0].ID)

	// 独立接口清空标签（需求方也可操作）
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/projects", strings.NewReader(`{"project_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdateProjects(c); err != nil {
		t.Fatalf("改标签错误: %v", err)
	}
	got, _ := svc.Get(ctx, rows[0].ID)
	if len(got.Edges.Projects) != 0 {
		t.Errorf("标签应已清空: %+v", got.Edges.Projects)
	}

	// 列表按项目筛选参数透传
	req = httptest.NewRequest(http.MethodGet, "/api/demands?project_id="+strconv.Itoa(p.ID), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatalf("列表错误: %v", err)
	}
	if strings.Contains(rec.Body.String(), "需求一") {
		t.Errorf("标签已清空，按该项目筛选不应包含需求一: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/api/handler/ -run 'TestDemandProjectsHandler' -count=1
```

Expected: 编译错误 `h.UpdateProjects undefined`。

- [ ] **Step 3: 实现 handler 变更**

修改 `internal/api/handler/demand.go`：

`demandCreateRequest` 加字段：

```go
	ProjectIDs []int `json:"project_ids"`
```

新增请求体与 handler：

```go
// demandProjectsRequest 项目标签全量覆盖请求体
type demandProjectsRequest struct {
	ProjectIDs []int `json:"project_ids"`
}

// UpdateProjects PUT /api/demands/:id/projects
// 任何状态可用：标签是归类元数据，不影响人天与账单金额
func (h *Demand) UpdateProjects(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandProjectsRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateProjects(c.Request().Context(), actor(c), id, req.ProjectIDs)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}
```

`List` 读取 `project_id`：

```go
// List GET /api/demands?status=&project_id=
func (h *Demand) List(c echo.Context) error {
	status := c.QueryParam("status")
	projectID, _ := strconv.Atoi(c.QueryParam("project_id")) // 非法或缺省按 0 处理，即不筛选

	demands, err := h.svc.List(c.Request().Context(), status, projectID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, demands)
}
```

`Create` 把 Task 4 的占位 `nil` 换为 `req.ProjectIDs`。

- [ ] **Step 4: 路由与 openapi**

`internal/api/router.go`：`DemandHandler` 接口加 `UpdateProjects(c echo.Context) error`；登录组加：

```go
authed.PUT("/demands/:id/projects", h.Demand.UpdateProjects)
```

`internal/api/docs/docs_test.go`：`expectedRouteCount` 38 → 39，注释登录组 10 → 11。

`internal/api/docs/openapi.yaml`：

- `Demand` schema `properties` 末尾加：

```yaml
        edges:
          type: object
          description: ent 关联预加载结果
          properties:
            projects:
              type: array
              description: 关联的项目标签
              items:
                $ref: '#/components/schemas/Project'
```

- 创建需求请求体（`POST /api/demands` 的 requestBody schema）加：

```yaml
                project_ids:
                  type: array
                  description: 关联的项目 ID 列表，可选；含不存在的 ID 返回业务错误
                  items:
                    type: integer
```

- `GET /api/demands` 的 `parameters` 加：

```yaml
        - name: project_id
          in: query
          required: false
          schema:
            type: integer
          description: 按项目筛选，缺省不筛选
```

- `paths` 加：

```yaml
  /api/demands/{id}/projects:
    put:
      tags: [需求]
      summary: 更新需求的项目标签
      description: 全量覆盖式更新，传空数组即清空；任何状态可用，登录即可操作
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                project_ids:
                  type: array
                  items:
                    type: integer
      responses:
        '200':
          description: 更新后的需求
```

（以上片段插入时对照文件内相邻定义的缩进与风格微调，`tags` 取现有需求接口所用的 tag 名。）

- [ ] **Step 5: 全量测试与提交**

```bash
go build ./... && go test ./... -count=1
```

Expected: 全部 PASS。

```bash
git add -A && git commit -m "feat(api): 需求创建带项目标签、独立改标签接口与按项目筛选"
```

---

### Task 6: 账单明细组装项目标签

**Files:**
- Modify: `internal/service/bill.go`（新增 ItemProjects 方法）
- Modify: `internal/api/handler/bill.go`、`internal/api/handler/bill_dto.go`
- Modify: `internal/api/docs/openapi.yaml`（BillItem schema）
- Test: `internal/service/bill_items_test.go`（追加用例）

**Interfaces:**
- Consumes: Task 1 的 `WithProjects`；`schema.SkipSoftDelete(ctx)`（`internal/ent/schema`）
- Produces:
  - `(*Bill) ItemProjects(ctx, items []*ent.BillItem) (map[int][]*ent.Project, error)` —— key 为 demand_id
  - `billItemDTO` 增加 `Projects []projectRefDTO`，JSON `projects: [{id, name, color}]`
  - `newBillDetailDTO(b *ent.Bill, projects map[int][]*ent.Project) billDTO` —— 签名变化

- [ ] **Step 1: 写失败测试**

在 `internal/service/bill_items_test.go` 追加（环境构造函数参考该文件现有用例，沿用其 helper）：

```go
// TestBillItemProjects 明细行项目组装：有关联、无关联与已软删需求
func TestBillItemProjects(t *testing.T) {
	// 沿用本文件现有测试的环境构造方式（enttest.Open + Seed + NewBill 装配）
	client, billSvc := newBillItemsEnv(t, "bitem-proj") // 若无此 helper，参考文件内现有用例内联构造
	ctx := context.Background()

	p := client.Project.Create().SetName("官网").SetColor("blue").SaveX(ctx)
	d1 := client.Demand.Create().SetTitle("有标签").AddProjectIDs(p.ID).SaveX(ctx)
	d2 := client.Demand.Create().SetTitle("无标签").SaveX(ctx)
	d3 := client.Demand.Create().SetTitle("将被软删").AddProjectIDs(p.ID).SaveX(ctx)
	client.Demand.DeleteOneID(d3.ID).ExecX(ctx)

	items := []*ent.BillItem{
		{DemandID: d1.ID},
		{DemandID: d2.ID},
		{DemandID: d3.ID},
	}

	m, err := billSvc.ItemProjects(ctx, items)
	if err != nil {
		t.Fatalf("组装失败: %v", err)
	}
	if len(m[d1.ID]) != 1 || m[d1.ID][0].Name != "官网" {
		t.Errorf("d1 项目异常: %+v", m[d1.ID])
	}
	if len(m[d2.ID]) != 0 {
		t.Errorf("d2 应无项目: %+v", m[d2.ID])
	}
	// 已软删需求的明细行仍能追溯项目（账单可追溯语义）
	if len(m[d3.ID]) != 1 {
		t.Errorf("软删需求的明细行应仍能取到项目: %+v", m[d3.ID])
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/service/ -run 'TestBillItemProjects' -count=1
```

Expected: 编译错误 `ItemProjects undefined`。

- [ ] **Step 3: 实现 service 方法**

在 `internal/service/bill.go` 添加（import 增加 `"clepsydra/internal/ent/schema"`）：

```go
// ItemProjects 按明细行的 demand_id 批量取需求的项目标签，key 为 demand_id
// 项目是实时关联而非快照；用 SkipSoftDelete 查询，已软删需求的明细行也能追溯项目
func (s *Bill) ItemProjects(ctx context.Context, items []*ent.BillItem) (map[int][]*ent.Project, error) {
	if len(items) == 0 {
		return map[int][]*ent.Project{}, nil
	}

	seen := make(map[int]bool, len(items))
	ids := make([]int, 0, len(items))
	for _, it := range items {
		if !seen[it.DemandID] {
			seen[it.DemandID] = true
			ids = append(ids, it.DemandID)
		}
	}

	rows, err := s.client.Demand.Query().
		Where(demand.IDIn(ids...)).
		WithProjects().
		All(schema.SkipSoftDelete(ctx))
	if err != nil {
		return nil, err
	}

	m := make(map[int][]*ent.Project, len(rows))
	for _, d := range rows {
		m[d.ID] = d.Edges.Projects
	}

	return m, nil
}
```

- [ ] **Step 4: 运行 service 测试**

```bash
go test ./internal/service/ -run 'TestBillItemProjects' -count=1 -v
```

Expected: PASS。

- [ ] **Step 5: DTO 与 handler 接线**

`internal/api/handler/bill_dto.go`：

新增：

```go
// projectRefDTO 明细行携带的项目标签精简引用
type projectRefDTO struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
```

`billItemDTO` 加字段：

```go
	Projects []projectRefDTO `json:"projects"`
```

`newBillItemDTO` 签名改为 `newBillItemDTO(it *ent.BillItem, projects []*ent.Project) billItemDTO`，函数体内组装：

```go
	refs := make([]projectRefDTO, 0, len(projects))
	for _, p := range projects {
		refs = append(refs, projectRefDTO{ID: p.ID, Name: p.Name, Color: p.Color})
	}
```

并赋给 `Projects: refs`。

`newBillDetailDTO` 签名改为 `newBillDetailDTO(b *ent.Bill, projects map[int][]*ent.Project) billDTO`，循环内改为 `newBillItemDTO(it, projects[it.DemandID])`。

`internal/api/handler/bill.go`：3 处 `newBillDetailDTO(...)` 调用点统一改走新 helper：

```go
// detail 组装含明细项目标签的账单详情响应
func (h *Bill) detail(c echo.Context, b *ent.Bill) error {
	projects, err := h.svc.ItemProjects(c.Request().Context(), b.Edges.Items)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newBillDetailDTO(b, projects))
}
```

原 `return api.OK(c, newBillDetailDTO(b))` 等 3 处改为 `return h.detail(c, b)`（变量名以实际为准）。

- [ ] **Step 6: openapi 与全量测试**

`openapi.yaml` 的 `BillItem` schema `properties` 加：

```yaml
        projects:
          type: array
          description: 所属需求的项目标签，实时关联（非快照），需求软删后仍可追溯
          items:
            $ref: '#/components/schemas/Project'
```

```bash
go build ./... && go test ./... -count=1
```

Expected: 全部 PASS（handler 现有账单测试若断言完整 JSON 需补 `"projects":[]`，按报错修复）。

- [ ] **Step 7: 提交**

```bash
git add -A && git commit -m "feat(bill): 账单明细响应携带需求的项目标签"
```

---

### Task 7: 前端类型与 API 封装

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts`
- Create: `dashboard/apps/web-antdv-next/src/api/project.ts`
- Modify: `dashboard/apps/web-antdv-next/src/api/demand.ts`

**Interfaces:**
- Consumes: Task 3/5/6 的接口
- Produces:
  - `Api.Project.Item`、`Api.Project.Ref`、`Api.Project.SaveParams`
  - `fetchProjects() / createProject(params) / updateProject(id, params) / deleteProject(id)`
  - `fetchDemands(params?: { project_id?: number; status?: Api.Demand.Status })` —— 签名变化
  - `updateDemandProjects(id, projectIds)`

- [ ] **Step 1: 类型定义**

`api.d.ts` 在 `namespace Demand` 之前加：

```ts
  /** 项目（轻量标签） */
  namespace Project {
    /** 项目实体，demand_count 仅列表接口有效 */
    interface Item {
      id: number;
      name: string;
      color: string;
      remark: string;
      demand_count: number;
      created_at: string;
      updated_at: string;
    }

    /** 需求 / 账单明细上携带的精简引用；需求走 ent 序列化，可选字段带 omitempty */
    interface Ref {
      id: number;
      name: string;
      color?: string;
    }

    /** 创建 / 更新请求体 */
    interface SaveParams {
      name: string;
      color?: string;
      remark?: string;
    }
  }
```

`Api.Demand.Item` 加字段（放在 `updated_at` 之后）：

```ts
      /** ent 关联预加载结果，目前仅项目标签 */
      edges?: {
        projects?: Api.Project.Ref[];
      };
```

`Api.Demand.CreateParams` 加：

```ts
      /** 关联的项目 ID 列表，可选 */
      project_ids?: number[];
```

`Api.Bill.Item` 加：

```ts
      /** 所属需求的项目标签，实时关联（非快照） */
      projects: Api.Project.Ref[];
```

- [ ] **Step 2: 项目 API 封装**

创建 `src/api/project.ts`：

```ts
import { requestClient } from '#/api/request';

/** 查询项目列表，登录即可，供管理页、下拉与筛选使用 */
export function fetchProjects() {
  return requestClient.get<Api.Project.Item[]>('/api/projects');
}

/** 创建项目（仅超级管理员），名称唯一 */
export function createProject(params: Api.Project.SaveParams) {
  return requestClient.post<Api.Project.Item>('/api/projects', params);
}

/** 更新项目（仅超级管理员），全量覆盖名称、颜色与备注 */
export function updateProject(id: number, params: Api.Project.SaveParams) {
  return requestClient.put<Api.Project.Item>(`/api/projects/${id}`, params);
}

/** 删除项目（仅超级管理员），自动解除与需求的关联，需求本身不受影响 */
export function deleteProject(id: number): Promise<void> {
  return requestClient.delete(`/api/projects/${id}`);
}
```

- [ ] **Step 3: 需求 API 变更**

`src/api/demand.ts`：`fetchDemands` 改为对象参数：

```ts
/** 查询需求列表，可按状态与项目筛选，缺省返回全部 */
export function fetchDemands(params?: {
  project_id?: number;
  status?: Api.Demand.Status;
}) {
  return requestClient.get<Api.Demand.Item[]>('/api/demands', { params });
}
```

新增：

```ts
/** 全量覆盖需求的项目标签，传空数组即清空；任何状态可用，登录即可操作 */
export function updateDemandProjects(id: number, projectIds: number[]) {
  return requestClient.put<Api.Demand.Item>(`/api/demands/${id}/projects`, {
    project_ids: projectIds,
  });
}
```

更新 `fetchDemands` 的全部调用点（`grep -rn "fetchDemands" src/` 逐一改）：`views/demands/index.vue` 的 `fetchDemands(status.value)` 改为 `fetchDemands({ status: status.value })`；其他调用点同理。

- [ ] **Step 4: 校验与提交**

```bash
cd dashboard && pnpm check:type && pnpm lint
```

Expected: 无错误（lint 有自动修复项则先跑 `pnpm format`）。

```bash
git add -A && git commit -m "feat(dashboard): 项目 API 封装与需求 / 账单类型扩展"
```

---

### Task 8: 项目管理页与菜单

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/projects/index.vue`
- Create: `dashboard/apps/web-antdv-next/src/views/projects/components/ProjectFormDialog.vue`
- Create: `dashboard/apps/web-antdv-next/src/router/routes/modules/projects.ts`

**Interfaces:**
- Consumes: Task 7 的 `fetchProjects/createProject/updateProject/deleteProject`
- Produces: 超管专属「项目管理」菜单页

- [ ] **Step 1: 路由模块**

创建 `src/router/routes/modules/projects.ts`（对照 `users.ts` 风格）：

```ts
import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'ProjectList',
    path: '/projects',
    component: () => import('#/views/projects/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:folder-2-line',
      order: 15,
      title: '项目管理',
    },
  },
];

export default routes;
```

- [ ] **Step 2: 表单弹窗**

创建 `src/views/projects/components/ProjectFormDialog.vue`：

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input, Select, Tag, Textarea } from 'antdv-next';

import { createProject, updateProject } from '#/api/project';
import { showSuccess } from '#/utils/http/error';

/** 项目创建 / 编辑弹窗 */
defineOptions({ name: 'ProjectFormDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const project = ref<Api.Project.Item>();
const formRef = ref<FormInstance>();

/** antdv Tag 预设色，value 与后端存储的 color 字符串一一对应，空串为默认色 */
const COLOR_OPTIONS = [
  { label: '默认', value: '' },
  { label: '蓝色', value: 'blue' },
  { label: '绿色', value: 'green' },
  { label: '橙色', value: 'orange' },
  { label: '红色', value: 'red' },
  { label: '紫色', value: 'purple' },
  { label: '青色', value: 'cyan' },
  { label: '金色', value: 'gold' },
  { label: '洋红', value: 'magenta' },
];

const form = reactive({
  color: '',
  name: '',
  remark: '',
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入项目名称', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { project: target } = modalApi.getData<{
      project?: Api.Project.Item;
    }>();
    project.value = target;
    form.name = target?.name ?? '';
    form.color = target?.color ?? '';
    form.remark = target?.remark ?? '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑项目' : '新建项目' });
  },
});

/** 保存：有编辑对象走更新，否则创建 */
async function save() {
  try {
    await formRef.value?.validate();
  } catch {
    // 校验失败的提示已由 FormItem 就地展示
    return;
  }

  modalApi.lock();
  try {
    const params: Api.Project.SaveParams = {
      color: form.color,
      name: form.name.trim(),
      remark: form.remark.trim(),
    };
    await (project.value
      ? updateProject(project.value.id, params)
      : createProject(params));
    showSuccess('已保存');
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[440px]">
    <Form
      ref="formRef"
      :label-col="{ style: { width: '64px' } }"
      :model="form"
      :rules="rules"
      class="pt-2"
    >
      <FormItem label="名称" name="name">
        <Input
          v-model:value="form.name"
          :maxlength="50"
          placeholder="项目名称，唯一"
          show-count
        />
      </FormItem>
      <FormItem label="颜色" name="color">
        <Select v-model:value="form.color" :options="COLOR_OPTIONS">
          <template #option="{ label, value }">
            <Tag :color="value || undefined">{{ label }}</Tag>
          </template>
        </Select>
      </FormItem>
      <FormItem label="备注" name="remark">
        <Textarea
          v-model:value="form.remark"
          :rows="3"
          placeholder="可留空"
        />
      </FormItem>
    </Form>
  </Modal>
</template>
```

- [ ] **Step 3: 列表页**

创建 `src/views/projects/index.vue`：

```vue
<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { onMounted, ref } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';

import { Button, Table, Tag } from 'antdv-next';

import { deleteProject, fetchProjects } from '#/api/project';
import { formatDateTime } from '#/utils/clepsydra/date';
import { showSuccess } from '#/utils/http/error';

import ProjectFormDialog from './components/ProjectFormDialog.vue';

/** 项目管理，仅超级管理员可见；项目是需求的轻量归类标签 */
defineOptions({ name: 'ProjectList' });

const list = ref<Api.Project.Item[]>([]);
const loading = ref(false);

const columns: TableColumnsType<Api.Project.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { key: 'name', minWidth: 160, title: '名称' },
  { dataIndex: 'remark', ellipsis: true, key: 'remark', minWidth: 200, title: '备注' },
  { dataIndex: 'demand_count', key: 'demand_count', title: '关联需求', width: 100 },
  { key: 'created_at', title: '创建时间', width: 176 },
  { key: 'action', title: '操作', width: 140 },
];

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: ProjectFormDialog,
});

/** 加载项目列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchProjects();
  } finally {
    loading.value = false;
  }
}

/** 打开新建弹窗，不带编辑对象即创建模式 */
function openCreate() {
  formModalApi.setData({}).open();
}

function openEdit(row: Api.Project.Item) {
  formModalApi.setData({ project: row }).open();
}

/** 删除项目：仅解除与需求的关联，需求本身不受影响 */
async function remove(row: Api.Project.Item) {
  const suffix =
    row.demand_count > 0
      ? `该项目已关联 ${row.demand_count} 个需求，删除后仅解除关联，不影响需求本身。`
      : '';
  try {
    await confirm(`确定删除项目「${row.name}」吗？${suffix}`, '删除确认');
  } catch {
    // 用户取消
    return;
  }

  await deleteProject(row.id);
  showSuccess('已删除');
  await load();
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-end">
      <Button type="primary" @click="openCreate">新建项目</Button>
    </div>

    <Table
      :columns="columns"
      :data-source="list"
      :loading="loading"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <Tag :color="record.color || undefined">{{ record.name }}</Tag>
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatDateTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <Button type="link" @click="openEdit(record)">编辑</Button>
          <Button danger type="link" @click="remove(record)">删除</Button>
        </template>
      </template>
    </Table>

    <FormModal @success="load" />
  </Page>
</template>
```

- [ ] **Step 4: 校验与提交**

```bash
cd dashboard && pnpm check:type && pnpm lint
```

Expected: 无错误。

```bash
git add -A && git commit -m "feat(dashboard): 项目管理页，超管维护项目标签"
```

---

### Task 9: 需求表单、列表与详情接入项目

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandFormDialog.vue`
- Create: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandProjectsDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/index.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/detail.vue`

**Interfaces:**
- Consumes: Task 7 的 API 与类型
- Produces: 表单多选项目、列表筛选与 tag 列、详情 tag 展示与任意状态可用的「编辑标签」入口

- [ ] **Step 1: DemandFormDialog 加项目多选**

修改 `DemandFormDialog.vue`：

- import 增加 `Select`（antdv-next）、`fetchProjects`（`#/api/project`）、`updateDemandProjects`（`#/api/demand`）
- `form` 增加 `projectIds: [] as number[]`
- 新增选项状态与加载：

```ts
/** 项目多选选项，弹窗每次打开时刷新 */
const projectOptions = ref<{ label: string; value: number }[]>([]);

/** 加载项目选项，失败不阻塞表单其余部分 */
async function loadProjectOptions() {
  try {
    const projects = await fetchProjects();
    projectOptions.value = projects.map((p) => ({
      label: p.name,
      value: p.id,
    }));
  } catch {
    // 错误提示已由请求拦截器统一弹出
  }
}
```

- `onOpenChange` 打开分支里初始化并加载：

```ts
    form.projectIds = (target?.edges?.projects ?? []).map((p) => p.id);
    void loadProjectOptions();
```

- `save()` 里：创建分支 `params.project_ids = form.projectIds.length > 0 ? form.projectIds : undefined;`（放在组装 `params` 处、仅创建模式生效不影响 PUT 请求体也无妨——`Api.Demand.CreateParams` 是 `SaveParams` 超集，更新接口会忽略多余字段；为清晰仍建议仅创建模式赋值）；编辑分支在 `updateDemand` 成功后追加：

```ts
    if (demand.value) {
      await updateDemand(demand.value.id, params);
      // 标签走独立接口，与标题描述的状态锁定解耦
      await updateDemandProjects(demand.value.id, form.projectIds);
    } else {
      await createDemand(params);
    }
```

（替换原 `await (demand.value ? updateDemand(...) : createDemand(...))` 三元式。）

- 模板在「标题」与预估区块之间加：

```vue
      <FormItem label="项目" name="projectIds">
        <Select
          v-model:value="form.projectIds"
          :options="projectOptions"
          allow-clear
          mode="multiple"
          placeholder="选择所属项目（可多选，可留空）"
        />
      </FormItem>
```

- [ ] **Step 2: 新建 DemandProjectsDialog**

创建 `src/views/demands/components/DemandProjectsDialog.vue`：

```vue
<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Select } from 'antdv-next';

import { updateDemandProjects } from '#/api/demand';
import { fetchProjects } from '#/api/project';
import { showSuccess } from '#/utils/http/error';

/**
 * 需求项目标签编辑弹窗
 *
 * 与 DemandFormDialog 的区别：标签不受需求状态锁定，任何状态（含已验收）都可用，
 * 存量已完成需求也能补打标签
 */
defineOptions({ name: 'DemandProjectsDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const demandId = ref(0);
const projectIds = ref<number[]>([]);
const options = ref<{ label: string; value: number }[]>([]);

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  async onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    projectIds.value = (demand.edges?.projects ?? []).map((p) => p.id);
    modalApi.setState({ title: '编辑项目标签' });

    try {
      const projects = await fetchProjects();
      options.value = projects.map((p) => ({ label: p.name, value: p.id }));
    } catch {
      // 错误提示已由请求拦截器统一弹出
    }
  },
});

/** 保存：全量覆盖需求的项目标签 */
async function save() {
  modalApi.lock();
  try {
    await updateDemandProjects(demandId.value, projectIds.value);
    showSuccess('已保存');
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[440px]">
    <Select
      v-model:value="projectIds"
      :options="options"
      allow-clear
      class="w-full"
      mode="multiple"
      placeholder="选择所属项目（可多选，清空即移除全部标签）"
    />
  </Modal>
</template>
```

- [ ] **Step 3: 需求列表筛选与 tag 列**

修改 `views/demands/index.vue`：

- import 增加 `fetchProjects`；新增状态：

```ts
/** 项目筛选，undefined 表示全部 */
const projectId = ref<number | undefined>(undefined);
const projectOptions = ref<{ label: string; value: number }[]>([]);
```

- `load()` 改为 `list.value = await fetchDemands({ project_id: projectId.value, status: status.value });`
- `onMounted` 里追加加载筛选选项：

```ts
onMounted(() => {
  void load();
  fetchProjects()
    .then((projects) => {
      projectOptions.value = projects.map((p) => ({
        label: p.name,
        value: p.id,
      }));
    })
    .catch(() => {
      // 错误提示已由请求拦截器统一弹出
    });
});
```

（替换原 `onMounted(load);`。）

- `columns` 在 `title` 列后加 `{ key: 'projects', title: '项目', width: 180 },`
- 模板筛选区状态 Select 旁（包一层 `<div class="flex items-center gap-2">`）加：

```vue
        <Select
          v-model:value="projectId"
          :options="projectOptions"
          allow-clear
          class="w-45"
          placeholder="全部项目"
          @change="load"
        />
```

- `bodyCell` 加分支：

```vue
        <template v-else-if="column.key === 'projects'">
          <Tag
            v-for="p in record.edges?.projects ?? []"
            :key="p.id"
            :color="p.color || undefined"
          >
            {{ p.name }}
          </Tag>
        </template>
```

- [ ] **Step 4: 需求详情展示与编辑入口**

修改 `views/demands/detail.vue`：

- import `DemandProjectsDialog`，注册弹窗：

```ts
const [ProjectsModal, projectsModalApi] = useVbenModal({
  connectedComponent: DemandProjectsDialog,
});
```

- `Descriptions` 末尾（更新时间之后）加：

```vue
            <DescriptionsItem :span="2" label="项目">
              <Tag
                v-for="p in demand.edges?.projects ?? []"
                :key="p.id"
                :color="p.color || undefined"
              >
                {{ p.name }}
              </Tag>
              <Button
                size="small"
                type="link"
                @click="projectsModalApi.setData({ demand }).open()"
              >
                编辑标签
              </Button>
            </DescriptionsItem>
```

- 模板底部弹窗区加 `<ProjectsModal @success="load" />`

- [ ] **Step 5: 校验与提交**

```bash
cd dashboard && pnpm check:type && pnpm lint
```

Expected: 无错误。

```bash
git add -A && git commit -m "feat(dashboard): 需求表单多选项目、列表筛选与详情标签编辑"
```

---

### Task 10: 账单详情明细展示项目

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/views/bills/detail.vue`

**Interfaces:**
- Consumes: Task 6 的明细 `projects` 字段（`Api.Bill.Item.projects`，Task 7 已定义类型）

- [ ] **Step 1: 加项目列**

`columns` 在 `demand_title` 列后加：

```ts
  { key: 'projects', title: '项目', width: 160 },
```

明细表格 `bodyCell` 加分支（对照该文件现有分支写法）：

```vue
        <template v-else-if="column.key === 'projects'">
          <Tag
            v-for="p in record.projects"
            :key="p.id"
            :color="p.color || undefined"
          >
            {{ p.name }}
          </Tag>
        </template>
```

- [ ] **Step 2: 校验与提交**

```bash
cd dashboard && pnpm check:type && pnpm lint
```

Expected: 无错误。

```bash
git add -A && git commit -m "feat(dashboard): 账单明细展示需求的项目标签"
```

---

### Task 11: 审计字典与全量校验

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/views/audit-logs/index.vue`

- [ ] **Step 1: 补审计筛选字典**

`TARGET_TYPE_OPTIONS` 加 `{ label: '项目', value: 'project' },`。

`ACTION_OPTIONS` 加（需求组末尾与列表末尾）：

```ts
  { label: '更新需求项目标签', value: 'demand.update_projects' },
  { label: '创建项目', value: 'project.create' },
  { label: '更新项目', value: 'project.update' },
  { label: '删除项目', value: 'project.delete' },
```

同步更新该常量上方「枚举自 …」注释，把 `project.go` 纳入枚举来源说明。

- [ ] **Step 2: 全量校验**

```bash
go build ./... && go test ./... -count=1
/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=master --timeout=10m
cd dashboard && pnpm check:type && pnpm lint && pnpm test:unit
```

Expected: 全部通过（gclint 的 `--new-from-rev` 按分支起点调整；若在 master 直接开发则用 `HEAD~1` 逐提交跑过即可）。

- [ ] **Step 3: 提交**

```bash
git add -A && git commit -m "feat(dashboard): 审计筛选字典补项目相关动作"
```

---

## Self-Review 记录

- Spec 覆盖：数据模型（Task 1）、项目 CRUD 接口与审计（Task 2/3）、需求关联与筛选（Task 4/5）、账单明细（Task 6）、前端项目管理页（Task 8）、需求表单/列表/详情（Task 9）、账单详情（Task 10）、错误处理（重名/无效 ID/404 分布在 Task 2/4）、测试（各任务内嵌）——无遗漏
- 类型一致性：`Create(..., projectIDs []int)`、`List(ctx, status, projectID)`、`UpdateProjects`、`ItemProjects`、`newBillDetailDTO(b, projects)` 前后引用一致
- 路由计数：Task 3 +4 条（38）、Task 5 +1 条（39），docs_test 分两次更新
