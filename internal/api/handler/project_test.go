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
