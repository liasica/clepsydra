package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestDashboardTodosHandler 覆盖工作台待办接口，验证响应字段与 service 层结果一致
func TestDashboardTodosHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdash?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)

	act := service.Actor{ID: 1, Name: "管理员"}
	d, err := demandSvc.Create(ctx, act, "待确认", "", 0, nil, false, nil, "")
	if err != nil {
		t.Fatalf("创建需求失败: %v", err)
	}
	if err = demandSvc.SubmitEstimate(ctx, act, d.ID, 2, nil); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}

	svc := service.NewDashboard(client, settingSvc)
	h := NewDashboard(svc)
	e := echo.New()

	c, rec := newDemandTestContext(e, http.MethodGet, "/api/dashboard/todos", "")
	if err = h.Todos(c); err != nil {
		t.Fatalf("Todos 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Todos 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, field := range []string{
		`"pending_estimate_count":1`,
		`"pending_acceptance_count":0`,
		`"pending_bill_count":0`,
		`"billing_due_date":"`,
		`"billing_due_today":`,
		`"prev_bill_generated":false`,
	} {
		if !strings.Contains(body, field) {
			t.Errorf("响应缺少字段 %s: %s", field, body)
		}
	}
}
