package task

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

var admin = service.Actor{ID: 1, Name: "超级管理员"}
var clientActor = service.Actor{ID: 2, Name: "甲方"}

// newEnv 构建定时任务测试环境
func newEnv(t *testing.T, name string) (*ent.Client, *service.Demand, *service.Bill, *Runner) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	runner := New(client, settingSvc, demandSvc, billSvc, nil, zerolog.Nop())

	return client, demandSvc, billSvc, runner
}

// finishDemand 造一个完成待确认的需求
func finishDemand(t *testing.T, svc *service.Demand, title string) int {
	t.Helper()

	ctx := context.Background()
	d, _ := svc.Create(ctx, admin, title, "", 4, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now().AddDate(0, 0, -10))
	if err := svc.Finish(ctx, admin, d.ID, time.Now().AddDate(0, 0, -10), time.Now().AddDate(0, 0, -8), 4); err != nil {
		t.Fatalf("完成需求失败: %v", err)
	}

	return d.ID
}

func TestScanExpired(t *testing.T) {
	_, demandSvc, _, runner := newEnv(t, "scan")
	ctx := context.Background()

	id := finishDemand(t, demandSvc, "过期需求")

	// 未过期：扫描不动它（deadline 是 now + 5 天）
	if err := runner.ScanExpired(ctx, time.Now()); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	d, _ := demandSvc.Get(ctx, id)
	if d.Status.String() != "pending_acceptance" {
		t.Fatalf("未过期需求不应被确认, status = %s", d.Status)
	}

	// 模拟 6 天后：应被自动确认
	if err := runner.ScanExpired(ctx, time.Now().AddDate(0, 0, 6)); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	d, _ = demandSvc.Get(ctx, id)
	if d.Status.String() != "accepted" || !d.AcceptAuto || d.AcceptLocked {
		t.Errorf("过期需求应被自动确认: status=%s auto=%v locked=%v", d.Status, d.AcceptAuto, d.AcceptLocked)
	}

	// 幂等：重复扫描无副作用
	if err := runner.ScanExpired(ctx, time.Now().AddDate(0, 0, 7)); err != nil {
		t.Errorf("重复扫描应无副作用: %v", err)
	}
}

func TestEnsurePrevBill(t *testing.T) {
	client, _, _, runner := newEnv(t, "ensure")
	ctx := context.Background()

	now := time.Date(2026, 8, 4, 0, 15, 0, 0, time.Local)

	// 首次执行生成上月账单
	if err := runner.EnsurePrevBill(ctx, now); err != nil {
		t.Fatalf("补生成失败: %v", err)
	}
	b := client.Bill.Query().Where(bill.Period("2026-07")).OnlyX(ctx)
	if b.Status.String() != "draft" {
		t.Errorf("生成的账单应为草稿, got %s", b.Status)
	}

	// 幂等：已存在则跳过，不重建
	if err := runner.EnsurePrevBill(ctx, now); err != nil {
		t.Fatalf("重复执行失败: %v", err)
	}
	if n := client.Bill.Query().CountX(ctx); n != 1 {
		t.Errorf("账单数量 = %d, want 1", n)
	}
}
