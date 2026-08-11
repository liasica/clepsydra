package service

import (
	"context"
	"database/sql"
	"errors"
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

// assertDuplicateNameBadRequest 断言错误被映射为指定文案的 40000 业务错误
func assertDuplicateNameBadRequest(t *testing.T, err error, message string) {
	t.Helper()

	var svcErr *Error
	if !errors.As(err, &svcErr) || svcErr.Code != 40000 || svcErr.Message != message {
		t.Fatalf("并发重名应映射为 ErrBadRequest(%s), got %v", message, err)
	}
}

// insertRawRow 用另一条指向同一共享内存库的连接直接插入一行，绕开 ent 抢占唯一名称
func insertRawRow(t *testing.T, ctx context.Context, dsn, query string, args ...any) {
	t.Helper()

	raw, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("打开并发连接失败: %v", err)
	}
	defer raw.Close()

	if _, err = raw.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("并发写入失败: %v", err)
	}
}

// TestProjectCreateConcurrentDuplicateMapsToBadRequest 复现 Exist 与 Save 之间的并发创建窗口：
// 借 ent mutation hook，在 Create 真正执行 INSERT 前，用另一条连接抢先插入同名行，
// 使 INSERT 命中唯一约束，断言 service 层将其映射为 ErrBadRequest 而非原样透传导致 500
func TestProjectCreateConcurrentDuplicateMapsToBadRequest(t *testing.T) {
	const dsn = "file:pcreate-race?mode=memory&cache=shared&_fk=1"
	client, svc := newProjectEnv(t, "pcreate-race")
	ctx := context.Background()

	client.Project.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op() == ent.OpCreate {
				insertRawRow(t, ctx, dsn,
					"INSERT INTO projects (name, color, remark, created_at, updated_at) VALUES (?, '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
					"并发创建项目")
			}

			return next.Mutate(ctx, m)
		})
	})

	_, err := svc.Create(ctx, admin, "并发创建项目", "", "")
	assertDuplicateNameBadRequest(t, err, "项目名称已存在")
}

// TestProjectUpdateConcurrentDuplicateMapsToBadRequest 复现 Exist 与 Save 之间的并发改名窗口：
// 在 UpdateOneID 真正执行 UPDATE 前，用另一条连接抢先插入目标名称的行，
// 使 UPDATE 命中唯一约束，断言 service 层将其映射为 ErrBadRequest
func TestProjectUpdateConcurrentDuplicateMapsToBadRequest(t *testing.T) {
	const dsn = "file:pupdate-race?mode=memory&cache=shared&_fk=1"
	client, svc := newProjectEnv(t, "pupdate-race")
	ctx := context.Background()

	p, err := svc.Create(ctx, admin, "原始项目", "", "")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	client.Project.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op() == ent.OpUpdateOne {
				insertRawRow(t, ctx, dsn,
					"INSERT INTO projects (name, color, remark, created_at, updated_at) VALUES (?, '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
					"并发改名项目")
			}

			return next.Mutate(ctx, m)
		})
	})

	_, err = svc.Update(ctx, admin, p.ID, "并发改名项目", "", "")
	assertDuplicateNameBadRequest(t, err, "项目名称已存在")
}
