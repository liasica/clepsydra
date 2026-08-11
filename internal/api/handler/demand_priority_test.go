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

// TestDemandPriorityHandler 覆盖创建带优先级、独立改优先级接口与非法值 400
func TestDemandPriorityHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdprio?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建带优先级（需求方也可指定）
	req := httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(`{"title":"需求一","priority":"urgent"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.Create(c); err != nil {
		t.Fatalf("创建错误: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"priority":"urgent"`) {
		t.Fatalf("创建响应应携带优先级: %s", rec.Body.String())
	}

	rows, _ := svc.List(ctx, "", 0, 0, "")
	id := strconv.Itoa(rows[0].ID)

	// 独立接口调整优先级（需求方也可操作）
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/priority", strings.NewReader(`{"priority":"low"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdatePriority(c); err != nil {
		t.Fatalf("改优先级错误: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"priority":"low"`) {
		t.Errorf("响应应携带更新后的优先级: %s", rec.Body.String())
	}

	// 非法优先级返回 400
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/priority", strings.NewReader(`{"priority":"p0"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdatePriority(c); err != nil {
		t.Fatalf("handler 不应返回原始错误: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法优先级应返回 400, got %d, body = %s", rec.Code, rec.Body.String())
	}

	// 列表按优先级筛选参数透传
	req = httptest.NewRequest(http.MethodGet, "/api/demands?priority=urgent", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatalf("列表错误: %v", err)
	}
	if strings.Contains(rec.Body.String(), "需求一") {
		t.Errorf("优先级已改为 low，按 urgent 筛选不应包含需求一: %s", rec.Body.String())
	}
}
