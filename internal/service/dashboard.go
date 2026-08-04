package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/workday"
)

// Todos 工作台待办汇总
type Todos struct {
	PendingEstimateCount   int    `json:"pending_estimate_count"`
	PendingAcceptanceCount int    `json:"pending_acceptance_count"`
	PendingBillCount       int    `json:"pending_bill_count"`
	BillingDueDate         string `json:"billing_due_date"`
	BillingDueToday        bool   `json:"billing_due_today"`
	PrevBillShared         bool   `json:"prev_bill_shared"`
}

// Dashboard 工作台服务
type Dashboard struct {
	client  *ent.Client
	setting *Setting
}

// NewDashboard 构建工作台服务
func NewDashboard(client *ent.Client, setting *Setting) *Dashboard {
	return &Dashboard{client: client, setting: setting}
}

// Todos 汇总待办信息
func (s *Dashboard) Todos(ctx context.Context, role string, now time.Time) (*Todos, error) {
	todos := new(Todos)

	// 各状态计数
	var err error
	if todos.PendingEstimateCount, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusPendingEstimate)).Count(ctx); err != nil {
		return nil, err
	}
	if todos.PendingAcceptanceCount, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusPendingAcceptance)).Count(ctx); err != nil {
		return nil, err
	}
	if todos.PendingBillCount, err = s.client.Bill.Query().
		Where(bill.StatusEQ(bill.StatusPending)).Count(ctx); err != nil {
		return nil, err
	}

	// 出账截止日与上月账单状态
	var cal *workday.Calendar
	cal, err = s.setting.Calendar(ctx)
	if err != nil {
		return nil, err
	}
	due := cal.BillingDueDate(now.Year(), now.Month())
	todos.BillingDueDate = due.Format("2006-01-02")
	todos.BillingDueToday = due.Format("2006-01-02") == now.Format("2006-01-02")

	var prev *ent.Bill
	prev, err = s.client.Bill.Query().Where(bill.Period(PrevPeriod(now))).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	todos.PrevBillShared = prev != nil && prev.Status != bill.StatusDraft

	return todos, nil
}
