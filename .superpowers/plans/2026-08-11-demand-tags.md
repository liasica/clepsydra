# 需求性质标签实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为需求增加独立的「性质标签」（Tag）维度：标签统一管理、颜色由后端按名称自动生成并固化、需求多选打标、列表可筛选。

**Architecture:** 完全平行复刻现有「项目」（Project）轻量标签模式——ent 多对多、service/handler 分层、审计日志、前端管理页与需求侧集成。唯一差异：颜色不可外部指定，创建时由 FNV-1a hash → HSL → hex 生成后固化存库，改名不重算。

**Tech Stack:** Go + ent + echo + Postgres（测试用 sqlite3），前端 Vue3 + antdv-next（vben 框架），spec 见 `.superpowers/specs/2026-08-11-demand-tags-design.md`。

## Global Constraints

- 注释使用中文，句末不加标点；遵循 `/Users/liasica/Golang规范.md`
- Git 提交遵循 Conventional Commits，禁止任何 AI 署名
- 每次 Go 代码提交前运行 `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`，保证无 issue
- 前端代码提交前在 `dashboard/` 运行 `pnpm lint` 保证无 issue
- 数据库迁移由启动时 `client.Schema.Create` 自动完成，无需手写迁移
- 禁止修改与本任务无关的代码

---

### Task 1: ent schema——Tag 实体与 Demand 多对多

**Files:**
- Create: `internal/ent/schema/tag.go`
- Modify: `internal/ent/schema/demand.go:50-54`（Edges 方法）
- 生成代码: `internal/ent/`（`make generate`）

**Interfaces:**
- Produces: `ent.Tag`（字段 `Name`、`Color`、`CreatedAt`、`UpdatedAt`），`client.Tag` 查询构造器，`Demand` 侧 `WithTags()` / `AddTagIDs()` / `ClearTags()` / `demand.HasTagsWith()`，中间表 `tag_demands`

- [ ] **Step 1: 创建 Tag schema**

创建 `internal/ent/schema/tag.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Tag 需求性质标签（如新功能、缺陷修复、优化），颜色创建时按名称生成并固化，改名不重算
// 不做软删除：删除即物理删除，与需求的关联由中间表外键级联清除
type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
		field.String("color"), // 十六进制色值（如 #3b82f6），由服务端生成，不接受外部传入
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("demands", Demand.Type),
	}
}
```

- [ ] **Step 2: Demand 增加 tags edge**

修改 `internal/ent/schema/demand.go` 的 Edges 方法：

```go
// Edges 需求与项目 / 标签的多对多关联：均为轻量归类元数据，不影响人天与账单金额
func (Demand) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("projects", Project.Type).Ref("demands"),
		edge.From("tags", Tag.Type).Ref("demands"),
	}
}
```

- [ ] **Step 3: 生成 ent 代码并验证编译**

Run: `make generate && go build ./...`
Expected: 生成 `internal/ent/tag*.go` 等文件，编译无错误

- [ ] **Step 4: 运行现有测试确认无回归**

Run: `go test ./internal/... 2>&1 | tail -20`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ent
git commit -m "feat(ent): 新增需求性质标签 Tag 实体与多对多关联"
```

---

### Task 2: 颜色生成函数 tagColor

**Files:**
- Create: `internal/service/tagcolor.go`
- Test: `internal/service/tagcolor_test.go`

**Interfaces:**
- Produces: `tagColor(name string) string`（service 包内私有函数，返回形如 `#8a2be2` 的小写十六进制色值），Task 3 的 `Tag.Create` 调用

- [ ] **Step 1: 写失败测试**

创建 `internal/service/tagcolor_test.go`：

```go
package service

import (
	"regexp"
	"testing"
)

// TestTagColor 颜色生成的确定性与格式
func TestTagColor(t *testing.T) {
	c1 := tagColor("优化")
	c2 := tagColor("优化")
	if c1 != c2 {
		t.Errorf("同名应得同色: %s != %s", c1, c2)
	}

	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(c1) {
		t.Errorf("颜色格式应为小写十六进制: %s", c1)
	}

	// 抽样几个常见标签名，不应全部撞色
	if tagColor("新功能") == tagColor("缺陷修复") && tagColor("缺陷修复") == tagColor("重构") {
		t.Error("多个不同名称全部同色，hash 可能失效")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestTagColor -v`
Expected: FAIL，报 `undefined: tagColor`（编译错误）

- [ ] **Step 3: 实现 tagColor**

创建 `internal/service/tagcolor.go`：

```go
package service

import (
	"fmt"
	"hash/fnv"
	"math"
)

// tagColor 按标签名生成十六进制颜色：FNV-1a hash 取色相，固定饱和度 65%、亮度 50%
// 仅在创建标签时调用一次并固化存库，同名必得同色；改名不重算，历史标签颜色不随算法调整变化
func tagColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	hue := float64(h.Sum32() % 360)

	r, g, b := hslToRGB(hue, 0.65, 0.50)

	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// hslToRGB HSL 转 RGB，h 取值 [0,360)，s / l 取值 [0,1]
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return uint8(math.Round((r + m) * 255)),
		uint8(math.Round((g + m) * 255)),
		uint8(math.Round((b + m) * 255))
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -run TestTagColor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/tagcolor.go internal/service/tagcolor_test.go
git commit -m "feat(service): 标签颜色生成函数，按名称 hash 取 HSL 色相"
```

---

### Task 3: Tag service

**Files:**
- Create: `internal/service/tag.go`
- Test: `internal/service/tag_test.go`

