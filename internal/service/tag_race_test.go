package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// TestTagCreateConcurrentDuplicateMapsToBadRequest 复现 Exist 与 Save 之间的并发创建窗口：
// 借 ent mutation hook，在 Create 真正执行 INSERT 前，用另一条连接抢先插入同名行，
// 使 INSERT 命中唯一约束，断言 service 层将其映射为 ErrBadRequest 而非原样透传导致 500
func TestTagCreateConcurrentDuplicateMapsToBadRequest(t *testing.T) {
	const dsn = "file:tcreate-race?mode=memory&cache=shared&_fk=1"
	client, svc := newTagEnv(t, "tcreate-race")
	ctx := context.Background()

	client.Tag.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op() == ent.OpCreate {
				insertRawRow(t, ctx, dsn,
					"INSERT INTO tags (name, color, created_at, updated_at) VALUES (?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
					"并发创建标签")
			}

			return next.Mutate(ctx, m)
		})
	})

	_, err := svc.Create(ctx, admin, "并发创建标签")
	assertDuplicateNameBadRequest(t, err, "标签名称已存在")
}

// TestTagUpdateConcurrentDuplicateMapsToBadRequest 复现 Exist 与 Save 之间的并发改名窗口：
// 在 UpdateOneID 真正执行 UPDATE 前，用另一条连接抢先插入目标名称的行，
// 使 UPDATE 命中唯一约束，断言 service 层将其映射为 ErrBadRequest
func TestTagUpdateConcurrentDuplicateMapsToBadRequest(t *testing.T) {
	const dsn = "file:tupdate-race?mode=memory&cache=shared&_fk=1"
	client, svc := newTagEnv(t, "tupdate-race")
	ctx := context.Background()

	tg, err := svc.Create(ctx, admin, "原始标签")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	client.Tag.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op() == ent.OpUpdateOne {
				insertRawRow(t, ctx, dsn,
					"INSERT INTO tags (name, color, created_at, updated_at) VALUES (?, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
					"并发改名标签")
			}

			return next.Mutate(ctx, m)
		})
	})

	_, err = svc.Update(ctx, admin, tg.ID, "并发改名标签")
	assertDuplicateNameBadRequest(t, err, "标签名称已存在")
}
