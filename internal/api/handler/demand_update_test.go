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

// putDemand 以指定角色调用 PUT /api/demands/:id，返回响应记录器
func putDemand(t *testing.T, h *Demand, id int, role string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/demands/"+strconv.Itoa(id),
		strings.NewReader(`{"title":"修改后标题","description":"修改后描述"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(id))
	c.Set("claims", &service.Claims{UserID: 1, Role: role, Name: role})
	if err := h.Update(c); err != nil {
		t.Fatalf("Update handler 错误: %v", err)
	}

	return rec
}

// TestDemandUpdateRoleGate 锁定状态下超管可编辑标题描述，需求方仍被状态锁拒绝
func TestDemandUpdateRoleGate(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdupdate?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)

	admin := service.Actor{ID: 1, Name: "超级管理员"}
	d, _ := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, nil, "")
	// 直接改库到锁定状态，绕开完整流转
	client.Demand.UpdateOneID(d.ID).SetStatus("confirmed").ExecX(ctx)

	// 需求方被状态锁拒绝，保持 422 语义
	rec := putDemand(t, h, d.ID, "client")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("需求方更新锁定需求应 422, got %d, body = %s", rec.Code, rec.Body.String())
	}

	// 超管任何状态可编辑
	rec = putDemand(t, h, d.ID, "admin")
	if rec.Code != http.StatusOK {
		t.Errorf("超管更新锁定需求应 200, got %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "修改后标题") {
		t.Errorf("响应应携带更新后的标题: %s", rec.Body.String())
	}
}
