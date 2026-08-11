package service

import (
	"context"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/hook"
)

// TestDemandUpdateProjectsConcurrentDeleteMapsToBadRequest 复现校验通过后、写入前项目被
// 并发删除的竞态窗口：借 ent mutation hook 在 AddProjectIDs 真正落库前插入一次项目删除，
// 断言外键约束冲突被 UpdateProjects 映射为 ErrBadRequest 而非原始 500 错误
func TestDemandUpdateProjectsConcurrentDeleteMapsToBadRequest(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-race-update")
	ctx := context.Background()

	p := client.Project.Create().SetName("并发删除项目").SaveX(ctx)
	d, err := svc.Create(ctx, admin, "需求", "", 0, nil, false, nil, nil, "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// normalizeProjectIDs 的存在性校验是一次独立查询，不会被此 hook 拦截；
	// hook 只拦截随后真正写 join 表的 Demand 更新 mutation，在其真正执行前删掉项目，
	// 从而复现「校验时还在、写入时已被删」的并发窗口
	client.Demand.Use(func(next ent.Mutator) ent.Mutator {
		return hook.DemandFunc(func(ctx context.Context, m *ent.DemandMutation) (ent.Value, error) {
			client.Project.DeleteOneID(p.ID).ExecX(ctx)
			return next.Mutate(ctx, m)
		})
	})

	_, err = svc.UpdateProjects(ctx, admin, d.ID, []int{p.ID})
	var svcErr *Error
	if !errors.As(err, &svcErr) || svcErr.Code != 40000 || svcErr.Message != "项目不存在" {
		t.Fatalf("外键约束冲突应映射为 ErrBadRequest(项目不存在), got %v", err)
	}
}

// TestDemandCreateConcurrentDeleteMapsToBadRequest 同上，覆盖 Create 路径：
// 创建需求时携带的项目 ID 在校验通过、写入前被并发删除，同样应转为业务错误而非 500
func TestDemandCreateConcurrentDeleteMapsToBadRequest(t *testing.T) {
	client, svc := newDemandEnv(t, "dproj-race-create")
	ctx := context.Background()

	p := client.Project.Create().SetName("并发删除项目").SaveX(ctx)

	client.Demand.Use(func(next ent.Mutator) ent.Mutator {
		return hook.DemandFunc(func(ctx context.Context, m *ent.DemandMutation) (ent.Value, error) {
			client.Project.DeleteOneID(p.ID).ExecX(ctx)
			return next.Mutate(ctx, m)
		})
	})

	_, err := svc.Create(ctx, admin, "需求", "", 0, nil, false, []int{p.ID}, nil, "")
	var svcErr *Error
	if !errors.As(err, &svcErr) || svcErr.Code != 40000 || svcErr.Message != "项目或标签不存在" {
		t.Fatalf("外键约束冲突应映射为 ErrBadRequest(项目或标签不存在), got %v", err)
	}
}
