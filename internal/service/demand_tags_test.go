package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// tagFixtures 建两个标签供关联测试使用
func tagFixtures(t *testing.T, client *ent.Client) (int, int) {
	t.Helper()
	ctx := context.Background()

	t1 := client.Tag.Create().SetName("新功能").SetColor("#112233").SaveX(ctx)
	t2 := client.Tag.Create().SetName("优化").SetColor("#445566").SaveX(ctx)

	return t1.ID, t2.ID
}

// TestDemandCreateWithTags 创建需求携带标签关联，并校验无效标签 ID 被拒绝
func TestDemandCreateWithTags(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-create")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)

	d, err := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, []int{t1, t2, t1}, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got := svc.mustGet(ctx, t, d.ID)
	if len(got.Edges.Tags) != 2 {
		t.Errorf("关联标签数 = %d, want 2（重复 ID 应去重）", len(got.Edges.Tags))
	}

	if _, err = svc.Create(ctx, admin, "需求二", "", 0, nil, false, nil, []int{999}, ""); err == nil {
		t.Error("不存在的标签 ID 应报错")
	}
}

// TestDemandUpdateTags 覆盖任意状态改标签、全量覆盖与清空
func TestDemandUpdateTags(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-update")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)

	d, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false, nil, []int{t1}, "")

	// 直接改库到 accepted，验证锁定态不受状态限制
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").ExecX(ctx)

	got, err := svc.UpdateTags(ctx, admin, d.ID, []int{t2})
	if err != nil {
		t.Fatalf("已验收需求改标签失败: %v", err)
	}
	if len(got.Edges.Tags) != 1 || got.Edges.Tags[0].ID != t2 {
		t.Errorf("覆盖结果异常: %+v", got.Edges.Tags)
	}

	// 空数组清空
	got, err = svc.UpdateTags(ctx, admin, d.ID, nil)
	if err != nil {
		t.Fatalf("清空标签失败: %v", err)
	}
	if len(got.Edges.Tags) != 0 {
		t.Errorf("清空后仍有关联: %+v", got.Edges.Tags)
	}

	if _, err = svc.UpdateTags(ctx, admin, d.ID, []int{999}); err == nil {
		t.Error("不存在的标签 ID 应报错")
	}
	if _, err = svc.UpdateTags(ctx, admin, 999, []int{t1}); err != ErrNotFound {
		t.Errorf("不存在的需求应返回 ErrNotFound, got %v", err)
	}
}

// TestDemandListFilterByTag 按标签筛选需求列表，并验证与项目筛选可叠加
func TestDemandListFilterByTag(t *testing.T) {
	client, svc := newDemandEnv(t, "dtag-list")
	ctx := context.Background()
	t1, t2 := tagFixtures(t, client)
	p := client.Project.Create().SetName("官网").SaveX(ctx)

	_, _ = svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p.ID}, []int{t1}, "")
	_, _ = svc.Create(ctx, admin, "需求二", "", 0, nil, false, nil, []int{t2}, "")
	_, _ = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil, nil, "")

	rows, err := svc.List(ctx, "", 0, t1, "")
	if err != nil || len(rows) != 1 || rows[0].Title != "需求一" {
		t.Fatalf("按标签筛选异常: %v, len=%d", err, len(rows))
	}

	// 项目与标签筛选叠加：项目命中但标签不命中应为空
	rows, _ = svc.List(ctx, "", p.ID, t2, "")
	if len(rows) != 0 {
		t.Errorf("叠加筛选应为空, len=%d", len(rows))
	}

	rows, _ = svc.List(ctx, "", 0, 0, "")
	if len(rows) != 3 {
		t.Errorf("不筛选应返回全部, len=%d", len(rows))
	}
	// 列表应预加载标签关联
	if rows[len(rows)-1].Edges.Tags == nil {
		t.Error("列表应预加载 Edges.Tags")
	}
}
