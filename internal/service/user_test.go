package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent/enttest"
)

func TestUserCRUD(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:usercrud?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	svc := NewUser(client)

	// 创建需求方用户
	u, err := svc.Create(ctx, "jiafang", "pass1234", "甲方对接人", "client")
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	// 用户名重复拒绝
	if _, err = svc.Create(ctx, "jiafang", "x", "重复", "client"); err == nil {
		t.Error("重复用户名应拒绝")
	}

	// 非法角色拒绝
	if _, err = svc.Create(ctx, "bad", "x", "非法角色", "root"); err == nil {
		t.Error("非法角色应拒绝")
	}

	// 更新与禁用
	newName := "新名字"
	disabled := false
	u, err = svc.Update(ctx, u.ID, &newName, &disabled)
	if err != nil || u.Name != "新名字" || u.Enabled {
		t.Errorf("更新失败: %v, %+v", err, u)
	}

	// enabled 传 nil 表示未携带该字段，只改姓名不应影响启用状态
	renamed := "改名不改状态"
	u, err = svc.Update(ctx, u.ID, &renamed, nil)
	if err != nil || u.Name != "改名不改状态" || u.Enabled {
		t.Errorf("仅改姓名时启用状态被意外改动: %v, %+v", err, u)
	}

	// name 传 nil 表示未携带该字段，只改启用状态不应影响姓名（对应启停开关只传 enabled 的场景）
	enabled := true
	u, err = svc.Update(ctx, u.ID, nil, &enabled)
	if err != nil || u.Name != "改名不改状态" || !u.Enabled {
		t.Errorf("仅改启用状态时姓名被意外清空: %v, %+v", err, u)
	}
}
