package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

// newProjectEnv 构建 Project 测试环境
func newProjectEnv(t *testing.T, name string) (*ent.Client, *Project) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	return client, NewProject(client, NewAudit(client))
}

// TestProjectCRUD 覆盖创建、更新、删除与列表的正常路径
func TestProjectCRUD(t *testing.T) {
	_, svc := newProjectEnv(t, "pcrud")
	ctx := context.Background()

	p, err := svc.Create(ctx, admin, "小程序", "blue", "小程序相关需求")
	if err != nil {
		t.Fatalf("创建项目失败: %v", err)
	}
	if p.Name != "小程序" || p.Color != "blue" {
		t.Errorf("创建结果异常: %+v", p)
	}

	p, err = svc.Update(ctx, admin, p.ID, "小程序端", "green", "")
	if err != nil {
		t.Fatalf("更新项目失败: %v", err)
	}
	if p.Name != "小程序端" || p.Color != "green" || p.Remark != "" {
		t.Errorf("更新结果异常: %+v", p)
	}

	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("列表失败: %v, len=%d", err, len(rows))
	}

	if err = svc.Delete(ctx, admin, p.ID); err != nil {
		t.Fatalf("删除项目失败: %v", err)
	}
	rows, _ = svc.List(ctx)
	if len(rows) != 0 {
		t.Errorf("删除后列表应为空, len=%d", len(rows))
	}
}

// TestProjectValidation 覆盖空名称、重名与不存在记录的错误路径
func TestProjectValidation(t *testing.T) {
	_, svc := newProjectEnv(t, "pvalid")
	ctx := context.Background()

	if _, err := svc.Create(ctx, admin, "", "", ""); err == nil {
		t.Error("空名称应报错")
	}

	p1, _ := svc.Create(ctx, admin, "官网", "", "")
	p2, _ := svc.Create(ctx, admin, "后台", "", "")

	if _, err := svc.Create(ctx, admin, "官网", "", ""); err == nil {
		t.Error("重名创建应报错")
	}
	if _, err := svc.Update(ctx, admin, p2.ID, "官网", "", ""); err == nil {
		t.Error("更新为已有名称应报错")
	}
	// 名称不变的更新不应误报重名
	if _, err := svc.Update(ctx, admin, p1.ID, "官网", "red", ""); err != nil {
		t.Errorf("原名更新不应报错: %v", err)
	}

	if _, err := svc.Update(ctx, admin, 999, "任意", "", ""); err != ErrNotFound {
		t.Errorf("更新不存在项目应返回 ErrNotFound, got %v", err)
	}
	if err := svc.Delete(ctx, admin, 999); err != ErrNotFound {
		t.Errorf("删除不存在项目应返回 ErrNotFound, got %v", err)
	}
}

// TestProjectDemandCountAndDetach 关联需求数统计（软删需求不计入）与删除项目解除关联
func TestProjectDemandCountAndDetach(t *testing.T) {
	client, svc := newProjectEnv(t, "pcount")
	ctx := context.Background()

	p, _ := svc.Create(ctx, admin, "官网", "", "")
	// estimated_half_days 为必填字段，测试仅关心项目关联，取 0 即可
	d1 := client.Demand.Create().SetTitle("需求一").SetEstimatedHalfDays(0).AddProjectIDs(p.ID).SaveX(ctx)
	client.Demand.Create().SetTitle("需求二").SetEstimatedHalfDays(0).AddProjectIDs(p.ID).SaveX(ctx)

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

	// 删除项目自动解除关联，需求本身不受影响
	if err = svc.Delete(ctx, admin, p.ID); err != nil {
		t.Fatalf("删除项目失败: %v", err)
	}
	d2 := client.Demand.Query().WithProjects().AllX(ctx)
	for _, d := range d2 {
		if len(d.Edges.Projects) != 0 {
			t.Errorf("需求 %d 的项目关联应已解除", d.ID)
		}
	}
}
