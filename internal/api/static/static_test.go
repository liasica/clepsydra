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
