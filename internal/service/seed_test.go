package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/ent/user"
	"clepsydra/internal/workday"
)

func TestSeedIdempotent(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:seed?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	adminCfg := config.Admin{Username: "admin", Password: "admin123"}
	entries := []workday.Entry{{Date: "2026-01-01", Type: "holiday", Name: "元旦"}}

	// 执行两次验证幂等
	if err := Seed(ctx, client, adminCfg, entries); err != nil {
		t.Fatalf("首次种子失败: %v", err)
	}
	if err := Seed(ctx, client, adminCfg, entries); err != nil {
		t.Fatalf("二次种子失败: %v", err)
	}

	// admin 只有一个
	if n := client.User.Query().Where(user.Username("admin")).CountX(ctx); n != 1 {
		t.Errorf("admin 数量 = %d, want 1", n)
	}

	// 默认设置齐全
	if n := client.Setting.Query().CountX(ctx); n != 6 {
		t.Errorf("设置项数量 = %d, want 6", n)
	}
	rate := client.Setting.Query().Where(setting.Key(SettingDailyRate)).OnlyX(ctx)
	if rate.Value != "1200" {
		t.Errorf("默认单价 = %s, want 1200", rate.Value)
	}

	// 节假日已导入
	if n := client.Holiday.Query().CountX(ctx); n != 1 {
		t.Errorf("节假日数量 = %d, want 1", n)
	}
}
