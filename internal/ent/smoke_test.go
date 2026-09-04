package ent_test

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

func TestSchemaSmoke(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()

	// 创建需求并流转状态，验证枚举与默认值
	d := client.Demand.Create().
		SetTitle("测试需求").
		SetEstimatedHalfDays(4).
		SaveX(ctx)
	if d.Status.String() != "draft" {
		t.Errorf("默认状态 = %s, want draft", d.Status)
	}

	// 一个需求只能出现在一张账单里
	b := client.Bill.Create().SetName("八月账单").SetDailyRate(1200).SetBaseFee(0).
		SetTotalHalfDays(0).SetTotalAmount(0).SaveX(ctx)
	client.BillItem.Create().SetBill(b).SetDemandID(d.ID).SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).SetHalfDays(4).SetAmount(2400).SaveX(ctx)
	_, err := client.BillItem.Create().SetBill(b).SetDemandID(d.ID).SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).SetHalfDays(4).SetAmount(2400).Save(ctx)
	if err == nil {
		t.Error("同一需求重复入账应违反唯一约束")
	}

	_ = ent.Asc
}
