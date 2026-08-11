package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/ent/project"
)

// Project 项目服务，轻量标签的增删改查
type Project struct {
	client *ent.Client
	audit  *Audit
}

// NewProject 构建项目服务
func NewProject(client *ent.Client, audit *Audit) *Project {
	return &Project{client: client, audit: audit}
}

// List 查询全部项目，预加载关联需求供 handler 统计关联数
// 关联需求查询会走 Demand 的软删除拦截器，已软删需求不计入
// handler 只需要数量，预加载仅取需求 id 列，避免整行加载
func (s *Project) List(ctx context.Context) ([]*ent.Project, error) {
	return s.client.Project.Query().
		WithDemands(func(q *ent.DemandQuery) {
			q.Select(demand.FieldID)
		}).
		Order(ent.Asc(project.FieldID)).
		All(ctx)
}

// Create 创建项目，名称必填且唯一
func (s *Project) Create(ctx context.Context, actor Actor, name, color, remark string) (*ent.Project, error) {
	if name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}

	exists, err := s.client.Project.Query().Where(project.Name(name)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("项目名称已存在")
	}

	p, err := s.client.Project.Create().
		SetName(name).
		SetColor(color).
		SetRemark(remark).
		Save(ctx)
	if err != nil {
		// Exist 与 Save 之间存在并发创建窗口，命中唯一约束时转 400 而非原生错误导致的 500
		if ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目名称已存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "project.create", "project", p.ID, map[string]any{
		"name": name,
	})

	return p, nil
}

// Update 全量更新项目名称、颜色与备注
func (s *Project) Update(ctx context.Context, actor Actor, id int, name, color, remark string) (*ent.Project, error) {
	if name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}

	// 重名检查排除自身，允许原名保存
	exists, err := s.client.Project.Query().
		Where(project.Name(name), project.IDNEQ(id)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("项目名称已存在")
	}

	p, err := s.client.Project.UpdateOneID(id).
		SetName(name).
		SetColor(color).
		SetRemark(remark).
		Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		// Exist 与 Save 之间存在并发改名窗口，命中唯一约束时转 400 而非原生错误导致的 500
		if ent.IsConstraintError(err) {
			return nil, ErrBadRequest("项目名称已存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "project.update", "project", id, map[string]any{
		"name": name,
	})

	return p, nil
}

// Delete 物理删除项目，与需求的关联由中间表外键级联清除，需求本身不受影响
func (s *Project) Delete(ctx context.Context, actor Actor, id int) error {
	p, err := s.client.Project.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if err = s.client.Project.DeleteOneID(id).Exec(ctx); err != nil {
		// Get 与 DeleteOneID 之间存在并发删除窗口，命中时转 404 而非原生 NotFound 导致的 500
		if ent.IsNotFound(err) {
			return ErrNotFound
		}

		return err
	}

	s.audit.Record(ctx, actor, "project.delete", "project", id, map[string]any{
		"name": p.Name,
	})

	return nil
}
