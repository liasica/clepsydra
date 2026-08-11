package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// projectFixtures 建两个项目供关联测试使用
func projectFixtures(t *testing.T, client *ent.Client) (int, int) {
	t.Helper()
	ctx := context.Background()

	p1 := client.Project.Create().SetName("官网").SaveX(ctx)
	p2 := client.Project.Create().SetName("小程序").SaveX(ctx)

	return p1.ID, p2.ID
}

// TestDemandCreateWithProjects 创建需求携带项目关联，并校验无效项目 ID 被拒绝
func TestDemandCreateWithProjects(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-create")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	d, err := svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1, p2, p1}, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got := svc.mustGet(ctx, t, d.ID)
	if len(got.Edges.Projects) != 2 {
		t.Errorf("关联项目数 = %d, want 2（重复 ID 应去重）", len(got.Edges.Projects))
	}

	if _, err = svc.Create(ctx, admin, "需求二", "", 0, nil, false, []int{999}, nil, ""); err == nil {
		t.Error("不存在的项目 ID 应报错")
	}
}

// TestDemandUpdateProjects 覆盖任意状态改标签、全量覆盖与清空
func TestDemandUpdateProjects(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-update")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	d, _ := svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1}, nil, "")

	// 推进到 accepted 之外的锁定态验证不受状态限制：直接改库到 accepted
	client.Demand.UpdateOneID(d.ID).SetStatus("accepted").ExecX(ctx)

	got, err := svc.UpdateProjects(ctx, admin, d.ID, []int{p2})
	if err != nil {
		t.Fatalf("已验收需求改标签失败: %v", err)
	}
	if len(got.Edges.Projects) != 1 || got.Edges.Projects[0].ID != p2 {
		t.Errorf("覆盖结果异常: %+v", got.Edges.Projects)
	}

	// 空数组清空
	got, err = svc.UpdateProjects(ctx, admin, d.ID, nil)
	if err != nil {
		t.Fatalf("清空标签失败: %v", err)
	}
	if len(got.Edges.Projects) != 0 {
		t.Errorf("清空后仍有关联: %+v", got.Edges.Projects)
	}

	if _, err = svc.UpdateProjects(ctx, admin, d.ID, []int{999}); err == nil {
		t.Error("不存在的项目 ID 应报错")
	}
	if _, err = svc.UpdateProjects(ctx, admin, 999, []int{p1}); err != ErrNotFound {
		t.Errorf("不存在的需求应返回 ErrNotFound, got %v", err)
	}
}

// TestDemandListFilterByProject 按项目筛选需求列表
func TestDemandListFilterByProject(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-list")
	ctx := context.Background()
	p1, p2 := projectFixtures(t, client)

	_, _ = svc.Create(ctx, admin, "需求一", "", 0, nil, false, []int{p1}, nil, "")
	_, _ = svc.Create(ctx, admin, "需求二", "", 0, nil, false, []int{p2}, nil, "")
	_, _ = svc.Create(ctx, admin, "需求三", "", 0, nil, false, nil, nil, "")
	_ = client

	rows, err := svc.List(ctx, "", p1, 0, "")
	if err != nil || len(rows) != 1 || rows[0].Title != "需求一" {
		t.Fatalf("按项目筛选异常: %v, len=%d", err, len(rows))
	}

	rows, _ = svc.List(ctx, "", 0, 0, "")
	if len(rows) != 3 {
		t.Errorf("不筛选应返回全部, len=%d", len(rows))
	}
	// 列表应预加载项目关联
	if rows[len(rows)-1].Edges.Projects == nil {
		t.Error("列表应预加载 Edges.Projects")
	}
}
