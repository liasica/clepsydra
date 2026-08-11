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

// assertBillableConflict 断言错误为「该需求已被其他账单计费」的友好业务报错
func assertBillableConflict(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("跨账单重复计费应被数据库唯一索引拒绝")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) || svcErr.Code != 40000 || !strings.Contains(svcErr.Message, "该需求已被其他账单计费") {
		t.Fatalf("冲突错误应转换为友好业务报错, got %v", err)
	}
}

// TestBillItemBillableUniqueAcrossBills 验证计费防重的数据库不变量：
// service 层预检查（billedDemandIDs）存在并发窗口，绕过预检查直接落库时，
// bill_items(demand_id) WHERE billable 部分唯一索引兜底拒绝跨账单重复计费，
// 该路径同时覆盖 Generate 与 CreateManual 共用的 createItem
func TestBillItemBillableUniqueAcrossBills(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bdedup")
	ctx := context.Background()

	// 需求已被账单 A 计费
	demandID := prepareAccepted(t, demandSvc, "已计费需求", 2)
	_, err := billSvc.CreateManual(ctx, admin, "账单A", []int{demandID})
	if err != nil {
		t.Fatalf("生成账单 A 失败: %v", err)
	}

	// 账单 B 合法创建后，模拟并发竞争者绕过预检查直插同一需求的计费行
	var billB *ent.Bill
	billB, err = billSvc.CreateManual(ctx, admin, "账单B", []int{prepareAccepted(t, demandSvc, "占位需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 B 失败: %v", err)
	}

	target := client.Demand.GetX(ctx, demandID)
	assertBillableConflict(t, billSvc.createItem(ctx, billB, target, 2, 1200, true))
}

// TestBillItemDisplayRowsNotLimited 展示行不受计费唯一索引限制，同一需求可同时在多张账单展示
func TestBillItemDisplayRowsNotLimited(t *testing.T) {
	_, demandSvc, billSvc := newBillEnv(t, "bdedupdisplay")
	ctx := context.Background()

	// 进行中需求作为展示行
	d, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 0, nil, false, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID, 4, nil)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = demandSvc.Start(ctx, admin, d.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	billA, err := billSvc.CreateManual(ctx, admin, "账单A", []int{prepareAccepted(t, demandSvc, "A 计费需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 A 失败: %v", err)
	}
	var billB *ent.Bill
	billB, err = billSvc.CreateManual(ctx, admin, "账单B", []int{prepareAccepted(t, demandSvc, "B 计费需求", 2)})
	if err != nil {
		t.Fatalf("生成账单 B 失败: %v", err)
	}

	// 同一需求作为展示行加入两张账单均应成功
	if err = billSvc.AddItem(ctx, admin, billA.ID, d.ID); err != nil {
		t.Fatalf("展示行加入账单 A 失败: %v", err)
	}
	if err = billSvc.AddItem(ctx, admin, billB.ID, d.ID); err != nil {
		t.Errorf("展示行加入账单 B 应成功: %v", err)
	}
}

// TestBillAddItemBillableConflictOnRace 复现 check-then-act 并发窗口：
// AddItem 预检查通过后、明细落库前，并发竞争者将同一需求计入另一账单，
// 落库时应命中部分唯一索引并返回友好业务报错
func TestBillAddItemBillableConflictOnRace(t *testing.T) {
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
				if cerr := billSvc.createItem(ctx, billA, target, 2, 1200, true); cerr != nil {
					t.Fatalf("竞争计费行落库失败: %v", cerr)
				}
			}

			return next.Mutate(ctx, m)
		})
	})

	assertBillableConflict(t, billSvc.AddItem(ctx, admin, billB.ID, demandID))
}
