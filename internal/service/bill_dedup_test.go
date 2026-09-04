package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/hook"
)

// assertDemandConflict 断言错误为「该需求已在其他账单中」的友好业务报错
func assertDemandConflict(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("跨账单重复入账应被数据库唯一索引拒绝")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) || svcErr.Code != 40000 || !strings.Contains(svcErr.Message, "该需求已在其他账单中") {
		t.Fatalf("冲突错误应转换为友好业务报错, got %v", err)
	}
}

// TestBillItemUniqueAcrossBills 验证防重的数据库不变量：
// service 层预检查（billedDemandIDs）存在并发窗口，绕过预检查直接落库时，
// bill_items(demand_id) 唯一索引兜底拒绝同一需求进入第二张账单
func TestBillItemUniqueAcrossBills(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bdedup")
	ctx := context.Background()

	// 需求已在账单 A 中
	demandID := prepareAccepted(t, demandSvc, "已计费需求", 2)
	_, err := billSvc.CreateManual(ctx, admin, "账单A", []int{demandID})
	if err != nil {
		t.Fatalf("生成账单 A 失败: %v", err)
	}

	// 账单 B 合法创建后，模拟并发竞争者绕过预检查直插同一需求的明细
	var billB *ent.Bill
	billB, err = billSvc.CreateManual(ctx, admin, "账单B", []int{prepareAccepted(t, demandSvc, "占位需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 B 失败: %v", err)
	}

	target := client.Demand.GetX(ctx, demandID)
	assertDemandConflict(t, billSvc.createItem(ctx, billB, target, 2, 1200))
}

// TestBillItemAcrossBillsRejected 同一需求已在一张账单里时，加入另一张账单被拒绝
func TestBillItemAcrossBillsRejected(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bdedupacross")
	ctx := context.Background()

	// 进行中需求，账单 A 已包含它
	d, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID, 4, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = demandSvc.Start(ctx, admin, d.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	_, err := billSvc.CreateManual(ctx, admin, "账单A", []int{d.ID})
	if err != nil {
		t.Fatalf("生成账单 A 失败: %v", err)
	}
	var billB *ent.Bill
	billB, err = billSvc.CreateManual(ctx, admin, "账单B", []int{prepareAccepted(t, demandSvc, "B 占位需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 B 失败: %v", err)
	}

	if err = billSvc.AddItem(ctx, admin, billB.ID, d.ID); err == nil {
		t.Error("已在其他账单中的需求应拒绝加入")
	}
}

// TestBillAddItemConflictOnRace 复现 check-then-act 并发窗口：
// AddItem 预检查通过后、明细落库前，并发竞争者将同一需求计入另一账单，
// 落库时应命中唯一索引并返回友好业务报错
func TestBillAddItemConflictOnRace(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bdeduprace")
	ctx := context.Background()

	demandID := prepareAccepted(t, demandSvc, "竞争需求", 2)
	billA, err := billSvc.CreateManual(ctx, admin, "账单A", []int{prepareAccepted(t, demandSvc, "A 占位需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 A 失败: %v", err)
	}
	var billB *ent.Bill
	billB, err = billSvc.CreateManual(ctx, admin, "账单B", []int{prepareAccepted(t, demandSvc, "B 占位需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 B 失败: %v", err)
	}

	// 用 mutation 钩子在 AddItem 的明细插入执行前注入竞争写：账单 A 先一步计费同一需求
	injected := false
	client.BillItem.Use(func(next ent.Mutator) ent.Mutator {
		return hook.BillItemFunc(func(ctx context.Context, m *ent.BillItemMutation) (ent.Value, error) {
			if m.Op().Is(ent.OpCreate) && !injected {
				injected = true
				target := client.Demand.GetX(ctx, demandID)
				if cerr := billSvc.createItem(ctx, billA, target, 2, 1200); cerr != nil {
					t.Fatalf("竞争明细落库失败: %v", cerr)
				}
			}

			return next.Mutate(ctx, m)
		})
	})

	assertDemandConflict(t, billSvc.AddItem(ctx, admin, billB.ID, demandID))
}
