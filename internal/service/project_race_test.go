package service

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
)

// TestProjectDeleteConcurrentDeleteMapsToNotFound 复现 Get 与 DeleteOneID 之间的并发删除窗口：
// 借 ent mutation hook，在 DeleteOneID 真正执行 DELETE 语句前，用另一条指向同一共享内存库的
// 连接把这行提前删掉，使 DeleteOneID 的 DELETE 影响 0 行、触发 ent 原生 NotFoundError，
// 断言 service 层将其映射为 ErrNotFound 而非原样透传导致 500
func TestProjectDeleteConcurrentDeleteMapsToNotFound(t *testing.T) {
	const dsn = "file:pdelete-race?mode=memory&cache=shared&_fk=1"
	client, svc := newProjectEnv(t, "pdelete-race")
	ctx := context.Background()

	p, err := svc.Create(ctx, admin, "并发删除项目", "", "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	client.Project.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op() == ent.OpDeleteOne {
				raw, openErr := sql.Open("sqlite3", dsn)
				if openErr != nil {
					t.Fatalf("打开并发连接失败: %v", openErr)
				}
				if _, execErr := raw.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", p.ID); execErr != nil {
					t.Fatalf("并发删除失败: %v", execErr)
				}
				raw.Close()
			}

			return next.Mutate(ctx, m)
		})
	})

	if err = svc.Delete(ctx, admin, p.ID); err != ErrNotFound {
		t.Fatalf("Get 与 DeleteOneID 之间被并发删除应映射为 ErrNotFound, got %v", err)
	}
}
