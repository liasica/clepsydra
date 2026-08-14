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

	"clepsydra/internal/api"
	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// newMandayEnv 构建人天调整 handler 测试环境，返回 client、service 与 handler
func newMandayEnv(t *testing.T, name string) (*ent.Client, *service.Demand, *Demand) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	svc := service.NewDemand(client, service.NewSetting(client), service.NewAudit(client))

	return client, svc, NewDemand(svc)
}

// callManday 以指定角色与请求体调用 UpdateHalfDays handler
func callManday(t *testing.T, h *Demand, id int, role, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/demands/"+strconv.Itoa(id)+"/half-days",
		strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(id))
	c.Set("claims", &service.Claims{UserID: 1, Role: role, Name: role})
	if err := h.UpdateHalfDays(c); err != nil {
		t.Fatalf("UpdateHalfDays handler 错误: %v", err)
	}

	return rec
}

// TestDemandUpdateHalfDaysHandler 覆盖 200 / 400 / 422 语义
func TestDemandUpdateHalfDaysHandler(t *testing.T) {
	client, svc, h := newMandayEnv(t, "hmanday")
	ctx := context.Background()

	adminActor := service.Actor{ID: 1, Name: "超级管理员"}
	d, _ := svc.Create(ctx, adminActor, "需求", "", 4, nil, false, nil, nil, "")

	// 任意状态改预估 200
	rec := callManday(t, h, d.ID, "admin", `{"estimated_half_days":6}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"estimated_half_days":6`) {
		t.Errorf("改预估应 200 且返回新值, got %d: %s", rec.Code, rec.Body.String())
	}

	// 未完成状态改实际人天 422
	rec = callManday(t, h, d.ID, "admin", `{"actual_half_days":4}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("未完成改实际人天应 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// 完成后改实际人天 200
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").SetActualHalfDays(4).ExecX(ctx)
	rec = callManday(t, h, d.ID, "admin", `{"actual_half_days":8}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"actual_half_days":8`) {
		t.Errorf("完成后改实际人天应 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 两字段全缺 400
	rec = callManday(t, h, d.ID, "admin", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空请求体应 400, got %d", rec.Code)
	}
}

// TestDemandUpdateHalfDaysForbiddenForClient 非超管经 RequireAdmin 中间件拦截返回 403
func TestDemandUpdateHalfDaysForbiddenForClient(t *testing.T) {
	_, _, h := newMandayEnv(t, "hmandayperm")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/demands/1/half-days",
		strings.NewReader(`{"estimated_half_days":6}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})

	// 非超管经 RequireAdmin 包装后应被直接拦截，不进入业务逻辑
	if err := api.RequireAdmin(h.UpdateHalfDays)(c); err == nil {
		t.Error("非超管调整人天应被拒绝")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("非超管调整人天应 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDemandMandayHistoryHandler 历史查询登录即可，返回调整记录
func TestDemandMandayHistoryHandler(t *testing.T) {
	_, svc, h := newMandayEnv(t, "hmandayhist")
	ctx := context.Background()

	adminActor := service.Actor{ID: 1, Name: "超级管理员"}
	d, _ := svc.Create(ctx, adminActor, "需求", "", 4, nil, false, nil, nil, "")
	est := 6
	_, _ = svc.UpdateHalfDays(ctx, adminActor, d.ID, service.DemandHalfDaysPatch{EstimatedHalfDays: &est})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/demands/"+strconv.Itoa(d.ID)+"/manday-history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(d.ID))
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.MandayHistory(c); err != nil {
		t.Fatalf("MandayHistory handler 错误: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "demand.update_half_days") {
		t.Errorf("历史查询应 200 且含调整记录, got %d: %s", rec.Code, rec.Body.String())
	}
}
