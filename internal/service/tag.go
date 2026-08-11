package service

import (
	"context"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/ent/tag"
)

// Tag 标签服务，需求性质标签的增删改查
// 颜色在创建时按名称生成并固化，更新只改名不动色，接口层不接受外部传入颜色
type Tag struct {
	client *ent.Client
	audit  *Audit
}

// NewTag 构建标签服务
func NewTag(client *ent.Client, audit *Audit) *Tag {
	return &Tag{client: client, audit: audit}
}

// List 查询全部标签，预加载关联需求供 handler 统计关联数
// 关联需求查询会走 Demand 的软删除拦截器，已软删需求不计入
// handler 只需要数量，预加载仅取需求 id 列，避免整行加载
func (s *Tag) List(ctx context.Context) ([]*ent.Tag, error) {
	return s.client.Tag.Query().
		WithDemands(func(q *ent.DemandQuery) {
			q.Select(demand.FieldID)
		}).
		Order(ent.Asc(tag.FieldID)).
		All(ctx)
}

// Create 创建标签，名称必填且唯一，颜色按名称生成后固化
func (s *Tag) Create(ctx context.Context, actor Actor, name string) (*ent.Tag, error) {
	if name == "" {
		return nil, ErrBadRequest("标签名称不能为空")
	}

	exists, err := s.client.Tag.Query().Where(tag.Name(name)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("标签名称已存在")
	}

	t, err := s.client.Tag.Create().
		SetName(name).
		SetColor(tagColor(name)).
		Save(ctx)
	if err != nil {
		// Exist 与 Save 之间存在并发创建窗口，命中唯一约束时转 400 而非原生错误导致的 500
		if ent.IsConstraintError(err) {
			return nil, ErrBadRequest("标签名称已存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "tag.create", "tag", t.ID, map[string]any{
		"name": name,
	})

	return t, nil
}

// Update 更新标签名称；颜色保持创建时的固化值不变
func (s *Tag) Update(ctx context.Context, actor Actor, id int, name string) (*ent.Tag, error) {
	if name == "" {
		return nil, ErrBadRequest("标签名称不能为空")
	}

	// 重名检查排除自身，允许原名保存
	exists, err := s.client.Tag.Query().
		Where(tag.Name(name), tag.IDNEQ(id)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("标签名称已存在")
	}

	t, err := s.client.Tag.UpdateOneID(id).
		SetName(name).
		Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		// Exist 与 Save 之间存在并发改名窗口，命中唯一约束时转 400 而非原生错误导致的 500
		if ent.IsConstraintError(err) {
			return nil, ErrBadRequest("标签名称已存在")
		}

		return nil, err
	}

	s.audit.Record(ctx, actor, "tag.update", "tag", id, map[string]any{
		"name": name,
	})

	return t, nil
}

// Delete 物理删除标签，与需求的关联由中间表外键级联清除，需求本身不受影响
func (s *Tag) Delete(ctx context.Context, actor Actor, id int) error {
	t, err := s.client.Tag.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if err = s.client.Tag.DeleteOneID(id).Exec(ctx); err != nil {
		// Get 与 DeleteOneID 之间存在并发删除窗口，命中时转 404 而非原生 NotFound 导致的 500
		if ent.IsNotFound(err) {
			return ErrNotFound
		}

		return err
	}

	s.audit.Record(ctx, actor, "tag.delete", "tag", id, map[string]any{
		"name": t.Name,
	})

	return nil
}
