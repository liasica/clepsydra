package service

import "context"

// Notifier 通知接口，本期无外部通知渠道，预留给后续邮件或微信实现
// 接入点：需求自动确认、账单分享与自动确认后各调用一次对应方法
type Notifier interface {
	// DemandAccepted 需求被确认（含自动确认）后通知
	DemandAccepted(ctx context.Context, demandID int, auto bool)
	// BillShared 账单分享后通知需求方
	BillShared(ctx context.Context, billID int)
	// BillConfirmed 账单被确认（含自动确认）后通知
	BillConfirmed(ctx context.Context, billID int, auto bool)
}

// NopNotifier 空通知实现
type NopNotifier struct{}

func (NopNotifier) DemandAccepted(ctx context.Context, demandID int, auto bool) {}
func (NopNotifier) BillShared(ctx context.Context, billID int)                  {}
func (NopNotifier) BillConfirmed(ctx context.Context, billID int, auto bool)    {}