**Interfaces:**
- Consumes: Task 1 的 `client.Tag`，Task 2 的 `tagColor`，现有 `Audit.Record` / `Actor` / `ErrBadRequest` / `ErrNotFound` / `Seed`
- Produces: `service.Tag` 结构体与 `NewTag(client *ent.Client, audit *Audit) *Tag`；方法 `List(ctx) ([]*ent.Tag, error)`、`Create(ctx, actor Actor, name string) (*ent.Tag, error)`、`Update(ctx, actor Actor, id int, name string) (*ent.Tag, error)`、`Delete(ctx, actor Actor, id int) error`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/tag_test.go`（`admin` fixture 已在 `demand_test.go:33` 定义，直接使用）：

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

// newTagEnv 构建 Tag 测试环境
func newTagEnv(t *testing.T, name string) (*ent.Client, *Tag) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	return client, NewTag(client, NewAudit(client))
}

// TestTagCRUD 覆盖创建、更新、删除与列表的正常路径，重点校验颜色生成与固化
func TestTagCRUD(t *testing.T) {
	_, svc := newTagEnv(t, "tcrud")
	ctx := context.Background()

	tg, err := svc.Create(ctx, admin, "优化")
	if err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	if tg.Name != "优化" {
		t.Errorf("创建结果异常: %+v", tg)
	}
	if tg.Color != tagColor("优化") {
		t.Errorf("颜色应由名称生成: got %s, want %s", tg.Color, tagColor("优化"))
	}

	// 改名不重算颜色：固化的是创建时的色值
	created := tg.Color
	tg, err = svc.Update(ctx, admin, tg.ID, "性能优化")
	if err != nil {
		t.Fatalf("更新标签失败: %v", err)
	}
	if tg.Name != "性能优化" {
		t.Errorf("更新结果异常: %+v", tg)
	}
	if tg.Color != created {
		t.Errorf("改名后颜色应保持固化值: got %s, want %s", tg.Color, created)
	}

	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("列表失败: %v, len=%d", err, len(rows))
	}

	if err = svc.Delete(ctx, admin, tg.ID); err != nil {
		t.Fatalf("删除标签失败: %v", err)
	}
	rows, _ = svc.List(ctx)
	if len(rows) != 0 {
		t.Errorf("删除后列表应为空, len=%d", len(rows))
	}
}

// TestTagValidation 覆盖空名称、重名与不存在记录的错误路径
func TestTagValidation(t *testing.T) {
	_, svc := newTagEnv(t, "tvalid")
	ctx := context.Background()

	if _, err := svc.Create(ctx, admin, ""); err == nil {
		t.Error("空名称应报错")
	}

	t1, _ := svc.Create(ctx, admin, "新功能")
	t2, _ := svc.Create(ctx, admin, "缺陷修复")

	if _, err := svc.Create(ctx, admin, "新功能"); err == nil {
		t.Error("重名创建应报错")
	}
	if _, err := svc.Update(ctx, admin, t2.ID, "新功能"); err == nil {
		t.Error("更新为已有名称应报错")
	}
	// 名称不变的更新不应误报重名
	if _, err := svc.Update(ctx, admin, t1.ID, "新功能"); err != nil {
		t.Errorf("原名更新不应报错: %v", err)
	}

	if _, err := svc.Update(ctx, admin, 999, "任意"); err != ErrNotFound {
		t.Errorf("更新不存在标签应返回 ErrNotFound, got %v", err)
	}
	if err := svc.Delete(ctx, admin, 999); err != ErrNotFound {
		t.Errorf("删除不存在标签应返回 ErrNotFound, got %v", err)
	}
}

// TestTagDemandCountAndDetach 关联需求数统计（软删需求不计入）与删除标签解除关联
func TestTagDemandCountAndDetach(t *testing.T) {
	client, svc := newTagEnv(t, "tcount")
	ctx := context.Background()

	tg, _ := svc.Create(ctx, admin, "优化")
	// estimated_half_days 为必填字段，测试仅关心标签关联，取 0 即可
	d1 := client.Demand.Create().SetTitle("需求一").SetEstimatedHalfDays(0).AddTagIDs(tg.ID).SaveX(ctx)
	client.Demand.Create().SetTitle("需求二").SetEstimatedHalfDays(0).AddTagIDs(tg.ID).SaveX(ctx)

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

	// 删除标签自动解除关联，需求本身不受影响
	if err = svc.Delete(ctx, admin, tg.ID); err != nil {
		t.Fatalf("删除标签失败: %v", err)
	}
	demands := client.Demand.Query().WithTags().AllX(ctx)
	for _, d := range demands {
		if len(d.Edges.Tags) != 0 {
			t.Errorf("需求 %d 的标签关联应已解除", d.ID)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestTag -v`
Expected: FAIL，报 `undefined: NewTag`（编译错误；TestTagColor 因同包编译错误一并失败属预期）

- [ ] **Step 3: 实现 Tag service**

创建 `internal/service/tag.go`：

```go
package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/tag"
)

// Tag 标签服务，需求性质标签的增删改查
// 颜色在创建时按名称生成并固化，更新只改名不动色，接口层不接受外部传入颜色
type Tag struct {
	client *ent.Client
	audit  *Audit
}

// NewTag 构建标签服务
func NewTag(client *ent.Client, audit *Audit) *Tag {
	return &Tag{client: client, audit: audit}
}

// List 查询全部标签，预加载关联需求供 handler 统计关联数
// 关联需求查询会走 Demand 的软删除拦截器，已软删需求不计入
func (s *Tag) List(ctx context.Context) ([]*ent.Tag, error) {
	return s.client.Tag.Query().
		WithDemands().
		Order(ent.Asc(tag.FieldID)).
		All(ctx)
}

// Create 创建标签，名称必填且唯一，颜色按名称生成后固化
func (s *Tag) Create(ctx context.Context, actor Actor, name string) (*ent.Tag, error) {
	if name == "" {
		return nil, ErrBadRequest("标签名称不能为空")
	}

	exists, err := s.client.Tag.Query().Where(tag.Name(name)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("标签名称已存在")
	}

	t, err := s.client.Tag.Create().
		SetName(name).
		SetColor(tagColor(name)).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "tag.create", "tag", t.ID, map[string]any{
		"name": name,
	})

	return t, nil
}

// Update 更新标签名称；颜色保持创建时的固化值不变
func (s *Tag) Update(ctx context.Context, actor Actor, id int, name string) (*ent.Tag, error) {
	if name == "" {
		return nil, ErrBadRequest("标签名称不能为空")
	}

	// 重名检查排除自身，允许原名保存
	exists, err := s.client.Tag.Query().
		Where(tag.Name(name), tag.IDNEQ(id)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("标签名称已存在")
	}

	t, err := s.client.Tag.UpdateOneID(id).
		SetName(name).
		Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "tag.update", "tag", id, map[string]any{
		"name": name,
	})

	return t, nil
}

// Delete 物理删除标签，与需求的关联由中间表外键级联清除，需求本身不受影响
func (s *Tag) Delete(ctx context.Context, actor Actor, id int) error {
	t, err := s.client.Tag.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if err = s.client.Tag.DeleteOneID(id).Exec(ctx); err != nil {
		// Get 与 DeleteOneID 之间存在并发删除窗口，命中时转 404 而非原生 NotFound 导致的 500
		if ent.IsNotFound(err) {
			return ErrNotFound
		}

		return err
	}

	s.audit.Record(ctx, actor, "tag.delete", "tag", id, map[string]any{
		"name": t.Name,
	})

	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -run TestTag -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/tag.go internal/service/tag_test.go
git commit -m "feat(service): 标签增删改查，颜色创建时生成并固化"
```

---

### Task 4: Tag handler + 路由 + 装配

**Files:**
- Create: `internal/api/handler/tag.go`
- Modify: `internal/api/router.go`（TagHandler 接口、Handlers 字段、路由）
- Modify: `cmd/clepsydra/main.go:80-104`（装配 tagSvc 与 handler）
- Test: `internal/api/handler/tag_test.go`

**Interfaces:**
- Consumes: Task 3 的 `service.Tag`（`NewTag` / `List` / `Create` / `Update` / `Delete`），现有 `api.OK` / `api.Fail` / `parseID` / `actor`
- Produces: 路由 `GET /api/tags`（登录）、`POST /api/tags`、`PUT /api/tags/:id`、`DELETE /api/tags/:id`（均超管）；响应 DTO 字段 `id` / `name` / `color` / `demand_count` / `created_at` / `updated_at`

- [ ] **Step 1: 写失败测试**

创建 `internal/api/handler/tag_test.go`：

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

