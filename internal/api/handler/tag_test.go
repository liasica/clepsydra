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
