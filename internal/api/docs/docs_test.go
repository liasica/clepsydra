package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

// expectedRouteCount 与 router.go 的业务路由数量保持一致：1 条公开 login + 9 条登录组（含 auth/me）+ 24 条 admin 组，
// docs 自身的两条路由不计入 spec，因此不计入此数
const expectedRouteCount = 34

// httpMethods 用于从 path item 中筛出真正的操作，排除 parameters 等非方法字段
var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "delete": true,
	"patch": true, "head": true, "options": true, "trace": true,
}

// openAPIDoc 仅声明测试断言需要的字段，其余字段解析时按 any 忽略
type openAPIDoc struct {
	OpenAPI string                    `yaml:"openapi"`
	Tags    []struct{ Name string }   `yaml:"tags"`
	Paths   map[string]map[string]any `yaml:"paths"`
}

// loadDoc 解析 embed 的 spec，供多个测试复用
func loadDoc(t *testing.T) *openAPIDoc {
	t.Helper()

	doc := new(openAPIDoc)
	if err := yaml.Unmarshal(openAPISpec, doc); err != nil {
		t.Fatalf("openapi.yaml 解析失败: %v", err)
	}

	return doc
}

// TestRegisterDocsServesScalarPage 校验 /docs 返回 200 且包含 scalar 官方集成的 script 标签
func TestRegisterDocsServesScalarPage(t *testing.T) {
	e := echo.New()
	RegisterDocs(e)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="api-reference"`) {
		t.Errorf("响应体缺少 api-reference script 标签: %s", body)
	}
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Errorf("响应体缺少 scalar 渲染脚本引用: %s", body)
	}
}

// TestRegisterDocsServesOpenAPISpec 校验 /docs/openapi.yaml 返回 200 且能被 yaml.v3 正确解析
func TestRegisterDocsServesOpenAPISpec(t *testing.T) {
	e := echo.New()
	RegisterDocs(e)

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	contentType := rec.Header().Get(echo.HeaderContentType)
	if !strings.Contains(contentType, "yaml") {
		t.Errorf("期望 Content-Type 含 yaml，实际 %s", contentType)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi.yaml 解析失败: %v", err)
	}
	if doc["openapi"] == nil {
		t.Fatal("解析结果缺少 openapi 版本字段")
	}
}

// TestOpenAPIPathsCountMatchesRouter 是防止未来加路由忘更新文档的守护断言：
// spec 中登记的操作数量必须与 router.go 实际注册的业务路由数量完全一致
func TestOpenAPIPathsCountMatchesRouter(t *testing.T) {
	doc := loadDoc(t)

	count := 0
	for _, item := range doc.Paths {
		for method := range item {
			if httpMethods[method] {
				count++
			}
		}
	}

	if count != expectedRouteCount {
		t.Fatalf("spec 路由数 %d 与 router.go 路由数 %d 不一致", count, expectedRouteCount)
	}
}

// TestOpenAPITagsCoverEightModules 校验 tags 集合恰为用户要求的 8 个模块
func TestOpenAPITagsCoverEightModules(t *testing.T) {
	doc := loadDoc(t)

	want := map[string]bool{
		"Auth": true, "Users": true, "Demands": true, "Bills": true,
		"Settings": true, "Holidays": true, "AuditLogs": true, "Dashboard": true,
	}

	got := make(map[string]bool, len(doc.Tags))
	for _, tag := range doc.Tags {
		got[tag.Name] = true
	}

	if len(got) != len(want) {
		t.Fatalf("tag 数量应为 %d 个，实际 %d 个：%v", len(want), len(got), got)
	}
	for name := range want {
		if !got[name] {
			t.Errorf("缺少 tag: %s", name)
		}
	}
}

// TestOpenAPIEveryOperationHasTag 校验每条路由都归属了模块 tag，不遗漏未分类的接口
func TestOpenAPIEveryOperationHasTag(t *testing.T) {
	doc := loadDoc(t)

	for path, item := range doc.Paths {
		for method, raw := range item {
			if !httpMethods[method] {
				continue
			}

			op, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("%s %s 操作结构解析失败", strings.ToUpper(method), path)
			}

			tags, _ := op["tags"].([]any)
			if len(tags) == 0 {
				t.Errorf("%s %s 缺少 tags", strings.ToUpper(method), path)
			}
		}
	}
}

// TestOpenAPILoginHasNoSecurityOthersDo 校验 login 接口无 security（公开访问），其余接口均声明 bearerAuth
func TestOpenAPILoginHasNoSecurityOthersDo(t *testing.T) {
	doc := loadDoc(t)

	for path, item := range doc.Paths {
		for method, raw := range item {
			if !httpMethods[method] {
				continue
			}

			op, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("%s %s 操作结构解析失败", strings.ToUpper(method), path)
			}

			security, hasSecurity := op["security"]
			isLogin := path == "/api/auth/login"

			if isLogin && hasSecurity && len(security.([]any)) > 0 {
				t.Errorf("login 接口不应声明 security")
			}
			if !isLogin {
				list, _ := security.([]any)
				if !hasSecurity || len(list) == 0 {
					t.Errorf("%s %s 缺少 security 声明", strings.ToUpper(method), path)
				}
			}
		}
	}
}
