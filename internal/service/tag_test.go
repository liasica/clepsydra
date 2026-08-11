package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

// newTagEnv 构建 Tag 测试环境
func newTagEnv(t *testing.T, name string) (*ent.Client, *Tag) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	return client, NewTag(client, NewAudit(client))
}

// TestTagCRUD 覆盖创建、更新、删除与列表的正常路径，重点校验颜色生成与固化
func TestTagCRUD(t *testing.T) {
	_, svc := newTagEnv(t, "tcrud")
	ctx := context.Background()

	tg, err := svc.Create(ctx, admin, "优化")
	if err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	if tg.Name != "优化" {
		t.Errorf("创建结果异常: %+v", tg)
	}
	if tg.Color != tagColor("优化") {
		t.Errorf("颜色应由名称生成: got %s, want %s", tg.Color, tagColor("优化"))
	}

	// 改名不重算颜色：固化的是创建时的色值
	created := tg.Color
	tg, err = svc.Update(ctx, admin, tg.ID, "性能优化")
	if err != nil {
		t.Fatalf("更新标签失败: %v", err)
	}
	if tg.Name != "性能优化" {
		t.Errorf("更新结果异常: %+v", tg)
	}
	if tg.Color != created {
		t.Errorf("改名后颜色应保持固化值: got %s, want %s", tg.Color, created)
	}

	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("列表失败: %v, len=%d", err, len(rows))
	}

	if err = svc.Delete(ctx, admin, tg.ID); err != nil {
		t.Fatalf("删除标签失败: %v", err)
	}
	rows, _ = svc.List(ctx)
	if len(rows) != 0 {
		t.Errorf("删除后列表应为空, len=%d", len(rows))
	}
}

// TestTagValidation 覆盖空名称、重名与不存在记录的错误路径
func TestTagValidation(t *testing.T) {
	_, svc := newTagEnv(t, "tvalid")
	ctx := context.Background()

	if _, err := svc.Create(ctx, admin, ""); err == nil {
		t.Error("空名称应报错")
	}

	t1, _ := svc.Create(ctx, admin, "新功能")
	t2, _ := svc.Create(ctx, admin, "缺陷修复")

	if _, err := svc.Create(ctx, admin, "新功能"); err == nil {
		t.Error("重名创建应报错")
	}
	if _, err := svc.Update(ctx, admin, t2.ID, "新功能"); err == nil {
		t.Error("更新为已有名称应报错")
	}
	// 名称不变的更新不应误报重名
	if _, err := svc.Update(ctx, admin, t1.ID, "新功能"); err != nil {
		t.Errorf("原名更新不应报错: %v", err)
	}

	if _, err := svc.Update(ctx, admin, 999, "任意"); err != ErrNotFound {
		t.Errorf("更新不存在标签应返回 ErrNotFound, got %v", err)
	}
	if err := svc.Delete(ctx, admin, 999); err != ErrNotFound {
		t.Errorf("删除不存在标签应返回 ErrNotFound, got %v", err)
	}
}

// TestTagDemandCountAndDetach 关联需求数统计（软删需求不计入）与删除标签解除关联
func TestTagDemandCountAndDetach(t *testing.T) {
	client, svc := newTagEnv(t, "tcount")
	ctx := context.Background()

	tg, _ := svc.Create(ctx, admin, "优化")
	// estimated_half_days 为必填字段，测试仅关心标签关联，取 0 即可
	d1 := client.Demand.Create().SetTitle("需求一").SetEstimatedHalfDays(0).AddTagIDs(tg.ID).SaveX(ctx)
	client.Demand.Create().SetTitle("需求二").SetEstimatedHalfDays(0).AddTagIDs(tg.ID).SaveX(ctx)

	rows, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if n := len(rows[0].Edges.Demands); n != 2 {
		t.Errorf("关联需求数 = %d, want 2", n)
	}

	// 软删需求后不再计入
	client.Demand.DeleteOneID(d1.ID).ExecX(ctx)
	rows, _ = svc.List(ctx)
	if n := len(rows[0].Edges.Demands); n != 1 {
		t.Errorf("软删后关联需求数 = %d, want 1", n)
	}

	// 删除标签自动解除关联，需求本身不受影响
	if err = svc.Delete(ctx, admin, tg.ID); err != nil {
		t.Fatalf("删除标签失败: %v", err)
	}
	demands := client.Demand.Query().WithTags().AllX(ctx)
	for _, d := range demands {
		if len(d.Edges.Tags) != 0 {
			t.Errorf("需求 %d 的标签关联应已解除", d.ID)
		}
	}
}
