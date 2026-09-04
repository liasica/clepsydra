package task

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/service"
)

// Runner 定时任务运行器，所有任务均为幂等扫描式
type Runner struct {
	client  *ent.Client
	demand  *service.Demand
	bill    *service.Bill
	rotator *lumberjack.Logger
	log     zerolog.Logger
	cron    *cron.Cron
}

// New 构建定时任务运行器，rotator 为 nil 时跳过日志轮转任务
func New(
	client *ent.Client,
	demandSvc *service.Demand,
	billSvc *service.Bill,
	rotator *lumberjack.Logger,
	log zerolog.Logger,
) *Runner {
	return &Runner{
		client:  client,
		demand:  demandSvc,
		bill:    billSvc,
		rotator: rotator,
		log:     log,
		cron:    cron.New(),
	}
}

// Start 注册并启动全部定时任务
func (r *Runner) Start() {
	// 每日零点切割日志文件
	if r.rotator != nil {
		_, _ = r.cron.AddFunc("0 0 * * *", func() {
			if err := r.rotator.Rotate(); err != nil {
				r.log.Error().Err(err).Msg("日志轮转失败")
			}
		})
	}

	// 每日 00:05 扫描过期未确认
	_, _ = r.cron.AddFunc("5 0 * * *", func() {
		if err := r.ScanExpired(context.Background(), time.Now()); err != nil {
			r.log.Error().Err(err).Msg("自动确认扫描失败")
		}
	})

	r.cron.Start()
}

// Stop 停止定时任务
func (r *Runner) Stop() {
	r.cron.Stop()
}

// ScanExpired 自动确认所有过期未确认的需求与账单
func (r *Runner) ScanExpired(ctx context.Context, now time.Time) error {
	// 过期需求自动确认
	demands, err := r.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusPendingAcceptance),
		demand.AcceptDeadlineLT(now),
	).All(ctx)
	if err != nil {
		return err
	}
	for _, d := range demands {
		if err = r.demand.Accept(ctx, service.SystemActor, d.ID, true); err != nil {
			return err
		}
		r.log.Info().Int("demand_id", d.ID).Msg("需求逾期自动确认")
	}

	// 过期账单自动确认
	bills, err := r.client.Bill.Query().Where(
		bill.StatusEQ(bill.StatusPending),
		bill.ConfirmDeadlineLT(now),
	).All(ctx)
	if err != nil {
		return err
	}
	for _, b := range bills {
		if err = r.bill.Confirm(ctx, service.SystemActor, b.ID, true); err != nil {
			return err
		}
		r.log.Info().Int("bill_id", b.ID).Str("name", b.Name).Msg("账单逾期自动确认")
	}

	return nil
}