// TestTagHandlerCRUD 覆盖标签接口的创建、列表、更新与删除，重点校验颜色不接受外部传入
func TestTagHandlerCRUD(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:htag?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	h := NewTag(service.NewTag(client, service.NewAudit(client)))
	e := echo.New()

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
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
		switch method {
		case http.MethodGet:
			err = h.List(c)
		case http.MethodPost:
			err = h.Create(c)
		case http.MethodPut:
			err = h.Update(c)
		case http.MethodDelete:
			err = h.Delete(c)
		}
		if err != nil {
			t.Fatalf("%s %s 错误: %v", method, path, err)
		}
		return rec
	}

	// 请求体带 color 字段应被忽略：颜色只能由服务端生成
	rec := do(http.MethodPost, "/api/tags", `{"name":"优化","color":"red"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("创建响应异常: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"color":"red"`) {
		t.Errorf("颜色不应接受外部传入: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"color":"#`) {
		t.Errorf("响应应含生成的十六进制颜色: %s", rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/tags", "")
	if !strings.Contains(rec.Body.String(), `"demand_count":0`) {
		t.Errorf("列表应含 demand_count: %s", rec.Body.String())
	}

	// 从服务层取回 ID 再走更新与删除
	rows, _ := service.NewTag(client, service.NewAudit(client)).List(ctx)
	id := strconv.Itoa(rows[0].ID)
	created := rows[0].Color

	rec = do(http.MethodPut, "/api/tags/"+id, `{"name":"性能优化"}`)
	if !strings.Contains(rec.Body.String(), "性能优化") {
		t.Errorf("更新响应异常: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"color":"`+created+`"`) {
		t.Errorf("改名后颜色应保持固化值 %s: %s", created, rec.Body.String())
	}

	rec = do(http.MethodDelete, "/api/tags/"+id, "")
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Errorf("删除响应异常: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/api/handler/ -run TestTagHandlerCRUD -v`
Expected: FAIL，报 `undefined: NewTag`（编译错误）

- [ ] **Step 3: 实现 Tag handler**

创建 `internal/api/handler/tag.go`：

```go
package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Tag 标签管理接口
type Tag struct {
	svc *service.Tag
}

// NewTag 构建标签 handler
func NewTag(svc *service.Tag) *Tag {
	return &Tag{svc: svc}
}

// tagRequest 创建 / 更新请求体，仅名称：颜色由服务端按名称生成并固化，不接受外部传入
type tagRequest struct {
	Name string `json:"name"`
}

// tagDTO 标签响应结构；demand_count 为关联需求数（不含已软删需求），
// 仅列表接口的查询预加载了关联，创建 / 更新响应中恒为 0，前端以列表为准
type tagDTO struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	DemandCount int       `json:"demand_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// newTagDTO 将 ent.Tag 映射为响应结构
func newTagDTO(t *ent.Tag) tagDTO {
	return tagDTO{
		ID:          t.ID,
		Name:        t.Name,
		Color:       t.Color,
		DemandCount: len(t.Edges.Demands),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// List GET /api/tags
func (h *Tag) List(c echo.Context) error {
	rows, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	dtos := make([]tagDTO, 0, len(rows))
	for _, t := range rows {
		dtos = append(dtos, newTagDTO(t))
	}

	return api.OK(c, dtos)
}

// Create POST /api/tags
func (h *Tag) Create(c echo.Context) error {
	var req tagRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	t, err := h.svc.Create(c.Request().Context(), actor(c), req.Name)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newTagDTO(t))
}

// Update PUT /api/tags/:id
func (h *Tag) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req tagRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	t, err := h.svc.Update(c.Request().Context(), actor(c), id, req.Name)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newTagDTO(t))
}

// Delete DELETE /api/tags/:id
func (h *Tag) Delete(c echo.Context) error {
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

- [ ] **Step 4: 注册路由**

修改 `internal/api/router.go`：

在 `ProjectHandler` 接口定义之后新增：

```go
// TagHandler 标签管理接口方法集
type TagHandler interface {
	List(c echo.Context) error
	Create(c echo.Context) error
	Update(c echo.Context) error
	Delete(c echo.Context) error
}
```

`Handlers` 结构体的 `Project` 字段后新增：

```go
	Tag       TagHandler
```

登录组 `authed.GET("/projects", h.Project.List)` 之后新增：

```go
	authed.GET("/tags", h.Tag.List)
```

超管组 `adminGroup.DELETE("/projects/:id", h.Project.Delete)` 之后新增：

```go
	adminGroup.POST("/tags", h.Tag.Create)
	adminGroup.PUT("/tags/:id", h.Tag.Update)
	adminGroup.DELETE("/tags/:id", h.Tag.Delete)
```

- [ ] **Step 5: main.go 装配**

修改 `cmd/clepsydra/main.go`：`projectSvc := service.NewProject(client, audit)` 之后新增：

```go
	tagSvc := service.NewTag(client, audit)
```

`Handlers` 字面量中 `Project: handler.NewProject(projectSvc),` 之后新增：

```go
		Tag:       handler.NewTag(tagSvc),
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go build ./... && go test ./internal/api/handler/ -run TestTagHandlerCRUD -v`
Expected: 编译通过，PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api/handler/tag.go internal/api/handler/tag_test.go internal/api/router.go cmd/clepsydra/main.go
git commit -m "feat(api): 标签管理接口与路由，写操作仅超级管理员"
```

---

### Task 5: Demand service 标签关联与筛选

**Files:**
- Modify: `internal/service/demand.go`（`List` / `Get` / `Create` / 新增 `UpdateTags` / `normalizeTagIDs` / 抽取 `dedupIDs`）
- Modify: 所有调用 `Demand.Create` / `Demand.List` 的现有测试（编译器指出，追加 `nil` / `0` 实参）
- Test: `internal/service/demand_tags_test.go`

**Interfaces:**
- Consumes: Task 1 的 `WithTags` / `AddTagIDs` / `ClearTags` / `demand.HasTagsWith` / `tag.IDIn`
- Produces: `Demand.List(ctx, status string, projectID, tagID int)`（签名变更，追加 `tagID`）、`Demand.Create(ctx, actor, title, description string, estimatedHalfDays int, plannedStart *time.Time, confirmed bool, projectIDs, tagIDs []int)`（签名变更，追加 `tagIDs`）、新增 `Demand.UpdateTags(ctx, actor Actor, id int, tagIDs []int) (*ent.Demand, error)`；`List` / `Get` 结果带 `Edges.Tags`

- [ ] **Step 1: 写失败测试**

创建 `internal/service/demand_tags_test.go`（`newDemandEnv` / `admin` / `mustGet` 为 `demand_test.go` 现有设施；本步之后 `Create` / `List` 按新签名调用，旧签名调用点在 Step 3 一并修复）：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// tagFixtures 建两个标签供关联测试使用
func tagFixtures(t *testing.T, client *ent.Client) (int, int) {
	t.Helper()
	ctx := context.Background()

	t1 := client.Tag.Create().SetName("新功能").SetColor("#112233").SaveX(ctx)
	t2 := client.Tag.Create().SetName("优化").SetColor("#445566").SaveX(ctx)

	return t1.ID, t2.ID
}

// TestDemandCreateWithTags 创建需求携带标签关联，并校验无效标签 ID 被拒绝
func TestDemandCreateWithTags(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-create")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)

	d, err := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, []int{t1, t2, t1})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got := svc.mustGet(ctx, t, d.ID)
	if len(got.Edges.Tags) != 2 {
		t.Errorf("关联标签数 = %d, want 2（重复 ID 应去重）", len(got.Edges.Tags))
	}

	if _, err = svc.Create(ctx, admin, "需求二", "", 0, nil, false, nil, []int{999}); err == nil {
		t.Error("不存在的标签 ID 应报错")
	}
}

// TestDemandUpdateTags 覆盖任意状态改标签、全量覆盖与清空
func TestDemandUpdateTags(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-update")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)

	d, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, []int{t1})

	// 直接改库到 accepted，验证锁定态不受状态限制
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").ExecX(ctx)

	got, err := svc.UpdateTags(ctx, admin, d.ID, []int{t2})
	if err != nil {
		t.Fatalf("已验收需求改标签失败: %v", err)
	}
	if len(got.Edges.Tags) != 1 || got.Edges.Tags[0].ID != t2 {
		t.Errorf("覆盖结果异常: %+v", got.Edges.Tags)
	}

	// 空数组清空
	got, err = svc.UpdateTags(ctx, admin, d.ID, nil)
	if err != nil {
		t.Fatalf("清空标签失败: %v", err)
	}
	if len(got.Edges.Tags) != 0 {
		t.Errorf("清空后仍有关联: %+v", got.Edges.Tags)
	}

	if _, err = svc.UpdateTags(ctx, admin, d.ID, []int{999}); err == nil {
		t.Error("不存在的标签 ID 应报错")
	}
	if _, err = svc.UpdateTags(ctx, admin, 999, []int{t1}); err != ErrNotFound {
		t.Errorf("不存在的需求应返回 ErrNotFound, got %v", err)
	}
}

// TestDemandListFilterByTag 按标签筛选需求列表，并验证与项目筛选可叠加
func TestDemandListFilterByTag(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-list")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)
	p := client.Project.Create().SetName("官网").SaveX(ctx)

	_, _ = svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p.ID}, []int{t1})
	_, _ = svc.Create(ctx, admin, "需求二", "", 0, nil, false, nil, []int{t2})
	_, _ = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil, nil)

	rows, err := svc.List(ctx, "", 0, t1)
	if err != nil || len(rows) != 1 || rows[0].Title != "需求一" {
		t.Fatalf("按标签筛选异常: %v, len=%d", err, len(rows))
	}

	// 项目与标签筛选叠加：项目命中但标签不命中应为空
	rows, _ = svc.List(ctx, "", p.ID, t2)
	if len(rows) != 0 {
		t.Errorf("叠加筛选应为空, len=%d", len(rows))
	}

	rows, _ = svc.List(ctx, "", 0, 0)
	if len(rows) != 3 {
		t.Errorf("不筛选应返回全部, len=%d", len(rows))
	}
	// 列表应预加载标签关联
	if rows[len(rows)-1].Edges.Tags == nil {
		t.Error("列表应预加载 Edges.Tags")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestDemandCreateWithTags -v 2>&1 | head -20`
Expected: FAIL（编译错误：`Create` 实参数量不符、`UpdateTags` 未定义）

- [ ] **Step 3: 实现 service 变更**

修改 `internal/service/demand.go`：

import 增加 `"clepsydra/internal/ent/tag"`。

`List` 替换为：

```go
// List 按状态、项目与标签筛选需求，status 为空、projectID / tagID 为 0 表示不筛选；预加载项目与性质标签
func (s *Demand) List(ctx context.Context, status string, projectID, tagID int) ([]*ent.Demand, error) {
	q := s.client.Demand.Query().WithProjects().WithTags().Order(ent.Desc(demand.FieldID))
	if status != "" {
		q = q.Where(demand.StatusEQ(demand.Status(status)))
	}
	if projectID > 0 {
		q = q.Where(demand.HasProjectsWith(project.ID(projectID)))
	}
	if tagID > 0 {
		q = q.Where(demand.HasTagsWith(tag.ID(tagID)))
	}

	return q.All(ctx)
}
```

`Get` 的查询链在 `WithProjects()` 后追加 `.WithTags()`：

```go
// Get 按 ID 查询需求，预加载项目与性质标签
func (s *Demand) Get(ctx context.Context, id int) (*ent.Demand, error) {
	d, err := s.client.Demand.Query().
		Where(demand.ID(id)).
		WithProjects().
		WithTags().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return d, err
}
```

`Create` 签名追加 `tagIDs []int` 形参（位于 `projectIDs` 之后），方法体中：

原：

```go
	ids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	create := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays).
		AddProjectIDs(ids...)
```

改为：

```go
	pids, err := s.normalizeProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	tids, err := s.normalizeTagIDs(ctx, tagIDs)
	if err != nil {
		return nil, err
	}

	create := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays).
		AddProjectIDs(pids...).
		AddTagIDs(tids...)
```

约束错误分支原：

```go
		// 校验通过后写入前项目被并发删除会触发外键约束冲突，转为业务错误而非 500
		if len(ids) > 0 && ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目不存在")
		}
```

改为：

```go
		// 校验通过后写入前项目 / 标签被并发删除会触发外键约束冲突，转为业务错误而非 500
		if (len(pids) > 0 || len(tids) > 0) && ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目或标签不存在")
		}
```

审计 payload 原 `if len(ids) > 0 { payload["project_ids"] = ids }` 改为：

```go
	if len(pids) > 0 {
		payload["project_ids"] = pids
	}
	if len(tids) > 0 {
		payload["tag_ids"] = tids
	}
```

`UpdateProjects` 之后新增 `UpdateTags`：

```go
// UpdateTags 全量覆盖需求的性质标签，任何状态均可：
// 标签是归类元数据，不影响人天与账单金额，存量已完成需求也要能补打标签
func (s *Demand) UpdateTags(ctx context.Context, actor Actor, id int, tagIDs []int) (*ent.Demand, error) {
	ids, err := s.normalizeTagIDs(ctx, tagIDs)
	if err != nil {
		return nil, err
	}

	err = s.client.Demand.UpdateOneID(id).
		ClearTags().
		AddTagIDs(ids...).
		Exec(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		// 校验通过后写入前标签被并发删除会触发外键约束冲突，转为业务错误而非 500
		if len(ids) > 0 && ent.IsConstraintError(err) {
			return nil, ErrBadRequest("标签不存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update_tags", "demand", id, map[string]any{
		"tag_ids": ids,
	})

	return s.Get(ctx, id)
}
```

`normalizeProjectIDs` 重构为复用去重，并新增 `normalizeTagIDs`（整段替换原 `normalizeProjectIDs`）：

```go
// dedupIDs 保序去重
func dedupIDs(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	uniq := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}

	return uniq
}

// normalizeProjectIDs 去重并校验项目 ID 均存在，空切片直接通过
func (s *Demand) normalizeProjectIDs(ctx context.Context, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	uniq := dedupIDs(ids)
	n, err := s.client.Project.Query().Where(project.IDIn(uniq...)).Count(ctx)
	if err != nil {
		return nil, err
	}
	if n != len(uniq) {
		return nil, ErrBadRequest("项目不存在")
	}

	return uniq, nil
}

// normalizeTagIDs 去重并校验标签 ID 均存在，空切片直接通过
func (s *Demand) normalizeTagIDs(ctx context.Context, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	uniq := dedupIDs(ids)
	n, err := s.client.Tag.Query().Where(tag.IDIn(uniq...)).Count(ctx)
	if err != nil {
		return nil, err
	}
	if n != len(uniq) {
		return nil, ErrBadRequest("标签不存在")
	}

	return uniq, nil
}
```

- [ ] **Step 4: 修复全部旧签名调用点**

Run: `go build ./... && go vet ./... 2>&1 | head -30`，随后 `go test ./... 2>&1 | grep -v "^ok" | head -40`

编译器会指出所有旧签名调用点（生产代码仅 `internal/api/handler/demand.go`，留待 Task 6；其余全部是测试文件，如 `internal/service/demand_test.go`、`internal/service/demand_projects_test.go`、`internal/service/bill*_test.go`、`internal/task/task_test.go` 等）。机械修复规则：

- `svc.Create(ctx, ..., <projectIDs>)` → 追加一个 `nil` 实参：`svc.Create(ctx, ..., <projectIDs>, nil)`
- `svc.List(ctx, <status>, <projectID>)` → 追加一个 `0` 实参：`svc.List(ctx, <status>, <projectID>, 0)`

handler 包在 Task 6 前无法编译属预期：本步骤只要求 `go test ./internal/service/... ./internal/task/...` 通过。若 `internal/api/handler` 阻塞 `go build ./...`，可顺手在 `handler/demand.go` 的 `h.svc.Create(...)` 追加 `nil`、`h.svc.List(...)` 追加 `0` 让编译通过（完整 handler 功能在 Task 6 实现）。

Run: `go test ./internal/service/... ./internal/task/... 2>&1 | tail -10`
Expected: 全部 PASS（含新增三个测试）

- [ ] **Step 5: Commit**

```bash
git add internal/service internal/task internal/api/handler/demand.go
git commit -m "feat(service): 需求支持性质标签关联、覆盖更新与列表筛选"
```

---

### Task 6: Demand handler 标签接口与路由

**Files:**
- Modify: `internal/api/handler/demand.go`（`demandCreateRequest`、`List`、新增 `demandTagsRequest` / `UpdateTags`）
- Modify: `internal/api/router.go`（`DemandHandler` 接口、路由）
- Test: `internal/api/handler/demand_test.go`（新增 `TestDemandTagsHandler`）

**Interfaces:**
- Consumes: Task 5 的 `Demand.UpdateTags` / 新版 `List` / `Create` 签名
- Produces: `POST /api/demands` 接受 `tag_ids []int`；`GET /api/demands?tag_id=` 筛选；`PUT /api/demands/:id/tags`（登录即可）

- [ ] **Step 1: 写失败测试**

在 `internal/api/handler/demand_test.go` 末尾新增（仿 `TestDemandProjectsHandler`）：

```go
func TestDemandTagsHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdtag?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	tg := client.Tag.Create().SetName("优化").SetColor("#112233").SaveX(ctx)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建带标签
	body := `{"title":"需求一","tag_ids":[` + strconv.Itoa(tg.ID) + `]}`
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

	rows, _ := svc.List(ctx, "", 0, 0)
	id := strconv.Itoa(rows[0].ID)

	// 独立接口清空标签（需求方也可操作）
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/tags", strings.NewReader(`{"tag_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdateTags(c); err != nil {
		t.Fatalf("改标签错误: %v", err)
	}
	got, _ := svc.Get(ctx, rows[0].ID)
	if len(got.Edges.Tags) != 0 {
		t.Errorf("标签应已清空: %+v", got.Edges.Tags)
	}

	// 列表按标签筛选参数透传
	req = httptest.NewRequest(http.MethodGet, "/api/demands?tag_id="+strconv.Itoa(tg.ID), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatalf("列表错误: %v", err)
	}
	if strings.Contains(rec.Body.String(), "需求一") {
		t.Errorf("标签已清空，按该标签筛选不应包含需求一: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/api/handler/ -run TestDemandTagsHandler -v 2>&1 | head -10`
Expected: FAIL（`UpdateTags` 未定义，或 Create 未透传 `tag_ids` 导致断言失败）

- [ ] **Step 3: 实现 handler 变更**

修改 `internal/api/handler/demand.go`：

`demandCreateRequest` 增加字段：

```go
	TagIDs            []int  `json:"tag_ids"`
```

`demandProjectsRequest` 之后新增：

```go
// demandTagsRequest 性质标签全量覆盖请求体
type demandTagsRequest struct {
	TagIDs []int `json:"tag_ids"`
}
```

`List` 改为：

```go
// List GET /api/demands?status=&project_id=&tag_id=
func (h *Demand) List(c echo.Context) error {
	status := c.QueryParam("status")
	projectID, _ := strconv.Atoi(c.QueryParam("project_id")) // 非法或缺省按 0 处理，即不筛选
	tagID, _ := strconv.Atoi(c.QueryParam("tag_id"))         // 同上

	demands, err := h.svc.List(c.Request().Context(), status, projectID, tagID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, demands)
}
```

`Create` 中调用改为：

```go
	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description,
		req.EstimatedHalfDays, planned, req.Confirmed, req.ProjectIDs, req.TagIDs)
```

`UpdateProjects` 之后新增：

```go
// UpdateTags PUT /api/demands/:id/tags
// 任何状态可用：标签是归类元数据，不影响人天与账单金额
func (h *Demand) UpdateTags(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandTagsRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateTags(c.Request().Context(), actor(c), id, req.TagIDs)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}
```

- [ ] **Step 4: 注册路由与接口方法集**

修改 `internal/api/router.go`：`DemandHandler` 接口 `UpdateProjects` 之后新增一行：

```go
	UpdateTags(c echo.Context) error
```

路由 `authed.PUT("/demands/:id/projects", h.Demand.UpdateProjects)` 之后新增：

```go
	authed.PUT("/demands/:id/tags", h.Demand.UpdateTags)
```

- [ ] **Step 5: 全量测试与 lint**

Run: `go build ./... && go test ./... 2>&1 | grep -v "^ok" | head -10`
Expected: 无编译错误、无 FAIL

Run: `/usr/local/bin/gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m`
Expected: 无 issue（覆盖 Task 5-6 未提交改动；如报 Task 1-4 已提交代码的 issue 也一并修复）

- [ ] **Step 6: Commit**

```bash
git add internal/api
git commit -m "feat(api): 需求创建携带标签、按标签筛选与标签覆盖更新接口"
```

---

### Task 7: openapi.yaml 接口文档

**Files:**
- Modify: `internal/api/docs/openapi.yaml`

**Interfaces:**
- Consumes: Task 4 / 6 定义的路由与 DTO
- Produces: 与实现一致的接口文档（前端类型注释声明与此文件保持一致）

- [ ] **Step 1: 补充文档**

在 `internal/api/docs/openapi.yaml` 中做以下修改（仿照 Projects 的既有写法）：

1. `paths` 中 `/api/projects/{id}` 定义之后新增：

```yaml
  /api/tags:
    get:
      tags: [Tags]
      operationId: tagsList
      summary: 查询标签列表
      description: 全部性质标签及各自关联需求数（不含已软删需求），登录即可访问，供管理页、下拉与筛选使用
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
                        type: array
                        items:
                          $ref: '#/components/schemas/Tag'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '500':
          $ref: '#/components/responses/ServerError'
    post:
      tags: [Tags]
      operationId: tagsCreate
      summary: 创建标签
      description: 创建需求性质标签，名称必填且唯一；颜色由服务端按名称生成并固化，不接受外部传入（仅超级管理员）
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TagSave'
      responses:
        '200':
          description: 创建成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Tag'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'
        '500':
          $ref: '#/components/responses/ServerError'

  /api/tags/{id}:
    put:
      tags: [Tags]
      operationId: tagsUpdate
      summary: 更新标签
      description: 更新标签名称，名称必填且唯一；颜色保持创建时的固化值不变（仅超级管理员）
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/TagID'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TagSave'
      responses:
        '200':
          description: 更新成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Tag'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/ServerError'
    delete:
      tags: [Tags]
      operationId: tagsDelete
      summary: 删除标签
      description: 物理删除并自动解除与需求的关联，需求本身不受影响（仅超级管理员）
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/TagID'
      responses:
        '200':
          description: 删除成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Envelope'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/ServerError'
```

2. `/api/demands/{id}/projects` 定义之后新增：

```yaml
  /api/demands/{id}/tags:
    put:
      tags: [Demands]
      operationId: demandsUpdateTags
      summary: 更新需求的性质标签
      description: 全量覆盖式更新，传空数组即清空；任何状态可用，登录即可操作
      security:
        - bearerAuth: []
      parameters:
        - $ref: '#/components/parameters/DemandID'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                tag_ids:
                  type: array
                  description: 关联的标签 ID 列表，传空数组即清空
                  items:
                    type: integer
      responses:
        '200':
          description: 更新成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Demand'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/ServerError'
```

3. `demandsList` 的 `parameters` 中 `project_id` 之后新增：

```yaml
        - name: tag_id
          in: query
          required: false
          schema:
            type: integer
          description: 按性质标签筛选，缺省不筛选
```

4. `components/parameters` 中 `ProjectID` 之后新增：

```yaml
    TagID:
      name: id
      in: path
      required: true
      description: 标签 ID
      schema:
        type: integer
```

5. `Demand` schema 的 `edges.properties.projects` 之后新增：

```yaml
            tags:
              type: array
              description: 关联的性质标签
              items:
                $ref: '#/components/schemas/Tag'
```

6. `components/schemas` 中 `ProjectSave` 之后新增：

```yaml
    Tag:
      type: object
      description: 需求性质标签，颜色创建时按名称生成并固化
      properties:
        id:
          type: integer
          description: 标签 ID
        name:
          type: string
          description: 标签名称，唯一
        color:
          type: string
          description: "十六进制色值（如 #3b82f6），由服务端生成"
        demand_count:
          type: integer
          description: 关联需求数（不含已软删需求），仅标签列表接口有效
        created_at:
          type: string
          format: date-time
          description: 创建时间
        updated_at:
          type: string
          format: date-time
          description: 更新时间

    TagSave:
      type: object
      description: 标签创建 / 更新请求体，仅名称，颜色由服务端生成
      required: [name]
      properties:
        name:
          type: string
          description: 标签名称，唯一
```

注意：`color` 的 description 必须用双引号包裹——yaml 值内 `#` 前是空白字符时会被解析为注释，裸写会把「#3b82f6…」整段截掉。

7. `demandsCreate` 请求体 schema（搜索 `DemandCreateRequest`）的 `project_ids` 属性之后新增：

```yaml
        tag_ids:
          type: array
          description: 关联的性质标签 ID 列表，可选
          items:
            type: integer
```

（若创建请求体为内联 schema 而无 `DemandCreateRequest` 组件，则在 `/api/demands` POST 的请求体 `project_ids` 同级处添加。）

- [ ] **Step 2: 校验 yaml 合法性**

Run: `python3 -c "import yaml; yaml.safe_load(open('internal/api/docs/openapi.yaml'))" && echo OK`
Expected: 输出 OK

- [ ] **Step 3: Commit**

```bash
git add internal/api/docs/openapi.yaml
git commit -m "docs(api): 标签接口与需求标签字段的 openapi 文档"
```

---

### Task 8: 前端类型与 API 封装

**Files:**
- Modify: `dashboard/apps/web-antdv-next/src/types/api/api.d.ts`
- Create: `dashboard/apps/web-antdv-next/src/api/tag.ts`
- Modify: `dashboard/apps/web-antdv-next/src/api/demand.ts`

**Interfaces:**
- Consumes: Task 4 / 6 的接口
- Produces: `Api.Tag.Item` / `Api.Tag.Ref` / `Api.Tag.SaveParams` 类型；`fetchTags` / `createTag` / `updateTag` / `deleteTag` / `updateDemandTags` 函数；`fetchDemands` 支持 `tag_id`；`Api.Demand.Item.edges.tags`、`Api.Demand.CreateParams.tag_ids`

- [ ] **Step 1: 类型定义**

修改 `src/types/api/api.d.ts`：

`namespace Project` 之后新增：

```ts
  /** 性质标签 */
  namespace Tag {
    /** 标签实体，demand_count 仅列表接口有效；颜色由后端按名称生成并固化 */
    interface Item {
      id: number;
      name: string;
      color: string;
      demand_count: number;
      created_at: string;
      updated_at: string;
    }

    /** 需求上携带的精简引用；需求走 ent 序列化，可选字段带 omitempty */
    interface Ref {
      id: number;
      name: string;
      color?: string;
    }

    /** 创建 / 更新请求体，仅名称，颜色由后端生成，不可指定 */
    interface SaveParams {
      name: string;
    }
  }
```

`Api.Demand.Item` 的 `edges` 改为：

```ts
      /** ent 关联预加载结果：项目与性质标签 */
      edges?: {
        projects?: Api.Project.Ref[];
        tags?: Api.Tag.Ref[];
      };
```

`Api.Demand.CreateParams` 的 `project_ids` 之后新增：

```ts
      /** 关联的性质标签 ID 列表，可选 */
      tag_ids?: number[];
```

- [ ] **Step 2: API 封装**

创建 `src/api/tag.ts`：

```ts
import { requestClient } from '#/api/request';

/** 查询标签列表，登录即可，供管理页、下拉与筛选使用 */
export function fetchTags() {
  return requestClient.get<Api.Tag.Item[]>('/api/tags');
}

/** 创建标签（仅超级管理员），名称唯一，颜色由后端按名称生成并固化 */
export function createTag(params: Api.Tag.SaveParams) {
  return requestClient.post<Api.Tag.Item>('/api/tags', params);
}

/** 更新标签名称（仅超级管理员），颜色保持创建时的固化值不变 */
export function updateTag(id: number, params: Api.Tag.SaveParams) {
  return requestClient.put<Api.Tag.Item>(`/api/tags/${id}`, params);
}

/** 删除标签（仅超级管理员），自动解除与需求的关联，需求本身不受影响 */
export function deleteTag(id: number): Promise<void> {
  return requestClient.delete(`/api/tags/${id}`);
}
```

修改 `src/api/demand.ts`：`fetchDemands` 参数类型加 `tag_id`：

```ts
/** 查询需求列表，可按状态、项目与性质标签筛选，缺省返回全部 */
export function fetchDemands(params?: {
  project_id?: number;
  status?: Api.Demand.Status;
  tag_id?: number;
}) {
  return requestClient.get<Api.Demand.Item[]>('/api/demands', { params });
}
```

文件末尾新增：

```ts
/** 全量覆盖需求的性质标签，传空数组即清空；任何状态可用，登录即可操作 */
export function updateDemandTags(id: number, tagIds: number[]) {
  return requestClient.put<Api.Demand.Item>(`/api/demands/${id}/tags`, {
    tag_ids: tagIds,
  });
}
```

- [ ] **Step 3: 类型检查**

Run: `cd dashboard && pnpm check:type 2>&1 | tail -5`
Expected: 无错误

- [ ] **Step 4: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/types/api/api.d.ts dashboard/apps/web-antdv-next/src/api/tag.ts dashboard/apps/web-antdv-next/src/api/demand.ts
git commit -m "feat(dashboard): 标签类型定义与 API 封装"
```

---

### Task 9: 标签管理页与菜单

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/tags/index.vue`
- Create: `dashboard/apps/web-antdv-next/src/views/tags/components/TagFormDialog.vue`
- Create: `dashboard/apps/web-antdv-next/src/router/routes/modules/tags.ts`

**Interfaces:**
- Consumes: Task 8 的 `fetchTags` / `createTag` / `updateTag` / `deleteTag`
- Produces: 超管可见的「标签管理」菜单与页面

- [ ] **Step 1: 路由模块**

创建 `src/router/routes/modules/tags.ts`：

```ts
import type { RouteRecordRaw } from 'vue-router';

// 一级菜单，仅 admin 可见
const routes: RouteRecordRaw[] = [
  {
    name: 'TagList',
    path: '/tags',
    component: () => import('#/views/tags/index.vue'),
    meta: {
      authority: ['admin'],
      icon: 'ri:price-tag-3-line',
      order: 16,
      title: '标签管理',
    },
  },
];

export default routes;
```

- [ ] **Step 2: 表单弹窗**

创建 `src/views/tags/components/TagFormDialog.vue`：

```vue
<script lang="ts" setup>
import type { FormInstance, FormProps } from 'antdv-next';

import { reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Form, FormItem, Input } from 'antdv-next';

import { createTag, updateTag } from '#/api/tag';
import { showSuccess } from '#/utils/http/error';

/** 标签创建 / 编辑弹窗：仅名称一项，颜色由后端按名称生成并固化，不可指定 */
defineOptions({ name: 'TagFormDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

/** 编辑对象，为空即创建模式 */
const tag = ref<Api.Tag.Item>();
const formRef = ref<FormInstance>();

const form = reactive({
  name: '',
});

const rules: FormProps['rules'] = {
  name: [{ message: '请输入标签名称', required: true, trigger: 'blur' }],
};

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { tag: target } = modalApi.getData<{ tag?: Api.Tag.Item }>();
    tag.value = target;
    form.name = target?.name ?? '';
    formRef.value?.clearValidate();
    modalApi.setState({ title: target ? '编辑标签' : '新建标签' });
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
    const params: Api.Tag.SaveParams = { name: form.name.trim() };
    await (tag.value ? updateTag(tag.value.id, params) : createTag(params));
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
          placeholder="标签名称，唯一；颜色将按名称自动生成"
          show-count
        />
      </FormItem>
    </Form>
  </Modal>
</template>
```

- [ ] **Step 3: 管理页**

创建 `src/views/tags/index.vue`：

```vue
<script lang="ts" setup>
import type { TableColumnsType } from 'antdv-next';

import { onMounted, ref } from 'vue';

import { confirm, Page, useVbenModal } from '@vben/common-ui';

import { Button, Table, Tag } from 'antdv-next';

import { deleteTag, fetchTags } from '#/api/tag';
import { formatDateTime } from '#/utils/clepsydra/date';
import { showSuccess } from '#/utils/http/error';

import TagFormDialog from './components/TagFormDialog.vue';

/** 标签管理，仅超级管理员可见；标签用于区分需求性质，颜色由后端按名称生成并固化 */
defineOptions({ name: 'TagList' });

const list = ref<Api.Tag.Item[]>([]);
const loading = ref(false);

const columns: TableColumnsType<Api.Tag.Item> = [
  { dataIndex: 'id', key: 'id', title: 'ID', width: 72 },
  { key: 'name', minWidth: 220, title: '名称' },
  {
    dataIndex: 'demand_count',
    key: 'demand_count',
    title: '关联需求',
    width: 100,
  },
  { key: 'created_at', title: '创建时间', width: 176 },
  // 「编辑」「删除」两个链接按钮并排，列宽不足会折行把行高撑高
  { key: 'action', title: '操作', width: 160 },
];

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: TagFormDialog,
});

/** 加载标签列表 */
async function load() {
  loading.value = true;
  try {
    list.value = await fetchTags();
  } finally {
    loading.value = false;
  }
}

/** 打开新建弹窗，不带编辑对象即创建模式 */
function openCreate() {
  formModalApi.setData({}).open();
}

function openEdit(row: Api.Tag.Item) {
  formModalApi.setData({ tag: row }).open();
}

/** 删除标签：仅解除与需求的关联，需求本身不受影响 */
async function remove(row: Api.Tag.Item) {
  const suffix =
    row.demand_count > 0
      ? `该标签已关联 ${row.demand_count} 个需求，删除后仅解除关联，不影响需求本身。`
      : '';
  try {
    await confirm(`确定删除标签「${row.name}」吗？${suffix}`, '删除确认');
  } catch {
    // 用户取消
    return;
  }

  await deleteTag(row.id);
  showSuccess('已删除');
  await load();
}

onMounted(load);
</script>

<template>
  <Page>
    <div class="mb-4 flex items-center justify-end">
      <Button type="primary" @click="openCreate">新建标签</Button>
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

- [ ] **Step 4: 类型检查与 lint**

Run: `cd dashboard && pnpm check:type 2>&1 | tail -5 && pnpm lint 2>&1 | tail -5`
Expected: 均无错误

- [ ] **Step 5: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/views/tags dashboard/apps/web-antdv-next/src/router/routes/modules/tags.ts
git commit -m "feat(dashboard): 标签管理页，颜色自动生成不可指定"
```

---

### Task 10: 需求侧集成——表单 / 列表 / 详情

**Files:**
- Create: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandTagsDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/components/DemandFormDialog.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/index.vue`
- Modify: `dashboard/apps/web-antdv-next/src/views/demands/detail.vue`

**Interfaces:**
- Consumes: Task 8 的 `fetchTags` / `updateDemandTags`、`Api.Demand.Item.edges.tags`、`CreateParams.tag_ids`
- Produces: 需求表单标签多选、列表标签列与筛选、详情标签展示与编辑入口

- [ ] **Step 1: 标签编辑弹窗**

创建 `src/views/demands/components/DemandTagsDialog.vue`（仿 `DemandProjectsDialog.vue`）：

```vue
<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Select } from 'antdv-next';

import { updateDemandTags } from '#/api/demand';
import { fetchTags } from '#/api/tag';
import { showSuccess } from '#/utils/http/error';

/**
 * 需求性质标签编辑弹窗
 *
 * 与 DemandFormDialog 的区别：标签不受需求状态锁定，任何状态（含已验收）都可用，
 * 存量已完成需求也能补打标签
 */
defineOptions({ name: 'DemandTagsDialog' });

const emit = defineEmits<{
  /** 保存成功 */
  success: [];
}>();

const demandId = ref(0);
const tagIds = ref<number[]>([]);
const options = ref<{ label: string; value: number }[]>([]);

const [Modal, modalApi] = useVbenModal({
  onConfirm: save,
  async onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    const { demand } = modalApi.getData<{ demand: Api.Demand.Item }>();
    demandId.value = demand.id;
    tagIds.value = (demand.edges?.tags ?? []).map((t) => t.id);
    modalApi.setState({ title: '编辑标签' });

    try {
      const tags = await fetchTags();
      options.value = tags.map((t) => ({ label: t.name, value: t.id }));
    } catch {
      // 错误提示已由请求拦截器统一弹出
    }
  },
});

/** 保存：全量覆盖需求的性质标签 */
async function save() {
  modalApi.lock();
  try {
    await updateDemandTags(demandId.value, tagIds.value);
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
      v-model:value="tagIds"
      :options="options"
      allow-clear
      class="w-full"
      mode="multiple"
      placeholder="选择性质标签（可多选，清空即移除全部标签）"
    />
  </Modal>
</template>
```

- [ ] **Step 2: 需求表单集成**

修改 `src/views/demands/components/DemandFormDialog.vue`：

import 变更：

```ts
import {
  createDemand,
  updateDemand,
  updateDemandProjects,
  updateDemandTags,
} from '#/api/demand';
import { fetchProjects } from '#/api/project';
import { fetchTags } from '#/api/tag';
```

`form` 增加 `tagIds`（`projectIds` 之后）：

```ts
  tagIds: [] as number[],
```

`projectOptions` 之后新增：

```ts
/** 标签多选选项，弹窗每次打开时刷新 */
const tagOptions = ref<{ label: string; value: number }[]>([]);
```

`initialProjectIdsKey` 之后新增：

```ts
/** 弹窗打开时记录的初始性质标签指纹，作用同 initialProjectIdsKey */
const initialTagIdsKey = ref('');
```

现有 `projectIdsKey` 函数改名为通用的 `idsKey`（函数体不变），并更新其两处现有调用：

```ts
/** 将 ID 数组归一化成排序后的指纹串，用于比对是否发生变化 */
function idsKey(ids: number[]) {
  return ids.toSorted((a, b) => a - b).join(',');
}
```

`loadProjectOptions` 之后新增：

```ts
/** 加载标签选项，失败不阻塞表单其余部分 */
async function loadTagOptions() {
  try {
    const tags = await fetchTags();
    tagOptions.value = tags.map((t) => ({
      label: t.name,
      value: t.id,
    }));
  } catch {
    // 错误提示已由请求拦截器统一弹出
  }
}
```

`onOpenChange` 中 `initialProjectIdsKey.value = ...` 之后新增：

```ts
    form.tagIds = (target?.edges?.tags ?? []).map((t) => t.id);
    initialTagIdsKey.value = idsKey(form.tagIds);
    void loadTagOptions();
```

（同步把上一行改为 `initialProjectIdsKey.value = idsKey(form.projectIds);`）

`save` 中编辑分支改为：

```ts
    if (demand.value) {
      await updateDemand(demand.value.id, params);
      // 标签走独立接口，与标题描述的状态锁定解耦；未变化则不调用，避免冗余审计
      if (idsKey(form.projectIds) !== initialProjectIdsKey.value) {
        await updateDemandProjects(demand.value.id, form.projectIds);
      }
      if (idsKey(form.tagIds) !== initialTagIdsKey.value) {
        await updateDemandTags(demand.value.id, form.tagIds);
      }
    } else {
      params.project_ids =
        form.projectIds.length > 0 ? form.projectIds : undefined;
      params.tag_ids = form.tagIds.length > 0 ? form.tagIds : undefined;
      await createDemand(params);
    }
```

模板中「项目」FormItem 之后新增：

```vue
      <FormItem label="标签" name="tagIds">
        <Select
          v-model:value="form.tagIds"
          :options="tagOptions"
          allow-clear
          mode="multiple"
          placeholder="选择性质标签（可多选，可留空）"
        />
      </FormItem>
```

- [ ] **Step 3: 需求列表集成**

修改 `src/views/demands/index.vue`：

import 增加：

```ts
import { fetchTags } from '#/api/tag';
```

`projectOptions` 定义之后新增：

```ts
/** 标签筛选，undefined 表示全部 */
const tagId = ref<number | undefined>(undefined);
const tagOptions = ref<{ label: string; value: number }[]>([]);
```

`columns` 中 `{ key: 'projects', title: '项目', width: 180 }` 之后新增：

```ts
  { key: 'tags', title: '标签', width: 160 },
```

`load` 中 `fetchDemands` 调用加参数：

```ts
    list.value = await fetchDemands({
      project_id: projectId.value,
      status: status.value,
      tag_id: tagId.value,
    });
```

`onMounted` 中 `fetchProjects()...` 之后新增：

```ts
  fetchTags()
    .then((tags) => {
      tagOptions.value = tags.map((t) => ({
        label: t.name,
        value: t.id,
      }));
    })
    .catch(() => {
      // 错误提示已由请求拦截器统一弹出
    });
```

模板筛选区项目 Select 之后新增：

```vue
        <Select
          v-model:value="tagId"
          :options="tagOptions"
          allow-clear
          class="w-45"
          placeholder="全部标签"
          @change="load"
        />
```

`bodyCell` 中 `projects` 模板之后新增：

```vue
        <template v-else-if="column.key === 'tags'">
          <div class="flex flex-wrap items-center gap-2">
            <Tag
              v-for="tg in record.edges?.tags ?? []"
              :key="tg.id"
              :color="tg.color || undefined"
              class="me-0"
            >
              {{ tg.name }}
            </Tag>
          </div>
        </template>
```

- [ ] **Step 4: 需求详情集成**

修改 `src/views/demands/detail.vue`：

import 增加（`DemandProjectsDialog` import 之后）：

```ts
import DemandTagsDialog from './components/DemandTagsDialog.vue';
```

`ProjectsModal` 定义之后新增：

```ts
const [TagsModal, tagsModalApi] = useVbenModal({
  connectedComponent: DemandTagsDialog,
});
```

模板中「项目」`DescriptionsItem` 之后新增：

```vue
            <DescriptionsItem :span="2" label="标签">
              <div class="flex flex-wrap items-center gap-2">
                <Tag
                  v-for="tg in demand.edges?.tags ?? []"
                  :key="tg.id"
                  :color="tg.color || undefined"
                  class="me-0"
                >
                  {{ tg.name }}
                </Tag>
                <Button
                  size="small"
                  type="link"
                  @click="tagsModalApi.setData({ demand }).open()"
                >
                  编辑标签
                </Button>
              </div>
            </DescriptionsItem>
```

模板中 `<ProjectsModal @success="load" />` 同级位置新增（搜索 `ProjectsModal` 的模板用法，紧随其后）：

```vue
    <TagsModal @success="load" />
```

- [ ] **Step 5: 类型检查、lint 与单测**

Run: `cd dashboard && pnpm check:type 2>&1 | tail -5 && pnpm lint 2>&1 | tail -5 && pnpm test:unit 2>&1 | tail -5`
Expected: 均无错误

- [ ] **Step 6: 浏览器验证**

启动前后端（后端 `make run` 或既有 launch 配置，前端 dev server），用浏览器工具验证：

1. 超管登录，「标签管理」菜单可见，新建标签「优化」→ 列表出现带自动生成颜色的 Tag
2. 编辑改名 → 颜色不变
3. 需求新建表单出现「标签」多选；选中保存后列表标签列展示彩色 Tag
4. 需求列表按标签筛选生效
5. 需求详情展示标签，「编辑标签」弹窗可改
6. 截图给用户确认

- [ ] **Step 7: Commit**

```bash
git add dashboard/apps/web-antdv-next/src/views/demands dashboard/apps/web-antdv-next/src/api dashboard/apps/web-antdv-next/src/types
git commit -m "feat(dashboard): 需求表单、列表与详情集成性质标签"
```
