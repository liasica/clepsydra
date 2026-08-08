package schema

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"

	gen "clepsydra/internal/ent"
	"clepsydra/internal/ent/hook"
	"clepsydra/internal/ent/intercept"
)

// softDeleteKey 用于在 context 上标记「本次操作绕过软删除」
type softDeleteKey struct{}

// SkipSoftDelete 返回一个跳过软删除拦截的 context
// 查询时表示把已删除记录一并查出来，删除时表示执行物理删除
func SkipSoftDelete(parent context.Context) context.Context {
	return context.WithValue(parent, softDeleteKey{}, true)
}

// skipSoftDelete 判断当前 context 是否要求绕过软删除
func skipSoftDelete(ctx context.Context) bool {
	skip, _ := ctx.Value(softDeleteKey{}).(bool)

	return skip
}

// SoftDeleteMixin 软删除能力
//
// 挂上它的实体会多出 deleted_at 字段，并获得三条统一行为：
// 1. 查询自动附加 deleted_at IS NULL，不必在每个 Query 里手写条件
// 2. Delete 操作改写为写入 deleted_at 的 Update，记录得以保留
// 3. Update 操作同样附加 deleted_at IS NULL，已删除的记录不能再被改动
type SoftDeleteMixin struct {
	mixin.Schema
}

// Fields 软删除时间，为空表示未删除
func (SoftDeleteMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("deleted_at").Optional().Nillable(),
	}
}

// Interceptors 查询侧过滤掉已软删除的记录
func (d SoftDeleteMixin) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		intercept.TraverseFunc(func(ctx context.Context, q intercept.Query) error {
			if skipSoftDelete(ctx) {
				return nil
			}
			d.P(q)

			return nil
		}),
	}
}

// Hooks 变更侧改写删除语义，并把已删除记录挡在更新之外
func (d SoftDeleteMixin) Hooks() []ent.Hook {
	return []ent.Hook{
		hook.On(d.softDeleteHook, ent.OpDeleteOne|ent.OpDelete),
		hook.On(d.excludeDeletedHook, ent.OpUpdate|ent.OpUpdateOne),
	}
}

// softDeleteHook 把物理删除转成写 deleted_at 的更新
func (d SoftDeleteMixin) softDeleteHook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if skipSoftDelete(ctx) {
			return next.Mutate(ctx, m)
		}

		mx, ok := m.(interface {
			Client() *gen.Client
			SetDeletedAt(time.Time)
			SetOp(ent.Op)
			WhereP(...func(*sql.Selector))
		})
		if !ok {
			return nil, fmt.Errorf("软删除遇到未预期的 mutation 类型 %T", m)
		}

		// 已经删过的不再重复写入时间，保留首次删除的时刻
		d.P(mx)
		mx.SetOp(ent.OpUpdate)
		mx.SetDeletedAt(time.Now())

		return mx.Client().Mutate(ctx, m)
	})
}

// excludeDeletedHook 让更新语句跳过已软删除的记录
func (d SoftDeleteMixin) excludeDeletedHook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if skipSoftDelete(ctx) {
			return next.Mutate(ctx, m)
		}

		if mx, ok := m.(interface {
			WhereP(...func(*sql.Selector))
		}); ok {
			d.P(mx)
		}

		return next.Mutate(ctx, m)
	})
}

// P 给查询与变更追加「未删除」谓词
func (d SoftDeleteMixin) P(w interface{ WhereP(...func(*sql.Selector)) }) {
	w.WhereP(sql.FieldIsNull(d.Fields()[0].Descriptor().Name))
}
