package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
)

// Todos 工作台待办汇总
type Todos struct {
	PendingEstimateCount   int `json:"pending_estimate_count"`
	PendingAcceptanceCount int `json:"pending_acceptance_count"`
	PendingBillCount       int `json:"pending_bill_count"`
}

// Dashboard 工作台服务
type Dashboard struct {
	client *ent.Client
}

// NewDashboard 构建工作台服务
func NewDashboard(client *ent.Client) *Dashboard {
	return &Dashboard{client: client}
}

// Todos 汇总待办信息
func (s *Dashboard) Todos(ctx context.Context) (*Todos, error) {
	todos := new(Todos)

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

	return todos, nil
}
