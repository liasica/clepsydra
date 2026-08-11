package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
)

func TestDashboardTodos(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:dash?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := NewSetting(client)
	audit := NewAudit(client)
	demandSvc := NewDemand(client, settingSvc, audit)

	// 一个待确认人天的需求
	d, _ := demandSvc.Create(ctx, admin, "待确认", "", 0, nil, false, nil, nil)
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID, 2, nil)

	svc := NewDashboard(client, settingSvc)
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.Local)

	todos, err := svc.Todos(ctx, "admin", now)
	if err != nil {
		t.Fatalf("查询待办失败: %v", err)
	}

	if todos.PendingEstimateCount != 1 {
		t.Errorf("待确认人天数 = %d, want 1", todos.PendingEstimateCount)
	}
	if todos.PrevBillGenerated {
		t.Error("上月账单未生成，PrevBillGenerated 应为 false")
	}
	if todos.BillingDueDate == "" {
		t.Error("出账截止日不应为空")
	}
}
