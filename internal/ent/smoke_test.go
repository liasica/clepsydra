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

	// 账单账期唯一约束
	client.Bill.Create().SetPeriod("2026-07").SetDailyRate(1200).SetBaseFee(12000).
		SetTotalHalfDays(0).SetTotalAmount(12000).SaveX(ctx)
	_, err := client.Bill.Create().SetPeriod("2026-07").SetDailyRate(1200).SetBaseFee(12000).
		SetTotalHalfDays(0).SetTotalAmount(12000).Save(ctx)
	if err == nil {
		t.Error("重复账期应违反唯一约束")
	}

	_ = ent.Asc
}
