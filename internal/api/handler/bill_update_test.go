package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// newBillUpdateEnv 构建账单编辑接口测试环境，返回一张含单条明细的账单
func newBillUpdateEnv(t *testing.T, name string) (*ent.Client, *Bill, *service.Bill, *ent.Bill) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)

	act := service.Actor{ID: 1, Name: "管理员"}
	d, _ := demandSvc.Create(ctx, act, "结算需求", "", 0, nil, false, nil, nil, "")
	_ = demandSvc.SubmitEstimate(ctx, act, d.ID, 4, nil)
	_ = demandSvc.ConfirmEstimate(ctx, act, d.ID)
	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	_ = demandSvc.Start(ctx, act, d.ID, start)
	_ = demandSvc.Finish(ctx, act, d.ID, start, end, 4)
	_ = demandSvc.Accept(ctx, act, d.ID, false)

	b, err := billSvc.CreateManual(ctx, act, "结算单", []int{d.ID})
	if err != nil {
		t.Fatalf("手动生成账单失败: %v", err)
	}

	return client, NewBill(billSvc), billSvc, b
}

func TestBillUpdateHandler(t *testing.T) {
	_, h, billSvc, b := newBillUpdateEnv(t, "hbupd")
	ctx := context.Background()
	e := echo.New()
	billIDStr := strconv.Itoa(b.ID)

	// 编辑名称与基础费
	c, rec := newDemandTestContext(e, http.MethodPatch, "/api/bills/"+billIDStr,
		`{"name":"八月结算单","base_fee":500}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.Update(c); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Update 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	got, _ := billSvc.Get(ctx, b.ID)
	if got.Name != "八月结算单" || got.BaseFee != 500 {
		t.Errorf("更新后 name=%s baseFee=%d, want 八月结算单 / 500", got.Name, got.BaseFee)
	}

	// 截止时间格式非法拒绝
	c, rec = newDemandTestContext(e, http.MethodPatch, "/api/bills/"+billIDStr,
		`{"confirm_deadline":"abc"}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	_ = h.Update(c)
	if rec.Code == http.StatusOK {
		t.Errorf("非法截止时间应拒绝: %s", rec.Body.String())
	}
}

func TestBillUpdateItemHandler(t *testing.T) {
	client, h, billSvc, b := newBillUpdateEnv(t, "hbupditem")
	ctx := context.Background()
	e := echo.New()

	item := client.BillItem.Query().Where(billitem.HasBillWith()).FirstX(ctx)
	c, rec := newDemandTestContext(e, http.MethodPatch,
		"/api/bills/"+strconv.Itoa(b.ID)+"/items/"+strconv.Itoa(item.ID),
		`{"half_days":6,"note":"补录说明"}`)
	c.SetParamNames("id", "itemId")
	c.SetParamValues(strconv.Itoa(b.ID), strconv.Itoa(item.ID))
	if err := h.UpdateItem(c); err != nil {
		t.Fatalf("UpdateItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	got := client.BillItem.GetX(ctx, item.ID)
	if got.HalfDays != 6 || got.Note != "补录说明" || got.Amount != 3600 {
		t.Errorf("更新后 halfDays=%d note=%q amount=%d, want 6 / 补录说明 / 3600", got.HalfDays, got.Note, got.Amount)
	}
	bill2, _ := billSvc.Get(ctx, b.ID)
	if bill2.TotalAmount != 3600 {
		t.Errorf("合计 = %d, want 3600", bill2.TotalAmount)
	}
}
